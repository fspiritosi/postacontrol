// Package taskapp es el cliente REST de Taskapp (tareas y tickets).
package taskapp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

var ErrUnauthorized = errors.New("taskapp: el token fue rechazado")

// APIError es una respuesta no exitosa de la API; Msg sale del campo "error".
type APIError struct {
	Status int
	Msg    string
}

func (e *APIError) Error() string { return e.Msg }

type Client struct {
	BaseURL string // API, sin barra final
	Token   string
	HTTP    *http.Client // nil = http.DefaultClient
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

// do hace el pedido, mapea 401 a ErrUnauthorized y el resto de los no-2xx a
// *APIError. in es JSON, salvo *multipartBody (ya armado).
func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	ctype := ""
	if mp, ok := in.(*multipartBody); ok {
		body, ctype = bytes.NewReader(mp.data), mp.ctype
	} else if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body, ctype = bytes.NewReader(b), "application/json"
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%w", ErrUnauthorized)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var e struct {
			Error string `json:"error"`
		}
		msg := ""
		if json.Unmarshal(raw, &e) == nil {
			msg = e.Error
		}
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return &APIError{Status: resp.StatusCode, Msg: msg}
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

type multipartBody struct {
	data  []byte
	ctype string
}

func (c *Client) Me(ctx context.Context) (User, error) {
	var w wireUser
	if err := c.do(ctx, "GET", "/api/auth/me", nil, &w); err != nil {
		return User{}, err
	}
	return User{ID: w.ID, Name: w.Name, Email: w.Email}, nil
}

func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var ws []wireProject
	if err := c.do(ctx, "GET", "/api/projects", nil, &ws); err != nil {
		return nil, err
	}
	out := make([]Project, 0, len(ws))
	for _, w := range ws {
		out = append(out, Project{ID: w.ID, WorkspaceID: w.WorkspaceID, Name: w.Name,
			Color: w.Color, Status: w.Status, ApproverEmails: w.ApproverEmails})
	}
	return out, nil
}

// ListMyPending devuelve las tareas y tickets asignados, sin las completadas.
func (c *Client) ListMyPending(ctx context.Context, assigneeID int64) ([]Task, error) {
	var ws []wireTask
	q := url.Values{"assignee_id": {strconv.FormatInt(assigneeID, 10)}}
	if err := c.do(ctx, "GET", "/api/tasks?"+q.Encode(), nil, &ws); err != nil {
		return nil, err
	}
	out := make([]Task, 0, len(ws))
	for _, w := range ws {
		if w.Status.IsCompleted {
			continue
		}
		out = append(out, w.task())
	}
	return out, nil
}

func (c *Client) GetTask(ctx context.Context, id int64) (Task, error) {
	var w wireTask
	if err := c.do(ctx, "GET", "/api/tasks/"+strconv.FormatInt(id, 10), nil, &w); err != nil {
		return Task{}, err
	}
	return w.task(), nil
}

func (c *Client) CreateTask(ctx context.Context, t NewTask) (Task, error) {
	in := map[string]any{
		"workspace_id": t.WorkspaceID,
		"title":        t.Title,
		"description":  t.Description,
		"priority":     t.Priority,
	}
	if t.ProjectID != 0 {
		in["project_id"] = t.ProjectID
	}
	if t.DueDate != "" {
		d, err := time.Parse("2006-01-02", t.DueDate)
		if err != nil {
			return Task{}, fmt.Errorf("fecha límite inválida %q: %w", t.DueDate, err)
		}
		in["due_date"] = d.UTC().Format(time.RFC3339)
	}
	var w wireTask
	if err := c.do(ctx, "POST", "/api/tasks", in, &w); err != nil {
		return Task{}, err
	}
	return w.task(), nil
}

func (c *Client) CreateTicket(ctx context.Context, t NewTicket) (Task, error) {
	in := map[string]any{
		"title":       t.Title,
		"description": t.Description,
		"priority":    t.Priority,
	}
	if t.ProjectID != 0 {
		in["project_id"] = t.ProjectID
	}
	var w wireTask
	if err := c.do(ctx, "POST", "/api/tickets", in, &w); err != nil {
		return Task{}, err
	}
	return w.task(), nil
}

// Upload sube un archivo y devuelve su key para usar en attachments.
func (c *Client) Upload(ctx context.Context, filename string, data []byte) (string, error) {
	if len(data) > MaxUpload {
		return "", fmt.Errorf("el archivo %q supera el máximo de %d MB", filename, MaxUpload>>20)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := fw.Write(data); err != nil {
		return "", err
	}
	if err := mw.Close(); err != nil {
		return "", err
	}
	var out struct {
		Key string `json:"key"`
	}
	if err := c.do(ctx, "POST", "/api/upload", &multipartBody{buf.Bytes(), mw.FormDataContentType()}, &out); err != nil {
		return "", err
	}
	return out.Key, nil
}

// SetAttachments reemplaza la lista completa de adjuntos de la tarea.
func (c *Client) SetAttachments(ctx context.Context, taskID int64, keys []string) error {
	if keys == nil {
		keys = []string{}
	}
	return c.do(ctx, "PUT", "/api/tasks/"+strconv.FormatInt(taskID, 10),
		map[string]any{"attachments": keys}, nil)
}

func (c *Client) AddComment(ctx context.Context, taskID int64, bodyHTML string, attachments []string, internal bool) error {
	if attachments == nil {
		attachments = []string{}
	}
	return c.do(ctx, "POST", "/api/tasks/"+strconv.FormatInt(taskID, 10)+"/comments", map[string]any{
		"body":        bodyHTML,
		"attachments": attachments,
		"is_internal": internal,
	}, nil)
}

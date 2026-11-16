package taskapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// servidor arma un httptest.Server que valida método, ruta y token, y
// entrega el cuerpo JSON recibido al callback.
func servidor(t *testing.T, method, path string, status int, resp string, check func(body map[string]any, r *http.Request)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, method, r.Method)
		assert.Equal(t, path, r.URL.RequestURI())
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		if check != nil {
			var m map[string]any
			if r.Header.Get("Content-Type") == "application/json" {
				b, _ := io.ReadAll(r.Body)
				require.NoError(t, json.Unmarshal(b, &m))
			}
			check(m, r)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(resp))
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, Token: "tok"}
}

func TestCreateTask_SendsWorkspaceAndProject(t *testing.T) {
	c := servidor(t, "POST", "/api/tasks", 201, `{"id":9,"project_id":5,"title":"T","source":"internal","status":{"id":1,"name":"Pendiente"},"due_date":"2026-12-01T00:00:00Z"}`,
		func(b map[string]any, _ *http.Request) {
			assert.EqualValues(t, 2, b["workspace_id"])
			assert.EqualValues(t, 5, b["project_id"])
			assert.Equal(t, "T", b["title"])
			assert.Equal(t, "<p>d</p>", b["description"])
			assert.Equal(t, "none", b["priority"])
			assert.Equal(t, "2026-12-01T00:00:00Z", b["due_date"])
		})
	got, err := c.CreateTask(context.Background(), NewTask{WorkspaceID: 2, ProjectID: 5, Title: "T", Description: "<p>d</p>", Priority: "none", DueDate: "2026-12-01"})
	require.NoError(t, err)
	assert.EqualValues(t, 9, got.ID)
	assert.Equal(t, "2026-12-01", got.DueDate)
	assert.False(t, got.Ticket)

	c = servidor(t, "POST", "/api/tasks", 201, `{"id":10}`, func(b map[string]any, _ *http.Request) {
		assert.NotContains(t, b, "due_date")
	})
	_, err = c.CreateTask(context.Background(), NewTask{WorkspaceID: 2, ProjectID: 5, Title: "T", Priority: "none"})
	require.NoError(t, err)
}

func TestCreateTicket_NoDueDate(t *testing.T) {
	c := servidor(t, "POST", "/api/tickets", 201, `{"id":3,"source":"widget"}`, func(b map[string]any, _ *http.Request) {
		assert.NotContains(t, b, "due_date")
		assert.NotContains(t, b, "project_id")
		assert.Equal(t, "medium", b["priority"])
	})
	got, err := c.CreateTicket(context.Background(), NewTicket{Title: "x", Priority: "medium"})
	require.NoError(t, err)
	assert.True(t, got.Ticket)

	c = servidor(t, "POST", "/api/tickets", 201, `{"id":4}`, func(b map[string]any, _ *http.Request) {
		assert.EqualValues(t, 7, b["project_id"])
	})
	_, err = c.CreateTicket(context.Background(), NewTicket{ProjectID: 7, Title: "x"})
	require.NoError(t, err)
}

func TestListMyPending_DropsCompleted(t *testing.T) {
	c := servidor(t, "GET", "/api/tasks?assignee_id=1", 200, `[
		{"id":1,"status":{"id":1,"is_completed":false},"due_date":null},
		{"id":2,"status":{"id":6,"is_completed":true}},
		{"id":3,"status":{"id":1},"source":"widget","due_date":"2026-12-01T10:30:00.123456-03:00","attachments":["a"]}]`, nil)
	got, err := c.ListMyPending(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.EqualValues(t, 1, got[0].ID)
	assert.Equal(t, "", got[0].DueDate)
	assert.EqualValues(t, 3, got[1].ID)
	assert.True(t, got[1].Ticket)
	assert.Equal(t, "2026-12-01", got[1].DueDate)
	assert.Equal(t, []string{"a"}, got[1].Attachments)
}

func TestMeProjectsGetTask(t *testing.T) {
	c := servidor(t, "GET", "/api/auth/me", 200, `{"id":1,"name":"Fab","email":"f@x.com","role":"admin"}`, nil)
	u, err := c.Me(context.Background())
	require.NoError(t, err)
	assert.Equal(t, User{ID: 1, Name: "Fab", Email: "f@x.com"}, u)

	c = servidor(t, "GET", "/api/projects", 200, `[{"id":3,"workspace_id":2,"name":"P","status":"active","color":"#fff","api_key":"SECRET","approver_emails":["a@x.com"]}]`, nil)
	ps, err := c.Projects(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Project{{ID: 3, WorkspaceID: 2, Name: "P", Color: "#fff", Status: "active", ApproverEmails: []string{"a@x.com"}}}, ps)

	c = servidor(t, "GET", "/api/tasks/8", 200, `{"id":8,"due_date":"2026-12-05"}`, nil)
	tk, err := c.GetTask(context.Background(), 8)
	require.NoError(t, err)
	assert.Equal(t, "2026-12-05", tk.DueDate)
}

func TestUnauthorized(t *testing.T) {
	c := servidor(t, "GET", "/api/auth/me", 401, `{"error":"missing authorization header"}`, nil)
	_, err := c.Me(context.Background())
	assert.True(t, errors.Is(err, ErrUnauthorized))
}

func TestAPIError_UsesErrorField(t *testing.T) {
	c := servidor(t, "POST", "/api/tasks/1/comments", 422, `{"error":"cannot comment on a completed task"}`, nil)
	err := c.AddComment(context.Background(), 1, "hola", nil, false)
	var ae *APIError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, 422, ae.Status)
	assert.Equal(t, "cannot comment on a completed task", ae.Error())

	c = servidor(t, "GET", "/api/auth/me", 500, `no json`, nil)
	_, err = c.Me(context.Background())
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, "Internal Server Error", ae.Msg)
}

func TestUpload_Multipart(t *testing.T) {
	c := servidor(t, "POST", "/api/upload", 201, `{"key":"internal/u.png","url":"http://x"}`, func(_ map[string]any, r *http.Request) {
		assert.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		f, h, err := r.FormFile("file")
		require.NoError(t, err)
		defer f.Close()
		b, _ := io.ReadAll(f)
		assert.Equal(t, "a.png", h.Filename)
		assert.Equal(t, "datos", string(b))
	})
	key, err := c.Upload(context.Background(), "a.png", []byte("datos"))
	require.NoError(t, err)
	assert.Equal(t, "internal/u.png", key)
}

func TestUpload_TooBig(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, Token: "tok"}
	_, err := c.Upload(context.Background(), "big.bin", make([]byte, MaxUpload+1))
	require.Error(t, err)
	assert.False(t, called)
}

func TestAddComment_Internal(t *testing.T) {
	c := servidor(t, "POST", "/api/tasks/4/comments", 201, `{"id":1}`, func(b map[string]any, _ *http.Request) {
		assert.Equal(t, "<p>x</p>", b["body"])
		assert.Equal(t, true, b["is_internal"])
		assert.Equal(t, []any{"k1"}, b["attachments"])
	})
	require.NoError(t, c.AddComment(context.Background(), 4, "<p>x</p>", []string{"k1"}, true))
}

func TestSetAttachments_FullList(t *testing.T) {
	c := servidor(t, "PUT", "/api/tasks/4", 200, `{"id":4}`, func(b map[string]any, _ *http.Request) {
		assert.Equal(t, []any{"k1", "k2"}, b["attachments"])
		assert.Len(t, b, 1)
	})
	require.NoError(t, c.SetAttachments(context.Background(), 4, []string{"k1", "k2"}))
}

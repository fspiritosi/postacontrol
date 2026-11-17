package taskapp

import (
	"strings"
	"time"
)

// MaxUpload es el tamaño máximo de un archivo que acepta Taskapp (20 MB).
const MaxUpload = 20 << 20

type User struct {
	ID          int64
	Name, Email string
}

type Status struct {
	ID        int64
	Name      string
	Completed bool
}

// Project no guarda la api_key que devuelve la API: nunca se decodifica.
type Project struct {
	ID, WorkspaceID     int64
	Name, Color, Status string
	ApproverEmails      []string
}

type Task struct {
	ID          int64
	ProjectID   int64
	Title       string
	Description string
	Priority    string
	DueDate     string // AAAA-MM-DD o ""
	Status      Status
	Ticket      bool // source == "widget"
	Attachments []string
}

type NewTask struct {
	WorkspaceID, ProjectID                int64
	Title, Description, Priority, DueDate string
}

type NewTicket struct {
	ProjectID                    int64 // 0 = sin proyecto
	Title, Description, Priority string
}

// --- formas de cable (snake_case) ---

type wireUser struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type wireProject struct {
	ID             int64    `json:"id"`
	WorkspaceID    int64    `json:"workspace_id"`
	Name           string   `json:"name"`
	Color          string   `json:"color"`
	Status         string   `json:"status"`
	ApproverEmails []string `json:"approver_emails"`
}

type wireTask struct {
	ID          int64    `json:"id"`
	ProjectID   *int64   `json:"project_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Priority    string   `json:"priority"`
	DueDate     *string  `json:"due_date"`
	Source      string   `json:"source"`
	Attachments []string `json:"attachments"`
	Status      struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		IsCompleted bool   `json:"is_completed"`
	} `json:"status"`
}

func (w wireTask) task() Task {
	t := Task{
		ID: w.ID, Title: w.Title, Description: w.Description, Priority: w.Priority,
		Ticket:      w.Source == "widget",
		Attachments: w.Attachments,
		Status:      Status{ID: w.Status.ID, Name: w.Status.Name, Completed: w.Status.IsCompleted},
	}
	if w.ProjectID != nil {
		t.ProjectID = *w.ProjectID
	}
	if w.DueDate != nil {
		t.DueDate = parseFecha(*w.DueDate)
	}
	return t
}

// parseFecha acepta RFC 3339 (con o sin fracciones, cualquier zona) o
// AAAA-MM-DD, y devuelve AAAA-MM-DD ("" si no se entiende).
func parseFecha(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.Format("2006-01-02")
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t.Format("2006-01-02")
	}
	return ""
}

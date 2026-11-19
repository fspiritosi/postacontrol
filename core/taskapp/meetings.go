package taskapp

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

type Workspace struct {
	ID   int64
	Name string
}

type Member struct {
	ID          int64
	Name, Email string
}

type MeetingAttendee struct {
	UserID            int64
	Name, Email, Rsvp string // Rsvp: pending, accepted, declined, maybe
}

type Meeting struct {
	ID, WorkspaceID, ProjectID                        int64  // ProjectID 0 = sin proyecto
	ProjectName, Title, Description, Location, Status string // Status: scheduled, completed, cancelled
	Start, End                                        time.Time
	AllDay                                            bool
	CreatedBy                                         int64
	Attendees                                         []MeetingAttendee
	UpdatedAt                                         time.Time
}

type NewMeeting struct {
	WorkspaceID, ProjectID       int64 // ProjectID 0 = sin proyecto (no se manda project_id)
	Title, Description, Location string
	Start, End                   time.Time
	AllDay                       bool
	AttendeeIDs                  []int64
}

// --- formas de cable (snake_case) ---

type wireWorkspace struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type wireMember struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type wireMeeting struct {
	ID          int64     `json:"id"`
	WorkspaceID int64     `json:"workspace_id"`
	ProjectID   *int64    `json:"project_id"`
	ProjectName *string   `json:"project_name"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	StartTime   time.Time `json:"start_time"`
	EndTime     time.Time `json:"end_time"`
	AllDay      bool      `json:"all_day"`
	CreatedBy   int64     `json:"created_by"`
	Status      string    `json:"status"`
	Attendees   []struct {
		UserID     int64  `json:"user_id"`
		Name       string `json:"name"`
		Email      string `json:"email"`
		RsvpStatus string `json:"rsvp_status"`
	} `json:"attendees"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (w wireMeeting) meeting() Meeting {
	m := Meeting{
		ID: w.ID, WorkspaceID: w.WorkspaceID,
		Title: w.Title, Description: w.Description, Location: w.Location, Status: w.Status,
		Start: w.StartTime.UTC(), End: w.EndTime.UTC(), AllDay: w.AllDay,
		CreatedBy: w.CreatedBy, UpdatedAt: w.UpdatedAt.UTC(),
		Attendees: make([]MeetingAttendee, 0, len(w.Attendees)),
	}
	if w.ProjectID != nil {
		m.ProjectID = *w.ProjectID
	}
	if w.ProjectName != nil {
		m.ProjectName = *w.ProjectName
	}
	for _, a := range w.Attendees {
		m.Attendees = append(m.Attendees, MeetingAttendee{UserID: a.UserID, Name: a.Name, Email: a.Email, Rsvp: a.RsvpStatus})
	}
	return m
}

func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error) {
	var ws []wireWorkspace
	if err := c.do(ctx, "GET", "/api/workspaces", nil, &ws); err != nil {
		return nil, err
	}
	out := make([]Workspace, 0, len(ws))
	for _, w := range ws {
		out = append(out, Workspace{ID: w.ID, Name: w.Name})
	}
	return out, nil
}

func (c *Client) Members(ctx context.Context, workspaceID int64) ([]Member, error) {
	var ms []wireMember
	if err := c.do(ctx, "GET", "/api/workspaces/"+strconv.FormatInt(workspaceID, 10)+"/members", nil, &ms); err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(ms))
	for _, m := range ms {
		out = append(out, Member{ID: m.ID, Name: m.Name, Email: m.Email})
	}
	return out, nil
}

// Meetings lista las reuniones del workspace. La API filtra solo por
// start_time (from <= start <= to): ensanchar el rango es cosa del llamador.
func (c *Client) Meetings(ctx context.Context, workspaceID int64, from, to time.Time) ([]Meeting, error) {
	q := url.Values{
		"workspace_id": {strconv.FormatInt(workspaceID, 10)},
		"from":         {from.UTC().Format(time.RFC3339)},
		"to":           {to.UTC().Format(time.RFC3339)},
	}
	var ws []wireMeeting
	if err := c.do(ctx, "GET", "/api/meetings?"+q.Encode(), nil, &ws); err != nil {
		return nil, err
	}
	out := make([]Meeting, 0, len(ws))
	for _, w := range ws {
		out = append(out, w.meeting())
	}
	return out, nil
}

func (c *Client) CreateMeeting(ctx context.Context, m NewMeeting) (Meeting, error) {
	ids := m.AttendeeIDs
	if ids == nil {
		ids = []int64{}
	}
	in := map[string]any{
		"workspace_id": m.WorkspaceID,
		"title":        m.Title,
		"description":  m.Description,
		"agenda":       "",
		"location":     m.Location,
		"start_time":   m.Start.UTC().Format(time.RFC3339),
		"end_time":     m.End.UTC().Format(time.RFC3339),
		"all_day":      m.AllDay,
		"attendee_ids": ids,
		"task_ids":     []int64{},
	}
	if m.ProjectID != 0 {
		in["project_id"] = m.ProjectID
	}
	var w wireMeeting
	if err := c.do(ctx, "POST", "/api/meetings", in, &w); err != nil {
		return Meeting{}, err
	}
	return w.meeting(), nil
}

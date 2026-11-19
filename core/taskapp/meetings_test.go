package taskapp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMeetings_Query(t *testing.T) {
	ba := time.FixedZone("ART", -3*3600)
	from := time.Date(2026, 12, 1, 0, 0, 0, 0, ba) // 03:00Z
	to := time.Date(2026, 12, 31, 0, 0, 0, 0, ba)
	resp := `[
	 {"id":4,"workspace_id":3,"project_id":null,"project_name":null,"title":"Sin proyecto","description":"<p>d</p>","location":"Sala",
	  "start_time":"2026-12-03T15:00:00Z","end_time":"2026-12-03T16:00:00Z","all_day":false,"created_by":1,"status":"scheduled",
	  "attendees":[{"user_id":2,"name":"Ana","email":"ana@example.com","rsvp_status":"accepted","responded_at":null,"attended":false}],
	  "updated_at":"2026-12-01T10:00:00.5Z"},
	 {"id":5,"workspace_id":3,"project_id":12,"project_name":"Proy","title":"Con proyecto","start_time":"2026-12-04T12:00:00-03:00",
	  "end_time":"2026-12-04T13:00:00-03:00","all_day":true,"created_by":2,"status":"cancelled","updated_at":"2026-12-02T10:00:00Z"}
	]`
	c := servidor(t, "GET", "/api/meetings?from=2026-12-01T03%3A00%3A00Z&to=2026-12-31T03%3A00%3A00Z&workspace_id=3", 200, resp,
		func(_ map[string]any, r *http.Request) {
			q := r.URL.Query()
			assert.Equal(t, "3", q.Get("workspace_id"))
			assert.Equal(t, "2026-12-01T03:00:00Z", q.Get("from"))
			assert.Equal(t, "2026-12-31T03:00:00Z", q.Get("to"))
		})
	got, err := c.Meetings(context.Background(), 3, from, to)
	require.NoError(t, err)
	require.Len(t, got, 2)

	m := got[0]
	assert.EqualValues(t, 4, m.ID)
	assert.EqualValues(t, 3, m.WorkspaceID)
	assert.EqualValues(t, 0, m.ProjectID)
	assert.Equal(t, "", m.ProjectName)
	assert.Equal(t, "Sin proyecto", m.Title)
	assert.Equal(t, "<p>d</p>", m.Description)
	assert.Equal(t, "Sala", m.Location)
	assert.Equal(t, "scheduled", m.Status)
	assert.True(t, m.Start.Equal(time.Date(2026, 12, 3, 15, 0, 0, 0, time.UTC)))
	assert.Equal(t, time.UTC, m.End.Location())
	assert.False(t, m.AllDay)
	assert.EqualValues(t, 1, m.CreatedBy)
	assert.Equal(t, []MeetingAttendee{{UserID: 2, Name: "Ana", Email: "ana@example.com", Rsvp: "accepted"}}, m.Attendees)
	assert.Equal(t, 500*time.Millisecond, m.UpdatedAt.Sub(time.Date(2026, 12, 1, 10, 0, 0, 0, time.UTC)))

	m = got[1]
	assert.EqualValues(t, 12, m.ProjectID)
	assert.Equal(t, "Proy", m.ProjectName)
	assert.True(t, m.AllDay)
	assert.Equal(t, "cancelled", m.Status)
	assert.Equal(t, time.Date(2026, 12, 4, 15, 0, 0, 0, time.UTC), m.Start)
	assert.NotNil(t, m.Attendees)
	assert.Empty(t, m.Attendees)
}

func TestCreateMeeting_Body(t *testing.T) {
	start := time.Date(2026, 12, 3, 12, 0, 0, 0, time.FixedZone("ART", -3*3600))
	end := start.Add(time.Hour)
	resp := `{"id":9,"workspace_id":2,"title":"R","start_time":"2026-12-03T15:00:00Z","end_time":"2026-12-03T16:00:00Z","status":"scheduled","attendees":[{"user_id":1,"name":"Yo","email":"yo@example.com","rsvp_status":"pending"}]}`
	c := servidor(t, "POST", "/api/meetings", 201, resp, func(b map[string]any, _ *http.Request) {
		assert.EqualValues(t, 2, b["workspace_id"])
		assert.Equal(t, "R", b["title"])
		assert.Equal(t, "<p>d</p>", b["description"])
		assert.Equal(t, "Sala", b["location"])
		assert.Equal(t, "", b["agenda"])
		assert.Equal(t, "2026-12-03T15:00:00Z", b["start_time"])
		assert.Equal(t, "2026-12-03T16:00:00Z", b["end_time"])
		assert.Equal(t, false, b["all_day"])
		assert.Equal(t, []any{float64(1), float64(4)}, b["attendee_ids"])
		assert.Equal(t, []any{}, b["task_ids"])
		assert.NotContains(t, b, "project_id")
	})
	got, err := c.CreateMeeting(context.Background(), NewMeeting{WorkspaceID: 2, Title: "R", Description: "<p>d</p>",
		Location: "Sala", Start: start, End: end, AttendeeIDs: []int64{1, 4}})
	require.NoError(t, err)
	assert.EqualValues(t, 9, got.ID)
	require.Len(t, got.Attendees, 1)
	assert.Equal(t, "pending", got.Attendees[0].Rsvp)

	// con proyecto, día completo y sin invitados: attendee_ids va como [].
	c = servidor(t, "POST", "/api/meetings", 201, `{"id":10}`, func(b map[string]any, _ *http.Request) {
		assert.EqualValues(t, 5, b["project_id"])
		assert.Equal(t, true, b["all_day"])
		assert.Equal(t, []any{}, b["attendee_ids"])
		assert.Equal(t, []any{}, b["task_ids"])
	})
	_, err = c.CreateMeeting(context.Background(), NewMeeting{WorkspaceID: 2, ProjectID: 5, Title: "R", Start: start, End: end, AllDay: true})
	require.NoError(t, err)
}

func TestWorkspacesAndMembers(t *testing.T) {
	c := servidor(t, "GET", "/api/workspaces", 200, `[
	 {"id":1,"user_id":1,"name":"Personal","is_default":true,"company_name":"X","company_logo_url":"http://firmada"},
	 {"id":2,"user_id":1,"name":"Equipo","is_default":false}]`, nil)
	ws, err := c.Workspaces(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []Workspace{{ID: 1, Name: "Personal"}, {ID: 2, Name: "Equipo"}}, ws)

	c = servidor(t, "GET", "/api/workspaces/2/members", 200, `[
	 {"id":4,"email":"ana@example.com","name":"Ana","role":"agent","status":"","hourly_rate":12.5},
	 {"id":5,"email":"bo@example.com","name":"Bo","role":"admin","status":""}]`, nil)
	ms, err := c.Members(context.Background(), 2)
	require.NoError(t, err)
	assert.Equal(t, []Member{{ID: 4, Name: "Ana", Email: "ana@example.com"}, {ID: 5, Name: "Bo", Email: "bo@example.com"}}, ms)
}

func TestMeetings_Unauthorized(t *testing.T) {
	c := servidor(t, "GET", "/api/meetings?from=2026-12-01T00%3A00%3A00Z&to=2026-12-02T00%3A00%3A00Z&workspace_id=3", 401, `{"error":"unauthorized"}`, nil)
	_, err := c.Meetings(context.Background(), 3, time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC))
	assert.True(t, errors.Is(err, ErrUnauthorized))
}

func TestMeeting_ByID(t *testing.T) {
	c := servidor(t, "GET", "/api/meetings/9", 200, `{"id":9,"workspace_id":2,"title":"R","start_time":"2026-12-20T15:00:00Z",
	 "end_time":"2026-12-20T16:00:00Z","status":"scheduled","created_by":7,"tasks":[]}`, nil)
	m, err := c.Meeting(context.Background(), 9)
	require.NoError(t, err)
	assert.EqualValues(t, 9, m.ID)
	assert.EqualValues(t, 2, m.WorkspaceID)
	assert.Equal(t, time.Date(2026, 12, 20, 15, 0, 0, 0, time.UTC), m.Start)

	c = servidor(t, "GET", "/api/meetings/9", 404, `{"error":"meeting not found"}`, nil)
	_, err = c.Meeting(context.Background(), 9)
	assert.ErrorIs(t, err, ErrNotFound)
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 404, api.Status)

	c = servidor(t, "GET", "/api/meetings/9", 500, `{"error":"falla"}`, nil)
	_, err = c.Meeting(context.Background(), 9)
	assert.NotErrorIs(t, err, ErrNotFound)

	c = servidor(t, "GET", "/api/meetings/9", 401, `{"error":"unauthorized"}`, nil)
	_, err = c.Meeting(context.Background(), 9)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

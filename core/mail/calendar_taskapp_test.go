package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/store"
	"postacontrol/core/testenv"
)

// fakeMeeting es una reunión del Taskapp falso, en la forma del cable.
type fakeMeeting struct {
	ID          int64     `json:"id"`
	WorkspaceID int64     `json:"workspace_id"`
	ProjectID   *int64    `json:"project_id"`
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

// meetTaskapp es un Taskapp falso con reuniones: el usuario es el 7 y hay un
// workspace (5) con un proyecto (1, "Alfa", #222222).
type meetTaskapp struct {
	srv      *httptest.Server
	mu       sync.Mutex
	meetings []fakeMeeting
	posts    []string      // cuerpos de los POST /api/meetings
	gets     int           // GET /api/meetings
	down     bool          // /api/meetings corta la conexión sin responder
	code     int           // /api/meetings responde con este código (0 = normal)
	postDrop bool          // el POST corta la conexión sin responder
	details  int           // GET /api/meetings/{id}
	detailDn bool          // GET /api/meetings/{id} corta la conexión sin responder
	hang     chan struct{} // si no es nil, GET /api/meetings no responde hasta que se cierre
	nextID   int64
}

func (f *meetTaskapp) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *meetTaskapp) edit(id int64, fn func(m *fakeMeeting)) {
	f.set(func() {
		for i := range f.meetings {
			if f.meetings[i].ID == id {
				fn(&f.meetings[i])
			}
		}
	})
}

func (f *meetTaskapp) remove(id int64) {
	f.set(func() {
		out := f.meetings[:0]
		for _, m := range f.meetings {
			if m.ID != id {
				out = append(out, m)
			}
		}
		f.meetings = out
	})
}

func (f *meetTaskapp) lastPost() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.posts) == 0 {
		return ""
	}
	return f.posts[len(f.posts)-1]
}

func newMeetTaskapp(t *testing.T) *meetTaskapp {
	t.Helper()
	f := &meetTaskapp{nextID: 77}
	drop := func(w http.ResponseWriter) {
		conn, _, _ := w.(http.Hijacker).Hijack()
		_ = conn.Close()
	}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/auth/me":
			if r.Header.Get("Authorization") != "Bearer "+taskappTok {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"id":7,"name":"Fabricio","email":"f@ej.com"}`))
		case r.URL.Path == "/api/projects":
			_, _ = w.Write([]byte(`[{"id":1,"workspace_id":5,"name":"Alfa","color":"#222222","status":"active","approver_emails":[]}]`))
		case r.URL.Path == "/api/workspaces":
			_, _ = w.Write([]byte(`[{"id":5,"name":"Code Control"}]`))
		case r.URL.Path == "/api/workspaces/5/members":
			_, _ = w.Write([]byte(`[{"id":7,"name":"Fabricio","email":"f@ej.com"},{"id":2,"name":"Ana","email":"ana@cc.com"}]`))
		case r.URL.Path == "/api/meetings" && r.Method == "GET":
			f.gets++
			if hang := f.hang; hang != nil {
				f.mu.Unlock()
				select {
				case <-hang:
				case <-r.Context().Done():
				}
				f.mu.Lock()
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			if f.down {
				drop(w)
				return
			}
			if f.code != 0 {
				w.WriteHeader(f.code)
				_, _ = w.Write([]byte(`{"error":"falla"}`))
				return
			}
			q := r.URL.Query()
			if q.Get("workspace_id") != "5" {
				_, _ = w.Write([]byte(`[]`))
				return
			}
			from, _ := time.Parse(time.RFC3339, q.Get("from"))
			to, _ := time.Parse(time.RFC3339, q.Get("to"))
			out := []fakeMeeting{}
			for _, m := range f.meetings { // como la API: solo por start_time
				if !m.StartTime.Before(from) && !m.StartTime.After(to) {
					out = append(out, m)
				}
			}
			_ = json.NewEncoder(w).Encode(out)
		case strings.HasPrefix(r.URL.Path, "/api/meetings/") && r.Method == "GET":
			f.details++
			if f.detailDn {
				drop(w)
				return
			}
			id, _ := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/meetings/"), 10, 64)
			for _, m := range f.meetings {
				if m.ID == id {
					_ = json.NewEncoder(w).Encode(m)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"meeting not found"}`))
		case r.URL.Path == "/api/meetings" && r.Method == "POST":
			f.posts = append(f.posts, string(body))
			if f.postDrop {
				drop(w)
				return
			}
			var in struct {
				WorkspaceID int64   `json:"workspace_id"`
				ProjectID   *int64  `json:"project_id"`
				Title       string  `json:"title"`
				Description string  `json:"description"`
				Location    string  `json:"location"`
				Start       string  `json:"start_time"`
				End         string  `json:"end_time"`
				AllDay      bool    `json:"all_day"`
				AttendeeIDs []int64 `json:"attendee_ids"`
			}
			_ = json.Unmarshal(body, &in)
			m := fakeMeeting{ID: f.nextID, WorkspaceID: in.WorkspaceID, ProjectID: in.ProjectID, Title: in.Title,
				Description: in.Description, Location: in.Location, AllDay: in.AllDay, CreatedBy: 7, Status: "scheduled"}
			m.StartTime, _ = time.Parse(time.RFC3339, in.Start)
			m.EndTime, _ = time.Parse(time.RFC3339, in.End)
			for _, id := range in.AttendeeIDs {
				m.Attendees = append(m.Attendees, struct {
					UserID     int64  `json:"user_id"`
					Name       string `json:"name"`
					Email      string `json:"email"`
					RsvpStatus string `json:"rsvp_status"`
				}{UserID: id, Name: "Ana", Email: "ana@cc.com", RsvpStatus: "pending"})
			}
			f.nextID++
			f.meetings = append(f.meetings, m)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(m)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// fm arma una reunión del falso; attendees son ids de usuario.
func fm(id, createdBy int64, start time.Time, attendees ...int64) fakeMeeting {
	proj := int64(1)
	m := fakeMeeting{ID: id, WorkspaceID: 5, ProjectID: &proj, Title: "Reunión " + strconv.FormatInt(id, 10),
		Location: "Sala", StartTime: start, EndTime: start.Add(time.Hour), CreatedBy: createdBy, Status: "scheduled"}
	for _, a := range attendees {
		m.Attendees = append(m.Attendees, struct {
			UserID     int64  `json:"user_id"`
			Name       string `json:"name"`
			Email      string `json:"email"`
			RsvpStatus string `json:"rsvp_status"`
		}{UserID: a, Name: "x", Email: "x@cc.com", RsvpStatus: "pending"})
	}
	return m
}

// meetCore arma un núcleo con cuenta de correo y Taskapp falso conectado.
func meetCore(t *testing.T) (*Core, *meetTaskapp, *testenv.SMTP, *recNotifier, model.Account) {
	t.Helper()
	c, smtp, n, acc := inviteCore(t)
	f := newMeetTaskapp(t)
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{TaskappAPIURL: f.srv.URL, TaskappWebURL: f.srv.URL}))
	connected(t, c)
	return c, f, smtp, n, acc
}

var (
	meetFrom = tu(2026, 12, 1, 0, 0)
	meetTo   = tu(2026, 12, 8, 0, 0)
)

// listRange lista [meetFrom, meetTo) con las reuniones ya refrescadas: ver listFresh.
func listRange(t *testing.T, c *Core) model.CalendarRange {
	t.Helper()
	return listFresh(t, c, meetFrom, meetTo)
}

// listFresh espera los refrescos en curso, deja que ListCalendar lance uno
// nuevo del rango (sin la espera entre refrescos), lo espera y vuelve a
// listar (esa segunda lectura ya no lanza otro).
func listFresh(t *testing.T, c *Core, from, to time.Time) model.CalendarRange {
	t.Helper()
	c.meetWG.Wait()
	c.resetMeetingRefresh()
	_, err := c.ListCalendar(context.Background(), from, to)
	require.NoError(t, err)
	c.meetWG.Wait()
	r, err := c.ListCalendar(context.Background(), from, to)
	require.NoError(t, err)
	return r
}

func meetingItems(r model.CalendarRange) []model.CalendarItem {
	out := []model.CalendarItem{}
	for _, it := range r.Items {
		if it.Source == "taskapp" {
			out = append(out, it)
		}
	}
	return out
}

// createReq pide una reunión el 3/12 a las 15:00 UTC con Ana (2) y los externos.
func createReq(acc model.Account, external ...model.Address) model.TaskappMeetingCreate {
	return model.TaskappMeetingCreate{WorkspaceID: 5, ProjectID: 1, Title: "Planificación", Description: "Temario",
		Location: "Meet", Start: tu(2026, 12, 3, 15, 0), End: tu(2026, 12, 3, 16, 0), AttendeeIDs: []int64{2},
		External: external, AccountID: acc.ID}
}

func mirrorOf(t *testing.T, c *Core, meetingID int64) model.CalendarEvent {
	t.Helper()
	e, err := c.store.EventByMeeting(context.Background(), meetingID)
	require.NoError(t, err)
	return e
}

func TestMeetings_FilteredToMe(t *testing.T) {
	c, f, _, _, _ := meetCore(t)
	f.set(func() {
		f.meetings = []fakeMeeting{
			fm(1, 2, tu(2026, 12, 2, 13, 0), 7, 3), // soy asistente
			fm(2, 7, tu(2026, 12, 3, 13, 0), 2),    // la creé yo
			fm(3, 2, tu(2026, 12, 4, 13, 0), 3),    // ajena
		}
	})
	r := listRange(t, c)
	items := meetingItems(r)
	require.Len(t, items, 2)
	assert.Equal(t, "m:1", items[0].Key)
	assert.Equal(t, "m:2", items[1].Key)
	it := items[0]
	assert.EqualValues(t, 1, it.MeetingID)
	assert.Equal(t, "Reunión 1", it.Title)
	assert.Equal(t, "Sala", it.Location)
	assert.True(t, it.Start.Equal(tu(2026, 12, 2, 13, 0)))
	assert.True(t, it.End.Equal(tu(2026, 12, 2, 14, 0)))
	assert.Equal(t, "#222222", it.Color)
	assert.Equal(t, "Alfa", it.ProjectName)
	assert.Equal(t, f.srv.URL+"/agenda?meeting=1", it.URL)
	assert.False(t, r.TaskappStale)
	assert.Empty(t, r.TaskappError)
	assert.False(t, r.TaskappFetchedAt.IsZero())
}

// Una reunión que empezó antes del rango y sigue en curso aparece (la API
// filtra solo por inicio); una que terminó antes, no.
func TestMeetings_OverlapFromEarlierStart(t *testing.T) {
	c, f, _, _, _ := meetCore(t)
	long := fm(1, 7, tu(2026, 11, 30, 12, 0))
	long.EndTime = tu(2026, 12, 1, 12, 0)
	f.set(func() { f.meetings = []fakeMeeting{long, fm(2, 7, tu(2026, 11, 30, 12, 0))} })
	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1)
	assert.Equal(t, "m:1", items[0].Key)
}

func TestMeetings_OfflineServesCache(t *testing.T) {
	c, f, _, _, _ := meetCore(t)
	f.set(func() {
		f.meetings = []fakeMeeting{fm(1, 7, tu(2026, 12, 2, 13, 0)), fm(2, 2, tu(2026, 12, 3, 13, 0), 7)}
	})
	first := listRange(t, c)
	require.Len(t, meetingItems(first), 2)

	f.set(func() { f.down = true })
	second := listRange(t, c)
	assert.Equal(t, meetingItems(first), meetingItems(second))
	assert.True(t, second.TaskappStale)
	assert.False(t, second.TaskappFetchedAt.IsZero())
	assert.Equal(t, "No pudimos conectar con Taskapp.", second.TaskappError)
}

func TestMeetings_AuthLatches(t *testing.T) {
	c, f, _, n, acc := meetCore(t)
	_, err := c.SaveEvent(context.Background(), model.EventSave{Event: model.CalendarEvent{Title: "Local",
		Start: tu(2026, 12, 2, 10, 0), End: tu(2026, 12, 2, 11, 0), AccountID: acc.ID}})
	require.NoError(t, err)
	f.set(func() { f.meetings = []fakeMeeting{fm(1, 7, tu(2026, 12, 2, 13, 0))} })
	require.Len(t, meetingItems(listRange(t, c)), 1)

	f.set(func() { f.code = http.StatusUnauthorized })
	r := listRange(t, c)
	assert.Equal(t, "Taskapp rechazó la conexión. Volvé a conectar desde Configuración.", r.TaskappError)
	assert.Empty(t, meetingItems(r), "con el token rechazado no se muestran las de la caché")
	require.Len(t, r.Items, 1)
	assert.Equal(t, "Local", r.Items[0].Title)
	assert.Contains(t, n.names(), EventTaskappAuth)

	// La traba: la próxima lectura no va a la red.
	f.set(func() { f.gets = 0 })
	r = listRange(t, c)
	assert.Equal(t, "Taskapp rechazó la conexión. Volvé a conectar desde Configuración.", r.TaskappError)
	f.set(func() { assert.Zero(t, f.gets) })
}

func TestMeetings_NotConnectedNoError(t *testing.T) {
	c, _, _, _ := inviteCore(t)
	r, err := c.ListCalendar(context.Background(), meetFrom, meetTo)
	require.NoError(t, err)
	assert.Empty(t, r.Items)
	assert.Empty(t, r.TaskappError)
	assert.False(t, r.TaskappStale)
}

func TestMeetingWorkspacesAndMembers(t *testing.T) {
	c, _, _, _, _ := meetCore(t)
	ws, err := c.TaskappWorkspaces(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []model.TaskappWorkspace{{ID: 5, Name: "Code Control"}}, ws)
	ms, err := c.TaskappMembers(context.Background(), 5)
	require.NoError(t, err)
	assert.Equal(t, []model.TaskappMember{{ID: 7, Name: "Fabricio", Email: "f@ej.com"}, {ID: 2, Name: "Ana", Email: "ana@cc.com"}}, ms)
}

func TestCreateMeeting_WithExternals(t *testing.T) {
	setLocalAR(t)
	c, f, smtp, _, acc := meetCore(t)
	item, err := c.CreateTaskappMeeting(context.Background(), createReq(acc,
		model.Address{Email: "BEA@cliente.com"}, model.Address{Name: "Bea", Email: "bea@cliente.com"}, // repetida: gana la última
		model.Address{Email: testUser})) // la propia cuenta no se invita
	require.NoError(t, err)
	assert.Equal(t, "m:77", item.Key)
	assert.Equal(t, "taskapp", item.Source)
	assert.EqualValues(t, 77, item.MeetingID)
	assert.Equal(t, f.srv.URL+"/agenda?meeting=77", item.URL)
	assert.Equal(t, 1, item.Attendees)

	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.lastPost()), &body))
	assert.Equal(t, []any{float64(2)}, body["attendee_ids"])
	assert.Equal(t, "Planificación", body["title"])
	assert.EqualValues(t, 5, body["workspace_id"])
	assert.EqualValues(t, 1, body["project_id"])

	mirror := mirrorOf(t, c, 77)
	assert.Equal(t, "taskapp", mirror.Kind)
	assert.Equal(t, "taskapp-77@postacontrol", mirror.UID)
	assert.Equal(t, "Planificación", mirror.Title)
	assert.Equal(t, "Temario", mirror.Description)
	assert.Equal(t, "Meet", mirror.Location)
	assert.Equal(t, 0, mirror.Sequence)
	assert.Equal(t, acc.ID, mirror.AccountID)
	assert.Equal(t, model.Address{Name: "Fabricio", Email: testUser}, mirror.Organizer)
	require.Len(t, mirror.Attendees, 1)
	assert.Equal(t, model.Attendee{Email: "bea@cliente.com", Name: "Bea", Partstat: "NEEDS-ACTION"}, mirror.Attendees[0])
	snap, err := c.store.TaskappSnapshot(context.Background(), mirror.ID)
	require.NoError(t, err)
	assert.JSONEq(t, `{"title":"Planificación","start":"2026-12-03T15:00:00Z","end":"2026-12-03T16:00:00Z",
		"allDay":false,"location":"Meet","description":"Temario","status":"scheduled"}`, snap)

	envs := waitMails(t, smtp, 1)
	m := readMail(t, envs[0])
	assert.Equal(t, "bea@cliente.com", m.To)
	assert.Contains(t, m.ICS, "METHOD:REQUEST")
	assert.Contains(t, m.ICS, "UID:taskapp-77@postacontrol")
	assert.Contains(t, m.ICS, "SEQUENCE:0")
	assert.Equal(t, "Invitación: Planificación — 3 de diciembre de 2026, 12:00", m.Subject)

	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1, "la reunión con espejo sale una sola vez")
	assert.Equal(t, "m:77", items[0].Key)
	assert.Equal(t, mirror.ID, items[0].EventID)
	assert.Equal(t, 1, items[0].Attendees)
	assert.Equal(t, 0, items[0].Confirmed)
	assert.Empty(t, items[0].TaskappChange)
	noMoreMails(t, c, smtp, 1)
}

func TestCreateMeeting_NoExternalsNoMirror(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	req := createReq(acc)
	req.AccountID = 0
	item, err := c.CreateTaskappMeeting(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "m:77", item.Key)
	assert.NotEmpty(t, f.lastPost())
	_, err = c.store.EventByMeeting(context.Background(), 77)
	assert.ErrorIs(t, err, store.ErrNotFound)
	noMoreMails(t, c, smtp, 0)
}

func TestCreateMeeting_Validations(t *testing.T) {
	c, f, _, _, acc := meetCore(t)
	req := createReq(acc)
	req.Title = "  "
	_, err := c.CreateTaskappMeeting(context.Background(), req)
	requireUserMsg(t, err, "invalid", "Poné un título.")
	req = createReq(acc)
	req.End = req.Start
	_, err = c.CreateTaskappMeeting(context.Background(), req)
	requireUserMsg(t, err, "invalid", "El fin tiene que ser después del inicio.")
	req = createReq(acc, model.Address{Email: "bea@cliente.com"})
	req.AccountID = 0
	_, err = c.CreateTaskappMeeting(context.Background(), req)
	requireUserMsg(t, err, "invalid", "Elegí desde qué cuenta mandar las invitaciones.")
	assert.Empty(t, f.lastPost(), "nada llegó a Taskapp")

	// Día completo: medianoche UTC de la fecha, fin exclusivo.
	req = createReq(acc)
	req.AllDay, req.Start, req.End = true, tu(2026, 12, 3, 15, 0), tu(2026, 12, 3, 15, 0)
	item, err := c.CreateTaskappMeeting(context.Background(), req)
	require.NoError(t, err)
	assert.True(t, item.AllDay)
	var body map[string]any
	require.NoError(t, json.Unmarshal([]byte(f.lastPost()), &body))
	assert.Equal(t, "2026-12-03T00:00:00Z", body["start_time"])
	assert.Equal(t, "2026-12-04T00:00:00Z", body["end_time"])
}

func TestCreateMeeting_OrphanMessage(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	c.meetingMirrorHook = func() error { return errors.New("disco lleno") }
	_, err := c.CreateTaskappMeeting(context.Background(), createReq(acc, model.Address{Email: "bea@cliente.com"}))
	requireUserMsg(t, err, "taskapp-orphan",
		"La reunión se creó en Taskapp, pero no se pudo guardar la lista de invitados externos en esta PC; no la vuelvas a crear.")
	assert.Contains(t, err.Error(), "no la vuelvas a crear")
	assert.NotEmpty(t, f.lastPost())
	_, err = c.store.EventByMeeting(context.Background(), 77)
	assert.ErrorIs(t, err, store.ErrNotFound)
	noMoreMails(t, c, smtp, 0)
}

func TestCreateMeeting_UncertainMessage(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	f.set(func() { f.postDrop = true })
	_, err := c.CreateTaskappMeeting(context.Background(), createReq(acc, model.Address{Email: "bea@cliente.com"}))
	requireUserMsg(t, err, "taskapp-uncertain",
		"No sabemos si la reunión se creó en Taskapp: revisala ahí antes de volver a intentar; no la vuelvas a crear sin revisar.")
	noMoreMails(t, c, smtp, 0)
}

// createdWithBea crea la reunión 77 con Bea de externa y espera su invitación.
func createdWithBea(t *testing.T, c *Core, smtp *testenv.SMTP, acc model.Account) model.CalendarEvent {
	t.Helper()
	_, err := c.CreateTaskappMeeting(context.Background(), createReq(acc, model.Address{Name: "Bea", Email: "bea@cliente.com"}))
	require.NoError(t, err)
	waitMails(t, smtp, 1)
	return mirrorOf(t, c, 77)
}

func TestTaskappChange_Detected(t *testing.T) {
	c, f, smtp, n, acc := meetCore(t)
	createdWithBea(t, c, smtp, acc)
	before := len(n.matching(EventCalendarChanged))
	f.edit(77, func(m *fakeMeeting) { m.StartTime, m.EndTime = tu(2026, 12, 3, 17, 0), tu(2026, 12, 3, 18, 0) })

	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1)
	assert.Equal(t, "changed", items[0].TaskappChange)
	assert.True(t, items[0].Start.Equal(tu(2026, 12, 3, 17, 0)), "se muestra la hora nueva de Taskapp")
	assert.Len(t, n.matching(EventCalendarChanged), before+1)

	// Sin cambio de estado no se vuelve a avisar.
	items = meetingItems(listRange(t, c))
	assert.Equal(t, "changed", items[0].TaskappChange)
	assert.Len(t, n.matching(EventCalendarChanged), before+1)
	noMoreMails(t, c, smtp, 1)
}

func TestTaskappChange_Cancelled(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) { m.Status = "cancelled" })
	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1)
	assert.Equal(t, "cancelled", items[0].TaskappChange)
	assert.True(t, items[0].Cancelled)
}

func TestTaskappChange_OfflineNoFalseDelete(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)

	f.set(func() { f.down = true })
	r := listRange(t, c)
	assert.True(t, r.TaskappStale)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange, "sin respuesta no se marca borrada")

	f.set(func() { f.code = http.StatusInternalServerError; f.down = false })
	listRange(t, c)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange, "con un error tampoco")

	// Un rango que no cubre la hora del espejo no dice nada de él.
	f.set(func() { f.code = 0 })
	f.remove(77)
	listFresh(t, c, tu(2026, 12, 10, 0, 0), tu(2026, 12, 17, 0, 0))
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)

	items := meetingItems(listRange(t, c))
	assert.Equal(t, "deleted", mirrorOf(t, c, 77).TaskappChange)
	require.Len(t, items, 1, "el espejo se sigue mostrando")
	assert.Equal(t, "m:77", items[0].Key)
	assert.Equal(t, mirror.ID, items[0].EventID)
	assert.Equal(t, "deleted", items[0].TaskappChange)
	assert.Equal(t, "Planificación", items[0].Title)
}

func TestNotifyTaskappChange_Update(t *testing.T) {
	setLocalAR(t)
	c, f, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) {
		m.Title, m.StartTime, m.EndTime = "Planificación 2", tu(2026, 12, 3, 17, 0), tu(2026, 12, 3, 18, 0)
	})
	listRange(t, c)
	require.Equal(t, "changed", mirrorOf(t, c, 77).TaskappChange)

	require.NoError(t, c.NotifyTaskappChange(context.Background(), mirror.ID))
	envs := waitMails(t, smtp, 2)
	m := readMail(t, envs[1])
	assert.Equal(t, "bea@cliente.com", m.To)
	assert.Contains(t, m.ICS, "METHOD:REQUEST")
	assert.Contains(t, m.ICS, "SEQUENCE:1")
	assert.Contains(t, m.ICS, "DTSTART:20261203T170000Z")
	assert.Contains(t, m.ICS, "SUMMARY:Planificación 2")
	assert.Equal(t, "Invitación actualizada: Planificación 2 — 3 de diciembre de 2026, 14:00", m.Subject)

	after := mirrorOf(t, c, 77)
	assert.Empty(t, after.TaskappChange)
	assert.Equal(t, 1, after.Sequence)
	assert.Equal(t, "Planificación 2", after.Title)
	assert.True(t, after.Start.Equal(tu(2026, 12, 3, 17, 0)))

	// Ya no queda nada pendiente: ni otra lectura lo marca ni avisar manda algo.
	listRange(t, c)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)
	require.NoError(t, c.NotifyTaskappChange(context.Background(), mirror.ID))
	noMoreMails(t, c, smtp, 2)
}

func TestNotifyTaskappChange_DeletedSendsCancel(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	f.remove(77)
	listRange(t, c)
	require.Equal(t, "deleted", mirrorOf(t, c, 77).TaskappChange)

	require.NoError(t, c.NotifyTaskappChange(context.Background(), mirror.ID))
	envs := waitMails(t, smtp, 2)
	m := readMail(t, envs[1])
	assert.Contains(t, m.ICS, "METHOD:CANCEL")
	assert.Contains(t, m.ICS, "SEQUENCE:1")
	after := mirrorOf(t, c, 77)
	assert.Equal(t, "cancelled", after.Status)
	assert.Empty(t, after.TaskappChange)

	// Cancelado, no se vuelve a marcar aunque la reunión siga sin venir.
	listRange(t, c)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)
	noMoreMails(t, c, smtp, 2)
}

func TestNotifyTaskappChange_NotAMeeting(t *testing.T) {
	c, _, _, _, acc := meetCore(t)
	e, err := c.SaveEvent(context.Background(), model.EventSave{Event: model.CalendarEvent{Title: "Local",
		Start: tu(2026, 12, 2, 10, 0), End: tu(2026, 12, 2, 11, 0), AccountID: acc.ID}})
	require.NoError(t, err)
	requireUserMsg(t, c.NotifyTaskappChange(context.Background(), e.ID), "invalid", "Este evento no es una reunión de Taskapp.")
	requireUserMsg(t, c.DismissTaskappChange(context.Background(), e.ID), "invalid", "Este evento no es una reunión de Taskapp.")
	_, err = c.RemoveTaskappExternals(context.Background(), e.ID)
	requireUserMsg(t, err, "invalid", "Este evento no es una reunión de Taskapp.")
}

func TestDismissTaskappChange_UntilNextChange(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) { m.StartTime, m.EndTime = tu(2026, 12, 3, 17, 0), tu(2026, 12, 3, 18, 0) })
	listRange(t, c)
	require.Equal(t, "changed", mirrorOf(t, c, 77).TaskappChange)

	require.NoError(t, c.DismissTaskappChange(context.Background(), mirror.ID))
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)
	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1)
	assert.Empty(t, items[0].TaskappChange, "con los mismos datos no se vuelve a marcar")

	f.edit(77, func(m *fakeMeeting) { m.Location = "Sala 2" })
	items = meetingItems(listRange(t, c))
	assert.Equal(t, "changed", items[0].TaskappChange)
	noMoreMails(t, c, smtp, 1)
}

func TestDismissTaskappChange_CancelledLeavesMirrorCancelled(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) { m.Status = "cancelled" })
	listRange(t, c)
	require.NoError(t, c.DismissTaskappChange(context.Background(), mirror.ID))
	after := mirrorOf(t, c, 77)
	assert.Equal(t, "cancelled", after.Status)
	assert.Empty(t, after.TaskappChange)
	listRange(t, c)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)
	noMoreMails(t, c, smtp, 1)
}

func TestRemoveMeetingExternals_CancelsAndDeletes(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	res, err := c.RemoveTaskappExternals(context.Background(), mirror.ID)
	require.NoError(t, err)
	assert.Equal(t, model.RespondResult{Warning: "La reunión sigue en Taskapp; solo se avisó a los invitados externos."}, res)
	envs := waitMails(t, smtp, 2)
	m := readMail(t, envs[1])
	assert.Equal(t, "bea@cliente.com", m.To)
	assert.Contains(t, m.ICS, "METHOD:CANCEL")
	assert.Contains(t, m.ICS, "SEQUENCE:1")
	_, err = c.store.EventByMeeting(context.Background(), 77)
	assert.ErrorIs(t, err, store.ErrNotFound)

	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1, "la reunión sigue, sin espejo")
	assert.Zero(t, items[0].EventID)
	assert.Zero(t, items[0].Attendees)
	f.set(func() { assert.Len(t, f.meetings, 1) })
}

func TestMeetingMirror_NotEditableAsLocal(t *testing.T) {
	c, _, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	e := mirror
	e.Title = "Otro"
	_, err := c.SaveEvent(context.Background(), model.EventSave{Event: e})
	requireUserMsg(t, err, "invalid", "Este evento no se edita desde acá.")
	err = c.DeleteEvent(context.Background(), model.EventDelete{ID: mirror.ID})
	requireUserMsg(t, err, "invalid", "Este evento no se edita desde acá.")
}

func TestMeetingRefresh_DetectsInBackground(t *testing.T) {
	c, f, smtp, n, acc := meetCore(t)
	createdWithBea(t, c, smtp, acc)
	c.now = func() time.Time { return tu(2026, 12, 2, 9, 0) }
	f.edit(77, func(m *fakeMeeting) { m.Title = "Movida" })
	c.StartMeetingRefresh(context.Background(), 20*time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for mirrorOf(t, c, 77).TaskappChange != "changed" {
		if time.Now().After(deadline) {
			t.Fatal("la vuelta de fondo no detectó el cambio")
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotEmpty(t, n.matching(EventCalendarChanged))
	// La ventana es [now−1d, now+14d].
	f.set(func() { f.gets = 0 })
	time.Sleep(60 * time.Millisecond)
	f.set(func() { assert.Positive(t, f.gets, "sigue refrescando") })
}

func TestMeetingRefresh_NotConnectedSkips(t *testing.T) {
	c, _, _, _ := inviteCore(t)
	f := newMeetTaskapp(t)
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{TaskappAPIURL: f.srv.URL, TaskappWebURL: f.srv.URL}))
	c.StartMeetingRefresh(context.Background(), 10*time.Millisecond)
	c.StartMeetingRefresh(context.Background(), 0) // no hace nada
	time.Sleep(60 * time.Millisecond)
	f.set(func() { assert.Zero(t, f.gets) })
}

func TestTaskappStatus_HasUserID(t *testing.T) {
	c, _, _, _, _ := meetCore(t)
	st, err := c.TaskappStatus(context.Background())
	require.NoError(t, err)
	assert.Equal(t, model.TaskappStatus{Connected: true, UserID: 7, UserName: "Fabricio", UserEmail: "f@ej.com"}, st)
	assert.True(t, strings.HasPrefix(c.TaskappWebURL(context.Background()), "http://127.0.0.1"))
}

// Una reunión reprogramada fuera del rango que se mira no se da por borrada:
// se confirma en Taskapp y queda "changed", con la hora nueva para avisar.
func TestTaskappChange_MovedOutsideWindowIsChanged(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) { m.StartTime, m.EndTime = tu(2026, 12, 20, 15, 0), tu(2026, 12, 20, 16, 0) })

	items := meetingItems(listRange(t, c))
	assert.Equal(t, "changed", mirrorOf(t, c, 77).TaskappChange)
	require.Len(t, items, 1, "el espejo se sigue mostrando en su hora vieja")
	assert.Equal(t, "changed", items[0].TaskappChange)
	f.set(func() { assert.Equal(t, 1, f.details) })
	listRange(t, c)
	assert.Equal(t, "changed", mirrorOf(t, c, 77).TaskappChange, "no va y viene")

	require.NoError(t, c.NotifyTaskappChange(context.Background(), mirror.ID))
	envs := waitMails(t, smtp, 2)
	m := readMail(t, envs[1])
	assert.Contains(t, m.ICS, "METHOD:REQUEST")
	assert.Contains(t, m.ICS, "DTSTART:20261220T150000Z")
	assert.True(t, mirrorOf(t, c, 77).Start.Equal(tu(2026, 12, 20, 15, 0)))
}

func TestTaskappChange_ConfirmFailsLeavesMirror(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	createdWithBea(t, c, smtp, acc)
	f.remove(77)
	f.set(func() { f.detailDn = true })
	listRange(t, c)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange, "sin poder confirmar no se marca")
	f.set(func() { f.detailDn = false })
	listRange(t, c)
	assert.Equal(t, "deleted", mirrorOf(t, c, 77).TaskappChange, "con el 404, sí")
}

// Si la reunión vuelve a quedar como en la foto, el cambio pendiente se borra.
func TestTaskappChange_RevertClearsFlag(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) { m.Location = "Sala 9" })
	listRange(t, c)
	require.Equal(t, "changed", mirrorOf(t, c, 77).TaskappChange)
	f.edit(77, func(m *fakeMeeting) { m.Location = "Meet" })
	listRange(t, c)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)
}

// Completar la reunión en Taskapp no es un cambio para los externos.
func TestTaskappChange_CompletedIsNotAChange(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) { m.Status = "completed" })
	listRange(t, c)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)
}

// El aviso de cambios del calendario sale una sola vez, con el espejo ya guardado.
func TestCreateMeeting_EmitsOnceAfterMirror(t *testing.T) {
	c, _, _, n, acc := meetCore(t)
	var mirrored []bool
	n.setHook(func(event string, _ any) {
		if event == EventCalendarChanged {
			_, err := c.store.EventByMeeting(context.Background(), 77)
			mirrored = append(mirrored, err == nil)
		}
	})
	_, err := c.CreateTaskappMeeting(context.Background(), createReq(acc, model.Address{Email: "bea@cliente.com"}))
	require.NoError(t, err)
	n.setHook(nil)
	assert.Equal(t, []bool{true}, mirrored)
}

// Si no salen las invitaciones, la UI no tiene que ofrecer crearla de nuevo.
func TestCreateMeeting_SendFailedMessageContract(t *testing.T) {
	msg := fmt.Sprintf(msgMeetingSendFailed, "motivo")
	assert.Contains(t, msg, "no la vuelvas a crear")
	assert.Equal(t, "La reunión se creó en Taskapp, pero no pudimos mandar la invitación a los invitados externos: motivo; no la vuelvas a crear.", msg)
}

// Con Taskapp colgado, el calendario vuelve enseguida con lo local y las
// reuniones de la caché; el refresco queda andando en el fondo.
func TestMeetings_ListDoesNotWaitForTaskapp(t *testing.T) {
	c, f, _, _, acc := meetCore(t)
	f.set(func() { f.meetings = []fakeMeeting{fm(1, 7, tu(2026, 12, 2, 13, 0))} })
	require.Len(t, meetingItems(listRange(t, c)), 1)
	_, err := c.SaveEvent(context.Background(), model.EventSave{Event: model.CalendarEvent{Title: "Local",
		Start: tu(2026, 12, 2, 10, 0), End: tu(2026, 12, 2, 11, 0), AccountID: acc.ID}})
	require.NoError(t, err)
	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	f.set(func() { f.hang, f.gets = hang, 0 })
	c.resetMeetingRefresh()

	done := make(chan model.CalendarRange, 1)
	go func() {
		r, err := c.ListCalendar(context.Background(), meetFrom, meetTo)
		assert.NoError(t, err)
		done <- r
	}()
	var r model.CalendarRange
	select {
	case r = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("ListCalendar esperó a Taskapp")
	}
	require.Len(t, r.Items, 2)
	assert.Equal(t, "Local", r.Items[0].Title)
	assert.Equal(t, "m:1", r.Items[1].Key)
	require.Eventually(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.gets == 1
	}, 5*time.Second, 5*time.Millisecond, "el refresco salió en el fondo")
}

// Dos lecturas seguidas del mismo rango lanzan un solo refresco: mientras
// uno anda no sale otro, y después hay que esperar entre refrescos.
func TestMeetings_OneRefreshPerRange(t *testing.T) {
	c, f, _, _, _ := meetCore(t)
	listRange(t, c)
	hang := make(chan struct{})
	f.set(func() { f.hang, f.gets = hang, 0 })
	c.resetMeetingRefresh()
	_, err := c.ListCalendar(context.Background(), meetFrom, meetTo)
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return f.gets == 1
	}, 5*time.Second, 5*time.Millisecond)
	c.resetMeetingRefresh() // sin la espera: solo frena que haya uno andando
	_, err = c.ListCalendar(context.Background(), meetFrom, meetTo)
	require.NoError(t, err)
	close(hang)
	c.meetWG.Wait()
	f.set(func() { assert.Equal(t, 1, f.gets, "uno en curso alcanza") })

	f.set(func() { f.hang, f.gets = nil, 0 })
	c.resetMeetingRefresh()
	for range 2 {
		_, err = c.ListCalendar(context.Background(), meetFrom, meetTo)
		require.NoError(t, err)
	}
	c.meetWG.Wait()
	_, err = c.ListCalendar(context.Background(), meetFrom, meetTo)
	require.NoError(t, err)
	c.meetWG.Wait()
	f.set(func() { assert.Equal(t, 1, f.gets, "como mucho uno cada 30 s") })
}

// Un refresco que no cambia nada no avisa; la caché vieja se marca stale.
func TestMeetings_QuietRefreshAndStaleCache(t *testing.T) {
	c, f, _, n, _ := meetCore(t)
	f.set(func() { f.meetings = []fakeMeeting{fm(1, 7, tu(2026, 12, 2, 13, 0))} })
	first := listRange(t, c)
	assert.False(t, first.TaskappStale)
	before := len(n.matching(EventCalendarChanged))
	listRange(t, c)
	assert.Len(t, n.matching(EventCalendarChanged), before, "sin cambios no se avisa")

	later := time.Now().Add(16 * time.Minute)
	c.now = func() time.Time { return later }
	r, err := c.ListCalendar(context.Background(), meetFrom, meetTo) // ya refrescado hace poco: no sale otro
	require.NoError(t, err)
	assert.True(t, r.TaskappStale, "la caché tiene más de 15 minutos")
	assert.Empty(t, r.TaskappError)
	assert.Len(t, meetingItems(r), 1)
}

// El ítem de una reunión trae a los asistentes de Taskapp (con su respuesta
// traducida) y después a los externos del espejo.
func TestMeetings_People(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	createdWithBea(t, c, smtp, acc)
	f.edit(77, func(m *fakeMeeting) {
		m.Attendees[0].RsvpStatus = "accepted"
		m.Attendees = append(m.Attendees, m.Attendees[0], m.Attendees[0], m.Attendees[0])
		m.Attendees[1].Name, m.Attendees[1].Email, m.Attendees[1].RsvpStatus = "Caro", "caro@cc.com", "declined"
		m.Attendees[2].Name, m.Attendees[2].Email, m.Attendees[2].RsvpStatus = "Dani", "dani@cc.com", "maybe"
		m.Attendees[3].Name, m.Attendees[3].Email, m.Attendees[3].RsvpStatus = "Eli", "eli@cc.com", "pending"
	})
	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1)
	assert.Equal(t, []model.Attendee{
		{Email: "ana@cc.com", Name: "Ana", Partstat: "ACCEPTED"},
		{Email: "caro@cc.com", Name: "Caro", Partstat: "DECLINED"},
		{Email: "dani@cc.com", Name: "Dani", Partstat: "TENTATIVE"},
		{Email: "eli@cc.com", Name: "Eli", Partstat: "NEEDS-ACTION"},
		{Email: "bea@cliente.com", Name: "Bea", Partstat: "NEEDS-ACTION"},
	}, items[0].People)
	assert.Equal(t, 1, items[0].Attendees, "el conteo sigue siendo de los externos")
}

// La descripción de Taskapp es HTML: a los externos les llega como texto.
func TestMeetingExternals_DescriptionAsText(t *testing.T) {
	c, f, smtp, _, acc := meetCore(t)
	req := createReq(acc, model.Address{Name: "Bea", Email: "bea@cliente.com"})
	req.Description = "<p>Hola <b>equipo</b></p>"
	_, err := c.CreateTaskappMeeting(context.Background(), req)
	require.NoError(t, err)
	m := readMail(t, waitMails(t, smtp, 1)[0])
	for _, part := range []string{m.ICS, m.HTML, m.Text} {
		assert.NotContains(t, part, "<b>")
		assert.NotContains(t, part, "&lt;b&gt;")
	}
	assert.Contains(t, m.ICS, "DESCRIPTION:Hola equipo")
	assert.Equal(t, "Hola equipo", mirrorOf(t, c, 77).Description)

	f.edit(77, func(fm *fakeMeeting) { fm.Description = "<p>Nuevo &amp; <i>mejor</i></p>" })
	listRange(t, c)
	require.Equal(t, "changed", mirrorOf(t, c, 77).TaskappChange)
	require.NoError(t, c.NotifyTaskappChange(context.Background(), mirrorOf(t, c, 77).ID))
	m = readMail(t, waitMails(t, smtp, 2)[1])
	assert.Contains(t, m.ICS, "DESCRIPTION:Nuevo & mejor")
	assert.NotContains(t, m.HTML, "&lt;i&gt;")
	assert.Equal(t, "Nuevo & mejor", mirrorOf(t, c, 77).Description)
}

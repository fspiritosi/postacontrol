package mail

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
)

const taskappTok = "tok-123"

type fakeTaskapp struct {
	srv      *httptest.Server
	meStatus atomic.Int32 // 0 = 200
	tasksHit atomic.Int32
}

func newFakeTaskapp(t *testing.T) *fakeTaskapp {
	t.Helper()
	f := &fakeTaskapp{}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		if st := int(f.meStatus.Load()); st != 0 {
			w.WriteHeader(st)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+taskappTok {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"id":7,"name":"Fabricio","email":"f@ej.com"}`))
	})
	mux.HandleFunc("/api/projects", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
{"id":2,"workspace_id":1,"name":"Zeta","color":"#111111","status":"active","approver_emails":[]},
{"id":1,"workspace_id":1,"name":"Alfa","color":"#222222","status":"active","approver_emails":[]},
{"id":3,"workspace_id":1,"name":"Vieja","color":"#333333","status":"completed","approver_emails":[]}]`))
	})
	mux.HandleFunc("/api/tasks", func(w http.ResponseWriter, r *http.Request) {
		f.tasksHit.Add(1)
		if r.URL.Query().Get("assignee_id") != "7" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`[
{"id":10,"project_id":1,"title":"Tarea","description":"<p>d</p>","priority":"high","due_date":"2026-12-01","source":"internal","status":{"id":1,"name":"Pendiente","is_completed":false}},
{"id":11,"project_id":99,"title":"Ticket","description":"","priority":"none","source":"widget","status":{"id":2,"name":"Abierto","is_completed":false}}]`))
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newTaskappCore(t *testing.T, f *fakeTaskapp) (*Core, *recNotifier, secrets.Store) {
	t.Helper()
	sec := secrets.NewMemory()
	n := &recNotifier{}
	c, err := New(Config{DBPath: t.TempDir() + "/t.db", Secrets: sec, Notifier: n, SkipInitialSync: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{TaskappAPIURL: f.srv.URL, TaskappWebURL: f.srv.URL}))
	return c, n, sec
}

// grantOpen completa el callback de Login como lo haría el navegador.
func grantOpen(token string) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		cb, state := u.Query().Get("cb"), u.Query().Get("state")
		go func() {
			q := url.Values{"state": {state}, "token": {token}}
			if resp, err := http.Get(cb + "?" + q.Encode()); err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
}

func TestConnectTaskapp_StoresTokenAfterMe(t *testing.T) {
	f := newFakeTaskapp(t)
	c, n, sec := newTaskappCore(t, f)
	st, err := c.ConnectTaskapp(context.Background(), grantOpen(taskappTok))
	require.NoError(t, err)
	assert.Equal(t, model.TaskappStatus{Connected: true, UserName: "Fabricio", UserEmail: "f@ej.com"}, st)
	tok, err := sec.Get(taskappSecret)
	require.NoError(t, err)
	assert.Equal(t, taskappTok, tok)
	assert.Contains(t, n.names(), EventTaskappChanged)
}

func TestConnectTaskapp_MeFailsNoToken(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, sec := newTaskappCore(t, f)
	_, err := c.ConnectTaskapp(context.Background(), grantOpen("otro-token"))
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-auth", ue.Code)
	_, err = sec.Get(taskappSecret)
	assert.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestConnectTaskapp_Denied(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	open := func(raw string) error {
		u, _ := url.Parse(raw)
		go func() {
			q := url.Values{"state": {u.Query().Get("state")}, "error": {"access_denied"}}
			if resp, err := http.Get(u.Query().Get("cb") + "?" + q.Encode()); err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}
	_, err := c.ConnectTaskapp(context.Background(), open)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-denied", ue.Code)
	assert.Equal(t, "No se autorizó la conexión con Taskapp.", ue.Msg)
}

func TestConnectTaskapp_SecondCancelsFirst(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	opened := make(chan struct{})
	errc := make(chan error, 1)
	go func() {
		_, err := c.ConnectTaskapp(context.Background(), func(string) error { close(opened); return nil })
		errc <- err
	}()
	<-opened
	_, err := c.ConnectTaskapp(context.Background(), grantOpen(taskappTok))
	require.NoError(t, err)
	assert.Error(t, <-errc)
}

func connected(t *testing.T, c *Core) {
	t.Helper()
	_, err := c.ConnectTaskapp(context.Background(), grantOpen(taskappTok))
	require.NoError(t, err)
}

func TestDisconnectTaskapp_ClearsSecretAndCache(t *testing.T) {
	f := newFakeTaskapp(t)
	c, n, sec := newTaskappCore(t, f)
	connected(t, c)
	_, err := c.ListTaskappTasks(context.Background())
	require.NoError(t, err)
	require.NoError(t, c.DisconnectTaskapp(context.Background()))
	_, err = sec.Get(taskappSecret)
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	_, _, ok, err := c.store.TaskappCache(context.Background())
	require.NoError(t, err)
	assert.False(t, ok)
	st, err := c.TaskappStatus(context.Background())
	require.NoError(t, err)
	assert.False(t, st.Connected)
	assert.Contains(t, n.names(), EventTaskappChanged)
}

func TestListTaskappTasks_CachesAndServesStale(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	connected(t, c)
	first, err := c.ListTaskappTasks(context.Background())
	require.NoError(t, err)
	require.Len(t, first.Tasks, 2)
	assert.False(t, first.Stale)
	assert.Equal(t, model.TaskappTask{ID: 10, Title: "Tarea", Priority: "high", DueDate: "2026-12-01",
		StatusName: "Pendiente", ProjectID: 1, ProjectName: "Alfa", ProjectColor: "#222222",
		URL: f.srv.URL + "/tasks/10", Description: "<p>d</p>"}, first.Tasks[0])
	assert.True(t, first.Tasks[1].Ticket)
	assert.Equal(t, f.srv.URL+"/tickets/11", first.Tasks[1].URL)
	assert.Empty(t, first.Tasks[1].ProjectName)

	f.srv.Close()
	second, err := c.ListTaskappTasks(context.Background())
	require.NoError(t, err)
	assert.True(t, second.Stale)
	assert.NotEmpty(t, second.Error)
	assert.Equal(t, first.Tasks, second.Tasks)
}

func TestListTaskappTasks_NoCacheOffline(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	connected(t, c)
	f.srv.Close()
	_, err := c.ListTaskappTasks(context.Background())
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-offline", ue.Code)
	assert.Equal(t, "No pudimos conectar con Taskapp.", ue.Msg)
}

func TestTaskapp401_EmitsAuthEvent(t *testing.T) {
	f := newFakeTaskapp(t)
	c, n, _ := newTaskappCore(t, f)
	connected(t, c)
	_, err := c.ListTaskappTasks(context.Background()) // llena la caché
	require.NoError(t, err)
	f.meStatus.Store(401)
	c.taskappMu.Lock()
	c.taskappMe = nil
	c.taskappMu.Unlock()
	_, err = c.ListTaskappTasks(context.Background())
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-auth", ue.Code)
	assert.Contains(t, n.names(), EventTaskappAuth)
}

func TestTaskappStatus_OfflineKeepsConnected(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	connected(t, c)
	f.srv.Close()
	st, err := c.TaskappStatus(context.Background())
	require.NoError(t, err)
	assert.Equal(t, model.TaskappStatus{Connected: true, UserName: "Fabricio", UserEmail: "f@ej.com"}, st)
}

func TestTaskappClient_Disconnected(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	_, err := c.ListTaskappTasks(context.Background())
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-disconnected", ue.Code)
	assert.Equal(t, "Conectá Taskapp en Configuración.", ue.Msg)
}

func TestTaskappProjects_OnlyActiveSorted(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	connected(t, c)
	ps, err := c.TaskappProjects(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []model.TaskappProject{
		{ID: 1, WorkspaceID: 1, Name: "Alfa", Color: "#222222"},
		{ID: 2, WorkspaceID: 1, Name: "Zeta", Color: "#111111"},
	}, ps)
}

func TestSetSettings_TaskappURLValidation(t *testing.T) {
	f := newFakeTaskapp(t)
	c, _, _ := newTaskappCore(t, f)
	for _, bad := range []string{"ftp://x.com", "http://ejemplo.com", "no es url", "/relativa"} {
		err := c.SetSettings(context.Background(), model.Settings{TaskappAPIURL: bad})
		var ue *UserError
		require.ErrorAs(t, err, &ue, bad)
		assert.Equal(t, "La dirección de Taskapp no es válida.", ue.Msg)
	}
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{TaskappAPIURL: "https://x.com", TaskappWebURL: "http://localhost:3000"}))
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{}))
}

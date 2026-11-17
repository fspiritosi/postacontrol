package mail

import (
	"context"
	"encoding/json"
	"errors"
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
)

type tReq struct{ Method, Path, Body string }

// createTaskapp es un Taskapp falso que registra los pedidos.
type createTaskapp struct {
	srv        *httptest.Server
	mu         sync.Mutex
	reqs       []tReq
	uploadCode int  // 0 = 200
	completed  bool // GET /api/tasks/N devuelve completada
	commentErr string
	postGate   chan struct{} // si no es nil, el POST de tarea avisa en postHit y espera
	postHit    chan struct{}
	postCode   int           // el POST de tarea responde con este código (0 = 200)
	postDrop   bool          // el POST de tarea corta la conexión sin responder
	uploadWait time.Duration // la subida tarda esto antes de responder
}

func (f *createTaskapp) count(method, path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, r := range f.reqs {
		if r.Method == method && r.Path == path {
			n++
		}
	}
	return n
}

func (f *createTaskapp) last(method, path string) (tReq, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.reqs) - 1; i >= 0; i-- {
		if f.reqs[i].Method == method && f.reqs[i].Path == path {
			return f.reqs[i], true
		}
	}
	return tReq{}, false
}

// set cambia la configuración del falso con el lock (el handler la lee con él).
func (f *createTaskapp) set(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn()
}

func (f *createTaskapp) total() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reqs)
}

func newCreateTaskapp(t *testing.T) *createTaskapp {
	t.Helper()
	f := &createTaskapp{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.reqs = append(f.reqs, tReq{r.Method, r.URL.Path, string(b)})
		postDrop, postCode := f.postDrop, f.postCode
		f.mu.Unlock()
		switch {
		case r.URL.Path == "/api/projects":
			_, _ = w.Write([]byte(`[{"id":1,"workspace_id":5,"name":"Alfa","color":"#222","status":"active","approver_emails":["bea@cliente.com"]}]`))
		case r.URL.Path == "/api/tasks" && r.Method == "POST":
			if f.postGate != nil {
				f.postHit <- struct{}{}
				<-f.postGate
			}
			if postDrop {
				conn, _, _ := w.(http.Hijacker).Hijack()
				_ = conn.Close()
				return
			}
			if postCode != 0 {
				w.WriteHeader(postCode)
				_, _ = w.Write([]byte(`{"error":"título repetido"}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":42,"project_id":1,"title":"x","source":"internal","attachments":["viejo"],"status":{"id":1,"name":"Pendiente","is_completed":false}}`))
		case r.URL.Path == "/api/tickets":
			_, _ = w.Write([]byte(`{"id":43,"title":"x","source":"widget","attachments":[],"status":{"id":1,"name":"Abierto","is_completed":false}}`))
		case r.URL.Path == "/api/upload":
			time.Sleep(f.uploadWait)
			if f.uploadCode != 0 {
				w.WriteHeader(f.uploadCode)
				return
			}
			_, _ = w.Write([]byte(`{"key":"k-1"}`))
		case strings.HasSuffix(r.URL.Path, "/comments"):
			if f.commentErr != "" {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"` + f.commentErr + `"}`))
				return
			}
			_, _ = w.Write([]byte(`{}`))
		case r.Method == "PUT":
			_, _ = w.Write([]byte(`{}`))
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/api/tasks/"):
			done := "false"
			if f.completed {
				done = "true"
			}
			_, _ = w.Write([]byte(`{"id":42,"title":"x","source":"internal","attachments":["viejo"],"status":{"id":1,"name":"S","is_completed":` + done + `}}`))
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// createEnv: núcleo con cuenta sincronizada, un correo de Bea y Taskapp falso
// conectado.
func createEnv(t *testing.T, raws ...[]byte) (*Core, *createTaskapp, []model.MessageSummary, *recNotifier) {
	t.Helper()
	c, imap, smtp, n := newTestCore(t)
	for _, r := range raws {
		imap.Deliver(t, "INBOX", r)
	}
	_, inboxID := setupSynced(t, imap, smtp, c)
	f := newCreateTaskapp(t)
	ctx := context.Background()
	require.NoError(t, c.SetSettings(ctx, model.Settings{TaskappAPIURL: f.srv.URL, TaskappWebURL: "https://web.ej"}))
	require.NoError(t, c.cfg.Secrets.Set(taskappSecret, "tok"))
	return c, f, listAll(t, c, inboxID), n
}

func hdrMail(from, subject, msgID, refs string) []byte {
	h := "From: " + from + "\r\nTo: " + testUser + "\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <" + msgID + ">\r\n"
	if refs != "" {
		h += "In-Reply-To: <" + refs + ">\r\nReferences: <" + refs + ">\r\n"
	}
	return []byte(h + "Content-Type: text/plain; charset=utf-8\r\n\r\nhola\r\n")
}

func msgBySubject(t *testing.T, ms []model.MessageSummary, subj string) model.MessageSummary {
	t.Helper()
	for _, m := range ms {
		if m.Subject == subj {
			return m
		}
	}
	t.Fatalf("no está %q", subj)
	return model.MessageSummary{}
}

func TestDraft_SuggestsAndCleans(t *testing.T) {
	c, _, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "RE: Fwd: Factura", "a@ej.com", ""))
	d, err := c.TaskappDraftFor(context.Background(), ms[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "Factura", d.Title)
	assert.Equal(t, int64(1), d.ProjectID)
	assert.False(t, d.Oversize)
	assert.Contains(t, d.DescriptionHTML, "<strong>De:</strong> Bea &lt;bea@cliente.com&gt;")
	assert.Contains(t, d.DescriptionHTML, "<p>hola</p>")
}

func TestDraft_Oversize(t *testing.T) {
	c, _, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "grande", "g@ej.com", ""))
	old := taskappEmlLimit
	taskappEmlLimit = 10
	t.Cleanup(func() { taskappEmlLimit = old })
	d, err := c.TaskappDraftFor(context.Background(), ms[0].ID)
	require.NoError(t, err)
	assert.True(t, d.Oversize)
}

func TestCreate_TaskWithoutProjectRejectedBeforeNetwork(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "s", "s@ej.com", ""))
	for _, req := range []model.TaskappCreate{
		{MessageID: ms[0].ID, Title: "x"},
		{MessageID: ms[0].ID, ProjectID: 1, Title: "   "},
		{MessageID: ms[0].ID, ProjectID: 1, Title: "x", Priority: "ya"},
		{MessageID: ms[0].ID, ProjectID: 1, Title: "x", DueDate: "19/11/2026"},
	} {
		_, err := c.CreateTaskappItem(context.Background(), req)
		var ue *UserError
		require.ErrorAs(t, err, &ue)
		assert.Equal(t, "invalid", ue.Code)
	}
	assert.Equal(t, 0, f.total())
	_, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, Title: "x"})
	assert.EqualError(t, err, "Elegí un proyecto para la tarea.")
}

func TestCreate_TicketWithoutProjectOK(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "s", "s@ej.com", ""))
	link, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, Ticket: true, Title: "Ticket", DueDate: "2026-12-01"})
	require.NoError(t, err)
	assert.True(t, link.Ticket)
	assert.False(t, link.AttachPending)
	assert.Equal(t, "https://web.ej/tickets/43", link.URL)
	req, ok := f.last("POST", "/api/tickets")
	require.True(t, ok)
	assert.NotContains(t, req.Body, "project_id")
	assert.NotContains(t, req.Body, "due_date")
	assert.Contains(t, req.Body, `"priority":"none"`)
}

func TestCreate_AttachesEml(t *testing.T) {
	c, f, ms, n := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	link, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura", Priority: "high", DueDate: "2026-12-01"})
	require.NoError(t, err)
	assert.False(t, link.AttachPending)
	assert.Equal(t, "Alfa", link.ProjectName)
	assert.Equal(t, 1, f.count("POST", "/api/tasks"))
	assert.Equal(t, 1, f.count("POST", "/api/upload"))
	up, _ := f.last("POST", "/api/upload")
	assert.Contains(t, up.Body, `filename="Factura.eml"`)
	put, ok := f.last("PUT", "/api/tasks/42")
	require.True(t, ok)
	var body struct{ Attachments []string }
	require.NoError(t, json.Unmarshal([]byte(put.Body), &body))
	assert.Equal(t, []string{"viejo", "k-1"}, body.Attachments)
	post, _ := f.last("POST", "/api/tasks")
	assert.Contains(t, post.Body, `"workspace_id":5`)
	assert.Contains(t, n.names(), EventTaskappChanged)
	stored, err := c.store.GetTaskLink(context.Background(), link.ID)
	require.NoError(t, err)
	assert.False(t, stored.AttachPending)
}

func TestCreate_AttachFailsKeepsLink(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	ctx := context.Background()
	f.uploadCode = 500
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	require.NoError(t, err)
	assert.True(t, link.AttachPending)
	assert.Equal(t, 1, f.count("POST", "/api/tasks"))

	f.uploadCode = 0
	got, err := c.RetryTaskLinkAttachment(ctx, link.ID)
	require.NoError(t, err)
	assert.False(t, got.AttachPending)
	assert.Equal(t, 1, f.count("POST", "/api/tasks"), "no se crea otra")
	stored, _ := c.store.GetTaskLink(ctx, link.ID)
	assert.False(t, stored.AttachPending)
	put, _ := f.last("PUT", "/api/tasks/42")
	assert.Contains(t, put.Body, `"viejo","k-1"`)
}

func TestCreate_RawFetchFailsCreatesNothing(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	require.NoError(t, c.Close())
	_, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	require.Error(t, err)
	assert.Equal(t, 0, f.count("POST", "/api/tasks"))
}

func TestCreate_OversizeStripsAndLists(t *testing.T) {
	raw := []byte("From: Bea <bea@cliente.com>\r\nTo: " + testUser + "\r\nSubject: Con adjunto\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <adj@ej.com>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=XX\r\n\r\n--XX\r\nContent-Type: text/plain\r\n\r\nhola\r\n--XX\r\n" +
		"Content-Type: application/pdf; name=\"plano.pdf\"\r\nContent-Disposition: attachment; filename=\"plano.pdf\"\r\nContent-Transfer-Encoding: base64\r\n\r\n" +
		strings.Repeat("QUJDREVGR0g=\r\n", 100) + "--XX--\r\n")
	c, f, ms, _ := createEnv(t, raw)
	old := taskappEmlLimit
	taskappEmlLimit = 200
	t.Cleanup(func() { taskappEmlLimit = old })
	link, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Con adjunto"})
	require.NoError(t, err)
	assert.False(t, link.AttachPending)
	post, _ := f.last("POST", "/api/tasks")
	assert.Contains(t, post.Body, "Adjuntos que no se subieron por tamaño:")
	assert.Contains(t, post.Body, "plano.pdf")
	assert.Equal(t, 0, f.count("PUT", "/api/tasks/42")-1, "un solo PUT: los adjuntos")
}

func TestDescriptionHTML_EscapesHeader(t *testing.T) {
	d := model.MessageDetail{MessageSummary: model.MessageSummary{
		From: model.Address{Name: "<script>x</script>", Email: "a@b.com"},
		Date: time.Date(2026, 11, 16, 13, 32, 0, 0, time.UTC)}, Text: "uno\r\ndos\r\n\r\n<b>tres</b>"}
	got := taskDescriptionHTML(d, time.UTC)
	assert.Equal(t, "<p><strong>De:</strong> &lt;script&gt;x&lt;/script&gt; &lt;a@b.com&gt; · <strong>Fecha:</strong> 16/11/2026 13:32</p>"+
		"<p>uno<br>dos</p><p>&lt;b&gt;tres&lt;/b&gt;</p>", got)
	d.From.Name = ""
	assert.Contains(t, taskDescriptionHTML(d, time.UTC), "<strong>De:</strong> a@b.com · ")
}

func TestLinks_ThreadWideAndAlreadyAdded(t *testing.T) {
	c, f, ms, _ := createEnv(t,
		hdrMail("Bea <bea@cliente.com>", "Hilo", "a@ej.com", ""),
		hdrMail("Bea <bea@cliente.com>", "Re: Hilo", "b@ej.com", "a@ej.com"))
	ctx := context.Background()
	a, b := msgBySubject(t, ms, "Hilo"), msgBySubject(t, ms, "Re: Hilo")
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: a.ID, ProjectID: 1, Title: "Hilo"})
	require.NoError(t, err)

	got, err := c.TaskLinksForMessage(ctx, b.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.False(t, got[0].AlreadyAdded)
	assert.True(t, got[0].StateKnown)
	assert.False(t, got[0].Completed)
	assert.Equal(t, "https://web.ej/tasks/42", got[0].URL)
	got, _ = c.TaskLinksForMessage(ctx, a.ID)
	assert.True(t, got[0].AlreadyAdded)

	require.NoError(t, c.AddMessageToTask(ctx, link.ID, b.ID))
	got, _ = c.TaskLinksForMessage(ctx, b.ID)
	assert.True(t, got[0].AlreadyAdded)
	err = c.AddMessageToTask(ctx, link.ID, b.ID)
	assert.EqualError(t, err, "Ese correo ya está en la tarea.")
	assert.Equal(t, 1, f.count("POST", "/api/tasks/42/comments"))
}

func TestLinks_StateUnknownOffline(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Hilo", "a@ej.com", ""))
	ctx := context.Background()
	_, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Hilo"})
	require.NoError(t, err)
	f.srv.Close()
	got, err := c.TaskLinksForMessage(ctx, ms[0].ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.False(t, got[0].StateKnown)

	require.NoError(t, c.cfg.Secrets.Delete(taskappSecret))
	got, err = c.TaskLinksForMessage(ctx, ms[0].ID)
	require.NoError(t, err)
	assert.False(t, got[0].StateKnown)
}

func TestAddMessage_CompletedRejected(t *testing.T) {
	c, f, ms, _ := createEnv(t,
		hdrMail("Bea <bea@cliente.com>", "Hilo", "a@ej.com", ""),
		hdrMail("Bea <bea@cliente.com>", "Re: Hilo", "b@ej.com", "a@ej.com"))
	ctx := context.Background()
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: msgBySubject(t, ms, "Hilo").ID, ProjectID: 1, Title: "Hilo"})
	require.NoError(t, err)
	b := msgBySubject(t, ms, "Re: Hilo")

	f.completed = true
	err = c.AddMessageToTask(ctx, link.ID, b.ID)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-completed", ue.Code)
	assert.Equal(t, "La tarea #42 está completada.", ue.Msg)

	f.completed, f.commentErr = false, "task is completed"
	err = c.AddMessageToTask(ctx, link.ID, b.ID)
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-completed", ue.Code)
	added, _ := c.store.AddedToLink(ctx, link.ID, "<b@ej.com>")
	assert.False(t, added)
}

func TestAddMessage_InternalComment(t *testing.T) {
	c, f, ms, n := createEnv(t,
		hdrMail("Bea <bea@cliente.com>", "Hilo", "a@ej.com", ""),
		hdrMail("Bea <bea@cliente.com>", "Re: Hilo", "b@ej.com", "a@ej.com"))
	ctx := context.Background()
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: msgBySubject(t, ms, "Hilo").ID, ProjectID: 1, Title: "Hilo"})
	require.NoError(t, err)
	before := len(n.matching(EventTaskappChanged))
	require.NoError(t, c.AddMessageToTask(ctx, link.ID, msgBySubject(t, ms, "Re: Hilo").ID))
	cm, ok := f.last("POST", "/api/tasks/42/comments")
	require.True(t, ok)
	assert.Contains(t, cm.Body, `"is_internal":true`)
	assert.Contains(t, cm.Body, `"attachments":["k-1"]`)
	var cb struct{ Body string }
	require.NoError(t, json.Unmarshal([]byte(cm.Body), &cb))
	assert.Contains(t, cb.Body, "Bea &lt;bea@cliente.com&gt;")
	assert.Greater(t, len(n.matching(EventTaskappChanged)), before)
}

func TestCreateLocalTaskFrom_LinksSource(t *testing.T) {
	c, _, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Re: Pedido", "p@ej.com", ""))
	lt, err := c.CreateLocalTaskFrom(context.Background(), ms[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "Pedido", lt.Title)
	assert.Equal(t, "<p@ej.com>", lt.SourceMessageID)
	assert.Equal(t, "Re: Pedido", lt.SourceSubject)
	assert.Zero(t, lt.ID)
}

func TestFindMessageByHeader_MissingIsUserError(t *testing.T) {
	c, _, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Hola", "h@ej.com", ""))
	m, err := c.FindMessageByHeader(context.Background(), "<h@ej.com>")
	require.NoError(t, err)
	assert.Equal(t, ms[0].ID, m.ID)
	_, err = c.FindMessageByHeader(context.Background(), "no@ej.com")
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "not_found", ue.Code)
	assert.Equal(t, "El correo ya no está en esta PC.", ue.Msg)
}

func TestCreate_OrphanWhenLinkSaveFails(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	c.addLinkHook = func() error { return errors.New("disco lleno") }
	_, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-orphan", ue.Code)
	assert.Contains(t, ue.Msg, "La tarea #42 se creó en Taskapp")
	assert.Equal(t, 1, f.count("POST", "/api/tasks"))
}

func TestCreate_CancelAfterPostStillSavesLink(t *testing.T) {
	c, f, ms, n := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	n.setHook(func(event string, _ any) {
		if event == EventTaskappChanged {
			cancel() // justo después de la respuesta del POST
		}
	})
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	require.NoError(t, err)
	assert.False(t, link.AttachPending)
	stored, err := c.store.TaskLinksForThread(context.Background(), link.ThreadRoot)
	require.NoError(t, err)
	assert.Len(t, stored, 1)
	assert.Equal(t, 1, f.count("POST", "/api/tasks"))
}

func TestCreate_ConcurrentSameMessageRejected(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	f.postGate, f.postHit = make(chan struct{}), make(chan struct{}, 1)
	req := model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"}
	done := make(chan error, 1)
	go func() { _, err := c.CreateTaskappItem(context.Background(), req); done <- err }()
	<-f.postHit
	_, err := c.CreateTaskappItem(context.Background(), req)
	assert.EqualError(t, err, "Ya se está creando la tarea de este correo.")
	close(f.postGate)
	require.NoError(t, <-done)
	assert.Equal(t, 1, f.count("POST", "/api/tasks"))
}

func TestAddMessage_AndRetry_ConcurrentRejected(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Hilo", "a@ej.com", ""))
	ctx := context.Background()
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Hilo"})
	require.NoError(t, err)
	r1, err := c.claim("add:1:2", "x")
	require.NoError(t, err)
	_, err = c.claim("add:1:2", "otro")
	assert.EqualError(t, err, "otro")
	r1()
	r2, err := c.claim("add:1:2", "x")
	require.NoError(t, err)
	r2()
	// Los guardas reales de AddMessageToTask y Retry usan estas claves.
	rel, err := c.claim("add:"+itoa(link.ID)+":"+itoa(ms[0].ID), "x")
	require.NoError(t, err)
	assert.EqualError(t, c.AddMessageToTask(ctx, link.ID, ms[0].ID), "Ya se está sumando este correo a la tarea.")
	rel()
	rel, err = c.claim("retry:"+itoa(link.ID), "x")
	require.NoError(t, err)
	_, err = c.RetryTaskLinkAttachment(ctx, link.ID)
	assert.EqualError(t, err, "Ya se está adjuntando el correo.")
	rel()
	assert.Equal(t, 0, f.count("POST", "/api/tasks/42/comments"))
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestCreate_DescriptionIsSanitizedBeforePost(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	_, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura",
		DescriptionHTML: `<style>p{color:red}</style><p style="x:y">hola</p><img src="https://e.com/t.png"><script>x()</script>`})
	require.NoError(t, err)
	post, _ := f.last("POST", "/api/tasks")
	var body struct{ Description string }
	require.NoError(t, json.Unmarshal([]byte(post.Body), &body))
	assert.Equal(t, "<p>hola</p>", body.Description)
}

// Cancelar mientras sale el POST no corta la creación: la tarea se crea y
// queda vinculada (si no, quedaría huérfana y se crearía dos veces).
func TestCreate_CancelDuringPostStillLinks(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	f.postGate, f.postHit = make(chan struct{}), make(chan struct{}, 1)
	ctx, cancel := context.WithCancel(context.Background())
	type res struct {
		link model.TaskLink
		err  error
	}
	done := make(chan res, 1)
	go func() {
		l, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
		done <- res{l, err}
	}()
	<-f.postHit
	cancel()
	time.Sleep(20 * time.Millisecond)
	close(f.postGate)
	r := <-done
	require.NoError(t, r.err)
	assert.Equal(t, int64(42), r.link.TaskID)
	assert.False(t, r.link.AttachPending)
	stored, err := c.store.TaskLinksForThread(context.Background(), r.link.ThreadRoot)
	require.NoError(t, err)
	assert.Len(t, stored, 1)
}

// Cancelar antes del POST no crea nada.
func TestCreate_CancelledBeforePostCreatesNothing(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	require.Error(t, err)
	assert.Equal(t, 0, f.count("POST", "/api/tasks"))
}

// Close espera a que termine la creación en curso antes de cerrar la base.
func TestCreate_CloseWaitsForCreate(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	f.postGate, f.postHit = make(chan struct{}), make(chan struct{}, 1)
	var release sync.Once
	open := func() { release.Do(func() { close(f.postGate) }) }
	t.Cleanup(open) // antes de cerrar el servidor, aunque el test falle
	type res struct {
		link model.TaskLink
		err  error
	}
	done := make(chan res, 1)
	go func() {
		l, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
		done <- res{l, err}
	}()
	<-f.postHit
	closed := make(chan error, 1)
	go func() { closed <- c.Close() }()
	select {
	case <-closed:
		t.Fatal("Close volvió con la creación en curso")
	case <-time.After(50 * time.Millisecond):
	}
	open()
	r := <-done
	require.NoError(t, r.err)
	assert.NotZero(t, r.link.ID, "el vínculo se guardó")
	require.NoError(t, <-closed)
}

// Si la conexión se corta después de mandar el POST no sabemos si la tarea
// se creó: el error lo dice y la UI lo trata como la huérfana.
func TestCreate_UncertainOnNetworkErrorAfterPost(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	f.set(func() { f.postDrop = true })
	_, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp-uncertain", ue.Code)
	assert.Contains(t, ue.Msg, "no la vuelvas a crear")

	f.set(func() { f.postDrop, f.postCode = false, 422 })
	_, err = c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "taskapp", ue.Code, "una respuesta de Taskapp no es incierta")
	assert.Equal(t, "Taskapp: título repetido", ue.Msg)
}

// La subida del .eml no la corta el timeout corto del cliente de la API.
func TestCreate_SlowUploadNotCutByClientTimeout(t *testing.T) {
	c, f, ms, _ := createEnv(t,
		hdrMail("Bea <bea@cliente.com>", "Hilo", "a@ej.com", ""),
		hdrMail("Bea <bea@cliente.com>", "Re: Hilo", "b@ej.com", "a@ej.com"))
	c.cfg.TaskappHTTP = &http.Client{Timeout: 100 * time.Millisecond}
	f.uploadWait = 300 * time.Millisecond
	ctx := context.Background()
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: msgBySubject(t, ms, "Hilo").ID, ProjectID: 1, Title: "Hilo"})
	require.NoError(t, err)
	assert.False(t, link.AttachPending)
	require.NoError(t, c.AddMessageToTask(ctx, link.ID, msgBySubject(t, ms, "Re: Hilo").ID))
	assert.Equal(t, 2, f.count("POST", "/api/upload"))
}

// Si se cancela después de que Taskapp aceptó el comentario, el correo queda
// marcado igual: si no, se ofrecería "Sumar" otra vez y se duplicaría.
func TestAddMessage_CancelAfterCommentStillMarks(t *testing.T) {
	c, f, ms, _ := createEnv(t,
		hdrMail("Bea <bea@cliente.com>", "Hilo", "a@ej.com", ""),
		hdrMail("Bea <bea@cliente.com>", "Re: Hilo", "b@ej.com", "a@ej.com"))
	link, err := c.CreateTaskappItem(context.Background(), model.TaskappCreate{MessageID: msgBySubject(t, ms, "Hilo").ID, ProjectID: 1, Title: "Hilo"})
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	c.afterCommentHook = cancel
	require.NoError(t, c.AddMessageToTask(ctx, link.ID, msgBySubject(t, ms, "Re: Hilo").ID))
	added, err := c.store.AddedToLink(context.Background(), link.ID, "<b@ej.com>")
	require.NoError(t, err)
	assert.True(t, added)
	assert.Equal(t, 1, f.count("POST", "/api/tasks/42/comments"))
}

// Reintentar un vínculo que ya tiene el .eml no sube nada.
func TestRetry_NotPendingDoesNothing(t *testing.T) {
	c, f, ms, _ := createEnv(t, hdrMail("Bea <bea@cliente.com>", "Factura", "s@ej.com", ""))
	ctx := context.Background()
	link, err := c.CreateTaskappItem(ctx, model.TaskappCreate{MessageID: ms[0].ID, ProjectID: 1, Title: "Factura"})
	require.NoError(t, err)
	require.False(t, link.AttachPending)
	total := f.total()
	got, err := c.RetryTaskLinkAttachment(ctx, link.ID)
	require.NoError(t, err)
	assert.False(t, got.AttachPending)
	assert.Equal(t, "https://web.ej/tasks/42", got.URL)
	assert.Equal(t, total, f.total(), "no se pide nada a Taskapp")
}

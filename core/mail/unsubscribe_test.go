package mail

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func newsletter(subject, unsub, post string) []byte {
	h := "From: Tienda <news@tienda.com>\r\nTo: " + testUser + "\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <" + subject + "@tienda.com>\r\n"
	if unsub != "" {
		h += "List-Unsubscribe: " + unsub + "\r\n"
	}
	if post != "" {
		h += "List-Unsubscribe-Post: " + post + "\r\n"
	}
	return []byte(h + "Content-Type: text/plain; charset=utf-8\r\n\r\nofertas\r\n")
}

// oneClickServer recibe los POST de desuscripción y anota qué llegó.
type oneClickServer struct {
	*httptest.Server
	mu     sync.Mutex
	bodies []string
	types  []string
	status int
}

func newOneClickServer(t *testing.T) *oneClickServer {
	s := &oneClickServer{status: http.StatusOK}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		if r.Method == http.MethodPost {
			s.bodies = append(s.bodies, string(b))
			s.types = append(s.types, r.Header.Get("Content-Type"))
		}
		status := s.status
		s.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(s.Close)
	return s
}

func onlyMessage(t *testing.T, c *Core, inboxID int64) model.MessageSummary {
	t.Helper()
	ms := listAll(t, c, inboxID)
	require.Len(t, ms, 1)
	return ms[0]
}

func TestUnsubscribe_OneClickPosts(t *testing.T) {
	srv := newOneClickServer(t)
	c, imap, smtp, _ := newTestCore(t)
	c.cfg.HTTPClient = srv.Client()
	imap.Deliver(t, "INBOX", newsletter("oferta", "<mailto:baja@tienda.com>, <"+srv.URL+"/baja?u=1>", "List-Unsubscribe=One-Click"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := onlyMessage(t, c, inboxID)
	ctx := context.Background()

	info, err := c.UnsubscribeInfo(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, model.Unsubscribe{Available: true, OneClick: true, URL: srv.URL + "/baja?u=1", Mailto: "mailto:baja@tienda.com"}, info)

	res, err := c.Unsubscribe(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, model.UnsubscribeResult{Method: "post"}, res)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	assert.Equal(t, []string{"List-Unsubscribe=One-Click"}, srv.bodies)
	assert.Equal(t, []string{"application/x-www-form-urlencoded"}, srv.types)
	assert.Empty(t, smtp.Received(), "con un clic alcanza: no manda correo")
	// Mirarlo no lo marcó leído.
	assert.False(t, onlyMessage(t, c, inboxID).Seen)
}

func TestUnsubscribe_MailtoSendsAMail(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", newsletter("lista", "<mailto:baja@lista.com?subject=Darme%20de%20baja>", ""))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	m := onlyMessage(t, c, inboxID)

	res, err := c.Unsubscribe(context.Background(), m.ID)
	require.NoError(t, err)
	assert.Equal(t, model.UnsubscribeResult{Method: "mail"}, res)
	got := smtp.Received()
	require.Len(t, got, 1)
	assert.Equal(t, []string{"baja@lista.com"}, got[0].To)
	assert.Contains(t, string(got[0].Data), "Subject: Darme de baja")
	assert.Empty(t, sentMessages(t, c, acc.ID), "no deja copia en Enviados")
}

func TestUnsubscribe_WebOnlyIsOpenedByTheUser(t *testing.T) {
	srv := newOneClickServer(t)
	c, imap, smtp, _ := newTestCore(t)
	c.cfg.HTTPClient = srv.Client()
	imap.Deliver(t, "INBOX", newsletter("web", "<"+srv.URL+"/preferencias>", ""))
	_, inboxID := setupSynced(t, imap, smtp, c)

	res, err := c.Unsubscribe(context.Background(), onlyMessage(t, c, inboxID).ID)
	require.NoError(t, err)
	assert.Equal(t, model.UnsubscribeResult{Method: "web", URL: srv.URL + "/preferencias"}, res)
	srv.mu.Lock()
	defer srv.mu.Unlock()
	assert.Empty(t, srv.bodies, "sin List-Unsubscribe-Post no se hace POST")
}

func TestUnsubscribe_OneClickNeedsHTTPS(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", newsletter("http", "<http://x.com/baja>", "List-Unsubscribe=One-Click"))
	_, inboxID := setupSynced(t, imap, smtp, c)

	info, err := c.UnsubscribeInfo(context.Background(), onlyMessage(t, c, inboxID).ID)
	require.NoError(t, err)
	assert.False(t, info.OneClick)
	assert.Equal(t, "http://x.com/baja", info.URL)
}

func TestUnsubscribe_WithoutHeaderOrFailing(t *testing.T) {
	srv := newOneClickServer(t)
	srv.status = http.StatusInternalServerError
	c, imap, smtp, _ := newTestCore(t)
	c.cfg.HTTPClient = srv.Client()
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "normal"))
	imap.Deliver(t, "INBOX", newsletter("rota", "<"+srv.URL+"/baja>", "List-Unsubscribe=One-Click"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()

	normal := bySubject(t, listAll(t, c, inboxID), "normal")
	info, err := c.UnsubscribeInfo(ctx, normal.ID)
	require.NoError(t, err)
	assert.False(t, info.Available)
	var ue *UserError
	_, err = c.Unsubscribe(ctx, normal.ID)
	require.ErrorAs(t, err, &ue)

	_, err = c.Unsubscribe(ctx, bySubject(t, listAll(t, c, inboxID), "rota").ID)
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, msgUnsubFailed, ue.Msg)
}

func TestParseListUnsubscribe(t *testing.T) {
	got := parseListUnsubscribe("List-Unsubscribe: <mailto:a@b.com>,\r\n <https://x.com/u?a=1>\r\nList-Unsubscribe-Post: List-Unsubscribe=One-Click\r\n\r\n")
	assert.Equal(t, model.Unsubscribe{Available: true, OneClick: true, URL: "https://x.com/u?a=1", Mailto: "mailto:a@b.com"}, got)
	assert.Equal(t, model.Unsubscribe{}, parseListUnsubscribe(""))
	// Esquemas raros no se ofrecen.
	assert.Equal(t, model.Unsubscribe{}, parseListUnsubscribe("List-Unsubscribe: <javascript:alert(1)>\r\n\r\n"))
}

package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

// waitSendFinished espera el EventSendFinished del correo id.
func waitSendFinished(t *testing.T, n *recNotifier, id int64) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range n.matching(EventSendFinished) {
			if e["draftId"] == id {
				return e
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("no llegó %s para el correo %d", EventSendFinished, id)
	return nil
}

func outbox(t *testing.T, c *Core) []model.LocalDraft {
	t.Helper()
	ds, err := c.ListLocalDrafts(context.Background())
	require.NoError(t, err)
	return ds
}

func TestQueueSend_SendsInBackgroundAndEmptiesOutbox(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)

	id, err := c.QueueSend(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "En segundo plano", BodyHTML: "<p>hola</p>",
	})
	require.NoError(t, err)
	require.NotZero(t, id)

	ev := waitSendFinished(t, n, id)
	assert.Equal(t, true, ev["ok"])
	assert.Empty(t, ev["message"])
	assert.Empty(t, ev["warning"])
	assert.Len(t, smtp.Received(), 1)
	assert.Empty(t, outbox(t, c), "el correo enviado sale de la bandeja de salida")
	assert.NotEmpty(t, n.matching(EventOutboxUpdated))
	require.Len(t, sentMessages(t, c, acc.ID), 1)
}

func TestQueueSend_FailureStaysInOutboxAndNotifies(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)

	id, err := c.QueueSend(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs(testenv.RejectedRecipient), Subject: "Rebota", BodyHTML: "<p>x</p>",
	})
	require.NoError(t, err)

	ev := waitSendFinished(t, n, id)
	assert.Equal(t, false, ev["ok"])
	msg, _ := ev["message"].(string)
	assert.NotEmpty(t, msg)
	ds := outbox(t, c)
	require.Len(t, ds, 1)
	assert.Equal(t, id, ds[0].ID)
	assert.Equal(t, model.DraftFailed, ds[0].Status)
	assert.Equal(t, msg, ds[0].Error)
	assert.Equal(t, "Rebota", ds[0].Request.Subject)
}

// Reintentar desde la bandeja de salida (con lo editado) reusa la fila y, si
// sale, la vacía: no quedan copias del mismo correo.
func TestQueueSend_RetryFromOutboxReusesDraft(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	id, err := c.QueueSend(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs(testenv.RejectedRecipient), Subject: "Primero", BodyHTML: "<p>x</p>",
	})
	require.NoError(t, err)
	waitSendFinished(t, n, id)

	again, err := c.QueueSend(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "Corregido", BodyHTML: "<p>x</p>", DraftID: id,
	})
	require.NoError(t, err)
	assert.Equal(t, id, again)
	n.mu.Lock()
	n.events = nil
	n.mu.Unlock()
	ev := waitSendFinished(t, n, id)
	assert.Equal(t, true, ev["ok"])
	assert.Empty(t, outbox(t, c))
	envs := smtp.Received()
	require.Len(t, envs, 1)
	assert.Equal(t, []string{"d@x"}, envs[0].To)
}

func TestQueueSend_ValidationErrorsAreImmediate(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.QueueSend(context.Background(), model.SendRequest{AccountID: acc.ID, Subject: "x"})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "no_recipients", ue.Code)
	assert.Empty(t, outbox(t, c))
}

func TestQueueSend_DraftAlreadySendingIsBusy(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	req := model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Subject: "x"}
	id, err := c.store.QueueLocalDraft(context.Background(), 0, req)
	require.NoError(t, err)
	req.DraftID = id
	_, err = c.QueueSend(context.Background(), req)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "busy", ue.Code)
	assert.Empty(t, smtp.Received())
}

// Si la app se cerró con un envío en curso, al volver el correo figura como
// interrumpido (pudo haber salido: no se reintenta solo).
func TestNew_MarksInterruptedSendsAsFailed(t *testing.T) {
	path := t.TempDir() + "/test.db"
	open := func() *Core {
		c, err := New(Config{DBPath: path, Secrets: secrets.NewMemory(), SkipInitialSync: true})
		require.NoError(t, err)
		return c
	}
	c := open()
	accID, err := c.store.InsertAccount(context.Background(), model.Account{Email: "a@x", Username: "a@x"})
	require.NoError(t, err)
	_, err = c.store.QueueLocalDraft(context.Background(), 0, model.SendRequest{AccountID: accID, Subject: "a medias"})
	require.NoError(t, err)
	require.NoError(t, c.Close())

	c = open()
	defer c.Close()
	ds := outbox(t, c)
	require.Len(t, ds, 1)
	assert.Equal(t, model.DraftFailed, ds[0].Status)
	assert.Equal(t, msgSendInterrupted, ds[0].Error)
}

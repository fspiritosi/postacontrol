package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
)

func TestQueueSend_WaitsAndCanBeCancelled(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	c.cfg.UndoSendDelay = time.Minute
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()

	id, err := c.QueueSend(ctx, model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Subject: "Me arrepentí", BodyHTML: "<p>x</p>"})
	require.NoError(t, err)
	require.NoError(t, c.CancelSend(ctx, id))

	// No sale: queda en la bandeja de salida para seguir editándolo.
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, smtp.Received())
	ds := outbox(t, c)
	require.Len(t, ds, 1)
	assert.Equal(t, model.DraftFailed, ds[0].Status)
	assert.Equal(t, msgSendCancelled, ds[0].Error)
	assert.Equal(t, "Me arrepentí", ds[0].Request.Subject)
	assert.Empty(t, n.matching(EventSendFinished), "cancelar no es un envío fallido")

	// Ya cancelado, no se cancela de nuevo.
	var ue *UserError
	require.ErrorAs(t, c.CancelSend(ctx, id), &ue)
}

func TestQueueSend_SendsAfterTheDelay(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	c.cfg.UndoSendDelay = 100 * time.Millisecond
	acc, _ := setupSynced(t, imap, smtp, c)

	id, err := c.QueueSend(context.Background(), model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Subject: "Sale", BodyHTML: "<p>x</p>"})
	require.NoError(t, err)
	assert.Empty(t, smtp.Received(), "espera antes de salir")
	ev := waitSendFinished(t, n, id)
	assert.Equal(t, true, ev["ok"])
	assert.Len(t, smtp.Received(), 1)

	var ue *UserError
	require.ErrorAs(t, c.CancelSend(context.Background(), id), &ue)
	assert.Equal(t, msgSendAlreadyGone, ue.Msg)
}

func TestQueueSend_UndoTurnedOffSendsRightAway(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	c.cfg.UndoSendDelay = time.Hour
	acc, _ := setupSynced(t, imap, smtp, c)
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{DisableUndoSend: true}))

	id, err := c.QueueSend(context.Background(), model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Subject: "Ya", BodyHTML: "<p>x</p>"})
	require.NoError(t, err)
	assert.Equal(t, true, waitSendFinished(t, n, id)["ok"])
}

func TestQueueSend_ClosingSendsWhatIsWaiting(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	c.cfg.UndoSendDelay = time.Hour
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.QueueSend(context.Background(), model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Subject: "Al cerrar", BodyHTML: "<p>x</p>"})
	require.NoError(t, err)

	// Cerrar la app no es arrepentirse: sale sin esperar la hora.
	start := time.Now()
	require.NoError(t, c.Close())
	assert.Less(t, time.Since(start), 10*time.Second)
	assert.Len(t, smtp.Received(), 1)
	again, err := New(Config{DBPath: c.cfg.DBPath, Secrets: secrets.NewMemory(), SkipInitialSync: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })
	ds, err := again.ListLocalDrafts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, ds)
}

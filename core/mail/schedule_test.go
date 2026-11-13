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

func scheduledReq(acc model.Account, subject string, at time.Time) model.SendRequest {
	return model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Subject: subject, BodyHTML: "<p>x</p>", SendAt: at.Unix()}
}

func TestScheduledSend_WaitsUntilItsTime(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	at := time.Now().Add(time.Hour)

	id, err := c.QueueSend(ctx, scheduledReq(acc, "Mañana", at))
	require.NoError(t, err)
	ds := outbox(t, c)
	require.Len(t, ds, 1)
	assert.Equal(t, model.DraftScheduled, ds[0].Status)
	assert.Equal(t, at.Unix(), ds[0].Request.SendAt)

	// Todavía no es la hora.
	require.NoError(t, c.sendDueScheduled(ctx, at.Add(-time.Minute)))
	assert.Empty(t, smtp.Received())
	assert.Empty(t, n.matching(EventSendFinished))

	require.NoError(t, c.sendDueScheduled(ctx, at.Add(time.Minute)))
	assert.Equal(t, true, waitSendFinished(t, n, id)["ok"])
	assert.Len(t, smtp.Received(), 1)
	assert.Empty(t, outbox(t, c))
}

func TestScheduledSend_PastTimeSendsNow(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	id, err := c.QueueSend(context.Background(), scheduledReq(acc, "Ya", time.Now().Add(-time.Hour)))
	require.NoError(t, err)
	assert.Equal(t, true, waitSendFinished(t, n, id)["ok"])
}

func TestScheduledSend_SurvivesRestart(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.QueueSend(context.Background(), scheduledReq(acc, "Después", time.Now().Add(time.Hour)))
	require.NoError(t, err)
	require.NoError(t, c.Close())

	again, err := New(Config{DBPath: c.cfg.DBPath, Secrets: secrets.NewMemory(), SkipInitialSync: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = again.Close() })
	ds, err := again.ListLocalDrafts(context.Background())
	require.NoError(t, err)
	require.Len(t, ds, 1)
	assert.Equal(t, model.DraftScheduled, ds[0].Status, "no pasa a interrumpido al volver a abrir")
}

func TestScheduledSend_SendNowUnscheduleAndReschedule(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	later := time.Now().Add(time.Hour)

	a, err := c.QueueSend(ctx, scheduledReq(acc, "A", later))
	require.NoError(t, err)
	require.NoError(t, c.SendScheduledNow(ctx, a))
	assert.Equal(t, true, waitSendFinished(t, n, a)["ok"])

	b, err := c.QueueSend(ctx, scheduledReq(acc, "B", later))
	require.NoError(t, err)
	require.NoError(t, c.UnscheduleSend(ctx, b))
	ds := outbox(t, c)
	require.Len(t, ds, 1)
	assert.Equal(t, model.DraftFailed, ds[0].Status)
	assert.Equal(t, msgUnscheduled, ds[0].Error)

	// Desde la bandeja se vuelve a programar, editado, en la misma fila.
	req := scheduledReq(acc, "B editado", later.Add(time.Hour))
	req.DraftID = b
	b2, err := c.QueueSend(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, b, b2)
	ds = outbox(t, c)
	require.Len(t, ds, 1)
	assert.Equal(t, model.DraftScheduled, ds[0].Status)
	assert.Equal(t, "B editado", ds[0].Request.Subject)

	var ue *UserError
	require.ErrorAs(t, c.SendScheduledNow(ctx, 999), &ue)
	assert.Len(t, smtp.Received(), 1)
}

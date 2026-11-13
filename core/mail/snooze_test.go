package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnooze_MovesAndComesBackUnread(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 2, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	m1 := bySubject(t, listAll(t, c, inboxID), "m-1")
	_, err := c.GetMessage(ctx, m1.ID) // leído
	require.NoError(t, err)
	until := time.Now().Add(time.Hour)

	undo, err := c.SnoozeMessages(ctx, []int64{m1.ID}, until.Unix())
	require.NoError(t, err)
	assert.NotZero(t, undo)
	assert.Equal(t, []string{"m-2"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, snoozeFolderName))
	zs, err := c.ListSnoozes(ctx)
	require.NoError(t, err)
	require.Len(t, zs, 1)
	assert.Equal(t, until.Unix(), zs[0].Until)
	assert.Equal(t, "INBOX", zs[0].ReturnFolder)

	// Mientras no sea la hora, sigue pospuesto.
	require.NoError(t, c.returnDueSnoozes(ctx, until.Add(-time.Minute)))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, snoozeFolderName))

	require.NoError(t, c.returnDueSnoozes(ctx, until.Add(time.Minute)))
	assert.ElementsMatch(t, []string{"m-1", "m-2"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, snoozeFolderName))
	assert.NotEmpty(t, n.matching(EventSnoozeReturned))
	zs, err = c.ListSnoozes(ctx)
	require.NoError(t, err)
	assert.Empty(t, zs)
	// Vuelve como no leído.
	require.NoError(t, c.SyncFolder(ctx, inboxID))
	waitFor(t, func() bool {
		for _, m := range listAll(t, c, inboxID) {
			if m.Subject == "m-1" {
				return !m.Seen
			}
		}
		return false
	})
	_ = acc
}

func TestSnooze_SnoozeForAndReturnNow(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	until := time.Now().Add(24 * time.Hour)
	_, err := c.SnoozeMessages(ctx, []int64{listAll(t, c, inboxID)[0].ID}, until.Unix())
	require.NoError(t, err)

	snoozedID := folderID(t, c, acc.ID, snoozeFolderName)
	require.NoError(t, c.SyncFolder(ctx, snoozedID))
	there := listAll(t, c, snoozedID)
	require.Len(t, there, 1)
	z, err := c.SnoozeFor(ctx, there[0].ID)
	require.NoError(t, err)
	require.NotNil(t, z)
	assert.Equal(t, until.Unix(), z.Until)

	require.NoError(t, c.Unsnooze(ctx, z.ID))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	zs, err := c.ListSnoozes(ctx)
	require.NoError(t, err)
	assert.Empty(t, zs)

	// Un correo que no está pospuesto no tiene hora de vuelta.
	require.NoError(t, c.SyncFolder(ctx, inboxID))
	back := listAll(t, c, inboxID)
	require.Len(t, back, 1)
	none, err := c.SnoozeFor(ctx, back[0].ID)
	require.NoError(t, err)
	assert.Nil(t, none)
}

func TestSnooze_MovedByHandIsForgotten(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	require.NoError(t, imap.User.Create("Otra", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	until := time.Now().Add(time.Hour)
	_, err := c.SnoozeMessages(ctx, []int64{listAll(t, c, inboxID)[0].ID}, until.Unix())
	require.NoError(t, err)

	// Alguien lo saca a mano de Pospuestos.
	snoozedID := folderID(t, c, acc.ID, snoozeFolderName)
	require.NoError(t, c.SyncFolder(ctx, snoozedID))
	require.NoError(t, c.MoveMessages(ctx, []int64{listAll(t, c, snoozedID)[0].ID}, folderID(t, c, acc.ID, "Otra")))

	require.NoError(t, c.returnDueSnoozes(ctx, until.Add(time.Minute)))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "Otra"), "no se lo toca")
	zs, err := c.ListSnoozes(ctx)
	require.NoError(t, err)
	assert.Empty(t, zs)
}

func TestSnooze_PastTimeIsRefused(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	_, err := c.SnoozeMessages(context.Background(), []int64{listAll(t, c, inboxID)[0].ID}, time.Now().Add(-time.Minute).Unix())
	var ue *UserError
	require.ErrorAs(t, err, &ue)
}

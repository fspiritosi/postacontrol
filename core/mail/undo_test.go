package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitFor espera (hasta 10 s) a que cond se cumpla: los destinos se
// sincronizan en segundo plano.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !cond() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, cond())
}

func TestUndo_MoveGoesBack(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 2, "m")
	require.NoError(t, imap.User.Create("Archivo", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	archivoID := folderID(t, c, acc.ID, "Archivo")
	m1 := bySubject(t, listAll(t, c, inboxID), "m-1")
	ctx := context.Background()

	undo, err := c.MoveMessagesUndoable(ctx, []int64{m1.ID}, archivoID)
	require.NoError(t, err)
	require.NotZero(t, undo)
	require.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "Archivo"))

	require.NoError(t, c.UndoMove(ctx, undo))
	assert.ElementsMatch(t, []string{"m-1", "m-2"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Archivo"))
	// La caché se pone al día sola: vuelve a la Entrada y sale de Archivo.
	waitFor(t, func() bool { return len(listAll(t, c, inboxID)) == 2 && len(listAll(t, c, archivoID)) == 0 })

	// Se deshace una sola vez.
	var ue *UserError
	require.ErrorAs(t, c.UndoMove(ctx, undo), &ue)
}

func TestUndo_TrashAndArchiveGoBack(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 2, "m")
	require.NoError(t, imap.User.Create("Trash", nil))
	_, inboxID := setupSynced(t, imap, smtp, c)
	ms := listAll(t, c, inboxID)
	ctx := context.Background()

	undo, err := c.DeleteMessagesUndoable(ctx, []int64{bySubject(t, ms, "m-1").ID})
	require.NoError(t, err)
	require.NoError(t, c.UndoMove(ctx, undo))
	assert.ElementsMatch(t, []string{"m-1", "m-2"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Trash"))

	// Archivar crea la carpeta; deshacer devuelve el correo (la carpeta queda).
	undo, err = c.ArchiveMessagesUndoable(ctx, []int64{bySubject(t, ms, "m-2").ID})
	require.NoError(t, err)
	require.NoError(t, c.UndoMove(ctx, undo))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Archivo"))
	assert.Contains(t, serverSubjects(t, c, imap, testUser, "INBOX"), "m-2")
}

func TestUndo_PermanentDeleteIsNotUndoable(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	require.NoError(t, imap.User.Create("Trash", nil))
	imap.Deliver(t, "Trash", rawMail("Bea <bea@ej.com>", "t-1"))
	acc, _ := setupSynced(t, imap, smtp, c)
	trashID := folderID(t, c, acc.ID, "Trash")
	require.NoError(t, c.SyncFolder(context.Background(), trashID))

	undo, err := c.DeleteMessagesUndoable(context.Background(), []int64{listAll(t, c, trashID)[0].ID})
	require.NoError(t, err)
	assert.Zero(t, undo)
}

func TestUndo_FindsByMessageIDWithoutCopyUID(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	require.NoError(t, imap.User.Create("Archivo", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	archivoID := folderID(t, c, acc.ID, "Archivo")
	ctx := context.Background()

	undo, err := c.MoveMessagesUndoable(ctx, []int64{listAll(t, c, inboxID)[0].ID}, archivoID)
	require.NoError(t, err)
	// Como un servidor que no informa los UID nuevos (sin UIDPLUS).
	c.undoMu.Lock()
	e := c.undos[undo]
	for i := range e.batches {
		e.batches[i].destUIDs = nil
	}
	c.undos[undo] = e
	c.undoMu.Unlock()

	require.NoError(t, c.UndoMove(ctx, undo))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Archivo"))
}

func TestUndo_UnknownID(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	var ue *UserError
	require.ErrorAs(t, c.UndoMove(context.Background(), 12345), &ue)
	assert.Equal(t, msgUndoGone, ue.Msg)
}

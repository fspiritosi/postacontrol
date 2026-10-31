package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestThreadRoot_Rule(t *testing.T) {
	assert.Equal(t, "<r>", threadRoot("<m>", "<p>", []string{"<r>", "<p>"}, 7))
	assert.Equal(t, "<p>", threadRoot("<m>", "<p>", nil, 7))
	assert.Equal(t, "<m>", threadRoot("<m>", "", nil, 7))
	assert.Equal(t, "id:7", threadRoot("", "", nil, 7))
	assert.Equal(t, "<p>", threadRoot("<m>", "<p>", []string{""}, 7))
}

func msg(uid uint32, id, reply string, refs []string, from string, date time.Time, seen bool) HeaderRow {
	h := hdr(uid, date, seen)
	h.MessageID, h.InReplyTo, h.References = id, reply, refs
	h.From = model.Address{Name: from, Email: from + "@x.com"}
	return h
}

// threadSeed crea INBOX y Sent con rol sent en una cuenta.
func threadSeed(t *testing.T, s *Store) (inbox, sent int64) {
	t.Helper()
	ctx := context.Background()
	acc, err := s.InsertAccount(ctx, hostinger())
	require.NoError(t, err)
	require.NoError(t, s.ReplaceFolders(ctx, acc, []model.Folder{
		{Name: "INBOX", Delimiter: ".", Role: "inbox", Selectable: true},
		{Name: "Sent", Delimiter: ".", Role: "sent", Selectable: true},
	}))
	fs, err := s.ListFolders(ctx, acc)
	require.NoError(t, err)
	for _, f := range fs {
		if f.Name == "INBOX" {
			inbox = f.ID
		} else {
			sent = f.ID
		}
	}
	return
}

func TestThreads_GroupsReplies(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, _ := threadSeed(t, s)
	b := time.Unix(1700000000, 0)
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{
		msg(1, "<a>", "", nil, "ana", b, true),
		msg(2, "<b>", "<a>", []string{"<a>"}, "beto", b.Add(time.Hour), true),
		msg(3, "<c>", "<b>", []string{"<a>", "<b>"}, "ana", b.Add(2*time.Hour), false),
	}))
	ts, err := s.ListThreads(ctx, in, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, "<a>", ts[0].ThreadRoot)
	assert.Equal(t, 3, ts[0].Count)
	assert.Equal(t, 1, ts[0].Unread)
	assert.EqualValues(t, 3, ts[0].Latest.UID)
	assert.Equal(t, []string{"ana", "beto"}, ts[0].Participants)
}

func TestThreads_ReplyBeforeParent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, _ := threadSeed(t, s)
	b := time.Unix(1700000000, 0)
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{msg(2, "<b>", "<a>", []string{"<a>"}, "beto", b.Add(time.Hour), false)}))
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{msg(1, "<a>", "", nil, "ana", b, false)}))
	ts, err := s.ListThreads(ctx, in, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, 2, ts[0].Count)
}

func TestThreads_ParentRootReused(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, _ := threadSeed(t, s)
	b := time.Unix(1700000000, 0)
	// <b> responde a <a> pero sin References; <c> responde a <b> sin References.
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{
		msg(1, "<a>", "", nil, "ana", b, true),
		msg(2, "<b>", "<a>", nil, "beto", b.Add(time.Hour), true),
		msg(3, "<c>", "<b>", nil, "ana", b.Add(2*time.Hour), true),
	}))
	ts, err := s.ListThreads(ctx, in, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, 3, ts[0].Count)
}

func TestThreads_NoIdentifiers(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, _ := threadSeed(t, s)
	b := time.Unix(1700000000, 0)
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{msg(1, "", "", nil, "ana", b, true), msg(2, "", "", nil, "ana", b, true)}))
	ts, err := s.ListThreads(ctx, in, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 2)
	assert.NotEqual(t, ts[0].ThreadRoot, ts[1].ThreadRoot)
	assert.Contains(t, ts[0].ThreadRoot, "id:")
}

func TestThreads_IncludesOwnRepliesFromSent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, sent := threadSeed(t, s)
	b := time.Unix(1700000000, 0)
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{msg(1, "<a>", "", nil, "ana", b, false)}))
	require.NoError(t, s.UpsertHeaders(ctx, sent, []HeaderRow{msg(1, "<r>", "<a>", []string{"<a>"}, "yo", b.Add(time.Hour), true)}))
	ts, err := s.ListThreads(ctx, in, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, 2, ts[0].Count)
	assert.Equal(t, in, ts[0].Latest.FolderID)
	assert.Equal(t, []string{"yo", "ana"}, ts[0].Participants)
	ms, err := s.ThreadMessages(ctx, in, "<a>")
	require.NoError(t, err)
	require.Len(t, ms, 2)
	assert.Equal(t, in, ms[0].FolderID)
	assert.Equal(t, sent, ms[1].FolderID)
}

func TestThreads_UnreadOnlyAndOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, _ := threadSeed(t, s)
	b := time.Unix(1700000000, 0)
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{
		msg(1, "<a>", "", nil, "ana", b, false),
		msg(2, "<x>", "", nil, "xavi", b.Add(time.Hour), true),
		msg(3, "<a2>", "<a>", []string{"<a>"}, "ana", b.Add(2*time.Hour), true),
	}))
	ts, err := s.ListThreads(ctx, in, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 2)
	assert.Equal(t, "<a>", ts[0].ThreadRoot)
	assert.Equal(t, "<x>", ts[1].ThreadRoot)
	ts, err = s.ListThreads(ctx, in, model.ListQuery{UnreadOnly: true})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, "<a>", ts[0].ThreadRoot)
	ts, err = s.ListThreads(ctx, in, model.ListQuery{Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, "<x>", ts[0].ThreadRoot)
}

func TestThreads_NoDuplicatesInSent(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, sent := threadSeed(t, s)
	b := time.Unix(1700000000, 0)
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{msg(1, "<a>", "", nil, "ana", b, true)}))
	require.NoError(t, s.UpsertHeaders(ctx, sent, []HeaderRow{msg(1, "<r>", "<a>", []string{"<a>"}, "yo", b.Add(time.Hour), true)}))
	require.NoError(t, s.UpsertHeaders(ctx, sent, []HeaderRow{msg(2, "<r2>", "<r>", []string{"<a>"}, "yo", b.Add(2*time.Hour), true)}))
	// Con Enviados abierta solo cuentan los suyos, cada uno una vez.
	ms, err := s.ThreadMessages(ctx, sent, "<a>")
	require.NoError(t, err)
	assert.Len(t, ms, 2)
	ts, err := s.ListThreads(ctx, sent, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, 2, ts[0].Count)
}

func rootOf(t *testing.T, s *Store, folderID int64, uid uint32) string {
	t.Helper()
	var root string
	require.NoError(t, s.db.QueryRow(`SELECT thread_root FROM messages WHERE folder_id = ? AND uid = ?`, folderID, uid).Scan(&root))
	return root
}

func TestThreads_RepliesBeforeParentsWithoutReferences(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	in, sent := threadSeed(t, s)
	other := seedSecondFolder(t, s)
	b := time.Unix(1700000000, 0)
	// Otra cuenta con un mensaje que también responde a <b>: no se toca.
	require.NoError(t, s.UpsertHeaders(ctx, other, []HeaderRow{msg(1, "<z>", "<b>", nil, "zoe", b, true)}))
	// La sincronización va de lo más nuevo a lo más viejo: C, después B, después A.
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{msg(3, "<c>", "<b>", nil, "ana", b.Add(2*time.Hour), true)}))
	require.NoError(t, s.UpsertHeaders(ctx, sent, []HeaderRow{msg(2, "<b>", "<a>", nil, "yo", b.Add(time.Hour), true)}))
	require.NoError(t, s.UpsertHeaders(ctx, in, []HeaderRow{msg(1, "<a>", "", nil, "ana", b, true)}))

	assert.Equal(t, "<a>", rootOf(t, s, in, 3))
	assert.Equal(t, "<a>", rootOf(t, s, sent, 2))
	assert.Equal(t, "<a>", rootOf(t, s, in, 1))
	assert.Equal(t, "<b>", rootOf(t, s, other, 1))
	ts, err := s.ListThreads(ctx, in, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, 3, ts[0].Count)
}

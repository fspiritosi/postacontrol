package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestMigrate_V17(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v16.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	require.NoError(t, migrateTo(db, 16))
	require.NoError(t, db.Close())

	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	for _, tbl := range []string{"local_tasks", "task_links", "task_link_messages", "taskapp_cache"} {
		var n int
		require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&n))
		assert.Equal(t, 1, n, tbl)
	}
}

func TestLocalTask_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rb := 15
	got, err := s.SaveLocalTask(ctx, model.LocalTask{Title: "Llamar", Notes: "n", DueDate: "2026-12-01", DueTime: "10:30", RemindBefore: &rb, SourceMessageID: "<a@x>", SourceSubject: "Hola"}, false)
	require.NoError(t, err)
	require.NotZero(t, got.ID)
	assert.False(t, got.CreatedAt.IsZero())

	read, err := s.GetLocalTask(ctx, got.ID)
	require.NoError(t, err)
	assert.Equal(t, "Llamar", read.Title)
	require.NotNil(t, read.RemindBefore)
	assert.Equal(t, 15, *read.RemindBefore)
	assert.Nil(t, read.DoneAt)

	_, err = s.MarkReminded(ctx, got, time.Unix(1000, 0))
	require.NoError(t, err)
	read.Title = "Llamar a Ana"
	read.RemindBefore = nil
	_, err = s.SaveLocalTask(ctx, read, false)
	require.NoError(t, err)
	var reminded sql.NullInt64
	require.NoError(t, s.db.QueryRow(`SELECT reminded_at FROM local_tasks WHERE id=?`, got.ID).Scan(&reminded))
	assert.True(t, reminded.Valid, "rearm=false conserva reminded_at")
	again, err := s.GetLocalTask(ctx, got.ID)
	require.NoError(t, err)
	assert.Equal(t, "Llamar a Ana", again.Title)
	assert.Nil(t, again.RemindBefore)
	assert.Equal(t, got.CreatedAt.Unix(), again.CreatedAt.Unix())

	_, err = s.SaveLocalTask(ctx, again, true)
	require.NoError(t, err)
	require.NoError(t, s.db.QueryRow(`SELECT reminded_at FROM local_tasks WHERE id=?`, got.ID).Scan(&reminded))
	assert.False(t, reminded.Valid, "rearm=true limpia reminded_at")

	_, err = s.SaveLocalTask(ctx, model.LocalTask{ID: 999, Title: "x"}, false)
	assert.ErrorIs(t, err, sql.ErrNoRows)
	_, err = s.GetLocalTask(ctx, 999)
	assert.ErrorIs(t, err, sql.ErrNoRows)

	require.NoError(t, s.DeleteLocalTask(ctx, got.ID))
	require.NoError(t, s.DeleteLocalTask(ctx, got.ID), "borrar uno inexistente no falla")
}

func TestListLocalTasks_HidesDone(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a, err := s.SaveLocalTask(ctx, model.LocalTask{Title: "a"}, false)
	require.NoError(t, err)
	b, err := s.SaveLocalTask(ctx, model.LocalTask{Title: "b"}, false)
	require.NoError(t, err)
	at := time.Unix(5000, 0)
	require.NoError(t, s.SetLocalTaskDone(ctx, b.ID, &at))

	open, err := s.ListLocalTasks(ctx, false)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, a.ID, open[0].ID)
	all, err := s.ListLocalTasks(ctx, true)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	require.NoError(t, s.SetLocalTaskDone(ctx, b.ID, nil))
	open, err = s.ListLocalTasks(ctx, false)
	require.NoError(t, err)
	assert.Len(t, open, 2)
}

func TestPendingReminders_OnlyTimedUndone(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	mk := func(title, due, tm string) int64 {
		got, err := s.SaveLocalTask(ctx, model.LocalTask{Title: title, DueDate: due, DueTime: tm}, false)
		require.NoError(t, err)
		return got.ID
	}
	mk("sin hora", "2026-12-01", "")
	done := mk("hecha", "2026-12-01", "09:00")
	remindedID := mk("avisada", "2026-12-01", "09:00")
	pending := mk("pendiente", "2026-12-01", "09:00")
	at := time.Unix(10, 0)
	require.NoError(t, s.SetLocalTaskDone(ctx, done, &at))
	reminded, err := s.GetLocalTask(ctx, remindedID)
	require.NoError(t, err)
	ok, err := s.MarkReminded(ctx, reminded, at)
	require.NoError(t, err)
	require.True(t, ok)

	got, err := s.PendingReminders(ctx)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, pending, got[0].ID)
}

func TestTaskLinks_ByThread(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	l1, err := s.AddTaskLink(ctx, model.TaskLink{ThreadRoot: "r1", TaskID: 1, ProjectName: "P", Title: "uno", AttachPending: true}, "<m1@x>")
	require.NoError(t, err)
	l2, err := s.AddTaskLink(ctx, model.TaskLink{ThreadRoot: "r1", TaskID: 2, Ticket: true, Title: "dos"}, "")
	require.NoError(t, err)
	_, err = s.AddTaskLink(ctx, model.TaskLink{ThreadRoot: "r2", TaskID: 3, Title: "tres"}, "")
	require.NoError(t, err)

	got, err := s.TaskLinksForThread(ctx, "r1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, l1.ID, got[0].ID)
	assert.Equal(t, l2.ID, got[1].ID)
	assert.True(t, got[0].AttachPending)
	assert.True(t, got[1].Ticket)
	assert.False(t, got[0].CreatedAt.IsZero())

	ok, err := s.AddedToLink(ctx, l1.ID, "<m1@x>")
	require.NoError(t, err)
	assert.True(t, ok, "el correo de origen queda sumado")

	one, err := s.GetTaskLink(ctx, l1.ID)
	require.NoError(t, err)
	assert.Equal(t, "uno", one.Title)
	_, err = s.GetTaskLink(ctx, 999)
	assert.ErrorIs(t, err, sql.ErrNoRows)

	require.NoError(t, s.SetAttachPending(ctx, l1.ID, false))
	one, err = s.GetTaskLink(ctx, l1.ID)
	require.NoError(t, err)
	assert.False(t, one.AttachPending)
}

func TestAddedToLink(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	l, err := s.AddTaskLink(ctx, model.TaskLink{ThreadRoot: "r", TaskID: 1}, "")
	require.NoError(t, err)
	ok, err := s.AddedToLink(ctx, l.ID, "<x@y>")
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, s.MarkAddedToLink(ctx, l.ID, "<x@y>"))
	require.NoError(t, s.MarkAddedToLink(ctx, l.ID, "<x@y>"))
	ok, err = s.AddedToLink(ctx, l.ID, "<x@y>")
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestMessageTaskMeta_EmptyRoot(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	ins := `INSERT INTO messages (folder_id, uid, message_id, thread_root, size, date) VALUES (?,?,?,?,?,?)`
	r1, err := s.db.Exec(ins, fid, 1, "<a@x>", "", 123, 100)
	require.NoError(t, err)
	id1, _ := r1.LastInsertId()
	r2, err := s.db.Exec(ins, fid, 2, "<a@x>", "root", 456, 200)
	require.NoError(t, err)
	id2, _ := r2.LastInsertId()

	h, root, size, err := s.MessageTaskMeta(ctx, id1)
	require.NoError(t, err)
	assert.Equal(t, "<a@x>", h)
	assert.Equal(t, "local:"+strconv.FormatInt(id1, 10), root)
	assert.EqualValues(t, 123, size)
	_, root, _, err = s.MessageTaskMeta(ctx, id2)
	require.NoError(t, err)
	assert.Equal(t, "root", root)
	_, _, _, err = s.MessageTaskMeta(ctx, 999)
	assert.ErrorIs(t, err, sql.ErrNoRows)

	got, err := s.MessageIDByHeader(ctx, "<a@x>")
	require.NoError(t, err)
	assert.Equal(t, id2, got, "el más nuevo")
	_, err = s.MessageIDByHeader(ctx, "<nope>")
	assert.ErrorIs(t, err, sql.ErrNoRows)
	_, err = s.MessageIDByHeader(ctx, "")
	assert.ErrorIs(t, err, sql.ErrNoRows)
}

func TestTaskappCache(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, _, ok, err := s.TaskappCache(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, s.SaveTaskappCache(ctx, `{"a":1}`, time.Unix(777, 0)))
	require.NoError(t, s.SaveTaskappCache(ctx, `{"a":2}`, time.Unix(888, 0)))
	js, at, ok, err := s.TaskappCache(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, `{"a":2}`, js)
	assert.EqualValues(t, 888, at.Unix())

	l, err := s.AddTaskLink(ctx, model.TaskLink{ThreadRoot: "r", TaskID: 1}, "")
	require.NoError(t, err)
	require.NoError(t, s.ClearTaskappData(ctx))
	_, _, ok, err = s.TaskappCache(ctx)
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = s.GetTaskLink(ctx, l.ID)
	assert.NoError(t, err, "los vínculos se conservan")
}

func TestMarkReminded_FalseWhenChangedSinceSnapshot(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	snap, err := s.SaveLocalTask(ctx, model.LocalTask{Title: "x", DueDate: "2026-12-01", DueTime: "09:00"}, false)
	require.NoError(t, err)
	edited := snap
	edited.DueTime = "15:00"
	_, err = s.SaveLocalTask(ctx, edited, true)
	require.NoError(t, err)

	ok, err := s.MarkReminded(ctx, snap, time.Unix(10, 0))
	require.NoError(t, err)
	assert.False(t, ok)
	pending, err := s.PendingReminders(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)

	ok, err = s.MarkReminded(ctx, edited, time.Unix(10, 0))
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.MarkReminded(ctx, edited, time.Unix(11, 0))
	require.NoError(t, err)
	assert.False(t, ok)
}

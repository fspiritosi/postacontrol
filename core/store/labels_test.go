package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

// labelAccount crea una cuenta con email propio y dos carpetas (INBOX y Otra).
func labelAccount(t *testing.T, s *Store, email string) (acc, f1, f2 int64) {
	t.Helper()
	ctx := context.Background()
	a := hostinger()
	a.Email, a.Username = email, email
	acc, err := s.InsertAccount(ctx, a)
	require.NoError(t, err)
	require.NoError(t, s.ReplaceFolders(ctx, acc, []model.Folder{
		{Name: "INBOX", Delimiter: ".", Role: "inbox", Selectable: true},
		{Name: "Otra", Delimiter: ".", Selectable: true},
	}))
	fs, err := s.ListFolders(ctx, acc)
	require.NoError(t, err)
	for _, f := range fs {
		if f.Name == "INBOX" {
			f1 = f.ID
		} else {
			f2 = f.ID
		}
	}
	return
}

func mkLabel(t *testing.T, s *Store, name, keyword string) model.Label {
	t.Helper()
	l, err := s.InsertLabel(context.Background(), name, "#2563eb", keyword)
	require.NoError(t, err)
	return l
}

func labelsOf(t *testing.T, s *Store, folder int64, uid uint32) []int64 {
	t.Helper()
	ms, err := s.ListMessages(context.Background(), folder, model.ListQuery{})
	require.NoError(t, err)
	for _, m := range ms {
		if m.UID == uid {
			return m.Labels
		}
	}
	t.Fatal("mensaje no encontrado")
	return nil
}

func TestLabelsCRUDAndOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	b := mkLabel(t, s, "b", "$pc_bbbbbbbb")
	c := mkLabel(t, s, "c", "$pc_cccccccc")
	require.NoError(t, s.SetLabelPositions(ctx, []int64{c.ID, a.ID, b.ID}))
	ls, err := s.ListLabels(ctx)
	require.NoError(t, err)
	names := []string{}
	for _, l := range ls {
		names = append(names, l.Name)
	}
	assert.Equal(t, []string{"c", "a", "b"}, names)

	a.Name, a.Color = "a2", "#dc2626"
	require.NoError(t, s.UpdateLabel(ctx, a))
	kw, err := s.LabelKeyword(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, "$pc_aaaaaaaa", kw)
	ls, _ = s.ListLabels(ctx)
	assert.Equal(t, "a2", ls[1].Name)
	assert.Equal(t, "#dc2626", ls[1].Color)

	m, err := s.LabelByKeyword(ctx)
	require.NoError(t, err)
	assert.Equal(t, b.ID, m["$pc_bbbbbbbb"])
}

func TestInsertLabelGoesLast(t *testing.T) {
	s := newStore(t)
	a := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	b := mkLabel(t, s, "b", "$pc_bbbbbbbb")
	assert.Greater(t, b.Position, a.Position)
}

func TestUpsertHeadersMapsKeywords(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, f := seedFolder(t, s)
	l := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	h := hdr(1, t0, false)
	h.Keywords = []string{"$pc_aaaaaaaa", "$label1", "$pc_ffffffff"}
	require.NoError(t, s.UpsertHeaders(ctx, f, []HeaderRow{h}))
	assert.Equal(t, []int64{l.ID}, labelsOf(t, s, f, 1))
	h.Keywords = nil
	require.NoError(t, s.UpsertHeaders(ctx, f, []HeaderRow{h}))
	assert.Equal(t, []int64{}, labelsOf(t, s, f, 1))
}

func TestUpsertHeadersKeepsLocalLabels(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	acc, f := seedFolder(t, s)
	l := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	ch, err := s.SetAccountLabelsLocal(ctx, acc)
	require.NoError(t, err)
	assert.True(t, ch)
	ch, err = s.SetAccountLabelsLocal(ctx, acc)
	require.NoError(t, err)
	assert.False(t, ch)
	local, err := s.AccountLabelsLocal(ctx, acc)
	require.NoError(t, err)
	assert.True(t, local)

	id := put(t, s, f, 1, "x", "y", t0)
	require.NoError(t, s.AddMessageLabel(ctx, []int64{id}, l.ID, true))
	h := hdr(1, t0, true)
	h.Keywords = nil
	require.NoError(t, s.UpsertHeaders(ctx, f, []HeaderRow{h}))
	assert.Equal(t, []int64{l.ID}, labelsOf(t, s, f, 1))
}

func TestLabelCarryAppliesOnInsert(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	acc, f1, f2 := labelAccount(t, s, "uno@ejemplo.com")
	l := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	_, err := s.SetAccountLabelsLocal(ctx, acc)
	require.NoError(t, err)
	require.NoError(t, s.SaveLabelCarry(ctx, acc, "<x@y>", []int64{l.ID}))

	h := hdr(1, t0, false)
	h.MessageID = "<x@y>"
	require.NoError(t, s.UpsertHeaders(ctx, f2, []HeaderRow{h}))
	assert.Equal(t, []int64{l.ID}, labelsOf(t, s, f2, 1))

	require.NoError(t, s.UpsertHeaders(ctx, f1, []HeaderRow{h}))
	assert.Equal(t, []int64{}, labelsOf(t, s, f1, 1))
}

func TestDeleteLabelCascadesAndQueuesCleanup(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	acc, f := seedFolder(t, s)
	l := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	id := put(t, s, f, 1, "x", "y", t0)
	require.NoError(t, s.SetMessageLabels(ctx, id, []int64{l.ID}))
	n, err := s.CountLabeled(ctx, l.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	kw, err := s.DeleteLabel(ctx, l.ID, []int64{acc})
	require.NoError(t, err)
	assert.Equal(t, "$pc_aaaaaaaa", kw)
	n, _ = s.CountLabeled(ctx, l.ID)
	assert.Equal(t, 0, n)
	assert.Equal(t, []int64{}, labelsOf(t, s, f, 1))
	p, err := s.PendingLabelCleanups(ctx, acc)
	require.NoError(t, err)
	assert.Equal(t, []string{"$pc_aaaaaaaa"}, p)
	require.NoError(t, s.DoneLabelCleanup(ctx, acc, "$pc_aaaaaaaa"))
	p, _ = s.PendingLabelCleanups(ctx, acc)
	assert.Empty(t, p)
}

func TestListLabeledScopesAndFilters(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	acc1, a1, a2 := labelAccount(t, s, "uno@ejemplo.com")
	_, b1, b2 := labelAccount(t, s, "dos@ejemplo.com")
	l := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	var ids []int64
	for i, f := range []int64{a1, a2, b1, b2} {
		id := put(t, s, f, 1, "m", "y", t0.Add(time.Duration(i)*time.Hour))
		ids = append(ids, id)
		require.NoError(t, s.AddMessageLabel(ctx, []int64{id}, l.ID, true))
	}
	all, err := s.ListLabeled(ctx, l.ID, 0, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, all, 4)
	assert.Equal(t, []int64{ids[3], ids[2], ids[1], ids[0]}, []int64{all[0].ID, all[1].ID, all[2].ID, all[3].ID})
	one, err := s.ListLabeled(ctx, l.ID, acc1, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, one, 2)
	for _, m := range one {
		assert.Equal(t, acc1, m.AccountID)
	}
	// Dos mensajes leídos: UnreadOnly los descarta.
	h := hdr(1, t0, true)
	require.NoError(t, s.UpsertHeaders(ctx, a1, []HeaderRow{h}))
	unread, err := s.ListLabeled(ctx, l.ID, 0, model.ListQuery{UnreadOnly: true})
	require.NoError(t, err)
	assert.Len(t, unread, 3)
}

func TestThreadLabelsUnion(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	inbox, _ := threadSeed(t, s)
	a := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	b := mkLabel(t, s, "b", "$pc_bbbbbbbb")
	require.NoError(t, s.UpsertHeaders(ctx, inbox, []HeaderRow{
		msg(1, "<a>", "", nil, "ana", t0, true),
		msg(2, "<b>", "<a>", nil, "luis", t0.Add(time.Hour), true),
	}))
	m1 := labelsMsgID(t, s, inbox, 1)
	m2 := labelsMsgID(t, s, inbox, 2)
	require.NoError(t, s.AddMessageLabel(ctx, []int64{m2}, b.ID, true))
	require.NoError(t, s.AddMessageLabel(ctx, []int64{m1}, a.ID, true))
	ts, err := s.ListThreads(ctx, inbox, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ts, 1)
	assert.Equal(t, []int64{a.ID, b.ID}, ts[0].Labels)
	assert.Equal(t, []int64{b.ID}, ts[0].Latest.Labels)
}

func labelsMsgID(t *testing.T, s *Store, folder int64, uid uint32) int64 {
	t.Helper()
	ms, err := s.ListMessages(context.Background(), folder, model.ListQuery{})
	require.NoError(t, err)
	for _, m := range ms {
		if m.UID == uid {
			return m.ID
		}
	}
	t.Fatal("mensaje no encontrado")
	return 0
}

func TestThreadLabelsEmptyNotNil(t *testing.T) {
	s := newStore(t)
	inbox, _ := threadSeed(t, s)
	require.NoError(t, s.UpsertHeaders(context.Background(), inbox, []HeaderRow{msg(1, "<a>", "", nil, "ana", t0, true)}))
	ts, err := s.ListThreads(context.Background(), inbox, model.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, []int64{}, ts[0].Labels)
}

func TestSearchByLabel(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, f := seedFolder(t, s)
	l := mkLabel(t, s, "a", "$pc_aaaaaaaa")
	id1 := put(t, s, f, 1, "factura uno", "x", t0)
	put(t, s, f, 2, "factura dos", "x", t0.Add(time.Hour))
	require.NoError(t, s.AddMessageLabel(ctx, []int64{id1}, l.ID, true))
	ms, err := s.Search(ctx, model.SearchQuery{Text: "factura", LabelID: l.ID})
	require.NoError(t, err)
	require.Len(t, ms, 1)
	assert.Equal(t, id1, ms[0].ID)
}

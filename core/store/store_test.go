package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "sub", "t.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func hostinger() model.Account {
	return model.Account{
		Email: "yo@ejemplo.com", DisplayName: "Yo", Username: "yo@ejemplo.com", Color: "#336699",
		IMAP:         model.ServerSettings{Host: "imap.hostinger.com", Port: 993, Security: model.SecurityTLS},
		SMTP:         model.ServerSettings{Host: "smtp.hostinger.com", Port: 465, Security: model.SecurityTLS},
		SaveSentCopy: true,
	}
}

func seedFolder(t *testing.T, s *Store) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	accID, err := s.InsertAccount(ctx, hostinger())
	require.NoError(t, err)
	require.NoError(t, s.ReplaceFolders(ctx, accID, []model.Folder{{Name: "INBOX", Delimiter: ".", Role: "inbox", Selectable: true}}))
	fs, err := s.ListFolders(ctx, accID)
	require.NoError(t, err)
	require.Len(t, fs, 1)
	return accID, fs[0].ID
}

func hdr(uid uint32, date time.Time, seen bool) HeaderRow {
	return HeaderRow{
		MessageSummary: model.MessageSummary{
			UID: uid, From: model.Address{Name: "A", Email: "a@x.com"}, Subject: "s", Date: date, Seen: seen,
		},
		MessageID: "<m@x>",
	}
}

func uids(t *testing.T, s *Store, folderID int64) []uint32 {
	t.Helper()
	u, err := s.KnownUIDs(context.Background(), folderID)
	require.NoError(t, err)
	return u
}

func TestOpen_MigratesAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	s, err := Open(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	s, err = Open(path)
	require.NoError(t, err)
	defer s.Close()
	var v int
	require.NoError(t, s.db.QueryRow("PRAGMA user_version").Scan(&v))
	assert.Equal(t, 1, v)
	var fk int
	require.NoError(t, s.db.QueryRow("PRAGMA foreign_keys").Scan(&fk))
	assert.Equal(t, 1, fk)
}

func TestAccounts_CRUD(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a := hostinger()
	id, err := s.InsertAccount(ctx, a)
	require.NoError(t, err)
	a.ID = id
	got, err := s.GetAccount(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, a, got)
	list, err := s.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	require.NoError(t, s.DeleteAccount(ctx, id))
	_, err = s.GetAccount(ctx, id)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestReplaceFolders_UpsertsAndRemovesMissing(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, err := s.InsertAccount(ctx, hostinger())
	require.NoError(t, err)
	mk := func(names ...string) []model.Folder {
		var out []model.Folder
		for _, n := range names {
			out = append(out, model.Folder{Name: n, Delimiter: ".", Selectable: true})
		}
		return out
	}
	require.NoError(t, s.ReplaceFolders(ctx, accID, mk("INBOX", "INBOX.Sent", "Old")))
	before, err := s.ListFolders(ctx, accID)
	require.NoError(t, err)
	require.Len(t, before, 3)
	ids := map[string]int64{}
	for _, f := range before {
		ids[f.Name] = f.ID
	}
	require.NoError(t, s.SetFolderCounts(ctx, ids["INBOX"], 2, 9))

	require.NoError(t, s.ReplaceFolders(ctx, accID, mk("INBOX", "INBOX.Sent")))
	after, err := s.ListFolders(ctx, accID)
	require.NoError(t, err)
	require.Len(t, after, 2)
	for _, f := range after {
		assert.Equal(t, ids[f.Name], f.ID)
		if f.Name == "INBOX" {
			assert.Equal(t, 2, f.Unread)
			assert.Equal(t, 9, f.Total)
		}
	}
	assert.Equal(t, "INBOX", after[0].Name)
}

func TestFolderSync_ZeroForNew(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	f, fs, err := s.GetFolder(ctx, fid)
	require.NoError(t, err)
	assert.Equal(t, "INBOX", f.Name)
	assert.Equal(t, FolderSync{}, fs)
	require.NoError(t, s.SetFolderSync(ctx, fid, FolderSync{UIDValidity: 7, UIDNext: 12}))
	_, fs, err = s.GetFolder(ctx, fid)
	require.NoError(t, err)
	assert.Equal(t, FolderSync{UIDValidity: 7, UIDNext: 12}, fs)
	_, _, err = s.GetFolder(ctx, 9999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpsertHeaders_ListOrderAndUnreadFilter(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	base := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{
		hdr(1, base, false), hdr(2, base.Add(2*time.Hour), true), hdr(3, base.Add(time.Hour), false),
	}))
	all, err := s.ListMessages(ctx, fid, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, []uint32{2, 3, 1}, []uint32{all[0].UID, all[1].UID, all[2].UID})
	assert.True(t, all[0].Date.Equal(base.Add(2*time.Hour)))
	unread, err := s.ListMessages(ctx, fid, model.ListQuery{UnreadOnly: true})
	require.NoError(t, err)
	assert.Len(t, unread, 2)
	page, err := s.ListMessages(ctx, fid, model.ListQuery{Offset: 1, Limit: 1})
	require.NoError(t, err)
	require.Len(t, page, 1)
	assert.Equal(t, uint32(3), page[0].UID)

	// re-upsert actualiza flags sin duplicar
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{hdr(1, base, true)}))
	assert.Equal(t, []uint32{1, 2, 3}, uids(t, s, fid))
	require.NoError(t, s.SetFlags(ctx, fid, 3, true, true, true))
	unread, err = s.ListMessages(ctx, fid, model.ListQuery{UnreadOnly: true})
	require.NoError(t, err)
	assert.Empty(t, unread)
}

func TestUpsertHeaders_AtomicOnFailure(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	now := time.Now()
	// las dos primeras filas son válidas; la tercera (UID 0) viola el CHECK de uid
	err := s.UpsertHeaders(ctx, fid, []HeaderRow{hdr(1, now, false), hdr(2, now, false), hdr(0, now, false)})
	require.Error(t, err)
	assert.Empty(t, uids(t, s, fid), "ninguna fila del lote debe persistir")
}

func TestUpsertHeaders_KeepsCachedBodyOnReupsert(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	row := hdr(1, time.Now(), false)
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{row}))
	list, err := s.ListMessages(ctx, fid, model.ListQuery{})
	require.NoError(t, err)
	id := list[0].ID
	att := []model.Attachment{{Index: 0, Filename: "a.pdf", MIMEType: "application/pdf", Size: 1, Part: []int{2}}}
	require.NoError(t, s.SetBody(ctx, id, BodyRow{HTML: "<p>x</p>", Text: "x", Snippet: "resumen", Attachments: att}))

	row.Snippet = "" // las cabeceras no traen snippet
	row.HasAttachments = false
	row.Seen = true
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{row}))
	d, cached, err := s.GetMessage(ctx, id)
	require.NoError(t, err)
	assert.True(t, cached)
	assert.Equal(t, "<p>x</p>", d.HTML)
	assert.Equal(t, "x", d.Text)
	assert.Equal(t, "resumen", d.Snippet)
	assert.Equal(t, att, d.Attachments)
	assert.True(t, d.HasAttachments)
	assert.True(t, d.Seen, "los flags sí se actualizan")
}

func TestListMessages_Paging(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	var rows []HeaderRow
	for i := 1; i <= 5; i++ {
		rows = append(rows, hdr(uint32(i), base.Add(time.Duration(i)*time.Hour), false))
	}
	require.NoError(t, s.UpsertHeaders(ctx, fid, rows))
	get := func(q model.ListQuery) []uint32 {
		l, err := s.ListMessages(ctx, fid, q)
		require.NoError(t, err)
		out := []uint32{}
		for _, m := range l {
			out = append(out, m.UID)
		}
		return out
	}
	assert.Equal(t, []uint32{5, 4}, get(model.ListQuery{Limit: 2}))
	assert.Equal(t, []uint32{3, 2}, get(model.ListQuery{Limit: 2, Offset: 2}))
	assert.Equal(t, []uint32{1}, get(model.ListQuery{Limit: 2, Offset: 4}))
	assert.Equal(t, []uint32{2, 1}, get(model.ListQuery{Offset: 3}))
	assert.Empty(t, get(model.ListQuery{Limit: 2, Offset: 10}))
}

func TestDeleteUIDsNotIn(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	now := time.Now()
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{hdr(1, now, false), hdr(2, now, false), hdr(3, now, false)}))
	require.NoError(t, s.DeleteUIDsNotIn(ctx, fid, []uint32{1, 3}))
	assert.Equal(t, []uint32{1, 3}, uids(t, s, fid))
	require.NoError(t, s.DeleteUIDsNotIn(ctx, fid, nil))
	assert.Empty(t, uids(t, s, fid))
}

func TestResetFolder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, fid := seedFolder(t, s)
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{hdr(1, time.Now(), false)}))
	require.NoError(t, s.SetFolderSync(ctx, fid, FolderSync{UIDValidity: 5, UIDNext: 2}))
	require.NoError(t, s.ResetFolder(ctx, fid))
	assert.Empty(t, uids(t, s, fid))
	_, fs, err := s.GetFolder(ctx, fid)
	require.NoError(t, err)
	assert.Equal(t, FolderSync{}, fs)
}

func TestGetMessage_BodyCachedFlag(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, fid := seedFolder(t, s)
	row := hdr(10, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), false)
	row.References = []string{"<a@x>", "<b@x>"}
	row.InReplyTo = "<b@x>"
	row.To = []model.Address{{Name: "Yo", Email: "yo@ejemplo.com"}}
	row.Cc = []model.Address{{Email: "c@x.com"}}
	row.Size = 1234
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{row}))
	list, err := s.ListMessages(ctx, fid, model.ListQuery{})
	require.NoError(t, err)
	id := list[0].ID
	assert.Equal(t, accID, list[0].AccountID)

	d, cached, err := s.GetMessage(ctx, id)
	require.NoError(t, err)
	assert.False(t, cached)
	assert.Equal(t, []string{"<a@x>", "<b@x>"}, d.References)
	assert.Equal(t, row.To, d.To)
	assert.Equal(t, row.Cc, d.Cc)
	assert.NotNil(t, d.ReplyTo)
	assert.NotNil(t, d.Attachments)

	att := []model.Attachment{{Index: 0, Filename: "a.pdf", MIMEType: "application/pdf", Size: 10, Part: []int{2, 1}}}
	require.NoError(t, s.SetBody(ctx, id, BodyRow{HTML: "<p>hola</p>", Text: "hola", Snippet: "hola", HasRemoteImages: true, Attachments: att}))
	d, cached, err = s.GetMessage(ctx, id)
	require.NoError(t, err)
	assert.True(t, cached)
	assert.Equal(t, "<p>hola</p>", d.HTML)
	assert.Equal(t, "hola", d.Text)
	assert.Equal(t, "hola", d.Snippet)
	assert.True(t, d.HasRemoteImages)
	assert.True(t, d.HasAttachments)
	assert.Equal(t, att, d.Attachments)
	assert.Equal(t, []string{"<a@x>", "<b@x>"}, d.References)
	assert.Equal(t, row.To, d.To)

	// un upsert posterior de cabeceras no pierde el cuerpo
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{row}))
	_, cached, err = s.GetMessage(ctx, id)
	require.NoError(t, err)
	assert.True(t, cached)

	_, _, err = s.GetMessage(ctx, 9999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestDeleteAccount_Cascades(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, fid := seedFolder(t, s)
	require.NoError(t, s.UpsertHeaders(ctx, fid, []HeaderRow{hdr(1, time.Now(), false)}))
	_, err := s.SaveLocalDraft(ctx, model.SendRequest{AccountID: accID, Subject: "x"}, "boom")
	require.NoError(t, err)
	require.NoError(t, s.DeleteAccount(ctx, accID))
	for _, tbl := range []string{"folders", "messages", "local_drafts"} {
		var n int
		require.NoError(t, s.db.QueryRow("SELECT COUNT(*) FROM "+tbl).Scan(&n))
		assert.Zero(t, n, tbl)
	}
}

func TestLocalDrafts_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, err := s.InsertAccount(ctx, hostinger())
	require.NoError(t, err)
	req := model.SendRequest{
		AccountID: accID, To: []model.Address{{Name: "B", Email: "b@x.com"}}, Subject: "Re: hola", BodyHTML: "<p>ok</p>",
		Reply: &model.ReplyContext{Mode: "reply", OriginalMessageID: 42},
	}
	id, err := s.SaveLocalDraft(ctx, req, "smtp caído")
	require.NoError(t, err)
	list, err := s.ListLocalDrafts(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, id, list[0].ID)
	assert.Equal(t, "smtp caído", list[0].Error)
	assert.Equal(t, req.Subject, list[0].Request.Subject)
	require.NotNil(t, list[0].Request.Reply)
	assert.Equal(t, int64(42), list[0].Request.Reply.OriginalMessageID)
	assert.WithinDuration(t, time.Now(), list[0].CreatedAt, time.Minute)
	require.NoError(t, s.DeleteLocalDraft(ctx, id))
	list, err = s.ListLocalDrafts(ctx)
	require.NoError(t, err)
	assert.Empty(t, list)
}

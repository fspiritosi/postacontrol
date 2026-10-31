package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

var t0 = time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)

// put inserta un mensaje con asunto y remitente, y devuelve su id.
func put(t *testing.T, s *Store, folderID int64, uid uint32, subject, fromName string, date time.Time) int64 {
	t.Helper()
	h := hdr(uid, date, false)
	h.Subject, h.From = subject, model.Address{Name: fromName, Email: "x@ejemplo.com"}
	require.NoError(t, s.UpsertHeaders(context.Background(), folderID, []HeaderRow{h}))
	ms, err := s.ListMessages(context.Background(), folderID, model.ListQuery{})
	require.NoError(t, err)
	for _, m := range ms {
		if m.UID == uid {
			return m.ID
		}
	}
	t.Fatal("mensaje no insertado")
	return 0
}

func search(t *testing.T, s *Store, text string, folder int64) []model.MessageSummary {
	t.Helper()
	ms, err := s.Search(context.Background(), model.SearchQuery{Text: text, FolderID: folder})
	require.NoError(t, err)
	return ms
}

func subjects(ms []model.MessageSummary) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.Subject)
	}
	return out
}

func TestSearch_FindsSubjectSenderAndBody(t *testing.T) {
	s := newStore(t)
	_, f := seedFolder(t, s)
	put(t, s, f, 1, "Presupuesto anual", "Juan", t0)
	put(t, s, f, 2, "Hola", "Mariana", t0.Add(time.Hour))
	id3 := put(t, s, f, 3, "Otro", "Pedro", t0.Add(2*time.Hour))
	require.NoError(t, s.SetBody(context.Background(), id3, BodyRow{Text: "adjunto la cotización pedida"}))

	assert.Equal(t, []string{"Presupuesto anual"}, subjects(search(t, s, "presupuesto", 0)))
	assert.Equal(t, []string{"Hola"}, subjects(search(t, s, "mariana", 0)))
	assert.Equal(t, []string{"Otro"}, subjects(search(t, s, "cotizacion", 0)))
}

func TestSearch_IgnoresAccentsAndCase(t *testing.T) {
	s := newStore(t)
	_, f := seedFolder(t, s)
	put(t, s, f, 1, "Información", "x", t0)
	put(t, s, f, 2, "Hola", "peña", t0)
	assert.Len(t, search(t, s, "informacion", 0), 1)
	assert.Len(t, search(t, s, "PEÑA", 0), 1)
}

func TestSearch_PrefixAndAllWords(t *testing.T) {
	s := newStore(t)
	_, f := seedFolder(t, s)
	put(t, s, f, 1, "presupuesto marzo", "a", t0)
	put(t, s, f, 2, "presupuesto abril", "a", t0)
	assert.Len(t, search(t, s, "presu", 0), 2)
	assert.Equal(t, []string{"presupuesto marzo"}, subjects(search(t, s, "presupuesto marzo", 0)))
}

func TestSearch_SpecialCharactersAreLiteral(t *testing.T) {
	s := newStore(t)
	_, f := seedFolder(t, s)
	put(t, s, f, 1, "hola mundo", "a", t0)
	for _, q := range []string{`"`, `*`, `-`, `:`, `(`, `NEAR`, `AND`, `OR`, `a"b`, `"hola`, `hola*`, `subject:hola`, `NOT`, `^`} {
		_, err := s.Search(context.Background(), model.SearchQuery{Text: q})
		assert.NoError(t, err, q)
	}
	assert.Empty(t, search(t, s, "", 0))
	assert.Empty(t, search(t, s, "   ", 0))
	assert.NotNil(t, search(t, s, "", 0))
	assert.Equal(t, `"a""b"* AND "c"*`, ftsQuery(`a"b c`))
	assert.Equal(t, "", ftsQuery("  "))
}

func TestSearch_ScopeAndOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, f1 := seedFolder(t, s)
	a2 := hostinger()
	a2.Email, a2.Username = "otra@ejemplo.com", "otra@ejemplo.com"
	acc2, err := s.InsertAccount(ctx, a2)
	require.NoError(t, err)
	require.NoError(t, s.ReplaceFolders(ctx, acc2, []model.Folder{{Name: "INBOX", Delimiter: ".", Role: "inbox", Selectable: true}}))
	fs, err := s.ListFolders(ctx, acc2)
	require.NoError(t, err)
	f2 := fs[0].ID

	put(t, s, f1, 1, "informe uno", "a", t0)
	put(t, s, f1, 2, "informe dos", "a", t0.Add(2*time.Hour))
	put(t, s, f2, 1, "informe tres", "a", t0.Add(time.Hour))

	assert.Equal(t, []string{"informe dos", "informe uno"}, subjects(search(t, s, "informe", f1)))
	assert.Equal(t, []string{"informe dos", "informe tres", "informe uno"}, subjects(search(t, s, "informe", 0)))
	ms, err := s.Search(ctx, model.SearchQuery{Text: "informe", Limit: 1, Offset: 1})
	require.NoError(t, err)
	assert.Equal(t, []string{"informe tres"}, subjects(ms))
}

func TestSearch_IndexFollowsDeletes(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	acc, f := seedFolder(t, s)
	id1 := put(t, s, f, 1, "borrar uno", "a", t0)
	put(t, s, f, 2, "borrar dos", "a", t0)
	assert.Len(t, search(t, s, "borrar", 0), 2)

	require.NoError(t, s.DeleteMessages(ctx, []int64{id1}))
	assert.Len(t, search(t, s, "borrar", 0), 1)

	require.NoError(t, s.ResetFolder(ctx, f))
	assert.Empty(t, search(t, s, "borrar", 0))

	put(t, s, f, 3, "borrar tres", "a", t0)
	assert.Len(t, search(t, s, "borrar", 0), 1)
	require.NoError(t, s.DeleteAccount(ctx, acc))
	assert.Empty(t, search(t, s, "borrar", 0))
	var n int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM messages_fts`).Scan(&n))
	assert.Zero(t, n)
}

func TestPendingBodies(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, f := seedFolder(t, s)
	put(t, s, f, 1, "a", "a", t0)
	put(t, s, f, 2, "b", "a", t0)
	put(t, s, f, 3, "c", "a", t0)
	id4 := put(t, s, f, 4, "d", "a", t0)
	id5 := put(t, s, f, 5, "e", "a", t0)
	require.NoError(t, s.SetBody(ctx, id4, BodyRow{Text: "x"}))
	_, err := s.db.Exec(`UPDATE messages SET index_failures = 3 WHERE id = ?`, id5)
	require.NoError(t, err)
	n, err := s.PendingBodies(ctx, f)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	n, err = s.PendingBodies(ctx, 0)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestMigrateV4_BackfillsExistingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v3.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	require.NoError(t, migrateTo(db, 3))
	_, err = db.Exec(`INSERT INTO accounts (email, display_name, username, color, imap_host, imap_port, imap_security, smtp_host, smtp_port, smtp_security, save_sent_copy)
 VALUES ('a@x.com','A','a@x.com','#fff','h',993,'tls','h',465,'tls',1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO folders (account_id, name, delimiter, role, selectable) VALUES (1,'INBOX','.','inbox',1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO messages (folder_id, uid, subject, from_name, from_email, date, body_text) VALUES (1,1,'Viejo presupuesto','Mariana','m@x.com',1700000000,'texto cotización')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	assert.Len(t, search(t, s, "presupuesto", 0), 1)
	assert.Len(t, search(t, s, "mariana", 0), 1)
	assert.Len(t, search(t, s, "cotizacion", 0), 1)
}

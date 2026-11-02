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

func TestContacts_CRUDKeepsEmailOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ana, err := s.SaveContact(ctx, model.Contact{Name: "Ana", Emails: []string{"b@x.com", "a@x.com"}, Phones: []string{"+54 11 1"}})
	require.NoError(t, err)
	require.NotZero(t, ana.ID)

	got, err := s.GetContact(ctx, ana.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"b@x.com", "a@x.com"}, got.Emails)
	assert.Equal(t, []string{"+54 11 1"}, got.Phones)

	ana.Emails = []string{"a@x.com"}
	_, err = s.SaveContact(ctx, ana)
	require.NoError(t, err)
	got, err = s.GetContact(ctx, ana.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"a@x.com"}, got.Emails)

	all, err := s.ListContacts(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, []string{"a@x.com"}, all[0].Emails)

	_, err = s.SaveContact(ctx, model.Contact{ID: 9999, Name: "Nadie"})
	assert.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.DeleteContact(ctx, ana.ID))
	_, err = s.GetContact(ctx, ana.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.ContactIDForEmail(ctx, "a@x.com")
	assert.ErrorIs(t, err, ErrNotFound, "los correos se van con la ficha")
	assert.ErrorIs(t, s.DeleteContact(ctx, ana.ID), ErrNotFound)
}

func TestContacts_NeverNilSlices(t *testing.T) {
	s := newStore(t)
	c, err := s.SaveContact(context.Background(), model.Contact{Name: "Sin nada"})
	require.NoError(t, err)
	assert.NotNil(t, c.Emails)
	assert.NotNil(t, c.Phones)
	got, err := s.GetContact(context.Background(), c.ID)
	require.NoError(t, err)
	assert.NotNil(t, got.Emails)
	assert.NotNil(t, got.Phones)
}

func TestContacts_EmailTakenIgnoringCase(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ana, err := s.SaveContact(ctx, model.Contact{Name: "Ana", Emails: []string{"juan@x.com"}})
	require.NoError(t, err)

	_, err = s.SaveContact(ctx, model.Contact{Name: "Otra", Emails: []string{"Juan@X.com"}})
	assert.Equal(t, ErrEmailTaken{Email: "Juan@X.com", ContactID: ana.ID}, err)
	all, err := s.ListContacts(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1, "la ficha rechazada no se crea")

	// Re-guardar la propia ficha con sus mismos correos no es un choque.
	_, err = s.SaveContact(ctx, ana)
	assert.NoError(t, err)
}

func TestContacts_ContactIDForEmailIgnoresCase(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ana, err := s.SaveContact(ctx, model.Contact{Name: "Ana", Emails: []string{"Ana@X.com"}})
	require.NoError(t, err)
	id, err := s.ContactIDForEmail(ctx, "ana@x.COM")
	require.NoError(t, err)
	assert.Equal(t, ana.ID, id)
	_, err = s.ContactIDForEmail(ctx, "otro@x.com")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestRecipients_RecordAccumulates(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	t1 := time.Unix(1000, 0)
	t2 := time.Unix(2000, 0)
	require.NoError(t, s.RecordRecipients(ctx, []model.Address{
		{Name: "Ana", Email: "ANA@x.com"}, {Email: "ana@x.com"}, {Email: "sin-arroba"},
	}, t2))
	require.NoError(t, s.RecordRecipients(ctx, []model.Address{{Email: "ana@x.com"}}, t1))

	rs, err := s.ListRecipients(ctx)
	require.NoError(t, err)
	require.Len(t, rs, 1)
	assert.Equal(t, Recipient{Email: "ana@x.com", Name: "Ana", Times: 2, LastUsed: 2000}, rs[0])
}

func TestRecipients_SentTriggerCountsOncePerMessage(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, inbox := seedFolder(t, s)
	require.NoError(t, s.ReplaceFolders(ctx, accID, []model.Folder{
		{Name: "INBOX", Delimiter: ".", Role: "inbox", Selectable: true},
		{Name: "Sent", Delimiter: ".", Role: "sent", Selectable: true},
	}))
	fs, err := s.ListFolders(ctx, accID)
	require.NoError(t, err)
	var sent int64
	for _, f := range fs {
		if f.Role == "sent" {
			sent = f.ID
		}
	}
	require.NotZero(t, sent)

	h := hdr(1, time.Unix(5000, 0), true)
	h.To = []model.Address{{Name: "Bea", Email: "Bea@y.com"}}
	h.Cc = []model.Address{{Email: "cc@y.com"}, {Email: "bea@y.com"}}
	require.NoError(t, s.UpsertHeaders(ctx, sent, []HeaderRow{h}))
	require.NoError(t, s.UpsertHeaders(ctx, sent, []HeaderRow{h}))
	other := hdr(1, time.Unix(6000, 0), true)
	other.To = []model.Address{{Email: "inbox@y.com"}}
	require.NoError(t, s.UpsertHeaders(ctx, inbox, []HeaderRow{other}))

	rs, err := s.ListRecipients(ctx)
	require.NoError(t, err)
	require.Len(t, rs, 2)
	assert.Equal(t, Recipient{Email: "bea@y.com", Name: "Bea", Times: 1, LastUsed: 5000}, rs[0])
	assert.Equal(t, Recipient{Email: "cc@y.com", Times: 1, LastUsed: 5000}, rs[1])
}

func TestMigrateV7_BackfillsSentRecipients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v6.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	require.NoError(t, migrateTo(db, 6))
	_, err = db.Exec(`INSERT INTO accounts (email, display_name, username, color, imap_host, imap_port, imap_security, smtp_host, smtp_port, smtp_security, save_sent_copy)
 VALUES ('a@x.com','A','a@x.com','#fff','h',993,'tls','h',465,'tls',1)`)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO folders (account_id, name, delimiter, role, selectable) VALUES (1,'Sent','.','sent',1), (1,'INBOX','.','inbox',1)`)
	require.NoError(t, err)
	ins := `INSERT INTO messages (folder_id, uid, to_json, cc_json, date) VALUES (?,?,?,?,?)`
	_, err = db.Exec(ins, 1, 1, `[{"name":"Bea","email":"Bea@y.com"}]`, `[{"name":"","email":"bea@y.com"},{"name":"","email":"cc@y.com"}]`, 100)
	require.NoError(t, err)
	_, err = db.Exec(ins, 1, 2, `[{"name":"","email":"bea@y.com"}]`, `[]`, 300)
	require.NoError(t, err)
	_, err = db.Exec(ins, 2, 1, `[{"name":"X","email":"inbox@y.com"}]`, `[]`, 400)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	rs, err := s.ListRecipients(context.Background())
	require.NoError(t, err)
	require.Len(t, rs, 2)
	assert.Equal(t, Recipient{Email: "bea@y.com", Name: "Bea", Times: 2, LastUsed: 300}, rs[0])
	assert.Equal(t, Recipient{Email: "cc@y.com", Times: 1, LastUsed: 100}, rs[1])
}

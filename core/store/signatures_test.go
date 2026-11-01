package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestSignatures_CRUDAndOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	b, err := s.SaveSignature(ctx, model.Signature{Name: "Beta", HTML: "<p>b</p>"})
	require.NoError(t, err)
	a, err := s.SaveSignature(ctx, model.Signature{Name: "Alfa", HTML: "<p>a</p>"})
	require.NoError(t, err)
	assert.NotZero(t, a.ID)
	assert.NotEqual(t, a.ID, b.ID)

	list, err := s.ListSignatures(ctx)
	require.NoError(t, err)
	assert.Equal(t, []model.Signature{a, b}, list)

	b.Name, b.HTML = "Aaa", "<p>nueva</p>"
	got, err := s.SaveSignature(ctx, b)
	require.NoError(t, err)
	assert.Equal(t, b, got)
	list, err = s.ListSignatures(ctx)
	require.NoError(t, err)
	assert.Equal(t, []model.Signature{b, a}, list)

	one, err := s.GetSignature(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, a, one)

	require.NoError(t, s.DeleteSignature(ctx, a.ID))
	_, err = s.GetSignature(ctx, a.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSignatures_DeleteClearsAccountDefault(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	sig, err := s.SaveSignature(ctx, model.Signature{Name: "X", HTML: "x"})
	require.NoError(t, err)
	require.NoError(t, s.SetAccountSignature(ctx, accID, sig.ID))
	acc, err := s.GetAccount(ctx, accID)
	require.NoError(t, err)
	assert.Equal(t, sig.ID, acc.DefaultSignatureID)

	require.NoError(t, s.DeleteSignature(ctx, sig.ID))
	acc, err = s.GetAccount(ctx, accID)
	require.NoError(t, err)
	assert.Zero(t, acc.DefaultSignatureID)
}

func TestSetAccountSignature_ZeroClears(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	sig, err := s.SaveSignature(ctx, model.Signature{Name: "X", HTML: "x"})
	require.NoError(t, err)
	require.NoError(t, s.SetAccountSignature(ctx, accID, sig.ID))
	require.NoError(t, s.SetAccountSignature(ctx, accID, 0))
	accs, err := s.ListAccounts(ctx)
	require.NoError(t, err)
	require.Len(t, accs, 1)
	assert.Zero(t, accs[0].DefaultSignatureID)
	assert.ErrorIs(t, s.SetAccountSignature(ctx, 9999, 0), ErrNotFound)
}

func TestMigrateV6_FromV5(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v5.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	require.NoError(t, migrateTo(db, 5))
	_, err = db.Exec(`INSERT INTO accounts (email, display_name, username, color, imap_host, imap_port, imap_security, smtp_host, smtp_port, smtp_security, save_sent_copy)
 VALUES ('a@x.com','A','a@x.com','#fff','h',993,'tls','h',465,'tls',1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	acc, err := s.GetAccount(context.Background(), 1)
	require.NoError(t, err)
	assert.Zero(t, acc.DefaultSignatureID)
	assert.Equal(t, "a@x.com", acc.Email)
}

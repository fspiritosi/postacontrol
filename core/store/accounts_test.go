package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestUpdateAccount_AllColumns(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	id, err := s.InsertAccount(ctx, hostinger())
	require.NoError(t, err)
	sig, err := s.SaveSignature(ctx, model.Signature{Name: "Trabajo", HTML: "<p>Fabri</p>"})
	require.NoError(t, err)

	want := model.Account{
		ID: id, Email: "otro@ejemplo.com", DisplayName: "Otro", Username: "usuario", Color: "#dc2626",
		IMAP:         model.ServerSettings{Host: "imap.ejemplo.com", Port: 143, Security: model.SecuritySTARTTLS},
		SMTP:         model.ServerSettings{Host: "smtp.ejemplo.com", Port: 587, Security: model.SecuritySTARTTLS},
		SaveSentCopy: false, DefaultSignatureID: sig.ID,
	}
	require.NoError(t, s.UpdateAccount(ctx, want))
	got, err := s.GetAccount(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// Sin firma se guarda NULL (la clave foránea no admite 0).
	want.DefaultSignatureID = 0
	require.NoError(t, s.UpdateAccount(ctx, want))
	got, err = s.GetAccount(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got.DefaultSignatureID)

	want.ID = id + 99
	assert.ErrorIs(t, s.UpdateAccount(ctx, want), ErrNotFound)
}

func TestResetAccountCache(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	acc, f1, _ := labelAccount(t, s, "a@ej.com")
	other, g1, _ := labelAccount(t, s, "b@ej.com")
	l := mkLabel(t, s, "Trabajo", "$pc_trabajo")
	now := time.Now()
	for _, f := range []int64{f1, g1} {
		require.NoError(t, s.UpsertHeaders(ctx, f, []HeaderRow{hdr(1, now, false)}))
		ms, err := s.ListMessages(ctx, f, model.ListQuery{})
		require.NoError(t, err)
		require.NoError(t, s.SetMessageLabels(ctx, ms[0].ID, []int64{l.ID}))
	}
	for _, a := range []int64{acc, other} {
		_, err := s.SetAccountLabelsLocal(ctx, a)
		require.NoError(t, err)
		require.NoError(t, s.SaveLabelCarry(ctx, a, "<m@x>", []int64{l.ID}))
	}
	l2 := mkLabel(t, s, "Viejo", "$pc_viejo")
	_, err := s.DeleteLabel(ctx, l2.ID, []int64{acc, other})
	require.NoError(t, err)
	_, err = s.QueueLocalDraft(ctx, 0, model.SendRequest{AccountID: acc, Subject: "pendiente"})
	require.NoError(t, err)

	require.NoError(t, s.ResetAccountCache(ctx, acc))

	fs, err := s.ListFolders(ctx, acc)
	require.NoError(t, err)
	assert.Empty(t, fs)
	local, err := s.AccountLabelsLocal(ctx, acc)
	require.NoError(t, err)
	assert.False(t, local)
	count := func(q string, args ...any) int {
		var n int
		require.NoError(t, s.db.QueryRowContext(ctx, q, args...).Scan(&n))
		return n
	}
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM label_cleanup WHERE account_id = ?`, acc))
	assert.Equal(t, 0, count(`SELECT COUNT(*) FROM label_carry WHERE account_id = ?`, acc))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM messages`), "solo queda el de la otra cuenta")
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM message_labels`))

	// Lo global y lo de la otra cuenta no se toca; los borradores locales quedan.
	ls, err := s.ListLabels(ctx)
	require.NoError(t, err)
	assert.Len(t, ls, 1)
	fs, err = s.ListFolders(ctx, other)
	require.NoError(t, err)
	assert.Len(t, fs, 2)
	local, err = s.AccountLabelsLocal(ctx, other)
	require.NoError(t, err)
	assert.True(t, local)
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM label_cleanup WHERE account_id = ?`, other))
	assert.Equal(t, 1, count(`SELECT COUNT(*) FROM label_carry WHERE account_id = ?`, other))
	drafts, err := s.ListLocalDrafts(ctx)
	require.NoError(t, err)
	assert.Len(t, drafts, 1)

	assert.ErrorIs(t, s.ResetAccountCache(ctx, other+99), ErrNotFound)
}

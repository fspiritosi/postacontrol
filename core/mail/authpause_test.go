package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

func syncErrorsOf(n *recNotifier, accountID int64) int {
	cnt := 0
	for _, e := range n.matching(EventSyncError) {
		if id, _ := e["accountId"].(int64); id == accountID {
			cnt++
		}
	}
	return cnt
}

// twoAccounts arma dos cuentas (A y B) contra servidores distintos.
func twoAccounts(t *testing.T) (c *Core, n *recNotifier, accA, accB int64) {
	t.Helper()
	tlsEnv := testenv.NewTLS(t)
	imapA := testenv.StartIMAP(t, tlsEnv, testUser, testPass)
	smtpA := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	imapB := testenv.StartIMAP(t, tlsEnv, "g@ej.com", testPass)
	smtpB := testenv.StartSMTP(t, tlsEnv, "g@ej.com", testPass)
	n = &recNotifier{}
	c, err := New(Config{DBPath: t.TempDir() + "/t.db", Secrets: secrets.NewMemory(), Notifier: n,
		TLS: tlsEnv.Client, SkipInitialSync: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()
	a, err := c.AddAccount(ctx, inputFor(imapA, smtpA, testPass))
	require.NoError(t, err)
	inB := inputFor(imapB, smtpB, testPass)
	inB.Account.Email = "g@ej.com"
	b, err := c.AddAccount(ctx, inB)
	require.NoError(t, err)
	return c, n, a.ID, b.ID
}

func TestBackgroundSync_SkipsAccountInAuthPause(t *testing.T) {
	c, n, a, b := twoAccounts(t)
	ctx := context.Background()
	require.NoError(t, c.cfg.Secrets.Set(testUser, "mala"))
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }

	c.backgroundPass(ctx)
	assert.Equal(t, 1, syncErrorsOf(n, a))
	c.backgroundPass(ctx)
	assert.Equal(t, 1, syncErrorsOf(n, a), "A está en pausa")
	assert.Equal(t, 0, syncErrorsOf(n, b))
	_, err := c.ListFolders(ctx, b)
	require.NoError(t, err)

	now = now.Add(31 * time.Minute)
	c.backgroundPass(ctx)
	assert.Equal(t, 2, syncErrorsOf(n, a), "pasada la pausa vuelve a intentar")
}

func TestUpdatePassword_WrongKeepsOld(t *testing.T) {
	c, _, a, _ := twoAccounts(t)
	err := c.UpdatePassword(context.Background(), a, "mala")
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "auth", ue.Code)
	got, err := c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, testPass, got)

	err = c.UpdatePassword(context.Background(), a, "")
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
}

func TestUpdatePassword_ClearsPause(t *testing.T) {
	c, n, a, _ := twoAccounts(t)
	ctx := context.Background()
	require.NoError(t, c.cfg.Secrets.Set(testUser, "mala"))
	now := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	c.backgroundPass(ctx)
	require.Equal(t, 1, syncErrorsOf(n, a))

	require.NoError(t, c.UpdatePassword(ctx, a, testPass))
	got, err := c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, testPass, got)

	c.backgroundPass(ctx)
	assert.Equal(t, 1, syncErrorsOf(n, a))
	folders, err := c.ListFolders(ctx, a)
	require.NoError(t, err)
	assert.NotEmpty(t, folders, "A sincronizó")
}

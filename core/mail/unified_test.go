package mail

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

func TestListUnified_UnknownViewInvalid(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	_, err := c.ListUnified(context.Background(), "spam", model.ListQuery{})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
}

func TestSyncUnified_OneAccountDownOthersSync(t *testing.T) {
	tlsEnv := testenv.NewTLS(t)
	imapA := testenv.StartIMAP(t, tlsEnv, testUser, testPass)
	smtpA := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	imapB := testenv.StartIMAP(t, tlsEnv, "g@ej.com", testPass)
	smtpB := testenv.StartSMTP(t, tlsEnv, "g@ej.com", testPass)
	n := &recNotifier{}
	c, err := New(Config{DBPath: t.TempDir() + "/t.db", Secrets: secrets.NewMemory(), Notifier: n,
		TLS: tlsEnv.Client, SkipInitialSync: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	ctx := context.Background()

	imapA.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "a"))
	imapB.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "b"))
	accA, err := c.AddAccount(ctx, inputFor(imapA, smtpA, testPass))
	require.NoError(t, err)
	inB := inputFor(imapB, smtpB, testPass)
	inB.Account.Email = "g@ej.com"
	accB, err := c.AddAccount(ctx, inB)
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, accA.ID))
	require.NoError(t, c.SyncAccount(ctx, accB.ID))
	imapB.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "b2"))
	imapA.Close()

	require.NoError(t, c.SyncUnified(ctx, model.UnifiedInbox))
	assert.Len(t, listAll(t, c, folderID(t, c, accB.ID, "INBOX")), 2)
	var failed bool
	for _, e := range n.matching(EventSyncError) {
		if e["accountId"] == accA.ID {
			failed = true
		}
	}
	assert.True(t, failed, "debe avisar el error de la cuenta A")
}

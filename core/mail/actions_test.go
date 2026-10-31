package mail

import (
	"context"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

func TestSetSeen_MarksUnreadOnServerAndCache(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 2, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	ms := listAll(t, c, inboxID)
	require.Len(t, ms, 2)
	require.NoError(t, c.SetSeen(context.Background(), []int64{ms[0].ID, ms[1].ID}, true))
	assert.Equal(t, 0, folderByID(t, c, acc.ID, inboxID).Unread)

	before := len(n.matching(EventFolderUpdated))
	require.NoError(t, c.SetSeen(context.Background(), []int64{ms[0].ID}, false))
	assert.False(t, hasFlag(serverFlags(t, c, imap, ms[0].UID), goimap.FlagSeen))
	assert.True(t, hasFlag(serverFlags(t, c, imap, ms[1].UID), goimap.FlagSeen))
	for _, m := range listAll(t, c, inboxID) {
		assert.Equal(t, m.ID != ms[0].ID, m.Seen)
	}
	assert.Equal(t, 1, folderByID(t, c, acc.ID, inboxID).Unread)
	assert.Greater(t, len(n.matching(EventFolderUpdated)), before)
}

func TestSetFlagged_RoundTrip(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 2, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	ms := listAll(t, c, inboxID)
	require.NoError(t, c.SetFlagged(context.Background(), []int64{ms[0].ID, ms[1].ID}, true))
	for _, m := range ms {
		assert.True(t, hasFlag(serverFlags(t, c, imap, m.UID), goimap.FlagFlagged))
	}
	for _, m := range listAll(t, c, inboxID) {
		assert.True(t, m.Flagged)
	}
	require.NoError(t, c.SetFlagged(context.Background(), []int64{ms[0].ID}, false))
	assert.False(t, hasFlag(serverFlags(t, c, imap, ms[0].UID), goimap.FlagFlagged))
	assert.True(t, hasFlag(serverFlags(t, c, imap, ms[1].UID), goimap.FlagFlagged))
	for _, m := range listAll(t, c, inboxID) {
		assert.Equal(t, m.ID == ms[1].ID, m.Flagged)
	}
}

func TestSetSeen_MixedAccountsRefused(t *testing.T) {
	tlsEnv := testenv.NewTLS(t)
	imapA := testenv.StartIMAP(t, tlsEnv, testUser, testPass)
	smtpA := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	imapB := testenv.StartIMAP(t, tlsEnv, "g@ej.com", testPass)
	smtpB := testenv.StartSMTP(t, tlsEnv, "g@ej.com", testPass)
	c, err := New(Config{DBPath: t.TempDir() + "/t.db", Secrets: secrets.NewMemory(), Notifier: &recNotifier{},
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
	mA := listAll(t, c, folderID(t, c, accA.ID, "INBOX"))[0]
	mB := listAll(t, c, folderID(t, c, accB.ID, "INBOX"))[0]

	err = c.SetSeen(ctx, []int64{mA.ID, mB.ID}, true)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
	assert.Equal(t, "Elegí correos de una sola cuenta.", ue.Msg)
	assert.False(t, listAll(t, c, mA.FolderID)[0].Seen)
	assert.False(t, listAll(t, c, mB.FolderID)[0].Seen)
}

func TestSetFlagged_StaleFolderRefuses(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := listAll(t, c, inboxID)[0]
	imap.RecreateMailbox(t, "INBOX")
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "otro"))

	err := c.SetFlagged(context.Background(), []int64{m.ID}, true)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "stale", ue.Code)
	assert.False(t, hasFlag(serverFlags(t, c, imap, 1), goimap.FlagFlagged))
}

func TestSetSeen_EmptyIsNoop(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	require.NoError(t, c.SetSeen(context.Background(), nil, true))
}

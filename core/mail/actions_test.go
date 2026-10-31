package mail

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
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

// serverSubjects devuelve los asuntos de un buzón, consultando directo al
// servidor con una conexión aparte.
func serverSubjects(t *testing.T, c *Core, imap *testenv.IMAP, user, mailbox string) []string {
	t.Helper()
	conn, err := imapx.Dial(context.Background(), imap.Settings, user, testPass, c.cfg.TLS)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.Select(mailbox)
	require.NoError(t, err)
	uids, err := conn.AllUIDs()
	require.NoError(t, err)
	out := []string{}
	if len(uids) == 0 {
		return out
	}
	hs, err := conn.FetchHeaders(uids)
	require.NoError(t, err)
	for _, h := range hs {
		out = append(out, h.Envelope.Subject)
	}
	slices.Sort(out)
	return out
}

func bySubject(t *testing.T, ms []model.MessageSummary, subject string) model.MessageSummary {
	t.Helper()
	for _, m := range ms {
		if m.Subject == subject {
			return m
		}
	}
	t.Fatalf("no está el correo %q en %v", subject, subjects(ms))
	return model.MessageSummary{}
}

func sortedSubjects(ms []model.MessageSummary) []string {
	out := subjects(ms)
	slices.Sort(out)
	return out
}

func requireUserError(t *testing.T, err error, code string) *UserError {
	t.Helper()
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	require.Equal(t, code, ue.Code)
	return ue
}

func TestMoveMessages_MovesAndResyncsDestination(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 2, "m")
	require.NoError(t, imap.User.Create("Archivo", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	archivoID := folderID(t, c, acc.ID, "Archivo")
	m1 := bySubject(t, listAll(t, c, inboxID), "m-1")

	require.NoError(t, c.MoveMessages(context.Background(), []int64{m1.ID}, archivoID))
	assert.Equal(t, []string{"m-2"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "Archivo"))
	assert.Equal(t, []string{"m-2"}, subjects(listAll(t, c, inboxID)))
	assert.Equal(t, 1, folderByID(t, c, acc.ID, inboxID).Total)
	assert.Contains(t, n.matching(EventFolderUpdated), map[string]any{"folderId": inboxID})
	assert.Contains(t, n.matching(EventFolderUpdated), map[string]any{"folderId": archivoID})

	deadline := time.Now().Add(10 * time.Second)
	for len(listAll(t, c, archivoID)) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, []string{"m-1"}, subjects(listAll(t, c, archivoID)))
}

func TestMoveMessages_AlreadyInDestinationIsNoop(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := listAll(t, c, inboxID)[0]
	require.NoError(t, c.MoveMessages(context.Background(), []int64{m.ID}, inboxID))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Equal(t, []string{"m-1"}, subjects(listAll(t, c, inboxID)))
	assert.False(t, hasFlag(serverFlags(t, c, imap, m.UID), goimap.FlagDeleted))
}

func TestMoveMessages_EmptyIsNoop(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	require.NoError(t, c.MoveMessages(context.Background(), nil, 1))
	require.NoError(t, c.DeleteMessages(context.Background(), nil))
}

func TestMoveMessages_OtherAccountFolderRefused(t *testing.T) {
	tlsEnv := testenv.NewTLS(t)
	imapA := testenv.StartIMAP(t, tlsEnv, testUser, testPass)
	smtpA := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	const userB = "g@ej.com"
	imapB := testenv.StartIMAP(t, tlsEnv, userB, testPass)
	smtpB := testenv.StartSMTP(t, tlsEnv, userB, testPass)
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
	inB.Account.Email = userB
	accB, err := c.AddAccount(ctx, inB)
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, accA.ID))
	require.NoError(t, c.SyncAccount(ctx, accB.ID))
	inboxA := folderID(t, c, accA.ID, "INBOX")
	mA := listAll(t, c, inboxA)[0]

	err = c.MoveMessages(ctx, []int64{mA.ID}, folderID(t, c, accB.ID, "INBOX"))
	requireUserError(t, err, "invalid")
	assert.Equal(t, []string{"a"}, serverSubjects(t, c, imapA, testUser, "INBOX"))
	assert.Equal(t, []string{"b"}, serverSubjects(t, c, imapB, userB, "INBOX"))
	assert.Equal(t, []string{"a"}, subjects(listAll(t, c, inboxA)))
}

func TestMoveMessages_UnknownDestinationRefused(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := listAll(t, c, inboxID)[0]
	requireUserError(t, c.MoveMessages(context.Background(), []int64{m.ID}, 9999), "not_found")
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
}

func TestMoveMessages_StaleFolderRefuses(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	require.NoError(t, imap.User.Create("Archivo", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	archivoID := folderID(t, c, acc.ID, "Archivo")
	m := listAll(t, c, inboxID)[0]
	imap.RecreateMailbox(t, "INBOX")
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "otro"))

	requireUserError(t, c.MoveMessages(context.Background(), []int64{m.ID}, archivoID), "stale")
	assert.Equal(t, []string{"otro"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Archivo"))
	assert.Equal(t, []string{"m-1"}, subjects(listAll(t, c, inboxID)), "sin escribir en el servidor no se toca la caché")
}

func TestDeleteMessages_MovesToTrash(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 2, "m")
	require.NoError(t, imap.User.Create("Trash", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	trashID := folderID(t, c, acc.ID, "Trash")
	require.Equal(t, model.FolderRole("trash"), folderByID(t, c, acc.ID, trashID).Role)
	m1 := bySubject(t, listAll(t, c, inboxID), "m-1")

	require.NoError(t, c.DeleteMessages(context.Background(), []int64{m1.ID}))
	assert.Equal(t, []string{"m-2"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "Trash"))
	assert.Equal(t, []string{"m-2"}, subjects(listAll(t, c, inboxID)))
	assert.Contains(t, n.matching(EventFolderUpdated), map[string]any{"folderId": trashID})
}

func TestDeleteMessages_InTrashIsPermanent(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 1, "m")
	require.NoError(t, imap.User.Create("Trash", nil))
	imap.Deliver(t, "Trash", rawMail("Bea <bea@ej.com>", "t-1"))
	imap.Deliver(t, "Trash", rawMail("Bea <bea@ej.com>", "t-2"))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	trashID := folderID(t, c, acc.ID, "Trash")
	require.NoError(t, c.SyncFolder(context.Background(), trashID))
	ms := listAll(t, c, trashID)
	require.Len(t, ms, 2)

	require.NoError(t, c.DeleteMessages(context.Background(), []int64{ms[0].ID, ms[1].ID}))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Trash"))
	assert.Empty(t, listAll(t, c, trashID))
	assert.Equal(t, 0, folderByID(t, c, acc.ID, trashID).Total)
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Sent"))
	assert.Equal(t, []string{"m-1"}, subjects(listAll(t, c, inboxID)))
	assert.Contains(t, n.matching(EventFolderUpdated), map[string]any{"folderId": trashID})
}

func TestDeleteMessages_MixedTrashAndInbox(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 2, "m")
	require.NoError(t, imap.User.Create("Trash", nil))
	imap.Deliver(t, "Trash", rawMail("Bea <bea@ej.com>", "t-1"))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	trashID := folderID(t, c, acc.ID, "Trash")
	require.NoError(t, c.SyncFolder(context.Background(), trashID))
	t1 := listAll(t, c, trashID)[0]
	m1 := bySubject(t, listAll(t, c, inboxID), "m-1")

	// t-1 se borra para siempre; m-1 va a la Papelera (y no se borra con t-1).
	require.NoError(t, c.DeleteMessages(context.Background(), []int64{t1.ID, m1.ID}))
	assert.Equal(t, []string{"m-2"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "Trash"))
}

func TestDeleteMessages_NoTrashErrors(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := listAll(t, c, inboxID)[0]

	ue := requireUserError(t, c.DeleteMessages(context.Background(), []int64{m.ID}), "no_trash")
	assert.Equal(t, "Esta cuenta no tiene carpeta Papelera.", ue.Msg)
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.False(t, hasFlag(serverFlags(t, c, imap, m.UID), goimap.FlagDeleted))
	assert.Equal(t, []string{"m-1"}, subjects(listAll(t, c, inboxID)))
}

func TestMoveMessages_UnsupportedServer(t *testing.T) {
	tlsEnv := testenv.NewTLS(t)
	imap := testenv.StartIMAPRev1(t, tlsEnv, testUser, testPass)
	smtp := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	c, err := New(Config{DBPath: t.TempDir() + "/t.db", Secrets: secrets.NewMemory(), Notifier: &recNotifier{},
		TLS: tlsEnv.Client, SkipInitialSync: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	deliverN(t, imap, 1, "m")
	require.NoError(t, imap.User.Create("Trash", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	m := listAll(t, c, inboxID)[0]

	ue := requireUserError(t, c.MoveMessages(context.Background(), []int64{m.ID}, folderID(t, c, acc.ID, "Trash")), "unsupported")
	assert.Equal(t, "El servidor no permite mover ni borrar correos de forma segura.", ue.Msg)
	requireUserError(t, c.DeleteMessages(context.Background(), []int64{m.ID}), "unsupported")
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Trash"))
	assert.False(t, hasFlag(serverFlags(t, c, imap, m.UID), goimap.FlagDeleted))
	assert.Equal(t, []string{"m-1"}, subjects(listAll(t, c, inboxID)))
}

func TestDeleteMessages_UsesTopLevelTrashNotNestedOne(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	// "Clientes/Papelera" va antes que "Trash" por nombre: sin una sola
	// Papelera por cuenta, borrar iría a parar a la subcarpeta.
	for _, name := range []string{"Clientes", "Clientes/Papelera", "Trash"} {
		require.NoError(t, imap.User.Create(name, nil))
	}
	acc, inboxID := setupSynced(t, imap, smtp, c)
	assert.Equal(t, model.FolderRole(""), folderByID(t, c, acc.ID, folderID(t, c, acc.ID, "Clientes/Papelera")).Role)
	m := listAll(t, c, inboxID)[0]

	require.NoError(t, c.DeleteMessages(context.Background(), []int64{m.ID}))
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "Trash"))
	assert.Empty(t, serverSubjects(t, c, imap, testUser, "Clientes/Papelera"))
}

func TestMoveMessages_ServerRefusalIsSpanishUserError(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	require.NoError(t, imap.User.Create("Archivo", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	archivoID := folderID(t, c, acc.ID, "Archivo")
	m := listAll(t, c, inboxID)[0]
	imap.DeleteMailbox(t, "Archivo") // el MOVE recibe un NO del servidor

	ue := requireUserError(t, c.MoveMessages(context.Background(), []int64{m.ID}, archivoID), "server")
	assert.True(t, strings.HasPrefix(ue.Msg, "El servidor rechazó la operación: "), ue.Msg)
	assert.NotContains(t, ue.Msg, "imapx:")
	assert.NotContains(t, ue.Msg, "imap: NO")
	assert.Equal(t, []string{"m-1"}, serverSubjects(t, c, imap, testUser, "INBOX"))
}

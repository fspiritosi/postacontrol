package mail

import (
	"context"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

// folderEnv es un núcleo con una cuenta ya sincronizada y la config TLS para
// abrir una conexión aparte al servidor de pruebas.
type folderEnv struct {
	c    *Core
	imap *testenv.IMAP
	smtp *testenv.SMTP
	n    *recNotifier
	acc  model.Account
	tls  testenv.TLS
}

func newFolderEnv(t *testing.T, mailboxes ...string) *folderEnv {
	t.Helper()
	tlsEnv := testenv.NewTLS(t)
	imap := testenv.StartIMAP(t, tlsEnv, testUser, testPass)
	smtp := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	n := &recNotifier{}
	c, err := New(Config{
		DBPath: t.TempDir() + "/test.db", Secrets: secrets.NewMemory(), Notifier: n,
		TLS: tlsEnv.Client, SkipInitialSync: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	for _, m := range mailboxes {
		require.NoError(t, imap.User.Create(m, nil))
	}
	acc, _ := setupSynced(t, imap, smtp, c)
	return &folderEnv{c: c, imap: imap, smtp: smtp, n: n, acc: acc, tls: tlsEnv}
}

func (e *folderEnv) folders(t *testing.T) []model.Folder {
	t.Helper()
	fs, err := e.c.ListFolders(context.Background(), e.acc.ID)
	require.NoError(t, err)
	return fs
}

func (e *folderEnv) find(t *testing.T, name string) (model.Folder, bool) {
	t.Helper()
	for _, f := range e.folders(t) {
		if f.Name == name {
			return f, true
		}
	}
	return model.Folder{}, false
}

func (e *folderEnv) id(t *testing.T, name string) int64 {
	t.Helper()
	return folderID(t, e.c, e.acc.ID, name)
}

// serverList devuelve los nombres que el servidor lista (subscribed = solo los
// suscriptos, como LSUB).
func (e *folderEnv) serverList(t *testing.T, subscribed bool) []string {
	t.Helper()
	cl, err := imapclient.DialTLS(e.imap.Addr, &imapclient.Options{TLSConfig: e.tls.Client})
	require.NoError(t, err)
	defer func() { _ = cl.Close() }()
	require.NoError(t, cl.Login(testUser, testPass).Wait())
	items, err := cl.List("", "*", &goimap.ListOptions{SelectSubscribed: subscribed}).Collect()
	require.NoError(t, err)
	out := []string{}
	for _, it := range items {
		out = append(out, it.Mailbox)
	}
	return out
}

func wantUserError(t *testing.T, err error, code, msg string) {
	t.Helper()
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, code, ue.Code)
	assert.Equal(t, msg, ue.Msg)
}

func TestCreateFolder_TopLevel(t *testing.T) {
	e := newFolderEnv(t)
	ctx := context.Background()
	f, err := e.c.CreateFolder(ctx, e.acc.ID, 0, "  Clientes ")
	require.NoError(t, err)
	assert.Equal(t, "Clientes", f.Name)
	got, ok := e.find(t, "Clientes")
	require.True(t, ok)
	assert.Equal(t, f.ID, got.ID)
	next, err := e.c.store.NextFolderPosition(ctx, e.acc.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, next) // nunca se reordenó: la nueva no recibe posición
	assert.Equal(t, 0, got.Position)
	assert.Contains(t, e.serverList(t, true), "Clientes")
	assert.NotEmpty(t, e.n.matching(EventFoldersUpdated))
}

func TestCreateFolder_PositionWhenAccountWasOrdered(t *testing.T) {
	e := newFolderEnv(t)
	ctx := context.Background()
	require.NoError(t, e.c.SetFolderOrder(ctx, e.acc.ID, []int64{e.id(t, "Sent"), e.id(t, "INBOX")}))
	f, err := e.c.CreateFolder(ctx, e.acc.ID, 0, "Nueva")
	require.NoError(t, err)
	assert.Equal(t, 3, f.Position)
	got, _ := e.find(t, "Nueva")
	assert.Equal(t, 3, got.Position)
}

func TestCreateFolder_Subfolder(t *testing.T) {
	e := newFolderEnv(t, "Clientes")
	require.NoError(t, e.c.SyncAccount(context.Background(), e.acc.ID))
	f, err := e.c.CreateFolder(context.Background(), e.acc.ID, e.id(t, "Clientes"), "Acme")
	require.NoError(t, err)
	assert.Equal(t, "Clientes/Acme", f.Name)
	assert.Contains(t, e.serverList(t, true), "Clientes/Acme")
}

func TestCreateFolder_Accents(t *testing.T) {
	e := newFolderEnv(t)
	f, err := e.c.CreateFolder(context.Background(), e.acc.ID, 0, "Facturación 2026")
	require.NoError(t, err)
	assert.Equal(t, "Facturación 2026", f.Name)
	got, ok := e.find(t, "Facturación 2026")
	require.True(t, ok)
	assert.Equal(t, f.ID, got.ID)
	assert.Contains(t, e.serverList(t, false), "Facturación 2026")
}

func TestCreateFolder_InvalidNames(t *testing.T) {
	e := newFolderEnv(t, "Clientes")
	require.NoError(t, e.c.SyncAccount(context.Background(), e.acc.ID))
	ctx := context.Background()
	before := e.serverList(t, false)

	_, err := e.c.CreateFolder(ctx, e.acc.ID, 0, "   ")
	wantUserError(t, err, "invalid", "Escribí un nombre para la carpeta.")
	_, err = e.c.CreateFolder(ctx, e.acc.ID, 0, "a/b")
	wantUserError(t, err, "invalid", "El nombre no puede llevar «/».")
	_, err = e.c.CreateFolder(ctx, e.acc.ID, 0, "a*")
	wantUserError(t, err, "invalid", "El nombre no puede llevar «*» ni «%».")
	_, err = e.c.CreateFolder(ctx, e.acc.ID, 0, "100%")
	wantUserError(t, err, "invalid", "El nombre no puede llevar «*» ni «%».")
	_, err = e.c.CreateFolder(ctx, e.acc.ID, 0, "clientes")
	wantUserError(t, err, "duplicate", "Ya hay una carpeta «clientes» ahí.")
	_, err = e.c.CreateFolder(ctx, e.acc.ID, 0, "inbox")
	wantUserError(t, err, "duplicate", "Ya hay una carpeta «inbox» ahí.")
	assert.Equal(t, before, e.serverList(t, false), "las validaciones van antes de escribir")
}

func TestRenameFolder_KeepsSubtreeCache(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Clientes/Acme")
	ctx := context.Background()
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c1"))
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c2"))
	e.imap.Deliver(t, "Clientes/Acme", rawMail("Bea <bea@ej.com>", "a1"))
	require.NoError(t, e.c.SyncAccount(ctx, e.acc.ID))
	cid, aid := e.id(t, "Clientes"), e.id(t, "Clientes/Acme")
	require.NoError(t, e.c.SyncFolder(ctx, cid))
	require.NoError(t, e.c.SyncFolder(ctx, aid))
	msgs := listAll(t, e.c, cid)
	amsgs := listAll(t, e.c, aid)
	require.Len(t, msgs, 2)
	require.Len(t, amsgs, 1)

	f, err := e.c.RenameFolder(ctx, cid, "Proyectos")
	require.NoError(t, err)
	assert.Equal(t, cid, f.ID)
	assert.Equal(t, "Proyectos", f.Name)
	assert.Equal(t, cid, e.id(t, "Proyectos"))
	assert.Equal(t, aid, e.id(t, "Proyectos/Acme"))
	_, ok := e.find(t, "Clientes")
	assert.False(t, ok)
	assert.Equal(t, msgs, listAll(t, e.c, cid))
	assert.Equal(t, amsgs, listAll(t, e.c, aid))

	// El servidor tiene los nombres nuevos, suscriptos, y los viejos no.
	all := e.serverList(t, false)
	assert.Contains(t, all, "Proyectos")
	assert.Contains(t, all, "Proyectos/Acme")
	assert.NotContains(t, all, "Clientes")
	assert.NotContains(t, all, "Clientes/Acme")
	subs := e.serverList(t, true)
	assert.Contains(t, subs, "Proyectos")
	assert.Contains(t, subs, "Proyectos/Acme")

	// No vuelve a bajar cabeceras.
	var progress int
	e.n.setHook(func(ev string, _ any) {
		if ev == EventSyncProgress {
			progress++
		}
	})
	require.NoError(t, e.c.SyncFolder(ctx, cid))
	require.NoError(t, e.c.SyncFolder(ctx, aid))
	assert.Zero(t, progress)
	assert.Equal(t, msgs, listAll(t, e.c, cid))
}

func TestRenameFolder_Errors(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Otra")
	require.NoError(t, e.c.SyncAccount(context.Background(), e.acc.ID))
	ctx := context.Background()
	cid := e.id(t, "Clientes")
	_, err := e.c.RenameFolder(ctx, cid, "  ")
	wantUserError(t, err, "invalid", "Escribí un nombre para la carpeta.")
	_, err = e.c.RenameFolder(ctx, cid, "x/y")
	wantUserError(t, err, "invalid", "El nombre no puede llevar «/».")
	_, err = e.c.RenameFolder(ctx, cid, "otra")
	wantUserError(t, err, "duplicate", "Ya hay una carpeta «otra» ahí.")
}

func TestMoveFolder_ToOtherParentAndTopLevel(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Clientes/Acme", "Archivo viejo")
	ctx := context.Background()
	e.imap.Deliver(t, "Clientes/Acme", rawMail("Bea <bea@ej.com>", "a1"))
	require.NoError(t, e.c.SyncAccount(ctx, e.acc.ID))
	aid := e.id(t, "Clientes/Acme")
	require.NoError(t, e.c.SyncFolder(ctx, aid))

	f, err := e.c.MoveFolder(ctx, aid, e.id(t, "Archivo viejo"))
	require.NoError(t, err)
	assert.Equal(t, aid, f.ID)
	assert.Equal(t, "Archivo viejo/Acme", f.Name)
	assert.Len(t, listAll(t, e.c, aid), 1)
	assert.Contains(t, e.serverList(t, true), "Archivo viejo/Acme")

	f, err = e.c.MoveFolder(ctx, aid, 0)
	require.NoError(t, err)
	assert.Equal(t, "Acme", f.Name)
	assert.Equal(t, aid, e.id(t, "Acme"))
	assert.NotContains(t, e.serverList(t, false), "Archivo viejo/Acme")
}

func TestMoveFolder_IntoItselfFails(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Clientes/Acme", "Clientes/Acme/Sub")
	require.NoError(t, e.c.SyncAccount(context.Background(), e.acc.ID))
	ctx := context.Background()
	cid := e.id(t, "Clientes")
	msg := "No se puede mover una carpeta dentro de sí misma."
	_, err := e.c.MoveFolder(ctx, cid, cid)
	wantUserError(t, err, "invalid", msg)
	_, err = e.c.MoveFolder(ctx, cid, e.id(t, "Clientes/Acme/Sub"))
	wantUserError(t, err, "invalid", msg)
	_, ok := e.find(t, "Clientes")
	assert.True(t, ok)
}

func TestFolder_SpecialIsNotRenamedNorMoved(t *testing.T) {
	e := newFolderEnv(t, "Padre", "Padre/Trash", "Otra")
	require.NoError(t, e.c.SyncAccount(context.Background(), e.acc.ID))
	ctx := context.Background()
	trash, ok := e.find(t, "Padre/Trash")
	require.True(t, ok)
	require.Equal(t, model.FolderRole("trash"), trash.Role)
	msg := "Las carpetas especiales no se pueden cambiar."
	for _, id := range []int64{e.id(t, "INBOX"), trash.ID, e.id(t, "Padre")} {
		_, err := e.c.RenameFolder(ctx, id, "Nueva")
		wantUserError(t, err, "invalid", msg)
		_, err = e.c.MoveFolder(ctx, id, e.id(t, "Otra"))
		wantUserError(t, err, "invalid", msg)
	}
	assert.NotContains(t, e.serverList(t, false), "Nueva")
	assert.Contains(t, e.serverList(t, false), "Padre/Trash")
}

func TestMoveFolder_OtherAccountParentFails(t *testing.T) {
	e := newFolderEnv(t, "Clientes")
	ctx := context.Background()
	other := inputFor(e.imap, e.smtp, testPass)
	other.Account.Email = "otro@ej.com"
	other.Account.Username = testUser
	acc2, err := e.c.AddAccount(ctx, other)
	require.NoError(t, err)
	require.NoError(t, e.c.SyncAccount(ctx, acc2.ID))
	require.NoError(t, e.c.SyncAccount(ctx, e.acc.ID))
	foreign := folderID(t, e.c, acc2.ID, "Sent")
	_, err = e.c.MoveFolder(ctx, e.id(t, "Clientes"), foreign)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
	_, err = e.c.CreateFolder(ctx, e.acc.ID, foreign, "X")
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
}

func TestSetFolderOrder_SavesPositions(t *testing.T) {
	e := newFolderEnv(t, "Clientes")
	require.NoError(t, e.c.SyncAccount(context.Background(), e.acc.ID))
	ctx := context.Background()
	ids := []int64{e.id(t, "Sent"), e.id(t, "Clientes"), e.id(t, "INBOX")}
	require.NoError(t, e.c.SetFolderOrder(ctx, e.acc.ID, ids))
	pos := map[string]int{}
	for _, f := range e.folders(t) {
		pos[f.Name] = f.Position
	}
	assert.Equal(t, map[string]int{"Sent": 1, "Clientes": 2, "INBOX": 3}, pos)
	assert.NotEmpty(t, e.n.matching(EventFoldersUpdated))
	err := e.c.SetFolderOrder(ctx, e.acc.ID, []int64{99999})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
}

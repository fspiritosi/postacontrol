package mail

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

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

// markSpecial deja en la caché la carpeta con ese rol, como si el servidor la
// marcara con special-use (imapmemserver no lo informa). Un SyncAccount
// posterior lo pisa.
func (e *folderEnv) markSpecial(t *testing.T, name string, role model.FolderRole) {
	t.Helper()
	fs := e.folders(t)
	found := false
	for i := range fs {
		if fs[i].Name == name {
			fs[i].Role, found = role, true
		}
	}
	require.True(t, found, name)
	require.NoError(t, e.c.store.ReplaceFolders(context.Background(), e.acc.ID, fs))
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

// La nueva va al final de su nivel: con posición solo si ese nivel ya se
// reordenó; si no, 0 y se ordena alfabética como siempre. Las especiales que
// cuelgan de INBOX se ven en la raíz, así que no cuentan como hermanas de las
// subcarpetas de INBOX.
func TestCreateFolder_PositionOnlyIfLevelOrdered(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Clientes/Acme", "INBOX/Trash")
	ctx := context.Background()
	trash, _ := e.find(t, "INBOX/Trash")
	require.Equal(t, model.FolderRole("trash"), trash.Role)
	require.NoError(t, e.c.SetFolderOrder(ctx, e.acc.ID, []int64{e.id(t, "INBOX/Trash"), e.id(t, "INBOX"), e.id(t, "Sent")}))

	f, err := e.c.CreateFolder(ctx, e.acc.ID, e.id(t, "Clientes"), "Beta")
	require.NoError(t, err)
	assert.Equal(t, 0, f.Position)
	f, err = e.c.CreateFolder(ctx, e.acc.ID, e.id(t, "INBOX"), "Nueva")
	require.NoError(t, err)
	assert.Equal(t, 0, f.Position)
	got, _ := e.find(t, "INBOX/Nueva")
	assert.Equal(t, 0, got.Position)

	// Ya ordenado ese nivel, la nueva va al final.
	require.NoError(t, e.c.SetFolderOrder(ctx, e.acc.ID, []int64{e.id(t, "Clientes/Beta"), e.id(t, "Clientes/Acme")}))
	f, err = e.c.CreateFolder(ctx, e.acc.ID, e.id(t, "Clientes"), "Zeta")
	require.NoError(t, err)
	assert.Equal(t, 4, f.Position)
}

// En la raíz cuentan también las especiales que la barra lateral muestra ahí.
func TestCreateFolder_RootOrderedOnlyBySpecial(t *testing.T) {
	e := newFolderEnv(t, "INBOX/Trash")
	ctx := context.Background()
	require.NoError(t, e.c.SetFolderOrder(ctx, e.acc.ID, []int64{e.id(t, "INBOX/Trash")}))
	f, err := e.c.CreateFolder(ctx, e.acc.ID, 0, "Raiz")
	require.NoError(t, err)
	assert.Equal(t, 2, f.Position)
}

func TestCreateFolder_Subfolder(t *testing.T) {
	e := newFolderEnv(t, "Clientes")
	require.NoError(t, e.c.SyncAccount(context.Background(), e.acc.ID))
	f, err := e.c.CreateFolder(context.Background(), e.acc.ID, e.id(t, "Clientes"), "Acme")
	require.NoError(t, err)
	assert.Equal(t, "Clientes/Acme", f.Name)
	assert.Contains(t, e.serverList(t, true), "Clientes/Acme")
}

// Una subcarpeta con nombre de especial ("Archivo") no toma el rol aunque la
// cuenta no tenga Archivo, y su padre se sigue pudiendo cambiar.
func TestCreateFolder_SpecialNameInsideIsPlain(t *testing.T) {
	e := newFolderEnv(t, "Clientes")
	ctx := context.Background()
	require.NoError(t, e.c.SyncAccount(ctx, e.acc.ID))
	f, err := e.c.CreateFolder(ctx, e.acc.ID, e.id(t, "Clientes"), "Archivo")
	require.NoError(t, err)
	assert.Equal(t, "Clientes/Archivo", f.Name)
	assert.Equal(t, model.FolderRole(""), f.Role)
	got, ok := e.find(t, "Clientes/Archivo")
	require.True(t, ok)
	assert.Equal(t, model.FolderRole(""), got.Role)

	r, err := e.c.RenameFolder(ctx, e.id(t, "Clientes"), "Clientes2")
	require.NoError(t, err)
	assert.Equal(t, "Clientes2", r.Name)
	_, err = e.c.RenameFolder(ctx, e.id(t, "Clientes2/Archivo"), "Viejo")
	require.NoError(t, err)
	assert.Contains(t, e.serverList(t, false), "Clientes2/Viejo")
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

// La movida toma la posición del nivel de destino: 0 si nunca se reordenó, y
// el final si sí; no se queda con la de su nivel viejo.
func TestMoveFolder_PositionFollowsDestinationLevel(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Clientes/Acme", "Otra", "Otra/X")
	ctx := context.Background()
	require.NoError(t, e.c.SetFolderOrder(ctx, e.acc.ID, []int64{e.id(t, "Otra"), e.id(t, "Clientes"), e.id(t, "INBOX"), e.id(t, "Sent")}))
	aid := e.id(t, "Clientes/Acme")
	require.NoError(t, e.c.SetFolderOrder(ctx, e.acc.ID, []int64{aid}))

	f, err := e.c.MoveFolder(ctx, aid, e.id(t, "Otra"))
	require.NoError(t, err)
	assert.Equal(t, "Otra/Acme", f.Name)
	assert.Equal(t, 0, f.Position)
	got, _ := e.find(t, "Otra/Acme")
	assert.Equal(t, 0, got.Position)

	f, err = e.c.MoveFolder(ctx, aid, 0)
	require.NoError(t, err)
	assert.Equal(t, "Acme", f.Name)
	assert.Equal(t, 5, f.Position)
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
	// Más adentro del primer nivel el rol sale solo de special-use.
	e.markSpecial(t, "Padre/Trash", "trash")
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

func TestRenameFolder_PartialFailureKeepsCache(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Clientes/Acme")
	ctx := context.Background()
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c1"))
	require.NoError(t, e.c.SyncAccount(ctx, e.acc.ID))
	cid := e.id(t, "Clientes")
	require.NoError(t, e.c.SyncFolder(ctx, cid))
	msgs := listAll(t, e.c, cid)
	require.Len(t, msgs, 1)
	// Otro cliente creó "Proyectos/Acme" (la caché no lo sabe): renombrar
	// "Clientes/Acme" a ese nombre falla después de que "Clientes" ya se renombró.
	require.NoError(t, e.imap.User.Create("Proyectos/Acme", nil))

	_, err := e.c.RenameFolder(ctx, cid, "Proyectos")
	if err == nil {
		t.Skip("el servidor de pruebas no dejó armar el fallo")
	}
	wantUserError(t, err, "server", msgPartialRename)
	assert.Equal(t, cid, e.id(t, "Proyectos"), "el id sobrevive con el nombre nuevo")
	assert.Equal(t, msgs, listAll(t, e.c, cid))
	_, ok := e.find(t, "Clientes")
	assert.False(t, ok)
}

// serverTotal cuenta los mensajes del buzón directo en el servidor de pruebas.
func (e *folderEnv) serverTotal(t *testing.T, name string) int {
	t.Helper()
	st, err := e.imap.User.Status(name, &goimap.StatusOptions{NumMessages: true})
	require.NoError(t, err)
	return int(*st.NumMessages)
}

func TestDeleteFolderPreview_CountsFromServer(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Clientes", "Clientes/Acme", "Clientes/Acme/Viejo")
	// Llegan después de sincronizar: la caché no los tiene.
	for _, s := range []string{"c1", "c2", "c3"} {
		e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", s))
	}
	e.imap.Deliver(t, "Clientes/Acme", rawMail("Bea <bea@ej.com>", "a1"))
	cid := e.id(t, "Clientes")
	require.Empty(t, listAll(t, e.c, cid))

	info, err := e.c.DeleteFolderPreview(context.Background(), cid)
	require.NoError(t, err)
	assert.Equal(t, model.FolderDeleteInfo{Total: 3, Subfolders: []model.SubfolderCount{
		{Name: "Acme", Total: 1}, {Name: "Acme/Viejo", Total: 0},
	}}, info)

	info, err = e.c.DeleteFolderPreview(context.Background(), e.id(t, "Clientes/Acme/Viejo"))
	require.NoError(t, err)
	assert.Equal(t, model.FolderDeleteInfo{Total: 0, Subfolders: []model.SubfolderCount{}}, info)
}

func TestDeleteFolder_Empty(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Vacía")
	moved, err := e.c.DeleteFolder(context.Background(), e.id(t, "Vacía"))
	require.NoError(t, err)
	assert.Equal(t, 0, moved)
	assert.NotContains(t, e.serverList(t, false), "Vacía")
	_, ok := e.find(t, "Vacía")
	assert.False(t, ok)
}

func TestDeleteFolder_MovesWhatIsNotCached(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Clientes", "Clientes/Acme")
	ctx := context.Background()
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c1"))
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c2"))
	e.imap.Deliver(t, "Clientes/Acme", rawMail("Bea <bea@ej.com>", "a1"))
	cid := e.id(t, "Clientes")
	require.Empty(t, listAll(t, e.c, cid), "la caché no tiene ninguno")
	trashID := e.id(t, "Trash")

	moved, err := e.c.DeleteFolder(ctx, cid)
	require.NoError(t, err)
	assert.Equal(t, 3, moved)
	assert.Equal(t, 3, e.serverTotal(t, "Trash"))
	all := e.serverList(t, false)
	assert.NotContains(t, all, "Clientes")
	assert.NotContains(t, all, "Clientes/Acme")
	_, ok := e.find(t, "Clientes")
	assert.False(t, ok)
	_, ok = e.find(t, "Clientes/Acme")
	assert.False(t, ok)
	assert.Contains(t, e.n.matching(EventFolderUpdated), map[string]any{"folderId": trashID})
	assert.Eventually(t, func() bool { return len(listAll(t, e.c, trashID)) == 3 },
		5*time.Second, 20*time.Millisecond, "la Papelera se sincroniza en segundo plano")
}

func TestDeleteFolder_NoTrashWithMail(t *testing.T) {
	e := newFolderEnv(t, "Clientes", "Clientes/Acme")
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c1"))
	_, err := e.c.DeleteFolder(context.Background(), e.id(t, "Clientes"))
	wantUserError(t, err, "no_trash", "Esta cuenta no tiene carpeta Papelera.")
	all := e.serverList(t, false)
	assert.Contains(t, all, "Clientes")
	assert.Contains(t, all, "Clientes/Acme", "tampoco se borra la subcarpeta vacía")
	assert.Equal(t, 1, e.serverTotal(t, "Clientes"))
}

func TestDeleteFolder_NoTrashEmpty(t *testing.T) {
	e := newFolderEnv(t, "Clientes")
	moved, err := e.c.DeleteFolder(context.Background(), e.id(t, "Clientes"))
	require.NoError(t, err)
	assert.Equal(t, 0, moved)
	assert.NotContains(t, e.serverList(t, false), "Clientes")
}

func TestDeleteFolder_SpecialRefused(t *testing.T) {
	e := newFolderEnv(t, "Padre", "Padre/Trash")
	ctx := context.Background()
	e.markSpecial(t, "Padre/Trash", "trash")
	trash, ok := e.find(t, "Padre/Trash")
	require.True(t, ok)
	require.Equal(t, model.FolderRole("trash"), trash.Role)
	for _, id := range []int64{e.id(t, "INBOX"), trash.ID, e.id(t, "Padre")} {
		_, err := e.c.DeleteFolder(ctx, id)
		wantUserError(t, err, "invalid", "Las carpetas especiales no se pueden cambiar.")
		_, err = e.c.DeleteFolderPreview(ctx, id)
		wantUserError(t, err, "invalid", "Las carpetas especiales no se pueden cambiar.")
	}
	all := e.serverList(t, false)
	assert.Contains(t, all, "INBOX")
	assert.Contains(t, all, "Padre")
	assert.Contains(t, all, "Padre/Trash")
}

func TestDeleteFolder_FailureKeepsFolderAndAncestors(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Clientes", "Clientes/Acme")
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c1"))
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c2"))
	e.imap.Deliver(t, "Clientes/Acme", rawMail("Bea <bea@ej.com>", "a1"))
	cid := e.id(t, "Clientes")
	// La caché todavía cree que la Papelera existe: el MOVE va a fallar.
	e.imap.DeleteMailbox(t, "Trash")

	_, err := e.c.DeleteFolder(context.Background(), cid)
	require.Error(t, err)
	all := e.serverList(t, false)
	assert.Contains(t, all, "Clientes")
	assert.Contains(t, all, "Clientes/Acme")
	assert.Equal(t, 2, e.serverTotal(t, "Clientes"))
	assert.Equal(t, 1, e.serverTotal(t, "Clientes/Acme"))
	// La caché quedó al día con el servidor.
	assert.Equal(t, cid, e.id(t, "Clientes"))
	_, ok := e.find(t, "Trash")
	assert.False(t, ok)
}

func TestDeleteFolder_StaleWhenMailKeepsArriving(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Clientes")
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c0"))
	n := 0
	e.imap.SetMoveHook(func(src string) {
		if src == "Clientes" {
			n++
			e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", fmt.Sprintf("n%d", n)))
		}
	})

	moved, err := e.c.DeleteFolder(context.Background(), e.id(t, "Clientes"))
	wantUserError(t, err, "stale", "Llegaron correos nuevos a «Clientes» mientras se vaciaba; probá de nuevo.")
	assert.Equal(t, 3, moved, "tres pasadas")
	assert.Contains(t, e.serverList(t, false), "Clientes")
	assert.Equal(t, 1, e.serverTotal(t, "Clientes"))
	assert.Equal(t, 3, e.serverTotal(t, "Trash"))
}

func TestDeleteFolder_NoselectWithChildren(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Padre/Hijo", "Padre/Otro")
	ctx := context.Background()
	e.imap.AddNoselect("Padre")
	e.imap.Deliver(t, "Padre/Hijo", rawMail("Bea <bea@ej.com>", "h1"))
	require.NoError(t, e.c.SyncAccount(ctx, e.acc.ID))
	padre, ok := e.find(t, "Padre")
	require.True(t, ok)
	require.False(t, padre.Selectable)

	info, err := e.c.DeleteFolderPreview(ctx, padre.ID)
	require.NoError(t, err)
	assert.Equal(t, model.FolderDeleteInfo{Total: 0, Subfolders: []model.SubfolderCount{
		{Name: "Hijo", Total: 1}, {Name: "Otro", Total: 0},
	}}, info)

	moved, err := e.c.DeleteFolder(ctx, padre.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, moved)
	all := e.serverList(t, false)
	for _, n := range []string{"Padre", "Padre/Hijo", "Padre/Otro"} {
		assert.NotContains(t, all, n)
		_, ok := e.find(t, n)
		assert.False(t, ok, n)
	}
	assert.Equal(t, 1, e.serverTotal(t, "Trash"))
	ops := e.imap.Ops()
	assert.NotContains(t, ops, "SELECT Padre")
	assert.NotContains(t, ops, "EXAMINE Padre")
	last := slices.Index(ops, "DELETE Padre")
	require.GreaterOrEqual(t, last, 0)
	assert.Less(t, slices.Index(ops, "DELETE Padre/Hijo"), last)
	assert.Less(t, slices.Index(ops, "DELETE Padre/Otro"), last)
	assert.GreaterOrEqual(t, slices.Index(ops, "DELETE Padre/Hijo"), 0)
	assert.GreaterOrEqual(t, slices.Index(ops, "DELETE Padre/Otro"), 0)
}

func TestDeleteFolder_StatusWithoutCountDoesNotDelete(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Clientes")
	e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c0"))
	// Después del primer MOVE llega otro correo y el servidor deja de
	// informar MESSAGES: no se puede dar la carpeta por vacía.
	e.imap.SetMoveHook(func(src string) {
		if src == "Clientes" {
			e.imap.SetMoveHook(nil)
			e.imap.OmitStatusMessages("Clientes")
			e.imap.Deliver(t, "Clientes", rawMail("Bea <bea@ej.com>", "c1"))
		}
	})

	_, err := e.c.DeleteFolder(context.Background(), e.id(t, "Clientes"))
	require.Error(t, err)
	assert.Contains(t, e.serverList(t, false), "Clientes")
	assert.Equal(t, 1, e.serverTotal(t, "Clientes"))
	assert.Equal(t, 1, e.serverTotal(t, "Trash"))
}

func TestDeleteFolder_NoselectThatBecameSelectableIsEmptied(t *testing.T) {
	e := newFolderEnv(t, "Trash", "Padre/Hijo")
	ctx := context.Background()
	e.imap.AddNoselect("Padre")
	e.imap.Deliver(t, "Padre/Hijo", rawMail("Bea <bea@ej.com>", "h1"))
	require.NoError(t, e.c.SyncAccount(ctx, e.acc.ID))
	padre, ok := e.find(t, "Padre")
	require.True(t, ok)
	require.False(t, padre.Selectable)
	// Mientras se vacía el hijo, otro cliente crea "Padre" de verdad y le
	// llega un correo.
	e.imap.SetMoveHook(func(src string) {
		if src == "Padre/Hijo" {
			e.imap.SetMoveHook(nil)
			e.imap.RemoveNoselect("Padre")
			if err := e.imap.User.Create("Padre", nil); err != nil {
				t.Errorf("crear Padre: %v", err)
			}
			e.imap.Deliver(t, "Padre", rawMail("Bea <bea@ej.com>", "p1"))
		}
	})

	moved, err := e.c.DeleteFolder(ctx, padre.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, moved)
	assert.Equal(t, 2, e.serverTotal(t, "Trash"), "el correo nuevo del padre va a la Papelera")
	assert.NotContains(t, e.serverList(t, false), "Padre")
}

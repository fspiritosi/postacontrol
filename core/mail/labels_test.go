package mail

import (
	"context"
	"fmt"
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

const okColor = "#2563eb"

// serverKeywords devuelve, por UID, los flags de todos los mensajes del buzón
// tal como están en el servidor.
func serverKeywords(t *testing.T, c *Core, imap *testenv.IMAP, mailbox string) map[uint32][]goimap.Flag {
	t.Helper()
	conn, err := imapx.Dial(context.Background(), imap.Settings, testUser, testPass, c.cfg.TLS)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.Select(mailbox)
	require.NoError(t, err)
	uids, err := conn.AllUIDs()
	require.NoError(t, err)
	if len(uids) == 0 {
		return map[uint32][]goimap.Flag{}
	}
	fl, err := conn.FetchFlags(uids)
	require.NoError(t, err)
	return fl
}

func hasPCKeyword(fs []goimap.Flag) bool {
	for _, f := range fs {
		if strings.HasPrefix(string(f), "$pc_") {
			return true
		}
	}
	return false
}

func requireInvalid(t *testing.T, err error, msg string) {
	t.Helper()
	ue := requireUserError(t, err, "invalid")
	assert.Equal(t, msg, ue.Msg)
}

func TestCreateLabelValidates(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()

	_, err := c.CreateLabel(ctx, "", okColor)
	requireInvalid(t, err, "Poné un nombre para la etiqueta.")
	_, err = c.CreateLabel(ctx, "   ", okColor)
	requireInvalid(t, err, "Poné un nombre para la etiqueta.")
	_, err = c.CreateLabel(ctx, "Facturas", "red")
	requireInvalid(t, err, "Ese color no es válido.")
	_, err = c.CreateLabel(ctx, "Facturas", "#ZZZZZZ")
	requireInvalid(t, err, "Ese color no es válido.")
	assert.Empty(t, n.matching(EventLabelsChanged), "los errores no emiten")

	l, err := c.CreateLabel(ctx, "  Facturas  ", okColor)
	require.NoError(t, err)
	assert.Equal(t, "Facturas", l.Name)
	assert.Len(t, n.matching(EventLabelsChanged), 1)
	_, err = c.CreateLabel(ctx, "facturas", okColor)
	requireInvalid(t, err, "Ya hay una etiqueta con ese nombre.")

	long, err := c.CreateLabel(ctx, strings.Repeat("ñ", 50), okColor)
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("ñ", 40), long.Name)

	kw, err := c.store.LabelKeyword(ctx, l.ID)
	require.NoError(t, err)
	assert.Regexp(t, `^\$pc_[0-9a-f]{8}$`, kw)
	kw2, err := c.store.LabelKeyword(ctx, long.ID)
	require.NoError(t, err)
	assert.NotEqual(t, kw, kw2)

	for i := 3; i <= 20; i++ {
		_, err = c.CreateLabel(ctx, fmt.Sprintf("e%d", i), okColor)
		require.NoError(t, err)
	}
	_, err = c.CreateLabel(ctx, "una más", okColor)
	requireInvalid(t, err, "Ya tenés 20 etiquetas, que es el máximo.")
}

func TestNewLabelKeywordSkipsTaken(t *testing.T) {
	// Con todos los keywords posibles de un byte fijo no se puede forzar una
	// colisión; alcanza con probar que nunca devuelve uno ya tomado.
	taken := map[string]int64{}
	for range 200 {
		kw, err := newLabelKeyword(taken)
		require.NoError(t, err)
		_, dup := taken[kw]
		require.False(t, dup)
		taken[kw] = 1
	}
}

func TestUpdateLabelNoServerWrite(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	setupSynced(t, imap, smtp, c)
	a, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	b, err := c.CreateLabel(ctx, "Clientes", okColor)
	require.NoError(t, err)
	kwBefore, err := c.store.LabelKeyword(ctx, a.ID)
	require.NoError(t, err)

	// Ni una conexión nueva ni un SELECT/EXAMINE: el servidor no ve nada.
	opsBefore, sessBefore := imap.Ops(), imap.Sessions()
	n0 := len(n.matching(EventLabelsChanged))
	up, err := c.UpdateLabel(ctx, model.Label{ID: a.ID, Name: "FACTURAS 2026", Color: "#16a34a"})
	require.NoError(t, err)
	assert.Equal(t, "FACTURAS 2026", up.Name)
	assert.Equal(t, "#16a34a", up.Color)
	assert.Equal(t, opsBefore, imap.Ops())
	assert.Equal(t, sessBefore, imap.Sessions())
	assert.Len(t, n.matching(EventLabelsChanged), n0+1)

	kwAfter, err := c.store.LabelKeyword(ctx, a.ID)
	require.NoError(t, err)
	assert.Equal(t, kwBefore, kwAfter)

	// Cambiar solo las mayúsculas de la propia etiqueta se puede.
	_, err = c.UpdateLabel(ctx, model.Label{ID: a.ID, Name: "facturas 2026", Color: "#16a34a"})
	require.NoError(t, err)

	_, err = c.UpdateLabel(ctx, model.Label{ID: a.ID, Name: "Clientes", Color: okColor})
	requireInvalid(t, err, "Ya hay una etiqueta con ese nombre.")
	_, err = c.UpdateLabel(ctx, model.Label{ID: a.ID, Name: "CLIENTES", Color: okColor})
	requireInvalid(t, err, "Ya hay una etiqueta con ese nombre.")
	_, err = c.UpdateLabel(ctx, model.Label{ID: b.ID, Name: "Clientes", Color: "azul"})
	requireInvalid(t, err, "Ese color no es válido.")
	_, err = c.UpdateLabel(ctx, model.Label{ID: 9999, Name: "x", Color: okColor})
	ue := requireUserError(t, err, "notfound")
	assert.Equal(t, "Esa etiqueta ya no existe.", ue.Msg)
}

func TestReorderLabels(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	a, _ := c.CreateLabel(ctx, "a", okColor)
	b, _ := c.CreateLabel(ctx, "b", okColor)
	d, _ := c.CreateLabel(ctx, "d", okColor)

	require.NoError(t, c.ReorderLabels(ctx, []int64{d.ID, a.ID, b.ID}))
	ls, err := c.ListLabels(ctx)
	require.NoError(t, err)
	require.Len(t, ls, 3)
	assert.Equal(t, []int64{d.ID, a.ID, b.ID}, []int64{ls[0].ID, ls[1].ID, ls[2].ID})
	n0 := len(n.matching(EventLabelsChanged))

	for _, bad := range [][]int64{{a.ID, b.ID}, {a.ID, b.ID, d.ID, 9999}, {a.ID, a.ID, b.ID}, {a.ID, b.ID, 9999}, nil} {
		requireUserError(t, c.ReorderLabels(ctx, bad), "invalid")
	}
	ls, _ = c.ListLabels(ctx)
	assert.Equal(t, d.ID, ls[0].ID, "un orden inválido no cambia nada")
	assert.Len(t, n.matching(EventLabelsChanged), n0)
}

func TestLabelUsage(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	deliverN(t, imap, 2, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	n, err := c.LabelUsage(ctx, l.ID)
	require.NoError(t, err)
	assert.Zero(t, n)
	ms := listAll(t, c, inboxID)
	require.NoError(t, c.store.AddMessageLabel(ctx, []int64{ms[0].ID, ms[1].ID}, l.ID, true))
	n, err = c.LabelUsage(ctx, l.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, n)
}

// labeledServer deja en el servidor mensajes con el keyword (y $label1) en
// INBOX y en Proveedores, una carpeta que nunca se sincronizó.
func labeledServer(t *testing.T, imap *testenv.IMAP, kw string) {
	t.Helper()
	require.NoError(t, imap.User.Create("Proveedores", nil))
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "con-1"), goimap.Flag(kw), goimap.Flag("$label1"))
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "sin"))
	imap.Deliver(t, "Proveedores", rawMail("Bea <bea@ej.com>", "con-2"), goimap.Flag(kw), goimap.FlagSeen)
}

func requireNoPCKeywords(t *testing.T, c *Core, imap *testenv.IMAP) {
	t.Helper()
	for _, mb := range []string{"INBOX", "Proveedores"} {
		for uid, fs := range serverKeywords(t, c, imap, mb) {
			assert.False(t, hasPCKeyword(fs), "%s UID %d: %v", mb, uid, fs)
		}
	}
}

func waitNoPending(t *testing.T, c *Core, accountID int64) {
	t.Helper()
	require.Eventually(t, func() bool {
		p, err := c.store.PendingLabelCleanups(context.Background(), accountID)
		return err == nil && len(p) == 0
	}, 10*time.Second, 10*time.Millisecond)
}

func TestDeleteLabelCleansServer(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, _ := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	kw, err := c.store.LabelKeyword(ctx, l.ID)
	require.NoError(t, err)
	labeledServer(t, imap, kw)
	// Se enteró de Proveedores por la lista de carpetas, sin sincronizar sus mensajes.
	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	n0 := len(n.matching(EventLabelsChanged))

	require.NoError(t, c.DeleteLabel(ctx, l.ID))
	waitNoPending(t, c, acc.ID)

	requireNoPCKeywords(t, c, imap)
	inbox := serverKeywords(t, c, imap, "INBOX")
	var label1 int
	for _, fs := range inbox {
		for _, f := range fs {
			if f == "$label1" {
				label1++
			}
		}
	}
	assert.Equal(t, 1, label1, "los keywords ajenos se quedan")
	for _, fs := range serverKeywords(t, c, imap, "Proveedores") {
		assert.Contains(t, fs, goimap.FlagSeen, "los demás flags se quedan")
	}
	ls, err := c.ListLabels(ctx)
	require.NoError(t, err)
	assert.Empty(t, ls)
	assert.Len(t, n.matching(EventLabelsChanged), n0+1)

	ue := requireUserError(t, c.DeleteLabel(ctx, l.ID), "notfound")
	assert.Equal(t, "Esa etiqueta ya no existe.", ue.Msg)
}

func TestDeleteLabelOfflineQueues(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, _ := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	kw, err := c.store.LabelKeyword(ctx, l.ID)
	require.NoError(t, err)
	labeledServer(t, imap, kw)
	require.NoError(t, c.SyncAccount(ctx, acc.ID))

	imap.Close()
	require.NoError(t, c.DeleteLabel(ctx, l.ID))
	ls, err := c.ListLabels(ctx)
	require.NoError(t, err)
	assert.Empty(t, ls)
	// La limpieza en segundo plano falla por falta de conexión: queda pendiente.
	c.runLabelCleanups(ctx, acc.ID)
	p, err := c.store.PendingLabelCleanups(ctx, acc.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{kw}, p)

	imap.Restart(t)
	c.backgroundPass(ctx)
	waitNoPending(t, c, acc.ID)
	requireNoPCKeywords(t, c, imap)
}

func TestDeleteLabelLocalAccountSkipsServer(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.store.SetAccountLabelsLocal(ctx, acc.ID)
	require.NoError(t, err)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)

	opsBefore, sessBefore := imap.Ops(), imap.Sessions()
	require.NoError(t, c.DeleteLabel(ctx, l.ID))
	p, err := c.store.PendingLabelCleanups(ctx, acc.ID)
	require.NoError(t, err)
	assert.Empty(t, p)
	// Se espera a las tareas en segundo plano para probar que no hubo tráfico.
	c.wg.Wait()
	assert.Equal(t, opsBefore, imap.Ops())
	assert.Equal(t, sessBefore, imap.Sessions())
}

// labelsByUID devuelve las etiquetas de la caché de cada mensaje de la carpeta.
func labelsByUID(t *testing.T, c *Core, folderID int64) map[uint32][]int64 {
	t.Helper()
	out := map[uint32][]int64{}
	for _, m := range listAll(t, c, folderID) {
		out[m.UID] = m.Labels
	}
	return out
}

func TestSetLabelWritesKeyword(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "m-1"), goimap.Flag("$label1"), goimap.Flag("$pc_ffffffff"))
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "m-2"), goimap.FlagSeen)
	require.NoError(t, imap.User.Create("Archivo", nil))
	imap.Deliver(t, "Archivo", rawMail("Bea <bea@ej.com>", "a-1"))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	archivoID := folderID(t, c, acc.ID, "Archivo")
	require.NoError(t, c.SyncFolder(ctx, archivoID))
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	kw, err := c.store.LabelKeyword(ctx, l.ID)
	require.NoError(t, err)
	ids := []int64{}
	for _, m := range append(listAll(t, c, inboxID), listAll(t, c, archivoID)...) {
		ids = append(ids, m.ID)
	}
	require.Len(t, ids, 3)

	n0 := len(n.matching(EventFolderUpdated))
	require.NoError(t, c.SetLabel(ctx, ids, l.ID, true))
	for _, mb := range []string{"INBOX", "Archivo"} {
		for uid, fs := range serverKeywords(t, c, imap, mb) {
			assert.Contains(t, fs, goimap.Flag(kw), "%s UID %d", mb, uid)
		}
	}
	inbox := serverKeywords(t, c, imap, "INBOX")
	assert.ElementsMatch(t, []goimap.Flag{"$label1", "$pc_ffffffff", goimap.Flag(kw)}, inbox[1],
		"los keywords ajenos se quedan")
	assert.Contains(t, inbox[2], goimap.FlagSeen, "los demás flags se quedan")
	for _, fid := range []int64{inboxID, archivoID} {
		for uid, ls := range labelsByUID(t, c, fid) {
			assert.Equal(t, []int64{l.ID}, ls, "carpeta %d UID %d", fid, uid)
		}
	}
	ev := n.matching(EventFolderUpdated)[n0:]
	assert.ElementsMatch(t, []map[string]any{{"folderId": inboxID}, {"folderId": archivoID}}, ev)

	require.NoError(t, c.SetLabel(ctx, ids, l.ID, false))
	inbox = serverKeywords(t, c, imap, "INBOX")
	assert.ElementsMatch(t, []goimap.Flag{"$label1", "$pc_ffffffff"}, inbox[1])
	assert.Equal(t, []goimap.Flag{goimap.FlagSeen}, inbox[2])
	for _, fs := range serverKeywords(t, c, imap, "Archivo") {
		assert.NotContains(t, fs, goimap.Flag(kw))
	}
	for _, fid := range []int64{inboxID, archivoID} {
		for uid, ls := range labelsByUID(t, c, fid) {
			assert.Equal(t, []int64{}, ls, "carpeta %d UID %d", fid, uid)
		}
	}
}

func TestSetLabelUnknown(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := listAll(t, c, inboxID)[0]
	ue := requireUserError(t, c.SetLabel(context.Background(), []int64{m.ID}, 9999, true), "notfound")
	assert.Equal(t, "Esa etiqueta ya no existe.", ue.Msg)
}

func TestSetLabelNoKeywordsServer(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	deliverN(t, imap, 2, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	ms := listAll(t, c, inboxID)
	m1, m2 := bySubject(t, ms, "m-1"), bySubject(t, ms, "m-2")
	imap.NoKeywords()

	require.NoError(t, c.SetLabel(ctx, []int64{m1.ID}, l.ID, true))
	assert.Equal(t, []int64{l.ID}, labelsByUID(t, c, inboxID)[m1.UID])
	local, err := c.store.AccountLabelsLocal(ctx, acc.ID)
	require.NoError(t, err)
	assert.True(t, local)
	assert.Equal(t, []map[string]any{{"accountId": acc.ID, "email": testUser}}, n.matching(EventLabelsLocalOnly))

	// La cuenta ya es local: ni conexión nueva ni SELECT.
	opsBefore, sessBefore := imap.Ops(), imap.Sessions()
	require.NoError(t, c.SetLabel(ctx, []int64{m2.ID}, l.ID, true))
	assert.Equal(t, opsBefore, imap.Ops())
	assert.Equal(t, sessBefore, imap.Sessions())
	assert.Len(t, n.matching(EventLabelsLocalOnly), 1)
	assert.Equal(t, []int64{l.ID}, labelsByUID(t, c, inboxID)[m2.UID])

	// Un sync no las borra aunque el servidor no tenga los keywords.
	require.NoError(t, c.SyncFolder(ctx, inboxID))
	for uid, ls := range labelsByUID(t, c, inboxID) {
		assert.Equal(t, []int64{l.ID}, ls, "UID %d", uid)
	}
	for uid, fs := range serverKeywords(t, c, imap, "INBOX") {
		assert.False(t, hasPCKeyword(fs), "UID %d: %v", uid, fs)
	}
}

func TestSetLabelServerRejects(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	deliverN(t, imap, 1, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	m := listAll(t, c, inboxID)[0]
	// El servidor dice que acepta keywords pero rechaza el STORE.
	imap.RejectKeywordStores()

	requireUserError(t, c.SetLabel(ctx, []int64{m.ID}, l.ID, true), "server")
	assert.Equal(t, []int64{}, labelsByUID(t, c, inboxID)[m.UID], "la caché queda como estaba")
	local, err := c.store.AccountLabelsLocal(ctx, acc.ID)
	require.NoError(t, err)
	assert.False(t, local)
	assert.Empty(t, n.matching(EventLabelsLocalOnly))
}

// waitLabeledIn espera a que la carpeta tenga exactamente un mensaje y que ese
// mensaje tenga las etiquetas want.
func waitLabeledIn(t *testing.T, c *Core, folderID int64, want []int64) {
	t.Helper()
	require.Eventually(t, func() bool {
		ms, err := c.ListMessages(context.Background(), folderID, model.ListQuery{})
		return err == nil && len(ms) == 1 && slices.Equal(ms[0].Labels, want)
	}, 10*time.Second, 10*time.Millisecond)
}

func TestMoveKeepsLabel(t *testing.T) {
	for _, noKeywords := range []bool{false, true} {
		t.Run(fmt.Sprintf("noKeywords=%v", noKeywords), func(t *testing.T) {
			c, imap, smtp, _ := newTestCore(t)
			ctx := context.Background()
			deliverN(t, imap, 1, "m")
			require.NoError(t, imap.User.Create("Archivo", nil))
			acc, inboxID := setupSynced(t, imap, smtp, c)
			archivoID := folderID(t, c, acc.ID, "Archivo")
			l, err := c.CreateLabel(ctx, "Facturas", okColor)
			require.NoError(t, err)
			if noKeywords {
				imap.NoKeywords()
			}
			m := listAll(t, c, inboxID)[0]
			require.NoError(t, c.SetLabel(ctx, []int64{m.ID}, l.ID, true))

			require.NoError(t, c.MoveMessages(ctx, []int64{m.ID}, archivoID))
			waitLabeledIn(t, c, archivoID, []int64{l.ID})
			local, err := c.store.AccountLabelsLocal(ctx, acc.ID)
			require.NoError(t, err)
			assert.Equal(t, noKeywords, local)
		})
	}
}

// Sin PERMANENTFLAGS no se sabe si el servidor guarda keywords: se manda el
// STORE y decide su respuesta. Un NO es un rechazo común, no vuelve local la cuenta.
func TestSetLabelUnknownPermanentFlags(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprintf("reject=%v", reject), func(t *testing.T) {
			c, imap, smtp, n := newTestCore(t)
			ctx := context.Background()
			deliverN(t, imap, 1, "m")
			acc, inboxID := setupSynced(t, imap, smtp, c)
			l, err := c.CreateLabel(ctx, "Facturas", okColor)
			require.NoError(t, err)
			kw, err := c.store.LabelKeyword(ctx, l.ID)
			require.NoError(t, err)
			m := listAll(t, c, inboxID)[0]
			imap.OmitPermanentFlags()
			if reject {
				imap.RejectKeywordStores()
			}

			err = c.SetLabel(ctx, []int64{m.ID}, l.ID, true)
			if reject {
				requireUserError(t, err, "server")
				assert.Equal(t, []int64{}, labelsByUID(t, c, inboxID)[m.UID], "la caché queda como estaba")
				assert.NotContains(t, serverKeywords(t, c, imap, "INBOX")[m.UID], goimap.Flag(kw))
			} else {
				require.NoError(t, err)
				assert.Equal(t, []int64{l.ID}, labelsByUID(t, c, inboxID)[m.UID])
				assert.Contains(t, serverKeywords(t, c, imap, "INBOX")[m.UID], goimap.Flag(kw))
			}
			local, err := c.store.AccountLabelsLocal(ctx, acc.ID)
			require.NoError(t, err)
			assert.False(t, local)
			assert.Empty(t, n.matching(EventLabelsLocalOnly))
		})
	}
}

// labeledSubjects devuelve los asuntos de ListLabeled.
func labeledSubjects(t *testing.T, c *Core, labelID, accountID int64) []string {
	t.Helper()
	ms, err := c.ListLabeled(context.Background(), labelID, accountID, model.ListQuery{})
	require.NoError(t, err)
	return subjects(ms)
}

func TestListLabeledAcrossFolders(t *testing.T) {
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
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }

	require.NoError(t, imapA.User.Create("Archivo", nil))
	imapA.DeliverAt(t, "INBOX", rawMail("Bea <bea@ej.com>", "a-inbox"), day(1))
	imapA.DeliverAt(t, "INBOX", rawMail("Bea <bea@ej.com>", "a-sin"), day(2))
	imapA.DeliverAt(t, "Archivo", rawMail("Bea <bea@ej.com>", "a-archivo"), day(3))
	imapB.DeliverAt(t, "INBOX", rawMail("Bea <bea@ej.com>", "b-inbox"), day(4))
	accA, err := c.AddAccount(ctx, inputFor(imapA, smtpA, testPass))
	require.NoError(t, err)
	inB := inputFor(imapB, smtpB, testPass)
	inB.Account.Email = "g@ej.com"
	accB, err := c.AddAccount(ctx, inB)
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, accA.ID))
	require.NoError(t, c.SyncAccount(ctx, accB.ID))
	archivoID := folderID(t, c, accA.ID, "Archivo")
	require.NoError(t, c.SyncFolder(ctx, archivoID))

	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	var ids []int64
	for _, f := range []int64{folderID(t, c, accA.ID, "INBOX"), archivoID, folderID(t, c, accB.ID, "INBOX")} {
		for _, m := range listAll(t, c, f) {
			if m.Subject != "a-sin" {
				ids = append(ids, m.ID)
			}
		}
	}
	// SetLabel exige una sola cuenta: se pone por cuenta.
	for _, acc := range []int64{accA.ID, accB.ID} {
		var accIDs []int64
		for _, id := range ids {
			ms, err := c.store.MessageRefs(ctx, []int64{id})
			require.NoError(t, err)
			if ms[0].AccountID == acc {
				accIDs = append(accIDs, id)
			}
		}
		require.NoError(t, c.SetLabel(ctx, accIDs, l.ID, true))
	}

	assert.Equal(t, []string{"a-archivo", "a-inbox"}, labeledSubjects(t, c, l.ID, accA.ID))
	assert.Equal(t, []string{"b-inbox", "a-archivo", "a-inbox"}, labeledSubjects(t, c, l.ID, 0))

	_, err = c.ListLabeled(ctx, 9999, 0, model.ListQuery{})
	requireUserError(t, err, "notfound")
}

func TestRefreshLabelFindsUnsyncedFolder(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	require.NoError(t, imap.User.Create("Archivo", nil))
	acc, _ := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	kw, err := c.store.LabelKeyword(ctx, l.ID)
	require.NoError(t, err)
	imap.Deliver(t, "Archivo", rawMail("Bea <bea@ej.com>", "oculto"), goimap.Flag(kw))
	imap.Deliver(t, "Archivo", rawMail("Bea <bea@ej.com>", "sin-etiqueta"))
	archivoID := folderID(t, c, acc.ID, "Archivo")
	assert.Empty(t, labeledSubjects(t, c, l.ID, 0))

	n0 := len(n.matching(EventFolderUpdated))
	require.NoError(t, c.RefreshLabel(ctx, l.ID, 0))
	ms, err := c.ListLabeled(ctx, l.ID, 0, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ms, 1)
	assert.Equal(t, []int64{l.ID}, ms[0].Labels)
	assert.Len(t, listAll(t, c, archivoID), 1, "solo baja lo que tiene el keyword")
	assert.Contains(t, n.matching(EventFolderUpdated)[n0:], map[string]any{"folderId": archivoID})

	// Un sync posterior completa el resto sin duplicar ni perder la etiqueta.
	require.NoError(t, c.SyncFolder(ctx, archivoID))
	assert.Len(t, listAll(t, c, archivoID), 2)
	assert.Equal(t, []string{"oculto"}, labeledSubjects(t, c, l.ID, 0))
}

func TestRefreshLabelDropsStale(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "uno"))
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "dos"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	var ids []int64
	for _, m := range listAll(t, c, inboxID) {
		ids = append(ids, m.ID)
	}
	require.NoError(t, c.SetLabel(ctx, ids, l.ID, true))
	// Otro cliente le saca el keyword a un mensaje.
	kw, err := c.store.LabelKeyword(ctx, l.ID)
	require.NoError(t, err)
	conn, err := imapx.Dial(ctx, imap.Settings, testUser, testPass, c.cfg.TLS)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.SelectRW("INBOX")
	require.NoError(t, err)
	require.NoError(t, conn.StoreFlags([]uint32{1}, false, goimap.Flag(kw)))
	assert.Len(t, labeledSubjects(t, c, l.ID, 0), 2)

	require.NoError(t, c.RefreshLabel(ctx, l.ID, 0))
	got := labelsByUID(t, c, inboxID)
	assert.Equal(t, []int64{}, got[1])
	assert.Equal(t, []int64{l.ID}, got[2])

	_, err = c.ListLabeled(ctx, l.ID, 0, model.ListQuery{})
	require.NoError(t, err)
	requireUserError(t, c.RefreshLabel(ctx, 9999, 0), "notfound")
}

func TestRefreshLabelKeepsCacheWhenServerHasNoKeywords(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	deliverN(t, imap, 2, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	for _, m := range listAll(t, c, inboxID) {
		require.NoError(t, c.store.AddMessageLabel(ctx, []int64{m.ID}, l.ID, true))
	}
	imap.NoKeywords()

	require.NoError(t, c.RefreshLabel(ctx, l.ID, 0))
	for uid, ls := range labelsByUID(t, c, inboxID) {
		assert.Equal(t, []int64{l.ID}, ls, "UID %d", uid)
	}
}

func TestRefreshLabelDeadAccountDoesNotStopOthers(t *testing.T) {
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
	require.NoError(t, imapA.User.Create("Archivo", nil))
	require.NoError(t, imapB.User.Create("Archivo", nil))
	accA, err := c.AddAccount(ctx, inputFor(imapA, smtpA, testPass))
	require.NoError(t, err)
	inB := inputFor(imapB, smtpB, testPass)
	inB.Account.Email = "g@ej.com"
	accB, err := c.AddAccount(ctx, inB)
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, accA.ID))
	require.NoError(t, c.SyncAccount(ctx, accB.ID))
	l, err := c.CreateLabel(ctx, "Facturas", okColor)
	require.NoError(t, err)
	kw, err := c.store.LabelKeyword(ctx, l.ID)
	require.NoError(t, err)
	imapB.Deliver(t, "Archivo", rawMail("Bea <bea@ej.com>", "oculto-b"), goimap.Flag(kw))
	imapA.Close()
	errsBefore := len(n.matching(EventSyncError))

	err = c.RefreshLabel(ctx, l.ID, 0)
	require.Error(t, err, "devuelve el error de la cuenta caída")
	assert.True(t, connFailure(err), "%v", err)
	assert.Equal(t, []string{"oculto-b"}, labeledSubjects(t, c, l.ID, 0))
	assert.Equal(t, 1, len(n.matching(EventSyncError))-errsBefore, "la cuenta caída se intenta una sola vez")
}

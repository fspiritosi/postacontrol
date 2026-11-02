package mail

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
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

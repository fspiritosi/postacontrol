package mail

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

// indexLog registra lo que hace el indexador por el gancho indexHook.
type indexLog struct {
	mu       sync.Mutex
	order    []int64         // mensajes indexados, en orden
	attempts map[int64]int   // intentos por mensaje (con o sin error)
	errs     map[int64]error // último error por mensaje
}

func (l *indexLog) indexed() []int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]int64{}, l.order...)
}

func (l *indexLog) tries(id int64) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.attempts[id]
}

func (l *indexLog) lastErr(id int64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.errs[id]
}

// fastIndex achica las esperas del indexador e instala el registro. Se llama
// antes de StartIndexing.
func fastIndex(c *Core) *indexLog {
	l := &indexLog{attempts: map[int64]int{}, errs: map[int64]error{}}
	c.indexPause = 10 * time.Millisecond
	c.indexIdle = 10 * time.Millisecond
	c.indexHook = func(id int64, err error) {
		l.mu.Lock()
		defer l.mu.Unlock()
		l.attempts[id]++
		l.errs[id] = err
		if err == nil {
			l.order = append(l.order, id)
		}
	}
	return l
}

func datedMail(subject string, date time.Time, body string) []byte {
	return []byte("From: Bea <bea@ej.com>\r\nTo: " + testUser + "\r\nSubject: " + subject +
		"\r\nDate: " + date.Format(time.RFC1123Z) + "\r\nMessage-ID: <" + subject + "@ej.com>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n" + body + "\r\n")
}

func ids(ms []model.MessageSummary) []int64 {
	out := []int64{}
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func pendingIn(t *testing.T, c *Core, folderID int64) int {
	t.Helper()
	n, err := c.store.PendingBodies(context.Background(), folderID)
	require.NoError(t, err)
	return n
}

func bodyCached(t *testing.T, c *Core, id int64) bool {
	t.Helper()
	_, cached, err := c.store.GetMessage(context.Background(), id)
	require.NoError(t, err)
	return cached
}

func TestIndexer_DownloadsNewestFirst(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	for i := range 30 {
		// Las fechas no siguen el orden de llegada (7 y 30 son coprimos):
		// el orden tiene que salir de la fecha, no del UID.
		date := base.Add(time.Duration((i*7)%30) * time.Hour)
		imap.Deliver(t, "INBOX", datedMail(fmt.Sprintf("m-%d", i), date, "hola"))
	}
	_, inboxID := setupSynced(t, imap, smtp, c)
	newestFirst := ids(listAll(t, c, inboxID)) // por fecha descendente
	require.Len(t, newestFirst, 30)
	require.Equal(t, 30, pendingIn(t, c, inboxID))
	log := fastIndex(c)
	updatesBefore := len(n.matching(EventFolderUpdated))

	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return len(log.indexed()) == 30 }, 10*time.Second, 5*time.Millisecond)

	assert.Equal(t, newestFirst, log.indexed(), "de lo más nuevo a lo más viejo")
	assert.Equal(t, 0, pendingIn(t, c, inboxID))
	for _, id := range newestFirst {
		assert.True(t, bodyCached(t, c, id))
	}
	assert.Equal(t, "hola", firstMessage(t, c, inboxID).Snippet)
	updates := n.matching(EventFolderUpdated)[updatesBefore:]
	assert.GreaterOrEqual(t, len(updates), 3, "un aviso por lote (30 mensajes = 3 lotes)")
	for _, u := range updates {
		assert.Equal(t, map[string]any{"folderId": inboxID}, u)
	}
	assert.Empty(t, n.matching(EventSyncError))
}

func TestIndexer_DoesNotMarkSeen(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 5, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	log := fastIndex(c)

	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return len(log.indexed()) == 5 }, 10*time.Second, 5*time.Millisecond)

	for _, m := range listAll(t, c, inboxID) {
		assert.False(t, m.Seen, "en la caché sigue sin leer: %s", m.Subject)
		assert.True(t, bodyCached(t, c, m.ID))
		assert.NotContains(t, serverFlags(t, c, imap, m.UID), goimap.FlagSeen, "en el servidor sigue sin leer: %s", m.Subject)
	}
	assert.Equal(t, 5, folderByID(t, c, acc.ID, inboxID).Unread)
}

func TestIndexer_SkipsAfterThreeFailures(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 5, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	var broken int64
	for _, m := range listAll(t, c, inboxID) {
		if m.UID == 3 {
			broken = m.ID
		}
	}
	require.NotZero(t, broken)
	// Otro cliente lo borra en el servidor; la caché todavía lo tiene.
	imap.Expunge(t, "INBOX", 3)
	log := fastIndex(c)

	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return log.tries(broken) >= 3 && len(log.indexed()) == 4 },
		10*time.Second, 5*time.Millisecond)
	assert.Equal(t, 0, pendingIn(t, c, inboxID), "el roto quedó salteado y el resto se indexó")
	assert.False(t, bodyCached(t, c, broken))

	// El indexador sigue andando (indexa un correo nuevo de otra carpeta) y
	// no vuelve a intentar con el roto.
	imap.Deliver(t, "Sent", datedMail("enviado", time.Now(), "hola"))
	sentID := folderID(t, c, acc.ID, "Sent")
	require.NoError(t, c.SyncFolder(context.Background(), sentID))
	require.Eventually(t, func() bool { return len(log.indexed()) == 5 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, 3, log.tries(broken), "tres intentos y no más")
	assert.Empty(t, n.matching(EventSyncError))
}

// indexAccounts arma un núcleo con dos cuentas (A y B) contra servidores
// distintos, cada una con 3 correos en la Entrada ya sincronizados.
type indexAccounts struct {
	c              *Core
	n              *recNotifier
	a, b           model.Account
	inboxA, inboxB int64
}

func twoIndexAccounts(t *testing.T) indexAccounts {
	t.Helper()
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
	deliverN(t, imapA, 3, "a")
	deliverN(t, imapB, 3, "b")
	ctx := context.Background()
	a, err := c.AddAccount(ctx, inputFor(imapA, smtpA, testPass))
	require.NoError(t, err)
	inB := inputFor(imapB, smtpB, testPass)
	inB.Account.Email = "g@ej.com"
	b, err := c.AddAccount(ctx, inB)
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, a.ID))
	require.NoError(t, c.SyncAccount(ctx, b.ID))
	return indexAccounts{c: c, n: n, a: a, b: b,
		inboxA: folderID(t, c, a.ID, "INBOX"), inboxB: folderID(t, c, b.ID, "INBOX")}
}

func TestIndexer_RespectsAuthPause(t *testing.T) {
	env := twoIndexAccounts(t)
	c := env.c
	idsA := ids(listAll(t, c, env.inboxA))
	idsB := ids(listAll(t, c, env.inboxB))
	c.noteAuthFailure(env.a.ID)
	log := fastIndex(c)

	c.StartIndexing(context.Background())
	// B se indexa entera: hubo pasadas, y en ninguna se tocó A.
	require.Eventually(t, func() bool { return len(log.indexed()) == 3 }, 10*time.Second, 5*time.Millisecond)
	assert.ElementsMatch(t, idsB, log.indexed())
	for _, id := range idsA {
		assert.Zero(t, log.tries(id), "A está en pausa")
	}
	assert.Equal(t, 3, pendingIn(t, c, env.inboxA))

	c.clearAuthFailure(env.a.ID)
	require.Eventually(t, func() bool { return len(log.indexed()) == 6 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, 0, pendingIn(t, c, env.inboxA))
}

func TestIndexer_StopsOnClose(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 60, "m")
	setupSynced(t, imap, smtp, c)
	log := fastIndex(c)
	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return len(log.indexed()) >= 1 }, 10*time.Second, 5*time.Millisecond)

	t0 := time.Now()
	require.NoError(t, c.Close())
	assert.Less(t, time.Since(t0), time.Second)
	got := len(log.indexed())
	// Después de Close no corre nada más del indexador (Close esperó a la
	// goroutine), y StartIndexing no lanza otra.
	c.StartIndexing(context.Background())
	assert.Equal(t, got, len(log.indexed()))
}

// Close también lo frena en medio de la espera larga sin pendientes.
func TestIndexer_StopsOnCloseWhileIdle(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	setupSynced(t, imap, smtp, c)
	c.indexPause = 10 * time.Millisecond
	c.indexIdle = time.Minute
	c.StartIndexing(context.Background())

	t0 := time.Now()
	require.NoError(t, c.Close())
	assert.Less(t, time.Since(t0), time.Second)
}

func TestIndexer_StopsWhenContextCancelled(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 60, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	log := fastIndex(c)
	ctx, cancel := context.WithCancel(context.Background())
	c.StartIndexing(ctx)
	require.Eventually(t, func() bool { return len(log.indexed()) >= 1 }, 10*time.Second, 5*time.Millisecond)
	cancel()
	// El indexador sale solo: Close no tiene que cortarlo.
	require.Eventually(t, func() bool {
		c.lifeMu.Lock()
		defer c.lifeMu.Unlock()
		return c.indexRunning == 0
	}, time.Second, 5*time.Millisecond)
	assert.Positive(t, pendingIn(t, c, inboxID), "frenó antes de terminar")
}

func TestIndexer_SearchFindsBodyText(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", datedMail("aviso", time.Now(), "la palabra zanahoria va solo en el cuerpo"))
	deliverN(t, imap, 2, "otro")
	_, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	before, err := c.store.Search(ctx, model.SearchQuery{Text: "zanahoria"})
	require.NoError(t, err)
	require.Empty(t, before, "sin indexar no se encuentra")
	log := fastIndex(c)

	c.StartIndexing(ctx)
	require.Eventually(t, func() bool { return len(log.indexed()) == 3 }, 10*time.Second, 5*time.Millisecond)

	got, err := c.store.Search(ctx, model.SearchQuery{Text: "zanahoria"})
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "aviso", got[0].Subject)
	assert.Equal(t, inboxID, got[0].FolderID)
}

// Sin red no se anotan fallas: al volver el servidor se indexa todo, aunque
// haya habido más de tres intentos cortados.
func TestIndexer_NetworkErrorsDoNotCountAsFailures(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 3, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	newest := firstMessage(t, c, inboxID).ID
	imap.Close()
	c.dropConn(firstMessage(t, c, inboxID).AccountID)
	log := fastIndex(c)

	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return log.tries(newest) >= 4 }, 10*time.Second, 5*time.Millisecond)
	assert.Empty(t, log.indexed())
	var ue *UserError
	require.ErrorAs(t, log.lastErr(newest), &ue)
	assert.Equal(t, "network", ue.Code)
	assert.Positive(t, len(n.matching(EventSyncError)), "withConn avisa el problema de conexión")

	imap.Restart(t)
	require.Eventually(t, func() bool { return len(log.indexed()) == 3 }, 10*time.Second, 5*time.Millisecond)
	assert.Equal(t, 0, pendingIn(t, c, inboxID))
}

// Si la carpeta se recreó en el servidor (UIDVALIDITY nuevo), el indexador no
// anota fallas: resincroniza la carpeta y sigue con los mensajes nuevos.
func TestIndexer_StaleFolderResyncs(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 3, "viejo")
	_, inboxID := setupSynced(t, imap, smtp, c)
	imap.RecreateMailbox(t, "INBOX")
	deliverN(t, imap, 2, "nuevo")
	log := fastIndex(c)

	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return len(log.indexed()) == 2 }, 10*time.Second, 5*time.Millisecond)
	ms := listAll(t, c, inboxID)
	assert.ElementsMatch(t, []string{"nuevo-1", "nuevo-2"}, subjects(ms))
	assert.ElementsMatch(t, ids(ms), log.indexed())
	assert.Equal(t, 0, pendingIn(t, c, inboxID))
}

func TestIndexer_StartIsIdempotent(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 3, "m")
	setupSynced(t, imap, smtp, c)
	log := fastIndex(c)
	c.StartIndexing(context.Background())
	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return len(log.indexed()) == 3 }, 10*time.Second, 5*time.Millisecond)
	c.lifeMu.Lock()
	running := c.indexRunning
	c.lifeMu.Unlock()
	assert.Equal(t, 1, running)
}

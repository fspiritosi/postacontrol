package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
)

// fastWatch acorta las esperas de los vigías y envuelve c.dialWatch para
// contar los intentos de conexión. Se llama antes de StartWatching.
func fastWatch(c *Core, backoff ...time.Duration) *atomic.Int32 {
	if len(backoff) == 0 {
		backoff = []time.Duration{50 * time.Millisecond}
	}
	c.watchDebounce = 50 * time.Millisecond
	c.watchBackoff = backoff
	var dials atomic.Int32
	inner := c.dialWatch
	c.dialWatch = func(ctx context.Context, s model.ServerSettings, user, pass string, tlsCfg *tls.Config, onChange func()) (*imapx.Conn, error) {
		dials.Add(1)
		return inner(ctx, s, user, pass, tlsCfg, onChange)
	}
	return &dials
}

// waitWatching espera a que el vigía haya conectado y hecho la sincronización
// que sigue a cada conexión (para entonces el IDLE ya arrancó).
func waitWatching(t *testing.T, n *recNotifier, dials *atomic.Int32, wantDials int32, updatesBefore int) {
	t.Helper()
	require.Eventually(t, func() bool {
		return dials.Load() >= wantDials && len(n.matching(EventFolderUpdated)) > updatesBefore
	}, 5*time.Second, 10*time.Millisecond, "el vigía no conectó")
}

func TestWatcher_NewMailWithoutManualSync(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	_, inboxID := setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	before := len(n.matching(EventFolderUpdated))

	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "alinstante"))

	require.Eventually(t, func() bool { return len(newMailEvents(n)) == 1 }, 5*time.Second, 10*time.Millisecond)
	evs := newMailEvents(n)
	assert.Equal(t, 1, evs[0].Count)
	assert.Equal(t, "alinstante", evs[0].Items[0].Subject)
	assert.Len(t, listAll(t, c, inboxID), 1)
	assert.Empty(t, n.matching(EventSyncError))
}

func TestWatcher_ReconnectsAfterServerRestart(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	before := len(n.matching(EventFolderUpdated))

	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)
	imap.Restart(t)
	// Correo que llega con el vigía caído o reconectando: lo levanta la
	// sincronización que sigue a la reconexión, o el aviso del IDLE nuevo.
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "trasreinicio"))

	require.Eventually(t, func() bool { return len(newMailEvents(n)) == 1 }, 10*time.Second, 10*time.Millisecond)
	assert.GreaterOrEqual(t, dials.Load(), int32(2), "el vigía volvió a conectar")

	// Y el IDLE nuevo avisa de lo que llega después.
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "otro"))
	require.Eventually(t, func() bool { return len(newMailEvents(n)) == 2 }, 10*time.Second, 10*time.Millisecond)
	assert.Empty(t, n.matching(EventSyncError), "el vigía no avisa sus cortes")
}

func TestWatcher_RespectsAuthPause(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	require.NoError(t, c.cfg.Secrets.Set(testUser, "mala"))

	c.StartWatching(context.Background())
	require.Eventually(t, func() bool { return dials.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(500 * time.Millisecond) // ventana de observación: sin pausa serían ~10 intentos
	assert.Equal(t, int32(1), dials.Load(), "una contraseña rechazada no se reintenta en seguida")
	assert.True(t, c.authPaused(acc.ID), "el rechazo quedó anotado")
	assert.Equal(t, 1, c.watcherCount(), "el vigía sigue esperando la contraseña")
	// El rechazo sí se avisa (una vez): si no, con la sincronización periódica
	// salteando la cuenta en pausa, el usuario nunca se enteraría.
	errs := n.matching(EventSyncError)
	require.Len(t, errs, 1)
	assert.Equal(t, acc.ID, errs[0]["accountId"])
	assert.Equal(t, "El servidor rechazó el usuario o la contraseña.", errs[0]["message"])
}

// Al vencer la pausa, el vigía prueba una sola vez más (y si falla, avisa y
// vuelve a esperar).
func TestWatcher_RetriesOnceAfterPauseExpires(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	var offset atomic.Int64
	base := time.Now()
	c.now = func() time.Time { return base.Add(time.Duration(offset.Load())) }
	dials := fastWatch(c)
	require.NoError(t, c.cfg.Secrets.Set(testUser, "mala"))

	c.StartWatching(context.Background())
	require.Eventually(t, func() bool { return dials.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
	require.Eventually(t, func() bool { return c.authPaused(acc.ID) }, 5*time.Second, 5*time.Millisecond)

	offset.Store(int64(authPause + time.Second))
	require.Eventually(t, func() bool { return dials.Load() >= 2 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(500 * time.Millisecond) // ventana de observación
	assert.Equal(t, int32(2), dials.Load(), "un solo intento por ventana de pausa")
	assert.Equal(t, 2, syncErrorsOf(n, acc.ID), "un aviso por rechazo")
}

// Si la contraseña cambió en otro lado mientras la sesión del vigía sigue
// viva, cada ráfaga de correo dispara una sincronización por el pool: la
// primera falla con el LOGIN y pone la cuenta en pausa; las siguientes no
// vuelven a probar la contraseña mala.
func TestWatcher_SyncRespectsAuthPause(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	before := len(n.matching(EventFolderUpdated))
	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)

	require.NoError(t, c.cfg.Secrets.Set(testUser, "mala"))
	c.dropConn(acc.ID) // la próxima sincronización tiene que hacer LOGIN
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "uno"))
	require.Eventually(t, func() bool { return c.authPaused(acc.ID) }, 5*time.Second, 5*time.Millisecond)
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "dos"))
	time.Sleep(500 * time.Millisecond) // ventana de observación: varias esperas de debounce

	assert.Equal(t, 1, syncErrorsOf(n, acc.ID), "un solo LOGIN fallido")
	assert.Equal(t, int32(1), dials.Load(), "la sesión del vigía sigue viva")
	assert.Empty(t, newMailEvents(n))
}

func TestWatcher_StopsOnRemoveAccount(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	before := len(n.matching(EventFolderUpdated))

	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)
	require.Equal(t, 1, c.watcherCount())

	require.NoError(t, c.RemoveAccount(context.Background(), acc.ID))
	assert.Eventually(t, func() bool { return c.watcherCount() == 0 }, time.Second, 5*time.Millisecond)

	t0 := time.Now()
	require.NoError(t, c.Close())
	assert.Less(t, time.Since(t0), time.Second)
}

// spySecrets anota, al borrar el secreto, si el vigía de la cuenta seguía
// registrado; fail hace que Delete falle.
type spySecrets struct {
	secrets.Store
	c            *Core
	accountID    int64
	fail         bool
	watcherAtDel atomic.Bool
	deleted      atomic.Bool
}

func (s *spySecrets) Delete(email string) error {
	s.c.watchMu.Lock()
	s.watcherAtDel.Store(s.c.watchers[s.accountID] != nil)
	s.c.watchMu.Unlock()
	s.deleted.Store(true)
	if s.fail {
		return errors.New("llavero bloqueado")
	}
	return s.Store.Delete(email)
}

// El vigía se frena antes de borrar el secreto: si no, uno que reconecta en
// ese momento avisaría "No encontramos la contraseña…" de una cuenta borrada.
func TestWatcher_RemoveAccountStopsWatcherBeforeSecret(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	spy := &spySecrets{Store: c.cfg.Secrets, c: c, accountID: acc.ID}
	c.cfg.Secrets = spy // antes de arrancar los vigías, que leen cfg.Secrets
	before := len(n.matching(EventFolderUpdated))
	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)

	require.NoError(t, c.RemoveAccount(context.Background(), acc.ID))
	require.True(t, spy.deleted.Load())
	assert.False(t, spy.watcherAtDel.Load(), "el vigía seguía andando al borrar el secreto")
	assert.Eventually(t, func() bool { return c.watcherCount() == 0 }, time.Second, 5*time.Millisecond)
	assert.Empty(t, n.matching(EventSyncError))
}

// Si el llavero no deja borrar el secreto, la cuenta queda y su vigía vuelve.
func TestWatcher_RemoveAccountSecretFailureKeepsWatcher(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	c.cfg.Secrets = &spySecrets{Store: c.cfg.Secrets, c: c, accountID: acc.ID, fail: true}
	before := len(n.matching(EventFolderUpdated))
	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)

	require.Error(t, c.RemoveAccount(context.Background(), acc.ID))
	c.watchMu.Lock()
	registered := c.watchers[acc.ID] != nil
	c.watchMu.Unlock()
	assert.True(t, registered, "la cuenta sigue y tiene que tener su vigía")
	require.Eventually(t, func() bool { return dials.Load() >= 2 }, 5*time.Second, 10*time.Millisecond, "el vigía nuevo conecta")
}

func TestWatcher_StartWatchingIsIdempotent(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	before := len(n.matching(EventFolderUpdated))

	c.StartWatching(context.Background())
	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "uno"))

	require.Eventually(t, func() bool { return len(newMailEvents(n)) == 1 }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, 1, c.watcherCount())
	assert.Equal(t, int32(1), dials.Load(), "una sola conexión de vigía")
	assert.Len(t, newMailEvents(n), 1)
}

// Una cuenta agregada después de StartWatching tiene su vigía (que espera a
// que la primera sincronización cree la Entrada).
func TestWatcher_StartsForAddedAccount(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	dials := fastWatch(c)
	c.StartWatching(context.Background())
	require.Equal(t, 0, c.watcherCount())

	acc, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	assert.Equal(t, 1, c.watcherCount())
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))
	waitWatching(t, n, dials, 1, len(n.matching(EventFolderUpdated)))

	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "nueva"))
	require.Eventually(t, func() bool { return len(newMailEvents(n)) == 1 }, 5*time.Second, 10*time.Millisecond)
}

// Si la conexión anda pero la sesión se cae enseguida (un servidor que acepta
// el LOGIN y corta), el vigía no puede volver a conectar a ritmo fijo: cada
// sesión corta cuenta como una falla y la espera crece.
func TestWatcher_BacksOffWhenSessionsDieImmediately(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	setupSynced(t, imap, smtp, c)
	c.watchDebounce = 50 * time.Millisecond
	c.watchBackoff = []time.Duration{10 * time.Millisecond, 200 * time.Millisecond, 5 * time.Second}
	var dials atomic.Int32
	inner := c.dialWatch
	c.dialWatch = func(ctx context.Context, s model.ServerSettings, user, pass string, tlsCfg *tls.Config, onChange func()) (*imapx.Conn, error) {
		dials.Add(1)
		conn, err := inner(ctx, s, user, pass, tlsCfg, onChange)
		if err == nil {
			_ = conn.Abort() // la sesión muere antes del IDLE
		}
		return conn, err
	}

	c.StartWatching(context.Background())
	require.Eventually(t, func() bool { return dials.Load() >= 1 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(700 * time.Millisecond) // ventana de observación: a ritmo fijo serían decenas
	assert.LessOrEqual(t, dials.Load(), int32(3))
	assert.Empty(t, n.matching(EventSyncError))
}

// Close frena los vigías aunque estén en medio de una espera larga.
func TestWatcher_CloseStopsWatchers(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	before := len(n.matching(EventFolderUpdated))
	c.StartWatching(context.Background())
	waitWatching(t, n, dials, 1, before)

	t0 := time.Now()
	require.NoError(t, c.Close())
	assert.Less(t, time.Since(t0), time.Second)
	assert.Equal(t, 0, c.watcherCount())

	// Después de Close no se lanzan vigías nuevos.
	c.StartWatching(context.Background())
	assert.Equal(t, 0, c.watcherCount())
}

// El ctx de StartWatching también los frena.
func TestWatcher_StopsWhenContextCancelled(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	setupSynced(t, imap, smtp, c)
	dials := fastWatch(c)
	before := len(n.matching(EventFolderUpdated))
	ctx, cancel := context.WithCancel(context.Background())
	c.StartWatching(ctx)
	waitWatching(t, n, dials, 1, before)

	cancel()
	assert.Eventually(t, func() bool { return c.watcherCount() == 0 }, time.Second, 5*time.Millisecond)
}

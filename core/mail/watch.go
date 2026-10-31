package mail

import (
	"context"
	"errors"
	"time"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

// watchDebounce es cuánto espera un vigía después del último aviso del
// servidor antes de sincronizar: una ráfaga de correos es una sola pasada.
const watchDebounce = 2 * time.Second

// watchBackoff son las esperas entre reconexiones de un vigía: avanza un
// escalón por cada falla seguida y se queda en el último.
var watchBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}

// watchStable es cuánto tiene que durar una sesión de vigía para contar como
// una conexión buena y volver al primer escalón de watchBackoff. Una sesión
// que se cae enseguida (por ejemplo, un servidor que acepta el LOGIN y corta,
// o que rechaza el EXAMINE) cuenta como una falla más: si no, el vigía
// volvería a iniciar sesión cada watchBackoff[0] para siempre, y eso puede
// hacer que el proveedor bloquee la cuenta.
const watchStable = time.Minute

// watcher es el vigía activo de una cuenta.
type watcher struct {
	cancel context.CancelFunc
}

// StartWatching lanza un vigía IDLE por cuenta: una conexión aparte que queda
// esperando avisos del servidor sobre la Entrada y la sincroniza al instante
// (SyncFolder emite EventNewMail). Desde esta llamada, AddAccount lanza el
// vigía de la cuenta nueva y RemoveAccount frena el de la cuenta quitada.
// Cancelar ctx o Close los frenan a todos. Llamarla de nuevo no hace nada. No
// bloquea.
//
// Los vigías no emiten EventSyncError por sus problemas de conexión: la
// sincronización periódica ya los informa.
func (c *Core) StartWatching(ctx context.Context) {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	if c.watching {
		return
	}
	// Listar con watchMu tomado: una cuenta que AddAccount inserta en paralelo
	// o sale en esta lista o AddAccount la ve después con watching en true.
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return
	}
	c.watching = true
	c.watchCtx = ctx
	for _, a := range accs {
		c.startWatcherLocked(a.ID)
	}
}

// startWatcher lanza el vigía de la cuenta si StartWatching ya se llamó y la
// cuenta no tiene uno.
func (c *Core) startWatcher(accountID int64) {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	c.startWatcherLocked(accountID)
}

func (c *Core) startWatcherLocked(accountID int64) {
	if !c.watching || c.watchers[accountID] != nil {
		return
	}
	ctx, cancel := context.WithCancel(c.watchCtx)
	w := &watcher{cancel: cancel}
	ok := c.spawn(func(life context.Context) {
		defer context.AfterFunc(life, cancel)()
		c.watchAccount(ctx, accountID)
		cancel()
		c.watchMu.Lock()
		defer c.watchMu.Unlock()
		if c.watchers[accountID] == w {
			delete(c.watchers, accountID)
		}
		c.watchLive--
	})
	if !ok { // el núcleo se está cerrando
		cancel()
		return
	}
	// La goroutine no puede terminar antes de esto: para salir necesita watchMu.
	c.watchers[accountID] = w
	c.watchLive++
}

// stopWatcher frena el vigía de la cuenta, si tiene. No espera a que termine.
func (c *Core) stopWatcher(accountID int64) {
	c.watchMu.Lock()
	w := c.watchers[accountID]
	delete(c.watchers, accountID)
	c.watchMu.Unlock()
	if w != nil {
		w.cancel()
	}
}

// watcherCount devuelve cuántas goroutines de vigía siguen vivas (para tests).
func (c *Core) watcherCount() int {
	c.watchMu.Lock()
	defer c.watchMu.Unlock()
	return c.watchLive
}

// watchAccount es el ciclo de un vigía. Termina cuando ctx se cancela, cuando
// la cuenta ya no existe o cuando el servidor no permite IDLE (queda la
// sincronización periódica).
func (c *Core) watchAccount(ctx context.Context, accountID int64) {
	step := 0
	// backoff espera el escalón actual y avanza uno. Devuelve false si ctx se
	// canceló mientras esperaba.
	backoff := func() bool {
		d := c.watchBackoff[step]
		step = min(step+1, len(c.watchBackoff)-1)
		return sleepCtx(ctx, d)
	}
	for ctx.Err() == nil {
		// 1. Con la contraseña rechazada no se prueba de nuevo hasta que se
		// levante la pausa (UpdatePassword, o que venza).
		if c.authPaused(accountID) {
			if !sleepCtx(ctx, c.watchBackoff[0]) {
				return
			}
			continue
		}
		// 2. La Entrada la crea la primera sincronización de la cuenta.
		inbox, ok, err := c.watchInbox(ctx, accountID)
		if err != nil || !ok {
			if !sleepCtx(ctx, c.watchBackoff[0]) {
				return
			}
			continue
		}
		// 3. Conectar.
		kick := make(chan struct{}, 1)
		onChange := func() { // corre en la goroutine lectora de go-imap: no bloquear
			select {
			case kick <- struct{}{}:
			default:
			}
		}
		conn, err := c.dialWatchAccount(ctx, accountID, onChange)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, store.ErrNotFound) {
				return // frenado, o la cuenta ya no existe
			}
			var ue *UserError
			if errors.As(asUserError(err), &ue) && ue.Code == "auth" {
				c.noteAuthFailure(accountID)
				continue
			}
			if !backoff() {
				return
			}
			continue
		}
		c.clearAuthFailure(accountID)
		// 4. IDLE hasta que se corte.
		start := time.Now()
		err = c.watchSession(ctx, conn, inbox, kick)
		_ = conn.Abort()
		if ctx.Err() != nil || errors.Is(err, imapx.ErrUnsupported) {
			return
		}
		// 5. Reconectar con espera.
		if time.Since(start) >= watchStable {
			step = 0
		}
		if !backoff() {
			return
		}
	}
}

// watchInbox busca la Entrada de la cuenta en la caché.
func (c *Core) watchInbox(ctx context.Context, accountID int64) (model.Folder, bool, error) {
	fs, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Folder{}, false, err
	}
	for _, f := range fs {
		if f.Role == "inbox" && f.Selectable {
			return f, true, nil
		}
	}
	return model.Folder{}, false, nil
}

// dialWatchAccount abre la conexión del vigía con la contraseña del llavero.
// No pasa por el pool ni emite nada.
func (c *Core) dialWatchAccount(ctx context.Context, accountID int64, onChange func()) (*imapx.Conn, error) {
	acc, user, password, err := c.credentials(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return c.dialWatch(ctx, acc.IMAP, user, password, c.cfg.TLS, onChange)
}

// watchSession corre IdleWatch sobre la Entrada y sincroniza (por el pool)
// watchDebounce después de conectar y del último aviso. Devuelve el error de
// IdleWatch; no deja goroutines vivas.
func (c *Core) watchSession(ctx context.Context, conn *imapx.Conn, inbox model.Folder, kick <-chan struct{}) error {
	ctx, cancel := context.WithCancel(ctx)
	// Al frenar se corta la conexión sin esperar el DONE del IDLE: con la red
	// caída, cerrarlo bien puede demorar el cierre de la app.
	stopAbort := context.AfterFunc(ctx, func() { _ = conn.Abort() })
	defer stopAbort()

	done := make(chan error, 1)
	go func() { done <- conn.IdleWatch(ctx, inbox.Name) }()
	finished := false
	defer func() {
		cancel()
		if !finished {
			<-done
		}
	}()

	// IdleWatch exige sincronizar después de arrancar: lo que llegó entre el
	// SELECT y el IDLE no genera aviso.
	t := time.NewTimer(c.watchDebounce)
	defer t.Stop()
	for {
		select {
		case err := <-done:
			finished = true
			return err
		case <-kick:
			t.Reset(c.watchDebounce)
		case <-t.C:
			// Los errores de la sincronización los avisa withConn; la
			// próxima pasada (por aviso o periódica) reintenta.
			_ = c.SyncFolder(ctx, inbox.ID)
		}
	}
}

// sleepCtx espera d o hasta que ctx se cancele. Devuelve false si se canceló.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

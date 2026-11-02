// Package mail es la fachada del núcleo: orquesta store, secrets, autoconfig,
// imapx y smtpx sin depender de Wails.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"postacontrol/core/autoconfig"
	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/store"
)

// Notifier recibe los eventos del núcleo (la cáscara los traduce a eventos de Wails).
//
// Emit se llama de forma sincrónica desde la goroutine que sincroniza, a veces
// con el lock de la conexión de una cuenta tomado: una implementación no debe
// volver a llamar a Core de forma sincrónica (se trabaría) ni bloquear por
// mucho tiempo. Si necesita reaccionar llamando a Core, que lo haga en otra
// goroutine.
type Notifier interface {
	Emit(event string, data any)
}

type Config struct {
	DBPath     string
	Secrets    secrets.Store
	Notifier   Notifier
	TLS        *tls.Config // nil = raíces del sistema; los tests inyectan RootCAs
	Autoconfig autoconfig.Options
	// SkipInitialSync evita lanzar SyncAccount al agregar una cuenta (solo para tests).
	SkipInitialSync bool
}

type Core struct {
	cfg   Config
	store *store.Store
	// dataDir es la carpeta de datos (la de la base); ahí vive el fondo.
	dataDir string

	// Ciclo de vida: las goroutines propias (sincronización inicial y
	// periódica) usan ctx y se registran en wg. Close cancela y espera.
	ctx       context.Context
	cancel    context.CancelFunc
	lifeMu    sync.Mutex
	closed    bool
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error

	// afterServerSeen es un gancho solo para tests: corre después de que el
	// servidor aceptó \Seen y antes de actualizar la caché.
	afterServerSeen func()
	// beforeFetchBody es un gancho solo para tests: fetchBody lo llama con la
	// conexión y el UID después de abrir la carpeta y antes de pedir el
	// mensaje (por ejemplo, para cortar la conexión con un UID dado).
	beforeFetchBody func(conn *imapx.Conn, uid uint32)
	// afterFolderList es un gancho solo para tests: refreshFolders lo llama
	// después del LIST y antes de guardar la lista en la caché.
	afterFolderList func(accountID int64)

	poolMu sync.Mutex
	pool   map[int64]*pooledConn // una conexión IMAP por cuenta

	// syncLocks serializa las sincronizaciones de una misma carpeta (la
	// conexión se toma lote por lote, así que dos pasadas se intercalarían).
	syncLocksMu sync.Mutex
	syncLocks   map[int64]*sync.Mutex

	// folderLocks serializa, por cuenta, el refresco de la lista de carpetas
	// (del LIST hasta guardarla en la caché) con lo que la cambia: crear,
	// renombrar, mover y eliminar carpetas, y guardar el orden manual. Sin
	// esto, un refresco que listó antes de un renombrado podía guardar
	// después y llevarse la fila renombrada con sus correos.
	folderLocksMu sync.Mutex
	folderLocks   map[int64]*sync.Mutex

	// draftLocks serializa lo que toca en el servidor un mismo borrador
	// (subir, descartar, limpiar después de enviar). beforeDraftAppend y
	// afterDraftAppend son ganchos solo para tests: corren justo antes y justo
	// después del APPEND de un borrador, antes de darlo por subido.
	draftLocksMu      sync.Mutex
	draftLocks        map[int64]*sync.Mutex
	beforeDraftAppend func(id int64)
	afterDraftAppend  func(id int64)

	// now es el reloj (se inyecta en los tests). authFailedAt anota cuándo el
	// servidor rechazó por última vez la contraseña de cada cuenta.
	now          func() time.Time
	authMu       sync.Mutex
	authFailedAt map[int64]time.Time

	// Vigías IDLE (watch.go). dialWatch, watchDebounce y watchBackoff se
	// inyectan en los tests. watchMu protege el resto: watching dice si ya se
	// llamó a StartWatching, watchers tiene el vigía activo de cada cuenta y
	// watchLive cuenta las goroutines de vigía vivas (incluidas las frenadas
	// que todavía no terminaron).
	dialWatch     func(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config, onChange func()) (*imapx.Conn, error)
	watchDebounce time.Duration
	watchBackoff  []time.Duration
	watchMu       sync.Mutex
	watching      bool
	watchCtx      context.Context
	watchers      map[int64]*watcher
	watchLive     int

	// Indexador (index.go). indexPause, indexIdle e indexBackoff se inyectan
	// en los tests; indexHook es un gancho solo para tests: corre después de
	// cada intento de bajar un cuerpo, con su resultado (nil = indexado).
	// indexStarted e indexRunning (goroutines del indexador vivas) los
	// protege lifeMu.
	indexPause   time.Duration
	indexIdle    time.Duration
	indexBackoff []time.Duration
	indexHook    func(messageID int64, err error)
	indexStarted bool
	indexRunning int
}

func New(cfg Config) (*Core, error) {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	// Envíos que quedaron a medias porque la app se cerró en el medio.
	if err := st.FailInterruptedDrafts(context.Background(), msgSendInterrupted); err != nil {
		_ = st.Close()
		return nil, err
	}
	// Adjuntos de redactores cancelados que nadie reclamó (mejor esfuerzo).
	_ = st.PruneStagedAttachments(context.Background(), time.Now().Add(-24*time.Hour))
	ctx, cancel := context.WithCancel(context.Background())
	return &Core{cfg: cfg, store: st, dataDir: filepath.Dir(cfg.DBPath), ctx: ctx, cancel: cancel,
		pool: map[int64]*pooledConn{}, syncLocks: map[int64]*sync.Mutex{}, draftLocks: map[int64]*sync.Mutex{},
		folderLocks: map[int64]*sync.Mutex{}, now: time.Now, authFailedAt: map[int64]time.Time{},
		dialWatch: imapx.DialWatch, watchDebounce: watchDebounce,
		watchBackoff: slices.Clone(watchBackoff), watchers: map[int64]*watcher{},
		indexPause: indexPause, indexIdle: indexIdle, indexBackoff: slices.Clone(indexBackoff)}, nil
}

// Close cancela las sincronizaciones en curso, espera a que terminen, cierra
// las conexiones IMAP y por último el store. Se puede llamar más de una vez.
func (c *Core) Close() error {
	c.closeOnce.Do(func() {
		c.lifeMu.Lock()
		c.closed = true
		c.lifeMu.Unlock()
		c.cancel()
		c.wg.Wait()
		c.closeConns()
		c.closeErr = c.store.Close()
	})
	return c.closeErr
}

// spawn corre fn en una goroutine atada al ciclo de vida del núcleo. Después
// de Close no lanza nada y devuelve false.
func (c *Core) spawn(fn func(ctx context.Context)) bool {
	c.lifeMu.Lock()
	defer c.lifeMu.Unlock()
	if c.closed {
		return false
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		fn(c.ctx)
	}()
	return true
}

// emit avisa por Notifier. Ver la advertencia de Notifier: se puede llamar con
// el lock de la conexión de una cuenta tomado.
func (c *Core) emit(event string, data any) {
	if c.cfg.Notifier != nil {
		c.cfg.Notifier.Emit(event, data)
	}
}

// DetectSettings detecta los servidores a partir del correo.
func (c *Core) DetectSettings(ctx context.Context, email string) (autoconfig.Settings, error) {
	return autoconfig.Detect(ctx, email, c.cfg.Autoconfig)
}

// pooledConn es la conexión IMAP de una cuenta. mu serializa su uso: IMAP
// tiene una sola carpeta seleccionada por sesión.
type pooledConn struct {
	mu   sync.Mutex
	conn *imapx.Conn // nil = sin conectar
}

func (c *Core) pooled(accountID int64) *pooledConn {
	c.poolMu.Lock()
	defer c.poolMu.Unlock()
	pc := c.pool[accountID]
	if pc == nil {
		pc = &pooledConn{}
		c.pool[accountID] = pc
	}
	return pc
}

// folderSyncLock devuelve el lock de sincronización de la carpeta.
func (c *Core) folderSyncLock(folderID int64) *sync.Mutex {
	c.syncLocksMu.Lock()
	defer c.syncLocksMu.Unlock()
	mu := c.syncLocks[folderID]
	if mu == nil {
		mu = &sync.Mutex{}
		c.syncLocks[folderID] = mu
	}
	return mu
}

// folderListLock devuelve el lock de la lista de carpetas de la cuenta (ver
// folderLocks). Quien lo tiene refresca con refreshFoldersLocked.
func (c *Core) folderListLock(accountID int64) *sync.Mutex {
	c.folderLocksMu.Lock()
	defer c.folderLocksMu.Unlock()
	mu := c.folderLocks[accountID]
	if mu == nil {
		mu = &sync.Mutex{}
		c.folderLocks[accountID] = mu
	}
	return mu
}

// lifeCtx deriva de ctx un contexto que además se cancela con Close.
func (c *Core) lifeCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)
	if c.ctx.Err() != nil { // AfterFunc cancelaría recién en otra goroutine
		cancel()
		return ctx, cancel
	}
	stop := context.AfterFunc(c.ctx, cancel)
	return ctx, func() { stop(); cancel() }
}

// ctxErr devuelve el error de ctx o, si el núcleo se está cerrando,
// context.Canceled. Mira c.ctx directamente porque la cancelación que propaga
// lifeCtx es asíncrona, y Close cancela c.ctx antes de cerrar el store.
func (c *Core) ctxErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.ctx.Err()
}

// ctxOrUserError devuelve el error del contexto si se canceló (por ejemplo,
// porque el núcleo se está cerrando y el store ya no está) y si no, err como
// UserError.
func (c *Core) ctxOrUserError(ctx context.Context, err error) error {
	if cerr := c.ctxErr(ctx); cerr != nil {
		return cerr
	}
	return asUserError(err)
}

// withConn corre fn con la conexión de la cuenta (la abre si hace falta). fn
// recibe un contexto derivado de ctx que también se cancela con Close. Si fn
// falla por un error de red, cierra la conexión, vuelve a conectar una vez y
// reintenta, así que fn tiene que ser idempotente; para operaciones que no lo
// son, usar withConnNoRetry.
//
// Si la conexión no se puede usar (red, credenciales o TLS) emite
// EventSyncError. Close o la cancelación de ctx cortan la conexión en curso;
// en ese caso no se reintenta ni se emite nada y el error es el del contexto
// (context.Canceled). Los demás errores salen como UserError. Si sale bien
// y para eso hizo un LOGIN nuevo, levanta la pausa por contraseña rechazada
// de la cuenta.
func (c *Core) withConn(ctx context.Context, accountID int64, fn func(context.Context, *imapx.Conn) error) error {
	return c.useConn(ctx, accountID, 2, fn)
}

// withConnNoRetry es como withConn pero hace un solo intento: si fn falla por
// un error de red, descarta la conexión y devuelve el error sin reintentar.
// Es para operaciones que no son idempotentes (por ejemplo, APPEND).
func (c *Core) withConnNoRetry(ctx context.Context, accountID int64, fn func(context.Context, *imapx.Conn) error) error {
	return c.useConn(ctx, accountID, 1, fn)
}

func (c *Core) useConn(ctx context.Context, accountID int64, attempts int, fn func(context.Context, *imapx.Conn) error) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()

	pc := c.pooled(accountID)
	pc.mu.Lock()
	defer pc.mu.Unlock()

	var (
		err        error
		freshLogin bool
	)
	for range attempts {
		var dialed bool
		dialed, err = c.runOnConn(ctx, pc, accountID, fn)
		freshLogin = freshLogin || dialed
		if err == nil || c.ctxErr(ctx) != nil || !errors.Is(err, imapx.ErrNetwork) {
			break
		}
	}
	if err == nil {
		// Si la contraseña guardada volvió a pasar un LOGIN, se levanta la
		// pausa y la sincronización periódica vuelve a incluir la cuenta. Que
		// ande una sesión abierta antes de que cambie la contraseña no prueba
		// nada: levantar la pausa ahí haría reintentar la contraseña mala.
		if freshLogin {
			c.clearAuthFailure(accountID)
		}
		return nil
	}
	err = c.ctxOrUserError(ctx, err)
	var ue *UserError
	if errors.As(err, &ue) && (ue.Code == "network" || ue.Code == "auth" || ue.Code == "tls") {
		if ue.Code == "auth" {
			c.noteAuthFailure(accountID)
		}
		c.emit(EventSyncError, map[string]any{"accountId": accountID, "message": ue.Msg})
	}
	return err
}

// runOnConn hace un intento: conecta si hace falta y corre fn. Si la conexión
// queda inservible (error de red o cancelación), la descarta. dialed dice si
// este intento hizo un LOGIN que el servidor aceptó.
func (c *Core) runOnConn(ctx context.Context, pc *pooledConn, accountID int64, fn func(context.Context, *imapx.Conn) error) (dialed bool, err error) {
	if pc.conn == nil {
		conn, err := c.dial(ctx, accountID)
		if err != nil {
			return false, err
		}
		pc.conn = conn
		dialed = true
	}
	conn := pc.conn
	stop := context.AfterFunc(ctx, func() { _ = conn.Abort() })
	err = fn(ctx, conn)
	if aborted := !stop(); aborted || errors.Is(err, imapx.ErrNetwork) {
		_ = conn.Abort()
		pc.conn = nil
	}
	return dialed, err
}

func (c *Core) dial(ctx context.Context, accountID int64) (*imapx.Conn, error) {
	acc, user, password, err := c.credentials(ctx, accountID)
	if err != nil {
		return nil, err
	}
	return imapx.Dial(ctx, acc.IMAP, user, password, c.cfg.TLS)
}

// credentials devuelve la cuenta, el usuario IMAP y la contraseña del
// llavero. Si la contraseña no está, el error es un UserError "auth".
func (c *Core) credentials(ctx context.Context, accountID int64) (model.Account, string, string, error) {
	acc, err := c.store.GetAccount(ctx, accountID)
	if err != nil {
		return model.Account{}, "", "", err
	}
	password, err := c.cfg.Secrets.Get(acc.Email)
	if errors.Is(err, secrets.ErrNotFound) {
		return model.Account{}, "", "", &UserError{Code: "auth", Msg: "No encontramos la contraseña de la cuenta en el llavero del sistema.", Err: err}
	}
	if err != nil {
		return model.Account{}, "", "", err
	}
	user := acc.Username
	if user == "" {
		user = acc.Email
	}
	return acc, user, password, nil
}

// dropConn cierra y olvida la conexión de una cuenta (por ejemplo, al borrarla).
func (c *Core) dropConn(accountID int64) {
	c.poolMu.Lock()
	pc := c.pool[accountID]
	delete(c.pool, accountID)
	c.poolMu.Unlock()
	if pc != nil {
		pc.close()
	}
}

// closeConns cierra todas las conexiones del pool.
func (c *Core) closeConns() {
	c.poolMu.Lock()
	pcs := make([]*pooledConn, 0, len(c.pool))
	for _, pc := range c.pool {
		pcs = append(pcs, pc)
	}
	c.poolMu.Unlock()
	for _, pc := range pcs {
		pc.close()
	}
}

// close corta la conexión sin LOGOUT: con la red caída, LOGOUT puede quedar
// esperando respuesta y demorar el cierre de la app.
func (pc *pooledConn) close() {
	pc.mu.Lock()
	defer pc.mu.Unlock()
	if pc.conn != nil {
		_ = pc.conn.Abort()
		pc.conn = nil
	}
}

// Package mail es la fachada del núcleo: orquesta store, secrets, autoconfig,
// imapx y smtpx sin depender de Wails.
package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"sync"

	"postacontrol/core/autoconfig"
	"postacontrol/core/imapx"
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

	// Ciclo de vida: las goroutines propias (sincronización inicial y
	// periódica) usan ctx y se registran en wg. Close cancela y espera.
	ctx       context.Context
	cancel    context.CancelFunc
	lifeMu    sync.Mutex
	closed    bool
	wg        sync.WaitGroup
	closeOnce sync.Once
	closeErr  error

	poolMu sync.Mutex
	pool   map[int64]*pooledConn // una conexión IMAP por cuenta
}

func New(cfg Config) (*Core, error) {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Core{cfg: cfg, store: st, ctx: ctx, cancel: cancel, pool: map[int64]*pooledConn{}}, nil
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
// de Close no lanza nada.
func (c *Core) spawn(fn func(ctx context.Context)) {
	c.lifeMu.Lock()
	defer c.lifeMu.Unlock()
	if c.closed {
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		fn(c.ctx)
	}()
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
// (context.Canceled). Los demás errores salen como UserError.
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

	var err error
	for range attempts {
		err = c.runOnConn(ctx, pc, accountID, fn)
		if err == nil || c.ctxErr(ctx) != nil || !errors.Is(err, imapx.ErrNetwork) {
			break
		}
	}
	if err == nil {
		return nil
	}
	err = c.ctxOrUserError(ctx, err)
	var ue *UserError
	if errors.As(err, &ue) && (ue.Code == "network" || ue.Code == "auth" || ue.Code == "tls") {
		c.emit(EventSyncError, map[string]any{"accountId": accountID, "message": ue.Msg})
	}
	return err
}

// runOnConn hace un intento: conecta si hace falta y corre fn. Si la conexión
// queda inservible (error de red o cancelación), la descarta.
func (c *Core) runOnConn(ctx context.Context, pc *pooledConn, accountID int64, fn func(context.Context, *imapx.Conn) error) error {
	if pc.conn == nil {
		conn, err := c.dial(ctx, accountID)
		if err != nil {
			return err
		}
		pc.conn = conn
	}
	conn := pc.conn
	stop := context.AfterFunc(ctx, func() { _ = conn.Abort() })
	err := fn(ctx, conn)
	if aborted := !stop(); aborted || errors.Is(err, imapx.ErrNetwork) {
		_ = conn.Abort()
		pc.conn = nil
	}
	return err
}

func (c *Core) dial(ctx context.Context, accountID int64) (*imapx.Conn, error) {
	acc, err := c.store.GetAccount(ctx, accountID)
	if err != nil {
		return nil, err
	}
	password, err := c.cfg.Secrets.Get(acc.Email)
	if errors.Is(err, secrets.ErrNotFound) {
		return nil, &UserError{Code: "auth", Msg: "No encontramos la contraseña de la cuenta en el llavero del sistema.", Err: err}
	}
	if err != nil {
		return nil, err
	}
	user := acc.Username
	if user == "" {
		user = acc.Email
	}
	return imapx.Dial(ctx, acc.IMAP, user, password, c.cfg.TLS)
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

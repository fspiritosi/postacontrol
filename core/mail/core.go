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

// withConn corre fn con la conexión de la cuenta (la abre si hace falta). Si
// fn falla por un error de red, cierra la conexión, vuelve a conectar una vez
// y reintenta. Si la conexión no se puede usar (red, credenciales o TLS) emite
// EventSyncError. Close o la cancelación de ctx cortan la conexión en curso;
// en ese caso no se reintenta ni se emite nada. Los errores salen como UserError.
func (c *Core) withConn(ctx context.Context, accountID int64, fn func(*imapx.Conn) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(c.ctx, cancel)()

	pc := c.pooled(accountID)
	pc.mu.Lock()
	defer pc.mu.Unlock()

	var err error
	for range 2 {
		err = c.runOnConn(ctx, pc, accountID, fn)
		if err == nil || ctx.Err() != nil || !errors.Is(err, imapx.ErrNetwork) {
			break
		}
	}
	err = asUserError(err)
	if err == nil || ctx.Err() != nil {
		return err
	}
	var ue *UserError
	if errors.As(err, &ue) && (ue.Code == "network" || ue.Code == "auth" || ue.Code == "tls") {
		c.emit(EventSyncError, map[string]any{"accountId": accountID, "message": ue.Msg})
	}
	return err
}

// runOnConn hace un intento: conecta si hace falta y corre fn. Si la conexión
// queda inservible (error de red o cancelación), la descarta.
func (c *Core) runOnConn(ctx context.Context, pc *pooledConn, accountID int64, fn func(*imapx.Conn) error) error {
	if pc.conn == nil {
		conn, err := c.dial(ctx, accountID)
		if err != nil {
			return err
		}
		pc.conn = conn
	}
	conn := pc.conn
	stop := context.AfterFunc(ctx, func() { _ = conn.Abort() })
	err := fn(conn)
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

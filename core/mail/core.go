// Package mail es la fachada del núcleo: orquesta store, secrets, autoconfig,
// imapx y smtpx sin depender de Wails.
package mail

import (
	"context"
	"crypto/tls"

	"postacontrol/core/autoconfig"
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
}

func New(cfg Config) (*Core, error) {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return &Core{cfg: cfg, store: st}, nil
}

func (c *Core) Close() error { return c.store.Close() }

func (c *Core) emit(event string, data any) {
	if c.cfg.Notifier != nil {
		c.cfg.Notifier.Emit(event, data)
	}
}

// DetectSettings detecta los servidores a partir del correo.
func (c *Core) DetectSettings(ctx context.Context, email string) (autoconfig.Settings, error) {
	return autoconfig.Detect(ctx, email, c.cfg.Autoconfig)
}

package mail

import (
	"context"
	"time"

	"postacontrol/core/model"
)

// authPause es cuánto espera la sincronización periódica antes de volver a
// probar una cuenta cuyo servidor rechazó la contraseña: reintentar cada pocos
// minutos con una contraseña mala puede hacer que el proveedor bloquee la cuenta.
const authPause = 30 * time.Minute

func (c *Core) noteAuthFailure(accountID int64) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	c.authFailedAt[accountID] = c.now()
}

func (c *Core) clearAuthFailure(accountID int64) {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	delete(c.authFailedAt, accountID)
}

// authPaused dice si la sincronización periódica debe saltear la cuenta.
func (c *Core) authPaused(accountID int64) bool {
	c.authMu.Lock()
	defer c.authMu.Unlock()
	at, ok := c.authFailedAt[accountID]
	return ok && c.now().Sub(at) < authPause
}

// UpdatePassword prueba la contraseña nueva contra los servidores guardados
// de la cuenta (IMAP y SMTP). Solo si pasa la guarda en el llavero, descarta
// la conexión del pool, levanta la pausa y sincroniza en segundo plano. Si la
// prueba falla no cambia nada.
func (c *Core) UpdatePassword(ctx context.Context, accountID int64, password string) error {
	if password == "" {
		return &UserError{Code: "invalid", Msg: "Ingresá la contraseña."}
	}
	acc, err := c.store.GetAccount(ctx, accountID)
	if err != nil {
		return asUserError(err)
	}
	if err := c.TestAccount(ctx, model.AccountInput{Account: acc, Password: password}); err != nil {
		return err
	}
	if err := c.cfg.Secrets.Set(acc.Email, password); err != nil {
		return asUserError(err)
	}
	c.dropConn(accountID)
	c.clearAuthFailure(accountID)
	if !c.cfg.SkipInitialSync {
		c.spawn(func(ctx context.Context) { _ = c.SyncAccount(ctx, accountID) })
	}
	return nil
}

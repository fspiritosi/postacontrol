package imapx

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync/atomic"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"postacontrol/core/model"
)

// DialWatch es como Dial, pero registra onChange: se llama cuando el servidor
// avisa que cambió la cantidad de mensajes de la carpeta (EXISTS) o que se
// expurgó uno (EXPUNGE). Solo avisa mientras IdleWatch está en IDLE, no por
// el SELECT inicial.
//
// onChange corre en la goroutine lectora de go-imap: no debe bloquear (por
// ejemplo, hacer un envío no bloqueante a un canal con buffer).
func DialWatch(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config, onChange func()) (*Conn, error) {
	var live atomic.Bool
	notify := func() {
		if live.Load() && onChange != nil {
			onChange()
		}
	}
	h := &imapclient.UnilateralDataHandler{
		Mailbox: func(d *imapclient.UnilateralDataMailbox) {
			if d.NumMessages != nil {
				notify()
			}
		},
		Expunge: func(uint32) { notify() },
	}
	c, err := dial(ctx, s, username, password, tlsCfg, h)
	if err != nil {
		return nil, err
	}
	c.live = &live
	return c, nil
}

// IdleWatch abre mailbox en solo lectura y queda en IDLE. Bloquea hasta que
// ctx se cancela (cierra el IDLE y devuelve ctx.Err()) o la conexión se corta
// (devuelve un error que envuelve ErrNetwork). Si el servidor no anuncia
// IDLE devuelve ErrUnsupported sin mandar nada.
func (c *Conn) IdleWatch(ctx context.Context, mailbox string) error {
	if !c.c.Caps().Has(imap.CapIdle) {
		return fmt.Errorf("%w: IDLE", ErrUnsupported)
	}
	if c.live != nil {
		c.live.Store(false)
	}
	if _, err := c.Select(mailbox); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd, err := c.c.Idle()
	if err != nil {
		return opErr("idle", err)
	}
	if c.live != nil {
		c.live.Store(true)
		defer c.live.Store(false)
	}
	select {
	case <-ctx.Done():
		_ = cmd.Close()
		_ = cmd.Wait()
		return ctx.Err()
	case <-c.c.Closed():
		_ = cmd.Close()
		return fmt.Errorf("imapx: idle: %w: conexión cerrada", ErrNetwork)
	}
}

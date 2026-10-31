package imapx

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync/atomic"
	"time"

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
// ctx se cancela (cierra el IDLE y devuelve ctx.Err()) o la conexión o el IDLE
// se cortan (devuelve un error que envuelve ErrNetwork, incluso si go-imap no
// logra reiniciar el IDLE o el servidor lo termina). Si el servidor no
// anuncia IDLE devuelve ErrUnsupported sin mandar nada.
//
// Hay una ventana entre el SELECT y el arranque del IDLE en la que un cambio
// no genera aviso: quien llama debe sincronizar la carpeta justo después de
// arrancar IdleWatch y después de cada reconexión. Solo se admite un
// IdleWatch a la vez por Conn.
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
	// El EXISTS del SELECT ya se consumió; activar los avisos antes de Idle()
	// para no perder los que lleguen justo tras la continuación.
	if c.live != nil {
		c.live.Store(true)
		defer c.live.Store(false)
	}
	cmd, err := c.c.Idle()
	if err != nil {
		return opErr("idle", err)
	}
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		_ = cmd.Close()
		select {
		case <-waited:
		case <-time.After(idleCloseTimeout):
			_ = c.c.Close()
		}
		return ctx.Err()
	case <-c.c.Closed():
		_ = cmd.Close()
		return fmt.Errorf("imapx: idle: %w: conexión cerrada%s", ErrNetwork, causeSuffix(<-waited))
	case err := <-waited:
		// IDLE terminó sin que lo pidiéramos: reiniciar falló o el servidor lo cerró.
		if err == nil {
			return fmt.Errorf("imapx: idle: %w: el servidor terminó el IDLE", ErrNetwork)
		}
		return opErr("idle", err)
	}
}

const idleCloseTimeout = 5 * time.Second

func causeSuffix(err error) string {
	if err == nil {
		return ""
	}
	return ": " + err.Error()
}

// Package smtpx envía correo por SMTP con autenticación y TLS, y traduce los
// errores a variables exportadas.
package smtpx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"postacontrol/core/internal/neterr"
	"postacontrol/core/model"
)

var (
	// ErrAuth: el servidor rechazó las credenciales.
	ErrAuth = errors.New("smtpx: credenciales rechazadas")
	// ErrNetwork: no se pudo conectar, o la conexión expiró o se cortó.
	ErrNetwork = errors.New("smtpx: error de red")
	// ErrTLS: falló la negociación o la verificación del certificado.
	ErrTLS = errors.New("smtpx: error de TLS")
	// ErrRejected: el servidor rechazó un remitente, un destinatario o el DATA.
	// El error envuelto incluye el mensaje del servidor.
	ErrRejected = errors.New("smtpx: envío rechazado")
)

const dialTimeout = 30 * time.Second

// Send entrega raw (mensaje RFC 5322 completo) a rcpts usando from como
// remitente del sobre. tlsCfg nil usa las raíces del sistema; los certificados
// siempre se verifican. Los errores envuelven ErrAuth, ErrNetwork, ErrTLS o
// ErrRejected.
func Send(ctx context.Context, s model.ServerSettings, username, password, from string, rcpts []string, raw []byte, tlsCfg *tls.Config) error {
	c, stop, err := connect(ctx, s, username, password, tlsCfg)
	if err != nil {
		return err
	}
	defer stop()
	defer c.Close()

	if err := c.Mail(from, nil); err != nil {
		return classifySession(ctx, err)
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r, nil); err != nil {
			return classifySession(ctx, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return classifySession(ctx, err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return classifySession(ctx, err)
	}
	if err := w.Close(); err != nil {
		return classifySession(ctx, err)
	}
	_ = c.Quit() // el mensaje ya fue aceptado; un fallo al cerrar no importa
	return nil
}

// Check conecta, se autentica y cierra, sin enviar nada.
func Check(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config) error {
	c, stop, err := connect(ctx, s, username, password, tlsCfg)
	if err != nil {
		return err
	}
	defer stop()
	defer c.Close()
	_ = c.Quit()
	return nil
}

// connect abre la conexión (TLS implícito o STARTTLS) y hace AUTH PLAIN. Si
// ctx termina durante la sesión, se cierra la conexión. stop libera ese
// vigilante y hay que llamarlo al terminar.
func connect(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config) (*smtp.Client, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrNetwork, err)
	}
	cfg := &tls.Config{}
	if tlsCfg != nil {
		cfg = tlsCfg.Clone()
	}
	if cfg.ServerName == "" {
		cfg.ServerName = s.Host
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	d := &net.Dialer{Timeout: dialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, nil, classifyDial(ctx, err)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	fail := func(err error) (*smtp.Client, func(), error) {
		stop()
		_ = conn.Close()
		return nil, nil, classifyDial(ctx, err)
	}

	var c *smtp.Client
	switch s.Security {
	case model.SecurityTLS:
		tc := tls.Client(conn, cfg)
		if err := tc.HandshakeContext(ctx); err != nil {
			return fail(err)
		}
		c = smtp.NewClient(tc)
	case model.SecuritySTARTTLS:
		c, err = smtp.NewClientStartTLS(conn, cfg)
		if err != nil {
			if strings.Contains(err.Error(), "doesn't support STARTTLS") {
				stop()
				_ = conn.Close()
				return nil, nil, fmt.Errorf("%w: %w", ErrTLS, err)
			}
			return fail(err)
		}
	default:
		return fail(fmt.Errorf("seguridad desconocida %q", s.Security))
	}

	if err := c.Auth(sasl.NewPlainClient("", username, password)); err != nil {
		_ = c.Close()
		stop()
		if ctx.Err() != nil {
			return nil, nil, fmt.Errorf("%w: %w", ErrNetwork, ctx.Err())
		}
		return nil, nil, classifyAuth(ctx, err)
	}
	return c, func() { stop() }, nil
}

func classifyDial(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ErrNetwork, ctx.Err())
	}
	if neterr.IsTLSError(err) {
		return fmt.Errorf("%w: %w", ErrTLS, err)
	}
	return fmt.Errorf("%w: %w", ErrNetwork, err)
}

// classifySession traduce un error de MAIL/RCPT/DATA: respuestas del servidor
// son ErrRejected; lo demás es de red.
func classifySession(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("%w: %w", ErrNetwork, ctx.Err())
	}
	var serr *smtp.SMTPError
	if errors.As(err, &serr) {
		return fmt.Errorf("%w: %w", ErrRejected, err)
	}
	return fmt.Errorf("%w: %w", ErrNetwork, err)
}

// classifyAuth traduce un error de AUTH: solo una respuesta 5xx significa
// credenciales rechazadas; una 4xx es un problema temporal del servidor.
func classifyAuth(ctx context.Context, err error) error {
	if ctx.Err() == nil {
		var serr *smtp.SMTPError
		if errors.As(err, &serr) && serr.Code >= 500 {
			return fmt.Errorf("%w: %w", ErrAuth, err)
		}
	}
	return classifyDial(ctx, err)
}

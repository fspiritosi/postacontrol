// Package imapx envuelve go-imap con las operaciones que necesita el núcleo
// de correo y traduce los errores de conexión a variables exportadas.
package imapx

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"mime"
	"net"
	"strconv"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
	"github.com/emersion/go-message/charset"

	"postacontrol/core/internal/neterr"
	"postacontrol/core/model"
)

var (
	// ErrAuth: el servidor rechazó las credenciales.
	ErrAuth = errors.New("imapx: credenciales rechazadas")
	// ErrNetwork: no se pudo conectar, o la conexión expiró o se cortó.
	ErrNetwork = errors.New("imapx: error de red")
	// ErrTLS: falló la negociación o la verificación del certificado.
	ErrTLS = errors.New("imapx: error de TLS")
)

const dialTimeout = 30 * time.Second

// Conn es una sesión IMAP autenticada.
type Conn struct {
	c  *imapclient.Client
	rw bool // la carpeta seleccionada se abrió con SelectRW
}

// Dial conecta (TLS implícito o STARTTLS según s.Security) y hace LOGIN.
// tlsCfg nil usa las raíces del sistema; los certificados siempre se verifican.
// Los errores envuelven ErrAuth, ErrNetwork o ErrTLS.
func Dial(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config) (*Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNetwork, err)
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	opts := &imapclient.Options{
		TLSConfig: cloneTLS(tlsCfg),
		Dialer:    &net.Dialer{Timeout: dialTimeout},
		// Subject y nombres del ENVELOPE llegan como palabras RFC 2047; sin
		// esto, los charsets legacy (windows-1252, etc.) quedan sin decodificar.
		WordDecoder: &mime.WordDecoder{CharsetReader: charset.Reader},
	}

	type result struct {
		c   *imapclient.Client
		err error
	}
	ch := make(chan result, 1)
	go func() {
		var (
			c   *imapclient.Client
			err error
		)
		switch s.Security {
		case model.SecurityTLS:
			c, err = imapclient.DialTLS(addr, opts)
		case model.SecuritySTARTTLS:
			c, err = imapclient.DialStartTLS(addr, opts)
		default:
			err = fmt.Errorf("seguridad desconocida %q", s.Security)
		}
		ch <- result{c, err}
	}()

	var res result
	select {
	case res = <-ch:
	case <-ctx.Done():
		go func() { // cerrar la conexión que llegue tarde
			if r := <-ch; r.c != nil {
				_ = r.c.Close()
			}
		}()
		return nil, fmt.Errorf("%w: %w", ErrNetwork, ctx.Err())
	}
	if res.err != nil {
		return nil, classifyDial(res.err)
	}
	// Si ctx termina durante el LOGIN, cerrar el cliente destraba el Wait.
	stop := context.AfterFunc(ctx, func() { _ = res.c.Close() })
	err := res.c.Login(username, password).Wait()
	stop()
	if err != nil {
		_ = res.c.Close()
		if ctx.Err() != nil {
			return nil, fmt.Errorf("%w: %w", ErrNetwork, ctx.Err())
		}
		var ierr *imap.Error
		if errors.As(err, &ierr) {
			return nil, fmt.Errorf("%w: %w", ErrAuth, err)
		}
		return nil, classifyDial(err)
	}
	return &Conn{c: res.c}, nil
}

func cloneTLS(c *tls.Config) *tls.Config {
	if c == nil {
		return nil
	}
	return c.Clone()
}

func classifyDial(err error) error {
	if neterr.IsTLSError(err) {
		return fmt.Errorf("%w: %w", ErrTLS, err)
	}
	return fmt.Errorf("%w: %w", ErrNetwork, err)
}

// Close cierra la sesión (LOGOUT best-effort) y la conexión.
func (c *Conn) Close() error {
	_ = c.c.Logout().Wait()
	return c.c.Close()
}

// opErr envuelve el error de un comando. Las respuestas NO/BAD del servidor
// (*imap.Error) quedan tal cual; cualquier otro error (conexión cortada,
// timeout, respuesta ilegible) deja la sesión inservible y envuelve ErrNetwork.
func opErr(op string, err error) error {
	var ierr *imap.Error
	if errors.As(err, &ierr) {
		return fmt.Errorf("imapx: %s: %w", op, err)
	}
	return fmt.Errorf("imapx: %s: %w: %w", op, ErrNetwork, err)
}

func toUIDs(uids []uint32) imap.UIDSet {
	set := make(imap.UIDSet, 0, len(uids))
	for _, u := range uids {
		set.AddNum(imap.UID(u))
	}
	return set
}

// Abort cierra la conexión de inmediato, sin LOGOUT. Destraba los comandos en
// curso, que terminan con ErrNetwork.
func (c *Conn) Abort() error { return c.c.Close() }

// Package imapx envuelve go-imap con las operaciones que necesita el núcleo
// de correo y traduce los errores de conexión a variables exportadas.
package imapx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

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
	c *imapclient.Client
}

// Dial conecta (TLS implícito o STARTTLS según s.Security) y hace LOGIN.
// tlsCfg nil usa las raíces del sistema; los certificados siempre se verifican.
// Los errores envuelven ErrAuth, ErrNetwork o ErrTLS.
func Dial(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config) (*Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNetwork, err)
	}
	addr := net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
	opts := &imapclient.Options{TLSConfig: cloneTLS(tlsCfg), Dialer: &net.Dialer{Timeout: dialTimeout}}

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
	if err := res.c.Login(username, password).Wait(); err != nil {
		_ = res.c.Close()
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
	if isTLSError(err) {
		return fmt.Errorf("%w: %w", ErrTLS, err)
	}
	return fmt.Errorf("%w: %w", ErrNetwork, err)
}

func isTLSError(err error) bool {
	var (
		certInvalid  x509.CertificateInvalidError
		unknownAuth  x509.UnknownAuthorityError
		hostErr      x509.HostnameError
		verifyErr    *tls.CertificateVerificationError
		alertErr     tls.AlertError
		recordHdrErr tls.RecordHeaderError
	)
	if errors.As(err, &certInvalid) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) ||
		errors.As(err, &verifyErr) || errors.As(err, &alertErr) || errors.As(err, &recordHdrErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:")
}

// Close cierra la sesión (LOGOUT best-effort) y la conexión.
func (c *Conn) Close() error {
	_ = c.c.Logout().Wait()
	return c.c.Close()
}

func toUIDs(uids []uint32) imap.UIDSet {
	set := make(imap.UIDSet, 0, len(uids))
	for _, u := range uids {
		set.AddNum(imap.UID(u))
	}
	return set
}

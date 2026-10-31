package mail

import (
	"errors"
	"fmt"
	"strings"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
	"postacontrol/core/smtpx"
	"postacontrol/core/store"
)

// UserError es un error pensado para mostrarse al usuario: Code es estable
// (lo usa el frontend) y Msg está en español.
type UserError struct {
	Code string
	Msg  string
	Err  error
}

func (e *UserError) Error() string { return e.Msg }
func (e *UserError) Unwrap() error { return e.Err }

const msgNotFound = "No encontramos lo que buscabas."

const smtpxRejectedPrefix = "smtpx: envío rechazado: "

const msgServerRefused = "El servidor rechazó la operación"

// asUserError traduce los errores de imapx, smtpx y store a *UserError. Un
// NO/BAD del servidor IMAP que no tiene una traducción más específica sale
// con Code "server" y el texto del servidor (sin el prefijo técnico). Los
// errores que no reconoce (incluido autoconfig.ErrNotDetected, que ya tiene su
// propio mensaje en español) se devuelven tal cual. Nunca incluye contraseñas.
func asUserError(err error) error {
	if err == nil {
		return nil
	}
	var ue *UserError
	if errors.As(err, &ue) {
		return err
	}
	switch {
	case errors.Is(err, imapx.ErrAuth), errors.Is(err, smtpx.ErrAuth):
		return &UserError{Code: "auth", Msg: "El servidor rechazó el usuario o la contraseña.", Err: err}
	case errors.Is(err, imapx.ErrTLS), errors.Is(err, smtpx.ErrTLS):
		return &UserError{Code: "tls", Msg: "El certificado del servidor no es válido. Por seguridad no nos conectamos.", Err: err}
	case errors.Is(err, imapx.ErrNetwork), errors.Is(err, smtpx.ErrNetwork):
		return &UserError{Code: "network", Msg: "No pudimos conectarnos al servidor. Revisá el host, el puerto y tu conexión.", Err: err}
	case errors.Is(err, smtpx.ErrRejected):
		detail := strings.TrimPrefix(err.Error(), smtpxRejectedPrefix)
		return &UserError{Code: "rejected", Msg: fmt.Sprintf("El servidor rechazó el envío: %s", detail), Err: err}
	case errors.Is(err, imapx.ErrUnsupported):
		return &UserError{Code: "unsupported", Msg: "El servidor no permite mover ni borrar correos de forma segura.", Err: err}
	case errors.Is(err, store.ErrNotFound), errors.Is(err, imapx.ErrNoMessage):
		return &UserError{Code: "not_found", Msg: msgNotFound, Err: err}
	}
	var ierr *goimap.Error
	if errors.As(err, &ierr) {
		msg := msgServerRefused + "."
		if text := strings.TrimRight(strings.TrimSpace(ierr.Text), "."); text != "" {
			msg = fmt.Sprintf("%s: %s.", msgServerRefused, text)
		}
		return &UserError{Code: "server", Msg: msg, Err: err}
	}
	return err
}

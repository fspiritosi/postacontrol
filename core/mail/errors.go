package mail

import (
	"errors"
	"fmt"
	"strings"

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

// asUserError traduce los errores de imapx, smtpx y store a *UserError. Los
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
	case errors.Is(err, store.ErrNotFound), errors.Is(err, imapx.ErrNoMessage):
		return &UserError{Code: "not_found", Msg: msgNotFound, Err: err}
	}
	return err
}

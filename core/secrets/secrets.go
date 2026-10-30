// Package secrets guarda las contraseñas de las cuentas en el llavero del
// sistema. Nunca se persisten en SQLite, logs ni eventos.
package secrets

import "errors"

// ErrNotFound indica que el llavero no tiene contraseña para ese correo.
var ErrNotFound = errors.New("contraseña no encontrada en el llavero")

// Store guarda una contraseña por correo.
type Store interface {
	Set(email, password string) error
	Get(email string) (string, error) // ErrNotFound si no existe
	Delete(email string) error        // si no existe, no es error
}

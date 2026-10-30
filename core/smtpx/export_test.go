package smtpx

import "time"

// SetHandshakeTimeout cambia el plazo de conexión para un test y devuelve cómo restaurarlo.
func SetHandshakeTimeout(d time.Duration) (restore func()) {
	old := handshakeTimeout
	handshakeTimeout = d
	return func() { handshakeTimeout = old }
}

// LocalName expone el nombre que se usa en EHLO.
var LocalName = localName

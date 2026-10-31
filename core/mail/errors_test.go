package mail

import (
	"errors"
	"fmt"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"

	"postacontrol/core/autoconfig"
	"postacontrol/core/imapx"
	"postacontrol/core/smtpx"
	"postacontrol/core/store"
)

func TestAsUserError(t *testing.T) {
	err := asUserError(fmt.Errorf("%w: %w", smtpx.ErrRejected, fmt.Errorf("550 sin permiso")))
	ue, ok := err.(*UserError)
	assert.True(t, ok)
	assert.Equal(t, "rejected", ue.Code)
	assert.Equal(t, "El servidor rechazó el envío: 550 sin permiso", ue.Msg)

	assert.Equal(t, "not_found", asUserError(store.ErrNotFound).(*UserError).Code)
	unsup := asUserError(fmt.Errorf("mover: %w", imapx.ErrUnsupported)).(*UserError)
	assert.Equal(t, "unsupported", unsup.Code)
	assert.Equal(t, "El servidor no permite mover ni borrar correos de forma segura.", unsup.Msg)
	assert.Equal(t, autoconfig.ErrNotDetected, asUserError(autoconfig.ErrNotDetected))
	assert.NoError(t, asUserError(nil))
}

func TestSeenIsBestEffort(t *testing.T) {
	refusal := fmt.Errorf("imapx: UID STORE: %w", &goimap.Error{Type: goimap.StatusResponseTypeNo, Text: "no permitido"})
	assert.True(t, seenIsBestEffort(refusal), "NO del servidor")
	assert.True(t, seenIsBestEffort(&goimap.Error{Type: goimap.StatusResponseTypeBad, Text: "x"}), "BAD del servidor")
	assert.True(t, seenIsBestEffort(&UserError{Code: "network"}))
	assert.True(t, seenIsBestEffort(&UserError{Code: "auth"}))
	assert.True(t, seenIsBestEffort(&UserError{Code: "tls"}))

	assert.False(t, seenIsBestEffort(&UserError{Code: "stale"}), "UIDVALIDITY cambió: se avisa")
	assert.False(t, seenIsBestEffort(errors.New("sql: database is locked")), "error de caché")
	assert.False(t, seenIsBestEffort(&UserError{Code: "not_found"}))
}

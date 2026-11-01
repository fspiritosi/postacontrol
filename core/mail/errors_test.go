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

func TestAsUserError_ServerRefusal(t *testing.T) {
	refusal := fmt.Errorf("imapx: UID MOVE %q: %w", "Archivo",
		&goimap.Error{Type: goimap.StatusResponseTypeNo, Code: "OVERQUOTA", Text: "Quota exceeded (mailbox for user is full)"})
	ue, ok := asUserError(refusal).(*UserError)
	assert.True(t, ok)
	assert.Equal(t, "server", ue.Code)
	assert.Equal(t, "El servidor rechazó la operación: Quota exceeded (mailbox for user is full).", ue.Msg)
	assert.ErrorIs(t, ue, refusal)

	bad := asUserError(&goimap.Error{Type: goimap.StatusResponseTypeBad, Text: "Error in IMAP command."}).(*UserError)
	assert.Equal(t, "El servidor rechazó la operación: Error in IMAP command.", bad.Msg)
	empty := asUserError(&goimap.Error{Type: goimap.StatusResponseTypeNo}).(*UserError)
	assert.Equal(t, "El servidor rechazó la operación.", empty.Msg)

	// Un rechazo al login sigue siendo de credenciales.
	auth := asUserError(fmt.Errorf("%w: %w", imapx.ErrAuth, &goimap.Error{Type: goimap.StatusResponseTypeNo, Text: "x"})).(*UserError)
	assert.Equal(t, "auth", auth.Code)
}

func TestSeenIsBestEffort(t *testing.T) {
	refusal := fmt.Errorf("imapx: UID STORE: %w", &goimap.Error{Type: goimap.StatusResponseTypeNo, Text: "no permitido"})
	assert.True(t, seenIsBestEffort(refusal), "NO del servidor")
	assert.True(t, seenIsBestEffort(&goimap.Error{Type: goimap.StatusResponseTypeBad, Text: "x"}), "BAD del servidor")
	assert.True(t, seenIsBestEffort(asUserError(refusal)), "NO del servidor ya traducido por withConn")
	assert.True(t, seenIsBestEffort(&UserError{Code: "network"}))
	assert.True(t, seenIsBestEffort(&UserError{Code: "auth"}))
	assert.True(t, seenIsBestEffort(&UserError{Code: "tls"}))

	assert.False(t, seenIsBestEffort(&UserError{Code: "stale"}), "UIDVALIDITY cambió: se avisa")
	assert.False(t, seenIsBestEffort(errors.New("sql: database is locked")), "error de caché")
	assert.False(t, seenIsBestEffort(&UserError{Code: "not_found"}))
}

// Contrato con el frontend: NOT_FOUND_MESSAGE en frontend/src/lib/errors.ts
// compara este texto exacto (el autoguardado deja de guardar un borrador que
// ya no existe).
func TestMsgNotFound_MatchesFrontend(t *testing.T) {
	assert.Equal(t, "No encontramos lo que buscabas.", msgNotFound)
	ue := asUserError(store.ErrNotFound).(*UserError)
	assert.Equal(t, "No encontramos lo que buscabas.", ue.Msg)
}

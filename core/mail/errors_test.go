package mail

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"postacontrol/core/autoconfig"
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
	assert.Equal(t, autoconfig.ErrNotDetected, asUserError(autoconfig.ErrNotDetected))
	assert.NoError(t, asUserError(nil))
}

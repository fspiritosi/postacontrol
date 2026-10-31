package mail

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestSetSettings_PersistsAndNotifies(t *testing.T) {
	c, _, _, n := newTestCore(t)
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{UnifiedInbox: true}))
	got, err := c.GetSettings(context.Background())
	require.NoError(t, err)
	assert.True(t, got.UnifiedInbox)
	assert.Contains(t, n.names(), EventSettingsChanged)
}

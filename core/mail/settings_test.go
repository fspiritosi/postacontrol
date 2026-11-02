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

func TestSetSettings_ReadingPane(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{ReadingPane: "top"}))
	got, err := c.GetSettings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "top", got.ReadingPane)
}

func TestSetSettings_ReadingPaneInvalid(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{ReadingPane: "right"}))
	err := c.SetSettings(context.Background(), model.Settings{ReadingPane: "left"})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
	got, err := c.GetSettings(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "right", got.ReadingPane)
}

func TestSetSettings_KeepsBackground(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	require.NoError(t, c.SetBackground(ctx, writePNG(t, "a.png", 8)))
	before, err := c.GetSettings(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, before.Background)
	require.NoError(t, c.SetSettings(ctx, model.Settings{ReadingPane: "top"}))
	got, err := c.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, before.Background, got.Background)
	assert.Equal(t, "top", got.ReadingPane)
}

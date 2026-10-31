package mail

import (
	"context"

	"postacontrol/core/model"
)

// GetSettings devuelve la configuración de la app.
func (c *Core) GetSettings(ctx context.Context) (model.Settings, error) {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return model.Settings{}, c.ctxOrUserError(ctx, err)
	}
	return st, nil
}

// SetSettings guarda la configuración y avisa a la UI.
func (c *Core) SetSettings(ctx context.Context, st model.Settings) error {
	if err := c.store.SaveSettings(ctx, st); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventSettingsChanged, nil)
	return nil
}

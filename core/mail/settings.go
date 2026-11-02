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
	if err := c.setSettingsLocked(ctx, st); err != nil {
		return err
	}
	c.emit(EventSettingsChanged, nil)
	return nil
}

func (c *Core) setSettingsLocked(ctx context.Context, st model.Settings) error {
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	switch st.ReadingPane {
	case "", "right", "bottom", "top":
	default:
		return &UserError{Code: "invalid", Msg: "Posición del panel de lectura desconocida."}
	}
	// El fondo se cambia solo con SetBackground/ClearBackground: lo que llega
	// acá se ignora, así guardar otra opción no lo pisa.
	cur, err := c.store.GetSettings(ctx)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	st.Background = cur.Background
	if err := c.store.SaveSettings(ctx, st); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	return nil
}

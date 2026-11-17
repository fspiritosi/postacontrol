package mail

import (
	"context"
	"errors"
	"strings"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
)

// GetSettings devuelve la configuración de la app.
func (c *Core) GetSettings(ctx context.Context) (model.Settings, error) {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return model.Settings{}, c.ctxOrUserError(ctx, err)
	}
	return st, nil
}

// SetSettings guarda la configuración y avisa a la UI. Si cambia la
// dirección de la API de Taskapp, desconecta (el token de un host nunca se
// manda a otro); si cambia solo la de la web, olvida lo que había en memoria.
func (c *Core) SetSettings(ctx context.Context, st model.Settings) error {
	apiChanged, webChanged, err := c.setSettingsLocked(ctx, st)
	if err != nil {
		return err
	}
	c.emit(EventSettingsChanged, nil)
	switch {
	case apiChanged:
		return c.DisconnectTaskapp(ctx)
	case webChanged:
		c.forgetTaskapp()
		c.emit(EventTaskappChanged, nil)
	}
	return nil
}

// sameTaskappURL compara dos direcciones de Taskapp tal como se usan (sin
// barra final y vacía = la de siempre).
func sameTaskappURL(a, b, def string) bool {
	norm := func(s string) string {
		s = strings.TrimRight(s, "/")
		if s == "" {
			return def
		}
		return s
	}
	return norm(a) == norm(b)
}

func (c *Core) setSettingsLocked(ctx context.Context, st model.Settings) (apiChanged, webChanged bool, err error) {
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	switch st.ReadingPane {
	case "", "right", "bottom", "top":
	default:
		return false, false, &UserError{Code: "invalid", Msg: "Posición del panel de lectura desconocida."}
	}
	switch st.Theme {
	case "", "light", "dark", "system":
	default:
		return false, false, &UserError{Code: "invalid", Msg: "Tema desconocido."}
	}
	if st.TaskReminderMinutes != nil && !validRemindMinutes(*st.TaskReminderMinutes) {
		return false, false, &UserError{Code: "invalid", Msg: msgBadReminder}
	}
	if !validTaskappURL(st.TaskappAPIURL) || !validTaskappURL(st.TaskappWebURL) {
		return false, false, &UserError{Code: "invalid", Msg: msgBadTaskappURL}
	}
	// El fondo se cambia solo con SetBackground/ClearBackground: lo que llega
	// acá se ignora, así guardar otra opción no lo pisa.
	cur, err := c.store.GetSettings(ctx)
	if err != nil {
		return false, false, c.ctxOrUserError(ctx, err)
	}
	st.Background = cur.Background
	st.AutoBackupDir = cur.AutoBackupDir
	apiChanged = !sameTaskappURL(cur.TaskappAPIURL, st.TaskappAPIURL, defaultTaskappAPI)
	webChanged = !sameTaskappURL(cur.TaskappWebURL, st.TaskappWebURL, defaultTaskappWeb)
	if apiChanged {
		// El token se borra antes de guardar la dirección nueva (y
		// taskappClient lee la dirección antes que el token): ningún pedido
		// manda el token viejo al host nuevo. Una conexión en curso es contra
		// el host viejo: se cancela.
		c.taskappMu.Lock()
		if c.connectCancel != nil {
			c.connectCancel()
		}
		c.taskappMu.Unlock()
		if err := c.cfg.Secrets.Delete(taskappSecret); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return false, false, err
		}
	}
	if err := c.store.SaveSettings(ctx, st); err != nil {
		return false, false, c.ctxOrUserError(ctx, err)
	}
	return apiChanged, webChanged, nil
}

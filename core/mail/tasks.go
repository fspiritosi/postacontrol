package mail

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"
	"time"

	"postacontrol/core/model"
)

const (
	msgTaskNoTitle    = "La tarea necesita un título."
	msgTaskTimeNoDate = "Para poner hora, elegí una fecha."
	msgTaskBadDate    = "Fecha u hora inválida."
	msgBadReminder    = "Anticipación del aviso inválida."
	msgTaskGone       = "Esa tarea ya no existe."
	defaultRemindMin  = 15
)

// Minutos de anticipación permitidos para el aviso de una tarea.
var remindMinutes = []int{0, 5, 15, 30, 60, 1440}

func validRemindMinutes(m int) bool { return slices.Contains(remindMinutes, m) }

// ListLocalTasks devuelve las tareas locales; con includeDone, también las hechas.
func (c *Core) ListLocalTasks(ctx context.Context, includeDone bool) ([]model.LocalTask, error) {
	list, err := c.store.ListLocalTasks(ctx, includeDone)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return list, nil
}

// SaveLocalTask crea la tarea (ID 0) o edita la existente. Al editar, si
// cambió la fecha, la hora o la anticipación, el aviso se vuelve a armar.
func (c *Core) SaveLocalTask(ctx context.Context, t model.LocalTask) (model.LocalTask, error) {
	rearm := false
	if t.ID != 0 {
		prev, err := c.store.GetLocalTask(ctx, t.ID)
		if errors.Is(err, sql.ErrNoRows) {
			return model.LocalTask{}, &UserError{Code: "not_found", Msg: msgTaskGone, Err: err}
		}
		if err != nil {
			return model.LocalTask{}, c.ctxOrUserError(ctx, err)
		}
		rearm = prev.DueDate != t.DueDate || prev.DueTime != t.DueTime || !sameMinutes(prev.RemindBefore, t.RemindBefore)
	}
	return c.saveLocalTask(ctx, t, rearm)
}

func sameMinutes(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func (c *Core) saveLocalTask(ctx context.Context, t model.LocalTask, rearm bool) (model.LocalTask, error) {
	t.Title = strings.TrimSpace(t.Title)
	if t.Title == "" {
		return model.LocalTask{}, &UserError{Code: "invalid", Msg: msgTaskNoTitle}
	}
	if t.DueTime != "" && t.DueDate == "" {
		return model.LocalTask{}, &UserError{Code: "invalid", Msg: msgTaskTimeNoDate}
	}
	if t.DueDate != "" {
		if _, err := time.Parse("2006-01-02", t.DueDate); err != nil {
			return model.LocalTask{}, &UserError{Code: "invalid", Msg: msgTaskBadDate}
		}
	}
	if t.DueTime != "" {
		if _, err := time.Parse("15:04", t.DueTime); err != nil {
			return model.LocalTask{}, &UserError{Code: "invalid", Msg: msgTaskBadDate}
		}
	}
	if t.RemindBefore != nil && !validRemindMinutes(*t.RemindBefore) {
		return model.LocalTask{}, &UserError{Code: "invalid", Msg: msgBadReminder}
	}
	saved, err := c.store.SaveLocalTask(ctx, t, rearm)
	if errors.Is(err, sql.ErrNoRows) {
		return model.LocalTask{}, &UserError{Code: "not_found", Msg: msgTaskGone, Err: err}
	}
	if err != nil {
		return model.LocalTask{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventTasksChanged, nil)
	return saved, nil
}

// SetLocalTaskDone marca la tarea como hecha ahora, o la reabre.
func (c *Core) SetLocalTaskDone(ctx context.Context, id int64, done bool) error {
	var at *time.Time
	if done {
		now := time.Now()
		at = &now
	}
	if err := c.store.SetLocalTaskDone(ctx, id, at); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventTasksChanged, nil)
	return nil
}

// DeleteLocalTask borra la tarea; si ya no estaba, no pasa nada.
func (c *Core) DeleteLocalTask(ctx context.Context, id int64) error {
	if err := c.store.DeleteLocalTask(ctx, id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventTasksChanged, nil)
	return nil
}

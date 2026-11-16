package mail

import (
	"context"
	"time"

	"postacontrol/core/model"
)

// Un aviso que quedó más de esto atrás (la app estuvo cerrada) ya no sirve:
// se marca como avisado sin mostrarlo.
const reminderStale = 24 * time.Hour

// dueMoment es la fecha y hora de la tarea en loc; false si no tiene hora o no
// se puede leer.
func dueMoment(t model.LocalTask, loc *time.Location) (time.Time, bool) {
	if t.DueDate == "" || t.DueTime == "" {
		return time.Time{}, false
	}
	due, err := time.ParseInLocation("2006-01-02 15:04", t.DueDate+" "+t.DueTime, loc)
	return due, err == nil
}

// remindAt es el momento del aviso: la fecha y hora de la tarea (en loc) menos
// su anticipación o, si no tiene, defaultMin. false si no hay hora o no se
// puede leer.
func remindAt(t model.LocalTask, defaultMin int, loc *time.Location) (time.Time, bool) {
	due, ok := dueMoment(t, loc)
	if !ok {
		return time.Time{}, false
	}
	min := defaultMin
	if t.RemindBefore != nil {
		min = *t.RemindBefore
	}
	return due.Add(-time.Duration(min) * time.Minute), true
}

// fireDueReminders avisa de las tareas con hora cuyo aviso llegó a now y las
// marca como avisadas.
func (c *Core) fireDueReminders(ctx context.Context, now time.Time) error {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	defMin := defaultRemindMin
	if st.TaskReminderMinutes != nil {
		defMin = *st.TaskReminderMinutes
	}
	pending, err := c.store.PendingReminders(ctx)
	if err != nil {
		return err
	}
	for _, t := range pending {
		if err := c.ctxErr(ctx); err != nil {
			return err
		}
		if err := c.fireReminder(ctx, t, defMin, now); err != nil {
			return err
		}
	}
	return nil
}

// fireReminder reclama el aviso de t (tal como estaba cuando se miró) y recién
// ahí lo emite: si la tarea se editó o se hizo mientras tanto, no se reclama,
// no se avisa con la hora vieja y el aviso rearmado queda pendiente.
func (c *Core) fireReminder(ctx context.Context, t model.LocalTask, defMin int, now time.Time) error {
	when, ok := remindAt(t, defMin, time.Local)
	if !ok || when.After(now) {
		return nil
	}
	claimed, err := c.store.MarkReminded(ctx, t, now)
	if err != nil || !claimed {
		return err
	}
	if now.Sub(when) <= reminderStale {
		due, _ := dueMoment(t, time.Local)
		c.emit(EventTaskReminder, model.TaskReminder{TaskID: t.ID, Title: t.Title, At: due})
	}
	return nil
}

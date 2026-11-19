package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"postacontrol/core/model"
	"postacontrol/core/taskapp"
)

const (
	// Ventana de ocurrencias que se miran: lo que empezó hace hasta 25 h (con
	// la anticipación ya vencida) y lo que empieza en las próximas 49 h (para
	// cubrir un día completo con un día de anticipación).
	eventRemindBack    = 25 * time.Hour
	eventRemindAhead   = 49 * time.Hour
	allDayRemindHour   = 9
	dayBeforeRemindMin = 1440
)

// eventRemindAt es el momento del aviso de una ocurrencia que empieza en
// start. En un día completo (la fecha es la de start en UTC) es a las 09:00 de
// loc de ese día, o del anterior si la anticipación es de un día; las demás
// anticipaciones no se aplican.
func eventRemindAt(start time.Time, allDay bool, lead int, loc *time.Location) time.Time {
	if !allDay {
		return start.Add(-time.Duration(lead) * time.Minute)
	}
	y, m, d := start.UTC().Date()
	at := time.Date(y, m, d, allDayRemindHour, 0, 0, 0, loc)
	if lead == dayBeforeRemindMin {
		at = time.Date(y, m, d-1, allDayRemindHour, 0, 0, 0, loc)
	}
	return at
}

// fireEventReminders avisa de los eventos y las reuniones de Taskapp cuyo
// aviso llegó a now y los marca como avisados. Los espejos de reuniones
// (Kind "taskapp") no avisan aparte: avisa la reunión.
func (c *Core) fireEventReminders(ctx context.Context, now time.Time) error {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	defLead := defaultRemindMin
	if st.EventReminderMinutes != nil {
		defLead = *st.EventReminderMinutes
	}
	from, to := now.Add(-eventRemindBack), now.Add(eventRemindAhead)
	evs, err := c.store.EventsInRange(ctx, from, to)
	if err != nil {
		return err
	}
	for _, e := range evs {
		if (e.Kind != "local" && e.Kind != "invite") || e.Status == "cancelled" || (e.Kind == "invite" && e.MyPartstat == "DECLINED") {
			continue
		}
		lead := defLead
		if e.RemindBefore != nil {
			lead = *e.RemindBefore
		}
		for _, o := range c.occurrences(e, from, to) {
			if err := c.ctxErr(ctx); err != nil {
				return err
			}
			if o.Start.Before(from) || o.Start.After(to) {
				continue
			}
			key := fmt.Sprintf("e:%d:%d", e.ID, o.Original.Unix())
			err := c.fireEventReminder(ctx, fmt.Sprintf("e:%d", e.ID), key, o.Original, o.Title,
				o.Start, e.AllDay, eventRemindAt(o.Start, e.AllDay, lead, time.Local), now)
			if err != nil {
				return err
			}
		}
	}
	meetings, _, err := c.store.MeetingsInRange(ctx, from, to)
	if err != nil {
		return err
	}
	for _, sm := range meetings {
		if err := c.ctxErr(ctx); err != nil {
			return err
		}
		var m taskapp.Meeting
		if err := json.Unmarshal([]byte(sm.JSON), &m); err != nil {
			slog.Warn("reunión de Taskapp ilegible en la caché, sin aviso", "id", sm.ID, "err", err)
			continue
		}
		if m.Status != "scheduled" {
			continue
		}
		if sm.Start.Before(from) || sm.Start.After(to) {
			continue
		}
		key := fmt.Sprintf("m:%d", sm.ID)
		err := c.fireEventReminder(ctx, key, key, sm.Start, m.Title, sm.Start, m.AllDay,
			eventRemindAt(sm.Start, m.AllDay, defLead, time.Local), now)
		if err != nil {
			return err
		}
	}
	return nil
}

// fireEventReminder reclama el aviso de la ocurrencia (original) y recién ahí
// lo emite; uno que quedó más de reminderStale atrás se reclama sin mostrarse.
func (c *Core) fireEventReminder(ctx context.Context, claimKey, itemKey string, original time.Time, title string,
	start time.Time, allDay bool, when, now time.Time) error {
	if when.After(now) {
		return nil
	}
	claimed, err := c.store.ClaimEventReminder(ctx, claimKey, original, now)
	if err != nil || !claimed {
		return err
	}
	if now.Sub(when) <= reminderStale {
		c.emit(EventCalendarReminder, model.EventReminder{Key: itemKey, Title: title, At: start, AllDay: allDay})
	}
	return nil
}

package mail

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

// setLocalAR pone time.Local en Buenos Aires durante el test (no correr en paralelo).
func setLocalAR(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		loc = time.FixedZone("ART", -3*3600)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })
	return loc
}

func tu(y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, time.UTC)
}

// weekly da de alta una serie semanal de una hora que empieza el lunes 2/11 a las 13:00 UTC.
func weekly(t *testing.T, c *Core, rule string) model.CalendarEvent {
	t.Helper()
	ev, err := c.SaveEvent(context.Background(), model.EventSave{Event: model.CalendarEvent{
		Title: "Semanal", Start: tu(2026, 11, 2, 13, 0), End: tu(2026, 11, 2, 14, 0), TZID: "UTC", RRule: rule,
	}})
	require.NoError(t, err)
	return ev
}

func listItems(t *testing.T, c *Core, from, to time.Time) []model.CalendarItem {
	t.Helper()
	r, err := c.ListCalendar(context.Background(), from, to)
	require.NoError(t, err)
	require.NotNil(t, r.Items)
	return r.Items
}

func starts(items []model.CalendarItem) []time.Time {
	out := []time.Time{}
	for _, it := range items {
		out = append(out, it.Start.UTC())
	}
	return out
}

func requireUserMsg(t *testing.T, err error, code, msg string) {
	t.Helper()
	assert.Equal(t, msg, requireUserError(t, err, code).Msg)
}

func TestSaveEvent_Validations(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	base := func() model.CalendarEvent {
		return model.CalendarEvent{Title: "Reunión", Start: tu(2026, 11, 20, 13, 0), End: tu(2026, 11, 20, 14, 0)}
	}
	cases := []struct {
		name string
		mod  func(e *model.CalendarEvent)
		msg  string
	}{
		{"título vacío", func(e *model.CalendarEvent) { e.Title = "   " }, "Poné un título."},
		{"fin igual al inicio", func(e *model.CalendarEvent) { e.End = e.Start }, "El fin tiene que ser después del inicio."},
		{"fin antes del inicio", func(e *model.CalendarEvent) { e.End = e.Start.Add(-time.Hour) }, "El fin tiene que ser después del inicio."},
		{"anticipación", func(e *model.CalendarEvent) { e.RemindBefore = ip(7) }, "Esa anticipación de aviso no es válida."},
		{"invitado", func(e *model.CalendarEvent) {
			e.Attendees = []model.Attendee{{Email: "ana@ej.com"}, {Email: "x"}}
		}, "Ese invitado no tiene una dirección válida: x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := base()
			tc.mod(&e)
			_, err := c.SaveEvent(ctx, model.EventSave{Event: e})
			requireUserMsg(t, err, "invalid", tc.msg)
		})
	}
	all, err := c.store.AllEvents(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)

	t.Run("solo eventos locales", func(t *testing.T) {
		inv, err := c.store.SaveEvent(ctx, model.CalendarEvent{UID: "inv@ej", Kind: "invite", Title: "Invitación",
			Start: tu(2026, 11, 20, 13, 0), End: tu(2026, 11, 20, 14, 0), Status: "confirmed", MyPartstat: "ACCEPTED"}, nil)
		require.NoError(t, err)
		e := base()
		e.ID = inv.ID
		_, err = c.SaveEvent(ctx, model.EventSave{Event: e})
		requireUserMsg(t, err, "invalid", "Este evento no se edita desde acá.")
		err = c.DeleteEvent(ctx, model.EventDelete{ID: inv.ID})
		requireUserMsg(t, err, "invalid", "Este evento no se edita desde acá.")
		got, err := c.store.GetEvent(ctx, inv.ID)
		require.NoError(t, err)
		assert.Equal(t, "Invitación", got.Title)
	})

	t.Run("invitado con nombre", func(t *testing.T) {
		e := base()
		e.Attendees = []model.Attendee{{Email: "  Ana <Ana@Ej.com> "}}
		saved, err := c.SaveEvent(ctx, model.EventSave{Event: e})
		require.NoError(t, err)
		require.Len(t, saved.Attendees, 1)
		assert.Equal(t, "ana@ej.com", saved.Attendees[0].Email)
		assert.Equal(t, "Ana", saved.Attendees[0].Name)
		assert.Equal(t, "NEEDS-ACTION", saved.Attendees[0].Partstat)
	})
}

func TestListCalendar_LocalTaskAndAllDay(t *testing.T) {
	loc := setLocalAR(t)
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	day, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Feriado", AllDay: true, Start: tu(2026, 11, 20, 0, 0), End: tu(2026, 11, 21, 0, 0),
	}})
	require.NoError(t, err)
	timed, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "Llamar", DueDate: "2026-11-20", DueTime: "10:00"})
	require.NoError(t, err)
	dated, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "Pagar", DueDate: "2026-11-21"})
	require.NoError(t, err)
	_, err = c.SaveLocalTask(ctx, model.LocalTask{Title: "Sin fecha"})
	require.NoError(t, err)
	done, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "Hecha", DueDate: "2026-11-20"})
	require.NoError(t, err)
	require.NoError(t, c.SetLocalTaskDone(ctx, done.ID, true))
	_, err = c.SaveLocalTask(ctx, model.LocalTask{Title: "Otra semana", DueDate: "2026-12-20"})
	require.NoError(t, err)

	items := listItems(t, c, time.Date(2026, 11, 16, 0, 0, 0, 0, loc), time.Date(2026, 11, 23, 0, 0, 0, 0, loc))
	require.Len(t, items, 3, "%+v", items)

	ev := items[0]
	assert.Equal(t, fmt.Sprintf("e:%d:%d", day.ID, tu(2026, 11, 20, 0, 0).Unix()), ev.Key)
	assert.Equal(t, "local", ev.Source)
	assert.Equal(t, day.ID, ev.EventID)
	assert.Equal(t, "Feriado", ev.Title)
	assert.True(t, ev.AllDay)
	assert.True(t, ev.Start.Equal(tu(2026, 11, 20, 0, 0)), "%v", ev.Start)
	assert.Equal(t, time.UTC, ev.Start.Location())
	assert.True(t, ev.End.Equal(tu(2026, 11, 21, 0, 0)))
	assert.False(t, ev.Recurring)

	task := items[1]
	assert.Equal(t, fmt.Sprintf("t:%d", timed.ID), task.Key)
	assert.Equal(t, "task", task.Source)
	assert.Equal(t, timed.ID, task.TaskID)
	assert.Equal(t, "Llamar", task.Title)
	assert.False(t, task.AllDay)
	assert.True(t, task.Start.Equal(time.Date(2026, 11, 20, 10, 0, 0, 0, loc)), "%v", task.Start)
	assert.True(t, task.End.Equal(time.Date(2026, 11, 20, 10, 30, 0, 0, loc)), "%v", task.End)
	assert.Equal(t, "10:00", task.Start.Format("15:04"), "en hora local")
	assert.True(t, task.Start.Equal(tu(2026, 11, 20, 13, 0)))

	allDay := items[2]
	assert.Equal(t, fmt.Sprintf("t:%d", dated.ID), allDay.Key)
	assert.True(t, allDay.AllDay)
	assert.True(t, allDay.Start.Equal(tu(2026, 11, 21, 0, 0)), "%v", allDay.Start)
	assert.True(t, allDay.End.Equal(tu(2026, 11, 22, 0, 0)))

	empty, err := c.ListCalendar(ctx, tu(2030, 1, 1, 0, 0), tu(2030, 1, 2, 0, 0))
	require.NoError(t, err)
	assert.NotNil(t, empty.Items)
	assert.Empty(t, empty.Items)
}

func TestListCalendar_DeclinedHidden(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	mk := func(uid, kind, partstat, status string, h int) model.CalendarEvent {
		e, err := c.store.SaveEvent(ctx, model.CalendarEvent{UID: uid, Kind: kind, Title: uid, Status: status, MyPartstat: partstat,
			Start: tu(2026, 11, 20, h, 0), End: tu(2026, 11, 20, h+1, 0), NeedsReview: uid == "aceptada",
			Attendees: []model.Attendee{{Email: "a@ej.com", Partstat: "ACCEPTED"}, {Email: "b@ej.com", Partstat: "NEEDS-ACTION"}}}, nil)
		require.NoError(t, err)
		return e
	}
	acc := mk("aceptada", "invite", "ACCEPTED", "confirmed", 10)
	mk("rechazada", "invite", "DECLINED", "confirmed", 11)
	canc := mk("cancelada", "invite", "TENTATIVE", "cancelled", 12)
	mk("espejo", "taskapp", "", "confirmed", 13)

	items := listItems(t, c, tu(2026, 11, 20, 0, 0), tu(2026, 11, 21, 0, 0))
	require.Len(t, items, 2, "%+v", items)
	assert.Equal(t, acc.ID, items[0].EventID)
	assert.Equal(t, "invite", items[0].Source)
	assert.Equal(t, "ACCEPTED", items[0].Partstat)
	assert.True(t, items[0].NeedsReview)
	assert.False(t, items[0].Cancelled)
	assert.Equal(t, 2, items[0].Attendees)
	assert.Equal(t, 1, items[0].Confirmed)
	assert.Equal(t, canc.ID, items[1].EventID)
	assert.True(t, items[1].Cancelled)
	assert.Equal(t, "TENTATIVE", items[1].Partstat)
}

func TestSaveEvent_ScopeThis(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	ser := weekly(t, c, "FREQ=WEEKLY")
	occ := tu(2026, 11, 16, 13, 0)
	moved := ser
	moved.Title, moved.Location = "Movida", "Sala 2"
	moved.Start, moved.End = tu(2026, 11, 17, 15, 0), tu(2026, 11, 17, 16, 30)
	moved.RRule = "FREQ=DAILY" // se ignora: "solo este" no cambia la serie
	got, err := c.SaveEvent(ctx, model.EventSave{Event: moved, Scope: "this", OccurrenceAt: occ})
	require.NoError(t, err)
	assert.Equal(t, ser.ID, got.ID)
	assert.Equal(t, "FREQ=WEEKLY", got.RRule)
	assert.Equal(t, "Semanal", got.Title)
	assert.True(t, got.Start.Equal(ser.Start))
	require.Len(t, got.Exceptions, 1)
	x := got.Exceptions[0]
	assert.True(t, x.RecurrenceAt.Equal(occ))
	assert.False(t, x.Cancelled)
	assert.Equal(t, "Movida", x.Title)

	items := listItems(t, c, tu(2026, 11, 1, 0, 0), tu(2026, 12, 1, 0, 0))
	assert.Equal(t, []time.Time{tu(2026, 11, 2, 13, 0), tu(2026, 11, 9, 13, 0), tu(2026, 11, 17, 15, 0),
		tu(2026, 11, 23, 13, 0), tu(2026, 11, 30, 13, 0)}, starts(items))
	it := items[2]
	assert.Equal(t, "Movida", it.Title)
	assert.Equal(t, "Sala 2", it.Location)
	assert.True(t, it.End.Equal(tu(2026, 11, 17, 16, 30)))
	assert.True(t, it.OccurrenceAt.Equal(occ))
	assert.True(t, it.Recurring)
	assert.Equal(t, fmt.Sprintf("e:%d:%d", ser.ID, occ.Unix()), it.Key)
	assert.Equal(t, "Semanal", items[3].Title)

	// La movida aparece en su hora nueva aunque el inicio original quede fuera del rango.
	assert.Equal(t, []time.Time{tu(2026, 11, 17, 15, 0)}, starts(listItems(t, c, tu(2026, 11, 17, 0, 0), tu(2026, 11, 18, 0, 0))))
	assert.Empty(t, listItems(t, c, tu(2026, 11, 16, 0, 0), tu(2026, 11, 17, 0, 0)))

	// Volver a editar la misma ocurrencia reemplaza la excepción.
	moved.Title = "Otra vez"
	got, err = c.SaveEvent(ctx, model.EventSave{Event: moved, Scope: "this", OccurrenceAt: occ})
	require.NoError(t, err)
	require.Len(t, got.Exceptions, 1)
	assert.Equal(t, "Otra vez", got.Exceptions[0].Title)

	// Una ocurrencia que no es de la serie no se toca.
	_, err = c.SaveEvent(ctx, model.EventSave{Event: moved, Scope: "this", OccurrenceAt: tu(2026, 11, 18, 13, 0)})
	requireUserMsg(t, err, "not_found", msgNotFound)
}

func TestSaveEvent_ScopeFollowingKeepsEarlierExceptions(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	ser := weekly(t, c, "FREQ=WEEKLY")
	// Semana 2 movida, semana 3 borrada y semana 6 con otro título.
	w2 := ser
	w2.Start, w2.End = tu(2026, 11, 10, 13, 0), tu(2026, 11, 10, 14, 0)
	_, err := c.SaveEvent(ctx, model.EventSave{Event: w2, Scope: "this", OccurrenceAt: tu(2026, 11, 9, 13, 0)})
	require.NoError(t, err)
	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: ser.ID, Scope: "this", OccurrenceAt: tu(2026, 11, 16, 13, 0)}))
	w6 := ser
	w6.Title = "Con otro título"
	w6.Start, w6.End = tu(2026, 12, 7, 13, 0), tu(2026, 12, 7, 14, 0)
	_, err = c.SaveEvent(ctx, model.EventSave{Event: w6, Scope: "this", OccurrenceAt: tu(2026, 12, 7, 13, 0)})
	require.NoError(t, err)

	// Desde la semana 4, a las 16:00.
	edit := ser
	edit.Title = "Nueva hora"
	edit.Start, edit.End = tu(2026, 11, 23, 16, 0), tu(2026, 11, 23, 17, 0)
	newEv, err := c.SaveEvent(ctx, model.EventSave{Event: edit, Scope: "following", OccurrenceAt: tu(2026, 11, 23, 13, 0)})
	require.NoError(t, err)

	all, err := c.store.AllEvents(ctx)
	require.NoError(t, err)
	require.Len(t, all, 2)
	old, err := c.store.GetEvent(ctx, ser.ID)
	require.NoError(t, err)
	assert.Equal(t, "FREQ=WEEKLY;UNTIL=20261123T125959Z", old.RRule)
	assert.Equal(t, "Semanal", old.Title)
	assert.Equal(t, ser.UID, old.UID)
	require.Len(t, old.Exceptions, 2)
	assert.True(t, old.Exceptions[0].RecurrenceAt.Equal(tu(2026, 11, 9, 13, 0)))
	assert.True(t, old.Exceptions[1].RecurrenceAt.Equal(tu(2026, 11, 16, 13, 0)))
	assert.True(t, old.Exceptions[1].Cancelled)

	assert.NotEqual(t, ser.ID, newEv.ID)
	assert.NotEqual(t, ser.UID, newEv.UID)
	assert.True(t, strings.HasSuffix(newEv.UID, "@postacontrol"))
	assert.Equal(t, "local", newEv.Kind)
	assert.Equal(t, "Nueva hora", newEv.Title)
	assert.Equal(t, "FREQ=WEEKLY", newEv.RRule)
	assert.True(t, newEv.Start.Equal(tu(2026, 11, 23, 16, 0)))
	require.Len(t, newEv.Exceptions, 1)
	assert.True(t, newEv.Exceptions[0].RecurrenceAt.Equal(tu(2026, 12, 7, 16, 0)), "corrida 3 h")
	assert.True(t, newEv.Exceptions[0].Start.Equal(tu(2026, 12, 7, 16, 0)))

	// Ocho semanas: ninguna ocurrencia repetida ni perdida.
	items := listItems(t, c, tu(2026, 11, 1, 0, 0), tu(2026, 12, 27, 0, 0))
	assert.Equal(t, []time.Time{tu(2026, 11, 2, 13, 0), tu(2026, 11, 10, 13, 0), tu(2026, 11, 23, 16, 0),
		tu(2026, 11, 30, 16, 0), tu(2026, 12, 7, 16, 0), tu(2026, 12, 14, 16, 0), tu(2026, 12, 21, 16, 0)}, starts(items))
	keys := map[string]bool{}
	for _, it := range items {
		assert.False(t, keys[it.Key], "clave repetida %s", it.Key)
		keys[it.Key] = true
	}
	assert.Equal(t, "Semanal", items[1].Title)
	assert.Equal(t, "Nueva hora", items[2].Title)
	assert.Equal(t, "Con otro título", items[4].Title)
	assert.Equal(t, newEv.ID, items[4].EventID)
}

func TestSaveEvent_ScopeFollowingWithCount(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	ser := weekly(t, c, "FREQ=WEEKLY;COUNT=6")
	edit := ser
	edit.Start, edit.End = tu(2026, 11, 16, 10, 0), tu(2026, 11, 16, 11, 0)
	newEv, err := c.SaveEvent(ctx, model.EventSave{Event: edit, Scope: "following", OccurrenceAt: tu(2026, 11, 16, 13, 0)})
	require.NoError(t, err)
	old, err := c.store.GetEvent(ctx, ser.ID)
	require.NoError(t, err)
	assert.Equal(t, "FREQ=WEEKLY;UNTIL=20261116T125959Z", old.RRule)
	assert.Equal(t, "FREQ=WEEKLY;COUNT=4", newEv.RRule)
	items := listItems(t, c, tu(2026, 11, 1, 0, 0), tu(2027, 3, 1, 0, 0))
	assert.Equal(t, []time.Time{tu(2026, 11, 2, 13, 0), tu(2026, 11, 9, 13, 0), tu(2026, 11, 16, 10, 0),
		tu(2026, 11, 23, 10, 0), tu(2026, 11, 30, 10, 0), tu(2026, 12, 7, 10, 0)}, starts(items))

	// Día completo: la serie vieja termina el día anterior, en forma de fecha.
	day, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{Title: "Diario", AllDay: true,
		Start: tu(2027, 1, 4, 0, 0), End: tu(2027, 1, 5, 0, 0), RRule: "FREQ=DAILY;COUNT=5"}})
	require.NoError(t, err)
	dEdit := day
	dEdit.Title = "Diario nuevo"
	dEdit.Start, dEdit.End = tu(2027, 1, 6, 0, 0), tu(2027, 1, 7, 0, 0)
	dNew, err := c.SaveEvent(ctx, model.EventSave{Event: dEdit, Scope: "following", OccurrenceAt: tu(2027, 1, 6, 0, 0)})
	require.NoError(t, err)
	dOld, err := c.store.GetEvent(ctx, day.ID)
	require.NoError(t, err)
	assert.Equal(t, "FREQ=DAILY;UNTIL=20270105", dOld.RRule)
	assert.Equal(t, "FREQ=DAILY;COUNT=3", dNew.RRule)
	assert.Len(t, listItems(t, c, tu(2027, 1, 1, 0, 0), tu(2027, 2, 1, 0, 0)), 5)
}

func TestSaveEvent_FollowingFromFirstEqualsAll(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	ser := weekly(t, c, "FREQ=WEEKLY")
	edit := ser
	edit.Title = "Toda la serie"
	edit.Start, edit.End = tu(2026, 11, 2, 15, 0), tu(2026, 11, 2, 16, 0)
	got, err := c.SaveEvent(ctx, model.EventSave{Event: edit, Scope: "following", OccurrenceAt: ser.Start})
	require.NoError(t, err)
	assert.Equal(t, ser.ID, got.ID)
	assert.Equal(t, ser.UID, got.UID)
	assert.Equal(t, "FREQ=WEEKLY", got.RRule)
	assert.Equal(t, "Toda la serie", got.Title)
	all, err := c.store.AllEvents(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, []time.Time{tu(2026, 11, 2, 15, 0), tu(2026, 11, 9, 15, 0)},
		starts(listItems(t, c, tu(2026, 11, 1, 0, 0), tu(2026, 11, 15, 0, 0))))
}

func TestDeleteEvent_ScopeThis(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	ser := weekly(t, c, "FREQ=WEEKLY")
	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: ser.ID, Scope: "this", OccurrenceAt: tu(2026, 11, 9, 13, 0)}))
	month := func() []time.Time { return starts(listItems(t, c, tu(2026, 11, 1, 0, 0), tu(2026, 12, 1, 0, 0))) }
	assert.Equal(t, []time.Time{tu(2026, 11, 2, 13, 0), tu(2026, 11, 16, 13, 0), tu(2026, 11, 23, 13, 0), tu(2026, 11, 30, 13, 0)}, month())

	// "Este y los siguientes" corta la serie.
	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: ser.ID, Scope: "following", OccurrenceAt: tu(2026, 11, 23, 13, 0)}))
	assert.Equal(t, []time.Time{tu(2026, 11, 2, 13, 0), tu(2026, 11, 16, 13, 0)}, month())
	got, err := c.store.GetEvent(ctx, ser.ID)
	require.NoError(t, err)
	assert.Equal(t, "FREQ=WEEKLY;UNTIL=20261123T125959Z", got.RRule)
	assert.Len(t, got.Exceptions, 1)

	// Sin alcance borra todo.
	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: ser.ID}))
	assert.Empty(t, month())
	_, err = c.GetEvent(ctx, ser.ID)
	requireUserMsg(t, err, "not_found", msgNotFound)
	assert.Len(t, n.matching(EventCalendarChanged), 4)
}

func TestSaveEvent_EmitsCalendarChanged(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	ev, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Uno", Start: tu(2026, 11, 20, 13, 0), End: tu(2026, 11, 20, 14, 0),
		Kind: "invite", Status: "cancelled", // un alta siempre es local y confirmada
	}})
	require.NoError(t, err)
	assert.Equal(t, []string{EventCalendarChanged}, n.names())
	assert.Equal(t, "mail:calendar-changed", EventCalendarChanged)
	assert.Nil(t, n.events[0].Data)
	assert.Equal(t, "local", ev.Kind)
	assert.Equal(t, "confirmed", ev.Status)
	assert.Equal(t, "America/Argentina/Buenos_Aires", ev.TZID)
	assert.True(t, strings.HasSuffix(ev.UID, "@postacontrol"), ev.UID)
	assert.Len(t, strings.TrimSuffix(ev.UID, "@postacontrol"), 36)

	ev.Title = "Dos"
	_, err = c.SaveEvent(ctx, model.EventSave{Event: ev})
	require.NoError(t, err)
	got, err := c.GetEvent(ctx, ev.ID)
	require.NoError(t, err)
	assert.Equal(t, "Dos", got.Title)
	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: ev.ID}))
	assert.Len(t, n.matching(EventCalendarChanged), 3)

	_, err = c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{Title: ""}})
	require.Error(t, err)
	assert.Len(t, n.matching(EventCalendarChanged), 3, "un error no emite")
}

func TestSettings_EventReminderValidated(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	err := c.SetSettings(ctx, model.Settings{EventReminderMinutes: ip(7)})
	requireUserMsg(t, err, "invalid", msgBadReminder)
	require.NoError(t, c.SetSettings(ctx, model.Settings{EventReminderMinutes: ip(30)}))
	st, err := c.GetSettings(ctx)
	require.NoError(t, err)
	require.NotNil(t, st.EventReminderMinutes)
	assert.Equal(t, 30, *st.EventReminderMinutes)
}

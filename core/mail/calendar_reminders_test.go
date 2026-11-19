package mail

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/store"
	"postacontrol/core/taskapp"
)

func eventReminders(n *recNotifier) []model.EventReminder {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := []model.EventReminder{}
	for _, e := range n.events {
		if e.Name == EventCalendarReminder {
			out = append(out, e.Data.(model.EventReminder))
		}
	}
	return out
}

// saveRaw guarda un evento tal cual, sin pasar por las reglas del alta.
func saveRaw(t *testing.T, c *Core, e model.CalendarEvent) model.CalendarEvent {
	t.Helper()
	saved, err := c.store.SaveEvent(context.Background(), e, nil)
	require.NoError(t, err)
	return saved
}

func TestEventReminder_FiresOnceAtLead(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	ev, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Reunión", Start: at("2026-11-19", "10:00"), End: at("2026-11-19", "11:00"), RemindBefore: ip(15),
	}})
	require.NoError(t, err)

	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:44")))
	assert.Empty(t, eventReminders(n))
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:45")))
	got := eventReminders(n)
	require.Len(t, got, 1)
	assert.Equal(t, "Reunión", got[0].Title)
	assert.True(t, got[0].At.Equal(at("2026-11-19", "10:00")))
	assert.False(t, got[0].AllDay)
	assert.Equal(t, "e:"+itoa(ev.ID)+":"+itoa(at("2026-11-19", "10:00").Unix()), got[0].Key)
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:46")))
	assert.Len(t, eventReminders(n), 1)
}

func TestEventReminder_DefaultLeadFromSettings(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	st, err := c.store.GetSettings(ctx)
	require.NoError(t, err)
	st.EventReminderMinutes = ip(30)
	require.NoError(t, c.SetSettings(ctx, st))
	_, err = c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "x", Start: at("2026-11-19", "10:00"), End: at("2026-11-19", "11:00"),
	}})
	require.NoError(t, err)
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:29")))
	assert.Empty(t, eventReminders(n))
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:30")))
	assert.Len(t, eventReminders(n), 1)
}

func TestEventReminder_EachOccurrence(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	_, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Diario", Start: at("2026-11-19", "10:00"), End: at("2026-11-19", "11:00"), RRule: "FREQ=DAILY", RemindBefore: ip(15),
	}})
	require.NoError(t, err)
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:50")))
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-20", "09:50")))
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-20", "09:55")))
	got := eventReminders(n)
	require.Len(t, got, 2)
	assert.True(t, got[0].At.Equal(at("2026-11-19", "10:00")))
	assert.True(t, got[1].At.Equal(at("2026-11-20", "10:00")))
	assert.NotEqual(t, got[0].Key, got[1].Key)
}

func TestEventReminder_SkipsDeclinedAndCancelled(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	base := model.CalendarEvent{Start: at("2026-11-19", "10:00"), End: at("2026-11-19", "11:00"), RemindBefore: ip(15), Status: "confirmed"}
	cancelled, declined, mirror := base, base, base
	cancelled.UID, cancelled.Kind, cancelled.Title, cancelled.Status = "c@x", "local", "Cancelado", "cancelled"
	declined.UID, declined.Kind, declined.Title, declined.MyPartstat = "d@x", "invite", "Rechazado", "DECLINED"
	mirror.UID, mirror.Kind, mirror.Title = "m@x", "taskapp", "Espejo"
	saveRaw(t, c, cancelled)
	saveRaw(t, c, declined)
	saveRaw(t, c, mirror)
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:50")))
	assert.Empty(t, eventReminders(n))
}

func TestEventReminder_UnansweredInviteFires(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	saveRaw(t, c, model.CalendarEvent{UID: "i@x", Kind: "invite", Title: "Invitación", Status: "confirmed", MyPartstat: "NEEDS-ACTION",
		Start: at("2026-11-19", "10:00"), End: at("2026-11-19", "11:00"), RemindBefore: ip(15)})
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:50")))
	got := eventReminders(n)
	require.Len(t, got, 1)
	assert.Equal(t, "Invitación", got[0].Title)
}

func TestEventReminder_StaleSkipped(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	// Día completo del 19 con anticipación de un día: el aviso era el 18 a las 09:00.
	ev, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Feriado", AllDay: true, Start: tu(2026, 11, 19, 0, 0), End: tu(2026, 11, 20, 0, 0), RemindBefore: ip(1440),
	}})
	require.NoError(t, err)
	now := at("2026-11-19", "15:00") // 30 h después
	require.NoError(t, c.fireEventReminders(ctx, now))
	assert.Empty(t, eventReminders(n))
	// Se reclamó: no avisa después tampoco.
	ok, err := c.store.ClaimEventReminder(ctx, "e:"+itoa(ev.ID), tu(2026, 11, 19, 0, 0), now)
	require.NoError(t, err)
	assert.False(t, ok, "el aviso viejo quedó reclamado")
}

func TestEventReminder_AllDayAt9(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	_, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Día libre", AllDay: true, Start: tu(2026, 11, 20, 0, 0), End: tu(2026, 11, 21, 0, 0), RemindBefore: ip(15),
	}})
	require.NoError(t, err)
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-20", "08:59")))
	assert.Empty(t, eventReminders(n))
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-20", "09:00")))
	got := eventReminders(n)
	require.Len(t, got, 1)
	assert.True(t, got[0].AllDay)

	// Con anticipación de un día, a las 09:00 del anterior.
	c2, _, _, n2 := newTestCore(t)
	_, err = c2.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Día libre", AllDay: true, Start: tu(2026, 11, 20, 0, 0), End: tu(2026, 11, 21, 0, 0), RemindBefore: ip(1440),
	}})
	require.NoError(t, err)
	require.NoError(t, c2.fireEventReminders(ctx, at("2026-11-19", "08:59")))
	assert.Empty(t, eventReminders(n2))
	require.NoError(t, c2.fireEventReminders(ctx, at("2026-11-19", "09:00")))
	assert.Len(t, eventReminders(n2), 1)
}

func TestEventRemindAt(t *testing.T) {
	loc := setLocalAR(t)
	assert.True(t, eventRemindAt(tu(2026, 11, 19, 13, 0), false, 15, loc).Equal(tu(2026, 11, 19, 12, 45)))
	assert.True(t, eventRemindAt(tu(2026, 11, 20, 0, 0), true, 5, loc).Equal(at("2026-11-20", "09:00")))
	assert.True(t, eventRemindAt(tu(2026, 11, 20, 0, 0), true, 1440, loc).Equal(at("2026-11-19", "09:00")))
}

func putMeeting(t *testing.T, c *Core, id int64, status string, start time.Time) {
	t.Helper()
	m := taskapp.Meeting{ID: id, WorkspaceID: 1, Title: "Daily", Status: status, Start: start, End: start.Add(time.Hour)}
	raw, err := json.Marshal(m)
	require.NoError(t, err)
	require.NoError(t, c.store.ReplaceMeetings(context.Background(), 1, start.Add(-48*time.Hour), start.Add(48*time.Hour),
		append(existingMeetings(t, c, start), store.StoredMeeting{ID: id, WorkspaceID: 1, Start: start, End: m.End, JSON: string(raw)}), time.Now()))
}

func existingMeetings(t *testing.T, c *Core, around time.Time) []store.StoredMeeting {
	t.Helper()
	ms, _, err := c.store.MeetingsInRange(context.Background(), around.Add(-48*time.Hour), around.Add(48*time.Hour))
	require.NoError(t, err)
	return ms
}

func TestEventReminder_TaskappMeeting(t *testing.T) {
	setLocalAR(t)
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	st, err := c.store.GetSettings(ctx)
	require.NoError(t, err)
	st.EventReminderMinutes = ip(30)
	require.NoError(t, c.SetSettings(ctx, st))
	start := at("2026-11-19", "10:00")
	putMeeting(t, c, 7, "scheduled", start)
	putMeeting(t, c, 8, "cancelled", start)
	putMeeting(t, c, 9, "completed", start)

	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:29")))
	assert.Empty(t, eventReminders(n))
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:30")))
	require.NoError(t, c.fireEventReminders(ctx, at("2026-11-19", "09:31")))
	got := eventReminders(n)
	require.Len(t, got, 1)
	assert.Equal(t, "m:7", got[0].Key)
	assert.Equal(t, "Daily", got[0].Title)
	assert.True(t, got[0].At.Equal(start))
}

package mail

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func reminderCount(n *recNotifier) int {
	n.mu.Lock()
	defer n.mu.Unlock()
	k := 0
	for _, e := range n.events {
		if e.Name == EventTaskReminder {
			k++
		}
	}
	return k
}

func at(date, hm string) time.Time {
	tm, err := time.ParseInLocation("2006-01-02 15:04", date+" "+hm, time.Local)
	if err != nil {
		panic(err)
	}
	return tm
}

func TestRemindAt(t *testing.T) {
	task := model.LocalTask{DueDate: "2026-11-19", DueTime: "10:00"}
	got, ok := remindAt(task, 15, time.Local)
	require.True(t, ok)
	assert.True(t, got.Equal(at("2026-11-19", "09:45")))
	task.RemindBefore = ip(0)
	got, ok = remindAt(task, 15, time.Local)
	require.True(t, ok)
	assert.True(t, got.Equal(at("2026-11-19", "10:00")))
	_, ok = remindAt(model.LocalTask{DueDate: "2026-11-19"}, 15, time.Local)
	assert.False(t, ok)
}

func TestFireDueReminders_FiresOnce(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	saved, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "Reunión", DueDate: "2026-11-19", DueTime: "10:00"})
	require.NoError(t, err)
	now := at("2026-11-19", "09:50")
	require.NoError(t, c.fireDueReminders(ctx, now))
	require.NoError(t, c.fireDueReminders(ctx, now.Add(time.Minute)))
	assert.Equal(t, 1, reminderCount(n))
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, e := range n.events {
		if e.Name == EventTaskReminder {
			r := e.Data.(model.TaskReminder)
			assert.Equal(t, saved.ID, r.TaskID)
			assert.Equal(t, "Reunión", r.Title)
			assert.True(t, r.At.Equal(at("2026-11-19", "10:00")))
		}
	}
}

func TestFireDueReminders_NotYet(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	_, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "x", DueDate: "2026-11-19", DueTime: "10:00"})
	require.NoError(t, err)
	require.NoError(t, c.fireDueReminders(ctx, at("2026-11-19", "09:30")))
	assert.Equal(t, 0, reminderCount(n))
}

func TestFireDueReminders_SkipsOlderThan24h(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	saved, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "x", DueDate: "2026-11-19", DueTime: "10:00", RemindBefore: ip(0)})
	require.NoError(t, err)
	require.NoError(t, c.fireDueReminders(ctx, at("2026-11-19", "10:00").Add(25*time.Hour)))
	assert.Equal(t, 0, reminderCount(n))
	pend, err := c.store.PendingReminders(ctx)
	require.NoError(t, err)
	for _, p := range pend {
		assert.NotEqual(t, saved.ID, p.ID)
	}
}

func TestFireDueReminders_RearmOnEdit(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	saved, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "x", DueDate: "2026-11-19", DueTime: "10:00"})
	require.NoError(t, err)
	require.NoError(t, c.fireDueReminders(ctx, at("2026-11-19", "09:50")))
	require.Equal(t, 1, reminderCount(n))
	saved.DueTime = "15:00"
	_, err = c.SaveLocalTask(ctx, saved)
	require.NoError(t, err)
	require.NoError(t, c.fireDueReminders(ctx, at("2026-11-19", "14:50")))
	assert.Equal(t, 2, reminderCount(n))
	// Editar solo el título no rearma.
	saved.Title = "y"
	_, err = c.SaveLocalTask(ctx, saved)
	require.NoError(t, err)
	require.NoError(t, c.fireDueReminders(ctx, at("2026-11-19", "14:55")))
	assert.Equal(t, 2, reminderCount(n))
}

func TestFireDueReminders_DoneOrDeletedNever(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	a, _ := c.SaveLocalTask(ctx, model.LocalTask{Title: "a", DueDate: "2026-11-19", DueTime: "10:00"})
	b, _ := c.SaveLocalTask(ctx, model.LocalTask{Title: "b", DueDate: "2026-11-19", DueTime: "10:00"})
	require.NoError(t, c.SetLocalTaskDone(ctx, a.ID, true))
	require.NoError(t, c.DeleteLocalTask(ctx, b.ID))
	require.NoError(t, c.fireDueReminders(ctx, at("2026-11-19", "10:00")))
	assert.Equal(t, 0, reminderCount(n))
}

func TestSetSettings_RejectsBadReminder(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	err := c.SetSettings(ctx, model.Settings{TaskReminderMinutes: ip(7)})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "Anticipación del aviso inválida.", ue.Msg)
	require.NoError(t, c.SetSettings(ctx, model.Settings{TaskReminderMinutes: ip(30)}))
}

func TestFireReminder_EditBetweenSnapshotAndClaim(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	saved, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "x", DueDate: "2026-11-19", DueTime: "10:00"})
	require.NoError(t, err)
	snap, err := c.store.PendingReminders(ctx)
	require.NoError(t, err)
	require.Len(t, snap, 1)

	// El usuario cambia la hora justo después de la foto.
	saved.DueTime = "15:00"
	_, err = c.SaveLocalTask(ctx, saved)
	require.NoError(t, err)

	require.NoError(t, c.fireReminder(ctx, snap[0], 15, at("2026-11-19", "09:50")))
	assert.Equal(t, 0, reminderCount(n), "no avisa con la hora vieja")

	require.NoError(t, c.fireDueReminders(ctx, at("2026-11-19", "14:50")))
	assert.Equal(t, 1, reminderCount(n), "el aviso rearmado sigue en pie")
}

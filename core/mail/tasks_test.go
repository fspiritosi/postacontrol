package mail

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func ip(n int) *int { return &n }

func TestSaveLocalTask_Validation(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	cases := []struct {
		name string
		task model.LocalTask
		msg  string
	}{
		{"título vacío", model.LocalTask{Title: "   "}, "La tarea necesita un título."},
		{"hora sin fecha", model.LocalTask{Title: "x", DueTime: "10:00"}, "Para poner hora, elegí una fecha."},
		{"fecha inválida", model.LocalTask{Title: "x", DueDate: "19/11/2026"}, "Fecha u hora inválida."},
		{"hora inválida", model.LocalTask{Title: "x", DueDate: "2026-11-19", DueTime: "25:00"}, "Fecha u hora inválida."},
		{"anticipación inválida", model.LocalTask{Title: "x", RemindBefore: ip(7)}, "Anticipación del aviso inválida."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := c.SaveLocalTask(context.Background(), tc.task)
			var ue *UserError
			require.True(t, errors.As(err, &ue))
			assert.Equal(t, "invalid", ue.Code)
			assert.Equal(t, tc.msg, ue.Msg)
		})
	}
}

func TestSaveLocalTask_CRUD(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	saved, err := c.SaveLocalTask(ctx, model.LocalTask{Title: "  Llamar  ", DueDate: "2026-11-19"})
	require.NoError(t, err)
	assert.Equal(t, "Llamar", saved.Title)
	require.NoError(t, c.SetLocalTaskDone(ctx, saved.ID, true))
	list, err := c.ListLocalTasks(ctx, false)
	require.NoError(t, err)
	assert.Empty(t, list)
	list, err = c.ListLocalTasks(ctx, true)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NotNil(t, list[0].DoneAt)
	require.NoError(t, c.SetLocalTaskDone(ctx, saved.ID, false))
	got, _ := c.store.GetLocalTask(ctx, saved.ID)
	assert.Nil(t, got.DoneAt)
	require.NoError(t, c.DeleteLocalTask(ctx, saved.ID))
	assert.Equal(t, 4, len(n.matching(EventTasksChanged)))

	_, err = c.SaveLocalTask(ctx, model.LocalTask{ID: 999, Title: "x"})
	var ue *UserError
	require.True(t, errors.As(err, &ue))
	assert.Equal(t, "not_found", ue.Code)
	assert.Equal(t, "Esa tarea ya no existe.", ue.Msg)
}

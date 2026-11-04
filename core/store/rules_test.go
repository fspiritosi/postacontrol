package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestRules_SaveListReorderAndCascade(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	acc, inbox, _ := labelAccount(t, s, "a@ej.com")
	other, _, _ := labelAccount(t, s, "b@ej.com")
	mk := func(account int64, name string) model.Rule {
		r, err := s.SaveRule(ctx, model.Rule{AccountID: account, Name: name, Enabled: true, MatchAll: true,
			Conditions: []model.RuleCondition{{Field: "from", Op: "is", Value: "x@ej.com"}},
			Actions:    []model.RuleAction{{Type: "move", FolderID: inbox}}})
		require.NoError(t, err)
		return r
	}
	a := mk(acc, "A")
	b := mk(acc, "B")
	mk(other, "C")
	assert.Equal(t, 1, a.Position)
	assert.Equal(t, 2, b.Position)
	assert.Equal(t, inbox, a.Actions[0].FolderID)

	require.NoError(t, s.ReorderRules(ctx, acc, []int64{b.ID, a.ID}))
	list, err := s.ListRules(ctx, acc)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, "B", list[0].Name)

	a.Name, a.Enabled = "A2", false
	got, err := s.SaveRule(ctx, a)
	require.NoError(t, err)
	assert.Equal(t, "A2", got.Name)
	assert.False(t, got.Enabled)
	assert.Equal(t, 2, got.Position)

	_, err = s.SaveRule(ctx, model.Rule{ID: 999, AccountID: acc, Name: "x"})
	assert.ErrorIs(t, err, ErrNotFound)

	require.NoError(t, s.DeleteAccount(ctx, acc))
	list, err = s.ListRules(ctx, acc)
	require.NoError(t, err)
	assert.Empty(t, list)
	list, err = s.ListRules(ctx, other)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestRuleMessages_ByUIDsInArrivalOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, inbox, _ := labelAccount(t, s, "a@ej.com")
	rows := []HeaderRow{}
	for _, uid := range []uint32{3, 1, 2} {
		r := HeaderRow{To: []model.Address{{Email: "yo@ej.com"}}, Cc: []model.Address{}, ReplyTo: []model.Address{}}
		r.UID, r.Subject, r.Date = uid, "s", time.Unix(int64(uid), 0)
		r.From = model.Address{Name: "Bea", Email: "bea@ej.com"}
		rows = append(rows, r)
	}
	require.NoError(t, s.UpsertHeaders(ctx, inbox, rows))
	ms, err := s.RuleMessages(ctx, inbox, []uint32{3, 2})
	require.NoError(t, err)
	require.Len(t, ms, 2)
	assert.Equal(t, []uint32{2, 3}, []uint32{ms[0].UID, ms[1].UID})
	assert.Equal(t, "yo@ej.com", ms[0].To[0].Email)
	assert.Empty(t, ms[0].Cc)
	all, err := s.RuleMessages(ctx, inbox, nil)
	require.NoError(t, err)
	assert.Len(t, all, 3)
}

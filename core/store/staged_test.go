package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestStagedAttachments_RoundTripKeepsOrder(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a, err := s.StageAttachment(ctx, "a.txt", "text/plain", []byte("uno"))
	require.NoError(t, err)
	b, err := s.StageAttachment(ctx, "b.bin", "application/octet-stream", []byte("dos!"))
	require.NoError(t, err)
	assert.Equal(t, model.StagedAttachment{ID: a.ID, Filename: "a.txt", MIMEType: "text/plain", Size: 3}, a)

	files, err := s.StagedAttachments(ctx, []int64{b.ID, a.ID})
	require.NoError(t, err)
	require.Len(t, files, 2)
	assert.Equal(t, []byte("dos!"), files[0].Data)
	assert.Equal(t, "b.bin", files[0].Filename)
	assert.Equal(t, []byte("uno"), files[1].Data)

	_, err = s.StagedAttachments(ctx, []int64{a.ID, 9999})
	assert.ErrorIs(t, err, ErrNotFound)

	info, err := s.StagedInfo(ctx, []int64{b.ID, 9999, a.ID})
	require.NoError(t, err)
	assert.Equal(t, []model.StagedAttachment{b, a}, info)

	require.NoError(t, s.DeleteStagedAttachments(ctx, []int64{a.ID}))
	info, err = s.StagedInfo(ctx, []int64{a.ID, b.ID})
	require.NoError(t, err)
	assert.Equal(t, []model.StagedAttachment{b}, info)
}

func TestPruneStaged_KeepsReferencedByOutbox(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	mk := func(name string, age time.Duration) int64 {
		a, err := s.StageAttachment(ctx, name, "text/plain", []byte("x"))
		require.NoError(t, err)
		_, err = s.db.ExecContext(ctx, `UPDATE staged_attachments SET created_at = ? WHERE id = ?`,
			time.Now().Add(-age).Unix(), a.ID)
		require.NoError(t, err)
		return a.ID
	}
	referenced := mk("ref", 48*time.Hour)
	loose := mk("suelto", 48*time.Hour)
	fresh := mk("nuevo", time.Hour)

	_, err := s.QueueLocalDraft(ctx, 0, model.SendRequest{AccountID: accID, Attachments: []int64{referenced}})
	require.NoError(t, err)

	require.NoError(t, s.PruneStagedAttachments(ctx, time.Now().Add(-24*time.Hour)))
	info, err := s.StagedInfo(ctx, []int64{referenced, loose, fresh})
	require.NoError(t, err)
	require.Len(t, info, 2)
	assert.Equal(t, referenced, info[0].ID)
	assert.Equal(t, fresh, info[1].ID)
}

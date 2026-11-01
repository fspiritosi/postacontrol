package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestComposeDrafts_SaveUpdateAndLookup(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	req := model.SendRequest{AccountID: accID, Subject: "Hola", BodyHTML: "<p>x</p>", Attachments: []int64{3},
		SignatureID: 7, ComposeDraftID: 99, DraftMessageID: "<old@x>"}
	id, err := s.SaveComposeDraft(ctx, 0, accID, req, "<m1@x>")
	require.NoError(t, err)
	assert.NotZero(t, id)

	req.Subject = "Chau"
	id2, err := s.SaveComposeDraft(ctx, id, accID, req, "<m1@x>")
	require.NoError(t, err)
	assert.Equal(t, id, id2)

	d, err := s.ComposeDraftByMessageID(ctx, "<m1@x>")
	require.NoError(t, err)
	assert.Equal(t, id, d.ID)
	assert.Equal(t, req, d.Request)
	assert.Equal(t, "<m1@x>", d.MessageID)
	assert.True(t, d.NeedsUpload)
	assert.Empty(t, d.UploadError)
	got, err := s.GetComposeDraft(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, d, got)

	_, err = s.GetComposeDraft(ctx, 9999)
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.ComposeDraftByMessageID(ctx, "<nada>")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestComposeDrafts_SaveMissingIDIsNotFound(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	_, err := s.SaveComposeDraft(ctx, 42, accID, model.SendRequest{AccountID: accID}, "<z@x>")
	assert.ErrorIs(t, err, ErrNotFound)
	pending, err := s.ListPendingDrafts(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
}

func TestComposeDrafts_MarkUploadedIgnoresStale(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	id, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID}, "<m@x>")
	require.NoError(t, err)
	up, err := s.DraftUploadedAccount(ctx, id)
	require.NoError(t, err)
	assert.Zero(t, up, "nunca se subió")
	old, err := s.GetComposeDraft(ctx, id) // lo que lee la subida al empezar
	require.NoError(t, err)

	// Llega un guardado durante la subida.
	_, err = s.SaveComposeDraft(ctx, id, accID, model.SendRequest{AccountID: accID, Subject: "más nuevo"}, "<m@x>")
	require.NoError(t, err)
	cur, err := s.GetComposeDraft(ctx, id)
	require.NoError(t, err)
	assert.True(t, cur.UpdatedAt.After(old.UpdatedAt))

	require.NoError(t, s.MarkDraftUploaded(ctx, id, old.UpdatedAt, accID))
	cur, err = s.GetComposeDraft(ctx, id)
	require.NoError(t, err)
	assert.True(t, cur.NeedsUpload, "el guardado nuevo sigue pendiente")
	// La copia ya está en el servidor de la cuenta aunque siga pendiente.
	up, err = s.DraftUploadedAccount(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, accID, up)

	require.NoError(t, s.MarkDraftUploaded(ctx, id, cur.UpdatedAt, accID))
	cur, err = s.GetComposeDraft(ctx, id)
	require.NoError(t, err)
	assert.False(t, cur.NeedsUpload)
}

func TestComposeDrafts_PendingList(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	mk := func(mid string) int64 {
		id, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID}, mid)
		require.NoError(t, err)
		return id
	}
	a, b, c := mk("<a>"), mk("<b>"), mk("<c>")
	d, err := s.GetComposeDraft(ctx, b)
	require.NoError(t, err)
	require.NoError(t, s.MarkDraftUploaded(ctx, b, d.UpdatedAt, accID))
	require.NoError(t, s.MarkDraftUploadFailed(ctx, a, "sin red"))

	list, err := s.ListPendingDrafts(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, c, list[0].ID)
	assert.Equal(t, a, list[1].ID)
	assert.Equal(t, "sin red", list[1].UploadError)
	assert.True(t, list[1].NeedsUpload)

	// Un nuevo guardado limpia el error.
	_, err = s.SaveComposeDraft(ctx, a, accID, model.SendRequest{AccountID: accID}, "<a>")
	require.NoError(t, err)
	got, err := s.GetComposeDraft(ctx, a)
	require.NoError(t, err)
	assert.Empty(t, got.UploadError)

	require.NoError(t, s.DeleteComposeDraft(ctx, a))
	_, err = s.GetComposeDraft(ctx, a)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestComposeDrafts_DeleteAccountCascades(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	id, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID}, "<m@x>")
	require.NoError(t, err)
	require.NoError(t, s.DeleteAccount(ctx, accID))
	_, err = s.GetComposeDraft(ctx, id)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestPruneStaged_KeepsReferencedByComposeDraft(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	mk := func(name string) int64 {
		a, err := s.StageAttachment(ctx, name, "text/plain", []byte("x"))
		require.NoError(t, err)
		_, err = s.db.ExecContext(ctx, `UPDATE staged_attachments SET created_at = ? WHERE id = ?`,
			time.Now().Add(-48*time.Hour).Unix(), a.ID)
		require.NoError(t, err)
		return a.ID
	}
	referenced, loose := mk("ref"), mk("suelto")
	_, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID, Attachments: []int64{referenced}}, "<m@x>")
	require.NoError(t, err)
	// Un borrador con "attachments": null no debe frenar la limpieza.
	_, err = s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID}, "<n@x>")
	require.NoError(t, err)

	require.NoError(t, s.PruneStagedAttachments(ctx, time.Now().Add(-24*time.Hour)))
	info, err := s.StagedInfo(ctx, []int64{referenced, loose})
	require.NoError(t, err)
	require.Len(t, info, 1)
	assert.Equal(t, referenced, info[0].ID)
}

func TestComposeDrafts_DeleteWithStagedKeepsShared(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	stage := func(name string) int64 {
		a, err := s.StageAttachment(ctx, name, "text/plain", []byte(name))
		require.NoError(t, err)
		return a.ID
	}
	only, outbox, other := stage("solo"), stage("salida"), stage("otro")
	_, err := s.QueueLocalDraft(ctx, 0, model.SendRequest{AccountID: accID, Attachments: []int64{outbox}})
	require.NoError(t, err)
	_, err = s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID, Attachments: []int64{other}}, "<b@x>")
	require.NoError(t, err)
	id, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID, Attachments: []int64{only, outbox, other}}, "<a@x>")
	require.NoError(t, err)
	// Un borrador sin adjuntos (null en el JSON) tampoco rompe.
	empty, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID}, "<c@x>")
	require.NoError(t, err)

	require.NoError(t, s.DeleteComposeDraftWithStaged(ctx, id))
	require.NoError(t, s.DeleteComposeDraftWithStaged(ctx, empty))
	_, err = s.GetComposeDraft(ctx, id)
	require.ErrorIs(t, err, ErrNotFound)
	info, err := s.StagedInfo(ctx, []int64{only, outbox, other})
	require.NoError(t, err)
	got := []int64{}
	for _, a := range info {
		got = append(got, a.ID)
	}
	assert.Equal(t, []int64{outbox, other}, got)
}

// Un id borrado no se reutiliza: un id viejo que quedó en la bandeja de
// salida no apunta a un borrador nuevo.
func TestComposeDrafts_IDsAreNotReused(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, _ := seedFolder(t, s)
	id, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID}, "<a@x>")
	require.NoError(t, err)
	require.NoError(t, s.DeleteComposeDraft(ctx, id))
	id2, err := s.SaveComposeDraft(ctx, 0, accID, model.SendRequest{AccountID: accID}, "<b@x>")
	require.NoError(t, err)
	assert.Greater(t, id2, id)
	_, err = s.DraftUploadedAccount(ctx, id)
	require.ErrorIs(t, err, ErrNotFound)
}

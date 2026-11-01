package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"postacontrol/core/model"
)

// Los borradores del redactor (compose_drafts) se autoguardan acá y de ahí se
// suben a la carpeta Borradores del servidor. A diferencia de local_drafts (la
// bandeja de salida), updated_at va en nanosegundos y crece estrictamente en
// cada guardado, para que MarkDraftUploaded distinga un guardado que llegó
// durante la subida.

const composeDraftCols = `id, request_json, message_id, needs_upload, upload_error, updated_at`

func scanComposeDraft(sc interface{ Scan(...any) error }) (model.Draft, error) {
	var d model.Draft
	var js string
	var need int
	var ts int64
	if err := sc.Scan(&d.ID, &js, &d.MessageID, &need, &d.UploadError, &ts); err != nil {
		return model.Draft{}, err
	}
	if err := fromJSON(js, &d.Request); err != nil {
		return model.Draft{}, err
	}
	d.NeedsUpload = need != 0
	d.UpdatedAt = time.Unix(0, ts).UTC()
	return d, nil
}

// SaveComposeDraft crea (id == 0) o actualiza el borrador, y lo deja pendiente
// de subir y sin error. Con un id que ya no existe devuelve ErrNotFound: no
// resucita un borrador que se descartó.
func (s *Store) SaveComposeDraft(ctx context.Context, id int64, accountID int64, req model.SendRequest, messageID string) (int64, error) {
	js, err := toJSON(req)
	if err != nil {
		return 0, err
	}
	now := time.Now().UnixNano()
	if id == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO compose_drafts (account_id, request_json, message_id, needs_upload, upload_error, updated_at)
 VALUES (?,?,?,1,'',?)`, accountID, js, messageID, now)
		if err != nil {
			return 0, err
		}
		return res.LastInsertId()
	}
	// max(now, updated_at+1): estrictamente creciente aunque el reloj no avance.
	res, err := s.db.ExecContext(ctx, `UPDATE compose_drafts SET account_id = ?, request_json = ?, message_id = ?,
 needs_upload = 1, upload_error = '', updated_at = MAX(?, updated_at + 1) WHERE id = ?`, accountID, js, messageID, now, id)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotFound
	}
	return id, nil
}

func (s *Store) GetComposeDraft(ctx context.Context, id int64) (model.Draft, error) {
	d, err := scanComposeDraft(s.db.QueryRowContext(ctx, `SELECT `+composeDraftCols+` FROM compose_drafts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Draft{}, ErrNotFound
	}
	return d, err
}

func (s *Store) ComposeDraftByMessageID(ctx context.Context, messageID string) (model.Draft, error) {
	d, err := scanComposeDraft(s.db.QueryRowContext(ctx, `SELECT `+composeDraftCols+` FROM compose_drafts WHERE message_id = ?`, messageID))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Draft{}, ErrNotFound
	}
	return d, err
}

// ListPendingDrafts devuelve los borradores sin subir, el más nuevo primero.
func (s *Store) ListPendingDrafts(ctx context.Context) ([]model.Draft, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+composeDraftCols+` FROM compose_drafts WHERE needs_upload = 1 ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Draft{}
	for rows.Next() {
		d, err := scanComposeDraft(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// MarkDraftUploaded anota que la última copia del borrador está en la carpeta
// Borradores de accountID y lo da por subido solo si no cambió desde
// uploadedAt (el UpdatedAt que se leyó al empezar a subir): un guardado que
// llegó durante la subida sigue pendiente. La cuenta se anota siempre, porque
// la copia ya está en el servidor.
func (s *Store) MarkDraftUploaded(ctx context.Context, id int64, uploadedAt time.Time, accountID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE compose_drafts SET uploaded_account_id = ?,
 needs_upload = CASE WHEN updated_at <= ? THEN 0 ELSE needs_upload END,
 upload_error = CASE WHEN updated_at <= ? THEN '' ELSE upload_error END
 WHERE id = ?`, accountID, uploadedAt.UnixNano(), uploadedAt.UnixNano(), id)
	return err
}

// DraftUploadedAccount devuelve la cuenta en cuya carpeta Borradores está la
// última copia subida del borrador (0 si nunca se subió o la cuenta ya no
// existe).
func (s *Store) DraftUploadedAccount(ctx context.Context, id int64) (int64, error) {
	var acc sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT uploaded_account_id FROM compose_drafts WHERE id = ?`, id).Scan(&acc)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	return acc.Int64, nil
}

func (s *Store) MarkDraftUploadFailed(ctx context.Context, id int64, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE compose_drafts SET upload_error = ? WHERE id = ?`, msg, id)
	return err
}

// DeleteComposeDraft borra solo la fila local.
func (s *Store) DeleteComposeDraft(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM compose_drafts WHERE id = ?`, id)
	return err
}

// DeleteComposeDraftWithStaged borra el borrador y los adjuntos preparados que
// solo él referenciaba: los que usa un correo de la bandeja de salida u otro
// borrador quedan.
func (s *Store) DeleteComposeDraftWithStaged(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM staged_attachments WHERE id IN (
 SELECT j.value FROM compose_drafts c, json_each(c.request_json, '$.attachments') j WHERE c.id = ?)
 AND NOT EXISTS (
 SELECT 1 FROM local_drafts d, json_each(d.request_json, '$.attachments') j WHERE j.value = staged_attachments.id)
 AND NOT EXISTS (
 SELECT 1 FROM compose_drafts c, json_each(c.request_json, '$.attachments') j WHERE c.id <> ? AND j.value = staged_attachments.id)`,
		id, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM compose_drafts WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

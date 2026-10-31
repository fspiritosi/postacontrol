package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"postacontrol/core/model"
)

// Los borradores locales son la bandeja de salida: un correo entra antes de
// enviarse y se borra cuando el servidor lo acepta, así no se pierde lo
// escrito si el envío falla o la app se cierra en el medio.

// QueueLocalDraft pone req en la bandeja de salida como "sending". Con id != 0
// reusa esa fila (un reintento, quizá editado) salvo que ya se esté enviando
// (ErrBusy); si la fila ya no existe, crea otra. Devuelve el id final.
func (s *Store) QueueLocalDraft(ctx context.Context, id int64, req model.SendRequest) (int64, error) {
	req.DraftID = 0 // el id vive en la fila, no dentro del pedido
	js, err := toJSON(req)
	if err != nil {
		return 0, err
	}
	if id != 0 {
		var status string
		err := s.db.QueryRowContext(ctx, `SELECT status FROM local_drafts WHERE id = ?`, id).Scan(&status)
		switch {
		case err == nil && status == model.DraftSending:
			return 0, ErrBusy
		case err == nil:
			// La condición sobre status cierra la carrera entre dos reintentos.
			res, err := s.db.ExecContext(ctx, `UPDATE local_drafts SET account_id = ?, request_json = ?,
 status = ?, error = '' WHERE id = ? AND status <> ?`, req.AccountID, js, model.DraftSending, id, model.DraftSending)
			if err != nil {
				return 0, err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return 0, ErrBusy
			}
			return id, nil
		case !errors.Is(err, sql.ErrNoRows):
			return 0, err
		}
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO local_drafts (account_id, request_json, status, error, created_at)
 VALUES (?,?,?,'',?)`, req.AccountID, js, model.DraftSending, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FailLocalDraft marca el correo como no enviado, con el motivo.
func (s *Store) FailLocalDraft(ctx context.Context, id int64, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE local_drafts SET status = ?, error = ? WHERE id = ?`,
		model.DraftFailed, errMsg, id)
	return err
}

// FailInterruptedDrafts marca como fallidos los envíos que quedaron a medias
// (la app se cerró mientras salían). Se llama al abrir el núcleo.
func (s *Store) FailInterruptedDrafts(ctx context.Context, errMsg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE local_drafts SET status = ?, error = ? WHERE status = ?`,
		model.DraftFailed, errMsg, model.DraftSending)
	return err
}

func (s *Store) ListLocalDrafts(ctx context.Context) ([]model.LocalDraft, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, request_json, status, error, created_at FROM local_drafts ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LocalDraft{}
	for rows.Next() {
		var d model.LocalDraft
		var js string
		var ts int64
		if err := rows.Scan(&d.ID, &js, &d.Status, &d.Error, &ts); err != nil {
			return nil, err
		}
		if err := fromJSON(js, &d.Request); err != nil {
			return nil, err
		}
		d.CreatedAt = time.Unix(ts, 0).UTC()
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) DeleteLocalDraft(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_drafts WHERE id = ?`, id)
	return err
}

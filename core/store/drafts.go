package store

import (
	"context"
	"time"

	"postacontrol/core/model"
)

// SaveLocalDraft guarda un envío que no pudo completarse, para no perder lo escrito.
func (s *Store) SaveLocalDraft(ctx context.Context, req model.SendRequest, errMsg string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO local_drafts (account_id, request_json, error, created_at)
 VALUES (?,?,?,?)`, req.AccountID, toJSON(req), errMsg, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) ListLocalDrafts(ctx context.Context) ([]model.LocalDraft, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, request_json, error, created_at FROM local_drafts ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LocalDraft{}
	for rows.Next() {
		var d model.LocalDraft
		var js string
		var ts int64
		if err := rows.Scan(&d.ID, &js, &d.Error, &ts); err != nil {
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

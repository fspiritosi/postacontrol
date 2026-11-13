package store

import (
	"context"
	"database/sql"
	"errors"

	"postacontrol/core/model"
)

// InsertSnooze anota un correo pospuesto. Si ese Message-ID ya estaba
// pospuesto en la cuenta, se reemplaza (vale la hora nueva).
func (s *Store) InsertSnooze(ctx context.Context, z model.Snooze) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM snoozes WHERE account_id = ? AND message_id = ?`, z.AccountID, z.MessageID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO snoozes (account_id, message_id, return_folder, until) VALUES (?,?,?,?)`,
		z.AccountID, z.MessageID, z.ReturnFolder, z.Until); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) snoozes(ctx context.Context, where string, args ...any) ([]model.Snooze, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, account_id, message_id, return_folder, until FROM snoozes `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Snooze{}
	for rows.Next() {
		var z model.Snooze
		if err := rows.Scan(&z.ID, &z.AccountID, &z.MessageID, &z.ReturnFolder, &z.Until); err != nil {
			return nil, err
		}
		out = append(out, z)
	}
	return out, rows.Err()
}

// DueSnoozes devuelve los pospuestos cuya hora llegó (now en segundos unix).
func (s *Store) DueSnoozes(ctx context.Context, now int64) ([]model.Snooze, error) {
	return s.snoozes(ctx, `WHERE until <= ? ORDER BY until, id`, now)
}

// ListSnoozes devuelve todos los pospuestos, del que vuelve antes al último.
func (s *Store) ListSnoozes(ctx context.Context) ([]model.Snooze, error) {
	return s.snoozes(ctx, `ORDER BY until, id`)
}

// GetSnooze devuelve un pospuesto por id.
func (s *Store) GetSnooze(ctx context.Context, id int64) (model.Snooze, error) {
	zs, err := s.snoozes(ctx, `WHERE id = ?`, id)
	if err != nil {
		return model.Snooze{}, err
	}
	if len(zs) == 0 {
		return model.Snooze{}, ErrNotFound
	}
	return zs[0], nil
}

// SnoozeFor devuelve el pospuesto de ese Message-ID en la cuenta.
func (s *Store) SnoozeFor(ctx context.Context, accountID int64, messageID string) (model.Snooze, error) {
	var z model.Snooze
	err := s.db.QueryRowContext(ctx, `SELECT id, account_id, message_id, return_folder, until FROM snoozes
 WHERE account_id = ? AND message_id = ?`, accountID, messageID).Scan(&z.ID, &z.AccountID, &z.MessageID, &z.ReturnFolder, &z.Until)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Snooze{}, ErrNotFound
	}
	return z, err
}

// DeleteSnooze borra el pospuesto (ya volvió o se descartó).
func (s *Store) DeleteSnooze(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM snoozes WHERE id = ?`, id)
	return err
}

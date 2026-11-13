package store

import (
	"context"
	"database/sql"
	"errors"
)

// ListUnsub devuelve lo guardado sobre cómo desuscribirse del mensaje (JSON).
// known es false si todavía no se miró en el servidor.
func (s *Store) ListUnsub(ctx context.Context, messageID int64) (value string, known bool, err error) {
	var v sql.NullString
	err = s.db.QueryRowContext(ctx, `SELECT list_unsub FROM messages WHERE id = ?`, messageID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, ErrNotFound
	}
	return v.String, v.Valid, err
}

// SetListUnsub guarda cómo desuscribirse del mensaje.
func (s *Store) SetListUnsub(ctx context.Context, messageID int64, value string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE messages SET list_unsub = ? WHERE id = ?`, value, messageID)
	return err
}

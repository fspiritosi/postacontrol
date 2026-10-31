package store

import (
	"context"
	"database/sql"
	"errors"

	"postacontrol/core/model"
)

// GetSettings devuelve la configuración; el valor cero si todavía no hay fila.
func (s *Store) GetSettings(ctx context.Context) (model.Settings, error) {
	var out model.Settings
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT json FROM settings WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if err := fromJSON(raw, &out); err != nil {
		return model.Settings{}, err
	}
	return out, nil
}

// SaveSettings guarda la configuración en la fila única.
func (s *Store) SaveSettings(ctx context.Context, v model.Settings) error {
	raw, err := toJSON(v)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings (id, json) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET json = excluded.json`, raw)
	return err
}

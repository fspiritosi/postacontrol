package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"postacontrol/core/model"
)

func (s *Store) ListSignatures(ctx context.Context) ([]model.Signature, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, html FROM signatures ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Signature{}
	for rows.Next() {
		var g model.Signature
		if err := rows.Scan(&g.ID, &g.Name, &g.HTML); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) GetSignature(ctx context.Context, id int64) (model.Signature, error) {
	var g model.Signature
	err := s.db.QueryRowContext(ctx, `SELECT id, name, html FROM signatures WHERE id = ?`, id).Scan(&g.ID, &g.Name, &g.HTML)
	if errors.Is(err, sql.ErrNoRows) {
		return model.Signature{}, ErrNotFound
	}
	return g, err
}

// SaveSignature crea la firma si ID == 0 y si no la actualiza (ErrNotFound si no existe).
func (s *Store) SaveSignature(ctx context.Context, g model.Signature) (model.Signature, error) {
	now := time.Now().Unix()
	if g.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO signatures (name, html, updated_at) VALUES (?,?,?)`, g.Name, g.HTML, now)
		if err != nil {
			return model.Signature{}, err
		}
		g.ID, err = res.LastInsertId()
		return g, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE signatures SET name = ?, html = ?, updated_at = ? WHERE id = ?`, g.Name, g.HTML, now, g.ID)
	if err != nil {
		return model.Signature{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.Signature{}, ErrNotFound
	}
	return g, nil
}

// DeleteSignature borra la firma; las cuentas que la tenían por defecto quedan
// sin firma (ON DELETE SET NULL).
func (s *Store) DeleteSignature(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM signatures WHERE id = ?`, id)
	return err
}

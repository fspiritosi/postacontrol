package store

import (
	"context"
	"time"

	"postacontrol/core/model"
)

// ListTemplates devuelve las plantillas por nombre.
func (s *Store) ListTemplates(ctx context.Context) ([]model.Template, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, subject, html FROM templates ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Template{}
	for rows.Next() {
		var t model.Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Subject, &t.HTML); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SaveTemplate crea la plantilla si ID == 0 y si no la actualiza (ErrNotFound
// si no existe).
func (s *Store) SaveTemplate(ctx context.Context, t model.Template) (model.Template, error) {
	now := time.Now().Unix()
	if t.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO templates (name, subject, html, updated_at) VALUES (?,?,?,?)`,
			t.Name, t.Subject, t.HTML, now)
		if err != nil {
			return model.Template{}, err
		}
		t.ID, err = res.LastInsertId()
		return t, err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE templates SET name = ?, subject = ?, html = ?, updated_at = ? WHERE id = ?`,
		t.Name, t.Subject, t.HTML, now, t.ID)
	if err != nil {
		return model.Template{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.Template{}, ErrNotFound
	}
	return t, nil
}

// DeleteTemplate borra la plantilla; si no existe, no hace nada.
func (s *Store) DeleteTemplate(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM templates WHERE id = ?`, id)
	return err
}

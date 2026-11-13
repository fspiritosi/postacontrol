package store

import (
	"context"
	"strings"
	"time"
)

// TrustImageSender agrega el remitente a los que se les muestran siempre las
// imágenes remotas. email va en minúsculas; si ya estaba, no hace nada.
func (s *Store) TrustImageSender(ctx context.Context, email string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO image_senders (email, added_at) VALUES (?, ?)
 ON CONFLICT(email) DO NOTHING`, strings.ToLower(email), time.Now().Unix())
	return err
}

// UntrustImageSender lo saca de la lista (sin distinguir mayúsculas).
func (s *Store) UntrustImageSender(ctx context.Context, email string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM image_senders WHERE email = ?`, email)
	return err
}

// ListImageSenders devuelve la lista en orden alfabético.
func (s *Store) ListImageSenders(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT email FROM image_senders ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ImageSenderTrusted dice si el remitente está en la lista.
func (s *Store) ImageSenderTrusted(ctx context.Context, email string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM image_senders WHERE email = ?`, email).Scan(&n)
	return n > 0, err
}

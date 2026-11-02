package store

import (
	"context"
	"strings"
	"time"

	"postacontrol/core/model"
)

// Recipient es una dirección a la que ya escribí.
type Recipient struct {
	Email    string
	Name     string
	Times    int
	LastUsed int64
}

// RecordRecipients suma 1 a cada dirección (una vez por llamada aunque se repita),
// adelanta last_used a `at` si es mayor y guarda el nombre si viene no vacío.
func (s *Store) RecordRecipients(ctx context.Context, addrs []model.Address, at time.Time) error {
	type entry struct{ name string }
	seen := map[string]*entry{}
	order := []string{}
	for _, a := range addrs {
		email := strings.ToLower(strings.TrimSpace(a.Email))
		if !strings.Contains(email, "@") {
			continue
		}
		e, ok := seen[email]
		if !ok {
			e = &entry{}
			seen[email] = e
			order = append(order, email)
		}
		if a.Name != "" {
			e.name = a.Name
		}
	}
	if len(order) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, email := range order {
		if _, err := tx.ExecContext(ctx, `INSERT INTO recipients (email, name, times, last_used) VALUES (?,?,1,?)
 ON CONFLICT(email) DO UPDATE SET times = recipients.times + 1,
   last_used = max(recipients.last_used, excluded.last_used),
   name = CASE WHEN excluded.name <> '' THEN excluded.name ELSE recipients.name END`,
			email, seen[email].name, at.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListRecipients devuelve todos los destinatarios usados; el núcleo los ordena.
func (s *Store) ListRecipients(ctx context.Context) ([]Recipient, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT email, name, times, last_used FROM recipients ORDER BY email`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Recipient{}
	for rows.Next() {
		var r Recipient
		if err := rows.Scan(&r.Email, &r.Name, &r.Times, &r.LastUsed); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

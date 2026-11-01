package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"postacontrol/core/model"
)

// ErrEmailTaken avisa que un correo ya es de otra ficha (sin distinguir mayúsculas).
type ErrEmailTaken struct {
	Email     string
	ContactID int64
}

func (e ErrEmailTaken) Error() string {
	return fmt.Sprintf("el correo %s ya está en la ficha %d", e.Email, e.ContactID)
}

// ListContacts devuelve todas las fichas con sus correos en orden. El orden de
// las fichas (por id) lo reordena el núcleo según lo que muestre.
func (s *Store) ListContacts(ctx context.Context) ([]model.Contact, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, phones_json, company, notes FROM contacts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Contact{}
	byID := map[int64]int{}
	for rows.Next() {
		c, err := scanContact(rows)
		if err != nil {
			return nil, err
		}
		byID[c.ID] = len(out)
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	er, err := s.db.QueryContext(ctx, `SELECT contact_id, email FROM contact_emails ORDER BY contact_id, position`)
	if err != nil {
		return nil, err
	}
	defer er.Close()
	for er.Next() {
		var id int64
		var email string
		if err := er.Scan(&id, &email); err != nil {
			return nil, err
		}
		if i, ok := byID[id]; ok {
			out[i].Emails = append(out[i].Emails, email)
		}
	}
	return out, er.Err()
}

func (s *Store) GetContact(ctx context.Context, id int64) (model.Contact, error) {
	c, err := scanContact(s.db.QueryRowContext(ctx, `SELECT id, name, phones_json, company, notes FROM contacts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Contact{}, ErrNotFound
	}
	if err != nil {
		return model.Contact{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT email FROM contact_emails WHERE contact_id = ? ORDER BY position`, id)
	if err != nil {
		return model.Contact{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return model.Contact{}, err
		}
		c.Emails = append(c.Emails, e)
	}
	return c, rows.Err()
}

type rowScanner interface{ Scan(dest ...any) error }

func scanContact(r rowScanner) (model.Contact, error) {
	var c model.Contact
	var phones string
	if err := r.Scan(&c.ID, &c.Name, &phones, &c.Company, &c.Notes); err != nil {
		return model.Contact{}, err
	}
	c.Emails = []string{}
	c.Phones = []string{}
	if err := json.Unmarshal([]byte(phones), &c.Phones); err != nil {
		return model.Contact{}, err
	}
	if c.Phones == nil {
		c.Phones = []string{}
	}
	return c, nil
}

// SaveContact crea la ficha si ID == 0 y si no la actualiza (ErrNotFound si no
// existe). Reemplaza los correos en una transacción; si alguno ya es de otra
// ficha devuelve ErrEmailTaken y no guarda nada.
func (s *Store) SaveContact(ctx context.Context, c model.Contact) (model.Contact, error) {
	phones, err := toJSON(nonNil(c.Phones))
	if err != nil {
		return model.Contact{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Contact{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	if c.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO contacts (name, phones_json, company, notes, updated_at) VALUES (?,?,?,?,?)`,
			c.Name, phones, c.Company, c.Notes, now)
		if err != nil {
			return model.Contact{}, err
		}
		if c.ID, err = res.LastInsertId(); err != nil {
			return model.Contact{}, err
		}
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE contacts SET name = ?, phones_json = ?, company = ?, notes = ?, updated_at = ? WHERE id = ?`,
			c.Name, phones, c.Company, c.Notes, now, c.ID)
		if err != nil {
			return model.Contact{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return model.Contact{}, ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM contact_emails WHERE contact_id = ?`, c.ID); err != nil {
			return model.Contact{}, err
		}
	}
	for i, e := range c.Emails {
		var other int64
		err := tx.QueryRowContext(ctx, `SELECT contact_id FROM contact_emails WHERE email = ? COLLATE NOCASE`, e).Scan(&other)
		if err == nil {
			return model.Contact{}, ErrEmailTaken{Email: e, ContactID: other}
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return model.Contact{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO contact_emails (contact_id, position, email) VALUES (?,?,?)`, c.ID, i, e); err != nil {
			return model.Contact{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.Contact{}, err
	}
	c.Emails = append([]string{}, c.Emails...)
	c.Phones = nonNil(c.Phones)
	return c, nil
}

// DeleteContact borra la ficha; sus correos se van con ella (ON DELETE CASCADE).
func (s *Store) DeleteContact(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM contacts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ContactIDForEmail busca la ficha dueña del correo, sin distinguir mayúsculas.
func (s *Store) ContactIDForEmail(ctx context.Context, email string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT contact_id FROM contact_emails WHERE email = ? COLLATE NOCASE`, email).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

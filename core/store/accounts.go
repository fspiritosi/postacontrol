package store

import (
	"context"
	"database/sql"
	"errors"

	"postacontrol/core/model"
)

const accountCols = `id, email, display_name, username, color, imap_host, imap_port, imap_security,
 smtp_host, smtp_port, smtp_security, save_sent_copy`

func scanAccount(sc interface{ Scan(...any) error }) (model.Account, error) {
	var a model.Account
	var imapSec, smtpSec string
	var save int
	err := sc.Scan(&a.ID, &a.Email, &a.DisplayName, &a.Username, &a.Color,
		&a.IMAP.Host, &a.IMAP.Port, &imapSec, &a.SMTP.Host, &a.SMTP.Port, &smtpSec, &save)
	a.IMAP.Security = model.Security(imapSec)
	a.SMTP.Security = model.Security(smtpSec)
	a.SaveSentCopy = save != 0
	return a, err
}

func (s *Store) InsertAccount(ctx context.Context, a model.Account) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO accounts
 (email, display_name, username, color, imap_host, imap_port, imap_security,
  smtp_host, smtp_port, smtp_security, save_sent_copy)
 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		a.Email, a.DisplayName, a.Username, a.Color, a.IMAP.Host, a.IMAP.Port, string(a.IMAP.Security),
		a.SMTP.Host, a.SMTP.Port, string(a.SMTP.Security), b2i(a.SaveSentCopy))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) GetAccount(ctx context.Context, id int64) (model.Account, error) {
	a, err := scanAccount(s.db.QueryRowContext(ctx, `SELECT `+accountCols+` FROM accounts WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Account{}, ErrNotFound
	}
	return a, err
}

func (s *Store) ListAccounts(ctx context.Context) ([]model.Account, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountCols+` FROM accounts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Account{}
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAccount borra la cuenta; carpetas, mensajes y borradores caen por ON DELETE CASCADE.
func (s *Store) DeleteAccount(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSaveSentCopy cambia si se guarda la copia de lo enviado en Enviados.
func (s *Store) SetSaveSentCopy(ctx context.Context, id int64, save bool) error {
	res, err := s.db.ExecContext(ctx, `UPDATE accounts SET save_sent_copy = ? WHERE id = ?`, b2i(save), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

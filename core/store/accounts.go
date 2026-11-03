package store

import (
	"context"
	"database/sql"
	"errors"

	"postacontrol/core/model"
)

const accountCols = `id, email, display_name, username, color, imap_host, imap_port, imap_security,
 smtp_host, smtp_port, smtp_security, save_sent_copy, COALESCE(default_signature_id, 0)`

func scanAccount(sc interface{ Scan(...any) error }) (model.Account, error) {
	var a model.Account
	var imapSec, smtpSec string
	var save int
	err := sc.Scan(&a.ID, &a.Email, &a.DisplayName, &a.Username, &a.Color,
		&a.IMAP.Host, &a.IMAP.Port, &imapSec, &a.SMTP.Host, &a.SMTP.Port, &smtpSec, &save, &a.DefaultSignatureID)
	a.IMAP.Security = model.Security(imapSec)
	a.SMTP.Security = model.Security(smtpSec)
	a.SaveSentCopy = save != 0
	return a, err
}

func (s *Store) InsertAccount(ctx context.Context, a model.Account) (int64, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO accounts
 (email, display_name, username, color, imap_host, imap_port, imap_security,
  smtp_host, smtp_port, smtp_security, save_sent_copy, position)
 VALUES (?,?,?,?,?,?,?,?,?,?,?, (SELECT COALESCE(MAX(position), 0) + 1 FROM accounts))`,
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
	rows, err := s.db.QueryContext(ctx, `SELECT `+accountCols+` FROM accounts ORDER BY position, id`)
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

// SetAccountSignature fija la firma por defecto de la cuenta; 0 la deja sin firma.
func (s *Store) SetAccountSignature(ctx context.Context, accountID, signatureID int64) error {
	var sig any // NULL cuando no hay firma
	if signatureID != 0 {
		sig = signatureID
	}
	res, err := s.db.ExecContext(ctx, `UPDATE accounts SET default_signature_id = ? WHERE id = ?`, sig, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAccount guarda todos los datos de la cuenta salvo el id.
// DefaultSignatureID 0 la deja sin firma.
func (s *Store) UpdateAccount(ctx context.Context, a model.Account) error {
	var sig any // NULL cuando no hay firma
	if a.DefaultSignatureID != 0 {
		sig = a.DefaultSignatureID
	}
	res, err := s.db.ExecContext(ctx, `UPDATE accounts SET
 email = ?, display_name = ?, username = ?, color = ?, imap_host = ?, imap_port = ?, imap_security = ?,
 smtp_host = ?, smtp_port = ?, smtp_security = ?, save_sent_copy = ?, default_signature_id = ?
 WHERE id = ?`,
		a.Email, a.DisplayName, a.Username, a.Color, a.IMAP.Host, a.IMAP.Port, string(a.IMAP.Security),
		a.SMTP.Host, a.SMTP.Port, string(a.SMTP.Security), b2i(a.SaveSentCopy), sig, a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetAccountCache borra en una transacción la caché de la cuenta: sus
// carpetas (los mensajes, el índice y las etiquetas de los mensajes caen por
// cascada y triggers) y sus limpiezas y traslados de etiquetas pendientes. La
// cuenta vuelve a admitir etiquetas en el servidor. Las definiciones de
// etiquetas y los borradores locales no se tocan.
func (s *Store) ResetAccountCache(ctx context.Context, accountID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE accounts SET labels_local = 0 WHERE id = ?`, accountID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	for _, q := range []string{
		`DELETE FROM folders WHERE account_id = ?`,
		`DELETE FROM label_cleanup WHERE account_id = ?`,
		`DELETE FROM label_carry WHERE account_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetAccountPositions ordena las cuentas según ids (posición 1, 2, …) en una transacción.
func (s *Store) SetAccountPositions(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE accounts SET position = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

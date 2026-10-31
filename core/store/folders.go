package store

import (
	"context"
	"database/sql"
	"errors"

	"postacontrol/core/model"
)

// FolderSync es el estado de sincronización IMAP de una carpeta. Cero = nunca sincronizada.
type FolderSync struct{ UIDValidity, UIDNext uint32 }

const folderCols = `id, account_id, name, delimiter, role, unread, total, selectable, uid_validity, uid_next`

func scanFolder(sc interface{ Scan(...any) error }) (model.Folder, FolderSync, error) {
	var f model.Folder
	var fs FolderSync
	var role string
	var sel int
	err := sc.Scan(&f.ID, &f.AccountID, &f.Name, &f.Delimiter, &role, &f.Unread, &f.Total, &sel,
		&fs.UIDValidity, &fs.UIDNext)
	f.Role = model.FolderRole(role)
	f.Selectable = sel != 0
	return f, fs, err
}

// ReplaceFolders sincroniza la lista de carpetas de la cuenta en una transacción: actualiza por
// (account_id, name) conservando ID, contadores y estado de sync, inserta las nuevas y borra las ausentes.
func (s *Store) ReplaceFolders(ctx context.Context, accountID int64, fs []model.Folder) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	seen := make(map[string]bool, len(fs))
	for _, f := range fs {
		seen[f.Name] = true
		if _, err := tx.ExecContext(ctx, `INSERT INTO folders (account_id, name, delimiter, role, selectable)
 VALUES (?,?,?,?,?)
 ON CONFLICT(account_id, name) DO UPDATE SET
   delimiter = excluded.delimiter, role = excluded.role, selectable = excluded.selectable`,
			accountID, f.Name, f.Delimiter, string(f.Role), b2i(f.Selectable)); err != nil {
			return err
		}
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM folders WHERE account_id = ?`, accountID)
	if err != nil {
		return err
	}
	var stale []int64
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			_ = rows.Close()
			return err
		}
		if !seen[name] {
			stale = append(stale, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range stale {
		if _, err := tx.ExecContext(ctx, `DELETE FROM folders WHERE id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListFolders devuelve las carpetas ordenadas por nombre, con los contadores del último STATUS guardado.
func (s *Store) ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+folderCols+` FROM folders WHERE account_id = ? ORDER BY name`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Folder{}
	for rows.Next() {
		f, _, err := scanFolder(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s *Store) GetFolder(ctx context.Context, id int64) (model.Folder, FolderSync, error) {
	f, fs, err := scanFolder(s.db.QueryRowContext(ctx, `SELECT `+folderCols+` FROM folders WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Folder{}, FolderSync{}, ErrNotFound
	}
	return f, fs, err
}

func (s *Store) execFolder(ctx context.Context, folderID int64, q string, args ...any) error {
	res, err := s.db.ExecContext(ctx, q, append(args, folderID)...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetFolderSync(ctx context.Context, folderID int64, fs FolderSync) error {
	return s.execFolder(ctx, folderID, `UPDATE folders SET uid_validity = ?, uid_next = ? WHERE id = ?`,
		fs.UIDValidity, fs.UIDNext)
}

func (s *Store) SetFolderCounts(ctx context.Context, folderID int64, unread, total int) error {
	return s.execFolder(ctx, folderID, `UPDATE folders SET unread = ?, total = ? WHERE id = ?`, unread, total)
}

// ResetFolder borra los mensajes de la carpeta y deja FolderSync en cero (p. ej. si cambió UIDVALIDITY).
func (s *Store) ResetFolder(ctx context.Context, folderID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE folder_id = ?`, folderID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE folders SET uid_validity = 0, uid_next = 0 WHERE id = ?`, folderID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

// InboxUnreadTotal suma los no leídos de las carpetas con rol inbox de todas las cuentas.
func (s *Store) InboxUnreadTotal(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(unread), 0) FROM folders WHERE role = 'inbox'`).Scan(&n)
	return n, err
}

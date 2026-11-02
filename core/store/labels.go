package store

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"strings"

	"postacontrol/core/model"
)

// parseLabelIDs convierte "posición:id,posición:id" en ids ordenados por
// posición y después id (group_concat no garantiza orden); "" da []int64{}.
func parseLabelIDs(s string) []int64 {
	type pi struct{ pos, id int64 }
	ps := []pi{}
	if s != "" {
		for _, p := range strings.Split(s, ",") {
			a, b, ok := strings.Cut(p, ":")
			if !ok {
				continue
			}
			pos, e1 := strconv.ParseInt(a, 10, 64)
			id, e2 := strconv.ParseInt(b, 10, 64)
			if e1 == nil && e2 == nil {
				ps = append(ps, pi{pos, id})
			}
		}
	}
	slices.SortFunc(ps, func(x, y pi) int {
		if c := cmp.Compare(x.pos, y.pos); c != 0 {
			return c
		}
		return cmp.Compare(x.id, y.id)
	})
	out := make([]int64, len(ps))
	for i, p := range ps {
		out[i] = p.id
	}
	return out
}

// execer lo cumplen *sql.DB y *sql.Tx.
type execer interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func keywordMap(ctx context.Context, q execer) (map[string]int64, error) {
	rows, err := q.QueryContext(ctx, `SELECT keyword, id FROM labels`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var id int64
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		out[k] = id
	}
	return out, rows.Err()
}

// replaceKeywordLabels deja en la fila exactamente las etiquetas de los
// keywords conocidos; los desconocidos (también los $pc_…) se ignoran.
func replaceKeywordLabels(ctx context.Context, tx *sql.Tx, rowID int64, keywords []string, kw map[string]int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_labels WHERE message_id = ?`, rowID); err != nil {
		return err
	}
	for _, k := range keywords {
		id, ok := kw[k]
		if !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO message_labels (message_id, label_id) VALUES (?, ?)`, rowID, id); err != nil {
			return err
		}
	}
	return nil
}

// applyCarry pasa a la fila nueva las etiquetas guardadas para su Message-ID y las consume.
func applyCarry(ctx context.Context, tx *sql.Tx, accountID, rowID int64, messageID string) error {
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO message_labels (message_id, label_id)
 SELECT ?, label_id FROM label_carry WHERE account_id = ? AND message_id = ?`, rowID, accountID, messageID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM label_carry WHERE account_id = ? AND message_id = ?`, accountID, messageID)
	return err
}

// ListLabels devuelve las etiquetas por posición y después id.
func (s *Store) ListLabels(ctx context.Context) ([]model.Label, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, color, position FROM labels ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Label{}
	for rows.Next() {
		var l model.Label
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &l.Position); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// InsertLabel crea la etiqueta al final del orden.
func (s *Store) InsertLabel(ctx context.Context, name, color, keyword string) (model.Label, error) {
	l := model.Label{Name: name, Color: color}
	err := s.db.QueryRowContext(ctx, `INSERT INTO labels (name, color, keyword, position)
 VALUES (?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM labels)) RETURNING id, position`,
		name, color, keyword).Scan(&l.ID, &l.Position)
	return l, err
}

// UpdateLabel cambia solo nombre y color; el keyword no se toca nunca.
func (s *Store) UpdateLabel(ctx context.Context, l model.Label) error {
	res, err := s.db.ExecContext(ctx, `UPDATE labels SET name = ?, color = ? WHERE id = ?`, l.Name, l.Color, l.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetLabelPositions ordena las etiquetas según ids (posición 1, 2, …).
func (s *Store) SetLabelPositions(ctx context.Context, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE labels SET position = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) LabelKeyword(ctx context.Context, id int64) (string, error) {
	var k string
	err := s.db.QueryRowContext(ctx, `SELECT keyword FROM labels WHERE id = ?`, id).Scan(&k)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return k, err
}

// LabelByKeyword devuelve el id de cada etiqueta por su keyword.
func (s *Store) LabelByKeyword(ctx context.Context) (map[string]int64, error) {
	return keywordMap(ctx, s.db)
}

// DeleteLabel borra la etiqueta (message_labels y label_carry caen en cascada) y
// deja anotado el keyword para limpiar en cada cuenta de cleanupAccounts.
func (s *Store) DeleteLabel(ctx context.Context, id int64, cleanupAccounts []int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	var keyword string
	if err := tx.QueryRowContext(ctx, `SELECT keyword FROM labels WHERE id = ?`, id).Scan(&keyword); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM labels WHERE id = ?`, id); err != nil {
		return "", err
	}
	for _, acc := range cleanupAccounts {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO label_cleanup (account_id, keyword) VALUES (?, ?)`, acc, keyword); err != nil {
			return "", err
		}
	}
	return keyword, tx.Commit()
}

// CountLabeled cuenta los mensajes que tienen la etiqueta.
func (s *Store) CountLabeled(ctx context.Context, labelID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM message_labels WHERE label_id = ?`, labelID).Scan(&n)
	return n, err
}

// SetMessageLabels reemplaza el conjunto de etiquetas del mensaje.
func (s *Store) SetMessageLabels(ctx context.Context, messageID int64, labelIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM message_labels WHERE message_id = ?`, messageID); err != nil {
		return err
	}
	for _, l := range labelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO message_labels (message_id, label_id) VALUES (?, ?)`, messageID, l); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// AddMessageLabel pone (on) o saca la etiqueta de todos los mensajes ids.
func (s *Store) AddMessageLabel(ctx context.Context, ids []int64, labelID int64, on bool) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	q := `DELETE FROM message_labels WHERE message_id = ? AND label_id = ?`
	if on {
		q = `INSERT OR IGNORE INTO message_labels (message_id, label_id) VALUES (?, ?)`
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, q, id, labelID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListLabeled lista los mensajes con la etiqueta; accountID 0 = todas las cuentas.
func (s *Store) ListLabeled(ctx context.Context, labelID, accountID int64, q model.ListQuery) ([]model.MessageSummary, error) {
	where := `m.id IN (SELECT message_id FROM message_labels WHERE label_id = ?)`
	args := []any{labelID}
	if accountID != 0 {
		where += ` AND f.account_id = ?`
		args = append(args, accountID)
	}
	return s.listWhere(ctx, where, args, q)
}

// PendingLabelCleanups devuelve los keywords que faltan limpiar en la cuenta.
func (s *Store) PendingLabelCleanups(ctx context.Context, accountID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT keyword FROM label_cleanup WHERE account_id = ? ORDER BY keyword`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) DoneLabelCleanup(ctx context.Context, accountID int64, keyword string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM label_cleanup WHERE account_id = ? AND keyword = ?`, accountID, keyword)
	return err
}

// AccountLabelsLocal dice si la cuenta guarda las etiquetas solo en esta PC.
func (s *Store) AccountLabelsLocal(ctx context.Context, accountID int64) (bool, error) {
	var v int
	err := s.db.QueryRowContext(ctx, `SELECT labels_local FROM accounts WHERE id = ?`, accountID).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrNotFound
	}
	return v != 0, err
}

// SetAccountLabelsLocal marca la cuenta como local; changed solo en el paso de 0 a 1.
func (s *Store) SetAccountLabelsLocal(ctx context.Context, accountID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE accounts SET labels_local = 1 WHERE id = ? AND labels_local = 0`, accountID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// SaveLabelCarry guarda las etiquetas de un mensaje (por Message-ID) para
// reaplicarlas cuando reaparezca en otra carpeta de la cuenta.
func (s *Store) SaveLabelCarry(ctx context.Context, accountID int64, messageID string, labelIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, l := range labelIDs {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO label_carry (account_id, message_id, label_id) VALUES (?, ?, ?)`,
			accountID, messageID, l); err != nil {
			return err
		}
	}
	return tx.Commit()
}

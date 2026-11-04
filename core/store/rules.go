package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"postacontrol/core/model"
)

const ruleCols = `id, account_id, name, enabled, match_all, conditions_json, actions_json, position`

func scanRule(sc interface{ Scan(...any) error }) (model.Rule, error) {
	var r model.Rule
	var enabled, matchAll int
	var conds, acts string
	if err := sc.Scan(&r.ID, &r.AccountID, &r.Name, &enabled, &matchAll, &conds, &acts, &r.Position); err != nil {
		return model.Rule{}, err
	}
	r.Enabled, r.MatchAll = enabled != 0, matchAll != 0
	r.Conditions, r.Actions = []model.RuleCondition{}, []model.RuleAction{}
	if err := json.Unmarshal([]byte(conds), &r.Conditions); err != nil {
		return model.Rule{}, err
	}
	if err := json.Unmarshal([]byte(acts), &r.Actions); err != nil {
		return model.Rule{}, err
	}
	if r.Conditions == nil {
		r.Conditions = []model.RuleCondition{}
	}
	if r.Actions == nil {
		r.Actions = []model.RuleAction{}
	}
	return r, nil
}

// ListRules devuelve las reglas de la cuenta en su orden.
func (s *Store) ListRules(ctx context.Context, accountID int64) ([]model.Rule, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+ruleCols+` FROM rules WHERE account_id = ? ORDER BY position, id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Rule{}
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetRule(ctx context.Context, id int64) (model.Rule, error) {
	r, err := scanRule(s.db.QueryRowContext(ctx, `SELECT `+ruleCols+` FROM rules WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return model.Rule{}, ErrNotFound
	}
	return r, err
}

// SaveRule crea la regla al final del orden de su cuenta si ID == 0 y si no la
// actualiza sin cambiar ni la cuenta ni la posición (ErrNotFound si no existe).
func (s *Store) SaveRule(ctx context.Context, r model.Rule) (model.Rule, error) {
	conds, err := json.Marshal(r.Conditions)
	if err != nil {
		return model.Rule{}, err
	}
	acts, err := json.Marshal(r.Actions)
	if err != nil {
		return model.Rule{}, err
	}
	if r.ID == 0 {
		err := s.db.QueryRowContext(ctx, `INSERT INTO rules (account_id, name, enabled, match_all, conditions_json, actions_json, position)
 VALUES (?, ?, ?, ?, ?, ?, (SELECT COALESCE(MAX(position), 0) + 1 FROM rules WHERE account_id = ?)) RETURNING id`,
			r.AccountID, r.Name, b2i(r.Enabled), b2i(r.MatchAll), string(conds), string(acts), r.AccountID).Scan(&r.ID)
		if err != nil {
			return model.Rule{}, err
		}
		return s.GetRule(ctx, r.ID)
	}
	res, err := s.db.ExecContext(ctx, `UPDATE rules SET name = ?, enabled = ?, match_all = ?, conditions_json = ?, actions_json = ?
 WHERE id = ?`, r.Name, b2i(r.Enabled), b2i(r.MatchAll), string(conds), string(acts), r.ID)
	if err != nil {
		return model.Rule{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.Rule{}, ErrNotFound
	}
	return s.GetRule(ctx, r.ID)
}

func (s *Store) DeleteRule(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM rules WHERE id = ?`, id)
	return err
}

// ReorderRules pone las reglas de la cuenta en el orden de ids. Las que no
// son de la cuenta se ignoran.
func (s *Store) ReorderRules(ctx context.Context, accountID int64, ids []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for i, id := range ids {
		if _, err := tx.ExecContext(ctx, `UPDATE rules SET position = ? WHERE id = ? AND account_id = ?`, i+1, id, accountID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// RuleMessage es lo que las reglas miran de un correo.
type RuleMessage struct {
	ID      int64
	UID     uint32
	Seen    bool
	From    model.Address
	To, Cc  []model.Address
	Subject string
}

// RuleMessages devuelve los correos de la carpeta con esos UID, o todos si
// uids es nil, por UID ascendente (el orden de llegada).
func (s *Store) RuleMessages(ctx context.Context, folderID int64, uids []uint32) ([]RuleMessage, error) {
	const q = `SELECT id, uid, seen, from_name, from_email, to_json, cc_json, subject FROM messages WHERE folder_id = ?`
	if uids == nil {
		return s.ruleMessages(ctx, q+` ORDER BY uid`, folderID)
	}
	out := []RuleMessage{}
	const chunk = 500
	for i := 0; i < len(uids); i += chunk {
		part := uids[i:min(i+chunk, len(uids))]
		args := make([]any, 0, len(part)+1)
		args = append(args, folderID)
		for _, u := range part {
			args = append(args, int64(u))
		}
		ms, err := s.ruleMessages(ctx, q+` AND uid IN (?`+strings.Repeat(`,?`, len(part)-1)+`) ORDER BY uid`, args...)
		if err != nil {
			return nil, err
		}
		out = append(out, ms...)
	}
	// Los tramos llegan en el orden de uids, que puede no ser ascendente.
	slices.SortFunc(out, func(a, b RuleMessage) int { return int(a.UID) - int(b.UID) })
	return out, nil
}

func (s *Store) ruleMessages(ctx context.Context, q string, args ...any) ([]RuleMessage, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RuleMessage{}
	for rows.Next() {
		var m RuleMessage
		var seen int
		var to, cc string
		if err := rows.Scan(&m.ID, &m.UID, &seen, &m.From.Name, &m.From.Email, &to, &cc, &m.Subject); err != nil {
			return nil, err
		}
		m.Seen = seen != 0
		m.To, m.Cc = []model.Address{}, []model.Address{}
		if err := json.Unmarshal([]byte(to), &m.To); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cc), &m.Cc); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

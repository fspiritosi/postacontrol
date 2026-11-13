package store

import (
	"context"
	"strings"
	"unicode"

	"postacontrol/core/model"
)

// ftsTerm convierte un pedazo del texto del usuario en una expresión MATCH
// segura: cada palabra entre comillas (así los operadores y símbolos de FTS5
// son literales) con prefijo, unidas con AND. "" si no hay palabras.
func ftsTerm(text string) string {
	words := strings.Fields(text)
	parts := make([]string, 0, len(words))
	for _, w := range words {
		if !strings.ContainsFunc(w, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			continue
		}
		parts = append(parts, `"`+strings.ReplaceAll(w, `"`, `""`)+`"*`)
	}
	return strings.Join(parts, " AND ")
}

// ftsColumn es ftsTerm limitado a una columna del índice.
func ftsColumn(col, text string) string {
	if e := ftsTerm(text); e != "" {
		return col + " : (" + e + ")"
	}
	return ""
}

// searchWhere arma el WHERE de una búsqueda con sus filtros. ok es false si
// no queda nada que buscar.
func searchWhere(t SearchTerms) (where string, args []any, ok bool) {
	var fts []string
	add := func(e string) {
		if e != "" {
			fts = append(fts, e)
		}
	}
	for _, w := range t.Words {
		add(ftsTerm(w))
	}
	for _, v := range t.From {
		add(ftsColumn("sender", v))
	}
	for _, v := range t.To {
		add(ftsColumn("recipients", v))
	}
	for _, v := range t.Subject {
		add(ftsColumn("subject", v))
	}
	var conds []string
	if len(fts) > 0 {
		conds = append(conds, `m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)`)
		args = append(args, strings.Join(fts, " AND "))
	}
	if t.HasAttachment {
		conds = append(conds, `m.has_attachments = 1`)
	}
	if t.Unread {
		conds = append(conds, `m.seen = 0`)
	}
	if t.Flagged {
		conds = append(conds, `m.flagged = 1`)
	}
	if !t.Since.IsZero() {
		conds = append(conds, `m.date >= ?`)
		args = append(args, t.Since.Unix())
	}
	if !t.Before.IsZero() {
		conds = append(conds, `m.date < ?`)
		args = append(args, t.Before.Unix())
	}
	// Sin nada que buscar (ni palabras útiles ni filtros) no devuelve todo.
	if len(conds) == 0 {
		return "", nil, false
	}
	return strings.Join(conds, " AND "), args, true
}

// Search busca en el índice local, por fecha descendente. El texto admite los
// filtros de ParseSearch.
func (s *Store) Search(ctx context.Context, q model.SearchQuery) ([]model.MessageSummary, error) {
	where, args, ok := searchWhere(ParseSearch(q.Text))
	if !ok {
		return []model.MessageSummary{}, nil
	}
	if q.FolderID != 0 {
		where += ` AND m.folder_id = ?`
		args = append(args, q.FolderID)
	}
	if q.LabelID != 0 {
		where += ` AND m.id IN (SELECT message_id FROM message_labels WHERE label_id = ?)`
		args = append(args, q.LabelID)
	}
	return s.listWhere(ctx, where, args, model.ListQuery{Limit: q.Limit, Offset: q.Offset})
}

// PendingBodies cuenta los mensajes sin cuerpo indexado (y que no agotaron los
// reintentos) de una carpeta, o de todas si folderID es 0.
func (s *Store) PendingBodies(ctx context.Context, folderID int64) (int, error) {
	query := `SELECT COUNT(*) FROM messages WHERE body_cached = 0 AND index_failures < 3`
	args := []any{}
	if folderID != 0 {
		query += ` AND folder_id = ?`
		args = append(args, folderID)
	}
	var n int
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&n)
	return n, err
}

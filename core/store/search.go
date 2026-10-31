package store

import (
	"context"
	"strings"

	"postacontrol/core/model"
)

// ftsQuery convierte el texto del usuario en una expresión MATCH segura: cada
// palabra va entre comillas (así los operadores y símbolos de FTS5 son
// literales) con prefijo, unidas con AND. "" si no hay palabras.
func ftsQuery(text string) string {
	words := strings.Fields(text)
	parts := make([]string, 0, len(words))
	for _, w := range words {
		parts = append(parts, `"`+strings.ReplaceAll(w, `"`, `""`)+`"*`)
	}
	return strings.Join(parts, " AND ")
}

// Search busca en el índice local, por fecha descendente.
func (s *Store) Search(ctx context.Context, q model.SearchQuery) ([]model.MessageSummary, error) {
	expr := ftsQuery(q.Text)
	if expr == "" {
		return []model.MessageSummary{}, nil
	}
	where := `m.id IN (SELECT rowid FROM messages_fts WHERE messages_fts MATCH ?)`
	args := []any{expr}
	if q.FolderID != 0 {
		where += ` AND m.folder_id = ?`
		args = append(args, q.FolderID)
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

package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"

	"postacontrol/core/model"
)

// threadRoot devuelve la raíz del hilo: el primer valor no vacío entre
// References[0], In-Reply-To, Message-ID y "id:<rowID>".
func threadRoot(messageID, inReplyTo string, references []string, rowID int64) string {
	if len(references) > 0 && references[0] != "" {
		return references[0]
	}
	if inReplyTo != "" {
		return inReplyTo
	}
	if messageID != "" {
		return messageID
	}
	if rowID == 0 {
		return "" // UpsertHeaders lo completa con el id real tras insertar
	}
	return "id:" + strconv.FormatInt(rowID, 10)
}

// threadScope devuelve las carpetas que cuentan para un hilo visto desde
// folderID: ella misma y la de Enviados de su cuenta (si existe).
func (s *Store) threadScope(ctx context.Context, folderID int64) ([]any, error) {
	var sent int64
	err := s.db.QueryRowContext(ctx, `SELECT s.id FROM folders f JOIN folders s ON s.account_id = f.account_id AND s.role = 'sent'
 WHERE f.id = ? LIMIT 1`, folderID).Scan(&sent)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && sent == folderID) {
		return []any{folderID}, nil
	}
	if err != nil {
		return nil, err
	}
	return []any{folderID, sent}, nil
}

func inList(n int) string {
	if n == 1 {
		return "(?)"
	}
	return "(?,?)"
}

// threadsSQL arma la consulta de ListThreads para un alcance de scopeLen
// carpetas. Parámetros: el alcance dos veces, la carpeta, limit y offset.
func threadsSQL(scopeLen int, unreadOnly bool) string {
	in := inList(scopeLen)
	query := `SELECT * FROM (SELECT ` + summaryCols + `, m.thread_root,
 (SELECT COUNT(*) FROM messages x WHERE x.thread_root = m.thread_root AND x.folder_id IN ` + in + `) AS cnt,
 (SELECT COUNT(*) FROM messages x WHERE x.thread_root = m.thread_root AND x.folder_id IN ` + in + ` AND x.seen = 0) AS unread
 FROM messages m JOIN folders f ON f.id = m.folder_id
 WHERE m.folder_id = ? AND m.id = (SELECT y.id FROM messages y WHERE y.folder_id = m.folder_id AND y.thread_root = m.thread_root
                                   ORDER BY y.date DESC, y.id DESC LIMIT 1))`
	if unreadOnly {
		query += ` WHERE unread > 0`
	}
	return query + ` ORDER BY date DESC, id DESC LIMIT ? OFFSET ?`
}

// ListThreads agrupa los mensajes de la carpeta por hilo, del más reciente al
// más antiguo. Count y Unread incluyen los mensajes del hilo en Enviados.
func (s *Store) ListThreads(ctx context.Context, folderID int64, q model.ListQuery) ([]model.ThreadSummary, error) {
	scope, err := s.threadScope(ctx, folderID)
	if err != nil {
		return nil, err
	}
	query := threadsSQL(len(scope), q.UnreadOnly)
	limit, offset := -1, 0
	if q.Limit > 0 {
		limit = q.Limit
	}
	if q.Offset > 0 {
		offset = q.Offset
	}
	args := append(append([]any{}, scope...), scope...)
	args = append(args, folderID, limit, offset)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	out := []model.ThreadSummary{}
	for rows.Next() {
		var t model.ThreadSummary
		t.Latest, err = scanSummary(rows, &t.ThreadRoot, &t.Count, &t.Unread)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for i := range out {
		out[i].Participants, err = s.participants(ctx, out[i].ThreadRoot, scope)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// participants: remitentes distintos del hilo (nombre, o email si no hay), máximo 3, el más reciente primero.
func (s *Store) participants(ctx context.Context, root string, scope []any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT from_name, from_email FROM messages
 WHERE thread_root = ? AND folder_id IN `+inList(len(scope))+` ORDER BY date DESC, id DESC`, append([]any{root}, scope...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	seen := map[string]bool{}
	for rows.Next() && len(out) < 3 {
		var name, email string
		if err := rows.Scan(&name, &email); err != nil {
			return nil, err
		}
		if name == "" {
			name = email
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	return out, rows.Err()
}

// ThreadMessages devuelve los mensajes del hilo en la carpeta y en Enviados de
// su cuenta, por fecha ascendente, sin repetir.
func (s *Store) ThreadMessages(ctx context.Context, folderID int64, root string) ([]model.MessageSummary, error) {
	scope, err := s.threadScope(ctx, folderID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+summaryCols+` FROM messages m JOIN folders f ON f.id = m.folder_id
 WHERE m.thread_root = ? AND m.folder_id IN `+inList(len(scope))+` ORDER BY m.date ASC, m.id ASC`, append([]any{root}, scope...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.MessageSummary{}
	for rows.Next() {
		m, err := scanSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

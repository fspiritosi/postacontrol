package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"postacontrol/core/model"
)

// HeaderRow es lo que se guarda al sincronizar cabeceras (sin cuerpo).
type HeaderRow struct {
	model.MessageSummary
	MessageID, InReplyTo string
	References           []string
	To, Cc, ReplyTo      []model.Address
	Size                 int64
}

// BodyRow es el cuerpo ya procesado (HTML sanitizado) que se cachea al abrir un correo.
type BodyRow struct {
	HTML, Text, Snippet string
	HasRemoteImages     bool
	Attachments         []model.Attachment
}

// UpsertHeaders guarda el lote en una sola transacción: o entra todo o no entra nada.
// Si el mensaje ya existe actualiza cabeceras y flags, y conserva el cuerpo cacheado.
func (s *Store) UpsertHeaders(ctx context.Context, folderID int64, rows []HeaderRow) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO messages
 (folder_id, uid, message_id, in_reply_to, references_json, from_name, from_email, to_json, cc_json,
  reply_to_json, subject, snippet, date, size, seen, flagged, answered, has_attachments)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(folder_id, uid) DO UPDATE SET
   message_id = excluded.message_id, in_reply_to = excluded.in_reply_to,
   references_json = excluded.references_json, from_name = excluded.from_name,
   from_email = excluded.from_email, to_json = excluded.to_json, cc_json = excluded.cc_json,
   reply_to_json = excluded.reply_to_json, subject = excluded.subject,
   snippet = CASE WHEN excluded.snippet = '' THEN messages.snippet ELSE excluded.snippet END,
   date = excluded.date, size = excluded.size, seen = excluded.seen, flagged = excluded.flagged,
   answered = excluded.answered,
   has_attachments = CASE WHEN messages.body_cached = 1 THEN messages.has_attachments
                          ELSE excluded.has_attachments END`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		refs, err := toJSON(nonNil(r.References))
		if err != nil {
			return err
		}
		to, err := toJSON(nonNilAddr(r.To))
		if err != nil {
			return err
		}
		cc, err := toJSON(nonNilAddr(r.Cc))
		if err != nil {
			return err
		}
		replyTo, err := toJSON(nonNilAddr(r.ReplyTo))
		if err != nil {
			return err
		}
		if _, err := stmt.ExecContext(ctx, folderID, r.UID, r.MessageID, r.InReplyTo, refs,
			r.From.Name, r.From.Email, to, cc, replyTo,
			r.Subject, r.Snippet, r.Date.Unix(), r.Size, b2i(r.Seen), b2i(r.Flagged), b2i(r.Answered),
			b2i(r.HasAttachments)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilAddr(a []model.Address) []model.Address {
	if a == nil {
		return []model.Address{}
	}
	return a
}

func (s *Store) KnownUIDs(ctx context.Context, folderID int64) ([]uint32, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT uid FROM messages WHERE folder_id = ? ORDER BY uid`, folderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uint32{}
	for rows.Next() {
		var u uint32
		if err := rows.Scan(&u); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// DeleteUIDsNotIn borra los mensajes de la carpeta cuyo UID no está en keep (vacío = borra todos).
func (s *Store) DeleteUIDsNotIn(ctx context.Context, folderID int64, keep []uint32) error {
	known, err := s.KnownUIDs(ctx, folderID)
	if err != nil {
		return err
	}
	keepSet := make(map[uint32]bool, len(keep))
	for _, u := range keep {
		keepSet[u] = true
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, u := range known {
		if keepSet[u] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE folder_id = ? AND uid = ?`, folderID, u); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetFlags(ctx context.Context, folderID int64, uid uint32, seen, flagged, answered bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE messages SET seen = ?, flagged = ?, answered = ?
 WHERE folder_id = ? AND uid = ?`, b2i(seen), b2i(flagged), b2i(answered), folderID, uid)
	return err
}

const summaryCols = `m.id, f.account_id, m.folder_id, m.uid, m.from_name, m.from_email, m.subject, m.snippet,
 m.date, m.seen, m.flagged, m.answered, m.has_attachments`

func scanSummary(sc interface{ Scan(...any) error }, extra ...any) (model.MessageSummary, error) {
	var m model.MessageSummary
	var date int64
	var seen, flagged, answered, hasAtt int
	dest := append([]any{&m.ID, &m.AccountID, &m.FolderID, &m.UID, &m.From.Name, &m.From.Email, &m.Subject,
		&m.Snippet, &date, &seen, &flagged, &answered, &hasAtt}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return m, err
	}
	m.Date = time.Unix(date, 0).UTC()
	m.Seen, m.Flagged, m.Answered, m.HasAttachments = seen != 0, flagged != 0, answered != 0, hasAtt != 0
	return m, nil
}

// ListMessages devuelve los mensajes de la carpeta por fecha descendente. Limit <= 0 = sin tope.
func (s *Store) ListMessages(ctx context.Context, folderID int64, q model.ListQuery) ([]model.MessageSummary, error) {
	var sb strings.Builder
	sb.WriteString(`SELECT ` + summaryCols + ` FROM messages m JOIN folders f ON f.id = m.folder_id WHERE m.folder_id = ?`)
	if q.UnreadOnly {
		sb.WriteString(` AND m.seen = 0`)
	}
	sb.WriteString(` ORDER BY m.date DESC, m.id DESC`)
	limit, offset := -1, 0
	if q.Limit > 0 {
		limit = q.Limit
	}
	if q.Offset > 0 {
		offset = q.Offset
	}
	sb.WriteString(` LIMIT ? OFFSET ?`)
	rows, err := s.db.QueryContext(ctx, sb.String(), folderID, limit, offset)
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

// GetMessage devuelve el detalle y si el cuerpo ya está cacheado.
func (s *Store) GetMessage(ctx context.Context, id int64) (model.MessageDetail, bool, error) {
	var refs, to, cc, replyTo, atts string
	var cached, remote int
	var d model.MessageDetail
	sum, err := scanSummary(s.db.QueryRowContext(ctx, `SELECT `+summaryCols+`,
 m.message_id, m.in_reply_to, m.references_json, m.to_json, m.cc_json, m.reply_to_json,
 m.body_html, m.body_text, m.has_remote_images, m.attachments_json, m.body_cached
 FROM messages m JOIN folders f ON f.id = m.folder_id WHERE m.id = ?`, id),
		&d.MessageID, &d.InReplyTo, &refs, &to, &cc, &replyTo, &d.HTML, &d.Text, &remote, &atts, &cached)
	if errors.Is(err, sql.ErrNoRows) {
		return d, false, ErrNotFound
	}
	if err != nil {
		return d, false, err
	}
	d.MessageSummary = sum
	d.HasRemoteImages = remote != 0
	d.References, d.To, d.Cc, d.ReplyTo, d.Attachments = []string{}, []model.Address{}, []model.Address{}, []model.Address{}, []model.Attachment{}
	for _, p := range []struct {
		src string
		dst any
	}{{refs, &d.References}, {to, &d.To}, {cc, &d.Cc}, {replyTo, &d.ReplyTo}, {atts, &d.Attachments}} {
		if err := fromJSON(p.src, p.dst); err != nil {
			return d, false, err
		}
	}
	return d, cached != 0, nil
}

func (s *Store) SetBody(ctx context.Context, id int64, b BodyRow) error {
	atts := b.Attachments
	if atts == nil {
		atts = []model.Attachment{}
	}
	attsJSON, err := toJSON(atts)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE messages SET body_html = ?, body_text = ?, has_remote_images = ?,
 attachments_json = ?, has_attachments = ?, body_cached = 1,
 snippet = CASE WHEN ? = '' THEN snippet ELSE ? END
 WHERE id = ?`,
		b.HTML, b.Text, b2i(b.HasRemoteImages), attsJSON, b2i(len(atts) > 0), b.Snippet, b.Snippet, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

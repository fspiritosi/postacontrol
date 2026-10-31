package store

import (
	"context"
	"strings"
	"time"

	"postacontrol/core/model"
)

// StagedFile es un adjunto preparado con su contenido.
type StagedFile struct {
	model.StagedAttachment
	Data []byte
}

// StageAttachment guarda un archivo a la espera del envío.
func (s *Store) StageAttachment(ctx context.Context, filename, mimeType string, data []byte) (model.StagedAttachment, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO staged_attachments (filename, mime_type, size, data, created_at)
 VALUES (?,?,?,?,?)`, filename, mimeType, len(data), data, time.Now().Unix())
	if err != nil {
		return model.StagedAttachment{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.StagedAttachment{}, err
	}
	return model.StagedAttachment{ID: id, Filename: filename, MIMEType: mimeType, Size: int64(len(data))}, nil
}

// idsClause arma "?,?,?" y los argumentos para un IN (...).
func idsClause(ids []int64) (string, []any) {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","), args
}

// StagedAttachments devuelve los adjuntos con su contenido, en el orden de
// ids. ErrNotFound si falta alguno.
func (s *Store) StagedAttachments(ctx context.Context, ids []int64) ([]StagedFile, error) {
	if len(ids) == 0 {
		return []StagedFile{}, nil
	}
	in, args := idsClause(ids)
	rows, err := s.db.QueryContext(ctx, `SELECT id, filename, mime_type, size, data FROM staged_attachments WHERE id IN (`+in+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]StagedFile{}
	for rows.Next() {
		var f StagedFile
		if err := rows.Scan(&f.ID, &f.Filename, &f.MIMEType, &f.Size, &f.Data); err != nil {
			return nil, err
		}
		byID[f.ID] = f
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]StagedFile, 0, len(ids))
	for _, id := range ids {
		f, ok := byID[id]
		if !ok {
			return nil, ErrNotFound
		}
		out = append(out, f)
	}
	return out, nil
}

// StagedInfo devuelve los datos de los adjuntos sin su contenido, en el orden
// de ids; los que no existen se omiten.
func (s *Store) StagedInfo(ctx context.Context, ids []int64) ([]model.StagedAttachment, error) {
	out := []model.StagedAttachment{}
	if len(ids) == 0 {
		return out, nil
	}
	in, args := idsClause(ids)
	rows, err := s.db.QueryContext(ctx, `SELECT id, filename, mime_type, size FROM staged_attachments WHERE id IN (`+in+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[int64]model.StagedAttachment{}
	for rows.Next() {
		var a model.StagedAttachment
		if err := rows.Scan(&a.ID, &a.Filename, &a.MIMEType, &a.Size); err != nil {
			return nil, err
		}
		byID[a.ID] = a
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range ids {
		if a, ok := byID[id]; ok {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s *Store) DeleteStagedAttachments(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	in, args := idsClause(ids)
	_, err := s.db.ExecContext(ctx, `DELETE FROM staged_attachments WHERE id IN (`+in+`)`, args...)
	return err
}

// PruneStagedAttachments borra los adjuntos creados antes de olderThan que
// ningún correo de la bandeja de salida referencia: son restos de redactores
// cancelados. (La fase 3B suma a este filtro los borradores.)
func (s *Store) PruneStagedAttachments(ctx context.Context, olderThan time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM staged_attachments WHERE created_at < ? AND id NOT IN (
 SELECT j.value FROM local_drafts d, json_each(d.request_json, '$.attachments') j)`, olderThan.Unix())
	return err
}

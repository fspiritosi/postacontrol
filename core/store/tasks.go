package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"postacontrol/core/model"
)

const localTaskCols = `id, title, notes, due_date, due_time, remind_before, source_message_id, source_subject, done_at, created_at`

func scanLocalTask(r rowScanner) (model.LocalTask, error) {
	var t model.LocalTask
	var remind, done sql.NullInt64
	var created int64
	if err := r.Scan(&t.ID, &t.Title, &t.Notes, &t.DueDate, &t.DueTime, &remind, &t.SourceMessageID, &t.SourceSubject, &done, &created); err != nil {
		return model.LocalTask{}, err
	}
	if remind.Valid {
		v := int(remind.Int64)
		t.RemindBefore = &v
	}
	if done.Valid {
		d := time.Unix(done.Int64, 0)
		t.DoneAt = &d
	}
	t.CreatedAt = time.Unix(created, 0)
	return t, nil
}

func (s *Store) localTasks(ctx context.Context, where string) ([]model.LocalTask, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+localTaskCols+` FROM local_tasks `+where)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.LocalTask{}
	for rows.Next() {
		t, err := scanLocalTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListLocalTasks devuelve las tareas locales: primero las pendientes, con
// fecha antes que sin fecha, y las hechas al final (solo si includeDone).
func (s *Store) ListLocalTasks(ctx context.Context, includeDone bool) ([]model.LocalTask, error) {
	where := ``
	if !includeDone {
		where = `WHERE done_at IS NULL `
	}
	return s.localTasks(ctx, where+`ORDER BY done_at IS NOT NULL, due_date = '', due_date, due_time, id`)
}

// GetLocalTask devuelve una tarea; sql.ErrNoRows si no está.
func (s *Store) GetLocalTask(ctx context.Context, id int64) (model.LocalTask, error) {
	return scanLocalTask(s.db.QueryRowContext(ctx, `SELECT `+localTaskCols+` FROM local_tasks WHERE id = ?`, id))
}

// SaveLocalTask da de alta (ID 0) o edita una tarea. El alta pone created_at;
// la edición lo conserva y no toca done_at. rearm deja reminded_at en NULL
// para que el aviso vuelva a dispararse (cambió la fecha, la hora o el margen).
func (s *Store) SaveLocalTask(ctx context.Context, t model.LocalTask, rearm bool) (model.LocalTask, error) {
	var remind any
	if t.RemindBefore != nil {
		remind = *t.RemindBefore
	}
	if t.ID == 0 {
		res, err := s.db.ExecContext(ctx, `INSERT INTO local_tasks (title, notes, due_date, due_time, remind_before, source_message_id, source_subject, created_at)
 VALUES (?,?,?,?,?,?,?,?)`, t.Title, t.Notes, t.DueDate, t.DueTime, remind, t.SourceMessageID, t.SourceSubject, time.Now().Unix())
		if err != nil {
			return model.LocalTask{}, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return model.LocalTask{}, err
		}
		return s.GetLocalTask(ctx, id)
	}
	q := `UPDATE local_tasks SET title=?, notes=?, due_date=?, due_time=?, remind_before=?, source_message_id=?, source_subject=?`
	if rearm {
		q += `, reminded_at = NULL`
	}
	res, err := s.db.ExecContext(ctx, q+` WHERE id = ?`, t.Title, t.Notes, t.DueDate, t.DueTime, remind, t.SourceMessageID, t.SourceSubject, t.ID)
	if err != nil {
		return model.LocalTask{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return model.LocalTask{}, sql.ErrNoRows
	}
	return s.GetLocalTask(ctx, t.ID)
}

// SetLocalTaskDone marca la tarea como hecha en at, o la reabre con nil.
func (s *Store) SetLocalTaskDone(ctx context.Context, id int64, at *time.Time) error {
	var v any
	if at != nil {
		v = at.Unix()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE local_tasks SET done_at = ? WHERE id = ?`, v, id)
	return err
}

// DeleteLocalTask borra la tarea. Si no existe no es un error.
func (s *Store) DeleteLocalTask(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM local_tasks WHERE id = ?`, id)
	return err
}

// PendingReminders devuelve las tareas con hora, sin hacer y aún sin avisar.
func (s *Store) PendingReminders(ctx context.Context) ([]model.LocalTask, error) {
	return s.localTasks(ctx, `WHERE due_time != '' AND done_at IS NULL AND reminded_at IS NULL ORDER BY due_date, due_time, id`)
}

// MarkReminded anota que ya se avisó de la tarea, solo si sigue como t (la
// foto con la que se decidió avisar): sin hacer, sin avisar y con la misma
// fecha, hora y anticipación. Devuelve false si cambió o ya no está, así un
// aviso rearmado por una edición no se pierde.
func (s *Store) MarkReminded(ctx context.Context, t model.LocalTask, at time.Time) (bool, error) {
	var remind any
	if t.RemindBefore != nil {
		remind = *t.RemindBefore
	}
	res, err := s.db.ExecContext(ctx, `UPDATE local_tasks SET reminded_at = ?
 WHERE id = ? AND reminded_at IS NULL AND done_at IS NULL AND due_date = ? AND due_time = ? AND remind_before IS ?`,
		at.Unix(), t.ID, t.DueDate, t.DueTime, remind)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

const taskLinkCols = `id, thread_root, task_id, ticket, project_name, title, attach_pending, created_at`

func scanTaskLink(r rowScanner) (model.TaskLink, error) {
	var l model.TaskLink
	var ticket, pending, created int64
	if err := r.Scan(&l.ID, &l.ThreadRoot, &l.TaskID, &ticket, &l.ProjectName, &l.Title, &pending, &created); err != nil {
		return model.TaskLink{}, err
	}
	l.Ticket = ticket != 0
	l.AttachPending = pending != 0
	l.CreatedAt = time.Unix(created, 0)
	return l, nil
}

// AddTaskLink vincula el hilo con una tarea de Taskapp. sourceMessageID es el
// Message-ID del correo que originó el vínculo; queda como ya sumado.
func (s *Store) AddTaskLink(ctx context.Context, l model.TaskLink, sourceMessageID string) (model.TaskLink, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.TaskLink{}, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `INSERT INTO task_links (thread_root, task_id, ticket, project_name, title, source_message_id, attach_pending, created_at)
 VALUES (?,?,?,?,?,?,?,?)`, l.ThreadRoot, l.TaskID, b2i(l.Ticket), l.ProjectName, l.Title, sourceMessageID, b2i(l.AttachPending), time.Now().Unix())
	if err != nil {
		return model.TaskLink{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return model.TaskLink{}, err
	}
	if sourceMessageID != "" {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO task_link_messages (link_id, message_id) VALUES (?,?)`, id, sourceMessageID); err != nil {
			return model.TaskLink{}, err
		}
	}
	out, err := scanTaskLink(tx.QueryRowContext(ctx, `SELECT `+taskLinkCols+` FROM task_links WHERE id = ?`, id))
	if err != nil {
		return model.TaskLink{}, err
	}
	return out, tx.Commit()
}

// TaskLinksForThread devuelve los vínculos del hilo, del más viejo al más nuevo.
func (s *Store) TaskLinksForThread(ctx context.Context, root string) ([]model.TaskLink, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+taskLinkCols+` FROM task_links WHERE thread_root = ? ORDER BY created_at, id`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.TaskLink{}
	for rows.Next() {
		l, err := scanTaskLink(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetTaskLink devuelve un vínculo; sql.ErrNoRows si no está.
func (s *Store) GetTaskLink(ctx context.Context, id int64) (model.TaskLink, error) {
	return scanTaskLink(s.db.QueryRowContext(ctx, `SELECT `+taskLinkCols+` FROM task_links WHERE id = ?`, id))
}

// SetAttachPending marca si falta adjuntar el .eml a la tarea del vínculo.
func (s *Store) SetAttachPending(ctx context.Context, linkID int64, pending bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE task_links SET attach_pending = ? WHERE id = ?`, b2i(pending), linkID)
	return err
}

// MarkAddedToLink anota que el correo ya se sumó al vínculo (idempotente).
func (s *Store) MarkAddedToLink(ctx context.Context, linkID int64, messageID string) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO task_link_messages (link_id, message_id) VALUES (?,?)`, linkID, messageID)
	return err
}

// AddedToLink dice si el correo ya se sumó al vínculo.
func (s *Store) AddedToLink(ctx context.Context, linkID int64, messageID string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_link_messages WHERE link_id = ? AND message_id = ?`, linkID, messageID).Scan(&n)
	return n > 0, err
}

// MessageTaskMeta devuelve el Message-ID, la raíz del hilo y el tamaño del
// correo. Sin raíz de hilo, la raíz es "local:<id>". sql.ErrNoRows si no está.
func (s *Store) MessageTaskMeta(ctx context.Context, id int64) (headerID, threadRoot string, size int64, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT message_id, thread_root, size FROM messages WHERE id = ?`, id).Scan(&headerID, &threadRoot, &size)
	if err != nil {
		return "", "", 0, err
	}
	if threadRoot == "" {
		threadRoot = "local:" + strconv.FormatInt(id, 10)
	}
	return headerID, threadRoot, size, nil
}

// MessageIDByHeader busca el correo más nuevo con ese Message-ID.
func (s *Store) MessageIDByHeader(ctx context.Context, headerID string) (int64, error) {
	if headerID == "" {
		return 0, sql.ErrNoRows
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM messages WHERE message_id = ? ORDER BY date DESC, id DESC LIMIT 1`, headerID).Scan(&id)
	return id, err
}

// SaveTaskappCache guarda (reemplaza) la última lista traída de Taskapp.
func (s *Store) SaveTaskappCache(ctx context.Context, json string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO taskapp_cache (id, json, fetched_at) VALUES (1, ?, ?)
 ON CONFLICT(id) DO UPDATE SET json = excluded.json, fetched_at = excluded.fetched_at`, json, at.Unix())
	return err
}

// TaskappCache devuelve la caché; sin fila, ok es false y err nil.
func (s *Store) TaskappCache(ctx context.Context) (json string, at time.Time, ok bool, err error) {
	var ts int64
	err = s.db.QueryRowContext(ctx, `SELECT json, fetched_at FROM taskapp_cache WHERE id = 1`).Scan(&json, &ts)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("leer caché de Taskapp: %w", err)
	}
	return json, time.Unix(ts, 0), true, nil
}

// ClearTaskappData borra la caché; los vínculos se conservan.
func (s *Store) ClearTaskappData(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM taskapp_cache`)
	return err
}

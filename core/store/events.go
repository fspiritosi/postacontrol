package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"postacontrol/core/model"
)

const eventCols = `id, uid, kind, title, description, notes, location, all_day, start_at, end_at, tzid, rrule,
 remind_before, account_id, organizer_name, organizer_email, sequence, status, my_partstat, my_email,
 alias_reply, needs_review, raw_ics, taskapp_meeting_id, taskapp_change`

func scanEvent(r rowScanner) (model.CalendarEvent, error) {
	var e model.CalendarEvent
	var allDay, alias, review int
	var start, end int64
	var remind, account, meeting sql.NullInt64
	if err := r.Scan(&e.ID, &e.UID, &e.Kind, &e.Title, &e.Description, &e.Notes, &e.Location, &allDay, &start, &end,
		&e.TZID, &e.RRule, &remind, &account, &e.Organizer.Name, &e.Organizer.Email, &e.Sequence, &e.Status,
		&e.MyPartstat, &e.MyEmail, &alias, &review, &e.RawICS, &meeting, &e.TaskappChange); err != nil {
		return model.CalendarEvent{}, err
	}
	e.AllDay, e.AliasReply, e.NeedsReview = allDay != 0, alias != 0, review != 0
	e.Start, e.End = time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC()
	if remind.Valid {
		v := int(remind.Int64)
		e.RemindBefore = &v
	}
	e.AccountID = account.Int64
	e.TaskappMeetingID = meeting.Int64
	e.Attendees = []model.Attendee{}
	e.Exceptions = []model.EventException{}
	return e, nil
}

// loadEventChildren completa invitados y excepciones de los eventos.
func (s *Store) loadEventChildren(ctx context.Context, evs []model.CalendarEvent) error {
	if len(evs) == 0 {
		return nil
	}
	idx := make(map[int64]int, len(evs))
	ph, ids := "", make([]any, 0, len(evs))
	for i, e := range evs {
		idx[e.ID] = i
		if i > 0 {
			ph += ","
		}
		ph += "?"
		ids = append(ids, e.ID)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT event_id, email, name, partstat FROM event_attendees WHERE event_id IN (`+ph+`) ORDER BY rowid`, ids...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		var a model.Attendee
		if err := rows.Scan(&id, &a.Email, &a.Name, &a.Partstat); err != nil {
			_ = rows.Close()
			return err
		}
		if i, ok := idx[id]; ok {
			evs[i].Attendees = append(evs[i].Attendees, a)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT event_id, recurrence_at, cancelled, start_at, end_at, title, location, description
 FROM event_exceptions WHERE event_id IN (`+ph+`) ORDER BY recurrence_at`, ids...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, rec, start, end int64
		var cancelled int
		var x model.EventException
		if err := rows.Scan(&id, &rec, &cancelled, &start, &end, &x.Title, &x.Location, &x.Description); err != nil {
			return err
		}
		x.RecurrenceAt, x.Cancelled, x.Start, x.End = time.Unix(rec, 0).UTC(), cancelled != 0, time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC()
		if i, ok := idx[id]; ok {
			evs[i].Exceptions = append(evs[i].Exceptions, x)
		}
	}
	return rows.Err()
}

func (s *Store) queryEvents(ctx context.Context, where string, args ...any) ([]model.CalendarEvent, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+eventCols+` FROM events `+where, args...)
	if err != nil {
		return nil, err
	}
	out := []model.CalendarEvent{}
	for rows.Next() {
		e, err := scanEvent(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := s.loadEventChildren(ctx, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) oneEvent(ctx context.Context, where string, arg any) (model.CalendarEvent, error) {
	evs, err := s.queryEvents(ctx, where, arg)
	if err != nil {
		return model.CalendarEvent{}, err
	}
	if len(evs) == 0 {
		return model.CalendarEvent{}, ErrNotFound
	}
	return evs[0], nil
}

func (s *Store) GetEvent(ctx context.Context, id int64) (model.CalendarEvent, error) {
	return s.oneEvent(ctx, `WHERE id = ?`, id)
}

func (s *Store) EventByUID(ctx context.Context, uid string) (model.CalendarEvent, error) {
	return s.oneEvent(ctx, `WHERE uid = ?`, uid)
}

func (s *Store) EventByMeeting(ctx context.Context, meetingID int64) (model.CalendarEvent, error) {
	return s.oneEvent(ctx, `WHERE taskapp_meeting_id = ?`, meetingID)
}

// AllEvents devuelve todos los eventos, por inicio.
func (s *Store) AllEvents(ctx context.Context) ([]model.CalendarEvent, error) {
	return s.queryEvents(ctx, `ORDER BY start_at, id`)
}

// EventsInRange devuelve los eventos que pueden tener ocurrencias en [from, to):
// los simples que se superponen y las series que empezaron antes de to y no
// terminaron antes de from (expandirlas es cosa del llamador).
func (s *Store) EventsInRange(ctx context.Context, from, to time.Time) ([]model.CalendarEvent, error) {
	return s.queryEvents(ctx, `WHERE (rrule = '' AND start_at < ? AND (end_at > ? OR start_at >= ?))
 OR (rrule <> '' AND start_at < ? AND (series_end_at IS NULL OR series_end_at > ?))
 ORDER BY start_at, id`, to.Unix(), from.Unix(), from.Unix(), to.Unix(), from.Unix())
}

// SaveEvent da de alta (ID 0) o edita un evento y reemplaza sus invitados y
// excepciones, todo en una transacción. seriesEnd es el fin de la última
// ocurrencia (nil si la serie no termina o no hay serie).
func (s *Store) SaveEvent(ctx context.Context, e model.CalendarEvent, seriesEnd *time.Time) (model.CalendarEvent, error) {
	var remind, account, meeting, series any
	if e.RemindBefore != nil {
		remind = *e.RemindBefore
	}
	if e.AccountID != 0 {
		account = e.AccountID
	}
	if e.TaskappMeetingID != 0 {
		meeting = e.TaskappMeetingID
	}
	if seriesEnd != nil {
		series = seriesEnd.Unix()
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.CalendarEvent{}, err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().Unix()
	if e.ID == 0 {
		res, err := tx.ExecContext(ctx, `INSERT INTO events (uid, kind, title, description, notes, location, all_day, start_at, end_at,
 tzid, rrule, series_end_at, remind_before, account_id, organizer_name, organizer_email, sequence, status, my_partstat,
 my_email, alias_reply, needs_review, raw_ics, taskapp_meeting_id, taskapp_change, created_at, updated_at)
 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			e.UID, e.Kind, e.Title, e.Description, e.Notes, e.Location, b2i(e.AllDay), e.Start.Unix(), e.End.Unix(),
			e.TZID, e.RRule, series, remind, account, e.Organizer.Name, e.Organizer.Email, e.Sequence, e.Status, e.MyPartstat,
			e.MyEmail, b2i(e.AliasReply), b2i(e.NeedsReview), e.RawICS, meeting, e.TaskappChange, now, now)
		if err != nil {
			return model.CalendarEvent{}, err
		}
		if e.ID, err = res.LastInsertId(); err != nil {
			return model.CalendarEvent{}, err
		}
	} else {
		res, err := tx.ExecContext(ctx, `UPDATE events SET uid=?, kind=?, title=?, description=?, notes=?, location=?, all_day=?,
 start_at=?, end_at=?, tzid=?, rrule=?, series_end_at=?, remind_before=?, account_id=?, organizer_name=?, organizer_email=?,
 sequence=?, status=?, my_partstat=?, my_email=?, alias_reply=?, needs_review=?, raw_ics=?, taskapp_meeting_id=?,
 taskapp_change=?, updated_at=? WHERE id=?`,
			e.UID, e.Kind, e.Title, e.Description, e.Notes, e.Location, b2i(e.AllDay), e.Start.Unix(), e.End.Unix(),
			e.TZID, e.RRule, series, remind, account, e.Organizer.Name, e.Organizer.Email, e.Sequence, e.Status, e.MyPartstat,
			e.MyEmail, b2i(e.AliasReply), b2i(e.NeedsReview), e.RawICS, meeting, e.TaskappChange, now, e.ID)
		if err != nil {
			return model.CalendarEvent{}, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return model.CalendarEvent{}, ErrNotFound
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM event_attendees WHERE event_id = ?`, e.ID); err != nil {
			return model.CalendarEvent{}, err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM event_exceptions WHERE event_id = ?`, e.ID); err != nil {
			return model.CalendarEvent{}, err
		}
	}
	for _, a := range dedupeAttendees(e.Attendees) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO event_attendees (event_id, email, name, partstat) VALUES (?,?,?,?)`,
			e.ID, a.Email, a.Name, a.Partstat); err != nil {
			return model.CalendarEvent{}, err
		}
	}
	for _, x := range dedupeExceptions(e.Exceptions) {
		if _, err := tx.ExecContext(ctx, `INSERT INTO event_exceptions (event_id, recurrence_at, cancelled, start_at, end_at, title, location, description)
 VALUES (?,?,?,?,?,?,?,?)`, e.ID, x.RecurrenceAt.Unix(), b2i(x.Cancelled), x.Start.Unix(), x.End.Unix(), x.Title, x.Location, x.Description); err != nil {
			return model.CalendarEvent{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return model.CalendarEvent{}, err
	}
	return s.GetEvent(ctx, e.ID)
}

// dedupeAttendees normaliza los emails y, si se repiten, se queda con el último
// (en la posición del primero).
func dedupeAttendees(in []model.Attendee) []model.Attendee {
	pos := map[string]int{}
	out := make([]model.Attendee, 0, len(in))
	for _, a := range in {
		a.Email = strings.ToLower(strings.TrimSpace(a.Email))
		if i, ok := pos[a.Email]; ok {
			out[i] = a
			continue
		}
		pos[a.Email] = len(out)
		out = append(out, a)
	}
	return out
}

// dedupeExceptions se queda con la última excepción de cada recurrence_at.
func dedupeExceptions(in []model.EventException) []model.EventException {
	pos := map[int64]int{}
	out := make([]model.EventException, 0, len(in))
	for _, x := range in {
		k := x.RecurrenceAt.Unix()
		if i, ok := pos[k]; ok {
			out[i] = x
			continue
		}
		pos[k] = len(out)
		out = append(out, x)
	}
	return out
}

func (s *Store) DeleteEvent(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM events WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetAttendeePartstat cambia la respuesta de un invitado; false si no está.
func (s *Store) SetAttendeePartstat(ctx context.Context, eventID int64, email, partstat string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE event_attendees SET partstat = ? WHERE event_id = ? AND email = ?`,
		partstat, eventID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// TaskappSnapshot devuelve la última versión de Taskapp vista para el evento.
func (s *Store) TaskappSnapshot(ctx context.Context, eventID int64) (string, error) {
	var snap string
	err := s.db.QueryRowContext(ctx, `SELECT taskapp_snapshot FROM events WHERE id = ?`, eventID).Scan(&snap)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return snap, err
}

// SetTaskappState guarda la versión de Taskapp vista y el cambio pendiente.
func (s *Store) SetTaskappState(ctx context.Context, eventID int64, snapshot, change string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE events SET taskapp_snapshot = ?, taskapp_change = ? WHERE id = ?`, snapshot, change, eventID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimEventReminder reclama el aviso de una ocurrencia; true solo la primera vez.
func (s *Store) ClaimEventReminder(ctx context.Context, key string, occurrence, at time.Time) (bool, error) {
	res, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO event_reminders (event_key, occurrence_at, reminded_at) VALUES (?,?,?)`,
		key, occurrence.Unix(), at.Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// InviteRecord es el resultado de procesar un correo con invitación.
type InviteRecord struct {
	MessageID int64
	Method    string
	UID       string
	Sequence  int
	Sender    string
	ICS       string
	Outcome   string
}

// InviteRecord devuelve el registro del correo; ok=false si no se procesó.
func (s *Store) InviteRecord(ctx context.Context, messageID int64) (InviteRecord, bool, error) {
	r := InviteRecord{MessageID: messageID}
	err := s.db.QueryRowContext(ctx, `SELECT method, uid, sequence, sender, ics, outcome FROM invite_messages WHERE message_id = ?`, messageID).
		Scan(&r.Method, &r.UID, &r.Sequence, &r.Sender, &r.ICS, &r.Outcome)
	if errors.Is(err, sql.ErrNoRows) {
		return InviteRecord{}, false, nil
	}
	if err != nil {
		return InviteRecord{}, false, err
	}
	return r, true, nil
}

// InviteSeen dice si ya se procesó esa invitación (misma uid, secuencia, remitente y método).
func (s *Store) InviteSeen(ctx context.Context, uid string, sequence int, sender, method string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM invite_messages WHERE uid = ? AND sequence = ? AND sender = ? AND method = ?`,
		uid, sequence, sender, method).Scan(&n)
	return n > 0, err
}

func (s *Store) SaveInviteRecord(ctx context.Context, r InviteRecord) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO invite_messages (message_id, method, uid, sequence, sender, ics, outcome, processed_at)
 VALUES (?,?,?,?,?,?,?,?)`, r.MessageID, r.Method, r.UID, r.Sequence, r.Sender, r.ICS, r.Outcome, time.Now().Unix())
	return err
}

// StoredMeeting es una reunión de Taskapp en la caché; JSON es la reunión completa.
type StoredMeeting struct {
	ID, WorkspaceID int64
	Start, End      time.Time
	JSON            string
}

// ReplaceMeetings reemplaza las reuniones del workspace que empiezan en [from, to).
func (s *Store) ReplaceMeetings(ctx context.Context, workspaceID int64, from, to time.Time, ms []StoredMeeting, at time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM taskapp_meetings WHERE workspace_id = ? AND start_at >= ? AND start_at < ?`,
		workspaceID, from.Unix(), to.Unix()); err != nil {
		return err
	}
	for _, m := range ms {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO taskapp_meetings (id, workspace_id, start_at, end_at, json, fetched_at) VALUES (?,?,?,?,?,?)`,
			m.ID, workspaceID, m.Start.Unix(), m.End.Unix(), m.JSON, at.Unix()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// MeetingsInRange devuelve las reuniones que se superponen con [from, to) y la
// fecha de la lectura más vieja entre ellas (cero si no hay).
func (s *Store) MeetingsInRange(ctx context.Context, from, to time.Time) ([]StoredMeeting, time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, workspace_id, start_at, end_at, json, fetched_at FROM taskapp_meetings
 WHERE start_at < ? AND end_at > ? ORDER BY start_at, id`, to.Unix(), from.Unix())
	if err != nil {
		return nil, time.Time{}, err
	}
	defer rows.Close()
	out := []StoredMeeting{}
	var oldest int64
	for rows.Next() {
		var m StoredMeeting
		var start, end, fetched int64
		if err := rows.Scan(&m.ID, &m.WorkspaceID, &start, &end, &m.JSON, &fetched); err != nil {
			return nil, time.Time{}, err
		}
		m.Start, m.End = time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC()
		if len(out) == 0 || fetched < oldest {
			oldest = fetched
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, time.Time{}, err
	}
	if len(out) == 0 {
		return out, time.Time{}, nil
	}
	return out, time.Unix(oldest, 0).UTC(), nil
}

// CachedMeeting devuelve la reunión de la caché; ErrNotFound si no está.
func (s *Store) CachedMeeting(ctx context.Context, id int64) (StoredMeeting, error) {
	var m StoredMeeting
	var start, end int64
	err := s.db.QueryRowContext(ctx, `SELECT id, workspace_id, start_at, end_at, json FROM taskapp_meetings WHERE id = ?`, id).
		Scan(&m.ID, &m.WorkspaceID, &start, &end, &m.JSON)
	if errors.Is(err, sql.ErrNoRows) {
		return StoredMeeting{}, ErrNotFound
	}
	if err != nil {
		return StoredMeeting{}, err
	}
	m.Start, m.End = time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC()
	return m, nil
}

// SaveMeeting guarda (o reemplaza) una sola reunión en la caché.
func (s *Store) SaveMeeting(ctx context.Context, m StoredMeeting, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO taskapp_meetings (id, workspace_id, start_at, end_at, json, fetched_at) VALUES (?,?,?,?,?,?)`,
		m.ID, m.WorkspaceID, m.Start.Unix(), m.End.Unix(), m.JSON, at.Unix())
	return err
}

package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func day(d int) time.Time { return time.Date(2026, 11, d, 0, 0, 0, 0, time.UTC) }

func evAt(uid string, start, end time.Time) model.CalendarEvent {
	return model.CalendarEvent{UID: uid, Kind: "local", Title: uid, Start: start, End: end, Status: "confirmed"}
}

func TestMigrate_V18(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v17.db")
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	require.NoError(t, migrateTo(db, 17))
	require.NoError(t, db.Close())

	// Se cargan datos en la base v17 con SQL crudo (el store ya migra a v18 al abrir).
	db, err = sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	require.NoError(t, err)
	// Alta de cuenta con SQL mínimo: solo las columnas NOT NULL sin valor por defecto.
	rows, err := db.Query(`SELECT name FROM pragma_table_info('accounts') WHERE "notnull" = 1 AND dflt_value IS NULL AND pk = 0`)
	require.NoError(t, err)
	var need []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		need = append(need, n)
	}
	require.NoError(t, rows.Close())
	q, args := "INSERT INTO accounts (", []any{}
	ph := ""
	for i, n := range need {
		if i > 0 {
			q += ","
			ph += ","
		}
		q += n
		ph += "?"
		switch n {
		case "imap_port", "smtp_port":
			args = append(args, 993)
		default:
			args = append(args, "x")
		}
	}
	_, err = db.Exec(q+") VALUES ("+ph+")", args...)
	require.NoError(t, err)
	var accID int64
	require.NoError(t, db.QueryRow(`SELECT id FROM accounts`).Scan(&accID))
	ins := func(name, role string) int64 {
		res, err := db.Exec(`INSERT INTO folders (account_id, name, role) VALUES (?,?,?)`, accID, name, role)
		require.NoError(t, err)
		id, _ := res.LastInsertId()
		return id
	}
	inbox, sent := ins("INBOX", "inbox"), ins("Sent", "sent")
	now := time.Now()
	msg := func(folder int64, uid int, age time.Duration) int64 {
		res, err := db.Exec(`INSERT INTO messages (folder_id, uid, date, body_cached) VALUES (?,?,?,1)`, folder, uid, now.Add(-age).Unix())
		require.NoError(t, err)
		id, _ := res.LastInsertId()
		return id
	}
	recent := msg(inbox, 1, 10*24*time.Hour)
	old := msg(inbox, 2, 90*24*time.Hour)
	sentRecent := msg(sent, 3, 10*24*time.Hour)
	require.NoError(t, db.Close())

	s, err := Open(path)
	require.NoError(t, err)
	defer s.Close()
	var v int
	require.NoError(t, s.db.QueryRow("PRAGMA user_version").Scan(&v))
	assert.Equal(t, 18, v)
	cached := func(id int64) int {
		var c int
		require.NoError(t, s.db.QueryRow(`SELECT body_cached FROM messages WHERE id = ?`, id).Scan(&c))
		return c
	}
	assert.Equal(t, 0, cached(recent), "Entrada reciente")
	assert.Equal(t, 1, cached(old), "Entrada vieja")
	assert.Equal(t, 1, cached(sentRecent), "Enviados")
}

func TestSaveEvent_RoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	rb := 30
	end := day(20).Add(time.Hour)
	in := model.CalendarEvent{
		UID: "u1", Kind: "invite", Title: "Reunión", Description: "d", Notes: "n", Location: "Sala", AllDay: false,
		Start: day(10).Add(9 * time.Hour), End: day(10).Add(10 * time.Hour), TZID: "America/Argentina/Buenos_Aires",
		RRule: "FREQ=DAILY;UNTIL=20261120T010000Z", RemindBefore: &rb,
		Organizer: model.Address{Name: "Org", Email: "org@x.com"}, Sequence: 2, Status: "confirmed",
		MyPartstat: "ACCEPTED", MyEmail: "yo@x.com", AliasReply: true, NeedsReview: true, RawICS: "BEGIN:VCALENDAR",
		TaskappMeetingID: 77, TaskappChange: "changed",
		Attendees: []model.Attendee{{Email: "  B@X.com ", Name: "B", Partstat: "NEEDS-ACTION"}, {Email: "a@x.com", Name: "A", Partstat: "ACCEPTED"}},
		Exceptions: []model.EventException{
			{RecurrenceAt: day(12).Add(9 * time.Hour), Cancelled: true},
			{RecurrenceAt: day(11).Add(9 * time.Hour), Start: day(11).Add(11 * time.Hour), End: day(11).Add(12 * time.Hour), Title: "Otro", Location: "L", Description: "D"},
		},
	}
	saved, err := s.SaveEvent(ctx, in, &end)
	require.NoError(t, err)
	require.NotZero(t, saved.ID)
	got, err := s.GetEvent(ctx, saved.ID)
	require.NoError(t, err)

	assert.Equal(t, in.UID, got.UID)
	assert.Equal(t, in.Kind, got.Kind)
	assert.Equal(t, in.Title, got.Title)
	assert.Equal(t, in.Description, got.Description)
	assert.Equal(t, in.Notes, got.Notes)
	assert.Equal(t, in.Location, got.Location)
	assert.True(t, in.Start.Equal(got.Start))
	assert.True(t, in.End.Equal(got.End))
	assert.Equal(t, in.TZID, got.TZID)
	assert.Equal(t, in.RRule, got.RRule)
	require.NotNil(t, got.RemindBefore)
	assert.Equal(t, 30, *got.RemindBefore)
	assert.Equal(t, in.Organizer, got.Organizer)
	assert.Equal(t, 2, got.Sequence)
	assert.Equal(t, "ACCEPTED", got.MyPartstat)
	assert.Equal(t, "yo@x.com", got.MyEmail)
	assert.True(t, got.AliasReply)
	assert.True(t, got.NeedsReview)
	assert.Equal(t, "BEGIN:VCALENDAR", got.RawICS)
	assert.EqualValues(t, 77, got.TaskappMeetingID)
	assert.Equal(t, "changed", got.TaskappChange)
	assert.EqualValues(t, 0, got.AccountID)
	assert.Equal(t, []model.Attendee{{Email: "b@x.com", Name: "B", Partstat: "NEEDS-ACTION"}, {Email: "a@x.com", Name: "A", Partstat: "ACCEPTED"}}, got.Attendees)
	require.Len(t, got.Exceptions, 2)
	assert.True(t, got.Exceptions[0].RecurrenceAt.Equal(day(11).Add(9*time.Hour)), "ordenadas por recurrence_at")
	assert.Equal(t, "Otro", got.Exceptions[0].Title)
	assert.True(t, got.Exceptions[1].Cancelled)

	// Editar reemplaza invitados y excepciones.
	got.Title = "Nuevo"
	got.Attendees = []model.Attendee{{Email: "c@x.com", Partstat: "NEEDS-ACTION"}}
	got.Exceptions = nil
	upd, err := s.SaveEvent(ctx, got, nil)
	require.NoError(t, err)
	assert.Equal(t, "Nuevo", upd.Title)
	assert.Len(t, upd.Attendees, 1)
	assert.Empty(t, upd.Exceptions)
	var series sql.NullInt64
	require.NoError(t, s.db.QueryRow(`SELECT series_end_at FROM events WHERE id = ?`, got.ID).Scan(&series))
	assert.False(t, series.Valid)

	got.ID = 9999
	_, err = s.SaveEvent(ctx, got, nil)
	assert.ErrorIs(t, err, ErrNotFound)

	byUID, err := s.EventByUID(ctx, "u1")
	require.NoError(t, err)
	assert.Equal(t, saved.ID, byUID.ID)
	byMeeting, err := s.EventByMeeting(ctx, 77)
	require.NoError(t, err)
	assert.Equal(t, saved.ID, byMeeting.ID)
	_, err = s.EventByUID(ctx, "nada")
	assert.ErrorIs(t, err, ErrNotFound)
	_, err = s.GetEvent(ctx, 12345)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestSaveEvent_UIDUnique(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, err := s.SaveEvent(ctx, evAt("dup", day(1), day(2)), nil)
	require.NoError(t, err)
	_, err = s.SaveEvent(ctx, evAt("dup", day(3), day(4)), nil)
	assert.Error(t, err)
}

func TestEventsInRange_SingleAndSeries(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	save := func(e model.CalendarEvent, series *time.Time) {
		_, err := s.SaveEvent(ctx, e, series)
		require.NoError(t, err)
	}
	cross := evAt("cruza", day(9).Add(23*time.Hour), day(10).Add(time.Hour))
	save(cross, nil)
	open := evAt("serie-abierta", day(1), day(1).Add(time.Hour))
	open.RRule = "FREQ=WEEKLY"
	save(open, nil)
	endedAt := day(5)
	ended := evAt("serie-vieja", day(1), day(1).Add(time.Hour))
	ended.RRule = "FREQ=DAILY;COUNT=4"
	save(ended, &endedAt)
	save(evAt("despues", day(20), day(21)), nil)
	save(evAt("antes", day(2), day(3)), nil)

	got, err := s.EventsInRange(ctx, day(10), day(15))
	require.NoError(t, err)
	var uids []string
	for _, e := range got {
		uids = append(uids, e.UID)
	}
	assert.Equal(t, []string{"serie-abierta", "cruza"}, uids)

	none, err := s.EventsInRange(ctx, day(25), day(26))
	require.NoError(t, err)
	assert.NotNil(t, none)
	all, err := s.AllEvents(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 5)
}

func TestClaimEventReminder_Once(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	ok, err := s.ClaimEventReminder(ctx, "ev:1", day(10), day(9))
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.ClaimEventReminder(ctx, "ev:1", day(10), day(9))
	require.NoError(t, err)
	assert.False(t, ok)
	ok, err = s.ClaimEventReminder(ctx, "ev:1", day(11), day(9))
	require.NoError(t, err)
	assert.True(t, ok)
}

// messageID crea una cuenta, una carpeta y un correo y devuelve el id del correo.
func messageID(t *testing.T, s *Store) int64 {
	t.Helper()
	ctx := context.Background()
	accID, err := s.InsertAccount(ctx, hostinger())
	require.NoError(t, err)
	res, err := s.db.Exec(`INSERT INTO folders (account_id, name, role) VALUES (?, 'INBOX', 'inbox')`, accID)
	require.NoError(t, err)
	fid, _ := res.LastInsertId()
	res, err = s.db.Exec(`INSERT INTO messages (folder_id, uid, date) VALUES (?, 1, 1700000000)`, fid)
	require.NoError(t, err)
	id, _ := res.LastInsertId()
	return id
}

func TestInviteSeen(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	mid := messageID(t, s)
	require.NoError(t, s.SaveInviteRecord(ctx, InviteRecord{MessageID: mid, Method: "REQUEST", UID: "u", Sequence: 3, Sender: "a@x", ICS: "ics", Outcome: "created"}))
	seen, err := s.InviteSeen(ctx, "u", 3, "a@x", "REQUEST")
	require.NoError(t, err)
	assert.True(t, seen)
	seen, err = s.InviteSeen(ctx, "u", 4, "a@x", "REQUEST")
	require.NoError(t, err)
	assert.False(t, seen)

	// INSERT OR REPLACE: se pisa el registro del mismo correo.
	require.NoError(t, s.SaveInviteRecord(ctx, InviteRecord{MessageID: mid, Method: "CANCEL", UID: "u", Sequence: 5, Sender: "a@x", Outcome: "cancelled"}))
	r, ok, err := s.InviteRecord(ctx, mid)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "CANCEL", r.Method)
	assert.Equal(t, 5, r.Sequence)
}

func TestInviteRecord_CascadeOnMessageDelete(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	mid := messageID(t, s)
	_, ok, err := s.InviteRecord(ctx, mid)
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, s.SaveInviteRecord(ctx, InviteRecord{MessageID: mid, Method: "REQUEST", UID: "u", Sender: "a@x"}))
	_, ok, err = s.InviteRecord(ctx, mid)
	require.NoError(t, err)
	assert.True(t, ok)
	_, err = s.db.Exec(`DELETE FROM messages WHERE id = ?`, mid)
	require.NoError(t, err)
	_, ok, err = s.InviteRecord(ctx, mid)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestReplaceMeetings_RangeOnly(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	at := time.Now()
	m := func(id, ws int64, d int) StoredMeeting {
		return StoredMeeting{ID: id, WorkspaceID: ws, Start: day(d), End: day(d).Add(time.Hour), JSON: fmt.Sprintf(`{"id":%d}`, id)}
	}
	dec := func(d int) StoredMeeting {
		x := m(int64(100+d), 1, 1)
		x.Start, x.End = time.Date(2026, 12, d, 9, 0, 0, 0, time.UTC), time.Date(2026, 12, d, 10, 0, 0, 0, time.UTC)
		return x
	}
	nov1, dec1 := day(1), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, s.ReplaceMeetings(ctx, 1, nov1, dec1, []StoredMeeting{m(1, 1, 5), m(2, 1, 6)}, at))
	require.NoError(t, s.ReplaceMeetings(ctx, 1, dec1, dec1.AddDate(0, 1, 0), []StoredMeeting{dec(3)}, at))
	require.NoError(t, s.ReplaceMeetings(ctx, 2, nov1, dec1, []StoredMeeting{m(50, 2, 7)}, at))

	// Se reemplaza noviembre del workspace 1: queda solo la reunión 3 de noviembre.
	require.NoError(t, s.ReplaceMeetings(ctx, 1, nov1, dec1, []StoredMeeting{m(3, 1, 8)}, at))
	got, _, err := s.MeetingsInRange(ctx, nov1, dec1.AddDate(0, 1, 0))
	require.NoError(t, err)
	var ids []int64
	for _, g := range got {
		ids = append(ids, g.ID)
	}
	assert.Equal(t, []int64{50, 3, 103}, ids)
}

func TestMeetingsInRange_FetchedAtIsOldest(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	m := StoredMeeting{ID: 1, WorkspaceID: 1, Start: day(5), End: day(5).Add(time.Hour), JSON: "{}"}
	m2 := StoredMeeting{ID: 2, WorkspaceID: 2, Start: day(6), End: day(6).Add(time.Hour), JSON: "{}"}
	require.NoError(t, s.ReplaceMeetings(ctx, 1, day(1), day(30), []StoredMeeting{m}, time.Unix(2000, 0)))
	require.NoError(t, s.ReplaceMeetings(ctx, 2, day(1), day(30), []StoredMeeting{m2}, time.Unix(1000, 0)))
	got, fetched, err := s.MeetingsInRange(ctx, day(1), day(30))
	require.NoError(t, err)
	assert.Len(t, got, 2)
	assert.EqualValues(t, 1000, fetched.Unix())
	// Superposición: una reunión que empieza antes del rango y lo cruza entra.
	got, _, err = s.MeetingsInRange(ctx, day(5).Add(30*time.Minute), day(5).Add(45*time.Minute))
	require.NoError(t, err)
	assert.Len(t, got, 1)
	got, fetched, err = s.MeetingsInRange(ctx, day(20), day(21))
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
	assert.True(t, fetched.IsZero())
}

func TestClearTaskappData_Meetings(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, err := s.SaveEvent(ctx, evAt("u", day(1), day(2)), nil)
	require.NoError(t, err)
	require.NoError(t, s.ReplaceMeetings(ctx, 1, day(1), day(30), []StoredMeeting{{ID: 1, WorkspaceID: 1, Start: day(5), End: day(6), JSON: "{}"}}, time.Now()))
	require.NoError(t, s.ClearTaskappData(ctx))
	got, _, err := s.MeetingsInRange(ctx, day(1), day(30))
	require.NoError(t, err)
	assert.Empty(t, got)
	all, err := s.AllEvents(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 1)
}

func TestDeleteAccount_KeepsEvents(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	accID, err := s.InsertAccount(ctx, hostinger())
	require.NoError(t, err)
	e := evAt("u", day(1), day(2))
	e.AccountID = accID
	saved, err := s.SaveEvent(ctx, e, nil)
	require.NoError(t, err)
	assert.Equal(t, accID, saved.AccountID)
	require.NoError(t, s.DeleteAccount(ctx, accID))
	got, err := s.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 0, got.AccountID)
}

func TestTaskappState_AndPartstat(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	e := evAt("u", day(1), day(2))
	e.Attendees = []model.Attendee{{Email: "a@x.com", Partstat: "NEEDS-ACTION"}}
	saved, err := s.SaveEvent(ctx, e, nil)
	require.NoError(t, err)
	require.NoError(t, s.SetTaskappState(ctx, saved.ID, `{"v":1}`, "changed"))
	snap, err := s.TaskappSnapshot(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, `{"v":1}`, snap)
	got, _ := s.GetEvent(ctx, saved.ID)
	assert.Equal(t, "changed", got.TaskappChange)
	assert.ErrorIs(t, s.SetTaskappState(ctx, 999, "", ""), ErrNotFound)
	_, err = s.TaskappSnapshot(ctx, 999)
	assert.ErrorIs(t, err, ErrNotFound)

	ok, err := s.SetAttendeePartstat(ctx, saved.ID, "A@X.com", "ACCEPTED")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = s.SetAttendeePartstat(ctx, saved.ID, "z@x.com", "ACCEPTED")
	require.NoError(t, err)
	assert.False(t, ok)
	require.NoError(t, s.DeleteEvent(ctx, saved.ID))
	assert.ErrorIs(t, s.DeleteEvent(ctx, saved.ID), ErrNotFound)
}

func TestEvents_ReadBackInUTC(t *testing.T) {
	old := time.Local
	time.Local = time.FixedZone("ART", -3*3600)
	t.Cleanup(func() { time.Local = old })
	s := newStore(t)
	ctx := context.Background()
	e := evAt("allday", day(10), day(11))
	e.AllDay = true
	e.Exceptions = []model.EventException{{RecurrenceAt: day(10), Start: day(10), End: day(11)}}
	saved, err := s.SaveEvent(ctx, e, nil)
	require.NoError(t, err)
	got, err := s.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, got.Start.Location())
	assert.Equal(t, time.UTC, got.End.Location())
	assert.Equal(t, "2026-11-10", got.Start.Format("2006-01-02"))
	assert.Equal(t, time.UTC, got.Exceptions[0].RecurrenceAt.Location())
	assert.Equal(t, "2026-11-10", got.Exceptions[0].Start.Format("2006-01-02"))
	require.NoError(t, s.ReplaceMeetings(ctx, 1, day(1), day(30), []StoredMeeting{{ID: 1, WorkspaceID: 1, Start: day(10), End: day(11), JSON: "{}"}}, day(9)))
	ms, fetched, err := s.MeetingsInRange(ctx, day(1), day(30))
	require.NoError(t, err)
	assert.Equal(t, time.UTC, ms[0].Start.Location())
	assert.Equal(t, time.UTC, fetched.Location())
}

func TestEventsInRange_ZeroDuration(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	_, err := s.SaveEvent(ctx, evAt("enFrom", day(10), day(10)), nil)
	require.NoError(t, err)
	_, err = s.SaveEvent(ctx, evAt("enTo", day(15), day(15)), nil)
	require.NoError(t, err)
	got, err := s.EventsInRange(ctx, day(10), day(15))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "enFrom", got[0].UID)
}

func TestSaveEvent_DedupesChildren(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	e := evAt("dd", day(1), day(2))
	e.Attendees = []model.Attendee{
		{Email: "A@x.com", Name: "primero", Partstat: "NEEDS-ACTION"},
		{Email: "b@x.com", Name: "B", Partstat: "NEEDS-ACTION"},
		{Email: " a@X.com ", Name: "ultimo", Partstat: "ACCEPTED"},
	}
	e.Exceptions = []model.EventException{
		{RecurrenceAt: day(1), Title: "uno"},
		{RecurrenceAt: day(1), Title: "dos", Cancelled: true},
	}
	saved, err := s.SaveEvent(ctx, e, nil)
	require.NoError(t, err)
	require.Len(t, saved.Attendees, 2)
	assert.Equal(t, "ultimo", saved.Attendees[0].Name)
	assert.Equal(t, "ACCEPTED", saved.Attendees[0].Partstat)
	require.Len(t, saved.Exceptions, 1)
	assert.Equal(t, "dos", saved.Exceptions[0].Title)
}

func TestEvents_ChildrenOnlyOfReturned(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	a := evAt("a", day(1), day(2))
	a.Attendees = []model.Attendee{{Email: "a@x.com"}}
	b := evAt("b", day(20), day(21))
	b.Attendees = []model.Attendee{{Email: "b@x.com"}}
	_, err := s.SaveEvent(ctx, a, nil)
	require.NoError(t, err)
	_, err = s.SaveEvent(ctx, b, nil)
	require.NoError(t, err)
	got, err := s.EventsInRange(ctx, day(1), day(3))
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []model.Attendee{{Email: "a@x.com"}}, got[0].Attendees)
}

func TestCachedMeeting_ByID(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	m := StoredMeeting{ID: 9, WorkspaceID: 3, Start: day(5), End: day(5).Add(time.Hour), JSON: `{"ID":9}`}
	require.NoError(t, s.ReplaceMeetings(ctx, 3, day(1), day(30), []StoredMeeting{m}, time.Now()))
	got, err := s.CachedMeeting(ctx, 9)
	require.NoError(t, err)
	assert.Equal(t, m, got)
	_, err = s.CachedMeeting(ctx, 10)
	assert.ErrorIs(t, err, ErrNotFound)
}

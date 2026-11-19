package mail

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/testenv"
)

const secretNotes = "notas privadas que no salen"

// inviteCore arma un núcleo con una cuenta ("Fabricio <f@ej.com>") para
// mandar invitaciones.
func inviteCore(t *testing.T) (*Core, *testenv.SMTP, *recNotifier, model.Account) {
	t.Helper()
	c, imap, smtp, n := newTestCore(t)
	in := inputFor(imap, smtp, testPass)
	in.Account.DisplayName = "Fabricio"
	acc, err := c.AddAccount(context.Background(), in)
	require.NoError(t, err)
	return c, smtp, n, acc
}

// meeting es una reunión de una hora el 19/11 a las 13:00 UTC (10:00 en Buenos Aires).
func meeting(acc model.Account, emails ...string) model.CalendarEvent {
	e := model.CalendarEvent{Title: "Reunión", Description: "Agenda:\nuno", Location: "Sala 1", Notes: secretNotes,
		Start: tu(2026, 11, 19, 13, 0), End: tu(2026, 11, 19, 14, 0), TZID: "UTC", AccountID: acc.ID}
	for _, m := range emails {
		e.Attendees = append(e.Attendees, model.Attendee{Email: m})
	}
	return e
}

// waitMails espera a que el servidor SMTP haya recibido n correos.
func waitMails(t *testing.T, smtp *testenv.SMTP, n int) []testenv.Envelope {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if envs := smtp.Received(); len(envs) >= n {
			require.Len(t, envs, n)
			return envs
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("llegaron %d correos, se esperaban %d", len(smtp.Received()), n)
	return nil
}

// noMoreMails comprueba que no salga nada más que los n correos ya recibidos.
func noMoreMails(t *testing.T, c *Core, smtp *testenv.SMTP, n int) {
	t.Helper()
	time.Sleep(150 * time.Millisecond)
	assert.Len(t, smtp.Received(), n)
	assert.Empty(t, outbox(t, c), "no quedó nada en la bandeja de salida")
}

// sentMail es un correo recibido, ya leído.
type sentMail struct {
	To, From, Subject string
	ICS, HTML, Text   string
	Raw               string
}

func readMail(t *testing.T, e testenv.Envelope) sentMail {
	t.Helper()
	require.Len(t, e.To, 1, "un destinatario por correo")
	h := parseEnvelope(t, e).Header
	subject, err := h.Subject()
	require.NoError(t, err)
	from, err := h.AddressList("From")
	require.NoError(t, err)
	require.Len(t, from, 1)
	return sentMail{To: e.To[0], From: from[0].String(), Subject: subject,
		ICS:  strings.ReplaceAll(envelopePart(t, e, "text/calendar"), "\r\n ", ""),
		HTML: envelopePart(t, e, "text/html"), Text: envelopePart(t, e, "text/plain"), Raw: string(e.Data)}
}

func mailsTo(t *testing.T, envs []testenv.Envelope, to string) []sentMail {
	t.Helper()
	out := []sentMail{}
	for _, e := range envs {
		if m := readMail(t, e); m.To == to {
			out = append(out, m)
		}
	}
	return out
}

func TestSaveEvent_NewWithAttendeesSendsRequest(t *testing.T) {
	setLocalAR(t)
	c, smtp, _, acc := inviteCore(t)
	saved, err := c.SaveEvent(context.Background(), model.EventSave{Event: meeting(acc, "ana@ej.com", "Beto <beto@ej.com>")})
	require.NoError(t, err)
	assert.Equal(t, model.Address{Name: "Fabricio", Email: testUser}, saved.Organizer)
	assert.Equal(t, 0, saved.Sequence)

	envs := waitMails(t, smtp, 2)
	for _, to := range []string{"ana@ej.com", "beto@ej.com"} {
		ms := mailsTo(t, envs, to)
		require.Len(t, ms, 1, to)
		m := ms[0]
		assert.Equal(t, "Invitación: Reunión — 19 de noviembre de 2026, 10:00", m.Subject)
		assert.Contains(t, m.From, testUser)
		assert.Contains(t, m.ICS, "METHOD:REQUEST")
		assert.Contains(t, m.ICS, "SEQUENCE:0")
		assert.Contains(t, m.ICS, "UID:"+saved.UID)
		assert.Contains(t, m.ICS, "ORGANIZER;CN=Fabricio:mailto:"+testUser)
		assert.Contains(t, m.ICS, "mailto:ana@ej.com")
		assert.Contains(t, m.ICS, "mailto:beto@ej.com")
		assert.Contains(t, m.HTML, "Sala 1")
		for _, part := range []string{m.ICS, m.HTML, m.Text, m.Raw} {
			assert.NotContains(t, part, secretNotes)
		}
	}
}

func TestSaveEvent_NoAccount(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	e := meeting(acc, "ana@ej.com")
	e.AccountID = 0
	_, err := c.SaveEvent(context.Background(), model.EventSave{Event: e})
	requireUserMsg(t, err, "invalid", "Elegí desde qué cuenta mandar las invitaciones.")
	all, err := c.store.AllEvents(context.Background())
	require.NoError(t, err)
	assert.Empty(t, all)
	noMoreMails(t, c, smtp, 0)

	// Sin invitados no hace falta cuenta.
	e.Attendees = nil
	_, err = c.SaveEvent(context.Background(), model.EventSave{Event: e})
	require.NoError(t, err)
}

func TestPreview_NewEventAndDelete(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	p, err := c.PreviewEventSave(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com", "beto@ej.com")})
	require.NoError(t, err)
	assert.Equal(t, model.SavePreview{NotifyNeeded: true, Added: 2}, p)
	p, err = c.PreviewEventSave(ctx, model.EventSave{Event: meeting(acc)})
	require.NoError(t, err)
	assert.Equal(t, model.SavePreview{}, p)
	all, err := c.store.AllEvents(ctx)
	require.NoError(t, err)
	assert.Empty(t, all, "la vista previa no guarda nada")

	with, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com")})
	require.NoError(t, err)
	without, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc)})
	require.NoError(t, err)
	waitMails(t, smtp, 1)
	p, err = c.PreviewEventDelete(ctx, model.EventDelete{ID: with.ID, Notify: true})
	require.NoError(t, err)
	assert.True(t, p.NotifyNeeded)
	p, err = c.PreviewEventDelete(ctx, model.EventDelete{ID: without.ID, Notify: true})
	require.NoError(t, err)
	assert.False(t, p.NotifyNeeded)
	_, err = c.store.GetEvent(ctx, with.ID)
	require.NoError(t, err, "la vista previa no borra")
	noMoreMails(t, c, smtp, 1)
}

func TestPreview_NotesOnlyNoNotice(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com")})
	require.NoError(t, err)
	waitMails(t, smtp, 1)

	notes := saved
	notes.Notes = "otra cosa"
	p, err := c.PreviewEventSave(ctx, model.EventSave{Event: notes, Notify: true})
	require.NoError(t, err)
	assert.Equal(t, model.SavePreview{}, p)

	remind := saved
	remind.RemindBefore = ip(60)
	p, err = c.PreviewEventSave(ctx, model.EventSave{Event: remind, Notify: true})
	require.NoError(t, err)
	assert.False(t, p.NotifyNeeded)

	// El nombre o la respuesta de un invitado tampoco les importa.
	renamed := saved
	renamed.Attendees = []model.Attendee{{Email: "ANA@ej.com", Name: "Ana María"}}
	p, err = c.PreviewEventSave(ctx, model.EventSave{Event: renamed, Notify: true})
	require.NoError(t, err)
	assert.False(t, p.NotifyNeeded)
}

func TestPreview_TimeChangeNeedsNotice(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com", "beto@ej.com")})
	require.NoError(t, err)
	waitMails(t, smtp, 2)

	moved := saved
	moved.Start, moved.End = moved.Start.Add(time.Hour), moved.End.Add(time.Hour)
	p, err := c.PreviewEventSave(ctx, model.EventSave{Event: moved, Notify: true})
	require.NoError(t, err)
	assert.Equal(t, model.SavePreview{NotifyNeeded: true}, p)

	swapped := saved
	swapped.Attendees = []model.Attendee{{Email: "ana@ej.com"}, {Email: "caro@ej.com"}, {Email: "dani@ej.com"}}
	p, err = c.PreviewEventSave(ctx, model.EventSave{Event: swapped, Notify: true})
	require.NoError(t, err)
	assert.Equal(t, model.SavePreview{NotifyNeeded: true, Added: 2, Removed: 1}, p)

	got, err := c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.True(t, got.Start.Equal(saved.Start), "la vista previa no guarda nada")
	assert.Len(t, got.Attendees, 2)
	assert.Equal(t, 0, got.Sequence)
	noMoreMails(t, c, smtp, 2)
}

func TestSaveEvent_NotifyFalseSendsNothing(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com")})
	require.NoError(t, err)
	waitMails(t, smtp, 1)

	moved := saved
	moved.Start, moved.End = moved.Start.Add(time.Hour), moved.End.Add(time.Hour)
	_, err = c.SaveEvent(ctx, model.EventSave{Event: moved})
	require.NoError(t, err)
	// Con Notify pero sin nada que les importe a los invitados, tampoco.
	notes := moved
	notes.Notes = "otra"
	_, err = c.SaveEvent(ctx, model.EventSave{Event: notes, Notify: true})
	require.NoError(t, err)

	noMoreMails(t, c, smtp, 1)
	got, err := c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.Sequence)
	assert.True(t, got.Start.Equal(moved.Start))
}

func TestSaveEvent_UpdateAddRemove(t *testing.T) {
	setLocalAR(t)
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com", "beto@ej.com")})
	require.NoError(t, err)
	waitMails(t, smtp, 2)

	edit := saved
	edit.Attendees = []model.Attendee{{Email: "ana@ej.com"}, {Email: "caro@ej.com"}}
	upd, err := c.SaveEvent(ctx, model.EventSave{Event: edit, Notify: true})
	require.NoError(t, err)
	assert.Equal(t, 1, upd.Sequence)
	envs := waitMails(t, smtp, 5)

	ana := mailsTo(t, envs, "ana@ej.com")
	require.Len(t, ana, 2)
	var update sentMail
	for _, m := range ana {
		if strings.Contains(m.ICS, "SEQUENCE:1") {
			update = m
		}
	}
	assert.Equal(t, "Invitación actualizada: Reunión — 19 de noviembre de 2026, 10:00", update.Subject)
	assert.Contains(t, update.ICS, "METHOD:REQUEST")
	assert.Contains(t, update.ICS, "mailto:caro@ej.com")
	assert.NotContains(t, update.ICS, "mailto:beto@ej.com")

	caro := mailsTo(t, envs, "caro@ej.com")
	require.Len(t, caro, 1)
	assert.Equal(t, "Invitación: Reunión — 19 de noviembre de 2026, 10:00", caro[0].Subject)
	assert.Contains(t, caro[0].ICS, "METHOD:REQUEST")
	assert.Contains(t, caro[0].ICS, "SEQUENCE:1")

	beto := mailsTo(t, envs, "beto@ej.com")
	require.Len(t, beto, 2)
	var cancel sentMail
	for _, m := range beto {
		if strings.Contains(m.ICS, "METHOD:CANCEL") {
			cancel = m
		}
	}
	assert.Equal(t, "Cancelado: Reunión — 19 de noviembre de 2026, 10:00", cancel.Subject)
	assert.Contains(t, cancel.ICS, "SEQUENCE:1")
	assert.Contains(t, cancel.ICS, "STATUS:CANCELLED")
	assert.Contains(t, cancel.ICS, "UID:"+saved.UID)

	got, err := c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Sequence)
}

func TestSaveEvent_FollowingNotifies(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	ser, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Semanal", Start: tu(2026, 11, 2, 13, 0), End: tu(2026, 11, 2, 14, 0), TZID: "UTC", RRule: "FREQ=WEEKLY",
		AccountID: acc.ID, Attendees: []model.Attendee{{Email: "ana@ej.com"}},
	}})
	require.NoError(t, err)
	waitMails(t, smtp, 1)

	edit := ser
	edit.Start, edit.End = tu(2026, 11, 16, 16, 0), tu(2026, 11, 16, 17, 0)
	newEv, err := c.SaveEvent(ctx, model.EventSave{Event: edit, Scope: "following", OccurrenceAt: tu(2026, 11, 16, 13, 0), Notify: true})
	require.NoError(t, err)
	require.NotEqual(t, ser.UID, newEv.UID)
	envs := waitMails(t, smtp, 3)

	var cut, fresh int
	for _, m := range mailsTo(t, envs, "ana@ej.com") {
		assert.Contains(t, m.ICS, "METHOD:REQUEST")
		switch {
		case strings.Contains(m.ICS, "UID:"+ser.UID) && strings.Contains(m.ICS, "UNTIL=20261116T125959Z"):
			cut++
			assert.Contains(t, m.ICS, "SEQUENCE:1")
			assert.True(t, strings.HasPrefix(m.Subject, "Invitación actualizada: Semanal — "), m.Subject)
		case strings.Contains(m.ICS, "UID:"+newEv.UID):
			fresh++
			assert.Contains(t, m.ICS, "SEQUENCE:0")
			assert.Contains(t, m.ICS, "DTSTART:20261116T160000Z")
		}
	}
	assert.Equal(t, 1, cut, "un REQUEST de la serie vieja cortada")
	assert.Equal(t, 1, fresh, "un REQUEST de la serie nueva")

	old, err := c.store.GetEvent(ctx, ser.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, old.Sequence)
	assert.Equal(t, 0, newEv.Sequence)
}

func TestDeleteEvent_NotifyCancel(t *testing.T) {
	setLocalAR(t)
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com")})
	require.NoError(t, err)
	waitMails(t, smtp, 1)

	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: saved.ID, Notify: true}))
	envs := waitMails(t, smtp, 2)
	var cancel *sentMail
	for _, m := range mailsTo(t, envs, "ana@ej.com") {
		if strings.Contains(m.ICS, "METHOD:CANCEL") {
			cancel = &m
		}
	}
	require.NotNil(t, cancel)
	assert.Equal(t, "Cancelado: Reunión — 19 de noviembre de 2026, 10:00", cancel.Subject)
	assert.Contains(t, cancel.ICS, "SEQUENCE:1")
	assert.Contains(t, cancel.ICS, "STATUS:CANCELLED")
	assert.Contains(t, cancel.ICS, "UID:"+saved.UID)
	assert.NotContains(t, cancel.Raw, secretNotes)
	_, err = c.store.GetEvent(ctx, saved.ID)
	requireUserError(t, c.ctxOrUserError(ctx, err), "not_found")
}

func TestDeleteEvent_NotifyThisSendsUpdatedSeries(t *testing.T) {
	setLocalAR(t)
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	ser, err := c.SaveEvent(ctx, model.EventSave{Event: model.CalendarEvent{
		Title: "Semanal", Start: tu(2026, 11, 2, 13, 0), End: tu(2026, 11, 2, 14, 0), TZID: "UTC", RRule: "FREQ=WEEKLY",
		AccountID: acc.ID, Attendees: []model.Attendee{{Email: "ana@ej.com"}},
	}})
	require.NoError(t, err)
	waitMails(t, smtp, 1)

	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: ser.ID, Scope: "this", OccurrenceAt: tu(2026, 11, 9, 13, 0), Notify: true}))
	envs := waitMails(t, smtp, 2)
	var upd *sentMail
	for _, m := range mailsTo(t, envs, "ana@ej.com") {
		if strings.Contains(m.ICS, "SEQUENCE:1") {
			upd = &m
		}
	}
	require.NotNil(t, upd)
	assert.Equal(t, "Invitación actualizada: Semanal — 9 de noviembre de 2026, 10:00", upd.Subject)
	assert.Contains(t, upd.ICS, "METHOD:REQUEST")
	assert.Contains(t, upd.ICS, "EXDATE:20261109T130000Z")
	got, err := c.store.GetEvent(ctx, ser.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Sequence)

	// Sin Notify no sale nada y la secuencia queda.
	require.NoError(t, c.DeleteEvent(ctx, model.EventDelete{ID: ser.ID, Scope: "this", OccurrenceAt: tu(2026, 11, 16, 13, 0)}))
	noMoreMails(t, c, smtp, 2)
	got, err = c.store.GetEvent(ctx, ser.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Sequence)
}

func TestSaveEvent_NotifyQueueFailsKeepsEvent(t *testing.T) {
	c, smtp, n, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com")})
	require.NoError(t, err)
	waitMails(t, smtp, 1)
	db, err := sql.Open("sqlite", c.cfg.DBPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.Exec(`CREATE TRIGGER falla_bandeja BEFORE INSERT ON local_drafts BEGIN SELECT RAISE(ABORT, 'falla simulada'); END`)
	require.NoError(t, err)
	before := len(n.matching(EventCalendarChanged))

	moved := saved
	moved.Start, moved.End = moved.Start.Add(time.Hour), moved.End.Add(time.Hour)
	_, err = c.SaveEvent(ctx, model.EventSave{Event: moved, Notify: true})
	ue := requireUserError(t, err, "send")
	assert.True(t, strings.HasPrefix(ue.Msg, "El evento se guardó, pero no pudimos mandar las invitaciones: "), ue.Msg)

	got, err := c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.True(t, got.Start.Equal(moved.Start), "el evento quedó guardado")
	assert.Equal(t, 0, got.Sequence, "no salió nada: la secuencia no cambia")
	assert.Len(t, n.matching(EventCalendarChanged), before+1)

	// Borrado entero: el evento se va igual.
	err = c.DeleteEvent(ctx, model.EventDelete{ID: saved.ID, Notify: true})
	ue = requireUserError(t, err, "send")
	assert.True(t, strings.HasPrefix(ue.Msg, "El evento se borró, pero no pudimos mandar las cancelaciones: "), ue.Msg)
	all, err := c.store.AllEvents(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)
	assert.Len(t, smtp.Received(), 1)
}

func TestInvitation_GoesThroughOutbox(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	c.cfg.UndoSendDelay = time.Minute
	ctx := context.Background()
	_, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com")})
	require.NoError(t, err)

	ds := outbox(t, c)
	require.Len(t, ds, 1)
	assert.Equal(t, model.DraftSending, ds[0].Status)
	assert.Equal(t, acc.ID, ds[0].Request.AccountID)
	assert.Equal(t, []model.Address{{Email: "ana@ej.com"}}, ds[0].Request.To)
	assert.Equal(t, int64(0), ds[0].Request.SignatureID)
	require.NotNil(t, ds[0].Request.Calendar)
	assert.Equal(t, "REQUEST", ds[0].Request.Calendar.Method)
	assert.Contains(t, ds[0].Request.Calendar.ICS, "METHOD:REQUEST")

	require.NoError(t, c.CancelSend(ctx, ds[0].ID))
	time.Sleep(50 * time.Millisecond)
	assert.Empty(t, smtp.Received())
}

func TestInvitationHTML_EscapesAndOmits(t *testing.T) {
	loc := time.FixedZone("ART", -3*3600)
	e := model.CalendarEvent{Title: "Reunión <b>", Description: "uno & dos\nsegunda", Location: "Sala \"A\"", Notes: secretNotes,
		Start: tu(2026, 11, 19, 13, 0), End: tu(2026, 11, 19, 14, 30), RRule: "FREQ=WEEKLY"}
	h := invitationHTML(e, loc)
	assert.Contains(t, h, "Reunión &lt;b&gt;")
	assert.Contains(t, h, "uno &amp; dos<br>segunda")
	assert.Contains(t, h, "Sala &#34;A&#34;")
	assert.Contains(t, h, "19 de noviembre de 2026, 10:00 – 11:30 · se repite")
	assert.NotContains(t, h, secretNotes)
	for _, row := range []string{"Qué", "Cuándo", "Dónde", "Descripción"} {
		assert.Contains(t, h, row)
	}

	bare := model.CalendarEvent{Title: "Feriado", AllDay: true, Start: tu(2026, 11, 20, 0, 0), End: tu(2026, 11, 22, 0, 0)}
	h = invitationHTML(bare, loc)
	assert.Contains(t, h, "20 de noviembre de 2026 – 21 de noviembre de 2026")
	assert.NotContains(t, h, "se repite")
	assert.NotContains(t, h, "Dónde")
	assert.NotContains(t, h, "Descripción")

	one := bare
	one.End = tu(2026, 11, 21, 0, 0)
	h = invitationHTML(one, loc)
	assert.Contains(t, h, "20 de noviembre de 2026<")
	assert.NotContains(t, h, "–")
}

func TestInvitation_SeriesWithExceptions(t *testing.T) {
	e := model.CalendarEvent{UID: "u@postacontrol", Sequence: 3, Title: "Semanal", Description: "d", Location: "l", Notes: secretNotes,
		Start: tu(2026, 11, 2, 13, 0), End: tu(2026, 11, 2, 14, 0), RRule: "FREQ=WEEKLY", Status: "confirmed",
		Organizer: model.Address{Name: "Fabricio", Email: testUser},
		Attendees: []model.Attendee{{Email: "ana@ej.com", Name: "Ana", Partstat: "ACCEPTED"}},
		Exceptions: []model.EventException{
			{RecurrenceAt: tu(2026, 11, 16, 13, 0), Cancelled: true},
			{RecurrenceAt: tu(2026, 11, 9, 13, 0), Start: tu(2026, 11, 10, 15, 0), End: tu(2026, 11, 10, 16, 0), Title: "Movida", Location: "otra"},
		}}
	s := eventToSeries(e)
	m := s.Master
	assert.Equal(t, "u@postacontrol", m.UID)
	assert.Equal(t, 3, m.Sequence)
	assert.Equal(t, "CONFIRMED", m.Status)
	assert.Equal(t, "Semanal", m.Summary)
	assert.Equal(t, "FREQ=WEEKLY", m.RRule)
	assert.Equal(t, e.Organizer, m.Organizer)
	require.Len(t, m.Attendees, 1)
	assert.Equal(t, "ACCEPTED", m.Attendees[0].Partstat)
	assert.Equal(t, "Ana", m.Attendees[0].Name)
	require.Len(t, m.ExDates, 1)
	assert.True(t, m.ExDates[0].Equal(tu(2026, 11, 16, 13, 0)))
	require.Len(t, s.Overrides, 1)
	ov := s.Overrides[0]
	assert.True(t, ov.RecurrenceID.Equal(tu(2026, 11, 9, 13, 0)))
	assert.True(t, ov.Start.Equal(tu(2026, 11, 10, 15, 0)))
	assert.Equal(t, "Movida", ov.Summary)
	assert.Equal(t, "otra", ov.Location)
	assert.Equal(t, 3, ov.Sequence)
	assert.Empty(t, ov.RRule)
}

func TestSaveEvent_DuplicateAttendeesOneMail(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com", "beto@ej.com", "Ana <ANA@ej.com>")})
	require.NoError(t, err)
	require.Len(t, saved.Attendees, 2)
	assert.Equal(t, model.Attendee{Email: "ana@ej.com", Name: "Ana", Partstat: "NEEDS-ACTION"}, saved.Attendees[0],
		"gana el último, en el lugar del primero")
	assert.Equal(t, "beto@ej.com", saved.Attendees[1].Email)

	envs := waitMails(t, smtp, 2)
	ana := mailsTo(t, envs, "ana@ej.com")
	require.Len(t, ana, 1)
	assert.Equal(t, 1, strings.Count(ana[0].ICS, "mailto:ana@ej.com"))
	noMoreMails(t, c, smtp, 2)

	p, err := c.PreviewEventSave(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com", "ANA@ej.com")})
	require.NoError(t, err)
	assert.Equal(t, 1, p.Added)
}

func TestSaveEvent_SenderNotInvited(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, "ana@ej.com", "Yo <F@EJ.com>")})
	require.NoError(t, err)
	require.Len(t, saved.Attendees, 1)
	assert.Equal(t, "ana@ej.com", saved.Attendees[0].Email)
	envs := waitMails(t, smtp, 1)
	assert.Equal(t, []string{"ana@ej.com"}, envs[0].To)
	assert.NotContains(t, readMail(t, envs[0]).ICS, "ATTENDEE;CN=Yo")
	noMoreMails(t, c, smtp, 1)

	// Solo la cuenta propia: queda sin invitados y no sale nada.
	only, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc, testUser)})
	require.NoError(t, err)
	assert.Empty(t, only.Attendees)
	assert.Empty(t, only.Organizer.Email)
	p, err := c.PreviewEventSave(ctx, model.EventSave{Event: meeting(acc, testUser)})
	require.NoError(t, err)
	assert.Equal(t, model.SavePreview{}, p)
	noMoreMails(t, c, smtp, 1)
}

func TestSaveEvent_OrganizerSavedWithoutNotify(t *testing.T) {
	c, smtp, _, acc := inviteCore(t)
	ctx := context.Background()
	saved, err := c.SaveEvent(ctx, model.EventSave{Event: meeting(acc)})
	require.NoError(t, err)
	assert.Empty(t, saved.Organizer.Email, "sin invitados no hay organizador")

	edit := saved
	edit.Attendees = []model.Attendee{{Email: "ana@ej.com"}}
	_, err = c.SaveEvent(ctx, model.EventSave{Event: edit})
	require.NoError(t, err)
	got, err := c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, model.Address{Name: "Fabricio", Email: testUser}, got.Organizer)
	assert.Equal(t, 0, got.Sequence)
	noMoreMails(t, c, smtp, 0)
}

// Contrato con la UI: lib/errors.ts (SEND_PARTIAL_FRAGMENT) reconoce estos
// avisos de "se guardó/borró, pero falló el correo" por esta frase.
func TestSendFailedMessagesCarryFrontendFragment(t *testing.T) {
	const fragment = "pero no pudimos mandar"
	for _, m := range []string{msgInviteSendFailed, msgCancelSendFailed} {
		if !strings.Contains(m, fragment) {
			t.Errorf("%q no contiene %q", m, fragment)
		}
	}
}

// Un evento semanal a las 22:00 de Buenos Aires (01:00 UTC del día siguiente)
// sale con la zona: si no, Google lo expandiría en UTC y caería el martes.
func TestSaveEvent_WeeklyInvitationKeepsLocalDay(t *testing.T) {
	ar := setLocalAR(t)
	c, smtp, _, acc := inviteCore(t)
	e := meeting(acc, "ana@ej.com")
	e.Start = time.Date(2026, 11, 23, 22, 0, 0, 0, ar).UTC()
	e.End = e.Start.Add(time.Hour)
	e.TZID = "America/Argentina/Buenos_Aires"
	e.RRule = "FREQ=WEEKLY;BYDAY=MO"
	_, err := c.SaveEvent(context.Background(), model.EventSave{Event: e, Notify: true})
	require.NoError(t, err)
	m := readMail(t, waitMails(t, smtp, 1)[0])
	assert.Contains(t, m.ICS, "DTSTART;TZID=America/Argentina/Buenos_Aires:20261123T220000")
	assert.Contains(t, m.ICS, "BEGIN:VTIMEZONE")
	assert.Contains(t, m.ICS, "RRULE:FREQ=WEEKLY;BYDAY=MO")
}

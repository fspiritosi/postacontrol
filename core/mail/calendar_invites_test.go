package mail

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/testenv"
)

const (
	meEmail   = "fabricio@example.com"
	orgEmail  = "organizador@example.com"
	sampleUID = "7q3k9d2m5x8reunionproyecto@google.com"
)

// inviteNow es el "ahora" de los tests de invitaciones: las muestras son de
// diciembre de 2026, así que todavía no pasaron.
var inviteNow = tu(2026, 11, 19, 12, 0)

// receiverCore arma un núcleo con el reloj en inviteNow y una cuenta con la
// dirección email (que entra con el usuario de prueba). Los correos ya
// entregados a imap quedan sincronizados.
func receiverCore(t *testing.T, email string) (*Core, *testenv.IMAP, *testenv.SMTP, *recNotifier, model.Account) {
	t.Helper()
	c, imap, smtp, n := newTestCore(t)
	c.now = func() time.Time { return inviteNow }
	in := inputFor(imap, smtp, testPass)
	in.Account.Email, in.Account.Username, in.Account.DisplayName = email, testUser, "Fabricio"
	acc, err := c.AddAccount(context.Background(), in)
	require.NoError(t, err)
	return c, imap, smtp, n, acc
}

func syncAll(t *testing.T, c *Core, acc model.Account) {
	t.Helper()
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))
}

func sampleICS(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "icalx", "testdata", name))
	require.NoError(t, err)
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

// inviteMail es un correo de from con asunto subject y el iCalendar ics como
// parte text/calendar en línea.
func inviteMail(subject, from, ics string) []byte {
	ics = strings.ReplaceAll(strings.ReplaceAll(ics, "\r\n", "\n"), "\n", "\r\n")
	return []byte("From: " + from + "\r\nTo: " + meEmail + "\r\nSubject: " + subject +
		"\r\nDate: Wed, 18 Nov 2026 12:00:00 +0000\r\nMessage-ID: <" + subject + "@ej.com>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: multipart/alternative; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nInvitación\r\n" +
		"--b\r\nContent-Type: text/calendar; charset=utf-8; method=REQUEST\r\n\r\n" + ics + "\r\n--b--\r\n")
}

const orgFrom = "Organizador Ejemplo <" + orgEmail + ">"

// messageBySubject busca en la carpeta el correo cuyo asunto contiene subject.
func messageBySubject(t *testing.T, c *Core, folderID int64, subject string) model.MessageSummary {
	t.Helper()
	for _, m := range listAll(t, c, folderID) {
		if strings.Contains(m.Subject, subject) {
			return m
		}
	}
	t.Fatalf("no está el correo %q", subject)
	return model.MessageSummary{}
}

// open abre el correo (como el lector) y devuelve su id.
func open(t *testing.T, c *Core, folderID int64, subject string) int64 {
	t.Helper()
	m := messageBySubject(t, c, folderID, subject)
	_, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	return m.ID
}

func inviteEvent(t *testing.T, c *Core) model.CalendarEvent {
	t.Helper()
	e, err := c.store.EventByUID(context.Background(), sampleUID)
	require.NoError(t, err)
	return e
}

func noEvents(t *testing.T, c *Core) {
	t.Helper()
	all, err := c.store.AllEvents(context.Background())
	require.NoError(t, err)
	assert.Empty(t, all)
}

func december(t *testing.T, c *Core) []model.CalendarItem {
	t.Helper()
	r, err := c.ListCalendar(context.Background(), tu(2026, 12, 1, 0, 0), tu(2027, 1, 1, 0, 0))
	require.NoError(t, err)
	return r.Items
}

func inviteFor(t *testing.T, c *Core, messageID int64) model.MessageInvite {
	t.Helper()
	inv, err := c.InviteForMessage(context.Background(), messageID)
	require.NoError(t, err)
	return inv
}

func outcome(t *testing.T, c *Core, messageID int64) string {
	t.Helper()
	r, ok, err := c.store.InviteRecord(context.Background(), messageID)
	require.NoError(t, err)
	require.True(t, ok, "el correo tiene registro")
	return r.Outcome
}

func setPartstat(t *testing.T, c *Core, partstat string) {
	t.Helper()
	e := inviteEvent(t, c)
	e.MyPartstat, e.NeedsReview = partstat, false
	_, err := c.store.SaveEvent(context.Background(), e, seriesEnd(e))
	require.NoError(t, err)
}

func TestInvite_RequestCreatesEvent(t *testing.T) {
	c, imap, _, n, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", calendarMail(t, "google-request.eml"))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")
	before := len(n.matching(EventCalendarChanged))

	id := open(t, c, inbox, "Reunión de proyecto")

	e := inviteEvent(t, c)
	assert.Equal(t, "invite", e.Kind)
	assert.Equal(t, "NEEDS-ACTION", e.MyPartstat)
	assert.Equal(t, acc.ID, e.AccountID)
	assert.Equal(t, meEmail, e.MyEmail)
	assert.False(t, e.AliasReply)
	assert.False(t, e.NeedsReview)
	assert.Equal(t, "Reunion de proyecto", e.Title)
	assert.Equal(t, "Sala 3 / https://meet.google.com/abc-defg-hij", e.Location)
	assert.Contains(t, e.Description, "Revision del avance")
	assert.Equal(t, tu(2026, 12, 3, 15, 0), e.Start)
	assert.Equal(t, tu(2026, 12, 3, 16, 0), e.End)
	assert.Equal(t, model.Address{Name: "Organizador Ejemplo", Email: orgEmail}, e.Organizer)
	assert.Equal(t, 1, e.Sequence)
	assert.Equal(t, "confirmed", e.Status)
	assert.Contains(t, e.RawICS, "UID:"+sampleUID)
	assert.Len(t, e.Attendees, 3)
	assert.Empty(t, e.Notes)
	assert.Len(t, n.matching(EventCalendarChanged), before+1)
	assert.Equal(t, "created", outcome(t, c, id))

	items := december(t, c)
	require.Len(t, items, 1)
	assert.Equal(t, "invite", items[0].Source)
	assert.Equal(t, "NEEDS-ACTION", items[0].Partstat)
	assert.Equal(t, e.ID, items[0].EventID)

	inv := inviteFor(t, c, id)
	assert.True(t, inv.Found)
	assert.Equal(t, "REQUEST", inv.Method)
	assert.Equal(t, e.ID, inv.EventID)
	assert.Equal(t, "Reunion de proyecto", inv.Title)
	assert.Equal(t, tu(2026, 12, 3, 15, 0), inv.Start)
	assert.Equal(t, orgEmail, inv.Organizer.Email)
	assert.Equal(t, "NEEDS-ACTION", inv.Partstat)
	assert.False(t, inv.Past)
	assert.False(t, inv.Outdated)
	assert.False(t, inv.Cancelled)
	assert.False(t, inv.Recurring)

	// Abrirlo de nuevo no cambia nada ni avisa.
	_, err := c.GetMessage(context.Background(), id)
	require.NoError(t, err)
	all, err := c.store.AllEvents(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Len(t, n.matching(EventCalendarChanged), before+1)
}

func TestInvite_IndexerProcesses(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", calendarMail(t, "outlook-request.eml"))
	syncAll(t, c, acc)
	log := fastIndex(c)

	c.StartIndexing(context.Background())
	require.Eventually(t, func() bool { return len(log.indexed()) == 1 }, 10*time.Second, 5*time.Millisecond)

	all, err := c.store.AllEvents(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "invite", all[0].Kind)
	assert.Equal(t, "Seguimiento Outlook", all[0].Title)
	assert.Equal(t, tu(2026, 12, 10, 13, 0), all[0].Start)
	assert.Equal(t, "NEEDS-ACTION", all[0].MyPartstat)
	assert.False(t, firstMessage(t, c, folderID(t, c, acc.ID, "INBOX")).Seen, "el indexador no lo abrió")
}

func TestInvite_SameMailTwiceNoDuplicate(t *testing.T) {
	c, imap, _, n, acc := receiverCore(t, meEmail)
	require.NoError(t, imap.User.Create("Archivo", nil))
	imap.Deliver(t, "INBOX", calendarMail(t, "google-request.eml"))
	imap.Deliver(t, "Archivo", calendarMail(t, "google-request.eml"))
	syncAll(t, c, acc)
	inbox, archive := folderID(t, c, acc.ID, "INBOX"), folderID(t, c, acc.ID, "Archivo")
	require.NoError(t, c.SyncFolder(context.Background(), archive))

	first := open(t, c, inbox, "Reunión de proyecto")
	changed := len(n.matching(EventCalendarChanged))
	second := open(t, c, archive, "Reunión de proyecto")
	open(t, c, inbox, "Reunión de proyecto")
	open(t, c, archive, "Reunión de proyecto")

	all, err := c.store.AllEvents(context.Background())
	require.NoError(t, err)
	assert.Len(t, all, 1)
	assert.Equal(t, "created", outcome(t, c, first))
	assert.Equal(t, "duplicate", outcome(t, c, second))
	assert.Len(t, n.matching(EventCalendarChanged), changed, "el repetido no cambia nada")
	assert.Equal(t, all[0].ID, inviteFor(t, c, second).EventID)
}

func TestInvite_OlderSequenceIgnored(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", inviteMail("update", orgFrom, sampleICS(t, "google-update.ics")))
	imap.Deliver(t, "INBOX", inviteMail("request", orgFrom, sampleICS(t, "google-request.ics")))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	upd := open(t, c, inbox, "update")
	req := open(t, c, inbox, "request")

	e := inviteEvent(t, c)
	assert.Equal(t, 2, e.Sequence)
	assert.Equal(t, tu(2026, 12, 4, 17, 0), e.Start)
	assert.Equal(t, tu(2026, 12, 4, 18, 0), e.End)
	assert.Equal(t, "outdated", outcome(t, c, req))
	assert.True(t, inviteFor(t, c, req).Outdated)
	assert.False(t, inviteFor(t, c, upd).Outdated)
}

func TestInvite_UpdateTimeNeedsReview(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", inviteMail("request", orgFrom, sampleICS(t, "google-request.ics")))
	imap.Deliver(t, "INBOX", inviteMail("update", orgFrom, sampleICS(t, "google-update.ics")))
	moved := strings.ReplaceAll(sampleICS(t, "google-update.ics"), "SEQUENCE:2", "SEQUENCE:3")
	moved = strings.ReplaceAll(moved, "LOCATION:Sala 3 / https://meet.google.com/abc-defg-hij", "LOCATION:Sala 5")
	imap.Deliver(t, "INBOX", inviteMail("lugar", orgFrom, moved))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "request")
	e := inviteEvent(t, c)
	e.Notes = "mis notas"
	_, err := c.store.SaveEvent(context.Background(), e, seriesEnd(e))
	require.NoError(t, err)
	setPartstat(t, c, "ACCEPTED")

	upd := open(t, c, inbox, "update")
	e = inviteEvent(t, c)
	assert.Equal(t, "updated", outcome(t, c, upd))
	assert.True(t, e.NeedsReview)
	assert.Equal(t, "NEEDS-ACTION", e.MyPartstat)
	assert.Equal(t, tu(2026, 12, 4, 17, 0), e.Start)
	assert.Equal(t, 2, e.Sequence)
	assert.Contains(t, e.RawICS, "SEQUENCE:2")
	assert.Equal(t, "mis notas", e.Notes, "las notas se conservan")
	inv := inviteFor(t, c, upd)
	assert.True(t, inv.NeedsReview)
	assert.Equal(t, "NEEDS-ACTION", inv.Partstat)

	// Un cambio que no es de hora no pide revisar ni borra la respuesta.
	setPartstat(t, c, "ACCEPTED")
	open(t, c, inbox, "lugar")
	e = inviteEvent(t, c)
	assert.Equal(t, "Sala 5", e.Location)
	assert.Equal(t, 3, e.Sequence)
	assert.False(t, e.NeedsReview)
	assert.Equal(t, "ACCEPTED", e.MyPartstat)
}

func TestInvite_DeclinedStaysDeclined(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", inviteMail("request", orgFrom, sampleICS(t, "google-request.ics")))
	imap.Deliver(t, "INBOX", inviteMail("update", orgFrom, sampleICS(t, "google-update.ics")))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "request")
	setPartstat(t, c, "DECLINED")
	open(t, c, inbox, "update")

	e := inviteEvent(t, c)
	assert.Equal(t, "DECLINED", e.MyPartstat)
	assert.True(t, e.NeedsReview)
	assert.Equal(t, tu(2026, 12, 4, 17, 0), e.Start)
	assert.Empty(t, december(t, c), "una invitación rechazada no se muestra")
}

func TestInvite_Cancel(t *testing.T) {
	c, imap, _, n, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", inviteMail("request", orgFrom, sampleICS(t, "google-request.ics")))
	imap.Deliver(t, "INBOX", inviteMail("cancel", orgFrom, sampleICS(t, "google-cancel.ics")))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "request")
	ev := inviteEvent(t, c)
	err := c.RemoveCancelledEvent(context.Background(), ev.ID)
	requireUserMsg(t, err, "invalid", "Solo se quitan invitaciones canceladas.")

	changed := len(n.matching(EventCalendarChanged))
	cancel := open(t, c, inbox, "cancel")
	assert.Len(t, n.matching(EventCalendarChanged), changed+1)
	assert.Equal(t, "cancelled", outcome(t, c, cancel))

	e := inviteEvent(t, c)
	assert.Equal(t, "cancelled", e.Status)
	assert.Equal(t, 3, e.Sequence)
	items := december(t, c)
	require.Len(t, items, 1)
	assert.True(t, items[0].Cancelled)
	inv := inviteFor(t, c, cancel)
	assert.True(t, inv.Found)
	assert.Equal(t, "CANCEL", inv.Method)
	assert.True(t, inv.Cancelled)
	assert.Equal(t, e.ID, inv.EventID)

	_, err = c.RespondInvite(context.Background(), e.ID, "ACCEPTED")
	requireUserMsg(t, err, "invalid", "Esta invitación fue cancelada.")

	// Se quita del calendario; los registros quedan.
	require.NoError(t, c.RemoveCancelledEvent(context.Background(), e.ID))
	noEvents(t, c)
	assert.Len(t, n.matching(EventCalendarChanged), changed+2)
	inv = inviteFor(t, c, cancel)
	assert.True(t, inv.Found)
	assert.Zero(t, inv.EventID)
	assert.Equal(t, "Reunion de proyecto", inv.Title)
	assert.True(t, inv.Cancelled)
}

func TestInvite_CancelOccurrence(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	series := strings.Replace(sampleICS(t, "google-request.ics"), "DTEND:20261203T160000Z",
		"DTEND:20261203T160000Z\nRRULE:FREQ=WEEKLY;COUNT=4", 1)
	occ := strings.Replace(sampleICS(t, "google-cancel.ics"), "DTSTART:20261204T170000Z\nDTEND:20261204T180000Z",
		"DTSTART:20261210T150000Z\nDTEND:20261210T160000Z\nRECURRENCE-ID:20261210T150000Z", 1)
	require.Contains(t, occ, "RECURRENCE-ID")
	imap.Deliver(t, "INBOX", inviteMail("serie", orgFrom, series))
	imap.Deliver(t, "INBOX", inviteMail("cancela-una", orgFrom, occ))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "serie")
	require.Len(t, december(t, c), 4)
	open(t, c, inbox, "cancela-una")

	e := inviteEvent(t, c)
	assert.Equal(t, "confirmed", e.Status)
	require.Len(t, e.Exceptions, 1)
	assert.True(t, e.Exceptions[0].Cancelled)
	assert.Equal(t, tu(2026, 12, 10, 15, 0), e.Exceptions[0].RecurrenceAt)
	items := december(t, c)
	require.Len(t, items, 3)
	for _, it := range items {
		assert.NotEqual(t, tu(2026, 12, 10, 15, 0), it.Start)
		assert.False(t, it.Cancelled)
	}
}

func TestInvite_ReplyUpdatesAttendee(t *testing.T) {
	c, imap, _, n, acc := receiverCore(t, meEmail)
	ctx := context.Background()
	local := model.CalendarEvent{UID: sampleUID, Kind: "local", Title: "Reunion de proyecto", Status: "confirmed",
		Start: tu(2026, 12, 3, 15, 0), End: tu(2026, 12, 3, 16, 0), AccountID: acc.ID, Sequence: 1,
		Organizer:  model.Address{Name: "Fabricio", Email: meEmail},
		Attendees:  []model.Attendee{{Email: "otra@example.com", Name: "Otra Persona", Partstat: "NEEDS-ACTION"}},
		Exceptions: []model.EventException{}}
	saved, err := c.store.SaveEvent(ctx, local, nil)
	require.NoError(t, err)
	imap.Deliver(t, "INBOX", inviteMail("respuesta", "Otra Persona <otra@example.com>", sampleICS(t, "reply-accepted.ics")))
	stranger := strings.ReplaceAll(sampleICS(t, "reply-accepted.ics"), "CN=Otra Persona;X-NUM-GUESTS=0:mailto:otra@examp\n le.com",
		"CN=Nadie:mailto:nadie@example.com")
	require.Contains(t, stranger, "nadie@example.com")
	imap.Deliver(t, "INBOX", inviteMail("intruso", "Nadie <nadie@example.com>", stranger))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	changed := len(n.matching(EventCalendarChanged))
	id := open(t, c, inbox, "respuesta")
	e, err := c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	require.Len(t, e.Attendees, 1)
	assert.Equal(t, "ACCEPTED", e.Attendees[0].Partstat)
	assert.Equal(t, "reply", outcome(t, c, id))
	assert.Len(t, n.matching(EventCalendarChanged), changed+1)
	inv := inviteFor(t, c, id)
	assert.True(t, inv.Found)
	assert.Equal(t, "REPLY", inv.Method)
	assert.Equal(t, saved.ID, inv.EventID)
	assert.Equal(t, model.Address{Name: "Otra Persona", Email: "otra@example.com"}, inv.ReplyFrom)
	assert.Equal(t, "ACCEPTED", inv.ReplyStatus)

	// La respuesta de alguien que no estaba invitado no lo agrega.
	open(t, c, inbox, "intruso")
	e, err = c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	require.Len(t, e.Attendees, 1)
	assert.Equal(t, "otra@example.com", e.Attendees[0].Email)
	assert.Len(t, n.matching(EventCalendarChanged), changed+1)
}

func TestInvite_IgnoresSentAndOwn(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "Sent", calendarMail(t, "google-request.eml"))
	imap.Deliver(t, "INBOX", inviteMail("propio", "Fabricio <"+meEmail+">", sampleICS(t, "google-request.ics")))
	mine := strings.ReplaceAll(sampleICS(t, "google-request.ics"),
		"ORGANIZER;CN=Organizador Ejemplo:mailto:organizador@example.com", "ORGANIZER;CN=Fabricio:mailto:Fabricio@Example.com")
	imap.Deliver(t, "INBOX", inviteMail("organizo-yo", "Google Calendar <calendar-notification@google.com>", mine))
	syncAll(t, c, acc)
	inbox, sent := folderID(t, c, acc.ID, "INBOX"), folderID(t, c, acc.ID, "Sent")
	require.NoError(t, c.SyncFolder(context.Background(), sent))

	inSent := open(t, c, sent, "Reunión de proyecto")
	own := open(t, c, inbox, "propio")
	organized := open(t, c, inbox, "organizo-yo")

	noEvents(t, c)
	for _, id := range []int64{inSent, own} {
		_, ok, err := c.store.InviteRecord(context.Background(), id)
		require.NoError(t, err)
		assert.False(t, ok, "un correo ignorado no se registra")
		assert.False(t, inviteFor(t, c, id).Found)
	}
	assert.Equal(t, "own", outcome(t, c, organized))
}

func TestInvite_PastNotCreated(t *testing.T) {
	c, imap, _, n, acc := receiverCore(t, meEmail)
	c.now = func() time.Time { return tu(2027, 1, 1, 0, 0) }
	imap.Deliver(t, "INBOX", calendarMail(t, "google-request.eml"))
	syncAll(t, c, acc)
	before := len(n.matching(EventCalendarChanged))

	id := open(t, c, folderID(t, c, acc.ID, "INBOX"), "Reunión de proyecto")

	noEvents(t, c)
	assert.Equal(t, "past", outcome(t, c, id))
	assert.Len(t, n.matching(EventCalendarChanged), before)
	inv := inviteFor(t, c, id)
	assert.True(t, inv.Found)
	assert.True(t, inv.Past)
	assert.Zero(t, inv.EventID)
	assert.Equal(t, "Reunion de proyecto", inv.Title)
	assert.Equal(t, tu(2026, 12, 3, 15, 0), inv.Start)
}

func TestInvite_GarbageIgnored(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", inviteMail("roto", orgFrom, "BEGIN:VCALENDAR\nesto no es"))
	imap.Deliver(t, "INBOX", inviteMail("vacio", orgFrom, "BEGIN:VCALENDAR\nVERSION:2.0\nMETHOD:REQUEST\nEND:VCALENDAR"))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	for _, s := range []string{"roto", "vacio"} {
		id := open(t, c, inbox, s)
		_, ok, err := c.store.InviteRecord(context.Background(), id)
		require.NoError(t, err)
		assert.False(t, ok, s)
		assert.False(t, inviteFor(t, c, id).Found, s)
	}
	noEvents(t, c)
}

func TestRespond_SendsReplyFromMatchingAccount(t *testing.T) {
	c, imap, smtp, n, first := receiverCore(t, "f@ej.com")
	const otro = "otro@ej.com"
	imap.AddUser(t, otro, testPass)
	smtp.AddUser(otro, testPass)
	in := inputFor(imap, smtp, testPass)
	in.Account.Email, in.Account.Username, in.Account.DisplayName = meEmail, otro, "Fabricio Ejemplo"
	second, err := c.AddAccount(context.Background(), in)
	require.NoError(t, err)

	ics := strings.ReplaceAll(sampleICS(t, "google-request.ics"), "mailto:fabricio@example.com", "MAILTO:Fabricio@Example.com")
	imap.Deliver(t, "INBOX", inviteMail("request", orgFrom, ics))
	syncAll(t, c, first)
	open(t, c, folderID(t, c, first.ID, "INBOX"), "request")

	e := inviteEvent(t, c)
	assert.Equal(t, second.ID, e.AccountID)
	assert.Equal(t, meEmail, e.MyEmail)
	assert.False(t, e.AliasReply)
	e.NeedsReview = true
	_, err = c.store.SaveEvent(context.Background(), e, seriesEnd(e))
	require.NoError(t, err)

	changed := len(n.matching(EventCalendarChanged))
	res, err := c.RespondInvite(context.Background(), e.ID, "ACCEPTED")
	require.NoError(t, err)
	assert.Empty(t, res.Warning)

	envs := waitMails(t, smtp, 1)
	m := readMail(t, envs[0])
	assert.Equal(t, orgEmail, m.To)
	assert.Contains(t, m.From, meEmail, "sale de la cuenta 2, no de la que lo recibió")
	assert.Equal(t, "Aceptado: Reunion de proyecto", m.Subject)
	assert.Contains(t, m.ICS, "METHOD:REPLY")
	assert.Contains(t, m.ICS, "PARTSTAT=ACCEPTED")
	assert.Contains(t, m.ICS, "UID:"+sampleUID)
	assert.Contains(t, m.ICS, "SEQUENCE:1")
	assert.Contains(t, m.ICS, "mailto:"+meEmail)
	assert.NotContains(t, m.ICS, "otra@example.com", "solo va el que responde")
	assert.Contains(t, m.HTML, "aceptó la invitación a «Reunion de proyecto»")

	e = inviteEvent(t, c)
	assert.Equal(t, "ACCEPTED", e.MyPartstat)
	assert.False(t, e.NeedsReview)
	assert.Len(t, n.matching(EventCalendarChanged), changed+1)
	items := december(t, c)
	require.Len(t, items, 1)
	assert.Equal(t, "ACCEPTED", items[0].Partstat)
}

func TestRespond_AliasWarns(t *testing.T) {
	c, imap, smtp, _, acc := receiverCore(t, testUser)
	imap.Deliver(t, "INBOX", calendarMail(t, "google-request.eml"))
	syncAll(t, c, acc)
	open(t, c, folderID(t, c, acc.ID, "INBOX"), "Reunión de proyecto")

	e := inviteEvent(t, c)
	assert.True(t, e.AliasReply)
	assert.Equal(t, acc.ID, e.AccountID)
	assert.Equal(t, testUser, e.MyEmail)
	assert.Equal(t, "NEEDS-ACTION", e.MyPartstat)

	res, err := c.RespondInvite(context.Background(), e.ID, "TENTATIVE")
	require.NoError(t, err)
	assert.Equal(t, "Ninguna de tus cuentas figura como invitada; respondimos desde f@ej.com.", res.Warning)

	m := readMail(t, waitMails(t, smtp, 1)[0])
	assert.Equal(t, orgEmail, m.To)
	assert.Contains(t, m.From, testUser)
	assert.Equal(t, "Tal vez: Reunion de proyecto", m.Subject)
	assert.Contains(t, m.ICS, "PARTSTAT=TENTATIVE")
	assert.Contains(t, m.ICS, "mailto:"+testUser)
	assert.Equal(t, "TENTATIVE", inviteEvent(t, c).MyPartstat)
}

func TestRespond_InvalidPartstat(t *testing.T) {
	c, imap, smtp, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", calendarMail(t, "google-request.eml"))
	syncAll(t, c, acc)
	open(t, c, folderID(t, c, acc.ID, "INBOX"), "Reunión de proyecto")
	e := inviteEvent(t, c)

	for _, p := range []string{"", "MAYBE", "NEEDS-ACTION"} {
		_, err := c.RespondInvite(context.Background(), e.ID, p)
		requireUserMsg(t, err, "invalid", "Esa respuesta no es válida.")
	}
	assert.Equal(t, "NEEDS-ACTION", inviteEvent(t, c).MyPartstat)

	local, err := c.SaveEvent(context.Background(), model.EventSave{Event: model.CalendarEvent{Title: "Mío",
		Start: tu(2026, 12, 1, 13, 0), End: tu(2026, 12, 1, 14, 0), TZID: "UTC"}})
	require.NoError(t, err)
	_, err = c.RespondInvite(context.Background(), local.ID, "ACCEPTED")
	requireUserMsg(t, err, "invalid", "Este evento no es una invitación.")
	err = c.RemoveCancelledEvent(context.Background(), local.ID)
	requireUserMsg(t, err, "invalid", "Este evento no es una invitación.")
	noMoreMails(t, c, smtp, 0)
}

func TestInviteForMessage_LegacyCachedAttachment(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	ics := strings.ReplaceAll(strings.ReplaceAll(sampleICS(t, "google-request.ics"), "\r\n", "\n"), "\n", "\r\n")
	imap.Deliver(t, "INBOX", []byte("From: "+orgFrom+"\r\nTo: "+meEmail+"\r\nSubject: viejo\r\n"+
		"Date: Wed, 18 Nov 2026 12:00:00 +0000\r\nMessage-ID: <viejo@ej.com>\r\nMIME-Version: 1.0\r\n"+
		"Content-Type: multipart/mixed; boundary=b\r\n\r\n"+
		"--b\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nInvitación\r\n"+
		"--b\r\nContent-Type: application/ics; name=\"invite.ics\"\r\nContent-Disposition: attachment; filename=\"invite.ics\"\r\n\r\n"+
		ics+"\r\n--b--\r\n"))
	syncAll(t, c, acc)
	m := messageBySubject(t, c, folderID(t, c, acc.ID, "INBOX"), "viejo")

	// Cacheado antes de v18: el cuerpo está, pero nunca se miró el .ics.
	row := fetchBodyOf(t, c, m.ID)
	require.NotEmpty(t, row.Calendar)
	row.Calendar = nil
	require.NoError(t, c.store.SetBody(context.Background(), m.ID, row))
	require.True(t, bodyCached(t, c, m.ID))
	noEvents(t, c)

	inv := inviteFor(t, c, m.ID)
	assert.True(t, inv.Found)
	assert.Equal(t, "REQUEST", inv.Method)
	e := inviteEvent(t, c)
	assert.Equal(t, e.ID, inv.EventID)
	assert.Equal(t, "invite", e.Kind)
	assert.Equal(t, "created", outcome(t, c, m.ID))
}

// weeklySeries es google-request.ics como serie semanal de 4 (3, 10, 17 y 24/12 a las 15:00Z).
func weeklySeries(t *testing.T) string {
	t.Helper()
	return strings.Replace(sampleICS(t, "google-request.ics"), "DTEND:20261203T160000Z",
		"DTEND:20261203T160000Z\nRRULE:FREQ=WEEKLY;COUNT=4", 1)
}

// occurrenceOf cambia las horas de la muestra ics (oldTimes = "DTSTART:...\nDTEND:...")
// por una sola ocurrencia (RECURRENCE-ID rid) con esas horas y SEQUENCE seq.
func occurrenceOf(t *testing.T, ics, oldTimes, oldSeq, rid, start, end, seq string) string {
	t.Helper()
	require.Contains(t, ics, oldTimes)
	out := strings.Replace(ics, oldTimes, "DTSTART:"+start+"\nDTEND:"+end+"\nRECURRENCE-ID:"+rid, 1)
	return strings.Replace(out, "SEQUENCE:"+oldSeq, "SEQUENCE:"+seq, 1)
}

const (
	requestTimes = "DTSTART:20261203T150000Z\nDTEND:20261203T160000Z"
	cancelTimes  = "DTSTART:20261204T170000Z\nDTEND:20261204T180000Z"
)

func TestInvite_ReplyChangedAnswerSameSequence(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	ctx := context.Background()
	saved, err := c.store.SaveEvent(ctx, model.CalendarEvent{UID: sampleUID, Kind: "local", Title: "Reunion de proyecto",
		Status: "confirmed", Start: tu(2026, 12, 3, 15, 0), End: tu(2026, 12, 3, 16, 0), AccountID: acc.ID, Sequence: 1,
		Attendees: []model.Attendee{{Email: "otra@example.com", Partstat: "NEEDS-ACTION"}}}, nil)
	require.NoError(t, err)
	from := "Otra Persona <otra@example.com>"
	imap.Deliver(t, "INBOX", inviteMail("acepta", from, sampleICS(t, "reply-accepted.ics")))
	imap.Deliver(t, "INBOX", inviteMail("rechaza", from,
		strings.Replace(sampleICS(t, "reply-accepted.ics"), "PARTSTAT=ACCEPTED", "PARTSTAT=DECLINED", 1)))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	first := open(t, c, inbox, "acepta")
	second := open(t, c, inbox, "rechaza")

	e, err := c.store.GetEvent(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, "DECLINED", e.Attendees[0].Partstat, "la respuesta nueva, aunque tenga la misma SEQUENCE")
	assert.Equal(t, "reply", outcome(t, c, first))
	assert.Equal(t, "reply", outcome(t, c, second))
}

func TestInvite_CancelTwoOccurrencesSameSequence(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	cancel := sampleICS(t, "google-cancel.ics")
	imap.Deliver(t, "INBOX", inviteMail("serie", orgFrom, weeklySeries(t)))
	imap.Deliver(t, "INBOX", inviteMail("cancela-10", orgFrom,
		occurrenceOf(t, cancel, cancelTimes, "3", "20261210T150000Z", "20261210T150000Z", "20261210T160000Z", "3")))
	imap.Deliver(t, "INBOX", inviteMail("cancela-17", orgFrom,
		occurrenceOf(t, cancel, cancelTimes, "3", "20261217T150000Z", "20261217T150000Z", "20261217T160000Z", "3")))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "serie")
	ten := open(t, c, inbox, "cancela-10")
	seventeen := open(t, c, inbox, "cancela-17")

	e := inviteEvent(t, c)
	assert.Equal(t, 3, e.Sequence)
	require.Len(t, e.Exceptions, 2)
	for _, x := range e.Exceptions {
		assert.True(t, x.Cancelled)
	}
	assert.Len(t, december(t, c), 2)
	assert.Equal(t, "cancelled", outcome(t, c, ten))
	assert.Equal(t, "cancelled", outcome(t, c, seventeen))
	r, _, err := c.store.InviteRecord(context.Background(), ten)
	require.NoError(t, err)
	assert.Equal(t, sampleUID+"#"+strconv.FormatInt(tu(2026, 12, 10, 15, 0).Unix(), 10), r.UID)
	inv := inviteFor(t, c, ten)
	assert.Equal(t, e.ID, inv.EventID)
	assert.True(t, inv.Cancelled)
	assert.Equal(t, tu(2026, 12, 10, 15, 0), inv.Start)
}

func TestInvite_OverrideRequestsSameSequence(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	req := sampleICS(t, "google-request.ics")
	imap.Deliver(t, "INBOX", inviteMail("serie", orgFrom, weeklySeries(t)))
	imap.Deliver(t, "INBOX", inviteMail("mueve-10", orgFrom,
		occurrenceOf(t, req, requestTimes, "1", "20261210T150000Z", "20261210T180000Z", "20261210T190000Z", "2")))
	imap.Deliver(t, "INBOX", inviteMail("mueve-17", orgFrom,
		occurrenceOf(t, req, requestTimes, "1", "20261217T150000Z", "20261217T180000Z", "20261217T190000Z", "2")))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "serie")
	ten := open(t, c, inbox, "mueve-10")
	seventeen := open(t, c, inbox, "mueve-17")

	e := inviteEvent(t, c)
	assert.Equal(t, 2, e.Sequence)
	assert.Equal(t, "RRULE:FREQ=WEEKLY;COUNT=4", "RRULE:"+e.RRule, "la serie sigue entera")
	require.Len(t, e.Exceptions, 2)
	assert.Equal(t, tu(2026, 12, 10, 18, 0), e.Exceptions[0].Start)
	assert.Equal(t, tu(2026, 12, 17, 18, 0), e.Exceptions[1].Start)
	assert.Equal(t, "updated", outcome(t, c, ten))
	assert.Equal(t, "updated", outcome(t, c, seventeen))
	assert.Len(t, december(t, c), 4)
}

func TestInvite_OverrideMoveNeedsReview(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	req := sampleICS(t, "google-request.ics")
	imap.Deliver(t, "INBOX", inviteMail("serie", orgFrom, weeklySeries(t)))
	imap.Deliver(t, "INBOX", inviteMail("mueve-10", orgFrom,
		occurrenceOf(t, req, requestTimes, "1", "20261210T150000Z", "20261210T180000Z", "20261210T190000Z", "2")))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "serie")
	setPartstat(t, c, "ACCEPTED")
	open(t, c, inbox, "mueve-10")

	e := inviteEvent(t, c)
	assert.True(t, e.NeedsReview)
	assert.Equal(t, "NEEDS-ACTION", e.MyPartstat)
}

func TestInvite_OldCancelOutdated(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	imap.Deliver(t, "INBOX", inviteMail("request", orgFrom, sampleICS(t, "google-request.ics")))
	imap.Deliver(t, "INBOX", inviteMail("update", orgFrom, sampleICS(t, "google-update.ics")))
	old := strings.Replace(sampleICS(t, "google-cancel.ics"), "SEQUENCE:3", "SEQUENCE:1", 1)
	imap.Deliver(t, "INBOX", inviteMail("cancel-viejo", orgFrom, old))
	syncAll(t, c, acc)
	inbox := folderID(t, c, acc.ID, "INBOX")

	open(t, c, inbox, "request")
	open(t, c, inbox, "update")
	id := open(t, c, inbox, "cancel-viejo")

	assert.Equal(t, "outdated", outcome(t, c, id))
	e := inviteEvent(t, c)
	assert.Equal(t, "confirmed", e.Status)
	assert.Equal(t, 2, e.Sequence)
}

func TestInvite_AllDayTodayNotPast(t *testing.T) {
	// El reloj va en la zona de Buenos Aires (sin tocar time.Local, que leen
	// las goroutines del servidor de prueba).
	art := time.FixedZone("ART", -3*3600)
	c, imap, _, _, acc := receiverCore(t, meEmail)
	c.now = func() time.Time { return tu(2026, 11, 20, 0, 0).In(art) } // 21:00 del 19/11 en Buenos Aires
	today := strings.Replace(sampleICS(t, "google-request.ics"), requestTimes,
		"DTSTART;VALUE=DATE:20261119\nDTEND;VALUE=DATE:20261120", 1)
	imap.Deliver(t, "INBOX", inviteMail("hoy", orgFrom, today))
	syncAll(t, c, acc)

	id := open(t, c, folderID(t, c, acc.ID, "INBOX"), "hoy")

	assert.Equal(t, "created", outcome(t, c, id))
	assert.False(t, inviteFor(t, c, id).Past)
	c.now = func() time.Time { return tu(2026, 11, 20, 3, 0).In(art) } // medianoche en Buenos Aires
	assert.True(t, inviteFor(t, c, id).Past)
}

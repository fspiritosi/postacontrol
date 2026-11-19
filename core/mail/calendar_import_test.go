package mail

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

// writeZip arma un zip con las entradas dadas (nombre → contenido).
func writeZip(t *testing.T, name string, entries map[string]string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	f, err := os.Create(p)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	for n, body := range entries {
		w, err := zw.Create(n)
		require.NoError(t, err)
		_, err = w.Write([]byte(body))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())
	return p
}

func oneEventICS(name, uid, summary string) string {
	cal := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Test//ES\r\n"
	if name != "" {
		cal += "X-WR-CALNAME:" + name + "\r\n"
	}
	return cal + "BEGIN:VEVENT\r\nUID:" + uid + "\r\nDTSTAMP:20261119T120000Z\r\nDTSTART:20261201T100000Z\r\n" +
		"DTEND:20261201T110000Z\r\nSUMMARY:" + summary + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

func eventOf(t *testing.T, c *Core, uid string) model.CalendarEvent {
	t.Helper()
	e, err := c.store.EventByUID(context.Background(), uid)
	require.NoError(t, err)
	return e
}

func TestPreviewICS_ZipEntries(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	two := oneEventICS("Principal", "a@x", "A")
	two = strings.Replace(two, "END:VCALENDAR\r\n", "BEGIN:VEVENT\r\nUID:b@x\r\nDTSTAMP:20261119T120000Z\r\nDTSTART:20261202T100000Z\r\nDTEND:20261202T110000Z\r\nSUMMARY:B\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", 1)
	zp := writeZip(t, "takeout.zip", map[string]string{
		"Calendar/principal.ics": two,
		"Calendar/feriados.ics":  oneEventICS("", "f@x", "Feriado"),
		"Calendar/leeme.txt":     "nada",
	})
	plain := writeFile(t, "suelto.ics", oneEventICS("Suelto", "s@x", "S"))

	list, err := c.PreviewICSImport(context.Background(), []string{zp, plain})
	require.NoError(t, err)
	require.Len(t, list, 3)
	got := map[string]model.ICSCalendar{}
	for _, l := range list {
		got[l.ID] = l
	}
	assert.Equal(t, model.ICSCalendar{ID: zp + "#Calendar/principal.ics", Name: "Principal", Events: 2}, got[zp+"#Calendar/principal.ics"])
	assert.Equal(t, model.ICSCalendar{ID: zp + "#Calendar/feriados.ics", Name: "feriados", Events: 1}, got[zp+"#Calendar/feriados.ics"])
	assert.Equal(t, model.ICSCalendar{ID: plain, Name: "Suelto", Events: 1}, got[plain])
}

func TestPreviewICS_Unreadable(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	bad := writeFile(t, "basura.ics", "esto no es un calendario")
	_, err := c.PreviewICSImport(context.Background(), []string{bad})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "No pudimos leer basura.ics.", ue.Msg)
	assert.Equal(t, "invalid", ue.Code)

	_, err = c.ImportICS(context.Background(), []string{filepath.Join(t.TempDir(), "no-existe.ics")})
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "No pudimos leer no-existe.ics.", ue.Msg)
}

func TestImportICS_KindsAndNoMail(t *testing.T) {
	c, _, smtp, n, acc := receiverCore(t, meEmail)
	ctx := context.Background()
	p := writeFile(t, "google.ics", sampleICS(t, "google-export.ics"))

	prev, err := c.PreviewICSImport(ctx, []string{p})
	require.NoError(t, err)
	require.Len(t, prev, 1)
	assert.Equal(t, meEmail, prev[0].Name)
	assert.Equal(t, 5, prev[0].Events)

	res, err := c.ImportICS(ctx, []string{p})
	require.NoError(t, err)
	assert.Equal(t, model.ICSImportResult{Added: 5}, res)
	assert.Len(t, n.matching(EventCalendarChanged), 1, "un solo aviso al final")

	other := eventOf(t, c, "organiza-otro-planificacion@google.com")
	assert.Equal(t, "invite", other.Kind)
	assert.Equal(t, "ACCEPTED", other.MyPartstat)
	assert.Equal(t, meEmail, other.MyEmail)
	assert.Equal(t, acc.ID, other.AccountID)
	assert.Equal(t, orgEmail, other.Organizer.Email)
	assert.Contains(t, other.RawICS, "METHOD:PUBLISH")
	assert.Contains(t, other.RawICS, "organiza-otro-planificacion@google.com")

	own := eventOf(t, c, "propio-demo-cliente@google.com")
	assert.Equal(t, "local", own.Kind)
	assert.Equal(t, acc.ID, own.AccountID)
	assert.Equal(t, meEmail, own.Organizer.Email)
	require.Len(t, own.Attendees, 1)
	assert.Equal(t, "otra@example.com", own.Attendees[0].Email)
	assert.Empty(t, own.MyPartstat)
	assert.Empty(t, own.MyEmail)
	assert.Empty(t, own.RawICS)

	series := eventOf(t, c, "serie-semanal-daily@google.com")
	assert.Equal(t, "local", series.Kind)
	assert.Equal(t, "America/Argentina/Buenos_Aires", series.TZID)
	require.Len(t, series.Exceptions, 2)
	var cancelled, moved int
	for _, x := range series.Exceptions {
		if x.Cancelled {
			cancelled++
		} else {
			moved++
			assert.Equal(t, "Daily del equipo (movida a la tarde)", x.Title)
		}
	}
	assert.Equal(t, 1, cancelled)
	assert.Equal(t, 1, moved)
	assert.NotNil(t, seriesEnd(series), "la serie con COUNT termina")

	day := eventOf(t, c, "todo-el-dia-feriado@google.com")
	assert.True(t, day.AllDay)
	assert.Equal(t, tu(2026, 12, 8, 0, 0), day.Start)
	assert.Equal(t, tu(2026, 12, 9, 0, 0), day.End)
	assert.Equal(t, int64(0), day.AccountID)
	assert.Equal(t, localTZID(), day.TZID)

	assert.Equal(t, "local", eventOf(t, c, "pasado-evento-viejo@google.com").Kind, "los pasados también entran")
	noMoreMails(t, c, smtp, 0)
}

func TestImportICS_Twice(t *testing.T) {
	c, _, _, n, _ := receiverCore(t, meEmail)
	ctx := context.Background()
	p := writeFile(t, "google.ics", sampleICS(t, "google-export.ics"))
	_, err := c.ImportICS(ctx, []string{p})
	require.NoError(t, err)
	before := len(n.matching(EventCalendarChanged))

	res, err := c.ImportICS(ctx, []string{p})
	require.NoError(t, err)
	assert.Equal(t, model.ICSImportResult{Skipped: 5}, res)
	assert.Len(t, n.matching(EventCalendarChanged), before, "sin cambios no se avisa")
	all, err := c.store.AllEvents(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 5)
}

func TestImportICS_SameUIDInOneImport(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	a := writeFile(t, "a.ics", oneEventICS("", "dup@x", "Primero"))
	b := writeFile(t, "b.ics", oneEventICS("", "dup@x", "Segundo"))
	res, err := c.ImportICS(context.Background(), []string{a, b})
	require.NoError(t, err)
	assert.Equal(t, model.ICSImportResult{Added: 1, Skipped: 1}, res)
	assert.Equal(t, "Primero", eventOf(t, c, "dup@x").Title)
}

func TestImportICS_SkippedVEventsCount(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	ics := strings.Replace(oneEventICS("", "ok@x", "Bueno"), "END:VCALENDAR\r\n",
		"BEGIN:VEVENT\r\nUID:roto@x\r\nSUMMARY:Sin hora\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n", 1)
	res, err := c.ImportICS(context.Background(), []string{writeFile(t, "a.ics", ics)})
	require.NoError(t, err)
	assert.Equal(t, model.ICSImportResult{Added: 1, Skipped: 1}, res)
}

func TestImportICS_NewerSequenceUpdatesInvite(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	ctx := context.Background()
	src := sampleICS(t, "google-export.ics")
	_, err := c.ImportICS(ctx, []string{writeFile(t, "a.ics", src)})
	require.NoError(t, err)
	e := eventOf(t, c, "organiza-otro-planificacion@google.com")
	e.Notes = "mis notas"
	remind := 30
	e.RemindBefore = &remind
	_, err = c.store.SaveEvent(ctx, e, seriesEnd(e))
	require.NoError(t, err)

	newer := strings.Replace(src, "SUMMARY:Planificacion trimestral", "SUMMARY:Planificacion anual", 1)
	// Misma SEQUENCE: se saltea todo.
	res, err := c.ImportICS(ctx, []string{writeFile(t, "b.ics", newer)})
	require.NoError(t, err)
	assert.Equal(t, 5, res.Skipped)
	assert.Equal(t, "Planificacion trimestral", eventOf(t, c, e.UID).Title)

	// SEQUENCE mayor del invite: se actualiza y conserva las notas y el aviso; el propio no.
	bumped := strings.Replace(newer, "SEQUENCE:0\nSTATUS:CONFIRMED\nSUMMARY:Planificacion anual", "SEQUENCE:3\nSTATUS:CONFIRMED\nSUMMARY:Planificacion anual", 1)
	require.NotEqual(t, newer, bumped)
	res, err = c.ImportICS(ctx, []string{writeFile(t, "c.ics", bumped)})
	require.NoError(t, err)
	assert.Equal(t, model.ICSImportResult{Updated: 1, Skipped: 4}, res)
	got := eventOf(t, c, e.UID)
	assert.Equal(t, "Planificacion anual", got.Title)
	assert.Equal(t, 3, got.Sequence)
	assert.Equal(t, "mis notas", got.Notes)
	require.NotNil(t, got.RemindBefore)
	assert.Equal(t, 30, *got.RemindBefore)
}

func TestImportICS_TooBig(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	p := filepath.Join(t.TempDir(), "enorme.zip")
	f, err := os.Create(p)
	require.NoError(t, err)
	zw := zip.NewWriter(f)
	w, err := zw.Create("grande.ics")
	require.NoError(t, err)
	chunk := make([]byte, 1<<20)
	for range 51 {
		_, err = w.Write(chunk)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, f.Close())

	_, err = c.PreviewICSImport(context.Background(), []string{p})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "Ese archivo es demasiado grande para importar.", ue.Msg)
	assert.Equal(t, "invalid", ue.Code)
	_, err = c.ImportICS(context.Background(), []string{p + "#grande.ics"})
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "Ese archivo es demasiado grande para importar.", ue.Msg)
}

func TestImportICS_TooManyEntries(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	entries := map[string]string{}
	for i := range 51 {
		entries[strings.Repeat("x", i+1)+".ics"] = oneEventICS("", "u@x", "U")
	}
	_, err := c.PreviewICSImport(context.Background(), []string{writeZip(t, "muchos.zip", entries)})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "Ese archivo es demasiado grande para importar.", ue.Msg)
}

func TestImportICS_FromZipEntry(t *testing.T) {
	c, _, _, _, _ := receiverCore(t, meEmail)
	zp := writeZip(t, "t.zip", map[string]string{"cal/a.ics": oneEventICS("A", "za@x", "Za"), "cal/b.ics": oneEventICS("B", "zb@x", "Zb")})
	res, err := c.ImportICS(context.Background(), []string{zp + "#cal/b.ics"})
	require.NoError(t, err)
	assert.Equal(t, model.ICSImportResult{Added: 1}, res)
	assert.Equal(t, "Zb", eventOf(t, c, "zb@x").Title)
}

func TestImportedOwnEvent_NextSequence(t *testing.T) {
	c, _, smtp, _, acc := receiverCore(t, meEmail)
	ctx := context.Background()
	src := strings.Replace(sampleICS(t, "google-export.ics"), "LOCATION:\nSEQUENCE:0\nSTATUS:CONFIRMED\nSUMMARY:Demo con cliente",
		"LOCATION:\nSEQUENCE:4\nSTATUS:CONFIRMED\nSUMMARY:Demo con cliente", 1)
	require.Contains(t, src, "SEQUENCE:4")
	_, err := c.ImportICS(ctx, []string{writeFile(t, "g.ics", src)})
	require.NoError(t, err)
	e := eventOf(t, c, "propio-demo-cliente@google.com")
	require.Equal(t, 4, e.Sequence)
	noMoreMails(t, c, smtp, 0)

	e.Start, e.End = e.Start.Add(time.Hour), e.End.Add(time.Hour)
	e.AccountID = acc.ID
	_, err = c.SaveEvent(ctx, model.EventSave{Event: e, Notify: true})
	require.NoError(t, err)
	mails := mailsTo(t, waitMails(t, smtp, 1), "otra@example.com")
	require.Len(t, mails, 1)
	assert.Contains(t, mails[0].ICS, "SEQUENCE:5")
	assert.Equal(t, 5, eventOf(t, c, e.UID).Sequence)
}

func TestImportedInvite_LaterUpdateApplies(t *testing.T) {
	c, imap, _, _, acc := receiverCore(t, meEmail)
	ctx := context.Background()
	src := sampleICS(t, "google-export.ics")
	_, err := c.ImportICS(ctx, []string{writeFile(t, "g.ics", src)})
	require.NoError(t, err)
	before := eventOf(t, c, "organiza-otro-planificacion@google.com")
	require.Equal(t, 0, before.Sequence)

	req := "BEGIN:VCALENDAR\nVERSION:2.0\nPRODID:-//Test//ES\nMETHOD:REQUEST\nBEGIN:VEVENT\nUID:organiza-otro-planificacion@google.com\n" +
		"DTSTAMP:20261119T130000Z\nDTSTART:20261212T140000Z\nDTEND:20261212T150000Z\nORGANIZER;CN=Organizador Ejemplo:mailto:" + orgEmail + "\n" +
		"ATTENDEE;PARTSTAT=NEEDS-ACTION:mailto:" + meEmail + "\nSEQUENCE:1\nSUMMARY:Planificacion trimestral\nEND:VEVENT\nEND:VCALENDAR\n"
	imap.Deliver(t, "INBOX", inviteMail("update", orgFrom, req))
	syncAll(t, c, acc)
	open(t, c, folderID(t, c, acc.ID, "INBOX"), "update")

	e := eventOf(t, c, before.UID)
	assert.Equal(t, 1, e.Sequence)
	assert.Equal(t, tu(2026, 12, 12, 14, 0), e.Start)
	assert.Equal(t, before.ID, e.ID, "es el mismo evento, no un duplicado")
	all, err := c.store.AllEvents(ctx)
	require.NoError(t, err)
	assert.Len(t, all, 5)
}

func TestImportICS_NoOrganizerNoAccounts(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	res, err := c.ImportICS(context.Background(), []string{writeFile(t, "a.ics", oneEventICS("", "x@x", "X"))})
	require.NoError(t, err)
	assert.Equal(t, 1, res.Added)
	e := eventOf(t, c, "x@x")
	assert.Equal(t, "local", e.Kind)
	assert.Equal(t, int64(0), e.AccountID)
}

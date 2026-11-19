package mail

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
	"postacontrol/core/testenv"
)

func wrap76(s string) string {
	var sb strings.Builder
	for len(s) > 76 {
		sb.WriteString(s[:76] + "\r\n")
		s = s[76:]
	}
	return sb.String() + s + "\r\n"
}

func mailWith(subject, ctype, body string) []byte {
	return []byte("From: Bea <bea@ej.com>\r\nTo: " + testUser + "\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <" + subject + "@ej.com>\r\n" +
		"MIME-Version: 1.0\r\nContent-Type: " + ctype + "\r\n\r\n" + body)
}

func firstMessage(t *testing.T, c *Core, folderID int64) model.MessageSummary {
	t.Helper()
	ms := listAll(t, c, folderID)
	require.NotEmpty(t, ms)
	return ms[0]
}

func TestGetMessage_FetchesSanitizesAndMarksSeen(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	imap.Deliver(t, "INBOX", mailWith("html", "text/html; charset=utf-8",
		"<p>Hola <b>mundo</b></p><script>alert(1)</script><img src=\"https://x.com/a.png\">\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)
	require.False(t, m.Seen)

	d, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(d.HTML), "script")
	assert.Contains(t, d.HTML, "mundo")
	assert.True(t, d.HasRemoteImages)
	assert.NotEmpty(t, d.Snippet)
	assert.Contains(t, d.Text, "Hola mundo")
	assert.True(t, d.Seen)
	assert.NotNil(t, d.Attachments)
	assert.Equal(t, []model.Address{{Email: testUser}}, d.To)

	cached, _, err := c.store.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	assert.True(t, cached.Seen)

	// En el servidor quedó \Seen.
	require.NoError(t, c.SyncFolder(context.Background(), inboxID))
	assert.True(t, firstMessage(t, c, inboxID).Seen)
	assert.Contains(t, serverFlags(t, c, imap, m.UID), goimap.FlagSeen)
	assert.Contains(t, n.matching(EventFolderUpdated), map[string]any{"folderId": inboxID})
	fs, err := c.ListFolders(context.Background(), m.AccountID)
	require.NoError(t, err)
	for _, f := range fs {
		if f.ID == inboxID {
			assert.Equal(t, 0, f.Unread)
		}
	}
}

// serverFlags consulta los flags directo al servidor con una conexión aparte.
func serverFlags(t *testing.T, c *Core, imap *testenv.IMAP, uid uint32) []goimap.Flag {
	t.Helper()
	conn, err := imapx.Dial(context.Background(), imap.Settings, testUser, testPass, c.cfg.TLS)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.Select("INBOX")
	require.NoError(t, err)
	fl, err := conn.FetchFlags([]uint32{uid})
	require.NoError(t, err)
	return fl[uid]
}

func storeBody(text string) store.BodyRow { return store.BodyRow{Text: text, Snippet: text} }

func TestGetMessage_SecondCallUsesCache(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", mailWith("txt", "text/plain; charset=utf-8", "hola\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)

	first, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	imap.Close()
	second, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestGetMessage_CachedButUnseenOffline(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	imap.Deliver(t, "INBOX", mailWith("txt", "text/plain; charset=utf-8", "hola\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)
	// Cuerpo cacheado pero el mensaje sigue sin leer.
	require.NoError(t, c.store.SetBody(context.Background(), m.ID, storeBody("hola")))
	imap.Close()

	d, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	assert.Equal(t, "hola", d.Text)
	assert.False(t, d.Seen)
	assert.NotEmpty(t, n.matching(EventSyncError))
	cached, _, err := c.store.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	assert.False(t, cached.Seen)
}

func TestGetMessage_InlineCIDBecomesDataURI(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG-fake"))
	body := "--b\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>x</p><img src=\"cid:logo@ej\">\r\n" +
		"--b\r\nContent-Type: image/png\r\nContent-ID: <logo@ej>\r\nContent-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: inline\r\n\r\n" + png + "\r\n--b--\r\n"
	imap.Deliver(t, "INBOX", mailWith("rel", "multipart/related; boundary=b", body))
	_, inboxID := setupSynced(t, imap, smtp, c)

	d, err := c.GetMessage(context.Background(), firstMessage(t, c, inboxID).ID)
	require.NoError(t, err)
	assert.Contains(t, d.HTML, "data:image/png;base64,"+png)
	assert.NotContains(t, d.HTML, "cid:")
}

func TestGetMessage_TextOnly(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", mailWith("txt", "text/plain; charset=utf-8", "solo texto\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	d, err := c.GetMessage(context.Background(), firstMessage(t, c, inboxID).ID)
	require.NoError(t, err)
	assert.Equal(t, "", d.HTML)
	assert.Contains(t, d.Text, "solo texto")
	assert.Equal(t, "solo texto", d.Snippet)
}

func TestGetMessage_UnknownID(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	_, err := c.GetMessage(context.Background(), 999)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "not_found", ue.Code)
}

var pdfBytes = []byte("%PDF-1.4\n\x00\x01\x02\xff binario \r\n%%EOF")

func attachmentMail(filename string) []byte {
	disp := "attachment"
	if filename != "" {
		disp += `; filename="` + filename + `"`
	}
	body := "--b\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nadjunto\r\n" +
		"--b\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: " + disp + "\r\n\r\n" + wrap76(base64.StdEncoding.EncodeToString(pdfBytes)) +
		"--b--\r\n"
	return mailWith("adj", "multipart/mixed; boundary=b", body)
}

func TestSaveAttachment_WritesExactBytes(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", attachmentMail("factura.pdf"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)

	d, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	require.Len(t, d.Attachments, 1)

	dest := filepath.Join(t.TempDir(), "out.pdf")
	require.NoError(t, os.WriteFile(dest, []byte("viejo"), 0o600))
	require.NoError(t, c.SaveAttachment(context.Background(), m.ID, 0, dest))
	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, pdfBytes, got)
}

func TestSaveAttachment_NoFilename(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", attachmentMail(""))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)

	att, data, err := c.attachmentBytes(context.Background(), m.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, "", att.Filename)
	assert.Equal(t, pdfBytes, data)
	dest := filepath.Join(t.TempDir(), "x.bin")
	require.NoError(t, c.SaveAttachment(context.Background(), m.ID, 0, dest))
}

func TestSaveAttachment_BadIndex(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", attachmentMail("factura.pdf"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)
	for _, idx := range []int{-1, 1, 5} {
		err := c.SaveAttachment(context.Background(), m.ID, idx, filepath.Join(t.TempDir(), "x"))
		var ue *UserError
		require.ErrorAs(t, err, &ue)
		assert.Equal(t, "not_found", ue.Code)
		assert.Equal(t, msgNotFound, ue.Msg)
	}
}

func TestGetMessage_CacheErrorAfterServerSeenIsReturned(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	imap.Deliver(t, "INBOX", mailWith("txt", "text/plain; charset=utf-8", "hola\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)
	c.afterServerSeen = func() { _ = c.store.Close() }

	_, err := c.GetMessage(context.Background(), m.ID)
	require.Error(t, err)
	// El STORE llegó al servidor aunque la caché falló.
	assert.Contains(t, serverFlags(t, c, imap, m.UID), goimap.FlagSeen)
	assert.Empty(t, n.matching(EventSyncError))
}

func TestGetMessage_ExpungedOnServerIsNotFound(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", mailWith("txt", "text/plain; charset=utf-8", "hola\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)
	imap.Expunge(t, "INBOX", m.UID)

	_, err := c.GetMessage(context.Background(), m.ID)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "not_found", ue.Code)
	assert.Equal(t, msgNotFound, ue.Msg)
}

func TestGetMessage_UIDValidityChangedIsStale(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", mailWith("txt", "text/plain; charset=utf-8", "hola\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)
	imap.RecreateMailbox(t, "INBOX")
	imap.Deliver(t, "INBOX", mailWith("otro", "text/plain; charset=utf-8", "otro\r\n"))

	_, err := c.GetMessage(context.Background(), m.ID)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "stale", ue.Code)
	// Contrato con el frontend (STALE_MESSAGE_FRAGMENT en src/lib/errors.ts).
	assert.Contains(t, ue.Msg, "cambió en el servidor")
}

func TestAttachmentData_ReturnsBytes(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", attachmentMail("factura.pdf"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)

	got, err := c.AttachmentData(context.Background(), m.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, pdfBytes, got.Data)
	assert.Equal(t, "factura.pdf", got.Filename)
	assert.Equal(t, "application/pdf", got.MIMEType)
}

func TestAttachmentData_TooBigSkipsFetch(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", attachmentMail("factura.pdf"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)

	// Deja el detalle en la caché y fuerza el tamaño del adjunto.
	d, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	require.Len(t, d.Attachments, 1)
	// 80 MB codificados son unos 60 MB decodificados: no entra.
	d.Attachments[0].Size = 80 << 20
	require.NoError(t, c.store.SetBody(context.Background(), m.ID, store.BodyRow{
		HTML: d.HTML, Text: d.Text, HasRemoteImages: d.HasRemoteImages, Attachments: d.Attachments,
	}))
	// Sin servidor, cualquier FETCH daría otro error.
	imap.Stop()

	_, err = c.AttachmentData(context.Background(), m.ID, 0)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "too_big", ue.Code)
	assert.Equal(t, "Es muy grande para verlo acá.", ue.Msg)
}

// Size es el tamaño en el servidor, en base64 (~4/3 del real): un adjunto de
// 45 MB llega como ~60 MB y se tiene que poder ver.
func TestAttachmentData_EncodedSizeOverCapStillFetches(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", attachmentMail("factura.pdf"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)

	d, err := c.GetMessage(context.Background(), m.ID)
	require.NoError(t, err)
	require.Len(t, d.Attachments, 1)
	d.Attachments[0].Size = 60 << 20
	require.NoError(t, c.store.SetBody(context.Background(), m.ID, store.BodyRow{
		HTML: d.HTML, Text: d.Text, HasRemoteImages: d.HasRemoteImages, Attachments: d.Attachments,
	}))

	got, err := c.AttachmentData(context.Background(), m.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, pdfBytes, got.Data)
}

func TestAttachmentData_BadIndex(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", attachmentMail("factura.pdf"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	m := firstMessage(t, c, inboxID)
	for _, idx := range []int{-1, 1, 5} {
		_, err := c.AttachmentData(context.Background(), m.ID, idx)
		var ue *UserError
		require.ErrorAs(t, err, &ue)
		assert.Equal(t, "not_found", ue.Code)
	}
}

func calendarMail(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "icalx", "testdata", name))
	require.NoError(t, err)
	return raw
}

func fetchBodyOf(t *testing.T, c *Core, id int64) store.BodyRow {
	t.Helper()
	d, _, err := c.store.GetMessage(context.Background(), id)
	require.NoError(t, err)
	row, err := c.fetchBody(context.Background(), d)
	require.NoError(t, err)
	return row
}

func TestFetchBody_Calendar(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", calendarMail(t, "google-request.eml"))
	imap.Deliver(t, "INBOX", calendarMail(t, "outlook-request.eml"))
	imap.Deliver(t, "INBOX", mailWith("sinics", "text/plain; charset=utf-8", "hola\r\n"))
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR\r\n"
	big := strings.Repeat("A", 1<<20+10)
	imap.Deliver(t, "INBOX", mailWith("grande", "multipart/mixed; boundary=b",
		"--b\r\nContent-Type: text/plain\r\n\r\nhola\r\n"+
			"--b\r\nContent-Type: text/calendar; name=\"x.ics\"\r\nContent-Disposition: attachment; filename=\"x.ics\"\r\n\r\n"+big+"\r\n--b--\r\n"))
	imap.Deliver(t, "INBOX", mailWith("adjunto", "multipart/mixed; boundary=b",
		"--b\r\nContent-Type: text/plain\r\n\r\nhola\r\n"+
			"--b\r\nContent-Type: application/octet-stream\r\nContent-Disposition: attachment; filename=\"Evento.ICS\"\r\n"+
			"Content-Transfer-Encoding: base64\r\n\r\n"+base64.StdEncoding.EncodeToString([]byte(ics))+"\r\n--b--\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)

	bySubject := map[string]model.MessageSummary{}
	for _, m := range listAll(t, c, inboxID) {
		switch {
		case strings.Contains(m.Subject, "Reunión de proyecto"):
			bySubject["google"] = m
		case strings.Contains(m.Subject, "Seguimiento Outlook"):
			bySubject["outlook"] = m
		default:
			bySubject[m.Subject] = m
		}
	}
	require.Len(t, bySubject, 5)

	gs := fetchBodyOf(t, c, bySubject["google"].ID)
	assert.Contains(t, string(gs.Calendar), "BEGIN:VCALENDAR")
	assert.Contains(t, string(gs.Calendar), "METHOD:REQUEST")
	assert.Len(t, gs.Attachments, 1, "invite.ics sigue como adjunto")

	ol := fetchBodyOf(t, c, bySubject["outlook"].ID)
	assert.Contains(t, string(ol.Calendar), "METHOD:REQUEST")
	assert.Empty(t, ol.Attachments)

	// El que no tiene invitación no trae nada.
	assert.Empty(t, fetchBodyOf(t, c, bySubject["sinics"].ID).Calendar)
	// Un .ics adjunto de más de 1 MB no se baja.
	assert.Empty(t, fetchBodyOf(t, c, bySubject["grande"].ID).Calendar)
	// Un adjunto .ics (por extensión) se baja y decodifica.
	assert.Equal(t, ics, string(fetchBodyOf(t, c, bySubject["adjunto"].ID).Calendar))
}

package mail

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	gomail "github.com/emersion/go-message/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/testenv"
)

func addrs(emails ...string) []model.Address {
	out := []model.Address{}
	for _, e := range emails {
		out = append(out, model.Address{Email: e})
	}
	return out
}

func threadMail() []byte {
	return []byte("From: a@x\r\nTo: f@ej.com, b@x\r\nCc: c@x\r\nSubject: Hola\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <orig@x>\r\nReferences: <r1@x>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\ncuerpo\r\n")
}

func prepared(t *testing.T, raw []byte) (*Core, *testenv.IMAP, *testenv.SMTP, model.Account, int64, model.MessageSummary) {
	t.Helper()
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", raw)
	acc, inboxID := setupSynced(t, imap, smtp, c)
	return c, imap, smtp, acc, inboxID, firstMessage(t, c, inboxID)
}

func parseEnvelope(t *testing.T, e testenv.Envelope) *gomail.Reader {
	t.Helper()
	r, err := gomail.CreateReader(bytes.NewReader(e.Data))
	require.NoError(t, err)
	return r
}

// envelopePart devuelve, decodificada, la parte en línea ctype del correo.
func envelopePart(t *testing.T, e testenv.Envelope, ctype string) string {
	t.Helper()
	r := parseEnvelope(t, e)
	for {
		p, err := r.NextPart()
		if err != nil {
			return ""
		}
		if ih, ok := p.Header.(*gomail.InlineHeader); ok {
			if ct, _, _ := ih.ContentType(); ct == ctype {
				b, err := io.ReadAll(p.Body)
				require.NoError(t, err)
				return string(b)
			}
		}
	}
}

func sentMessages(t *testing.T, c *Core, accID int64) []model.MessageSummary {
	t.Helper()
	require.NoError(t, c.SyncFolder(context.Background(), folderID(t, c, accID, "Sent")))
	return listAll(t, c, folderID(t, c, accID, "Sent"))
}

func TestPrepareCompose_ReplyAndReplyAllRecipients(t *testing.T) {
	c, _, _, _, _, m := prepared(t, threadMail())
	ctx := context.Background()

	d, err := c.PrepareCompose(ctx, m.ID, "reply")
	require.NoError(t, err)
	assert.Equal(t, addrs("a@x"), d.To)
	assert.Equal(t, []model.Address{}, d.Cc)
	assert.Equal(t, m.AccountID, d.AccountID)
	assert.Equal(t, model.ReplyContext{Mode: "reply", OriginalMessageID: m.ID}, d.Reply)

	d, err = c.PrepareCompose(ctx, m.ID, "replyAll")
	require.NoError(t, err)
	assert.Equal(t, addrs("a@x"), d.To)
	assert.Equal(t, addrs("b@x", "c@x"), d.Cc)

	d, err = c.PrepareCompose(ctx, m.ID, "forward")
	require.NoError(t, err)
	assert.Equal(t, []model.Address{}, d.To)
}

func TestFormatDateES(t *testing.T) {
	assert.Equal(t, "2 de enero de 2026, 15:04", formatDateES(time.Date(2026, 1, 2, 15, 4, 0, 0, time.Local)))
	assert.Equal(t, "31 de diciembre de 2026, 09:05", formatDateES(time.Date(2026, 12, 31, 9, 5, 0, 0, time.Local)))
}

func TestPrepareCompose_ReplyToWins(t *testing.T) {
	c, _, _, _, _, m := prepared(t, []byte("From: a@x\r\nReply-To: r@x\r\nTo: f@ej.com\r\nSubject: S\r\n"+
		"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <q@x>\r\n\r\nhola\r\n"))
	d, err := c.PrepareCompose(context.Background(), m.ID, "reply")
	require.NoError(t, err)
	assert.Equal(t, addrs("r@x"), d.To)
}

func TestPrepareCompose_SubjectPrefixNotDoubled(t *testing.T) {
	c, _, _, _, _, m := prepared(t, mailWith("Re: Hola", "text/plain", "x\r\n"))
	d, err := c.PrepareCompose(context.Background(), m.ID, "reply")
	require.NoError(t, err)
	assert.Equal(t, "Re: Hola", d.Subject)

	c2, _, _, _, _, m2 := prepared(t, mailWith("RV: Hola", "text/plain", "x\r\n"))
	d, err = c2.PrepareCompose(context.Background(), m2.ID, "forward")
	require.NoError(t, err)
	assert.Equal(t, "Fwd: RV: Hola", d.Subject)
}

// La cita que ve el redactor mantiene bloqueadas las imágenes remotas; el
// correo que sale las lleva restauradas, como en el original.
func TestPrepareCompose_QuoteBlocksRemoteImagesUntilSent(t *testing.T) {
	c, _, smtp, acc, _, m := prepared(t, mailWith("img", "text/html; charset=utf-8",
		`<p>Hola</p><img src="https://x.com/a.png">`+"\r\n"))
	d, err := c.PrepareCompose(context.Background(), m.ID, "reply")
	require.NoError(t, err)
	assert.Contains(t, d.QuotedHTML, `data-remote-src="https://x.com/a.png"`)
	assert.NotContains(t, d.QuotedHTML, ` src="https://x.com/a.png"`)

	_, err = c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: d.To, Subject: d.Subject, BodyHTML: "<p>ok</p>",
		Reply: &model.ReplyContext{Mode: "reply", OriginalMessageID: m.ID},
	})
	require.NoError(t, err)
	envs := smtp.Received()
	require.Len(t, envs, 1)
	sentHTML := envelopePart(t, envs[0], "text/html")
	assert.Contains(t, sentHTML, `<img src="https://x.com/a.png"`)
	assert.NotContains(t, sentHTML, "data-remote-src")

	assert.Contains(t, d.QuotedHTML, "escribió:")
	assert.Contains(t, d.QuotedHTML, `<blockquote type="cite"`)
	assert.Regexp(t, `El \d{1,2} de [a-z]+ de \d{4}, \d{2}:\d{2}, `, d.QuotedHTML)
	assert.Contains(t, d.QuotedHTML, "Bea &lt;bea@ej.com&gt;")

	f, err := c.PrepareCompose(context.Background(), m.ID, "forward")
	require.NoError(t, err)
	assert.Contains(t, f.QuotedHTML, "---------- Mensaje reenviado ----------")
	assert.Contains(t, f.QuotedHTML, "Asunto: img")
}

func TestSend_EndToEnd(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	in := inputFor(imap, smtp, testPass)
	in.DisplayName = "Fabricio Peña"
	acc, err := c.AddAccount(context.Background(), in)
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))

	res, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "Información", BodyHTML: "<p>Hola</p>",
	})
	require.NoError(t, err)
	assert.True(t, res.SentCopySaved)
	assert.Empty(t, res.Warning)

	envs := smtp.Received()
	require.Len(t, envs, 1)
	assert.Equal(t, testUser, envs[0].From)
	assert.Equal(t, []string{"d@x"}, envs[0].To)
	r := parseEnvelope(t, envs[0])
	subj, err := r.Header.Subject()
	require.NoError(t, err)
	assert.Equal(t, "Información", subj)
	from, err := r.Header.AddressList("From")
	require.NoError(t, err)
	assert.Equal(t, "Fabricio Peña", from[0].Name)
	assert.NotEmpty(t, r.Header.Get("Message-Id"))

	sent := sentMessages(t, c, acc.ID)
	require.Len(t, sent, 1)
	assert.True(t, sent[0].Seen)
	assert.Equal(t, "Información", sent[0].Subject)
	assert.Empty(t, outbox(t, c), "el correo enviado no queda en la bandeja de salida")
}

// La conexión del pool puede estar muerta (el servidor la cortó mientras la
// app esperaba). Antes del APPEND sin reintento se valida con un NOOP que sí
// reintenta: la copia se guarda una sola vez.
func TestSend_SentCopySurvivesDeadPooledConnection(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	imap.Close()
	imap.Restart(t)

	res, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "Tras corte", BodyHTML: "<p>x</p>",
	})
	require.NoError(t, err)
	assert.True(t, res.SentCopySaved)
	assert.Empty(t, res.Warning)
	sent := sentMessages(t, c, acc.ID)
	assert.Equal(t, []string{"Tras corte"}, subjects(sent), "una sola copia en Enviados")
}

// Lo que sale se ve como en el redactor: un Enter es un renglón y un
// párrafo vacío es una línea en blanco.
func TestSend_BodyParagraphsAreTight(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s", BodyHTML: "<p>uno</p><p></p><p>dos</p>",
	})
	require.NoError(t, err)
	html := envelopePart(t, smtp.Received()[0], "text/html")
	assert.Contains(t, html, `<p style="margin:0">uno</p><p style="margin:0"><br/></p><p style="margin:0">dos</p>`)
}

// Una imagen pegada en el redactor (data:) sale como parte cid, no embebida.
func TestSend_PastedImageTravelsAsCID(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	png := []byte("\x89PNG-fake")
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s",
		BodyHTML: `<p>mirá</p><img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(png) + `">`,
	})
	require.NoError(t, err)
	env := smtp.Received()[0]
	html := envelopePart(t, env, "text/html")
	m := regexp.MustCompile(`src="cid:([^"]+)"`).FindStringSubmatch(html)
	require.NotNil(t, m, html)
	assert.NotContains(t, html, "data:")
	assert.Contains(t, string(env.Data), "Content-Id: <"+m[1]+">")
	assert.Contains(t, string(env.Data), "multipart/related")
}

// GetMessage entrega el original con las imágenes cid ya como data:; al
// responder la cita las vuelve a mandar como cid.
func TestSend_ReplyQuotesOriginalInlineImageAsCID(t *testing.T) {
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG-fake"))
	body := "--b\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>x</p><img src=\"cid:logo@ej\">\r\n" +
		"--b\r\nContent-Type: image/png\r\nContent-ID: <logo@ej>\r\nContent-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: inline\r\n\r\n" + png + "\r\n--b--\r\n"
	c, _, smtp, acc, _, m := prepared(t, mailWith("rel", "multipart/related; boundary=b", body))
	d, err := c.PrepareCompose(context.Background(), m.ID, "reply")
	require.NoError(t, err)
	_, err = c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: d.To, Subject: d.Subject, BodyHTML: "<p>ok</p>",
		Reply: &model.ReplyContext{Mode: "reply", OriginalMessageID: m.ID},
	})
	require.NoError(t, err)
	env := smtp.Received()[0]
	html := envelopePart(t, env, "text/html")
	assert.Contains(t, html, `src="cid:`)
	assert.NotContains(t, html, "data:image")
	assert.Contains(t, string(env.Data), "multipart/related")
}

func TestSend_HeadingsAndTablesGetInlineStyles(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s",
		BodyHTML: "<h1>Título</h1><table><colgroup><col></colgroup><tr><td>a</td></tr></table>",
	})
	require.NoError(t, err)
	html := envelopePart(t, smtp.Received()[0], "text/html")
	assert.Contains(t, html, `<h1 style="margin:16px 0 8px;`)
	assert.Contains(t, html, `<table style="border-collapse:collapse;`)
	assert.Contains(t, html, `<td style="`)
	assert.NotContains(t, html, "colgroup")
}

func TestSend_ReplySetsThreadingAndAnswered(t *testing.T) {
	c, imap, smtp, acc, inboxID, m := prepared(t, threadMail())
	d, err := c.PrepareCompose(context.Background(), m.ID, "reply")
	require.NoError(t, err)
	res, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: d.To, Subject: d.Subject, BodyHTML: "<p>ok</p>",
		Reply: &model.ReplyContext{Mode: "reply", OriginalMessageID: m.ID},
	})
	require.NoError(t, err)
	assert.True(t, res.SentCopySaved)

	envs := smtp.Received()
	require.Len(t, envs, 1)
	r := parseEnvelope(t, envs[0])
	assert.Equal(t, "<orig@x>", r.Header.Get("In-Reply-To"))
	assert.Equal(t, "<r1@x> <orig@x>", r.Header.Get("References"))
	assert.Contains(t, string(envs[0].Data), "escribi=C3=B3:") // quoted-printable

	assert.Contains(t, serverFlags(t, c, imap, m.UID), goimap.FlagAnswered)
	assert.True(t, firstMessage(t, c, inboxID).Answered)
}

func TestSend_ForwardIncludesOriginalAttachments(t *testing.T) {
	c, _, smtp, acc, _, m := prepared(t, attachmentMail("factura.pdf"))
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "Fwd: adj", BodyHTML: "<p>mirá</p>",
		Reply: &model.ReplyContext{Mode: "forward", OriginalMessageID: m.ID},
	})
	require.NoError(t, err)
	envs := smtp.Received()
	require.Len(t, envs, 1)
	r := parseEnvelope(t, envs[0])
	found := false
	for {
		p, err := r.NextPart()
		if err != nil {
			break
		}
		if ah, ok := p.Header.(*gomail.AttachmentHeader); ok {
			name, _ := ah.Filename()
			assert.Equal(t, "factura.pdf", name)
			b, _ := io.ReadAll(p.Body)
			assert.Equal(t, pdfBytes, b)
			found = true
		}
	}
	assert.True(t, found, "no llegó el adjunto")
}

func TestSend_SMTPFailureSavesLocalDraft(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs(testenv.RejectedRecipient), Subject: "Fallido", BodyHTML: "<p>x</p>",
	})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "rejected", ue.Code)

	ds, err := c.ListLocalDrafts(context.Background())
	require.NoError(t, err)
	require.Len(t, ds, 1)
	assert.Equal(t, "Fallido", ds[0].Request.Subject)
	assert.Equal(t, ue.Msg, ds[0].Error)
	assert.Equal(t, model.DraftFailed, ds[0].Status)
	assert.Empty(t, sentMessages(t, c, acc.ID))
	assert.Empty(t, smtp.Received())

	require.NoError(t, c.DeleteLocalDraft(context.Background(), ds[0].ID))
	ds, err = c.ListLocalDrafts(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []model.LocalDraft{}, ds)
}

func TestSend_NoSentFolderWarns(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	imap.DeleteMailbox(t, "Sent")
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))

	res, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "Sin Enviados", BodyHTML: "<p>x</p>",
	})
	require.NoError(t, err)
	assert.False(t, res.SentCopySaved)
	assert.Equal(t, "El correo se envió, pero no pudimos guardar la copia en Enviados.", res.Warning)
	assert.Len(t, smtp.Received(), 1)
}

func TestSend_NoRecipients(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(context.Background(), model.SendRequest{AccountID: acc.ID, Subject: "x"})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "no_recipients", ue.Code)
	assert.True(t, strings.Contains(ue.Msg, "destinatario"))
}

func TestSend_ForwardOfExpungedOriginalSavesLocalDraft(t *testing.T) {
	c, imap, smtp, acc, _, m := prepared(t, attachmentMail("factura.pdf"))
	imap.Expunge(t, "INBOX", m.UID)
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "Reenvío perdido", BodyHTML: "<p>mirá</p>",
		Reply: &model.ReplyContext{Mode: "forward", OriginalMessageID: m.ID},
	})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	ds, err := c.ListLocalDrafts(context.Background())
	require.NoError(t, err)
	require.Len(t, ds, 1)
	assert.Equal(t, "Reenvío perdido", ds[0].Request.Subject)
	assert.Empty(t, smtp.Received())
}

func TestSend_ValidationErrorsDoNotSaveDraft(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "x",
		Reply: &model.ReplyContext{Mode: "raro", OriginalMessageID: 1},
	})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
	_, err = c.Send(context.Background(), model.SendRequest{AccountID: acc.ID, Subject: "x"})
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "no_recipients", ue.Code)
	ds, err := c.ListLocalDrafts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, ds)
}

func TestSend_PlainTextAlternativeQuotesOriginal(t *testing.T) {
	c, _, smtp, acc, _, m := prepared(t, threadMail())
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("a@x"), Subject: "Re: Hola", BodyHTML: "<p>ok</p>",
		Reply: &model.ReplyContext{Mode: "reply", OriginalMessageID: m.ID},
	})
	require.NoError(t, err)
	text := envelopePart(t, smtp.Received()[0], "text/plain")
	require.NotEmpty(t, text)
	assert.Contains(t, text, "escribió:")
	assert.Contains(t, text, "\n> cuerpo")
}

func TestSend_PlainTextOfTipTapListAndTable(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s",
		BodyHTML: `<ul><li><p>uno</p></li><li><p>dos</p></li></ul>` +
			`<table><tr><th><p>Nombre</p></th><th><p>Edad</p></th></tr><tr><td><p>Ana</p></td><td><p>30</p></td></tr></table>`,
	})
	require.NoError(t, err)
	text := envelopePart(t, smtp.Received()[0], "text/plain")
	assert.Contains(t, text, "- uno\r\n- dos")
	assert.Contains(t, text, "Nombre Edad\r\nAna 30")
}

func TestQuotedHTML_ZeroDateOmitsDate(t *testing.T) {
	d := model.MessageDetail{Text: "x"}
	d.From = model.Address{Name: "Bea", Email: "bea@ej.com"}
	q := quotedHTML("reply", d)
	assert.True(t, strings.HasPrefix(q, "<p>Bea &lt;bea@ej.com&gt; escribió:</p>"), q)
}

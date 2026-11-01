package mimex

import (
	"bytes"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/mail"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func baseOutgoing() Outgoing {
	return Outgoing{
		From:      model.Address{Name: "Fabricio", Email: "f@ej.com"},
		To:        []model.Address{{Name: "Ana", Email: "ana@ej.com"}},
		Subject:   "Hola",
		HTML:      "<p>Hola <b>mundo</b></p>",
		Text:      "Hola mundo",
		Date:      time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC),
		MessageID: "<id1@ej.com>",
	}
}

type parsedPart struct {
	ctype    string
	disp     string
	filename string
	body     []byte
}

func parse(t *testing.T, raw []byte) (*mail.Reader, []parsedPart) {
	t.Helper()
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	require.NoError(t, err)
	var parts []parsedPart
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		b, err := io.ReadAll(p.Body)
		require.NoError(t, err)
		pp := parsedPart{body: b}
		switch h := p.Header.(type) {
		case *mail.InlineHeader:
			pp.disp = "inline"
			pp.ctype, _, _ = h.ContentType()
		case *mail.AttachmentHeader:
			pp.disp = "attachment"
			pp.ctype, _, _ = h.ContentType()
			pp.filename, err = h.Filename()
			require.NoError(t, err)
		}
		parts = append(parts, pp)
	}
	return mr, parts
}

func topType(t *testing.T, raw []byte) string {
	t.Helper()
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	require.NoError(t, err)
	ct, _, err := mr.Header.ContentType()
	require.NoError(t, err)
	return ct
}

func TestBuild_AlternativeOnly(t *testing.T) {
	raw, err := Build(baseOutgoing())
	require.NoError(t, err)
	assert.Equal(t, "multipart/alternative", topType(t, raw))
	_, parts := parse(t, raw)
	require.Len(t, parts, 2)
	assert.Equal(t, "text/plain", parts[0].ctype)
	assert.Equal(t, "Hola mundo", strings.TrimSpace(string(parts[0].body)))
	assert.Equal(t, "text/html", parts[1].ctype)
	assert.Equal(t, "<p>Hola <b>mundo</b></p>", strings.TrimSpace(string(parts[1].body)))
	assert.Contains(t, string(raw), "quoted-printable")
}

func TestBuild_WithAttachments(t *testing.T) {
	o := baseOutgoing()
	data := []byte{0, 1, 2, 3, 255, 254, 'a', 'b'}
	o.Attachments = []OutAttachment{{Filename: "datos.bin", MIMEType: "application/octet-stream", Data: data}}
	raw, err := Build(o)
	require.NoError(t, err)
	assert.Equal(t, "multipart/mixed", topType(t, raw))
	assert.Contains(t, string(raw), "multipart/alternative")
	assert.Contains(t, string(raw), "base64")
	_, parts := parse(t, raw)
	// El lector aplana: texto, html y adjunto.
	require.Len(t, parts, 3)
	assert.Equal(t, "text/plain", parts[0].ctype)
	assert.Equal(t, "text/html", parts[1].ctype)
	assert.Equal(t, "attachment", parts[2].disp)
	assert.Equal(t, "datos.bin", parts[2].filename)
	assert.Equal(t, data, parts[2].body)
}

func TestBuild_NonASCIIHeaders(t *testing.T) {
	o := baseOutgoing()
	o.From = model.Address{Name: "Fabricio Peña", Email: "f@ej.com"}
	o.Subject = "Información de octubre"
	o.Attachments = []OutAttachment{{Filename: "presupuesto ñ.pdf", MIMEType: "application/pdf", Data: []byte("%PDF")}}
	raw, err := Build(o)
	require.NoError(t, err)

	headEnd := bytes.Index(raw, []byte("\r\n\r\n"))
	require.Greater(t, headEnd, 0)
	for _, b := range raw[:headEnd] {
		require.Less(t, b, byte(0x80), "encabezado con bytes no ASCII")
	}
	// Los encabezados de las partes también deben ser ASCII.
	for _, line := range strings.Split(string(raw), "\r\n") {
		if strings.HasPrefix(strings.ToLower(line), "content-disposition") || strings.HasPrefix(strings.ToLower(line), "content-type") {
			for _, r := range line {
				require.Less(t, r, rune(0x80), line)
			}
		}
	}

	mr, parts := parse(t, raw)
	subj, err := mr.Header.Subject()
	require.NoError(t, err)
	assert.Equal(t, "Información de octubre", subj)
	from, err := mr.Header.AddressList("From")
	require.NoError(t, err)
	require.Len(t, from, 1)
	assert.Equal(t, "Fabricio Peña", from[0].Name)
	assert.Equal(t, "f@ej.com", from[0].Address)
	assert.Equal(t, "presupuesto ñ.pdf", parts[len(parts)-1].filename)
}

func TestBuild_BccNotInHeaders(t *testing.T) {
	o := baseOutgoing()
	o.Bcc = []model.Address{{Name: "Oculto", Email: "oculto@ej.com"}}
	raw, err := Build(o)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(raw)), "bcc")
	assert.NotContains(t, string(raw), "oculto@ej.com")
	assert.Contains(t, Recipients(o), "oculto@ej.com")
}

func TestBuild_IncludeBccOnlyForDrafts(t *testing.T) {
	o := baseOutgoing()
	o.Bcc = []model.Address{{Name: "Oculto", Email: "oculto@ej.com"}}
	o.IncludeBcc = true
	raw, err := Build(o)
	require.NoError(t, err)
	assert.Contains(t, string(raw), "Bcc:")
	assert.Contains(t, string(raw), "oculto@ej.com")
	o.IncludeBcc = false
	raw, err = Build(o)
	require.NoError(t, err)
	assert.NotContains(t, strings.ToLower(string(raw)), "bcc")
}

func TestBuild_ReplyHeaders(t *testing.T) {
	o := baseOutgoing()
	o.InReplyTo = "<a@x>"
	o.References = []string{"<r@x>", "<a@x>"}
	raw, err := Build(o)
	require.NoError(t, err)
	s := string(raw)
	assert.Contains(t, s, "In-Reply-To: <a@x>\r\n")
	assert.Contains(t, s, "References: <r@x> <a@x>\r\n")
	assert.Contains(t, s, "Message-Id: <id1@ej.com>")
	assert.Contains(t, s, "Mime-Version: 1.0")
	assert.Contains(t, s, "Date: Fri, 30 Oct 2026 12:00:00 +0000")
}

func TestBuild_NoReplyHeadersWhenAbsent(t *testing.T) {
	raw, err := Build(baseOutgoing())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "In-Reply-To")
	assert.NotContains(t, string(raw), "References")
}

func TestRecipients_DedupLowercase(t *testing.T) {
	o := Outgoing{
		To:  []model.Address{{Email: "A@ej.com"}, {Email: "b@ej.com"}},
		Cc:  []model.Address{{Email: "a@ej.com"}},
		Bcc: []model.Address{{Email: "B@EJ.com"}, {Email: "c@ej.com"}},
	}
	assert.Equal(t, []string{"a@ej.com", "b@ej.com", "c@ej.com"}, Recipients(o))
}

func TestNewMessageID_Format(t *testing.T) {
	id := NewMessageID("f@ej.com")
	assert.Regexp(t, regexp.MustCompile(`^<[0-9a-f-]{36}@ej\.com>$`), id)
	assert.NotEqual(t, id, NewMessageID("f@ej.com"))
}

func TestBuild_AttachmentFilenameRFC2231(t *testing.T) {
	o := baseOutgoing()
	o.Attachments = []OutAttachment{
		{Filename: "presupuesto ñ.pdf", MIMEType: "application/pdf", Data: []byte("x")},
		{Filename: "simple.txt", MIMEType: "text/plain", Data: []byte("y")},
	}
	raw, err := Build(o)
	require.NoError(t, err)
	s := string(raw)
	assert.Contains(t, s, "filename*=utf-8''presupuesto%20%C3%B1.pdf")
	assert.NotContains(t, s, "=?utf-8?")
	assert.Contains(t, s, `filename=simple.txt`)
	_, parts := parse(t, raw)
	require.Len(t, parts, 4)
	assert.Equal(t, "presupuesto ñ.pdf", parts[2].filename)
	assert.Equal(t, "simple.txt", parts[3].filename)
}

func TestBuild_ZeroDateUsesNow(t *testing.T) {
	o := baseOutgoing()
	o.Date = time.Time{}
	before := time.Now().Add(-time.Minute)
	raw, err := Build(o)
	require.NoError(t, err)
	mr, err := mail.CreateReader(bytes.NewReader(raw))
	require.NoError(t, err)
	d, err := mr.Header.Date()
	require.NoError(t, err)
	assert.True(t, d.After(before) && d.Before(time.Now().Add(time.Minute)), d.String())
}

func TestBuild_RejectsHeaderInjection(t *testing.T) {
	o := baseOutgoing()
	o.InReplyTo = "<a@x>\r\nBcc: evil@x.com"
	_, err := Build(o)
	assert.Error(t, err)

	o = baseOutgoing()
	o.References = []string{"<r@x>", "<a@x>\nBcc: evil@x.com"}
	_, err = Build(o)
	assert.Error(t, err)
}

// mimeTree devuelve la estructura MIME real (sin aplanar) como
// "tipo[hijo,hijo]"; las partes con Content-ID agregan " cid=<valor>".
func mimeTree(t *testing.T, r io.Reader) string {
	t.Helper()
	e, err := message.Read(r)
	require.NoError(t, err)
	return describeEntity(t, e)
}

func describeEntity(t *testing.T, e *message.Entity) string {
	t.Helper()
	ct, _, err := e.Header.ContentType()
	require.NoError(t, err)
	if mr := e.MultipartReader(); mr != nil {
		var kids []string
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			require.NoError(t, err)
			kids = append(kids, describeEntity(t, p))
		}
		return ct + "[" + strings.Join(kids, ",") + "]"
	}
	if cid := e.Header.Get("Content-Id"); cid != "" {
		return ct + " cid=" + cid
	}
	return ct
}

func withInline() Outgoing {
	o := baseOutgoing()
	o.HTML = `<p>Hola</p><img src="cid:a@pc">`
	o.Inline = []OutInline{{CID: "a@pc", MIMEType: "image/png", Data: []byte("\x89PNG-fake")}}
	return o
}

func TestBuild_InlineImagesUseRelated(t *testing.T) {
	raw, err := Build(withInline())
	require.NoError(t, err)
	assert.Equal(t, "multipart/alternative", topType(t, raw))
	assert.Equal(t,
		"multipart/alternative[text/plain,multipart/related[text/html,image/png cid=<a@pc>]]",
		mimeTree(t, bytes.NewReader(raw)))
	s := string(raw)
	assert.Contains(t, s, "Content-Disposition: inline; filename=\"imagen-1.png\"")
	assert.Contains(t, s, "Content-Transfer-Encoding: base64")
}

func TestBuild_InlineImagesWithAttachments(t *testing.T) {
	o := withInline()
	o.Attachments = []OutAttachment{{Filename: "datos.bin", MIMEType: "application/octet-stream", Data: []byte{1, 2}}}
	raw, err := Build(o)
	require.NoError(t, err)
	assert.Equal(t,
		"multipart/mixed[multipart/alternative[text/plain,multipart/related[text/html,image/png cid=<a@pc>]],application/octet-stream]",
		mimeTree(t, bytes.NewReader(raw)))
}

func TestBuild_NoInlineUnchanged(t *testing.T) {
	raw, err := Build(baseOutgoing())
	require.NoError(t, err)
	assert.Equal(t, "multipart/alternative[text/plain,text/html]", mimeTree(t, bytes.NewReader(raw)))
	assert.NotContains(t, string(raw), "multipart/related")
}

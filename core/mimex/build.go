package mimex

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"strings"
	"time"

	"github.com/emersion/go-message"
	gomail "github.com/emersion/go-message/mail"
	"github.com/google/uuid"

	"postacontrol/core/model"
)

// OutAttachment es un adjunto de un mensaje saliente.
type OutAttachment struct {
	Filename, MIMEType string
	Data               []byte
}

// OutInline es una imagen en línea de un mensaje saliente; el HTML la
// referencia como src="cid:CID".
type OutInline struct {
	CID, MIMEType string
	Data          []byte
}

// Outgoing describe un mensaje saliente.
type Outgoing struct {
	From                model.Address
	To, Cc, Bcc         []model.Address
	Subject, HTML, Text string
	InReplyTo           string
	References          []string
	Attachments         []OutAttachment
	Inline              []OutInline
	Date                time.Time
	MessageID           string // con <>, lo genera el que llama
}

// NewMessageID genera un Message-ID "<uuid@dominio-del-from>".
func NewMessageID(fromEmail string) string {
	domain := "localhost"
	if i := strings.LastIndex(fromEmail, "@"); i >= 0 && i < len(fromEmail)-1 {
		domain = fromEmail[i+1:]
	}
	return "<" + uuid.NewString() + "@" + domain + ">"
}

// Recipients devuelve To+Cc+Bcc sin duplicados y en minúscula.
func Recipients(o Outgoing) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, list := range [][]model.Address{o.To, o.Cc, o.Bcc} {
		for _, a := range list {
			e := strings.ToLower(strings.TrimSpace(a.Email))
			if e == "" || seen[e] {
				continue
			}
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

func toMailAddrs(list []model.Address) []*gomail.Address {
	out := make([]*gomail.Address, 0, len(list))
	for _, a := range list {
		out = append(out, &gomail.Address{Name: a.Name, Address: a.Email})
	}
	return out
}

// Build arma el mensaje RFC 5322. Bcc nunca se escribe en los encabezados.
func Build(o Outgoing) ([]byte, error) {
	if o.From.Email == "" {
		return nil, fmt.Errorf("mimex: falta el remitente")
	}
	var h gomail.Header
	if strings.ContainsAny(o.InReplyTo, "\r\n") {
		return nil, fmt.Errorf("mimex: In-Reply-To inválido")
	}
	for _, r := range o.References {
		if strings.ContainsAny(r, "\r\n") {
			return nil, fmt.Errorf("mimex: References inválido")
		}
	}
	date := o.Date
	if date.IsZero() {
		date = time.Now()
	}
	h.SetDate(date)
	h.SetAddressList("From", toMailAddrs([]model.Address{o.From}))
	if len(o.To) > 0 {
		h.SetAddressList("To", toMailAddrs(o.To))
	}
	if len(o.Cc) > 0 {
		h.SetAddressList("Cc", toMailAddrs(o.Cc))
	}
	h.SetSubject(o.Subject)
	if o.MessageID != "" {
		h.SetMessageID(strings.TrimSuffix(strings.TrimPrefix(o.MessageID, "<"), ">"))
	}
	h.Set("MIME-Version", "1.0")
	if o.InReplyTo != "" {
		h.Set("In-Reply-To", o.InReplyTo)
	}
	if len(o.References) > 0 {
		h.Set("References", strings.Join(o.References, " "))
	}

	if len(o.Inline) > 0 {
		return buildWithInline(h, o)
	}

	var buf bytes.Buffer
	if len(o.Attachments) == 0 {
		iw, err := gomail.CreateInlineWriter(&buf, h)
		if err != nil {
			return nil, err
		}
		if err := writeTextParts(iw, o); err != nil {
			return nil, err
		}
		if err := iw.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}

	w, err := gomail.CreateWriter(&buf, h)
	if err != nil {
		return nil, err
	}
	iw, err := w.CreateInline()
	if err != nil {
		return nil, err
	}
	if err := writeTextParts(iw, o); err != nil {
		return nil, err
	}
	if err := iw.Close(); err != nil {
		return nil, err
	}
	for _, a := range o.Attachments {
		var ah gomail.AttachmentHeader
		mt := a.MIMEType
		if mt == "" {
			mt = "application/octet-stream"
		}
		ah.SetContentType(mt, nil)
		// Se arma a mano para que los nombres no ASCII salgan en RFC 2231
		// (filename*=utf-8''...); SetFilename usaría RFC 2047.
		ah.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename}))
		aw, err := w.CreateAttachment(ah)
		if err != nil {
			return nil, err
		}
		if _, err := aw.Write(a.Data); err != nil {
			return nil, err
		}
		if err := aw.Close(); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// buildWithInline arma el mensaje con imágenes en línea. go-message no sabe
// anidar un multipart/related dentro del alternative, así que la estructura se
// escribe con message.Writer:
//
//	[mixed >] alternative > text/plain, related > text/html, image/*...
func buildWithInline(h gomail.Header, o Outgoing) ([]byte, error) {
	var buf bytes.Buffer
	top := "multipart/alternative"
	if len(o.Attachments) > 0 {
		top = "multipart/mixed"
	}
	h.Set("Content-Type", top)
	mw, err := message.CreateWriter(&buf, h.Header)
	if err != nil {
		return nil, err
	}
	alt := mw
	if len(o.Attachments) > 0 {
		var ah message.Header
		ah.Set("Content-Type", "multipart/alternative")
		if alt, err = mw.CreatePart(ah); err != nil {
			return nil, err
		}
	}

	if err := writePart(alt, "text/plain; charset=utf-8", "quoted-printable", nil, []byte(o.Text)); err != nil {
		return nil, err
	}
	var rh message.Header
	rh.Set("Content-Type", "multipart/related")
	rel, err := alt.CreatePart(rh)
	if err != nil {
		return nil, err
	}
	if err := writePart(rel, "text/html; charset=utf-8", "quoted-printable", nil, []byte(o.HTML)); err != nil {
		return nil, err
	}
	for i, im := range o.Inline {
		extra := map[string]string{
			"Content-Id":          "<" + im.CID + ">",
			"Content-Disposition": fmt.Sprintf("inline; filename=\"imagen-%d.%s\"", i+1, imageExt(im.MIMEType)),
		}
		if err := writePart(rel, im.MIMEType, "base64", extra, im.Data); err != nil {
			return nil, err
		}
	}
	if err := rel.Close(); err != nil {
		return nil, err
	}
	if err := alt.Close(); err != nil {
		return nil, err
	}

	for _, a := range o.Attachments {
		mt := a.MIMEType
		if mt == "" {
			mt = "application/octet-stream"
		}
		extra := map[string]string{
			// Igual que en Build: RFC 2231 para nombres no ASCII.
			"Content-Disposition": mime.FormatMediaType("attachment", map[string]string{"filename": a.Filename}),
		}
		if err := writePart(mw, mt, "base64", extra, a.Data); err != nil {
			return nil, err
		}
	}
	if err := mw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writePart escribe una parte hoja dentro de parent.
func writePart(parent *message.Writer, ctype, cte string, extra map[string]string, data []byte) error {
	var ph message.Header
	ph.Set("Content-Type", ctype)
	ph.Set("Content-Transfer-Encoding", cte)
	if _, ok := extra["Content-Disposition"]; !ok {
		ph.Set("Content-Disposition", "inline")
	}
	for k, v := range extra {
		ph.Set(k, v)
	}
	pw, err := parent.CreatePart(ph)
	if err != nil {
		return err
	}
	if _, err := pw.Write(data); err != nil {
		return err
	}
	return pw.Close()
}

// imageExt devuelve la extensión del nombre de archivo para un image/*.
func imageExt(mimeType string) string {
	ext := strings.TrimPrefix(mimeType, "image/")
	if ext == "jpeg" {
		return "jpg"
	}
	return ext
}

// writeTextParts escribe text/plain y text/html (en ese orden, el más rico al
// final) en UTF-8 y quoted-printable.
func writeTextParts(iw *gomail.InlineWriter, o Outgoing) error {
	for _, p := range []struct{ ctype, body string }{
		{"text/plain", o.Text},
		{"text/html", o.HTML},
	} {
		var ih gomail.InlineHeader
		ih.SetContentType(p.ctype, map[string]string{"charset": "utf-8"})
		pw, err := iw.CreatePart(ih)
		if err != nil {
			return err
		}
		if _, err := io.WriteString(pw, p.body); err != nil {
			return err
		}
		if err := pw.Close(); err != nil {
			return err
		}
	}
	return nil
}

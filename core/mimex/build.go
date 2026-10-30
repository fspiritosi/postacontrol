package mimex

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"time"

	gomail "github.com/emersion/go-message/mail"
	"github.com/google/uuid"

	"postacontrol/core/model"
)

// OutAttachment es un adjunto de un mensaje saliente.
type OutAttachment struct {
	Filename, MIMEType string
	Data               []byte
}

// Outgoing describe un mensaje saliente.
type Outgoing struct {
	From                model.Address
	To, Cc, Bcc         []model.Address
	Subject, HTML, Text string
	InReplyTo           string
	References          []string
	Attachments         []OutAttachment
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
	h.SetDate(o.Date)
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
		ah.SetFilename(a.Filename) // go-message lo codifica (RFC 2047 si no es ASCII)
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

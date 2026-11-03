package app

import (
	"encoding/json"
	"errors"
	"html"
	netmail "net/mail"
	"net/url"
	"strings"

	"postacontrol/core/model"
)

// maxMailtoLen es el largo máximo que aceptamos para un enlace mailto:.
const maxMailtoLen = 8192

// mailtoFields es lo que se toma de un enlace mailto: (RFC 6068). Todo lo demás
// (attach, in-reply-to, otras cabeceras) se ignora a propósito.
type mailtoFields struct {
	To, Cc, Bcc   []model.Address
	Subject, Body string
}

// parseMailto interpreta un enlace mailto:. Devuelve error si el esquema no es
// mailto (sin distinguir mayúsculas) o si el enlace es demasiado largo.
func parseMailto(link string) (mailtoFields, error) {
	var f mailtoFields
	if len(link) > maxMailtoLen {
		return f, errors.New("El enlace es demasiado largo.")
	}
	u, err := url.Parse(link)
	if err != nil || !strings.EqualFold(u.Scheme, "mailto") {
		return f, errors.New("El enlace no es un mailto válido.")
	}
	q := u.Query()
	path := u.Opaque
	if path == "" {
		path = u.Path
	}
	if dec, err := url.PathUnescape(path); err == nil {
		path = dec
	}
	f.To = parseMailtoAddrs(append([]string{path}, q["to"]...))
	f.Cc = parseMailtoAddrs(q["cc"])
	f.Bcc = parseMailtoAddrs(q["bcc"])
	f.Subject = q.Get("subject")
	f.Body = q.Get("body")
	return f, nil
}

// parseMailtoAddrs separa por coma cada valor y descarta las direcciones inválidas.
func parseMailtoAddrs(values []string) []model.Address {
	out := []model.Address{}
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			a, err := netmail.ParseAddress(part)
			if err != nil {
				continue
			}
			out = append(out, model.Address{Name: a.Name, Email: a.Address})
		}
	}
	return out
}

// mailtoInit es el JSON de arranque del redactor: las claves son las de
// ComposerInit (frontend/src/features/compose/ComposerContext.tsx).
type mailtoInit struct {
	AccountID               int64           `json:"accountId"`
	To                      []model.Address `json:"to"`
	Cc                      []model.Address `json:"cc"`
	Bcc                     []model.Address `json:"bcc"`
	Subject                 string          `json:"subject"`
	BodyHTML                string          `json:"bodyHtml"`
	QuotedHTML              string          `json:"quotedHtml"`
	Reply                   *struct{}       `json:"reply"`
	Attachments             []struct{}      `json:"attachments"`
	OriginalAttachments     []struct{}      `json:"originalAttachments"`
	SkipOriginalAttachments []int64         `json:"skipOriginalAttachments"`
	SignatureID             int64           `json:"signatureId"`
}

// mailtoInitJSON arma el JSON con que arranca el redactor para un mailto:.
// El cuerpo es texto plano: se escapa y los saltos de línea pasan a <br>.
func mailtoInitJSON(f mailtoFields, acc model.Account) (string, error) {
	body := html.EscapeString(f.Body)
	body = strings.ReplaceAll(body, "\r\n", "<br>")
	body = strings.ReplaceAll(body, "\n", "<br>")
	init := mailtoInit{
		AccountID:               acc.ID,
		To:                      nonNilAddrs(f.To),
		Cc:                      nonNilAddrs(f.Cc),
		Bcc:                     nonNilAddrs(f.Bcc),
		Subject:                 f.Subject,
		BodyHTML:                body,
		Attachments:             []struct{}{},
		OriginalAttachments:     []struct{}{},
		SkipOriginalAttachments: []int64{},
		SignatureID:             acc.DefaultSignatureID,
	}
	b, err := json.Marshal(init)
	return string(b), err
}

func nonNilAddrs(a []model.Address) []model.Address {
	if a == nil {
		return []model.Address{}
	}
	return a
}

// pickMailtoAccount devuelve la cuenta con ese id o, si no existe, la primera.
// false si no hay cuentas.
func pickMailtoAccount(accs []model.Account, id int64) (model.Account, bool) {
	if len(accs) == 0 {
		return model.Account{}, false
	}
	for _, a := range accs {
		if a.ID == id {
			return a, true
		}
	}
	return accs[0], true
}

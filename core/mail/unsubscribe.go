package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	netmail "net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"

	"postacontrol/core/imapx"
	"postacontrol/core/mimex"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

// Desuscribirse de un newsletter con sus encabezados List-Unsubscribe
// (RFC 2369) y, si el remitente lo acepta, con un solo POST (RFC 8058).

const (
	unsubTimeout = 20 * time.Second

	msgUnsubNone   = "Este correo no dice cómo desuscribirse."
	msgUnsubFailed = "El remitente no confirmó la baja. Probá desde el enlace del correo."
	msgUnsubMail   = "No pudimos mandar el correo de baja."
)

var unsubURIRE = regexp.MustCompile(`<\s*([^<>]+?)\s*>`)

// parseListUnsubscribe lee el bloque de encabezados List-Unsubscribe y
// List-Unsubscribe-Post. Solo se ofrecen mailto:, https: y http:.
func parseListUnsubscribe(raw string) model.Unsubscribe {
	var u model.Unsubscribe
	msg, err := netmail.ReadMessage(strings.NewReader(raw + "\r\n"))
	if err != nil {
		return u
	}
	for _, m := range unsubURIRE.FindAllStringSubmatch(msg.Header.Get("List-Unsubscribe"), -1) {
		p, err := url.Parse(m[1])
		if err != nil {
			continue
		}
		switch strings.ToLower(p.Scheme) {
		case "mailto":
			if u.Mailto == "" {
				u.Mailto = m[1]
			}
		case "https":
			if u.URL == "" || !strings.HasPrefix(strings.ToLower(u.URL), "https:") {
				u.URL = m[1]
			}
		case "http":
			if u.URL == "" {
				u.URL = m[1]
			}
		}
	}
	post := strings.ToLower(msg.Header.Get("List-Unsubscribe-Post"))
	u.OneClick = strings.Contains(post, "list-unsubscribe=one-click") && strings.HasPrefix(strings.ToLower(u.URL), "https:")
	u.Available = u.URL != "" || u.Mailto != ""
	return u
}

// UnsubscribeInfo dice cómo desuscribirse del mensaje. La primera vez mira
// los encabezados en el servidor (sin marcarlo leído) y lo guarda.
func (c *Core) UnsubscribeInfo(ctx context.Context, messageID int64) (model.Unsubscribe, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	value, known, err := c.store.ListUnsub(ctx, messageID)
	if err != nil {
		return model.Unsubscribe{}, c.ctxOrUserError(ctx, err)
	}
	if known {
		var u model.Unsubscribe
		if json.Unmarshal([]byte(value), &u) == nil {
			return u, nil
		}
	}
	d, _, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return model.Unsubscribe{}, c.ctxOrUserError(ctx, err)
	}
	f, last, err := c.store.GetFolder(ctx, d.FolderID)
	if err != nil {
		return model.Unsubscribe{}, c.ctxOrUserError(ctx, err)
	}
	var raw []byte
	err = c.withConn(ctx, d.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, false); err != nil {
			return err
		}
		var err error
		raw, err = conn.FetchHeaderFields(d.UID, "List-Unsubscribe", "List-Unsubscribe-Post")
		return err
	})
	if err != nil {
		return model.Unsubscribe{}, err
	}
	u := parseListUnsubscribe(string(raw))
	if data, err := json.Marshal(u); err == nil {
		_ = c.store.SetListUnsub(ctx, messageID, string(data))
	}
	return u, nil
}

// Unsubscribe da de baja la dirección del remitente del mensaje: con un POST
// si acepta un clic, si no con el correo de baja, y si solo hay una página la
// devuelve para que la abra el usuario.
func (c *Core) Unsubscribe(ctx context.Context, messageID int64) (model.UnsubscribeResult, error) {
	u, err := c.UnsubscribeInfo(ctx, messageID)
	if err != nil {
		return model.UnsubscribeResult{}, err
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	switch {
	case !u.Available:
		return model.UnsubscribeResult{}, &UserError{Code: "invalid", Msg: msgUnsubNone}
	case u.OneClick:
		if err := c.oneClickUnsubscribe(ctx, u.URL); err != nil {
			return model.UnsubscribeResult{}, err
		}
		return model.UnsubscribeResult{Method: "post"}, nil
	case u.Mailto != "":
		if err := c.mailUnsubscribe(ctx, messageID, u.Mailto); err != nil {
			return model.UnsubscribeResult{}, err
		}
		return model.UnsubscribeResult{Method: "mail"}, nil
	default:
		return model.UnsubscribeResult{Method: "web", URL: u.URL}, nil
	}
}

// oneClickUnsubscribe hace el POST de RFC 8058. No sigue redirecciones: la
// respuesta del propio enlace alcanza (2xx o 3xx es baja aceptada).
func (c *Core) oneClickUnsubscribe(ctx context.Context, target string) error {
	client := &http.Client{Timeout: unsubTimeout}
	if c.cfg.HTTPClient != nil {
		cp := *c.cfg.HTTPClient
		client = &cp
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		return &UserError{Code: "invalid", Msg: msgUnsubFailed, Err: err}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		if cerr := c.ctxErr(ctx); cerr != nil {
			return cerr
		}
		return &UserError{Code: "unsubscribe", Msg: msgUnsubFailed, Err: err}
	}
	_ = resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &UserError{Code: "unsubscribe", Msg: msgUnsubFailed, Err: errors.New(resp.Status)}
	}
	return nil
}

// mailUnsubscribe manda el correo de baja de un mailto: desde la cuenta que
// recibió el mensaje. Va directo por SMTP: no pasa por la bandeja de salida ni
// deja copia en Enviados.
func (c *Core) mailUnsubscribe(ctx context.Context, messageID int64, mailto string) error {
	p, err := url.Parse(mailto)
	if err != nil {
		return &UserError{Code: "invalid", Msg: msgUnsubMail, Err: err}
	}
	addrsText := p.Opaque
	if s, err := url.PathUnescape(addrsText); err == nil {
		addrsText = s
	}
	var to []model.Address
	for _, a := range strings.Split(addrsText, ",") {
		if pa, err := netmail.ParseAddress(strings.TrimSpace(a)); err == nil {
			to = append(to, model.Address{Email: pa.Address})
		}
	}
	if len(to) == 0 {
		return &UserError{Code: "invalid", Msg: msgUnsubMail}
	}
	q := p.Query()
	subject, body := q.Get("subject"), q.Get("body")
	if subject == "" {
		subject = "unsubscribe"
	}
	if body == "" {
		body = "unsubscribe"
	}
	d, _, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	acc, err := c.store.GetAccount(ctx, d.AccountID)
	if errors.Is(err, store.ErrNotFound) {
		return &UserError{Code: "invalid", Msg: msgUnsubMail, Err: err}
	}
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	out := mimex.Outgoing{
		From:      model.Address{Name: acc.DisplayName, Email: acc.Email},
		To:        to,
		Subject:   subject,
		Text:      body,
		Date:      time.Now(),
		MessageID: mimex.NewMessageID(acc.Email),
	}
	raw, err := mimex.Build(out)
	if err != nil {
		return &UserError{Code: "invalid", Msg: msgUnsubMail, Err: err}
	}
	return c.smtpSend(ctx, acc, mimex.Recipients(out), raw)
}

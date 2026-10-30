package mail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/htmlx"
	"postacontrol/core/imapx"
	"postacontrol/core/mimex"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/smtpx"
)

const (
	modeReply    model.ComposeMode = "reply"
	modeReplyAll model.ComposeMode = "replyAll"
	modeForward  model.ComposeMode = "forward"

	msgSentCopyWarning = "El correo se envió, pero no pudimos guardar la copia en Enviados."
	bodyWrapperOpen    = `<div style="font-family:Arial,sans-serif;font-size:14px">`
)

var monthsES = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio",
	"agosto", "septiembre", "octubre", "noviembre", "diciembre"}

// formatDateES formatea "2 de enero de 2026, 15:04" en hora local.
func formatDateES(t time.Time) string {
	t = t.Local()
	return fmt.Sprintf("%d de %s de %d, %02d:%02d", t.Day(), monthsES[t.Month()-1], t.Year(), t.Hour(), t.Minute())
}

func addrHTML(a model.Address) string {
	if a.Name == "" {
		return html.EscapeString(a.Email)
	}
	return html.EscapeString(a.Name) + " &lt;" + html.EscapeString(a.Email) + "&gt;"
}

func addrsHTML(list []model.Address) string {
	parts := make([]string, 0, len(list))
	for _, a := range list {
		parts = append(parts, addrHTML(a))
	}
	return strings.Join(parts, ", ")
}

// hasPrefixFold dice si subject empieza con alguno de los prefijos ("re:",
// "fwd:") sin distinguir mayúsculas.
func hasPrefixFold(subject string, prefixes ...string) bool {
	s := strings.ToLower(strings.TrimSpace(subject))
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func subjectFor(mode model.ComposeMode, subject string) string {
	switch mode {
	case modeReply, modeReplyAll:
		if !hasPrefixFold(subject, "re:") {
			return "Re: " + subject
		}
	case modeForward:
		if !hasPrefixFold(subject, "fwd:", "fw:") {
			return "Fwd: " + subject
		}
	}
	return subject
}

// originalBodyHTML es el cuerpo del original listo para citar.
func originalBodyHTML(d model.MessageDetail) string {
	if d.HTML != "" {
		return htmlx.RestoreRemoteImages(d.HTML)
	}
	return "<pre>" + html.EscapeString(d.Text) + "</pre>"
}

// quoteLead es el comienzo de la cita ("El {fecha}, {Nombre} <email>"); sin
// fecha conocida queda solo el remitente.
func quoteLead(d model.MessageDetail) string {
	if d.Date.IsZero() {
		return addrHTML(d.From)
	}
	return "El " + formatDateES(d.Date) + ", " + addrHTML(d.From)
}

func dateLine(d model.MessageDetail) string {
	if d.Date.IsZero() {
		return ""
	}
	return "<br>Fecha: " + formatDateES(d.Date)
}

func quotedHTML(mode model.ComposeMode, d model.MessageDetail) string {
	switch mode {
	case modeReply, modeReplyAll:
		return "<p>" + quoteLead(d) + " escribió:</p>" +
			`<blockquote type="cite" style="margin:0 0 0 .8ex;border-left:2px solid #ccc;padding-left:1ex">` +
			originalBodyHTML(d) + "</blockquote>"
	case modeForward:
		return "<p>---------- Mensaje reenviado ----------<br>De: " + addrHTML(d.From) +
			dateLine(d) +
			"<br>Asunto: " + html.EscapeString(d.Subject) +
			"<br>Para: " + addrsHTML(d.To) + "</p>" + originalBodyHTML(d)
	}
	return ""
}

// PrepareCompose arma los destinatarios, el asunto y la cita para responder o
// reenviar el mensaje messageID.
func (c *Core) PrepareCompose(ctx context.Context, messageID int64, mode model.ComposeMode) (model.ComposeDraft, error) {
	d, err := c.GetMessage(ctx, messageID)
	if err != nil {
		return model.ComposeDraft{}, err
	}
	acc, err := c.store.GetAccount(ctx, d.AccountID)
	if err != nil {
		return model.ComposeDraft{}, c.ctxOrUserError(ctx, err)
	}
	out := model.ComposeDraft{
		AccountID:  d.AccountID,
		To:         []model.Address{},
		Cc:         []model.Address{},
		Subject:    subjectFor(mode, d.Subject),
		QuotedHTML: quotedHTML(mode, d),
		Reply:      model.ReplyContext{Mode: mode, OriginalMessageID: messageID},
	}
	if mode == modeReply || mode == modeReplyAll {
		if len(d.ReplyTo) > 0 {
			out.To = append(out.To, d.ReplyTo...)
		} else if d.From.Email != "" {
			out.To = append(out.To, d.From)
		}
	}
	if mode == modeReplyAll {
		seen := map[string]bool{strings.ToLower(acc.Email): true}
		for _, a := range out.To {
			seen[strings.ToLower(a.Email)] = true
		}
		for _, a := range append(append([]model.Address{}, d.To...), d.Cc...) {
			k := strings.ToLower(a.Email)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			out.Cc = append(out.Cc, a)
		}
	}
	return out, nil
}

// Send arma el mensaje, lo envía por SMTP y, si funciona, guarda la copia en
// Enviados y marca \Answered en el original. Si el SMTP falla, guarda un
// borrador local y devuelve un UserError.
func (c *Core) Send(ctx context.Context, req model.SendRequest) (model.SendResult, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return model.SendResult{}, err
	}
	acc, err := c.store.GetAccount(ctx, req.AccountID)
	if err != nil {
		return model.SendResult{}, c.ctxOrUserError(ctx, err)
	}

	replying, forwarding := false, false
	if req.Reply != nil {
		switch req.Reply.Mode {
		case modeReply, modeReplyAll:
			replying = true
		case modeForward:
			forwarding = true
		case "", "new":
		default:
			return model.SendResult{}, &UserError{Code: "invalid", Msg: "No pudimos preparar la respuesta."}
		}
	}
	if len(mimex.Recipients(mimex.Outgoing{To: req.To, Cc: req.Cc, Bcc: req.Bcc})) == 0 {
		return model.SendResult{}, &UserError{Code: "no_recipients", Msg: "Agregá al menos un destinatario."}
	}

	// Desde acá, cualquier falla guarda un borrador local para no perder lo escrito.
	var orig *model.MessageDetail
	if replying || forwarding {
		d, err := c.GetMessage(ctx, req.Reply.OriginalMessageID)
		if err != nil {
			return model.SendResult{}, c.failWithDraft(ctx, req, err)
		}
		orig = &d
	}

	body := bodyWrapperOpen + req.BodyHTML + "</div>"
	if orig != nil {
		body += quotedHTML(req.Reply.Mode, *orig)
	}
	out := mimex.Outgoing{
		From:      model.Address{Name: acc.DisplayName, Email: acc.Email},
		To:        req.To,
		Cc:        req.Cc,
		Bcc:       req.Bcc,
		Subject:   req.Subject,
		HTML:      body,
		Text:      htmlx.ToText(body),
		Date:      time.Now(),
		MessageID: mimex.NewMessageID(acc.Email),
	}
	if replying && orig.MessageID != "" && !strings.ContainsAny(orig.MessageID, "\r\n") {
		out.InReplyTo = orig.MessageID
		for _, r := range orig.References {
			if r != "" && !strings.ContainsAny(r, "\r\n") {
				out.References = append(out.References, r)
			}
		}
		out.References = append(out.References, orig.MessageID)
	}
	if forwarding {
		for i := range orig.Attachments {
			att, data, err := c.attachmentBytes(ctx, orig.ID, i)
			if err != nil {
				return model.SendResult{}, c.failWithDraft(ctx, req, err)
			}
			name := att.Filename
			if name == "" {
				name = fmt.Sprintf("adjunto-%d", i+1)
			}
			out.Attachments = append(out.Attachments, mimex.OutAttachment{Filename: name, MIMEType: att.MIMEType, Data: data})
		}
	}
	rcpts := mimex.Recipients(out)
	raw, err := mimex.Build(out)
	if err != nil {
		return model.SendResult{}, c.failWithDraft(ctx, req,
			&UserError{Code: "invalid", Msg: "No pudimos armar el correo: " + err.Error(), Err: err})
	}
	if err := c.smtpSend(ctx, acc, rcpts, raw); err != nil {
		return model.SendResult{}, c.failWithDraft(ctx, req, err)
	}

	// El correo ya salió: de acá en adelante nada convierte el envío en error.
	res := model.SendResult{}
	if acc.SaveSentCopy { // si el usuario no quiere copia, no es una falla
		if sentID, ok := c.appendSent(ctx, acc, raw); ok {
			res.SentCopySaved = true
			c.emit(EventFolderUpdated, map[string]any{"folderId": sentID})
		} else {
			res.Warning = msgSentCopyWarning
		}
	}
	if replying {
		c.markAnswered(ctx, *orig)
	}
	return res, nil
}

// failWithDraft guarda req como borrador local y devuelve err como UserError.
// Si el contexto se canceló devuelve ese error y no guarda nada. Si no se
// puede guardar el borrador, igual devuelve el error original del envío.
func (c *Core) failWithDraft(ctx context.Context, req model.SendRequest, err error) error {
	if cerr := c.ctxErr(ctx); cerr != nil {
		return cerr
	}
	var ue *UserError
	if !errors.As(asUserError(err), &ue) {
		ue = &UserError{Code: "send", Msg: "No pudimos enviar el correo.", Err: err}
	}
	_, _ = c.store.SaveLocalDraft(ctx, req, ue.Msg) // si falla, el error útil es el del envío
	return ue
}

func (c *Core) smtpSend(ctx context.Context, acc model.Account, rcpts []string, raw []byte) error {
	password, err := c.cfg.Secrets.Get(acc.Email)
	if errors.Is(err, secrets.ErrNotFound) {
		return &UserError{Code: "auth", Msg: "No encontramos la contraseña de la cuenta en el llavero del sistema.", Err: err}
	}
	if err != nil {
		return err
	}
	user := acc.Username
	if user == "" {
		user = acc.Email
	}
	return smtpx.Send(ctx, acc.SMTP, user, password, acc.Email, rcpts, raw, c.cfg.TLS)
}

// appendSent sube la copia a la carpeta Enviados (sin reintentos: APPEND no es
// idempotente). Devuelve el ID de la carpeta y si salió bien.
func (c *Core) appendSent(ctx context.Context, acc model.Account, raw []byte) (int64, bool) {
	folders, err := c.store.ListFolders(ctx, acc.ID)
	if err != nil {
		return 0, false
	}
	var sent *model.Folder
	for i := range folders {
		if folders[i].Role == "sent" {
			sent = &folders[i]
			break
		}
	}
	if sent == nil {
		return 0, false
	}
	// La conexión del pool puede haber muerto sin que lo sepamos (el servidor
	// corta las sesiones inactivas). NOOP es idempotente: withConn la valida
	// o la reemplaza, y recién entonces va el APPEND sin reintento.
	if err := c.withConn(ctx, acc.ID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Noop()
	}); err != nil {
		return 0, false
	}
	err = c.withConnNoRetry(ctx, acc.ID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Append(sent.Name, raw, goimap.FlagSeen)
	})
	if err != nil {
		return 0, false
	}
	return sent.ID, true
}

// markAnswered marca \Answered en el original, en el servidor y en la caché.
// Es de mejor esfuerzo: un fallo no afecta al envío.
func (c *Core) markAnswered(ctx context.Context, d model.MessageDetail) {
	f, last, err := c.store.GetFolder(ctx, d.FolderID)
	if err != nil {
		return
	}
	err = c.withConnNoRetry(ctx, d.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, true); err != nil {
			return err
		}
		return conn.AddFlags(d.UID, goimap.FlagAnswered)
	})
	if err != nil {
		return
	}
	if err := c.store.SetFlags(ctx, d.FolderID, d.UID, d.Seen, d.Flagged, true); err == nil {
		c.emit(EventFolderUpdated, map[string]any{"folderId": d.FolderID})
	}
}

// ListLocalDrafts devuelve los envíos que fallaron y quedaron guardados.
func (c *Core) ListLocalDrafts(ctx context.Context) ([]model.LocalDraft, error) {
	ds, err := c.store.ListLocalDrafts(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return ds, nil
}

// DeleteLocalDraft descarta un borrador local.
func (c *Core) DeleteLocalDraft(ctx context.Context, id int64) error {
	if err := c.store.DeleteLocalDraft(ctx, id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	return nil
}

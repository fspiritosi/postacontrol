package mail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"slices"
	"strings"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/google/uuid"

	"postacontrol/core/htmlx"
	"postacontrol/core/imapx"
	"postacontrol/core/mimex"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/smtpx"
	"postacontrol/core/store"
)

const (
	modeReply    model.ComposeMode = "reply"
	modeReplyAll model.ComposeMode = "replyAll"
	modeForward  model.ComposeMode = "forward"

	msgSentCopyWarning = "El correo se envió, pero no pudimos guardar la copia en Enviados."
	msgSendInterrupted = "El envío se interrumpió. Revisá Enviados antes de reintentar: puede que el correo haya salido."
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

// originalBodyHTML es el cuerpo del original listo para citar. Es el HTML
// saneado, con las imágenes remotas bloqueadas (data-remote-src): así se
// muestra en el redactor; Send las restaura recién en el correo que sale.
func originalBodyHTML(d model.MessageDetail) string {
	if d.HTML != "" {
		return d.HTML
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

		OriginalAttachments: []model.Attachment{},
	}
	if mode == modeForward {
		out.OriginalAttachments = append(out.OriginalAttachments, d.Attachments...)
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

// Send envía req y espera el resultado. Como QueueSend, el correo pasa por la
// bandeja de salida: si el envío falla queda ahí con el motivo y se devuelve un
// UserError.
func (c *Core) Send(ctx context.Context, req model.SendRequest) (model.SendResult, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	id, acc, err := c.enqueue(ctx, req)
	if err != nil {
		return model.SendResult{}, err
	}
	return c.deliver(ctx, id, acc, req)
}

// QueueSend valida req, lo guarda en la bandeja de salida y lo envía en
// segundo plano. Devuelve enseguida el id del correo en la bandeja; el
// resultado llega por EventSendFinished. Con req.DraftID reintenta ese correo.
func (c *Core) QueueSend(ctx context.Context, req model.SendRequest) (int64, error) {
	qctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	id, acc, err := c.enqueue(qctx, req)
	if err != nil {
		return 0, err
	}
	if !c.spawn(func(life context.Context) { _, _ = c.deliver(life, id, acc, req) }) {
		// El núcleo se está cerrando: el correo queda en la bandeja y al volver
		// figura como interrumpido.
		return 0, context.Canceled
	}
	return id, nil
}

// enqueue valida req y lo pone en la bandeja de salida como "enviando". Los
// errores de validación no guardan nada: el redactor sigue abierto.
func (c *Core) enqueue(ctx context.Context, req model.SendRequest) (int64, model.Account, error) {
	if err := c.ctxErr(ctx); err != nil {
		return 0, model.Account{}, err
	}
	if req.Reply != nil {
		switch req.Reply.Mode {
		case modeReply, modeReplyAll, modeForward, "", "new":
		default:
			return 0, model.Account{}, &UserError{Code: "invalid", Msg: "No pudimos preparar la respuesta."}
		}
	}
	if len(mimex.Recipients(mimex.Outgoing{To: req.To, Cc: req.Cc, Bcc: req.Bcc})) == 0 {
		return 0, model.Account{}, &UserError{Code: "no_recipients", Msg: "Agregá al menos un destinatario."}
	}
	acc, err := c.store.GetAccount(ctx, req.AccountID)
	if err != nil {
		return 0, model.Account{}, c.ctxOrUserError(ctx, err)
	}
	if err := c.checkSize(ctx, req); err != nil {
		return 0, model.Account{}, err
	}
	id, err := c.store.QueueLocalDraft(ctx, req.DraftID, req)
	if errors.Is(err, store.ErrBusy) {
		return 0, model.Account{}, &UserError{Code: "busy", Msg: "Este correo ya se está enviando."}
	}
	if err != nil {
		return 0, model.Account{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventOutboxUpdated, nil)
	return id, acc, nil
}

// checkSize verifica que los adjuntos preparados existan y que el correo (esos
// adjuntos, los del original que se reenvían y las imágenes pegadas en el
// cuerpo) no pase de MaxMessageBytes.
func (c *Core) checkSize(ctx context.Context, req model.SendRequest) error {
	info, err := c.store.StagedInfo(ctx, req.Attachments)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	found := map[int64]bool{}
	var total int64
	for _, a := range info {
		found[a.ID] = true
		total += a.Size
	}
	for _, id := range req.Attachments {
		if !found[id] {
			return &UserError{Code: "attach", Msg: "Uno de los adjuntos ya no está disponible. Volvé a adjuntarlo."}
		}
	}
	body := req.BodyHTML
	if req.Reply != nil {
		// Solo la caché: dimensionar no debe ir al servidor ni marcar \Seen. Si
		// el original no está cacheado o ya no existe, el fallo (si lo hay)
		// sale al enviar y el correo queda en la bandeja de salida.
		if d, cached, err := c.store.GetMessage(ctx, req.Reply.OriginalMessageID); err == nil && cached {
			if req.Reply.Mode == modeForward {
				for i, a := range d.Attachments {
					if !slices.Contains(req.SkipOriginalAttachments, i) {
						total += a.Size
					}
				}
			}
			// deliver también manda como partes las imágenes data: de la cita.
			body += quotedHTML(req.Reply.Mode, d)
		}
	}
	_, imgs := htmlx.ExtractInlineImages(body, func() string { return "x" })
	for _, im := range imgs {
		total += int64(len(im.Data))
	}
	if total > MaxMessageBytes {
		return &UserError{Code: "too_large", Msg: "El correo pesa " + formatSize(total) + " y el máximo es " + formatSize(MaxMessageBytes) + ". Sacá algún adjunto."}
	}
	return nil
}

// deliver arma el mensaje del correo id de la bandeja de salida y lo envía por
// SMTP. Si sale, lo quita de la bandeja, guarda la copia en Enviados y marca
// \Answered en el original; si no, lo deja en la bandeja con el motivo.
// Siempre termina emitiendo EventSendFinished.
func (c *Core) deliver(ctx context.Context, id int64, acc model.Account, req model.SendRequest) (model.SendResult, error) {
	replying, forwarding := false, false
	if req.Reply != nil {
		replying = req.Reply.Mode == modeReply || req.Reply.Mode == modeReplyAll
		forwarding = req.Reply.Mode == modeForward
	}

	var orig *model.MessageDetail
	if replying || forwarding {
		d, err := c.GetMessage(ctx, req.Reply.OriginalMessageID)
		if err != nil {
			return model.SendResult{}, c.failSend(ctx, id, err)
		}
		orig = &d
	}

	body := bodyWrapperOpen + htmlx.EmailHTML(req.BodyHTML) + "</div>"
	if orig != nil {
		// En la vista previa la cita tiene las imágenes remotas bloqueadas; el
		// destinatario las recibe como en el original.
		body += htmlx.RestoreRemoteImages(quotedHTML(req.Reply.Mode, *orig))
	}
	// El texto plano se calcula antes de sacar las imágenes; después, las
	// pegadas y las de la cita (que GetMessage entregó como data:) viajan como cid.
	text := htmlx.ToText(body)
	body, imgs := htmlx.ExtractInlineImages(body, func() string { return uuid.NewString() + "@postacontrol" })
	out := mimex.Outgoing{
		From:      model.Address{Name: acc.DisplayName, Email: acc.Email},
		To:        req.To,
		Cc:        req.Cc,
		Bcc:       req.Bcc,
		Subject:   req.Subject,
		HTML:      body,
		Text:      text,
		Date:      time.Now(),
		MessageID: mimex.NewMessageID(acc.Email),
	}
	for _, im := range imgs {
		out.Inline = append(out.Inline, mimex.OutInline{CID: im.CID, MIMEType: im.MIMEType, Data: im.Data})
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
			if slices.Contains(req.SkipOriginalAttachments, i) {
				continue
			}
			att, data, err := c.attachmentBytes(ctx, orig.ID, i)
			if err != nil {
				return model.SendResult{}, c.failSend(ctx, id, err)
			}
			name := att.Filename
			if name == "" {
				name = fmt.Sprintf("adjunto-%d", i+1)
			}
			out.Attachments = append(out.Attachments, mimex.OutAttachment{Filename: name, MIMEType: att.MIMEType, Data: data})
		}
	}
	staged, err := c.store.StagedAttachments(ctx, req.Attachments)
	if errors.Is(err, store.ErrNotFound) {
		return model.SendResult{}, c.failSend(ctx, id,
			&UserError{Code: "attach", Msg: "Uno de los adjuntos ya no está disponible. Volvé a adjuntarlo.", Err: err})
	}
	if err != nil {
		return model.SendResult{}, c.failSend(ctx, id, err)
	}
	for _, f := range staged {
		out.Attachments = append(out.Attachments, mimex.OutAttachment{Filename: f.Filename, MIMEType: f.MIMEType, Data: f.Data})
	}
	rcpts := mimex.Recipients(out)
	raw, err := mimex.Build(out)
	if err != nil {
		return model.SendResult{}, c.failSend(ctx, id,
			&UserError{Code: "invalid", Msg: "No pudimos armar el correo: " + err.Error(), Err: err})
	}
	if err := c.smtpSend(ctx, acc, rcpts, raw); err != nil {
		return model.SendResult{}, c.failSend(ctx, id, err)
	}

	// El correo ya salió: de acá en adelante nada convierte el envío en error.
	// Se quita de la bandeja aunque se esté cerrando la app.
	_ = c.store.DeleteLocalDraft(context.WithoutCancel(ctx), id)
	_ = c.store.DeleteStagedAttachments(context.WithoutCancel(ctx), req.Attachments)
	c.emit(EventOutboxUpdated, nil)
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
	c.emit(EventSendFinished, map[string]any{"draftId": id, "ok": true, "message": "", "warning": res.Warning})
	return res, nil
}

// failSend deja el correo id en la bandeja de salida con el motivo, lo avisa y
// devuelve err como UserError (o el error del contexto si se canceló).
func (c *Core) failSend(ctx context.Context, id int64, err error) error {
	var ue *UserError
	if !errors.As(asUserError(err), &ue) {
		ue = &UserError{Code: "send", Msg: "No pudimos enviar el correo.", Err: err}
	}
	msg := ue.Msg
	cerr := c.ctxErr(ctx)
	if cerr != nil {
		msg = msgSendInterrupted
	}
	// Sin cancelación: aunque la app se esté cerrando, el motivo queda guardado.
	_ = c.store.FailLocalDraft(context.WithoutCancel(ctx), id, msg)
	c.emit(EventOutboxUpdated, nil)
	c.emit(EventSendFinished, map[string]any{"draftId": id, "ok": false, "message": msg, "warning": ""})
	if cerr != nil {
		return cerr
	}
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

// ListLocalDrafts devuelve la bandeja de salida: los correos en camino y los
// que no se pudieron enviar.
func (c *Core) ListLocalDrafts(ctx context.Context) ([]model.LocalDraft, error) {
	ds, err := c.store.ListLocalDrafts(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return ds, nil
}

// DeleteLocalDraft descarta un borrador local.
func (c *Core) DeleteLocalDraft(ctx context.Context, id int64) error {
	// Los adjuntos preparados del correo se van con él.
	if ds, err := c.store.ListLocalDrafts(ctx); err == nil {
		for _, d := range ds {
			if d.ID == id {
				_ = c.store.DeleteStagedAttachments(ctx, d.Request.Attachments)
			}
		}
	}
	if err := c.store.DeleteLocalDraft(ctx, id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	return nil
}

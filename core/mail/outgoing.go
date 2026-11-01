package mail

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"postacontrol/core/htmlx"
	"postacontrol/core/mimex"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const signatureWrapperOpen = `<div style="margin-top:16px">`

// signatureFor devuelve el HTML de la firma id listo para pegar en el correo
// (envuelto y con las imágenes remotas restauradas). Sin firma, o si ya no
// existe, devuelve ok=false: el correo sale sin ella.
func (c *Core) signatureFor(ctx context.Context, id int64) (string, bool) {
	if id == 0 {
		return "", false
	}
	sig, err := c.store.GetSignature(ctx, id)
	if err != nil || strings.TrimSpace(sig.HTML) == "" {
		return "", false
	}
	return signatureWrapperOpen + htmlx.EmailHTML(htmlx.RestoreRemoteImages(sig.HTML)) + "</div>", true
}

// buildOutgoing arma el mensaje de req: el original (si responde o reenvía),
// cuerpo, firma, cita, imágenes, adjuntos y los encabezados de hilo. Con
// forDraft el mensaje es un borrador: lleva Bcc y el Message-ID del borrador.
func (c *Core) buildOutgoing(ctx context.Context, acc model.Account, req model.SendRequest, forDraft bool) (mimex.Outgoing, error) {
	replying, forwarding := false, false
	if req.Reply != nil {
		replying = req.Reply.Mode == modeReply || req.Reply.Mode == modeReplyAll
		forwarding = req.Reply.Mode == modeForward
	}

	// Si el original ya no está (se movió o se borró), el correo sale igual:
	// sin la cita ni los adjuntos del original, y con los encabezados de hilo
	// que guardó PrepareCompose.
	var orig *model.MessageDetail
	if replying || forwarding {
		d, err := c.GetMessage(ctx, req.Reply.OriginalMessageID)
		switch {
		case err == nil:
			orig = &d
		case !originalGone(err):
			return mimex.Outgoing{}, err
		}
	}

	body := bodyWrapperOpen + htmlx.EmailHTML(req.BodyHTML) + "</div>"
	if sig, ok := c.signatureFor(ctx, req.SignatureID); ok {
		body += sig
	}
	if orig != nil {
		// En la vista previa la cita tiene las imágenes remotas bloqueadas; el
		// destinatario las recibe como en el original.
		body += htmlx.RestoreRemoteImages(quotedHTML(req.Reply.Mode, *orig))
	}
	// El texto plano se calcula antes de sacar las imágenes; después, las
	// pegadas, las de la firma y las de la cita (que GetMessage entregó como
	// data:) viajan como cid, una sola vez cada imagen.
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
	if forDraft {
		out.IncludeBcc = true
		if req.DraftMessageID != "" {
			out.MessageID = req.DraftMessageID
		}
	}
	for _, im := range imgs {
		out.Inline = append(out.Inline, mimex.OutInline{CID: im.CID, MIMEType: im.MIMEType, Data: im.Data})
	}
	if replying {
		inReplyTo, refs := req.Reply.InReplyTo, req.Reply.References
		if orig != nil {
			inReplyTo, refs = threadHeaders(*orig)
		}
		if validHeaderID(inReplyTo) {
			out.InReplyTo = inReplyTo
			for _, r := range refs {
				if validHeaderID(r) {
					out.References = append(out.References, r)
				}
			}
		}
	}
	if forwarding && orig != nil {
		for i := range orig.Attachments {
			if slices.Contains(req.SkipOriginalAttachments, i) {
				continue
			}
			att, data, err := c.attachmentBytes(ctx, orig.ID, i)
			if err != nil {
				return mimex.Outgoing{}, err
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
		return mimex.Outgoing{}, &UserError{Code: "attach", Msg: "Uno de los adjuntos ya no está disponible. Volvé a adjuntarlo.", Err: err}
	}
	if err != nil {
		return mimex.Outgoing{}, err
	}
	for _, f := range staged {
		out.Attachments = append(out.Attachments, mimex.OutAttachment{Filename: f.Filename, MIMEType: f.MIMEType, Data: f.Data})
	}
	return out, nil
}

// originalGone dice si err es que el original de una respuesta o un reenvío
// ya no está en la caché: se movió o se borró, y su id no vuelve a servir. Si
// sigue en la caché pero el servidor ya no lo tiene, el error se mantiene
// hasta que la sincronización lo saque.
func originalGone(err error) bool {
	return errors.Is(err, store.ErrNotFound)
}

// threadHeaders son el In-Reply-To y el References de una respuesta a orig;
// vacíos si orig no tiene Message-ID.
func threadHeaders(orig model.MessageDetail) (string, []string) {
	if orig.MessageID == "" {
		return "", nil
	}
	refs := make([]string, 0, len(orig.References)+1)
	for _, r := range orig.References {
		if r != "" {
			refs = append(refs, r)
		}
	}
	return orig.MessageID, append(refs, orig.MessageID)
}

// validHeaderID dice si id se puede poner en un encabezado: no vacío y sin
// saltos de línea (el pedido puede venir de la UI).
func validHeaderID(id string) bool {
	return id != "" && !strings.ContainsAny(id, "\r\n")
}

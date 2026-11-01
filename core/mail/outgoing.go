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

	var orig *model.MessageDetail
	if replying || forwarding {
		d, err := c.GetMessage(ctx, req.Reply.OriginalMessageID)
		if err != nil {
			return mimex.Outgoing{}, err
		}
		orig = &d
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

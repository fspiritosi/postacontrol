package mail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/htmlx"
	"postacontrol/core/imapx"
	"postacontrol/core/mimex"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const snippetLen = 160

// GetMessage devuelve el detalle de un mensaje. Si el cuerpo no está en la
// caché lo baja del servidor, lo sanea y lo guarda; si el mensaje no está
// leído, lo marca \Seen en el servidor y en la caché. Los adjuntos no se
// bajan. Si el cuerpo ya está cacheado, funciona sin conexión: marcar \Seen
// es de mejor esfuerzo y un fallo de red solo emite EventSyncError.
func (c *Core) GetMessage(ctx context.Context, messageID int64) (model.MessageDetail, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return model.MessageDetail{}, err
	}
	d, cached, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return model.MessageDetail{}, c.ctxOrUserError(ctx, err)
	}
	if !cached {
		row, err := c.fetchBody(ctx, d)
		if err != nil {
			return model.MessageDetail{}, err
		}
		if err := c.store.SetBody(ctx, messageID, row); err != nil {
			return model.MessageDetail{}, c.ctxOrUserError(ctx, err)
		}
		d.HTML, d.Text, d.HasRemoteImages, d.Attachments = row.HTML, row.Text, row.HasRemoteImages, row.Attachments
		if row.Snippet != "" {
			d.Snippet = row.Snippet
		}
		d.HasAttachments = len(row.Attachments) > 0
	}
	if !d.Seen {
		if err := c.markSeen(ctx, d); err != nil {
			if cerr := c.ctxErr(ctx); cerr != nil {
				return model.MessageDetail{}, cerr
			}
			if !seenIsBestEffort(err) {
				// Error de caché o del mensaje: no se silencia, la caché
				// quedaría divergente del servidor.
				return model.MessageDetail{}, asUserError(err)
			}
			// Sin red (o servidor caído) o si el servidor rechaza el \Seen,
			// el mensaje se lee igual y queda no leído en la caché: la
			// próxima sincronización reconcilia.
		} else {
			d.Seen = true
		}
	}
	return normalizeDetail(d), nil
}

// seenIsBestEffort dice si una falla al marcar \Seen se puede ignorar y
// mostrar igual el mensaje: errores de conexión (withConn ya los informa por
// EventSyncError) o un NO/BAD del servidor al STORE.
//
// El NO/BAD se busca primero: withConn ya lo traduce a UserError "server",
// que lo conserva en la cadena.
func seenIsBestEffort(err error) bool {
	var ierr *goimap.Error
	if errors.As(err, &ierr) {
		return true
	}
	var ue *UserError
	if errors.As(err, &ue) {
		return ue.Code == "network" || ue.Code == "auth" || ue.Code == "tls"
	}
	return false
}

func normalizeDetail(d model.MessageDetail) model.MessageDetail {
	if d.References == nil {
		d.References = []string{}
	}
	if d.To == nil {
		d.To = []model.Address{}
	}
	if d.Cc == nil {
		d.Cc = []model.Address{}
	}
	if d.ReplyTo == nil {
		d.ReplyTo = []model.Address{}
	}
	if d.Attachments == nil {
		d.Attachments = []model.Attachment{}
	}
	return d
}

// selectChecked abre la carpeta y verifica que el UIDVALIDITY siga siendo el
// de la caché: si cambió, los UID guardados ya no apuntan a lo mismo.
func selectChecked(conn *imapx.Conn, f model.Folder, last store.FolderSync, rw bool) error {
	var (
		info imapx.SelectInfo
		err  error
	)
	if rw {
		info, err = conn.SelectRW(f.Name)
	} else {
		info, err = conn.Select(f.Name)
	}
	if err != nil {
		return err
	}
	if last.UIDValidity != 0 && info.UIDValidity != last.UIDValidity {
		return &UserError{Code: "stale", Msg: "El mensaje cambió en el servidor. Actualizá la carpeta."}
	}
	return nil
}

// fetchPart baja y devuelve la cabecera MIME y el cuerpo de una parte. Si el
// mensaje no es multipart, la única parte es el mensaje mismo: se baja con
// FetchPart(uid, nil) y la cabecera devuelta es la del mensaje.
func fetchPart(conn *imapx.Conn, uid uint32, part []int, multipart bool) ([]byte, []byte, error) {
	if !multipart {
		part = nil
	}
	return conn.FetchPart(uid, part)
}

func (c *Core) fetchBody(ctx context.Context, d model.MessageDetail) (store.BodyRow, error) {
	f, last, err := c.store.GetFolder(ctx, d.FolderID)
	if err != nil {
		return store.BodyRow{}, c.ctxOrUserError(ctx, err)
	}
	var row store.BodyRow
	err = c.withConn(ctx, d.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, false); err != nil {
			return err
		}
		if c.beforeFetchBody != nil {
			c.beforeFetchBody(conn, d.UID)
		}
		bs, err := conn.FetchStructure(d.UID)
		if err != nil {
			return err
		}
		_, multipart := bs.(*goimap.BodyStructureMultiPart)
		plan := mimex.Plan(bs)

		var text, html string
		if plan.TextPart != nil {
			hdr, body, err := fetchPart(conn, d.UID, plan.TextPart, multipart)
			if err != nil {
				return err
			}
			if text, err = mimex.DecodeText(hdr, body); err != nil {
				return err
			}
		}
		if plan.HTMLPart != nil {
			hdr, body, err := fetchPart(conn, d.UID, plan.HTMLPart, multipart)
			if err != nil {
				return err
			}
			if html, err = mimex.DecodeText(hdr, body); err != nil {
				return err
			}
		}
		if html != "" {
			cids := map[string]string{}
			for _, in := range plan.Inline {
				if !strings.HasPrefix(strings.ToLower(in.MIMEType), "image/") {
					continue
				}
				hdr, body, err := fetchPart(conn, d.UID, in.Part, multipart)
				if err != nil {
					return err
				}
				raw, err := mimex.DecodeBinary(hdr, body)
				if err != nil {
					continue // imagen ilegible: queda el cid: sin resolver
				}
				cids[in.ContentID] = "data:" + in.MIMEType + ";base64," + base64.StdEncoding.EncodeToString(raw)
			}
			html = mimex.ResolveCID(html, cids)
		}

		row = store.BodyRow{Attachments: plan.Attachments}
		if html != "" {
			row.HTML, row.HasRemoteImages = htmlx.Sanitize(html)
		}
		row.Text = text
		if row.Text == "" && row.HTML != "" {
			row.Text = htmlx.ToText(row.HTML)
		}
		row.Snippet = htmlx.Snippet(row.Text, snippetLen)
		return nil
	})
	return row, err
}

// markSeen marca \Seen en el servidor y refleja el cambio en la caché.
func (c *Core) markSeen(ctx context.Context, d model.MessageDetail) error {
	f, last, err := c.store.GetFolder(ctx, d.FolderID)
	if err != nil {
		return err
	}
	// Marcar \Seen es idempotente, así que alcanza con withConn.
	err = c.withConn(ctx, d.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, true); err != nil {
			return err
		}
		return conn.AddFlags(d.UID, goimap.FlagSeen)
	})
	if err != nil {
		return err
	}
	if c.afterServerSeen != nil {
		c.afterServerSeen()
	}
	if err := c.store.SetFlags(ctx, d.FolderID, d.UID, true, d.Flagged, d.Answered); err != nil {
		return err
	}
	if f.Unread > 0 {
		if err := c.store.SetFolderCounts(ctx, d.FolderID, f.Unread-1, f.Total); err != nil {
			return err
		}
	}
	c.emit(EventFolderUpdated, map[string]any{"folderId": d.FolderID})
	return nil
}

// SaveAttachment baja el adjunto index del mensaje y lo escribe en destPath
// (0644). Si el archivo existe lo reemplaza: el diálogo nativo de guardar ya
// pidió confirmación.
func (c *Core) SaveAttachment(ctx context.Context, messageID int64, index int, destPath string) error {
	_, data, err := c.attachmentBytes(ctx, messageID, index)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(destPath, data, 0o644); err != nil {
		return &UserError{Code: "io", Msg: "No pudimos guardar el archivo en esa ubicación.", Err: err}
	}
	return nil
}

// writeFileAtomic escribe en un temporal del mismo directorio y lo renombra,
// así un corte a mitad de camino no deja destPath a medio escribir.
func writeFileAtomic(destPath string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".postacontrol-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Chmod(perm)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, destPath)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}

// attachmentBytes baja y decodifica un adjunto. Lo usa SaveAttachment y el
// reenvío (Task 13). Funciona aunque el adjunto no tenga nombre.
func (c *Core) attachmentBytes(ctx context.Context, messageID int64, index int) (model.Attachment, []byte, error) {
	d, err := c.GetMessage(ctx, messageID)
	if err != nil {
		return model.Attachment{}, nil, err
	}
	if index < 0 || index >= len(d.Attachments) {
		return model.Attachment{}, nil, &UserError{Code: "not_found", Msg: msgNotFound}
	}
	att := d.Attachments[index]
	f, last, err := c.store.GetFolder(ctx, d.FolderID)
	if err != nil {
		return model.Attachment{}, nil, c.ctxOrUserError(ctx, err)
	}
	var data []byte
	err = c.withConn(ctx, d.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, false); err != nil {
			return err
		}
		multipart := true
		if len(att.Part) == 1 && att.Part[0] == 1 { // ambiguo: hijo 1 o mensaje simple
			bs, err := conn.FetchStructure(d.UID)
			if err != nil {
				return err
			}
			_, multipart = bs.(*goimap.BodyStructureMultiPart)
		}
		hdr, body, err := fetchPart(conn, d.UID, att.Part, multipart)
		if err != nil {
			return err
		}
		data, err = mimex.DecodeBinary(hdr, body)
		if err != nil {
			return fmt.Errorf("decodificar adjunto: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.Attachment{}, nil, err
	}
	return att, data, nil
}

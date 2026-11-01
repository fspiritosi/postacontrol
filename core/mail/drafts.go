package mail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"strings"
	"sync"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
	"postacontrol/core/mimex"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

// Borradores del redactor: se guardan en la compu (SaveDraft, sin red) y se
// suben a la carpeta con rol "drafts" de la cuenta (UploadDraft). Cada
// borrador tiene un Message-ID fijo: la versión nueva se sube con APPEND y
// después se borran las anteriores, que son las copias con ese Message-ID.
// Son las únicas escrituras en el servidor que hace este archivo, y nunca hay
// un EXPUNGE sin UID: sin UIDPLUS las copias viejas quedan con \Deleted.

const (
	msgNoDraftsFolder    = "Esta cuenta no tiene carpeta Borradores en el servidor."
	msgDraftServerDelete = "Borramos el borrador de esta compu, pero no pudimos borrarlo del servidor."
	msgNotADraft         = "Ese correo no es un borrador."
	msgDraftUploadFailed = "No pudimos guardar el borrador en el servidor."
	msgDraftBuildFailed  = "No pudimos armar el borrador."
)

// withDraftIDs completa en el pedido el id y el Message-ID del borrador: el
// redactor los manda de vuelta al enviar.
func withDraftIDs(d model.Draft) model.Draft {
	d.Request.ComposeDraftID = d.ID
	d.Request.DraftMessageID = d.MessageID
	return d
}

// draftLock devuelve el lock del borrador id.
func (c *Core) draftLock(id int64) *sync.Mutex {
	c.draftLocksMu.Lock()
	defer c.draftLocksMu.Unlock()
	mu := c.draftLocks[id]
	if mu == nil {
		mu = &sync.Mutex{}
		c.draftLocks[id] = mu
	}
	return mu
}

// SaveDraft guarda el borrador en la compu, sin tocar la red, y lo deja
// pendiente de subir. Con id 0 lo crea con un Message-ID nuevo. Solo valida la
// cuenta: un borrador puede no tener destinatarios. Un id que ya no existe
// (porque se envió o se descartó) devuelve UserError "not_found": nunca lo
// recrea.
func (c *Core) SaveDraft(ctx context.Context, id int64, req model.SendRequest) (model.Draft, error) {
	if err := c.ctxErr(ctx); err != nil {
		return model.Draft{}, err
	}
	acc, err := c.store.GetAccount(ctx, req.AccountID)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	var messageID string
	if id == 0 {
		messageID = mimex.NewMessageID(acc.Email)
	} else {
		d, err := c.store.GetComposeDraft(ctx, id)
		if err != nil {
			return model.Draft{}, c.ctxOrUserError(ctx, err)
		}
		messageID = d.MessageID
	}
	// El id y el Message-ID viven en la fila, no dentro del pedido.
	req.ComposeDraftID, req.DraftMessageID = 0, ""
	id, err = c.store.SaveComposeDraft(ctx, id, acc.ID, req, messageID)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	d, err := c.store.GetComposeDraft(ctx, id)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventDraftsChanged, nil)
	return withDraftIDs(d), nil
}

// ListPendingDrafts devuelve los borradores que todavía no se subieron al
// servidor, el más nuevo primero.
func (c *Core) ListPendingDrafts(ctx context.Context) ([]model.Draft, error) {
	ds, err := c.store.ListPendingDrafts(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	for i := range ds {
		ds[i] = withDraftIDs(ds[i])
	}
	return ds, nil
}

// UploadDraft sube el borrador a la carpeta Borradores de su cuenta y borra
// las versiones anteriores. Es autoguardado: si falla (sin red, sin carpeta
// Borradores, el servidor lo rechaza) no devuelve error, sino el borrador con
// el motivo en UploadError y todavía pendiente. Un borrador que ya no existe
// devuelve UserError "not_found" sin ir a la red.
func (c *Core) UploadDraft(ctx context.Context, id int64) (model.Draft, error) {
	return c.uploadDraftByID(ctx, id, false)
}

// uploadDraftIfPending es UploadDraft para la pasada de fondo: si el borrador
// ya no está pendiente (lo subió el redactor después de que la pasada lo
// listó) lo devuelve como está, sin ir a la red.
func (c *Core) uploadDraftIfPending(ctx context.Context, id int64) (model.Draft, error) {
	return c.uploadDraftByID(ctx, id, true)
}

func (c *Core) uploadDraftByID(ctx context.Context, id int64, onlyPending bool) (model.Draft, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	mu := c.draftLock(id)
	mu.Lock()
	defer mu.Unlock()
	if err := c.ctxErr(ctx); err != nil {
		return model.Draft{}, err
	}
	d, err := c.store.GetComposeDraft(ctx, id)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	if onlyPending && !d.NeedsUpload {
		return withDraftIDs(d), nil
	}
	if err := c.uploadDraft(ctx, d); err != nil {
		if cerr := c.ctxErr(ctx); cerr != nil {
			return model.Draft{}, cerr // sigue pendiente; se reintenta en la próxima pasada
		}
		msg := msgDraftUploadFailed
		var ue *UserError
		if errors.As(asUserError(err), &ue) {
			msg = ue.Msg
		}
		if err := c.store.MarkDraftUploadFailed(ctx, id, msg); err != nil {
			return model.Draft{}, c.ctxOrUserError(ctx, err)
		}
	}
	out, err := c.store.GetComposeDraft(ctx, id)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventDraftsChanged, nil)
	return withDraftIDs(out), nil
}

// uploadDraft hace la subida de d; quien llama tiene el lock del borrador.
func (c *Core) uploadDraft(ctx context.Context, d model.Draft) error {
	acc, err := c.store.GetAccount(ctx, d.Request.AccountID)
	if err != nil {
		return err
	}
	// Primero lo que no va a la red: armar un reenvío o una respuesta puede
	// bajar el original del servidor.
	drafts, ok, err := c.draftsFolder(ctx, acc.ID)
	if err != nil {
		return err
	}
	if !ok {
		return &UserError{Code: "no_drafts_folder", Msg: msgNoDraftsFolder}
	}
	// El autoguardado no es un pedido explícito: con la cuenta en pausa por
	// contraseña rechazada no repite el LOGIN (el proveedor podría bloquearla).
	if c.authPaused(acc.ID) {
		return &UserError{Code: "auth", Msg: msgAuthRejected}
	}
	req := d.Request
	req.DraftMessageID = d.MessageID // siempre el de la fila
	out, err := c.buildOutgoing(ctx, acc, req, true)
	if err != nil {
		return err
	}
	raw, err := mimex.Build(out)
	if err != nil {
		return &UserError{Code: "invalid", Msg: msgDraftBuildFailed, Err: err}
	}
	// Como en appendSent: NOOP valida (o reemplaza) la conexión del pool y
	// recién entonces va el APPEND, sin reintento porque no es idempotente.
	if err := c.withConn(ctx, acc.ID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Noop()
	}); err != nil {
		return err
	}
	if c.beforeDraftAppend != nil {
		c.beforeDraftAppend(d.ID)
	}
	if err := c.withConnNoRetry(ctx, acc.ID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Append(drafts.Name, raw, goimap.FlagDraft, goimap.FlagSeen)
	}); err != nil {
		return err
	}
	if c.afterDraftAppend != nil {
		c.afterDraftAppend(d.ID)
	}
	if err := c.withConn(ctx, acc.ID, func(_ context.Context, conn *imapx.Conn) error {
		return pruneDraftCopies(conn, drafts.Name, d.MessageID, true)
	}); err != nil {
		// La versión nueva ya está, pero pueden quedar copias viejas: sigue
		// pendiente y la próxima subida las limpia.
		return err
	}
	prevAccountID, prevErr := c.store.DraftUploadedAccount(ctx, d.ID)
	// Con el UpdatedAt que se leyó al empezar: si se guardó durante la subida,
	// sigue pendiente.
	if err := c.store.MarkDraftUploaded(ctx, d.ID, d.UpdatedAt, acc.ID); err != nil {
		return err
	}
	if prevErr == nil && prevAccountID != acc.ID {
		// Cambió "De": la copia anterior quedó en la otra cuenta.
		c.removeOldAccountCopies(ctx, prevAccountID, d.MessageID)
	}
	_ = c.SyncFolder(ctx, drafts.ID) // mejor esfuerzo
	c.emit(EventFolderUpdated, map[string]any{"folderId": drafts.ID})
	return nil
}

// draftsFolder devuelve la carpeta con rol "drafts" de la cuenta, si la hay.
func (c *Core) draftsFolder(ctx context.Context, accountID int64) (model.Folder, bool, error) {
	fs, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Folder{}, false, err
	}
	for _, f := range fs {
		if f.Role == "drafts" && f.Selectable {
			return f, true, nil
		}
	}
	return model.Folder{}, false, nil
}

// pruneDraftCopies abre la carpeta Borradores para escritura y borra las
// copias del borrador messageID; con keepNewest deja la de UID más alto (la
// recién subida). Solo toca los UID que devuelve SearchHeader y cuyo
// Message-ID coincide exacto (el servidor busca por subcadena). Borra con
// UID EXPUNGE; sin UIDPLUS solo marca \Deleted.
func pruneDraftCopies(conn *imapx.Conn, mailbox, messageID string, keepNewest bool) error {
	if messageID == "" {
		return nil
	}
	if _, err := conn.SelectRW(mailbox); err != nil {
		return err
	}
	found, err := conn.SearchHeader("Message-ID", messageID)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return nil
	}
	hs, err := conn.FetchHeaders(found)
	if err != nil {
		return err
	}
	exact := map[uint32]bool{}
	for _, h := range hs {
		if h.Envelope != nil && bracketMsgID(h.Envelope.MessageID) == messageID {
			exact[h.UID] = true
		}
	}
	uids := make([]uint32, 0, len(found))
	for _, u := range found { // found ya viene en orden ascendente
		if exact[u] {
			uids = append(uids, u)
		}
	}
	if keepNewest && len(uids) > 0 {
		uids = uids[:len(uids)-1]
	}
	if len(uids) == 0 {
		return nil
	}
	err = conn.ExpungeUIDs(uids)
	if errors.Is(err, imapx.ErrUnsupported) {
		return conn.StoreFlags(uids, true, goimap.FlagDeleted)
	}
	return err
}

// removeDraftCopies borra del servidor todas las copias del borrador
// messageID en la carpeta Borradores de la cuenta y la vuelve a sincronizar.
// Sin carpeta Borradores no hay nada que borrar.
func (c *Core) removeDraftCopies(ctx context.Context, accountID int64, messageID string) error {
	if messageID == "" {
		return nil
	}
	drafts, ok, err := c.draftsFolder(ctx, accountID)
	if err != nil || !ok {
		return err
	}
	if err := c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		return pruneDraftCopies(conn, drafts.Name, messageID, false)
	}); err != nil {
		return err
	}
	_ = c.SyncFolder(ctx, drafts.ID) // mejor esfuerzo
	c.emit(EventFolderUpdated, map[string]any{"folderId": drafts.ID})
	return nil
}

// removeOldAccountCopies borra las copias del borrador messageID que quedaron
// en la carpeta Borradores de accountID, una cuenta que el borrador ya no usa.
// Es de mejor esfuerzo, y con la cuenta en pausa por contraseña rechazada no
// lo intenta.
func (c *Core) removeOldAccountCopies(ctx context.Context, accountID int64, messageID string) {
	if accountID == 0 || c.authPaused(accountID) {
		return
	}
	_ = c.removeDraftCopies(ctx, accountID, messageID)
}

// DeleteDraft descarta el borrador: borra sus copias del servidor, la fila y
// los adjuntos preparados que solo él usaba (los de la bandeja de salida
// quedan). Si el servidor no responde, borra lo local igual y devuelve un
// UserError "draft".
func (c *Core) DeleteDraft(ctx context.Context, id int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	mu := c.draftLock(id)
	mu.Lock()
	defer mu.Unlock()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	d, err := c.store.GetComposeDraft(ctx, id)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	serverErr := c.removeDraftCopies(ctx, d.Request.AccountID, d.MessageID)
	if up, err := c.store.DraftUploadedAccount(ctx, id); err == nil && up != d.Request.AccountID {
		c.removeOldAccountCopies(ctx, up, d.MessageID)
	}
	// Lo local se borra aunque se esté cerrando la app.
	if err := c.store.DeleteComposeDraftWithStaged(context.WithoutCancel(ctx), id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventDraftsChanged, nil)
	if serverErr != nil {
		return &UserError{Code: "draft", Msg: msgDraftServerDelete, Err: serverErr}
	}
	return nil
}

// discardSentDraft borra del servidor las copias del borrador del que salió un
// envío, en la cuenta del envío y, si el borrador había quedado subido en
// otra (oldAccountID), también ahí. Es de mejor esfuerzo: el correo ya salió.
// Si en la compu hay otro borrador vivo con ese Message-ID (el usuario lo
// retomó desde Borradores después de que el envío falló), las copias son de
// ese borrador y no se tocan.
func (c *Core) discardSentDraft(ctx context.Context, accountID int64, req model.SendRequest, oldAccountID int64) {
	if req.DraftMessageID == "" {
		return
	}
	if req.ComposeDraftID != 0 {
		// Si había una subida en curso, se espera a que termine: si no, su
		// APPEND dejaría una copia después de la limpieza.
		mu := c.draftLock(req.ComposeDraftID)
		mu.Lock()
		defer mu.Unlock()
	}
	// En un reintento de la bandeja de salida ComposeDraftID es 0: cualquier
	// fila con ese Message-ID es un borrador retomado.
	if d, err := c.store.ComposeDraftByMessageID(ctx, req.DraftMessageID); err == nil && d.ID != req.ComposeDraftID {
		return
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return // sin poder confirmarlo, no se borra nada
	}
	_ = c.removeDraftCopies(ctx, accountID, req.DraftMessageID)
	if oldAccountID != accountID {
		c.removeOldAccountCopies(ctx, oldAccountID, req.DraftMessageID)
	}
}

// forgetLocalDrafts borra de la compu los borradores de la cuenta cuyas
// copias el usuario acaba de borrar o de sacar de la carpeta Borradores
// (DeleteMessages o MoveMessages), con los adjuntos preparados que solo ellos
// usaban. Así la pasada de fondo no vuelve a subir un borrador que el usuario
// borró. No escribe nada en el servidor: eso ya lo hizo quien llama.
func (c *Core) forgetLocalDrafts(ctx context.Context, accountID int64, messageIDs []string) {
	changed := false
	for _, mid := range messageIDs {
		if mid == "" {
			continue
		}
		d, err := c.store.ComposeDraftByMessageID(ctx, mid)
		// Si el borrador ya es de otra cuenta (cambió "De"), lo borrado era la
		// copia vieja: el borrador sigue.
		if err != nil || d.Request.AccountID != accountID {
			continue
		}
		if c.forgetLocalDraft(ctx, d.ID, mid) {
			changed = true
		}
	}
	if changed {
		c.emit(EventDraftsChanged, nil)
	}
}

// forgetLocalDraft borra el borrador id si todavía tiene ese Message-ID. Toma
// su lock: si había una subida en curso, espera a que termine.
func (c *Core) forgetLocalDraft(ctx context.Context, id int64, messageID string) bool {
	mu := c.draftLock(id)
	mu.Lock()
	defer mu.Unlock()
	d, err := c.store.GetComposeDraft(ctx, id)
	if err != nil || d.MessageID != messageID {
		return false
	}
	return c.store.DeleteComposeDraftWithStaged(context.WithoutCancel(ctx), id) == nil
}

// uploadPendingDrafts sube los borradores pendientes de la cuenta (pasada de
// fondo). Los resultados quedan en cada borrador.
func (c *Core) uploadPendingDrafts(ctx context.Context, accountID int64) {
	ds, err := c.store.ListPendingDrafts(ctx)
	if err != nil {
		return
	}
	for _, d := range ds {
		if ctx.Err() != nil {
			return
		}
		if d.Request.AccountID == accountID {
			_, _ = c.uploadDraftIfPending(ctx, d.ID)
		}
	}
}

// OpenDraftMessage abre para seguir editando un mensaje de la carpeta
// Borradores. Si es un borrador de esta compu (mismo Message-ID) devuelve ese;
// si no (lo creó el webmail o el celular), crea uno con sus destinatarios
// (CCO incluido), asunto, cuerpo y adjuntos, y el mismo Message-ID: así la
// próxima subida reemplaza al original. La cuenta es la de la carpeta.
func (c *Core) OpenDraftMessage(ctx context.Context, messageID int64) (model.Draft, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return model.Draft{}, err
	}
	m, _, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	f, last, err := c.store.GetFolder(ctx, m.FolderID)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	if f.Role != "drafts" {
		return model.Draft{}, &UserError{Code: "invalid", Msg: msgNotADraft}
	}
	if m.MessageID != "" {
		d, err := c.store.ComposeDraftByMessageID(ctx, m.MessageID)
		if err == nil {
			return withDraftIDs(d), nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return model.Draft{}, c.ctxOrUserError(ctx, err)
		}
	}
	acc, err := c.store.GetAccount(ctx, f.AccountID)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	full, err := c.GetMessage(ctx, messageID)
	if err != nil {
		return model.Draft{}, err
	}
	// La caché no guarda el CCO: sale del ENVELOPE del servidor.
	bcc := []model.Address{}
	err = c.withConn(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, false); err != nil {
			return err
		}
		hs, err := conn.FetchHeaders([]uint32{m.UID})
		if err != nil {
			return err
		}
		if len(hs) == 0 {
			return fmt.Errorf("%w: UID %d", imapx.ErrNoMessage, m.UID)
		}
		if hs[0].Envelope != nil {
			bcc = toAddresses(hs[0].Envelope.Bcc)
		}
		return nil
	})
	if err != nil {
		return model.Draft{}, err
	}

	staged := []int64{}
	rollback := func() { _ = c.store.DeleteStagedAttachments(context.WithoutCancel(ctx), staged) }
	for i := range full.Attachments {
		att, data, err := c.attachmentBytes(ctx, messageID, i)
		if err != nil {
			rollback()
			return model.Draft{}, err
		}
		name := att.Filename
		if name == "" {
			name = fmt.Sprintf("adjunto-%d", i+1)
		}
		st, err := c.store.StageAttachment(ctx, name, att.MIMEType, data)
		if err != nil {
			rollback()
			return model.Draft{}, c.ctxOrUserError(ctx, err)
		}
		staged = append(staged, st.ID)
	}

	req := model.SendRequest{
		AccountID: f.AccountID, To: full.To, Cc: full.Cc, Bcc: bcc,
		Subject: full.Subject, BodyHTML: draftBodyHTML(full),
		Attachments: staged, SkipOriginalAttachments: []int{},
	}
	msgID := m.MessageID
	if msgID == "" {
		msgID = mimex.NewMessageID(acc.Email)
	}
	id, err := c.store.SaveComposeDraft(ctx, 0, f.AccountID, req, msgID)
	if err != nil {
		rollback()
		// Otra apertura del mismo mensaje pudo crearlo primero.
		if d, e2 := c.store.ComposeDraftByMessageID(ctx, msgID); e2 == nil {
			return withDraftIDs(d), nil
		}
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	d, err := c.store.GetComposeDraft(ctx, id)
	if err != nil {
		return model.Draft{}, c.ctxOrUserError(ctx, err)
	}
	if m.MessageID != "" {
		// El servidor ya tiene esta versión: no hay nada que subir hasta que
		// se edite.
		if err := c.store.MarkDraftUploaded(ctx, id, d.UpdatedAt, f.AccountID); err == nil {
			d.NeedsUpload = false
		}
	}
	return withDraftIDs(d), nil
}

// draftBodyHTML es el cuerpo de un borrador ajeno para el redactor: su HTML
// saneado o, si solo tiene texto, el texto escapado en un párrafo.
func draftBodyHTML(d model.MessageDetail) string {
	if d.HTML != "" {
		return d.HTML
	}
	text := strings.ReplaceAll(strings.TrimSpace(d.Text), "\r\n", "\n")
	return "<p>" + strings.ReplaceAll(html.EscapeString(text), "\n", "<br>") + "</p>"
}

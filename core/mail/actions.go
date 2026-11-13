package mail

import (
	"context"
	"errors"
	"log/slog"
	"slices"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const msgOneAccount = "Elegí correos de una sola cuenta."

// groupByFolder agrupa las referencias por carpeta y verifica que todas sean
// de la misma cuenta; si no, devuelve un UserError "invalid".
func groupByFolder(refs []store.MessageRef) (int64, map[int64][]store.MessageRef, error) {
	byFolder := map[int64][]store.MessageRef{}
	var accountID int64
	for i, r := range refs {
		if i == 0 {
			accountID = r.AccountID
		} else if r.AccountID != accountID {
			return 0, nil, &UserError{Code: "invalid", Msg: msgOneAccount}
		}
		byFolder[r.FolderID] = append(byFolder[r.FolderID], r)
	}
	return accountID, byFolder, nil
}

// SetSeen marca los mensajes como leídos o no leídos en el servidor y en la caché.
func (c *Core) SetSeen(ctx context.Context, ids []int64, seen bool) error {
	return c.setFlag(ctx, ids, seen, goimap.FlagSeen)
}

// SetFlagged destaca o quita el destacado de los mensajes.
func (c *Core) SetFlagged(ctx context.Context, ids []int64, flagged bool) error {
	return c.setFlag(ctx, ids, flagged, goimap.FlagFlagged)
}

func (c *Core) setFlag(ctx context.Context, ids []int64, on bool, flag goimap.Flag) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	refs, err := c.store.MessageRefs(ctx, ids)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	accountID, byFolder, err := groupByFolder(refs)
	if err != nil {
		return err
	}
	for folderID, rs := range byFolder {
		f, last, err := c.store.GetFolder(ctx, folderID)
		if err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		uids := make([]uint32, len(rs))
		for i, r := range rs {
			uids[i] = r.UID
		}
		// Escribir el mismo flag dos veces es idempotente: alcanza con withConn.
		err = c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
			if err := selectChecked(conn, f, last, true); err != nil {
				return err
			}
			return conn.StoreFlags(uids, on, flag)
		})
		if err != nil {
			return err
		}
		for _, r := range rs {
			seen, flagged := r.Seen, r.Flagged
			if flag == goimap.FlagSeen {
				seen = on
			} else {
				flagged = on
			}
			if err := c.store.SetFlags(ctx, folderID, r.UID, seen, flagged, r.Answered); err != nil {
				return c.ctxOrUserError(ctx, err)
			}
		}
		if err := c.store.RecountFolder(ctx, folderID); err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		c.emit(EventFolderUpdated, map[string]any{"folderId": folderID})
	}
	return nil
}

const (
	msgBadDest = "No se puede mover a esa carpeta."
	msgNoTrash = "Esta cuenta no tiene carpeta Papelera."
)

// moveBatch son los mensajes de una carpeta de origen y adónde van. dest nil
// quiere decir borrarlos definitivamente.
type moveBatch struct {
	src  model.Folder
	last store.FolderSync
	refs []store.MessageRef
	dest *model.Folder
}

// MoveMessages mueve los mensajes a otra carpeta de la misma cuenta. Los que
// ya están en el destino quedan como están.
func (c *Core) MoveMessages(ctx context.Context, ids []int64, destFolderID int64) error {
	_, err := c.moveMessages(ctx, ids, destFolderID, false)
	return err
}

// MoveMessagesUndoable es MoveMessages y además devuelve con qué deshacerlo
// (ver UndoMove; 0 = no hay nada que deshacer).
func (c *Core) MoveMessagesUndoable(ctx context.Context, ids []int64, destFolderID int64) (int64, error) {
	return c.moveMessages(ctx, ids, destFolderID, true)
}

func (c *Core) moveMessages(ctx context.Context, ids []int64, destFolderID int64, undoable bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	accountID, byFolder, err := c.refsByFolder(ctx, ids)
	if err != nil {
		return 0, err
	}
	dest, _, err := c.store.GetFolder(ctx, destFolderID)
	if err != nil {
		return 0, c.ctxOrUserError(ctx, err)
	}
	if dest.AccountID != accountID || !dest.Selectable {
		return 0, &UserError{Code: "invalid", Msg: msgBadDest}
	}
	batches, err := c.planMoves(ctx, byFolder, func(src model.Folder) (*model.Folder, bool) {
		return &dest, src.ID != dest.ID
	})
	if err != nil {
		return 0, err
	}
	return c.runMoves(ctx, accountID, batches, undoable)
}

// DeleteMessages manda los mensajes a la Papelera de la cuenta; los que ya
// están en la Papelera se borran definitivamente.
func (c *Core) DeleteMessages(ctx context.Context, ids []int64) error {
	_, err := c.deleteMessages(ctx, ids, false)
	return err
}

// DeleteMessagesUndoable es DeleteMessages y además devuelve con qué deshacer
// lo que fue a la Papelera (lo borrado definitivamente no se puede).
func (c *Core) DeleteMessagesUndoable(ctx context.Context, ids []int64) (int64, error) {
	return c.deleteMessages(ctx, ids, true)
}

func (c *Core) deleteMessages(ctx context.Context, ids []int64, undoable bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	accountID, byFolder, err := c.refsByFolder(ctx, ids)
	if err != nil {
		return 0, err
	}
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return 0, c.ctxOrUserError(ctx, err)
	}
	var trash *model.Folder
	for i := range folders {
		if folders[i].Role == "trash" && folders[i].Selectable {
			trash = &folders[i]
			break
		}
	}
	if trash == nil {
		return 0, &UserError{Code: "no_trash", Msg: msgNoTrash}
	}
	batches, err := c.planMoves(ctx, byFolder, func(src model.Folder) (*model.Folder, bool) {
		if src.ID == trash.ID {
			return nil, true
		}
		return trash, true
	})
	if err != nil {
		return 0, err
	}
	return c.runMoves(ctx, accountID, batches, undoable)
}

// archiveFolderName es la carpeta que se crea para archivar cuando la cuenta
// no tiene una (el nombre la marca como Archivo, ver imapx.roleByName).
const archiveFolderName = "Archivo"

// ArchiveMessages mueve los mensajes a la carpeta Archivo de la cuenta. Si la
// cuenta no tiene, la crea (como Thunderbird). Los que ya están en Archivo
// quedan como están.
func (c *Core) ArchiveMessages(ctx context.Context, ids []int64) error {
	_, err := c.archiveMessages(ctx, ids, false)
	return err
}

// ArchiveMessagesUndoable es ArchiveMessages y además devuelve con qué
// deshacerlo.
func (c *Core) ArchiveMessagesUndoable(ctx context.Context, ids []int64) (int64, error) {
	return c.archiveMessages(ctx, ids, true)
}

func (c *Core) archiveMessages(ctx context.Context, ids []int64, undoable bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	accountID, byFolder, err := c.refsByFolder(ctx, ids)
	if err != nil {
		return 0, err
	}
	archive, err := c.archiveFolder(ctx, accountID)
	if err != nil {
		return 0, err
	}
	batches, err := c.planMoves(ctx, byFolder, func(src model.Folder) (*model.Folder, bool) {
		return &archive, src.ID != archive.ID
	})
	if err != nil {
		return 0, err
	}
	return c.runMoves(ctx, accountID, batches, undoable)
}

// archiveFolder devuelve la carpeta Archivo de la cuenta y, si no tiene, la crea.
func (c *Core) archiveFolder(ctx context.Context, accountID int64) (model.Folder, error) {
	find := func() (model.Folder, bool, error) {
		folders, err := c.store.ListFolders(ctx, accountID)
		if err != nil {
			return model.Folder{}, false, c.ctxOrUserError(ctx, err)
		}
		for _, f := range folders {
			if f.Role == "archive" && f.Selectable {
				return f, true, nil
			}
		}
		return model.Folder{}, false, nil
	}
	if f, ok, err := find(); err != nil || ok {
		return f, err
	}
	created, err := c.CreateFolder(ctx, accountID, 0, archiveFolderName)
	var ue *UserError
	if errors.As(err, &ue) && ue.Code == "duplicate" {
		// Otra acción la creó recién (o existe con otro rol): se usa esa.
		if f, ok, ferr := find(); ferr != nil || ok {
			return f, ferr
		}
	}
	return created, err
}

const msgNotTrash = "Solo se puede vaciar la Papelera."

// EmptyTrash borra definitivamente todos los correos de la Papelera según el
// servidor (UID SEARCH ALL, también los que la caché todavía no tiene) y deja
// la caché de la carpeta vacía. Devuelve cuántos borró.
func (c *Core) EmptyTrash(ctx context.Context, folderID int64) (int, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return 0, err
	}
	f, last, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return 0, c.ctxOrUserError(ctx, err)
	}
	if f.Role != "trash" || !f.Selectable {
		return 0, &UserError{Code: "invalid", Msg: msgNotTrash}
	}
	// Sin sincronizar la carpeta mientras tanto: si no, podría volver a
	// meter en la caché lo que se está borrando.
	mu := c.folderSyncLock(folderID)
	mu.Lock()
	defer mu.Unlock()
	// EXPUNGE no es idempotente: NOOP con reintento y después la escritura
	// sin reintento, como en applyMove.
	if err := c.probe(ctx, f.AccountID); err != nil {
		return 0, err
	}
	var deleted int
	err = c.withConnNoRetry(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, true); err != nil {
			return err
		}
		uids, err := conn.AllUIDs()
		if err != nil {
			return err
		}
		if err := conn.ExpungeUIDs(uids); err != nil {
			return err
		}
		deleted = len(uids)
		return nil
	})
	if err != nil {
		return 0, err
	}
	// Lo que llegó después del SEARCH no está en la caché (la sincronización
	// estaba frenada): la de abajo lo trae.
	if err := c.store.DeleteUIDsNotIn(ctx, folderID, nil); err != nil {
		return deleted, c.ctxOrUserError(ctx, err)
	}
	if err := c.store.RecountFolder(ctx, folderID); err != nil {
		return deleted, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventFolderUpdated, map[string]any{"folderId": folderID})
	c.spawn(func(ctx context.Context) { _ = c.SyncFolder(ctx, folderID) })
	return deleted, nil
}

// refsByFolder busca los mensajes en la caché y los agrupa por carpeta.
func (c *Core) refsByFolder(ctx context.Context, ids []int64) (int64, map[int64][]store.MessageRef, error) {
	if err := c.ctxErr(ctx); err != nil {
		return 0, nil, err
	}
	refs, err := c.store.MessageRefs(ctx, ids)
	if err != nil {
		return 0, nil, c.ctxOrUserError(ctx, err)
	}
	return groupByFolder(refs)
}

// planMoves arma un lote por carpeta de origen, en orden de ID. route dice
// adónde va cada carpeta (nil = borrar) y si hay que hacer algo con ella. Lee
// todas las carpetas antes de escribir nada en el servidor.
func (c *Core) planMoves(ctx context.Context, byFolder map[int64][]store.MessageRef,
	route func(src model.Folder) (*model.Folder, bool)) ([]moveBatch, error) {
	folderIDs := make([]int64, 0, len(byFolder))
	for id := range byFolder {
		folderIDs = append(folderIDs, id)
	}
	slices.Sort(folderIDs)
	batches := make([]moveBatch, 0, len(folderIDs))
	for _, id := range folderIDs {
		f, last, err := c.store.GetFolder(ctx, id)
		if err != nil {
			return nil, c.ctxOrUserError(ctx, err)
		}
		dest, ok := route(f)
		if !ok {
			continue
		}
		batches = append(batches, moveBatch{src: f, last: last, refs: byFolder[id], dest: dest})
	}
	return batches, nil
}

// runMoves aplica los lotes en orden y corta en el primer error. Las carpetas
// destino que recibieron correos se avisan y se sincronizan en segundo plano,
// aunque un lote posterior haya fallado. Con undoable anota cómo deshacer lo
// movido (no lo borrado definitivamente) y devuelve su id, o 0 si no hay nada.
func (c *Core) runMoves(ctx context.Context, accountID int64, batches []moveBatch, undoable bool) (int64, error) {
	var received []int64
	defer func() {
		for _, id := range received {
			c.emit(EventFolderUpdated, map[string]any{"folderId": id})
			c.spawn(func(ctx context.Context) { _ = c.SyncFolder(ctx, id) })
		}
	}()
	var undo []undoBatch
	for _, b := range batches {
		res, err := c.applyMove(ctx, accountID, b)
		if err != nil {
			return 0, err
		}
		if b.dest == nil {
			continue
		}
		if !slices.Contains(received, b.dest.ID) {
			received = append(received, b.dest.ID)
		}
		msgIDs := make([]string, 0, len(b.refs))
		for _, r := range b.refs {
			if r.MessageID != "" {
				msgIDs = append(msgIDs, r.MessageID)
			}
		}
		undo = append(undo, undoBatch{srcID: b.src.ID, destID: b.dest.ID,
			uidValidity: res.UIDValidity, destUIDs: res.Dest, messageIDs: msgIDs})
	}
	if !undoable || len(undo) == 0 {
		return 0, nil
	}
	return c.recordUndo(accountID, undo), nil
}

// saveLabelCarry, en las cuentas que guardan las etiquetas solo acá, anota las
// etiquetas de los mensajes que se mueven para ponérselas de nuevo cuando la
// sincronización del destino los traiga (por Message-ID). En las demás cuentas
// el keyword viaja con el mensaje en el servidor.
func (c *Core) saveLabelCarry(ctx context.Context, accountID int64, refs []store.MessageRef) error {
	local, err := c.store.AccountLabelsLocal(ctx, accountID)
	if err != nil || !local {
		return err
	}
	for _, r := range refs {
		if r.MessageID == "" || len(r.Labels) == 0 {
			continue
		}
		if err := c.store.SaveLabelCarry(ctx, accountID, r.MessageID, r.Labels); err != nil {
			return err
		}
	}
	return nil
}

// applyMove mueve o borra un lote en el servidor y recién después lo saca de
// la caché. Si el servidor falla, la caché queda como estaba: la próxima
// sincronización la pone al día con lo que sí haya pasado.
func (c *Core) applyMove(ctx context.Context, accountID int64, b moveBatch) (imapx.MoveResult, error) {
	uids := make([]uint32, len(b.refs))
	ids := make([]int64, len(b.refs))
	for i, r := range b.refs {
		uids[i], ids[i] = r.UID, r.ID
	}
	// MOVE y EXPUNGE no son idempotentes: como en appendSent, NOOP con
	// reintento valida la conexión y después va la escritura sin reintento.
	if err := c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Noop()
	}); err != nil {
		return imapx.MoveResult{}, err
	}
	var res imapx.MoveResult
	err := c.withConnNoRetry(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, b.src, b.last, true); err != nil {
			return err
		}
		if b.dest == nil {
			return conn.ExpungeUIDs(uids)
		}
		var err error
		res, err = conn.MoveUIDs(uids, b.dest.Name)
		return err
	})
	if err != nil {
		return imapx.MoveResult{}, err
	}
	if b.dest != nil {
		// El MOVE ya pasó: si no se pudo anotar el carry, las etiquetas locales
		// se pierden, pero la caché igual tiene que reflejar el movimiento.
		if err := c.saveLabelCarry(ctx, accountID, b.refs); err != nil {
			slog.Warn("no se pudieron guardar las etiquetas de los correos movidos", "carpeta", b.src.Name, "err", err)
		}
	}
	if err := c.store.DeleteMessages(ctx, ids); err != nil {
		return res, c.ctxOrUserError(ctx, err)
	}
	if err := c.store.RecountFolder(ctx, b.src.ID); err != nil {
		return res, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventFolderUpdated, map[string]any{"folderId": b.src.ID})
	if b.src.Role == "drafts" {
		msgIDs := make([]string, 0, len(b.refs))
		for _, r := range b.refs {
			msgIDs = append(msgIDs, r.MessageID)
		}
		c.forgetLocalDrafts(ctx, b.src.AccountID, msgIDs)
	}
	return res, nil
}

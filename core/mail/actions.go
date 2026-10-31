package mail

import (
	"context"
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
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	accountID, byFolder, err := c.refsByFolder(ctx, ids)
	if err != nil {
		return err
	}
	dest, _, err := c.store.GetFolder(ctx, destFolderID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	if dest.AccountID != accountID || !dest.Selectable {
		return &UserError{Code: "invalid", Msg: msgBadDest}
	}
	batches, err := c.planMoves(ctx, byFolder, func(src model.Folder) (*model.Folder, bool) {
		return &dest, src.ID != dest.ID
	})
	if err != nil {
		return err
	}
	return c.runMoves(ctx, accountID, batches)
}

// DeleteMessages manda los mensajes a la Papelera de la cuenta; los que ya
// están en la Papelera se borran definitivamente.
func (c *Core) DeleteMessages(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	accountID, byFolder, err := c.refsByFolder(ctx, ids)
	if err != nil {
		return err
	}
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	var trash *model.Folder
	for i := range folders {
		if folders[i].Role == "trash" && folders[i].Selectable {
			trash = &folders[i]
			break
		}
	}
	if trash == nil {
		return &UserError{Code: "no_trash", Msg: msgNoTrash}
	}
	batches, err := c.planMoves(ctx, byFolder, func(src model.Folder) (*model.Folder, bool) {
		if src.ID == trash.ID {
			return nil, true
		}
		return trash, true
	})
	if err != nil {
		return err
	}
	return c.runMoves(ctx, accountID, batches)
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
// aunque un lote posterior haya fallado.
func (c *Core) runMoves(ctx context.Context, accountID int64, batches []moveBatch) error {
	var received []int64
	defer func() {
		for _, id := range received {
			c.emit(EventFolderUpdated, map[string]any{"folderId": id})
			c.spawn(func(ctx context.Context) { _ = c.SyncFolder(ctx, id) })
		}
	}()
	for _, b := range batches {
		if err := c.applyMove(ctx, accountID, b); err != nil {
			return err
		}
		if b.dest != nil && !slices.Contains(received, b.dest.ID) {
			received = append(received, b.dest.ID)
		}
	}
	return nil
}

// applyMove mueve o borra un lote en el servidor y recién después lo saca de
// la caché. Si el servidor falla, la caché queda como estaba: la próxima
// sincronización la pone al día con lo que sí haya pasado.
func (c *Core) applyMove(ctx context.Context, accountID int64, b moveBatch) error {
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
		return err
	}
	err := c.withConnNoRetry(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, b.src, b.last, true); err != nil {
			return err
		}
		if b.dest == nil {
			return conn.ExpungeUIDs(uids)
		}
		return conn.Move(uids, b.dest.Name)
	})
	if err != nil {
		return err
	}
	if err := c.store.DeleteMessages(ctx, ids); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	if err := c.store.RecountFolder(ctx, b.src.ID); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventFolderUpdated, map[string]any{"folderId": b.src.ID})
	return nil
}

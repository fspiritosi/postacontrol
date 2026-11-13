package mail

import (
	"context"
	"errors"
	"slices"
	"time"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

// Deshacer un movimiento (mover, archivar o mandar a la Papelera): el núcleo
// recuerda por un rato adónde fue cada lote y UndoMove lo devuelve a su
// carpeta de origen. Lo borrado definitivamente no se puede deshacer.

const (
	// undoTTL: después de esto ya no se ofrece deshacer (el aviso dura segundos).
	undoTTL = 10 * time.Minute
	// maxUndos acota lo que se recuerda si se mueven muchas cosas seguidas.
	maxUndos = 50

	msgUndoGone = "Ya no se puede deshacer."
)

// undoBatch es un lote movido de srcID a destID. destUIDs son los UID nuevos
// en el destino si el servidor los informó (COPYUID, con UIDPLUS), con la
// UIDVALIDITY del destino. Si no, se buscan por Message-ID.
type undoBatch struct {
	srcID       int64
	destID      int64
	uidValidity uint32
	destUIDs    []uint32
	messageIDs  []string
}

type undoEntry struct {
	accountID int64
	batches   []undoBatch
	at        time.Time
}

// recordUndo guarda los lotes y devuelve con qué id deshacerlos.
func (c *Core) recordUndo(accountID int64, batches []undoBatch) int64 {
	c.undoMu.Lock()
	defer c.undoMu.Unlock()
	if c.undos == nil {
		c.undos = map[int64]undoEntry{}
	}
	now := time.Now()
	for id, e := range c.undos {
		if now.Sub(e.at) > undoTTL {
			delete(c.undos, id)
		}
	}
	for len(c.undos) >= maxUndos {
		oldest := int64(0)
		for id := range c.undos {
			if oldest == 0 || id < oldest {
				oldest = id
			}
		}
		delete(c.undos, oldest)
	}
	c.undoSeq++
	c.undos[c.undoSeq] = undoEntry{accountID: accountID, batches: batches, at: now}
	return c.undoSeq
}

// takeUndo saca la entrada: se deshace una sola vez.
func (c *Core) takeUndo(id int64) (undoEntry, bool) {
	c.undoMu.Lock()
	defer c.undoMu.Unlock()
	e, ok := c.undos[id]
	delete(c.undos, id)
	if !ok || time.Since(e.at) > undoTTL {
		return undoEntry{}, false
	}
	return e, true
}

// UndoMove devuelve a su carpeta de origen lo que movió la acción undoID. Lo
// que ya no está en el destino (se movió o se borró después) queda como está.
func (c *Core) UndoMove(ctx context.Context, undoID int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	e, ok := c.takeUndo(undoID)
	if !ok {
		return &UserError{Code: "notfound", Msg: msgUndoGone}
	}
	var touched []int64
	defer func() {
		for _, id := range touched {
			c.emit(EventFolderUpdated, map[string]any{"folderId": id})
			c.spawn(func(ctx context.Context) { _ = c.SyncFolder(ctx, id) })
		}
	}()
	// Como en applyMove: NOOP con reintento valida la conexión y después va
	// el MOVE, que no es idempotente, sin reintento.
	if err := c.withConn(ctx, e.accountID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Noop()
	}); err != nil {
		return err
	}
	for _, b := range slices.Backward(e.batches) {
		src, _, err := c.store.GetFolder(ctx, b.srcID)
		if err == nil {
			var dest model.Folder
			dest, _, err = c.store.GetFolder(ctx, b.destID)
			if err == nil {
				err = c.withConnNoRetry(ctx, e.accountID, func(_ context.Context, conn *imapx.Conn) error {
					return moveBack(conn, b, dest, src)
				})
			}
		}
		if errors.Is(err, store.ErrNotFound) {
			return &UserError{Code: "notfound", Msg: msgUndoGone, Err: err}
		}
		if err != nil {
			return err
		}
		for _, id := range []int64{b.srcID, b.destID} {
			if !slices.Contains(touched, id) {
				touched = append(touched, id)
			}
		}
	}
	return nil
}

// moveBack mueve de dest a src los mensajes del lote: los UID que informó el
// servidor si la carpeta no cambió (misma UIDVALIDITY) y, si no, los que tengan
// alguno de sus Message-ID.
func moveBack(conn *imapx.Conn, b undoBatch, dest, src model.Folder) error {
	info, err := conn.SelectRW(dest.Name)
	if err != nil {
		return err
	}
	uids := b.destUIDs
	if len(uids) == 0 || info.UIDValidity != b.uidValidity {
		uids = nil
		for _, mid := range b.messageIDs {
			found, err := conn.SearchHeader("Message-ID", mid)
			if err != nil {
				return err
			}
			for _, u := range found {
				if !slices.Contains(uids, u) {
					uids = append(uids, u)
				}
			}
		}
		slices.Sort(uids)
	}
	if len(uids) == 0 {
		return nil
	}
	return conn.Move(uids, src.Name)
}

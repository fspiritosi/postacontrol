package mail

import (
	"context"
	"strings"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
)

// serverSearchMax es el tope de resultados de una búsqueda en el servidor.
const serverSearchMax = 200

// Search busca en el índice local. Limit 0 devuelve 100. Pending cuenta los
// mensajes del alcance cuyo cuerpo todavía no está indexado (los resultados
// pueden estar incompletos hasta que el indexador los baje).
func (c *Core) Search(ctx context.Context, q model.SearchQuery) (model.SearchResult, error) {
	if q.Limit <= 0 {
		q.Limit = defaultPageSize
	}
	ms, err := c.store.Search(ctx, q)
	if err != nil {
		return model.SearchResult{}, c.ctxOrUserError(ctx, err)
	}
	pending, err := c.store.PendingBodies(ctx, q.FolderID)
	if err != nil {
		return model.SearchResult{}, c.ctxOrUserError(ctx, err)
	}
	return model.SearchResult{Messages: ms, Pending: pending}, nil
}

// SearchServer busca text en una carpeta con UID SEARCH TEXT en el servidor
// (sirve para cuerpos todavía sin indexar). Si hay UIDs que la caché no tiene,
// sincroniza la carpeta una vez. Devuelve hasta 200 mensajes por fecha
// descendente.
func (c *Core) SearchServer(ctx context.Context, folderID int64, text string) ([]model.MessageSummary, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, &UserError{Code: "invalid", Msg: "Escribí qué buscar."}
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	f, last, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	var uids []uint32
	err = c.withConn(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, false); err != nil {
			return err
		}
		var err error
		uids, err = conn.SearchText(text)
		return err
	})
	if err != nil {
		return nil, err
	}
	ms, err := c.store.MessagesByUIDs(ctx, folderID, uids)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	if len(ms) < len(uids) {
		if err := c.SyncFolder(ctx, folderID); err != nil {
			return nil, err
		}
		if ms, err = c.store.MessagesByUIDs(ctx, folderID, uids); err != nil {
			return nil, c.ctxOrUserError(ctx, err)
		}
	}
	if len(ms) > serverSearchMax {
		ms = ms[:serverSearchMax]
	}
	return ms, nil
}

// ListThreads devuelve los hilos de la carpeta, del más reciente al más
// antiguo. Limit 0 devuelve 100.
func (c *Core) ListThreads(ctx context.Context, folderID int64, q model.ListQuery) ([]model.ThreadSummary, error) {
	if q.Limit <= 0 {
		q.Limit = defaultPageSize
	}
	ts, err := c.store.ListThreads(ctx, folderID, q)
	return ts, c.ctxOrUserError(ctx, err)
}

// GetThread devuelve los mensajes del hilo (también los de Enviados), del más
// viejo al más nuevo.
func (c *Core) GetThread(ctx context.Context, folderID int64, threadRoot string) ([]model.MessageSummary, error) {
	ms, err := c.store.ThreadMessages(ctx, folderID, threadRoot)
	return ms, c.ctxOrUserError(ctx, err)
}

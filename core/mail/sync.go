package mail

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	headerBatch     = 200 // cabeceras por lote (una transacción por lote)
	flagsWindow     = 500 // mensajes más recientes cuyos flags se refrescan
	defaultPageSize = 100
)

// ListFolders devuelve las carpetas de la cuenta desde la caché.
func (c *Core) ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error) {
	fs, err := c.store.ListFolders(ctx, accountID)
	return fs, asUserError(err)
}

// ListMessages devuelve los mensajes de la carpeta desde la caché, por fecha
// descendente. Limit 0 devuelve 100.
func (c *Core) ListMessages(ctx context.Context, folderID int64, q model.ListQuery) ([]model.MessageSummary, error) {
	if q.Limit <= 0 {
		q.Limit = defaultPageSize
	}
	ms, err := c.store.ListMessages(ctx, folderID, q)
	return ms, asUserError(err)
}

// SyncAccount trae la lista de carpetas con sus contadores (LIST + STATUS),
// la guarda en la caché y sincroniza INBOX.
func (c *Core) SyncAccount(ctx context.Context, accountID int64) error {
	var remote []imapx.RemoteFolder
	err := c.withConn(ctx, accountID, func(conn *imapx.Conn) error {
		var err error
		remote, err = conn.ListFolders()
		return err
	})
	if err != nil {
		return err
	}
	folders := make([]model.Folder, 0, len(remote))
	counts := make(map[string]imapx.RemoteFolder, len(remote))
	for _, r := range remote {
		folders = append(folders, model.Folder{
			AccountID: accountID, Name: r.Name, Delimiter: r.Delimiter, Role: r.Role, Selectable: r.Selectable,
		})
		counts[r.Name] = r
	}
	if err := c.store.ReplaceFolders(ctx, accountID, folders); err != nil {
		return asUserError(err)
	}
	cached, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return asUserError(err)
	}
	var inboxID int64
	for _, f := range cached {
		r := counts[f.Name]
		if err := c.store.SetFolderCounts(ctx, f.ID, r.Unread, r.Total); err != nil {
			return asUserError(err)
		}
		if f.Role == "inbox" && f.Selectable {
			inboxID = f.ID
		}
	}
	c.emit(EventFoldersUpdated, map[string]any{"accountId": accountID})
	if inboxID == 0 {
		return nil
	}
	return c.SyncFolder(ctx, inboxID)
}

// SyncFolder sincroniza las cabeceras y los flags de una carpeta. Nunca
// escribe en el servidor: abre la carpeta en solo lectura (EXAMINE).
func (c *Core) SyncFolder(ctx context.Context, folderID int64) error {
	f, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return asUserError(err)
	}
	return c.withConn(ctx, f.AccountID, func(conn *imapx.Conn) error {
		return c.syncFolder(ctx, conn, folderID)
	})
}

// syncFolder es idempotente: FolderSync se escribe al final, así que si se
// corta a mitad de camino la próxima pasada vuelve a calcular lo que falta a
// partir de lo que ya está en la caché.
func (c *Core) syncFolder(ctx context.Context, conn *imapx.Conn, folderID int64) error {
	f, last, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return err
	}
	info, err := conn.Select(f.Name)
	if err != nil {
		return err
	}
	if last.UIDValidity != 0 && last.UIDValidity != info.UIDValidity {
		if err := c.store.ResetFolder(ctx, folderID); err != nil {
			return err
		}
	}
	remote, err := conn.AllUIDs()
	if err != nil {
		return err
	}
	if err := c.store.DeleteUIDsNotIn(ctx, folderID, remote); err != nil {
		return err
	}
	// Después de DeleteUIDsNotIn, known ⊆ remote.
	known, err := c.store.KnownUIDs(ctx, folderID)
	if err != nil {
		return err
	}

	knownSet := make(map[uint32]bool, len(known))
	for _, u := range known {
		knownSet[u] = true
	}
	fresh := []uint32{}
	for i := len(remote) - 1; i >= 0; i-- { // lo más nuevo primero
		if !knownSet[remote[i]] {
			fresh = append(fresh, remote[i])
		}
	}
	for done := 0; done < len(fresh); {
		if err := ctx.Err(); err != nil {
			return err
		}
		batch := fresh[done:min(done+headerBatch, len(fresh))]
		hs, err := conn.FetchHeaders(batch)
		if err != nil {
			return err
		}
		if err := c.store.UpsertHeaders(ctx, folderID, headerRows(hs)); err != nil {
			return err
		}
		done += len(batch)
		c.emit(EventSyncProgress, map[string]any{"folderId": folderID, "done": done, "total": len(fresh)})
	}

	unread, total, err := c.refreshFlags(ctx, conn, folderID, known[max(0, len(known)-flagsWindow):])
	if err != nil {
		return err
	}
	if err := c.store.SetFolderSync(ctx, folderID, store.FolderSync{UIDValidity: info.UIDValidity, UIDNext: info.UIDNext}); err != nil {
		return err
	}
	if err := c.store.SetFolderCounts(ctx, folderID, unread, total); err != nil {
		return err
	}
	c.emit(EventFolderUpdated, map[string]any{"folderId": folderID})
	return nil
}

// refreshFlags trae los flags de uids, guarda solo los que cambiaron y
// devuelve los contadores de la carpeta según la caché.
func (c *Core) refreshFlags(ctx context.Context, conn *imapx.Conn, folderID int64, uids []uint32) (unread, total int, err error) {
	if err := ctx.Err(); err != nil {
		return 0, 0, err
	}
	remote, err := conn.FetchFlags(uids)
	if err != nil {
		return 0, 0, err
	}
	cached, err := c.store.ListMessages(ctx, folderID, model.ListQuery{})
	if err != nil {
		return 0, 0, err
	}
	for i := range cached {
		m := &cached[i]
		flags, ok := remote[m.UID]
		if !ok {
			continue
		}
		seen, flagged, answered := hasFlag(flags, imap.FlagSeen), hasFlag(flags, imap.FlagFlagged), hasFlag(flags, imap.FlagAnswered)
		if seen != m.Seen || flagged != m.Flagged || answered != m.Answered {
			if err := c.store.SetFlags(ctx, folderID, m.UID, seen, flagged, answered); err != nil {
				return 0, 0, err
			}
			m.Seen = seen
		}
	}
	for _, m := range cached {
		if !m.Seen {
			unread++
		}
	}
	return unread, len(cached), nil
}

// StartBackgroundSync sincroniza todas las cuentas cada every, hasta que se
// cancele ctx o se cierre el núcleo. No bloquea.
func (c *Core) StartBackgroundSync(ctx context.Context, every time.Duration) {
	if every <= 0 {
		return
	}
	c.spawn(func(life context.Context) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer context.AfterFunc(life, cancel)()
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			accs, err := c.store.ListAccounts(ctx)
			if err != nil {
				continue
			}
			for _, a := range accs {
				if ctx.Err() != nil {
					return
				}
				// Los errores de conexión ya los avisa withConn con EventSyncError.
				_ = c.SyncAccount(ctx, a.ID)
			}
		}
	})
}

func headerRows(hs []imapx.RemoteHeader) []store.HeaderRow {
	rows := make([]store.HeaderRow, 0, len(hs))
	for _, h := range hs {
		r := store.HeaderRow{
			References: h.References,
			To:         []model.Address{},
			Cc:         []model.Address{},
			ReplyTo:    []model.Address{},
			Size:       h.Size,
		}
		r.UID = h.UID
		r.Date = h.Date
		r.HasAttachments = h.HasAttachments
		r.Seen = hasFlag(h.Flags, imap.FlagSeen)
		r.Flagged = hasFlag(h.Flags, imap.FlagFlagged)
		r.Answered = hasFlag(h.Flags, imap.FlagAnswered)
		if e := h.Envelope; e != nil {
			r.Subject = e.Subject
			r.MessageID = bracketMsgID(e.MessageID)
			if len(e.InReplyTo) > 0 {
				r.InReplyTo = bracketMsgID(e.InReplyTo[0])
			}
			if len(e.From) > 0 {
				r.From = toAddress(e.From[0])
			}
			r.To, r.Cc, r.ReplyTo = toAddresses(e.To), toAddresses(e.Cc), toAddresses(e.ReplyTo)
		}
		rows = append(rows, r)
	}
	// Dentro del lote, en orden ascendente: ante fechas iguales, la lista
	// desempata por ID de fila y así lo más nuevo queda arriba.
	slices.SortFunc(rows, func(a, b store.HeaderRow) int { return cmp.Compare(a.UID, b.UID) })
	return rows
}

// bracketMsgID devuelve el Message-ID con <>, como lo guardan References y
// como lo necesitan In-Reply-To y References al responder. go-imap lo entrega
// sin los <>.
func bracketMsgID(id string) string {
	if id == "" || strings.HasPrefix(id, "<") {
		return id
	}
	return "<" + id + ">"
}

func toAddress(a imap.Address) model.Address {
	return model.Address{Name: a.Name, Email: a.Addr()}
}

func toAddresses(as []imap.Address) []model.Address {
	out := make([]model.Address, 0, len(as))
	for _, a := range as {
		if a.IsGroupStart() || a.IsGroupEnd() {
			continue
		}
		out = append(out, toAddress(a))
	}
	return out
}

func hasFlag(flags []imap.Flag, want imap.Flag) bool {
	for _, f := range flags {
		if strings.EqualFold(string(f), string(want)) {
			return true
		}
	}
	return false
}

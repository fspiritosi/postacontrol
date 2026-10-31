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
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	var remote []imapx.RemoteFolder
	err := c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		var err error
		remote, err = conn.ListFolders()
		return err
	})
	if err != nil {
		return err
	}
	if err := c.ctxErr(ctx); err != nil {
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
		return c.ctxOrUserError(ctx, err)
	}
	cached, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	var inboxID int64
	for _, f := range cached {
		r := counts[f.Name]
		if err := c.store.SetFolderCounts(ctx, f.ID, r.Unread, r.Total); err != nil {
			return c.ctxOrUserError(ctx, err)
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
//
// La conexión se toma por tramos (preparar, cada lote de cabeceras, flags) y
// no durante toda la carpeta: entre dos lotes, leer o enviar no espera a que
// termine la primera sincronización de una carpeta grande. Dos pasadas sobre
// la misma carpeta no se intercalan: la segunda espera a la primera.
func (c *Core) SyncFolder(ctx context.Context, folderID int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	mu := c.folderSyncLock(folderID)
	mu.Lock()
	defer mu.Unlock()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	f, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	var p syncPlan
	err = c.withConn(ctx, f.AccountID, func(ctx context.Context, conn *imapx.Conn) error {
		var err error
		p, err = c.prepareSync(ctx, conn, folderID)
		return err
	})
	if err != nil {
		return err
	}
	// Cada lote vuelve a abrir la carpeta: entre dos lotes otra operación pudo
	// seleccionar otra. Si el UIDVALIDITY cambió en el medio, se corta con
	// "stale" y la próxima pasada resetea la caché.
	checked := store.FolderSync{UIDValidity: p.info.UIDValidity}
	for done := 0; done < len(p.fresh); {
		if err := c.ctxErr(ctx); err != nil {
			return err
		}
		batch := p.fresh[done:min(done+headerBatch, len(p.fresh))]
		err := c.withConn(ctx, f.AccountID, func(ctx context.Context, conn *imapx.Conn) error {
			if err := selectChecked(conn, p.folder, checked, false); err != nil {
				return err
			}
			hs, err := conn.FetchHeaders(batch)
			if err != nil {
				return err
			}
			return c.store.UpsertHeaders(ctx, folderID, headerRows(hs))
		})
		if err != nil {
			return err
		}
		done += len(batch)
		c.emit(EventSyncProgress, map[string]any{"folderId": folderID, "done": done, "total": len(p.fresh)})
	}
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	err = c.withConn(ctx, f.AccountID, func(ctx context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, p.folder, checked, false); err != nil {
			return err
		}
		unread, total, err := c.refreshFlags(ctx, conn, folderID, p.known[max(0, len(p.known)-flagsWindow):])
		if err != nil {
			return err
		}
		if err := c.store.SetFolderSync(ctx, folderID, store.FolderSync{UIDValidity: p.info.UIDValidity, UIDNext: p.info.UIDNext}); err != nil {
			return err
		}
		return c.store.SetFolderCounts(ctx, folderID, unread, total)
	})
	if err != nil {
		return err
	}
	c.emit(EventFolderUpdated, map[string]any{"folderId": folderID})
	c.announceNewMail(ctx, f.AccountID, p)
	return nil
}

// maxNewMailItems es cuántos correos detalla un aviso NewMail.
const maxNewMailItems = 3

// announceNewMail emite EventNewMail si la pasada fue sobre una Entrada que ya
// había completado una sincronización y trajo correo sin leer. Es un aviso: si
// falla leer la caché, no se emite y la sincronización sigue siendo exitosa.
func (c *Core) announceNewMail(ctx context.Context, accountID int64, p syncPlan) {
	if p.folder.Role != "inbox" || p.firstSync || len(p.fresh) == 0 {
		return
	}
	ms, err := c.store.MessagesByUIDs(ctx, p.folder.ID, p.fresh)
	if err != nil {
		return
	}
	ev := model.NewMail{AccountID: accountID, FolderID: p.folder.ID, Items: []model.NewMailItem{}}
	for _, m := range ms {
		if m.Seen {
			continue
		}
		ev.Count++
		if len(ev.Items) < maxNewMailItems {
			ev.Items = append(ev.Items, model.NewMailItem{
				MessageID: m.ID, FromName: m.From.Name, FromEmail: m.From.Email, Subject: m.Subject,
			})
		}
	}
	if ev.Count > 0 {
		c.emit(EventNewMail, ev)
	}
}

// syncPlan es lo que prepareSync calcula para los lotes y el refresco de flags.
type syncPlan struct {
	folder model.Folder
	info   imapx.SelectInfo
	known  []uint32 // UID en la caché antes de bajar cabeceras (⊆ remotos)
	fresh  []uint32 // UID que faltan, lo más nuevo primero
	// firstSync: la carpeta nunca completó una pasada (UIDNext en 0, R19).
	firstSync bool
}

// prepareSync abre la carpeta, resetea la caché si cambió el UIDVALIDITY,
// borra lo que ya no está en el servidor y calcula qué cabeceras faltan.
//
// La sincronización es idempotente: UIDNext se escribe recién al final, así
// que si se corta a mitad de camino la próxima pasada vuelve a calcular lo que
// falta a partir de lo que ya está en la caché. UIDVALIDITY, en cambio, se
// guarda apenas se abre la carpeta con UIDNext en 0 ("incompleta", R19): si
// cambia antes de completar la primera pasada, la siguiente lo detecta y
// resetea la caché.
func (c *Core) prepareSync(ctx context.Context, conn *imapx.Conn, folderID int64) (syncPlan, error) {
	f, last, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return syncPlan{}, err
	}
	info, err := conn.Select(f.Name)
	if err != nil {
		return syncPlan{}, err
	}
	firstSync := last.UIDNext == 0
	if last.UIDValidity != info.UIDValidity {
		if last.UIDValidity != 0 {
			firstSync = true // caché reseteada: lo que baja no es "nuevo"
			if err := c.store.ResetFolder(ctx, folderID); err != nil {
				return syncPlan{}, err
			}
		}
		if err := c.store.SetFolderSync(ctx, folderID, store.FolderSync{UIDValidity: info.UIDValidity}); err != nil {
			return syncPlan{}, err
		}
	}
	remote, err := conn.AllUIDs()
	if err != nil {
		return syncPlan{}, err
	}
	if err := c.store.DeleteUIDsNotIn(ctx, folderID, remote); err != nil {
		return syncPlan{}, err
	}
	// Después de DeleteUIDsNotIn, known ⊆ remote.
	known, err := c.store.KnownUIDs(ctx, folderID)
	if err != nil {
		return syncPlan{}, err
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
	return syncPlan{folder: f, info: info, known: known, fresh: fresh, firstSync: firstSync}, nil
}

// refreshFlags trae los flags de uids, guarda solo los que cambiaron y
// devuelve los contadores de la carpeta según la caché.
func (c *Core) refreshFlags(ctx context.Context, conn *imapx.Conn, folderID int64, uids []uint32) (unread, total int, err error) {
	if err := c.ctxErr(ctx); err != nil {
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
			c.backgroundPass(ctx)
		}
	})
}

// backgroundPass sincroniza todas las cuentas una vez, salteando las que están
// en pausa por una contraseña rechazada. Lo que pide el usuario no pasa por acá.
func (c *Core) backgroundPass(ctx context.Context) {
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return
	}
	for _, a := range accs {
		if ctx.Err() != nil {
			return
		}
		if c.authPaused(a.ID) {
			continue
		}
		// Los errores de conexión ya los avisa withConn con EventSyncError.
		_ = c.SyncAccount(ctx, a.ID)
	}
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

package mail

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

// Posponer: el correo va a la carpeta Pospuestos del servidor (así sale de la
// Entrada también en el webmail o el celular) y a la hora elegida el
// temporizador (scheduler.go) lo devuelve a su carpeta, como no leído. Se lo
// reconoce por su Message-ID: si alguien lo saca de Pospuestos a mano, el
// pospuesto se descarta sin tocar nada.

const (
	snoozeFolderName = "Pospuestos"

	msgSnoozePast  = "Elegí una hora que todavía no pasó."
	msgSnoozeNoID  = "Este correo no se puede posponer: no tiene identificador (Message-ID)."
	msgSnoozeGone  = "Ese correo ya no está pospuesto."
	msgSnoozeInbox = "Esta cuenta no tiene carpeta de Entrada."
)

// isSnoozeFolder dice si f es la carpeta Pospuestos (por su último nombre,
// con o sin el prefijo del servidor, como INBOX.Pospuestos).
func isSnoozeFolder(f model.Folder) bool {
	leaf := f.Name
	if f.Delimiter != "" {
		if i := strings.LastIndex(f.Name, f.Delimiter); i >= 0 {
			leaf = f.Name[i+len(f.Delimiter):]
		}
	}
	return f.Selectable && strings.EqualFold(leaf, snoozeFolderName)
}

func findSnoozeFolder(folders []model.Folder) (model.Folder, bool) {
	for _, f := range folders {
		if isSnoozeFolder(f) {
			return f, true
		}
	}
	return model.Folder{}, false
}

// snoozeFolder devuelve la carpeta Pospuestos de la cuenta y, si no tiene, la crea.
func (c *Core) snoozeFolder(ctx context.Context, accountID int64) (model.Folder, error) {
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
	}
	if f, ok := findSnoozeFolder(folders); ok {
		return f, nil
	}
	created, err := c.CreateFolder(ctx, accountID, 0, snoozeFolderName)
	var ue *UserError
	if errors.As(err, &ue) && ue.Code == "duplicate" {
		if folders, ferr := c.store.ListFolders(ctx, accountID); ferr == nil {
			if f, ok := findSnoozeFolder(folders); ok {
				return f, nil
			}
		}
	}
	return created, err
}

// SnoozeMessages pospone los mensajes hasta until (segundos unix). Devuelve con
// qué deshacer el movimiento (ver UndoMove).
func (c *Core) SnoozeMessages(ctx context.Context, ids []int64, until int64) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	if until <= time.Now().Unix() {
		return 0, &UserError{Code: "invalid", Msg: msgSnoozePast}
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	refs, err := c.store.MessageRefs(ctx, ids)
	if err != nil {
		return 0, c.ctxOrUserError(ctx, err)
	}
	for _, r := range refs {
		if r.MessageID == "" {
			return 0, &UserError{Code: "invalid", Msg: msgSnoozeNoID}
		}
	}
	accountID, byFolder, err := groupByFolder(refs)
	if err != nil {
		return 0, err
	}
	dest, err := c.snoozeFolder(ctx, accountID)
	if err != nil {
		return 0, err
	}
	batches, err := c.planMoves(ctx, byFolder, func(src model.Folder) (*model.Folder, bool) {
		return &dest, true
	})
	if err != nil {
		return 0, err
	}
	// Uno que ya está en Pospuestos solo cambia de hora: vuelve adonde iba.
	for _, b := range batches {
		for _, r := range b.refs {
			ret := b.src.Name
			if b.src.ID == dest.ID {
				if old, err := c.store.SnoozeFor(ctx, accountID, r.MessageID); err == nil {
					ret = old.ReturnFolder
				} else {
					ret = "INBOX"
				}
			}
			if err := c.store.InsertSnooze(ctx, model.Snooze{AccountID: accountID, MessageID: r.MessageID,
				ReturnFolder: ret, Until: until}); err != nil {
				return 0, c.ctxOrUserError(ctx, err)
			}
		}
	}
	moving := slices.DeleteFunc(batches, func(b moveBatch) bool { return b.src.ID == dest.ID })
	undo, err := c.runMoves(ctx, accountID, moving, true)
	if err != nil {
		return 0, err
	}
	c.emit(EventSnoozesChanged, nil)
	return undo, nil
}

// ListSnoozes devuelve los pospuestos, del que vuelve antes al último.
func (c *Core) ListSnoozes(ctx context.Context) ([]model.Snooze, error) {
	zs, err := c.store.ListSnoozes(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return zs, nil
}

// SnoozeFor devuelve cuándo vuelve el mensaje, o nil si no está pospuesto.
func (c *Core) SnoozeFor(ctx context.Context, messageID int64) (*model.Snooze, error) {
	refs, err := c.store.MessageRefs(ctx, []int64{messageID})
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	if refs[0].MessageID == "" {
		return nil, nil
	}
	z, err := c.store.SnoozeFor(ctx, refs[0].AccountID, refs[0].MessageID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return &z, nil
}

// Unsnooze devuelve ya el pospuesto snoozeID a su carpeta.
func (c *Core) Unsnooze(ctx context.Context, snoozeID int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	z, err := c.store.GetSnooze(ctx, snoozeID)
	if errors.Is(err, store.ErrNotFound) {
		return &UserError{Code: "notfound", Msg: msgSnoozeGone}
	}
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	return c.returnSnooze(ctx, z)
}

// returnDueSnoozes devuelve los pospuestos cuya hora llegó a now. Un error de
// una cuenta no corta a las demás; devuelve el primero.
func (c *Core) returnDueSnoozes(ctx context.Context, now time.Time) error {
	due, err := c.store.DueSnoozes(ctx, now.Unix())
	if err != nil {
		return err
	}
	var first error
	for _, z := range due {
		if err := c.ctxErr(ctx); err != nil {
			return err
		}
		if c.authPaused(z.AccountID) {
			continue // contraseña rechazada: se reintenta cuando vuelva
		}
		if err := c.returnSnooze(ctx, z); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// returnSnooze mueve el correo de Pospuestos a su carpeta, lo deja no leído
// y borra el pospuesto. Si ya no está en Pospuestos, solo borra el pospuesto.
func (c *Core) returnSnooze(ctx context.Context, z model.Snooze) error {
	folders, err := c.store.ListFolders(ctx, z.AccountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	from, ok := findSnoozeFolder(folders)
	if !ok {
		return c.forgetSnooze(ctx, z)
	}
	to, ok := cachedFolderByName(folders, z.ReturnFolder)
	if !ok || !to.Selectable {
		if to, ok = roleFolder(folders, "inbox"); !ok {
			return &UserError{Code: "invalid", Msg: msgSnoozeInbox}
		}
	}
	if err := c.withConn(ctx, z.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Noop()
	}); err != nil {
		return err
	}
	moved := false
	err = c.withConnNoRetry(ctx, z.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if _, err := conn.SelectRW(from.Name); err != nil {
			return err
		}
		uids, err := conn.SearchHeader("Message-ID", z.MessageID)
		if err != nil || len(uids) == 0 {
			return err
		}
		res, err := conn.MoveUIDs(uids, to.Name)
		if err != nil {
			return err
		}
		moved = true
		// Vuelve como no leído: es lo que se quería volver a ver.
		if _, err := conn.SelectRW(to.Name); err != nil {
			return err
		}
		back := res.Dest
		if len(back) == 0 {
			if back, err = conn.SearchHeader("Message-ID", z.MessageID); err != nil {
				return err
			}
		}
		return conn.StoreFlags(back, false, goimap.FlagSeen)
	})
	if err != nil && !moved {
		return err
	}
	if err != nil {
		slog.Warn("el pospuesto volvió pero no se pudo marcar no leído", "err", err)
	}
	if moved {
		for _, id := range []int64{from.ID, to.ID} {
			c.emit(EventFolderUpdated, map[string]any{"folderId": id})
			c.spawn(func(ctx context.Context) { _ = c.SyncFolder(ctx, id) })
		}
		c.emit(EventSnoozeReturned, map[string]any{"accountId": z.AccountID, "folderId": to.ID})
	}
	return c.forgetSnooze(ctx, z)
}

func (c *Core) forgetSnooze(ctx context.Context, z model.Snooze) error {
	if err := c.store.DeleteSnooze(ctx, z.ID); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventSnoozesChanged, nil)
	return nil
}

func roleFolder(folders []model.Folder, role model.FolderRole) (model.Folder, bool) {
	for _, f := range folders {
		if f.Role == role && f.Selectable {
			return f, true
		}
	}
	return model.Folder{}, false
}

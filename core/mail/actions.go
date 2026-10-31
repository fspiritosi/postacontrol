package mail

import (
	"context"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
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

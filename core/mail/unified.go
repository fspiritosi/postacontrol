package mail

import (
	"context"

	"postacontrol/core/model"
)

// viewRole traduce la vista al rol de carpeta que la alimenta. Los destacados
// se sincronizan desde las Entradas.
func viewRole(v model.UnifiedView) (model.FolderRole, bool) {
	switch v {
	case model.UnifiedInbox, model.UnifiedFlagged:
		return "inbox", true
	case model.UnifiedSent:
		return "sent", true
	case model.UnifiedDrafts:
		return "drafts", true
	case model.UnifiedTrash:
		return "trash", true
	}
	return "", false
}

var errBadView = &UserError{Code: "invalid", Msg: "Esa vista no existe."}

// ListUnified devuelve los mensajes de la vista desde la caché, por fecha
// descendente. Limit 0 devuelve 100.
func (c *Core) ListUnified(ctx context.Context, view model.UnifiedView, q model.ListQuery) ([]model.MessageSummary, error) {
	role, ok := viewRole(view)
	if !ok {
		return nil, errBadView
	}
	if q.Limit <= 0 {
		q.Limit = defaultPageSize
	}
	var ms []model.MessageSummary
	var err error
	if view == model.UnifiedFlagged {
		ms, err = c.store.ListFlagged(ctx, q)
	} else {
		ms, err = c.store.ListByRole(ctx, role, q)
	}
	return ms, asUserError(err)
}

// SyncUnified sincroniza, cuenta por cuenta, las carpetas de la vista. Si una
// cuenta falla sigue con la siguiente (withConn ya avisa el error); solo
// devuelve el error del contexto.
func (c *Core) SyncUnified(ctx context.Context, view model.UnifiedView) error {
	role, ok := viewRole(view)
	if !ok {
		return errBadView
	}
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	for _, a := range accs {
		fs, err := c.store.ListFolders(ctx, a.ID)
		if err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		for _, f := range fs {
			if f.Role != role {
				continue
			}
			if err := c.SyncFolder(ctx, f.ID); err != nil {
				if cerr := c.ctxErr(ctx); cerr != nil {
					return cerr
				}
			}
		}
	}
	return c.ctxErr(ctx)
}

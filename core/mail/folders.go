package mail

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	msgNameEmpty     = "Escribí un nombre para la carpeta."
	msgNameWildcard  = "El nombre no puede llevar «*» ni «%»."
	msgSpecial       = "Las carpetas especiales no se pueden cambiar."
	msgIntoItself    = "No se puede mover una carpeta dentro de sí misma."
	msgPartialRename = "La carpeta se renombró, pero algunas subcarpetas no; revisalas."
	msgNoSubfolders  = "Esta cuenta no admite subcarpetas."
)

// maxEmptyPasses es cuántas veces se intenta vaciar una carpeta antes de
// eliminarla: si sigue llegando correo, se corta con "stale".
const maxEmptyPasses = 3

// CreateFolder crea una carpeta en el servidor. parentFolderID 0 la deja en el
// primer nivel (con el prefijo personal de NAMESPACE, si el servidor lo
// informa). Queda suscripta; si la suscripción falla, la carpeta igual cuenta
// como creada.
func (c *Core) CreateFolder(ctx context.Context, accountID, parentFolderID int64, name string) (model.Folder, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return model.Folder{}, err
	}
	mu := c.folderListLock(accountID)
	mu.Lock()
	defer mu.Unlock()
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
	}
	delim, err := c.accountDelimiter(ctx, accountID, folders)
	if err != nil {
		return model.Folder{}, err
	}
	name, err = validFolderName(name, delim)
	if err != nil {
		return model.Folder{}, err
	}
	var prefix string
	if parentFolderID != 0 {
		parent, ok := cachedFolder(folders, parentFolderID)
		if !ok {
			return model.Folder{}, &UserError{Code: "invalid", Msg: msgBadDest}
		}
		if delim == "" {
			return model.Folder{}, &UserError{Code: "invalid", Msg: msgNoSubfolders}
		}
		prefix = parent.Name + delim
	} else if err := c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		var err error
		prefix, err = conn.PersonalPrefix()
		return err
	}); err != nil {
		return model.Folder{}, err
	}
	full := prefix + name
	if folderNameTaken(folders, full, 0) || (parentFolderID == 0 && strings.EqualFold(full, "INBOX")) {
		return model.Folder{}, &UserError{Code: "duplicate", Msg: dupMsg(name)}
	}
	pos, err := c.levelPosition(ctx, accountID, folders, full, delim, 0)
	if err != nil {
		return model.Folder{}, err
	}
	if err := c.probe(ctx, accountID); err != nil {
		return model.Folder{}, err
	}
	err = c.withConnNoRetry(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := conn.CreateMailbox(full); err != nil {
			return err
		}
		if err := conn.Subscribe(full); err != nil {
			slog.Warn("no se pudo suscribir la carpeta", "carpeta", full, "err", err)
		}
		return nil
	})
	if err != nil {
		return model.Folder{}, err
	}
	if _, err := c.refreshFoldersLocked(ctx, accountID); err != nil {
		return model.Folder{}, err
	}
	created, err := c.folderByName(ctx, accountID, full)
	if err != nil {
		return model.Folder{}, err
	}
	if pos > 0 {
		if err := c.store.SetFolderPosition(ctx, created.ID, pos); err != nil {
			return model.Folder{}, c.ctxOrUserError(ctx, err)
		}
		created.Position = pos
		c.emit(EventFoldersUpdated, map[string]any{"accountId": accountID})
	}
	return created, nil
}

// RenameFolder cambia solo el último segmento del nombre de la carpeta.
func (c *Core) RenameFolder(ctx context.Context, folderID int64, name string) (model.Folder, error) {
	return c.relocateFolder(ctx, folderID, false, func(_ context.Context, f model.Folder, delim string, _ []model.Folder) (string, error) {
		name, err := validFolderName(name, delim)
		if err != nil {
			return "", err
		}
		return parentOf(f.Name, delim) + name, nil
	})
}

// MoveFolder mueve la carpeta (con todo su subárbol) bajo otro padre de la
// misma cuenta; 0 es el primer nivel. Conserva el último segmento del nombre.
// Toma la posición del nivel de destino (ver levelPosition).
func (c *Core) MoveFolder(ctx context.Context, folderID, newParentFolderID int64) (model.Folder, error) {
	return c.relocateFolder(ctx, folderID, true, func(ctx context.Context, f model.Folder, delim string, folders []model.Folder) (string, error) {
		last := f.Name[len(parentOf(f.Name, delim)):]
		if newParentFolderID == 0 {
			var prefix string
			if err := c.withConn(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
				var err error
				prefix, err = conn.PersonalPrefix()
				return err
			}); err != nil {
				return "", err
			}
			return prefix + last, nil
		}
		parent, ok := cachedFolder(folders, newParentFolderID)
		if !ok || parent.AccountID != f.AccountID {
			return "", &UserError{Code: "invalid", Msg: msgBadDest}
		}
		if delim == "" {
			return "", &UserError{Code: "invalid", Msg: msgNoSubfolders}
		}
		if parent.ID == f.ID || strings.HasPrefix(parent.Name, f.Name+delim) {
			return "", &UserError{Code: "invalid", Msg: msgIntoItself}
		}
		return parent.Name + delim + last, nil
	})
}

// relocateFolder es el cuerpo común de RenameFolder y MoveFolder: target arma
// el nombre completo nuevo (y valida); acá se valida lo especial y los
// duplicados, se renombra en el servidor y después en la caché. Con
// reposition (al cambiar de nivel) la carpeta toma la posición del nivel
// nuevo; si no, conserva la suya.
func (c *Core) relocateFolder(ctx context.Context, folderID int64, reposition bool,
	target func(ctx context.Context, f model.Folder, delim string, folders []model.Folder) (string, error)) (model.Folder, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return model.Folder{}, err
	}
	unlock, err := c.lockFolderAccount(ctx, folderID)
	if err != nil {
		return model.Folder{}, err
	}
	defer unlock()
	f, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
	}
	folders, err := c.store.ListFolders(ctx, f.AccountID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
	}
	delim := f.Delimiter
	if delim == "" {
		if delim, err = c.accountDelimiter(ctx, f.AccountID, folders); err != nil {
			return model.Folder{}, err
		}
	}
	if hasSpecial(folders, f.Name, delim) {
		return model.Folder{}, &UserError{Code: "invalid", Msg: msgSpecial}
	}
	newName, err := target(ctx, f, delim, folders)
	if err != nil {
		return model.Folder{}, err
	}
	if newName == f.Name {
		return f, nil
	}
	if folderNameTaken(folders, newName, f.ID) {
		return model.Folder{}, &UserError{Code: "duplicate", Msg: dupMsg(newName[len(parentOf(newName, delim)):])}
	}
	pos := f.Position
	if reposition {
		if pos, err = c.levelPosition(ctx, f.AccountID, folders, newName, delim, f.ID); err != nil {
			return model.Folder{}, err
		}
	}
	// Nombres viejos del subárbol que tiene la caché: hay que re-suscribirlos.
	var oldNames []string
	for _, g := range folders {
		if g.Name == f.Name || (delim != "" && strings.HasPrefix(g.Name, f.Name+delim)) {
			oldNames = append(oldNames, g.Name)
		}
	}

	if err := c.probe(ctx, f.AccountID); err != nil {
		return model.Folder{}, err
	}
	var renamed [][2]string // viejo, nuevo
	// partial guarda el primer error posterior al RENAME de la carpeta misma:
	// a partir de ahí el servidor ya cambió, así que la caché tiene que seguirlo
	// igual (si no, el próximo SyncAccount borraría la fila con sus correos).
	var partial error
	err = c.withConnNoRetry(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := conn.RenameMailbox(f.Name, newName); err != nil {
			return err
		}
		renamed = append(renamed, [2]string{f.Name, newName})
		// Algunos servidores (el de pruebas) no renombran las inferiores: si
		// quedaron colgando del nombre viejo, se renombran una por una, de la
		// menos profunda a la más profunda. Cada nombre nuevo sale del viejo
		// cambiando el prefijo, así que el orden no los pisa. En un servidor
		// que sí las renombra la lista vuelve vacía.
		rest, err := conn.ListUnder(f.Name, delim)
		if err != nil {
			partial = err
			return nil
		}
		slices.SortStableFunc(rest, func(a, b imapx.RemoteFolder) int {
			return strings.Count(a.Name, delim) - strings.Count(b.Name, delim)
		})
		for _, r := range rest {
			to := newName + delim + r.Name[len(f.Name+delim):]
			if err := conn.RenameMailbox(r.Name, to); err != nil {
				partial = err
				return nil
			}
			renamed = append(renamed, [2]string{r.Name, to})
		}
		return nil
	})
	if err != nil {
		return model.Folder{}, err
	}
	if err := c.store.RenameFolderTree(ctx, f.AccountID, f.Name, newName, delim); err != nil {
		partial = cmp.Or(partial, err)
	}
	if partial != nil {
		slog.Warn("el renombrado quedó a medias", "carpeta", f.Name, "err", partial)
		// Lo que quedó sin renombrar lo corrige el refresco desde el servidor.
		_, _ = c.refreshFoldersLocked(ctx, f.AccountID)
		return model.Folder{}, &UserError{Code: "server", Msg: msgPartialRename, Err: partial}
	}
	// Suscripción: nombres nuevos sí, viejos no. Si falla, solo se registra.
	for _, old := range oldNames {
		if !slices.ContainsFunc(renamed, func(r [2]string) bool { return r[0] == old }) {
			renamed = append(renamed, [2]string{old, newName + old[len(f.Name):]})
		}
	}
	if err := c.withConn(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		for _, r := range renamed {
			if err := conn.Unsubscribe(r[0]); err != nil {
				slog.Warn("no se pudo quitar la suscripción", "carpeta", r[0], "err", err)
			}
			if err := conn.Subscribe(r[1]); err != nil {
				slog.Warn("no se pudo suscribir la carpeta", "carpeta", r[1], "err", err)
			}
		}
		return nil
	}); err != nil {
		slog.Warn("no se pudo actualizar la suscripción", "carpeta", newName, "err", err)
	}
	if _, err := c.refreshFoldersLocked(ctx, f.AccountID); err != nil {
		return model.Folder{}, err
	}
	out, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
	}
	if out.Position != pos {
		if err := c.store.SetFolderPosition(ctx, out.ID, pos); err != nil {
			return model.Folder{}, c.ctxOrUserError(ctx, err)
		}
		out.Position = pos
		c.emit(EventFoldersUpdated, map[string]any{"accountId": f.AccountID})
	}
	return out, nil
}

// mailboxTotal es conn.MailboxTotal, pero si el servidor no informa la
// cantidad devuelve el error para el usuario (con el técnico adentro, para el
// registro).
func mailboxTotal(conn *imapx.Conn, name, delim string) (int, error) {
	n, err := conn.MailboxTotal(name)
	if errors.Is(err, imapx.ErrNoMessageCount) {
		return 0, &UserError{Code: "server", Err: err,
			Msg: "El servidor no informó cuántos correos tiene «" + name[len(parentOf(name, delim)):] + "»; no se borró nada."}
	}
	return n, err
}

// subtreeFolder es una carpeta del subárbol a eliminar, según el servidor, con
// su cantidad de mensajes (0 si es \Noselect).
type subtreeFolder struct {
	imapx.RemoteFolder
	total int
}

// DeleteFolderPreview cuenta en el servidor (no en la caché) los correos de la
// carpeta y de cada subcarpeta, para el aviso antes de eliminar. No escribe
// nada.
func (c *Core) DeleteFolderPreview(ctx context.Context, folderID int64) (model.FolderDeleteInfo, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	f, folders, delim, err := c.deletableFolder(ctx, folderID)
	if err != nil {
		return model.FolderDeleteInfo{}, err
	}
	tree, err := c.serverSubtree(ctx, f, delim, folders)
	if err != nil {
		return model.FolderDeleteInfo{}, err
	}
	info := model.FolderDeleteInfo{Subfolders: []model.SubfolderCount{}}
	for _, n := range tree {
		if n.Name == f.Name {
			info.Total = n.total
			continue
		}
		info.Subfolders = append(info.Subfolders, model.SubfolderCount{Name: n.Name[len(f.Name+delim):], Total: n.total})
	}
	slices.SortFunc(info.Subfolders, func(a, b model.SubfolderCount) int { return strings.Compare(a.Name, b.Name) })
	return info, nil
}

// DeleteFolder elimina la carpeta con todo su subárbol. Primero manda a la
// Papelera todos los correos según el servidor (UID SEARCH ALL, también los
// que la caché no tiene) y borra cada carpeta recién cuando STATUS dice que
// quedó vacía, de la más profunda a la menos. Ante cualquier error corta ahí:
// esa carpeta y sus padres quedan en el servidor; las más profundas que ya se
// borraron tenían sus correos en la Papelera. Devuelve cuántos correos movió.
func (c *Core) DeleteFolder(ctx context.Context, folderID int64) (moved int, err error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	unlock, err := c.lockFolderAccount(ctx, folderID)
	if err != nil {
		return 0, err
	}
	defer unlock()
	f, folders, delim, err := c.deletableFolder(ctx, folderID)
	if err != nil {
		return 0, err
	}
	tree, err := c.serverSubtree(ctx, f, delim, folders)
	if err != nil {
		return 0, err
	}
	var trash *model.Folder
	for i := range folders {
		if folders[i].Role == "trash" && folders[i].Selectable {
			trash = &folders[i]
			break
		}
	}
	// Sin Papelera no se toca nada si hay algún correo, ni las subcarpetas
	// vacías.
	if trash == nil && slices.ContainsFunc(tree, func(n subtreeFolder) bool { return n.total > 0 }) {
		return 0, &UserError{Code: "no_trash", Msg: msgNoTrash}
	}
	// De la más profunda a la menos: un padre se borra recién sin hijos.
	slices.SortStableFunc(tree, func(a, b subtreeFolder) int {
		return cmp.Or(strings.Count(b.Name, delim)-strings.Count(a.Name, delim), strings.Compare(b.Name, a.Name))
	})

	// Al terminar, o al cortar, la caché sigue al servidor y la Papelera se
	// pone al día, como en runMoves.
	defer func() {
		if _, err := c.refreshFoldersLocked(ctx, f.AccountID); err != nil {
			slog.Warn("no se pudo refrescar las carpetas después de eliminar", "carpeta", f.Name, "err", err)
		}
		if moved > 0 && trash != nil {
			trashID := trash.ID
			c.emit(EventFolderUpdated, map[string]any{"folderId": trashID})
			c.spawn(func(ctx context.Context) { _ = c.SyncFolder(ctx, trashID) })
		}
	}()
	for _, n := range tree {
		got, err := c.emptyAndDelete(ctx, f.AccountID, n.RemoteFolder, trash, delim)
		moved += got
		if err != nil {
			return moved, err
		}
		if g, ok := cachedFolderByName(folders, n.Name); ok {
			if err := c.store.DeleteFolderRow(ctx, g.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
				return moved, c.ctxOrUserError(ctx, err)
			}
		}
	}
	return moved, nil
}

// emptyAndDelete vacía una carpeta en el servidor mandando todo a la Papelera
// (hasta maxEmptyPasses pasadas) y la borra recién cuando STATUS dice que
// quedó vacía. Una \Noselect no puede tener correos: si el servidor la sigue
// informando así, se borra sin abrirla.
// Devuelve cuántos correos movió, aunque después falle.
func (c *Core) emptyAndDelete(ctx context.Context, accountID int64, r imapx.RemoteFolder, trash *model.Folder, delim string) (moved int, err error) {
	if err := c.probe(ctx, accountID); err != nil {
		return 0, err
	}
	err = c.withConnNoRetry(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		selectable := r.Selectable
		if !selectable {
			// Lo \Noselect salió del LIST del principio y entre tanto pudo
			// pasar a ser una carpeta de verdad con correo (otro cliente la
			// creó, o Sieve con fileinto :create): se vuelve a mirar en la
			// misma sesión, justo antes de borrarla.
			now, ok, err := conn.Lookup(r.Name)
			if err != nil {
				return err
			}
			if !ok {
				// Desapareció sola al borrar sus hijos (Dovecot): ya está.
				if err := conn.Unsubscribe(r.Name); err != nil {
					slog.Warn("no se pudo quitar la suscripción", "carpeta", r.Name, "err", err)
				}
				return nil
			}
			selectable = now.Selectable
		}
		for pass := 1; selectable; pass++ {
			if _, err := conn.SelectRW(r.Name); err != nil {
				return err
			}
			uids, err := conn.AllUIDs()
			if err != nil {
				return err
			}
			if len(uids) > 0 {
				if trash == nil {
					return &UserError{Code: "no_trash", Msg: msgNoTrash}
				}
				if err := conn.Move(uids, trash.Name); err != nil {
					return err
				}
				moved += len(uids)
			}
			// Se sale de la carpeta antes de contar y de borrarla: STATUS no
			// debería usarse sobre la carpeta abierta (RFC 3501) y Dovecot
			// corta la sesión si se borra la carpeta abierta. EXAMINE no
			// cambia nada.
			if _, err := conn.Select("INBOX"); err != nil {
				return err
			}
			total, err := mailboxTotal(conn, r.Name, delim)
			if err != nil {
				return err
			}
			if total == 0 {
				break
			}
			if pass == maxEmptyPasses {
				return &UserError{Code: "stale", Msg: staleMsg(r.Name[len(parentOf(r.Name, delim)):])}
			}
		}
		if err := conn.DeleteMailbox(r.Name); err != nil {
			// Una \Noselect que existía solo por sus hijos puede desaparecer
			// sola al borrarlos (Dovecot): ya está.
			var ierr *goimap.Error
			if selectable || !errors.As(err, &ierr) || ierr.Code != goimap.ResponseCodeNonExistent {
				return err
			}
		}
		if err := conn.Unsubscribe(r.Name); err != nil {
			slog.Warn("no se pudo quitar la suscripción", "carpeta", r.Name, "err", err)
		}
		return nil
	})
	return moved, err
}

// deletableFolder busca la carpeta en la caché y rechaza la especial y la que
// tiene una especial en su subárbol. Devuelve también las carpetas de la
// cuenta y el delimitador.
func (c *Core) deletableFolder(ctx context.Context, folderID int64) (model.Folder, []model.Folder, string, error) {
	if err := c.ctxErr(ctx); err != nil {
		return model.Folder{}, nil, "", err
	}
	f, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return model.Folder{}, nil, "", c.ctxOrUserError(ctx, err)
	}
	folders, err := c.store.ListFolders(ctx, f.AccountID)
	if err != nil {
		return model.Folder{}, nil, "", c.ctxOrUserError(ctx, err)
	}
	delim := f.Delimiter
	if delim == "" {
		if delim, err = c.accountDelimiter(ctx, f.AccountID, folders); err != nil {
			return model.Folder{}, nil, "", err
		}
	}
	if hasSpecial(folders, f.Name, delim) {
		return model.Folder{}, nil, "", &UserError{Code: "invalid", Msg: msgSpecial}
	}
	return f, folders, delim, nil
}

// serverSubtree lista en el servidor la carpeta y lo que cuelga de ella, y
// cuenta los mensajes de cada una con STATUS. Si el servidor no la tiene
// devuelve not_found; si algo del subárbol es especial (según el servidor o
// la caché), msgSpecial.
func (c *Core) serverSubtree(ctx context.Context, f model.Folder, delim string, folders []model.Folder) ([]subtreeFolder, error) {
	var tree []subtreeFolder
	var found bool
	err := c.withConn(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		tree = nil
		self, ok, err := conn.Lookup(f.Name)
		found = ok
		if err != nil || !ok {
			return err
		}
		remote := []imapx.RemoteFolder{self}
		// Sin delimitador no hay subcarpetas (y el patrón de ListUnder
		// agarraría todo lo que empiece igual).
		if delim != "" {
			under, err := conn.ListUnder(f.Name, delim)
			if err != nil {
				return err
			}
			remote = append(remote, under...)
		}
		for _, r := range remote {
			n := subtreeFolder{RemoteFolder: r}
			if r.Selectable {
				if n.total, err = mailboxTotal(conn, r.Name, delim); err != nil {
					return err
				}
			}
			tree = append(tree, n)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, &UserError{Code: "not_found", Msg: msgNotFound}
	}
	for _, n := range tree {
		if n.SpecialUse || strings.EqualFold(n.Name, "INBOX") || slices.ContainsFunc(folders, func(g model.Folder) bool {
			return g.Role != "" && g.Name == n.Name
		}) {
			return nil, &UserError{Code: "invalid", Msg: msgSpecial}
		}
	}
	return tree, nil
}

// SetFolderOrder guarda el orden manual: la carpeta i queda con posición i+1.
// Todas las carpetas tienen que ser de la cuenta. Las especiales también se
// pueden reordenar.
func (c *Core) SetFolderOrder(ctx context.Context, accountID int64, folderIDs []int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	mu := c.folderListLock(accountID)
	mu.Lock()
	defer mu.Unlock()
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	for _, id := range folderIDs {
		if _, ok := cachedFolder(folders, id); !ok {
			return &UserError{Code: "invalid", Msg: msgBadDest}
		}
	}
	if err := c.store.SetFolderPositions(ctx, accountID, folderIDs); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventFoldersUpdated, map[string]any{"accountId": accountID})
	return nil
}

// lockFolderAccount toma el lock de la lista de carpetas de la cuenta de la
// carpeta y devuelve cómo soltarlo. Quien lo llama vuelve a leer la carpeta
// después: un refresco pudo cambiarla mientras esperaba.
func (c *Core) lockFolderAccount(ctx context.Context, folderID int64) (func(), error) {
	if err := c.ctxErr(ctx); err != nil {
		return nil, err
	}
	f, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	mu := c.folderListLock(f.AccountID)
	mu.Lock()
	return mu.Unlock, nil
}

// levelPosition es la posición de una carpeta que llega al nivel de name
// (creada o movida): si alguna hermana ya tiene orden manual va al final
// (MAX+1 de la cuenta); si no, 0 y se ordena como siempre. Las hermanas son
// las que la barra lateral muestra en ese nivel: las del mismo padre sin rol
// y, en el primer nivel, también las especiales, que se ven siempre ahí.
// exceptID es la carpeta que se mueve (0 al crear).
func (c *Core) levelPosition(ctx context.Context, accountID int64, folders []model.Folder, name, delim string, exceptID int64) (int, error) {
	parent := parentOf(name, delim)
	ordered := slices.ContainsFunc(folders, func(g model.Folder) bool {
		if g.ID == exceptID || g.Position == 0 {
			return false
		}
		if parent == "" {
			return g.Role != "" || parentOf(g.Name, delim) == ""
		}
		return g.Role == "" && parentOf(g.Name, delim) == parent
	})
	if !ordered {
		return 0, nil
	}
	pos, err := c.store.NextFolderPosition(ctx, accountID)
	if err != nil {
		return 0, c.ctxOrUserError(ctx, err)
	}
	return pos, nil
}

// probe valida la conexión con NOOP (con reintento) antes de una escritura
// que no se puede reintentar, como en applyMove.
func (c *Core) probe(ctx context.Context, accountID int64) error {
	return c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		return conn.Noop()
	})
}

// accountDelimiter es el delimitador de las carpetas de la caché; si no hay
// ninguna, el del LIST.
func (c *Core) accountDelimiter(ctx context.Context, accountID int64, folders []model.Folder) (string, error) {
	for _, f := range folders {
		if f.Delimiter != "" {
			return f.Delimiter, nil
		}
	}
	var remote []imapx.RemoteFolder
	err := c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		var err error
		remote, err = conn.ListFolders()
		return err
	})
	if err != nil {
		return "", err
	}
	for _, r := range remote {
		if r.Delimiter != "" {
			return r.Delimiter, nil
		}
	}
	return "", nil
}

func (c *Core) folderByName(ctx context.Context, accountID int64, name string) (model.Folder, error) {
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
	}
	for _, f := range folders {
		if f.Name == name {
			return f, nil
		}
	}
	return model.Folder{}, &UserError{Code: "not_found", Msg: msgNotFound}
}

// validFolderName recorta los espacios de los bordes y rechaza el nombre
// vacío, el que lleva el delimitador y el que lleva comodines de LIST.
func validFolderName(name, delim string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", &UserError{Code: "invalid", Msg: msgNameEmpty}
	}
	if delim != "" && strings.Contains(name, delim) {
		return "", &UserError{Code: "invalid", Msg: "El nombre no puede llevar «" + delim + "»."}
	}
	// «*» y «%» son comodines de LIST y ListUnder no los escapa.
	if strings.ContainsAny(name, "*%") {
		return "", &UserError{Code: "invalid", Msg: msgNameWildcard}
	}
	return name, nil
}

func dupMsg(name string) string { return "Ya hay una carpeta «" + name + "» ahí." }

func staleMsg(name string) string {
	return "Llegaron correos nuevos a «" + name + "» mientras se vaciaba; probá de nuevo."
}

func cachedFolder(folders []model.Folder, id int64) (model.Folder, bool) {
	for _, f := range folders {
		if f.ID == id {
			return f, true
		}
	}
	return model.Folder{}, false
}

func cachedFolderByName(folders []model.Folder, name string) (model.Folder, bool) {
	for _, f := range folders {
		if f.Name == name {
			return f, true
		}
	}
	return model.Folder{}, false
}

// folderNameTaken dice si otra carpeta (que no sea exceptID) ya tiene ese
// nombre completo, sin distinguir mayúsculas.
func folderNameTaken(folders []model.Folder, full string, exceptID int64) bool {
	return slices.ContainsFunc(folders, func(f model.Folder) bool {
		return f.ID != exceptID && strings.EqualFold(f.Name, full)
	})
}

// parentOf devuelve el nombre sin el último segmento, con su delimitador al
// final ("" si está en el primer nivel).
func parentOf(name, delim string) string {
	if delim == "" {
		return ""
	}
	i := strings.LastIndex(name, delim)
	if i < 0 {
		return ""
	}
	return name[:i+len(delim)]
}

// hasSpecial dice si la carpeta o algo de su subárbol (por nombre) tiene rol.
func hasSpecial(folders []model.Folder, name, delim string) bool {
	return slices.ContainsFunc(folders, func(g model.Folder) bool {
		return g.Role != "" && (g.Name == name || (delim != "" && strings.HasPrefix(g.Name, name+delim)))
	})
}

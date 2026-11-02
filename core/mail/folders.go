package mail

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
)

const (
	msgNameEmpty    = "Escribí un nombre para la carpeta."
	msgNameWildcard = "El nombre no puede llevar «*» ni «%»."
	msgSpecial      = "Las carpetas especiales no se pueden cambiar."
	msgIntoItself   = "No se puede mover una carpeta dentro de sí misma."
	msgNoSubfolders = "Esta cuenta no admite subcarpetas."
)

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
	pos, err := c.store.NextFolderPosition(ctx, accountID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
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
	if _, err := c.refreshFolders(ctx, accountID); err != nil {
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
	return c.relocateFolder(ctx, folderID, func(_ context.Context, f model.Folder, delim string, _ []model.Folder) (string, error) {
		name, err := validFolderName(name, delim)
		if err != nil {
			return "", err
		}
		return parentOf(f.Name, delim) + name, nil
	})
}

// MoveFolder mueve la carpeta (con todo su subárbol) bajo otro padre de la
// misma cuenta; 0 es el primer nivel. Conserva el último segmento del nombre.
func (c *Core) MoveFolder(ctx context.Context, folderID, newParentFolderID int64) (model.Folder, error) {
	return c.relocateFolder(ctx, folderID, func(ctx context.Context, f model.Folder, delim string, folders []model.Folder) (string, error) {
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
// duplicados, se renombra en el servidor y después en la caché.
func (c *Core) relocateFolder(ctx context.Context, folderID int64,
	target func(ctx context.Context, f model.Folder, delim string, folders []model.Folder) (string, error)) (model.Folder, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return model.Folder{}, err
	}
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
			return err
		}
		slices.SortStableFunc(rest, func(a, b imapx.RemoteFolder) int {
			return strings.Count(a.Name, delim) - strings.Count(b.Name, delim)
		})
		for _, r := range rest {
			to := newName + delim + r.Name[len(f.Name+delim):]
			if err := conn.RenameMailbox(r.Name, to); err != nil {
				return err
			}
			renamed = append(renamed, [2]string{r.Name, to})
		}
		return nil
	})
	if err != nil {
		return model.Folder{}, err
	}
	if err := c.store.RenameFolderTree(ctx, f.AccountID, f.Name, newName, delim); err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
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
	if _, err := c.refreshFolders(ctx, f.AccountID); err != nil {
		return model.Folder{}, err
	}
	out, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return model.Folder{}, c.ctxOrUserError(ctx, err)
	}
	return out, nil
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

func cachedFolder(folders []model.Folder, id int64) (model.Folder, bool) {
	for _, f := range folders {
		if f.ID == id {
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

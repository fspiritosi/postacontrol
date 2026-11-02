package mail

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"slices"
	"strings"

	goimap "github.com/emersion/go-imap/v2"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	maxLabels       = 20
	maxLabelNameLen = 40

	msgLabelNoName  = "Poné un nombre para la etiqueta."
	msgLabelDup     = "Ya hay una etiqueta con ese nombre."
	msgLabelColor   = "Ese color no es válido."
	msgLabelMax     = "Ya tenés 20 etiquetas, que es el máximo."
	msgLabelMissing = "Esa etiqueta ya no existe."
)

var labelColorRE = regexp.MustCompile(`^#[0-9a-f]{6}$`)

// ListLabels devuelve las etiquetas en el orden elegido.
func (c *Core) ListLabels(ctx context.Context) ([]model.Label, error) {
	list, err := c.store.ListLabels(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return list, nil
}

// cleanLabel recorta y valida nombre y color. others son las demás etiquetas
// (sin la que se edita) contra las que se compara el nombre.
func cleanLabel(name, color string, others []model.Label) (string, error) {
	name = strings.TrimSpace(name)
	if r := []rune(name); len(r) > maxLabelNameLen {
		name = strings.TrimSpace(string(r[:maxLabelNameLen]))
	}
	if name == "" {
		return "", &UserError{Code: "invalid", Msg: msgLabelNoName}
	}
	if !labelColorRE.MatchString(color) {
		return "", &UserError{Code: "invalid", Msg: msgLabelColor}
	}
	for _, o := range others {
		if strings.EqualFold(o.Name, name) {
			return "", &UserError{Code: "invalid", Msg: msgLabelDup}
		}
	}
	return name, nil
}

// newLabelKeyword genera "$pc_" + 8 hex minúsculos al azar que no use ninguna
// etiqueta existente.
func newLabelKeyword(taken map[string]int64) (string, error) {
	for {
		var b [4]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		kw := "$pc_" + hex.EncodeToString(b[:])
		if _, used := taken[kw]; !used {
			return kw, nil
		}
	}
}

// CreateLabel crea una etiqueta al final del orden. No escribe en el servidor.
func (c *Core) CreateLabel(ctx context.Context, name, color string) (model.Label, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	existing, err := c.store.ListLabels(ctx)
	if err != nil {
		return model.Label{}, c.ctxOrUserError(ctx, err)
	}
	name, err = cleanLabel(name, color, existing)
	if err != nil {
		return model.Label{}, err
	}
	if len(existing) >= maxLabels {
		return model.Label{}, &UserError{Code: "invalid", Msg: msgLabelMax}
	}
	taken, err := c.store.LabelByKeyword(ctx)
	if err != nil {
		return model.Label{}, c.ctxOrUserError(ctx, err)
	}
	kw, err := newLabelKeyword(taken)
	if err != nil {
		return model.Label{}, c.ctxOrUserError(ctx, err)
	}
	l, err := c.store.InsertLabel(ctx, name, color, kw)
	if err != nil {
		return model.Label{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventLabelsChanged, nil)
	return l, nil
}

// UpdateLabel cambia nombre y color. El keyword no cambia y el servidor no se
// toca: renombrar es un asunto local.
func (c *Core) UpdateLabel(ctx context.Context, l model.Label) (model.Label, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	all, err := c.store.ListLabels(ctx)
	if err != nil {
		return model.Label{}, c.ctxOrUserError(ctx, err)
	}
	var cur *model.Label
	others := make([]model.Label, 0, len(all))
	for i := range all {
		if all[i].ID == l.ID {
			cur = &all[i]
		} else {
			others = append(others, all[i])
		}
	}
	if cur == nil {
		return model.Label{}, &UserError{Code: "notfound", Msg: msgLabelMissing}
	}
	name, err := cleanLabel(l.Name, l.Color, others)
	if err != nil {
		return model.Label{}, err
	}
	updated := model.Label{ID: cur.ID, Name: name, Color: l.Color, Position: cur.Position}
	if err := c.store.UpdateLabel(ctx, updated); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Label{}, &UserError{Code: "notfound", Msg: msgLabelMissing}
		}
		return model.Label{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventLabelsChanged, nil)
	return updated, nil
}

// ReorderLabels fija el orden. ids tiene que ser exactamente el conjunto de
// etiquetas que existen, cada una una vez.
func (c *Core) ReorderLabels(ctx context.Context, ids []int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	all, err := c.store.ListLabels(ctx)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	have := make([]int64, len(all))
	for i, l := range all {
		have[i] = l.ID
	}
	want := slices.Clone(ids)
	slices.Sort(have)
	slices.Sort(want)
	if !slices.Equal(have, want) {
		return &UserError{Code: "invalid", Msg: "La lista de etiquetas cambió. Volvé a abrirla."}
	}
	if err := c.store.SetLabelPositions(ctx, ids); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventLabelsChanged, nil)
	return nil
}

// LabelUsage dice en cuántos correos está puesta la etiqueta.
func (c *Core) LabelUsage(ctx context.Context, id int64) (int, error) {
	n, err := c.store.CountLabeled(ctx, id)
	if err != nil {
		return 0, c.ctxOrUserError(ctx, err)
	}
	return n, nil
}

// DeleteLabel borra la etiqueta y anota que hay que sacar su keyword de los
// correos del servidor de cada cuenta (menos las que guardan las etiquetas solo
// acá). La limpieza corre en segundo plano; si no hay conexión queda pendiente
// para la próxima pasada.
func (c *Core) DeleteLabel(ctx context.Context, id int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	remote := []int64{}
	for _, a := range accs {
		local, err := c.store.AccountLabelsLocal(ctx, a.ID)
		if err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		if !local {
			remote = append(remote, a.ID)
		}
	}
	if _, err := c.store.DeleteLabel(ctx, id, remote); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return &UserError{Code: "notfound", Msg: msgLabelMissing}
		}
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventLabelsChanged, nil)
	for _, accID := range remote {
		c.spawn(func(ctx context.Context) { c.runLabelCleanups(ctx, accID) })
	}
	return nil
}

// runLabelCleanups saca del servidor los keywords de etiquetas borradas que
// quedaron pendientes en la cuenta: recorre sus carpetas seleccionables, busca
// los mensajes con el keyword y se lo quita. Una carpeta que falla por algo
// que no es la conexión no frena a las demás. Solo da el keyword por limpio si
// todas las carpetas salieron bien; cualquier error lo deja pendiente para la
// próxima pasada. Es idempotente.
func (c *Core) runLabelCleanups(ctx context.Context, accountID int64) {
	kws, err := c.store.PendingLabelCleanups(ctx, accountID)
	if err != nil || len(kws) == 0 {
		return
	}
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return
	}
	for _, kw := range kws {
		if ctx.Err() != nil {
			return
		}
		var failed bool
		err := c.withConn(ctx, accountID, func(ctx context.Context, conn *imapx.Conn) error {
			failed = false // cada intento recorre todas las carpetas de nuevo
			for _, f := range folders {
				if !f.Selectable {
					continue
				}
				if err := cleanFolderKeyword(conn, f.Name, kw); err != nil {
					if errors.Is(err, imapx.ErrNetwork) || ctx.Err() != nil {
						return err // la conexión se cayó: se corta acá
					}
					failed = true
				}
			}
			return nil
		})
		if err == nil && !failed {
			_ = c.store.DoneLabelCleanup(ctx, accountID, kw)
		}
	}
}

// cleanFolderKeyword le quita el keyword a los mensajes de la carpeta que lo
// tienen. El STORE toca solo ese keyword.
func cleanFolderKeyword(conn *imapx.Conn, folder, kw string) error {
	if _, err := conn.SelectRW(folder); err != nil {
		return err
	}
	uids, err := conn.SearchKeyword(kw)
	if err != nil || len(uids) == 0 {
		return err
	}
	return conn.StoreFlags(uids, false, goimap.Flag(kw))
}

// SetLabel pone (on) o saca la etiqueta de los mensajes, que tienen que ser de
// una sola cuenta. En el servidor solo se toca el keyword de esta etiqueta. Si
// el servidor no guarda keywords, la cuenta pasa a guardar las etiquetas solo
// acá (se avisa una vez con EventLabelsLocalOnly) y desde entonces no se le
// escribe más. La caché cambia recién cuando el servidor aceptó el STORE.
func (c *Core) SetLabel(ctx context.Context, ids []int64, labelID int64, on bool) error {
	if len(ids) == 0 {
		return nil
	}
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	kw, err := c.store.LabelKeyword(ctx, labelID)
	if errors.Is(err, store.ErrNotFound) {
		return &UserError{Code: "notfound", Msg: msgLabelMissing}
	}
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	refs, err := c.store.MessageRefs(ctx, ids)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	accountID, byFolder, err := groupByFolder(refs)
	if err != nil {
		return err
	}
	local, err := c.store.AccountLabelsLocal(ctx, accountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	for folderID, rs := range byFolder {
		if !local {
			if local, err = c.storeLabelKeyword(ctx, accountID, folderID, rs, kw, on); err != nil {
				return err
			}
		}
		msgIDs := make([]int64, len(rs))
		for i, r := range rs {
			msgIDs[i] = r.ID
		}
		if err := c.store.AddMessageLabel(ctx, msgIDs, labelID, on); err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		c.emit(EventFolderUpdated, map[string]any{"folderId": folderID})
	}
	return nil
}

// storeLabelKeyword pone o saca el keyword en los mensajes de una carpeta. Si
// al abrirla para escritura el servidor anuncia flags permanentes sin \*, no
// manda el STORE, marca la cuenta como local (avisando la primera vez) y
// devuelve local en true. Si no anuncia ninguno, no se sabe: se manda el
// STORE y un NO es un rechazo común, que no vuelve local la cuenta.
func (c *Core) storeLabelKeyword(ctx context.Context, accountID, folderID int64, rs []store.MessageRef,
	kw string, on bool) (local bool, err error) {
	f, last, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return false, c.ctxOrUserError(ctx, err)
	}
	uids := make([]uint32, len(rs))
	for i, r := range rs {
		uids[i] = r.UID
	}
	// Como en setFlag, escribir el mismo keyword dos veces es idempotente.
	err = c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
		info, err := selectInfoChecked(conn, f, last, true)
		if err != nil {
			return err
		}
		if local = info.Keywords == imapx.KeywordsNo; local {
			return nil
		}
		return conn.StoreFlags(uids, on, goimap.Flag(kw))
	})
	if err != nil || !local {
		return false, err
	}
	changed, err := c.store.SetAccountLabelsLocal(ctx, accountID)
	if err != nil {
		return false, c.ctxOrUserError(ctx, err)
	}
	if changed {
		acc, err := c.store.GetAccount(ctx, accountID)
		if err != nil {
			return false, c.ctxOrUserError(ctx, err)
		}
		c.emit(EventLabelsLocalOnly, map[string]any{"accountId": accountID, "email": acc.Email})
	}
	return true, nil
}

// ListLabeled lista, de todas las carpetas de la cuenta (o de todas las
// cuentas con accountID 0), los mensajes en caché que tienen la etiqueta, por
// fecha descendente. Limit 0 devuelve 100.
func (c *Core) ListLabeled(ctx context.Context, labelID, accountID int64, q model.ListQuery) ([]model.MessageSummary, error) {
	if _, err := c.store.LabelKeyword(ctx, labelID); errors.Is(err, store.ErrNotFound) {
		return nil, &UserError{Code: "notfound", Msg: msgLabelMissing}
	} else if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	if q.Limit <= 0 {
		q.Limit = defaultPageSize
	}
	ms, err := c.store.ListLabeled(ctx, labelID, accountID, q)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return ms, nil
}

// RefreshLabel completa desde el servidor lo que la caché no sabe de la
// etiqueta: en cada carpeta seleccionable de cada cuenta del alcance (accountID
// 0 = todas) que guarde keywords, busca los mensajes con el keyword, baja los
// encabezados de los que la caché no tiene y le saca la etiqueta a los que ya
// no la tienen en el servidor. Un error de una cuenta no corta a las demás;
// devuelve el primero.
func (c *Core) RefreshLabel(ctx context.Context, labelID, accountID int64) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	kw, err := c.store.LabelKeyword(ctx, labelID)
	if errors.Is(err, store.ErrNotFound) {
		return &UserError{Code: "notfound", Msg: msgLabelMissing}
	}
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	var accIDs []int64
	if accountID != 0 {
		accIDs = []int64{accountID}
	} else {
		accs, err := c.store.ListAccounts(ctx)
		if err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		for _, a := range accs {
			accIDs = append(accIDs, a.ID)
		}
	}
	var first error
	for _, accID := range accIDs {
		if err := c.refreshLabelAccount(ctx, accID, labelID, kw); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// refreshLabelAccount recorre las carpetas de la cuenta. Las cuentas que
// guardan las etiquetas solo acá no se tocan: el servidor no las tiene.
func (c *Core) refreshLabelAccount(ctx context.Context, accountID, labelID int64, kw string) error {
	if c.authPaused(accountID) {
		return nil // contraseña rechazada: no se insiste con el servidor
	}
	local, err := c.store.AccountLabelsLocal(ctx, accountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	if local {
		return nil
	}
	folders, err := c.store.ListFolders(ctx, accountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	var first error
	for _, f := range folders {
		if !f.Selectable {
			continue
		}
		if err := c.refreshLabelFolder(ctx, f, labelID, kw); err != nil {
			if first == nil {
				first = err
			}
			if connFailure(err) {
				break // la cuenta no responde: no se la reintenta carpeta por carpeta
			}
		}
	}
	return first
}

// refreshLabelFolder aplica a una carpeta lo que dice el servidor sobre el
// keyword. Toma el lock de sincronización de la carpeta, como SyncFolder.
func (c *Core) refreshLabelFolder(ctx context.Context, f model.Folder, labelID int64, kw string) error {
	mu := c.folderSyncLock(f.ID)
	mu.Lock()
	defer mu.Unlock()
	if err := c.ctxErr(ctx); err != nil {
		return err
	}
	_, last, err := c.store.GetFolder(ctx, f.ID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	var (
		changed    bool
		noKeywords bool
		info       imapx.SelectInfo
		remote     []uint32
		heads      []imapx.RemoteHeader
	)
	known, err := c.store.KnownUIDs(ctx, f.ID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	knownSet := make(map[uint32]bool, len(known))
	for _, u := range known {
		knownSet[u] = true
	}
	err = c.withConn(ctx, f.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		var err error
		if info, err = selectInfoChecked(conn, f, last, false); err != nil {
			return err
		}
		if info.Keywords == imapx.KeywordsNo {
			noKeywords = true // sin keywords en el servidor, la búsqueda vacía no dice nada
			return nil
		}
		if remote, err = conn.SearchKeyword(kw); err != nil {
			return err
		}
		missing := []uint32{}
		for _, u := range remote {
			if !knownSet[u] {
				missing = append(missing, u)
			}
		}
		for i := 0; i < len(missing); i += headerBatch {
			hs, err := conn.FetchHeaders(missing[i:min(i+headerBatch, len(missing))])
			if err != nil {
				return err
			}
			heads = append(heads, hs...)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if noKeywords {
		return nil
	}
	if len(heads) > 0 {
		// Carpeta nunca sincronizada: se anota el UIDVALIDITY antes de insertar
		// filas, como prepareSync, para que el próximo sync calcule bien.
		if last.UIDValidity == 0 {
			if err := c.store.SetFolderSync(ctx, f.ID, store.FolderSync{UIDValidity: info.UIDValidity}); err != nil {
				return c.ctxOrUserError(ctx, err)
			}
		}
		if err := c.store.UpsertHeaders(ctx, f.ID, headerRows(heads)); err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		changed = true
	}
	labeled, err := c.store.LabeledInFolder(ctx, f.ID, labelID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	onServer := make(map[uint32]bool, len(remote))
	for _, u := range remote {
		onServer[u] = true
	}
	stale := []int64{}
	for uid, id := range labeled {
		if !onServer[uid] {
			stale = append(stale, id)
		}
	}
	if len(stale) > 0 {
		if err := c.store.AddMessageLabel(ctx, stale, labelID, false); err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		changed = true
	}
	// Al revés: mensajes de la caché que en el servidor tienen el keyword y acá
	// no la etiqueta (un SetLabel que le ganó al sync, o fuera de la ventana de
	// flags que el sync revisa).
	unlabeled := []uint32{}
	for _, u := range remote {
		if _, ok := labeled[u]; !ok && knownSet[u] {
			unlabeled = append(unlabeled, u)
		}
	}
	if len(unlabeled) > 0 {
		ms, err := c.store.MessagesByUIDs(ctx, f.ID, unlabeled)
		if err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		add := make([]int64, 0, len(ms))
		for _, m := range ms {
			add = append(add, m.ID)
		}
		if len(add) > 0 {
			if err := c.store.AddMessageLabel(ctx, add, labelID, true); err != nil {
				return c.ctxOrUserError(ctx, err)
			}
			changed = true
		}
	}
	// No se recalculan los contadores: las etiquetas no los cambian y, en una
	// carpeta que nunca se abrió, la caché tiene solo lo que bajó acá (los
	// contadores buenos son los del STATUS).
	if changed {
		c.emit(EventFolderUpdated, map[string]any{"folderId": f.ID})
	}
	return nil
}

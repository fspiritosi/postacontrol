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
// los mensajes con el keyword y se lo quita. Solo da el keyword por limpio si
// todas las carpetas salieron bien; un error (de conexión o de otro tipo) lo
// deja pendiente para la próxima pasada. Es idempotente.
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
		err := c.withConn(ctx, accountID, func(_ context.Context, conn *imapx.Conn) error {
			for _, f := range folders {
				if !f.Selectable {
					continue
				}
				if _, err := conn.SelectRW(f.Name); err != nil {
					return err
				}
				uids, err := conn.SearchKeyword(kw)
				if err != nil {
					return err
				}
				if len(uids) == 0 {
					continue
				}
				if err := conn.StoreFlags(uids, false, goimap.Flag(kw)); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			_ = c.store.DoneLabelCleanup(ctx, accountID, kw)
		}
	}
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

package mail

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"

	"postacontrol/core/model"
	"postacontrol/core/store"
)

// Reglas: como los filtros de Thunderbird, cada cuenta tiene su lista, en
// orden. Se aplican solas a los correos no leídos que llegan a la Entrada y, a
// mano, a todos los correos de una carpeta. Para cada correo se recorren las
// reglas prendidas en orden; mover, borrar o «dejar de aplicar reglas» cortan
// el recorrido de ese correo.

const (
	maxRules          = 100
	maxRuleNameLen    = 80
	maxRuleConditions = 20
	maxRuleActions    = 10
	maxRuleValueLen   = 200

	msgRuleNoName      = "Poné un nombre para la regla."
	msgRuleNoCondition = "Agregá al menos una condición."
	msgRuleEmptyValue  = "Completá el texto de cada condición."
	msgRuleBadCond     = "Hay una condición que no es válida."
	msgRuleNoAction    = "Agregá al menos una acción."
	msgRuleBadAction   = "Hay una acción que no es válida."
	msgRuleNoFolder    = "Elegí la carpeta a la que se mueven los correos."
	msgRuleBadFolder   = "Esa carpeta no es de la cuenta de la regla o no admite correos."
	msgRuleNoLabel     = "Elegí la etiqueta que se pone."
	msgRuleMoveDelete  = "Una regla no puede mover y borrar a la vez."
	msgRuleTwoMoves    = "Una regla solo puede mover a una carpeta."
	msgRuleMax         = "Esta cuenta ya tiene 100 reglas, que es el máximo."
	msgRuleMissing     = "Esa regla ya no existe."
	msgRuleOtherAcct   = "Esas reglas no son todas de la cuenta."
	msgRuleBadAccount  = "Esa cuenta ya no existe."
	msgRuleRunNoSelect = "Esa carpeta no tiene correos."
)

var (
	ruleFields = []string{"from", "to", "cc", "toOrCc", "subject"}
	ruleOps    = []string{"contains", "notContains", "is", "isNot", "beginsWith", "endsWith"}
	ruleTypes  = []string{"move", "markRead", "flag", "label", "delete", "stop"}
)

// ListRules devuelve las reglas de la cuenta en su orden.
func (c *Core) ListRules(ctx context.Context, accountID int64) ([]model.Rule, error) {
	rs, err := c.store.ListRules(ctx, accountID)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return rs, nil
}

// SaveRule crea la regla (ID 0, al final de la lista de su cuenta) o la
// actualiza. La cuenta de una regla existente no cambia.
func (c *Core) SaveRule(ctx context.Context, r model.Rule) (model.Rule, error) {
	if r.ID != 0 {
		old, err := c.store.GetRule(ctx, r.ID)
		if errors.Is(err, store.ErrNotFound) {
			return model.Rule{}, &UserError{Code: "notfound", Msg: msgRuleMissing}
		}
		if err != nil {
			return model.Rule{}, c.ctxOrUserError(ctx, err)
		}
		r.AccountID = old.AccountID
	}
	r, err := c.cleanRule(ctx, r)
	if err != nil {
		return model.Rule{}, err
	}
	if r.ID == 0 {
		existing, err := c.store.ListRules(ctx, r.AccountID)
		if err != nil {
			return model.Rule{}, c.ctxOrUserError(ctx, err)
		}
		if len(existing) >= maxRules {
			return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleMax}
		}
	}
	saved, err := c.store.SaveRule(ctx, r)
	if errors.Is(err, store.ErrNotFound) {
		return model.Rule{}, &UserError{Code: "notfound", Msg: msgRuleMissing}
	}
	if err != nil {
		return model.Rule{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventRulesChanged, map[string]any{"accountId": saved.AccountID})
	return saved, nil
}

// DeleteRule borra la regla. Si ya no existe, no hace nada.
func (c *Core) DeleteRule(ctx context.Context, id int64) error {
	r, err := c.store.GetRule(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	if err := c.store.DeleteRule(ctx, id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventRulesChanged, map[string]any{"accountId": r.AccountID})
	return nil
}

// ReorderRules pone las reglas de la cuenta en el orden de ids, que tienen que
// ser todas de esa cuenta.
func (c *Core) ReorderRules(ctx context.Context, accountID int64, ids []int64) error {
	rs, err := c.store.ListRules(ctx, accountID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	for _, id := range ids {
		if !slices.ContainsFunc(rs, func(r model.Rule) bool { return r.ID == id }) {
			return &UserError{Code: "invalid", Msg: msgRuleOtherAcct}
		}
	}
	if err := c.store.ReorderRules(ctx, accountID, ids); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventRulesChanged, map[string]any{"accountId": accountID})
	return nil
}

// cleanRule recorta y valida la regla contra las carpetas y etiquetas de ahora.
func (c *Core) cleanRule(ctx context.Context, r model.Rule) (model.Rule, error) {
	if _, err := c.store.GetAccount(ctx, r.AccountID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleBadAccount}
		}
		return model.Rule{}, c.ctxOrUserError(ctx, err)
	}
	r.Name = truncRunes(strings.TrimSpace(r.Name), maxRuleNameLen)
	if r.Name == "" {
		return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleNoName}
	}
	if len(r.Conditions) == 0 {
		return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleNoCondition}
	}
	if len(r.Conditions) > maxRuleConditions {
		return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleBadCond}
	}
	conds := make([]model.RuleCondition, 0, len(r.Conditions))
	for _, cd := range r.Conditions {
		if !slices.Contains(ruleFields, cd.Field) || !slices.Contains(ruleOps, cd.Op) {
			return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleBadCond}
		}
		cd.Value = truncRunes(strings.TrimSpace(cd.Value), maxRuleValueLen)
		if cd.Value == "" {
			return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleEmptyValue}
		}
		conds = append(conds, cd)
	}
	r.Conditions = conds
	if len(r.Actions) == 0 {
		return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleNoAction}
	}
	if len(r.Actions) > maxRuleActions {
		return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleBadAction}
	}
	acts := make([]model.RuleAction, 0, len(r.Actions))
	var moves, deletes int
	for _, a := range r.Actions {
		if !slices.Contains(ruleTypes, a.Type) {
			return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleBadAction}
		}
		clean := model.RuleAction{Type: a.Type}
		switch a.Type {
		case "move":
			moves++
			if a.FolderID == 0 {
				return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleNoFolder}
			}
			f, _, err := c.store.GetFolder(ctx, a.FolderID)
			if errors.Is(err, store.ErrNotFound) || (err == nil && (f.AccountID != r.AccountID || !f.Selectable)) {
				return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleBadFolder}
			}
			if err != nil {
				return model.Rule{}, c.ctxOrUserError(ctx, err)
			}
			clean.FolderID = a.FolderID
		case "label":
			if a.LabelID == 0 {
				return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleNoLabel}
			}
			if _, err := c.store.LabelKeyword(ctx, a.LabelID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					return model.Rule{}, &UserError{Code: "invalid", Msg: msgLabelMissing}
				}
				return model.Rule{}, c.ctxOrUserError(ctx, err)
			}
			clean.LabelID = a.LabelID
		case "delete":
			deletes++
		}
		acts = append(acts, clean)
	}
	if moves > 1 {
		return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleTwoMoves}
	}
	if moves > 0 && deletes > 0 {
		return model.Rule{}, &UserError{Code: "invalid", Msg: msgRuleMoveDelete}
	}
	r.Actions = acts
	return r, nil
}

func truncRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return strings.TrimSpace(string(r[:n]))
	}
	return s
}

// ruleMatches dice si el correo cumple la regla.
func ruleMatches(r model.Rule, m store.RuleMessage) bool {
	if len(r.Conditions) == 0 {
		return false
	}
	for _, cd := range r.Conditions {
		ok := conditionMatches(cd, m)
		if r.MatchAll && !ok {
			return false
		}
		if !r.MatchAll && ok {
			return true
		}
	}
	return r.MatchAll
}

// conditionMatches compara sin distinguir mayúsculas. En los campos de
// direcciones, una condición positiva se cumple si alguna dirección la cumple
// (por nombre o por correo) y una negativa si ninguna cumple la positiva.
func conditionMatches(cd model.RuleCondition, m store.RuleMessage) bool {
	want := strings.ToLower(strings.TrimSpace(cd.Value))
	op, negate := cd.Op, false
	switch op {
	case "notContains":
		op, negate = "contains", true
	case "isNot":
		op, negate = "is", true
	}
	var hit bool
	if cd.Field == "subject" {
		hit = textMatches(op, strings.ToLower(strings.TrimSpace(m.Subject)), want)
	} else {
		var addrs []model.Address
		switch cd.Field {
		case "from":
			if m.From.Email != "" || m.From.Name != "" {
				addrs = []model.Address{m.From}
			}
		case "to":
			addrs = m.To
		case "cc":
			addrs = m.Cc
		case "toOrCc":
			addrs = append(slices.Clone(m.To), m.Cc...)
		}
		hit = slices.ContainsFunc(addrs, func(a model.Address) bool { return addressMatches(op, a, want) })
	}
	return hit != negate
}

func addressMatches(op string, a model.Address, want string) bool {
	name := strings.ToLower(strings.TrimSpace(a.Name))
	email := strings.ToLower(strings.TrimSpace(a.Email))
	if op == "contains" {
		// «Nombre <correo>», como se ve el encabezado.
		full := email
		if name != "" {
			full = name + " <" + email + ">"
		}
		return strings.Contains(full, want)
	}
	return (email != "" && textMatches(op, email, want)) || (name != "" && textMatches(op, name, want))
}

func textMatches(op, got, want string) bool {
	switch op {
	case "contains":
		return strings.Contains(got, want)
	case "is":
		return got == want
	case "beginsWith":
		return strings.HasPrefix(got, want)
	case "endsWith":
		return strings.HasSuffix(got, want)
	}
	return false
}

// rulePlan es lo que hay que hacer con un grupo de correos, ya juntado por
// acción para hacer pocas idas al servidor.
type rulePlan struct {
	matched int
	seen    []int64
	flagged []int64
	labels  map[int64][]int64 // etiqueta → correos
	moves   map[int64][]int64 // carpeta destino → correos
	deletes []int64
}

// planRules recorre las reglas prendidas para cada correo. allowDelete en
// false ignora la acción de borrar (en la Papelera borrar es definitivo).
func planRules(rules []model.Rule, msgs []store.RuleMessage, allowDelete bool) rulePlan {
	p := rulePlan{labels: map[int64][]int64{}, moves: map[int64][]int64{}}
	for _, m := range msgs {
		var hit, seen, flagged, done bool
		labels := []int64{}
		for _, r := range rules {
			if done {
				break
			}
			if !r.Enabled || !ruleMatches(r, m) {
				continue
			}
			hit = true
			for _, a := range r.Actions {
				switch a.Type {
				case "markRead":
					seen = true
				case "flag":
					flagged = true
				case "label":
					if !slices.Contains(labels, a.LabelID) {
						labels = append(labels, a.LabelID)
					}
				case "move":
					p.moves[a.FolderID] = append(p.moves[a.FolderID], m.ID)
					done = true
				case "delete":
					if allowDelete {
						p.deletes = append(p.deletes, m.ID)
						done = true
					}
				case "stop":
					done = true
				}
			}
		}
		if !hit {
			continue
		}
		p.matched++
		if seen && !m.Seen {
			p.seen = append(p.seen, m.ID)
		}
		if flagged {
			p.flagged = append(p.flagged, m.ID)
		}
		for _, l := range labels {
			p.labels[l] = append(p.labels[l], m.ID)
		}
	}
	return p
}

// applyRulePlan aplica el plan: primero marcas y etiquetas (que viajan con el
// correo) y después mover y borrar. Sigue ante un error y devuelve el primero.
// Una carpeta o una etiqueta que ya no existe se saltea.
func (c *Core) applyRulePlan(ctx context.Context, accountID int64, p rulePlan) error {
	var first error
	keep := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	if len(p.seen) > 0 {
		keep(c.SetSeen(ctx, p.seen, true))
	}
	if len(p.flagged) > 0 {
		keep(c.SetFlagged(ctx, p.flagged, true))
	}
	for _, labelID := range sortedKeys(p.labels) {
		err := c.SetLabel(ctx, p.labels[labelID], labelID, true)
		var ue *UserError
		if errors.As(err, &ue) && ue.Code == "notfound" {
			slog.Warn("regla con una etiqueta que ya no existe", "etiqueta", labelID)
			continue
		}
		keep(err)
	}
	for _, folderID := range sortedKeys(p.moves) {
		f, _, err := c.store.GetFolder(ctx, folderID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && (f.AccountID != accountID || !f.Selectable)) {
			slog.Warn("regla con una carpeta que ya no existe", "carpeta", folderID)
			continue
		}
		if err != nil {
			keep(c.ctxOrUserError(ctx, err))
			continue
		}
		keep(c.MoveMessages(ctx, p.moves[folderID], folderID))
	}
	if len(p.deletes) > 0 {
		keep(c.DeleteMessages(ctx, p.deletes))
	}
	return first
}

func sortedKeys(m map[int64][]int64) []int64 {
	keys := make([]int64, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// applyNewMailRules aplica las reglas a los correos no leídos que la pasada
// trajo a la Entrada. No corre en la primera sincronización (como el aviso de
// correo nuevo): ahí todo es «nuevo». Los errores solo se registran: la
// sincronización ya terminó bien y el correo queda en la Entrada.
func (c *Core) applyNewMailRules(ctx context.Context, accountID int64, p syncPlan) {
	if p.folder.Role != "inbox" || p.firstSync || len(p.fresh) == 0 {
		return
	}
	rules, err := c.store.ListRules(ctx, accountID)
	if err != nil || !slices.ContainsFunc(rules, func(r model.Rule) bool { return r.Enabled }) {
		return
	}
	msgs, err := c.store.RuleMessages(ctx, p.folder.ID, p.fresh)
	if err != nil {
		slog.Warn("no se pudieron leer los correos nuevos para las reglas", "err", err)
		return
	}
	// Como Thunderbird, las reglas automáticas solo miran lo no leído: un
	// correo que se devolvió a mano a la Entrada no vuelve a moverse.
	msgs = slices.DeleteFunc(msgs, func(m store.RuleMessage) bool { return m.Seen })
	plan := planRules(rules, msgs, true)
	if plan.matched == 0 {
		return
	}
	if err := c.applyRulePlan(ctx, accountID, plan); err != nil {
		slog.Warn("no se pudieron aplicar todas las reglas al correo nuevo", "cuenta", accountID, "err", err)
	}
}

// RunRules aplica ya las reglas prendidas de la cuenta a todos los correos de
// la carpeta, leídos o no. En la Papelera la acción de borrar se ignora.
func (c *Core) RunRules(ctx context.Context, folderID int64) (model.RuleRun, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	if err := c.ctxErr(ctx); err != nil {
		return model.RuleRun{}, err
	}
	f, _, err := c.store.GetFolder(ctx, folderID)
	if err != nil {
		return model.RuleRun{}, c.ctxOrUserError(ctx, err)
	}
	if !f.Selectable {
		return model.RuleRun{}, &UserError{Code: "invalid", Msg: msgRuleRunNoSelect}
	}
	rules, err := c.store.ListRules(ctx, f.AccountID)
	if err != nil {
		return model.RuleRun{}, c.ctxOrUserError(ctx, err)
	}
	msgs, err := c.store.RuleMessages(ctx, folderID, nil)
	if err != nil {
		return model.RuleRun{}, c.ctxOrUserError(ctx, err)
	}
	plan := planRules(rules, msgs, f.Role != "trash")
	// Mover a la misma carpeta no hace nada.
	delete(plan.moves, folderID)
	if plan.matched == 0 {
		return model.RuleRun{}, nil
	}
	if err := c.applyRulePlan(ctx, f.AccountID, plan); err != nil {
		return model.RuleRun{Matched: plan.matched}, err
	}
	return model.RuleRun{Matched: plan.matched}, nil
}

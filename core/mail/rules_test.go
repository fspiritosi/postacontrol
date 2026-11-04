package mail

import (
	"context"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/store"
)

func cond(field, op, value string) model.RuleCondition {
	return model.RuleCondition{Field: field, Op: op, Value: value}
}

func moveTo(folderID int64) model.RuleAction {
	return model.RuleAction{Type: "move", FolderID: folderID}
}

func mustSaveRule(t *testing.T, c *Core, r model.Rule) model.Rule {
	t.Helper()
	saved, err := c.SaveRule(context.Background(), r)
	require.NoError(t, err)
	return saved
}

func rawMailFull(from, to, cc, subject string) []byte {
	h := "From: " + from + "\r\nTo: " + to + "\r\n"
	if cc != "" {
		h += "Cc: " + cc + "\r\n"
	}
	return []byte(h + "Subject: " + subject + "\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <" +
		subject + "@ej.com>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhola\r\n")
}

func TestConditionMatches(t *testing.T) {
	m := store.RuleMessage{
		From:    model.Address{Name: "Banco Galicia", Email: "Avisos@Galicia.com.ar"},
		To:      []model.Address{{Name: "Yo", Email: "yo@ej.com"}},
		Cc:      []model.Address{{Email: "contador@estudio.com"}},
		Subject: "Resumen de tu tarjeta",
	}
	cases := []struct {
		c    model.RuleCondition
		want bool
	}{
		{cond("from", "contains", "galicia"), true},
		{cond("from", "contains", "banco galicia <avisos"), true},
		{cond("from", "is", "avisos@galicia.com.ar"), true},
		{cond("from", "is", "BANCO GALICIA"), true},
		{cond("from", "is", "galicia"), false},
		{cond("from", "isNot", "otro@ej.com"), true},
		{cond("from", "endsWith", "@galicia.com.ar"), true},
		{cond("from", "beginsWith", "banco"), true},
		{cond("from", "notContains", "galicia"), false},
		{cond("to", "is", "yo@ej.com"), true},
		{cond("cc", "is", "yo@ej.com"), false},
		{cond("toOrCc", "endsWith", "estudio.com"), true},
		{cond("toOrCc", "notContains", "estudio"), false},
		{cond("subject", "contains", "TARJETA"), true},
		{cond("subject", "beginsWith", "resumen"), true},
		{cond("subject", "is", "resumen"), false},
		{cond("subject", "isNot", "resumen"), true},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, conditionMatches(tc.c, m), "%+v", tc.c)
	}
	// Sin Cc: una condición positiva no se cumple y una negativa sí.
	noCc := store.RuleMessage{From: m.From}
	assert.False(t, conditionMatches(cond("cc", "contains", "x"), noCc))
	assert.True(t, conditionMatches(cond("cc", "notContains", "x"), noCc))
}

func TestRuleMatches_AllOrAny(t *testing.T) {
	m := store.RuleMessage{From: model.Address{Email: "bea@ej.com"}, Subject: "Factura"}
	r := model.Rule{MatchAll: true, Conditions: []model.RuleCondition{
		cond("from", "is", "bea@ej.com"), cond("subject", "contains", "pedido"),
	}}
	assert.False(t, ruleMatches(r, m))
	r.MatchAll = false
	assert.True(t, ruleMatches(r, m))
}

func TestPlanRules_OrderStopAndMove(t *testing.T) {
	msgs := []store.RuleMessage{
		{ID: 1, From: model.Address{Email: "bea@ej.com"}},
		{ID: 2, From: model.Address{Email: "ana@ej.com"}},
	}
	rules := []model.Rule{
		{Enabled: true, MatchAll: true, Conditions: []model.RuleCondition{cond("from", "is", "ana@ej.com")},
			Actions: []model.RuleAction{{Type: "flag"}, {Type: "stop"}}},
		{Enabled: false, MatchAll: true, Conditions: []model.RuleCondition{cond("from", "contains", "@ej.com")},
			Actions: []model.RuleAction{{Type: "delete"}}},
		{Enabled: true, MatchAll: true, Conditions: []model.RuleCondition{cond("from", "contains", "@ej.com")},
			Actions: []model.RuleAction{{Type: "markRead"}, moveTo(9)}},
		{Enabled: true, MatchAll: true, Conditions: []model.RuleCondition{cond("from", "contains", "@ej.com")},
			Actions: []model.RuleAction{{Type: "label", LabelID: 5}}},
	}
	p := planRules(rules, msgs, true)
	assert.Equal(t, 2, p.matched)
	assert.Equal(t, []int64{2}, p.flagged)              // Ana se detiene en la primera
	assert.Equal(t, map[int64][]int64{9: {1}}, p.moves) // la regla apagada no borra
	assert.Equal(t, []int64{1}, p.seen)                 // marcar leído viaja con el movido
	assert.Empty(t, p.labels)                           // mover corta las reglas siguientes
	assert.Empty(t, p.deletes)
}

func TestSaveRule_Validation(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	require.NoError(t, imap.User.Create("Archivo", nil))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	archivoID := folderID(t, c, acc.ID, "Archivo")
	ctx := context.Background()
	good := model.Rule{AccountID: acc.ID, Name: " Bea ", Enabled: true, MatchAll: true,
		Conditions: []model.RuleCondition{cond("from", "is", " bea@ej.com ")},
		Actions:    []model.RuleAction{moveTo(archivoID)}}

	bad := func(mut func(r *model.Rule), msg string) {
		t.Helper()
		r := good
		r.Conditions = append([]model.RuleCondition{}, good.Conditions...)
		r.Actions = append([]model.RuleAction{}, good.Actions...)
		mut(&r)
		_, err := c.SaveRule(ctx, r)
		require.Error(t, err)
		assert.Equal(t, msg, err.Error())
	}
	bad(func(r *model.Rule) { r.Name = "  " }, msgRuleNoName)
	bad(func(r *model.Rule) { r.Conditions = nil }, msgRuleNoCondition)
	bad(func(r *model.Rule) { r.Conditions[0].Value = " " }, msgRuleEmptyValue)
	bad(func(r *model.Rule) { r.Conditions[0].Field = "body" }, msgRuleBadCond)
	bad(func(r *model.Rule) { r.Actions = nil }, msgRuleNoAction)
	bad(func(r *model.Rule) { r.Actions[0].FolderID = 0 }, msgRuleNoFolder)
	bad(func(r *model.Rule) { r.Actions[0].FolderID = 99999 }, msgRuleBadFolder)
	bad(func(r *model.Rule) { r.Actions = append(r.Actions, model.RuleAction{Type: "delete"}) }, msgRuleMoveDelete)
	bad(func(r *model.Rule) { r.Actions = append(r.Actions, moveTo(inboxID)) }, msgRuleTwoMoves)
	bad(func(r *model.Rule) { r.Actions = []model.RuleAction{{Type: "label", LabelID: 777}} }, msgLabelMissing)
	bad(func(r *model.Rule) { r.AccountID = 4242 }, msgRuleBadAccount)

	saved := mustSaveRule(t, c, good)
	assert.Equal(t, "Bea", saved.Name)
	assert.Equal(t, "bea@ej.com", saved.Conditions[0].Value)
}

func TestRules_CRUDAndReorder(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	base := model.Rule{AccountID: acc.ID, Enabled: true, MatchAll: true,
		Conditions: []model.RuleCondition{cond("subject", "contains", "x")},
		Actions:    []model.RuleAction{{Type: "markRead"}}}
	a, b := base, base
	a.Name, b.Name = "A", "B"
	ra := mustSaveRule(t, c, a)
	rb := mustSaveRule(t, c, b)
	assert.Contains(t, n.matching(EventRulesChanged), map[string]any{"accountId": acc.ID})

	require.NoError(t, c.ReorderRules(ctx, acc.ID, []int64{rb.ID, ra.ID}))
	list, err := c.ListRules(ctx, acc.ID)
	require.NoError(t, err)
	require.Len(t, list, 2)
	assert.Equal(t, []string{"B", "A"}, []string{list[0].Name, list[1].Name})

	// Editar no cambia la cuenta ni el orden.
	ra.Name, ra.AccountID, ra.Enabled = "A2", 999, false
	ra.Actions = []model.RuleAction{moveTo(inboxID)}
	upd := mustSaveRule(t, c, ra)
	assert.Equal(t, acc.ID, upd.AccountID)
	assert.False(t, upd.Enabled)
	assert.Equal(t, list[1].Position, upd.Position)

	require.Error(t, c.ReorderRules(ctx, acc.ID, []int64{rb.ID, 12345}))
	require.NoError(t, c.DeleteRule(ctx, ra.ID))
	require.NoError(t, c.DeleteRule(ctx, ra.ID)) // ya borrada: no hace nada
	list, err = c.ListRules(ctx, acc.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
	_, err = c.SaveRule(ctx, ra)
	assert.EqualError(t, err, msgRuleMissing)
}

func TestNewMailRules_MoveUnreadFromSender(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	require.NoError(t, imap.User.Create("Banco", nil))
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "viejo")) // ya estaba: primera pasada
	acc, inboxID := setupSynced(t, imap, smtp, c)
	bancoID := folderID(t, c, acc.ID, "Banco")
	mustSaveRule(t, c, model.Rule{AccountID: acc.ID, Name: "Banco", Enabled: true, MatchAll: true,
		Conditions: []model.RuleCondition{cond("from", "endsWith", "@galicia.com.ar")},
		Actions:    []model.RuleAction{{Type: "markRead"}, moveTo(bancoID)}})

	imap.Deliver(t, "INBOX", rawMail("Galicia <avisos@galicia.com.ar>", "resumen"))
	imap.Deliver(t, "INBOX", rawMail("Galicia <avisos@galicia.com.ar>", "leido"), goimap.FlagSeen)
	imap.Deliver(t, "INBOX", rawMail("Ana <ana@ej.com>", "hola"))
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))

	assert.Equal(t, []string{"resumen"}, serverSubjects(t, c, imap, testUser, "Banco"))
	assert.ElementsMatch(t, []string{"viejo", "leido", "hola"}, serverSubjects(t, c, imap, testUser, "INBOX"))
	assert.ElementsMatch(t, []string{"viejo", "leido", "hola"}, subjects(listAll(t, c, inboxID)))
	// El movido llegó leído y no se avisa como correo nuevo.
	require.NoError(t, c.SyncFolder(context.Background(), bancoID))
	assert.True(t, listAll(t, c, bancoID)[0].Seen)
	news := newMailEvents(n)
	require.Len(t, news, 1)
	assert.Equal(t, 1, news[0].Count, "solo Ana es correo nuevo en la Entrada")
	assert.Equal(t, "hola", news[0].Items[0].Subject)
}

func TestNewMailRules_NotOnFirstSync(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	require.NoError(t, imap.User.Create("Banco", nil))
	acc, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	// Las carpetas tienen que estar en la caché para guardar la regla.
	_, err = c.refreshFolders(context.Background(), acc.ID, c.accountEpoch(acc.ID))
	require.NoError(t, err)
	mustSaveRule(t, c, model.Rule{AccountID: acc.ID, Name: "Bea", Enabled: true, MatchAll: true,
		Conditions: []model.RuleCondition{cond("from", "contains", "bea")},
		Actions:    []model.RuleAction{moveTo(folderID(t, c, acc.ID, "Banco"))}})
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "uno"))
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))
	assert.Equal(t, []string{"uno"}, serverSubjects(t, c, imap, testUser, "INBOX"))
}

func TestRunRules_ManualAppliesToReadMail(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", rawMailFull("Bea <bea@ej.com>", testUser, "jefe@ej.com", "a"), goimap.FlagSeen)
	imap.Deliver(t, "INBOX", rawMailFull("Ana <ana@ej.com>", testUser, "", "b"))
	acc, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	lbl, err := c.CreateLabel(ctx, "Jefe", "#ff0000")
	require.NoError(t, err)
	mustSaveRule(t, c, model.Rule{AccountID: acc.ID, Name: "Jefe", Enabled: true, MatchAll: true,
		Conditions: []model.RuleCondition{cond("cc", "is", "jefe@ej.com")},
		Actions:    []model.RuleAction{{Type: "flag"}, {Type: "label", LabelID: lbl.ID}}})

	run, err := c.RunRules(ctx, inboxID)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Matched)
	a := bySubject(t, listAll(t, c, inboxID), "a")
	b := bySubject(t, listAll(t, c, inboxID), "b")
	assert.True(t, a.Flagged)
	assert.Equal(t, []int64{lbl.ID}, a.Labels)
	assert.False(t, b.Flagged)
	assert.True(t, hasFlag(serverFlags(t, c, imap, a.UID), goimap.FlagFlagged))
}

func TestRunRules_DeletedFolderIsSkipped(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	require.NoError(t, imap.User.Create("Viejo", nil))
	deliverN(t, imap, 1, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	viejoID := folderID(t, c, acc.ID, "Viejo")
	mustSaveRule(t, c, model.Rule{AccountID: acc.ID, Name: "Bea", Enabled: true, MatchAll: true,
		Conditions: []model.RuleCondition{cond("from", "contains", "bea")},
		Actions:    []model.RuleAction{{Type: "markRead"}, moveTo(viejoID)}})
	imap.DeleteMailbox(t, "Viejo")
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))

	run, err := c.RunRules(context.Background(), inboxID)
	require.NoError(t, err)
	assert.Equal(t, 1, run.Matched)
	ms := listAll(t, c, inboxID)
	require.Len(t, ms, 1)
	assert.True(t, ms[0].Seen, "las demás acciones se aplican igual")
}

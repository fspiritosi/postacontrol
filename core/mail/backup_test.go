package mail

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

// fillForBackup carga en c una ficha, una firma (por defecto de la cuenta),
// una etiqueta y dos reglas: una que mueve a Sent y otra que etiqueta.
func fillForBackup(t *testing.T, c *Core, acc model.Account) model.Label {
	t.Helper()
	ctx := context.Background()
	_, err := c.SaveContact(ctx, model.Contact{Name: "Ana", Emails: []string{"ana@x.com", "ana@trabajo.com"},
		Phones: []string{"123"}, Company: "ACME", Notes: "nota"})
	require.NoError(t, err)
	_, err = c.SaveContact(ctx, model.Contact{Name: "Beto", Emails: []string{"beto@x.com"}})
	require.NoError(t, err)
	sig, err := c.SaveSignature(ctx, model.Signature{Name: "Trabajo", HTML: "<p>Saludos</p>"})
	require.NoError(t, err)
	require.NoError(t, c.SetAccountSignature(ctx, acc.ID, sig.ID))
	label, err := c.CreateLabel(ctx, "Facturas", "#ff0000")
	require.NoError(t, err)
	base := model.Rule{AccountID: acc.ID, Enabled: true, MatchAll: true,
		Conditions: []model.RuleCondition{cond("from", "endsWith", "@banco.com")}}
	r1 := base
	r1.Name = "Banco a Sent"
	r1.Actions = []model.RuleAction{moveTo(folderID(t, c, acc.ID, "Sent")), {Type: "stop"}}
	mustSaveRule(t, c, r1)
	r2 := base
	r2.Name, r2.Enabled = "Etiquetar", false
	r2.Actions = []model.RuleAction{{Type: "label", LabelID: label.ID}}
	mustSaveRule(t, c, r2)
	return label
}

func exportTo(t *testing.T, c *Core) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "respaldo.json")
	require.NoError(t, c.ExportBackup(context.Background(), path))
	return path
}

func TestBackup_RestoresIntoAnotherInstall(t *testing.T) {
	src, imapA, smtpA, _ := newTestCore(t)
	accA, _ := setupSynced(t, imapA, smtpA, src)
	labelA := fillForBackup(t, src, accA)
	kwA, err := src.store.LabelKeyword(context.Background(), labelA.ID)
	require.NoError(t, err)
	path := exportTo(t, src)

	// Otra instalación con la misma cuenta (otro servidor, mismas carpetas) y
	// una ficha que ya tiene uno de los correos de Ana.
	dst, imapB, smtpB, n := newTestCore(t)
	accB, _ := setupSynced(t, imapB, smtpB, dst)
	ctx := context.Background()
	_, err = dst.SaveContact(ctx, model.Contact{Name: "Ana (vieja)", Emails: []string{"ANA@trabajo.com"}})
	require.NoError(t, err)

	got, err := dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, model.BackupImport{
		ContactsAdded: 1, ContactsSkipped: 1,
		SignaturesAdded: 1,
		LabelsAdded:     1,
		RulesAdded:      2,
	}, got)

	contacts, err := dst.ListContacts(ctx, "")
	require.NoError(t, err)
	names := []string{}
	for _, ct := range contacts {
		names = append(names, ct.Name)
	}
	assert.ElementsMatch(t, []string{"Ana (vieja)", "Beto"}, names)

	sigs, err := dst.ListSignatures(ctx)
	require.NoError(t, err)
	require.Len(t, sigs, 1)
	assert.Equal(t, "Trabajo", sigs[0].Name)
	acc, err := dst.store.GetAccount(ctx, accB.ID)
	require.NoError(t, err)
	assert.Equal(t, sigs[0].ID, acc.DefaultSignatureID)

	// La etiqueta vuelve con el mismo keyword: se reengancha con el servidor.
	byKw, err := dst.store.LabelByKeyword(ctx)
	require.NoError(t, err)
	labelB, ok := byKw[kwA]
	require.True(t, ok, "la etiqueta tiene que conservar su keyword")

	rules, err := dst.ListRules(ctx, accB.ID)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, "Banco a Sent", rules[0].Name)
	assert.True(t, rules[0].Enabled)
	assert.Equal(t, []model.RuleAction{moveTo(folderID(t, dst, accB.ID, "Sent")), {Type: "stop"}}, rules[0].Actions)
	assert.Equal(t, "Etiquetar", rules[1].Name)
	assert.False(t, rules[1].Enabled)
	assert.Equal(t, []model.RuleAction{{Type: "label", LabelID: labelB}}, rules[1].Actions)
	assert.Equal(t, []model.RuleCondition{cond("from", "endsWith", "@banco.com")}, rules[1].Conditions)

	assert.NotEmpty(t, n.matching(EventContactsChanged))
	assert.NotEmpty(t, n.matching(EventLabelsChanged))

	// Importar otra vez no duplica nada.
	again, err := dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, model.BackupImport{
		ContactsSkipped: 2, SignaturesSkipped: 1, LabelsSkipped: 1, RulesSkipped: 2,
	}, again)
}

func TestBackup_KeepsWhatIsAlreadyThere(t *testing.T) {
	src, imapA, smtpA, _ := newTestCore(t)
	accA, _ := setupSynced(t, imapA, smtpA, src)
	fillForBackup(t, src, accA)
	path := exportTo(t, src)

	dst, imapB, smtpB, _ := newTestCore(t)
	accB, _ := setupSynced(t, imapB, smtpB, dst)
	ctx := context.Background()
	// Una etiqueta con el mismo nombre (otro keyword) y una firma por defecto propia.
	mine, err := dst.CreateLabel(ctx, "facturas", "#00ff00")
	require.NoError(t, err)
	own, err := dst.SaveSignature(ctx, model.Signature{Name: "Personal", HTML: "<p>Abrazo</p>"})
	require.NoError(t, err)
	require.NoError(t, dst.SetAccountSignature(ctx, accB.ID, own.ID))

	got, err := dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 1, got.LabelsSkipped)
	assert.Equal(t, 1, got.SignaturesAdded)

	labels, err := dst.ListLabels(ctx)
	require.NoError(t, err)
	require.Len(t, labels, 1)
	assert.Equal(t, "#00ff00", labels[0].Color, "la etiqueta que ya estaba no cambia")
	rules, err := dst.ListRules(ctx, accB.ID)
	require.NoError(t, err)
	require.Len(t, rules, 2)
	assert.Equal(t, mine.ID, rules[1].Actions[0].LabelID, "la regla usa la etiqueta del mismo nombre")

	acc, err := dst.store.GetAccount(ctx, accB.ID)
	require.NoError(t, err)
	assert.Equal(t, own.ID, acc.DefaultSignatureID, "la firma por defecto elegida no se pisa")
}

func TestBackup_RulesWithoutAccountOrFolder(t *testing.T) {
	src, imapA, smtpA, _ := newTestCore(t)
	accA, _ := setupSynced(t, imapA, smtpA, src)
	fillForBackup(t, src, accA)
	path := exportTo(t, src)

	// Otra instalación sin esa cuenta: las reglas no tienen dónde ir.
	dst, _, _, _ := newTestCore(t)
	got, err := dst.ImportBackup(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, 2, got.RulesFailed)
	assert.Equal(t, 0, got.RulesAdded)
	assert.Equal(t, 2, got.ContactsAdded)
}

func TestBackup_RuleWhoseFolderIsMissing(t *testing.T) {
	src, imapA, smtpA, _ := newTestCore(t)
	accA, _ := setupSynced(t, imapA, smtpA, src)
	fillForBackup(t, src, accA)
	path := exportTo(t, src)

	// Se cambia la carpeta de la primera regla por una que no existe.
	var b map[string]any
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &b))
	rule := b["rules"].([]any)[0].(map[string]any)
	rule["actions"].([]any)[0].(map[string]any)["folder"] = "No existe"
	data, err = json.Marshal(b)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))

	dst, imapB, smtpB, _ := newTestCore(t)
	setupSynced(t, imapB, smtpB, dst)
	got, err := dst.ImportBackup(context.Background(), path)
	require.NoError(t, err)
	assert.Equal(t, 1, got.RulesFailed)
	assert.Equal(t, 1, got.RulesAdded)
}

func TestBackup_RejectsOtherFiles(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	dir := t.TempDir()
	cases := map[string]string{
		"no-json.json": "hola",
		"otro.json":    `{"contacts": []}`,
		"futuro.json":  `{"app": "postacontrol", "kind": "respaldo", "version": 99}`,
	}
	for name, content := range cases {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		_, err := c.ImportBackup(context.Background(), p)
		var ue *UserError
		require.ErrorAs(t, err, &ue, name)
	}
	_, err := c.ImportBackup(context.Background(), filepath.Join(dir, "no-esta.json"))
	var ue *UserError
	require.ErrorAs(t, err, &ue)
}

func TestBackup_ExportHasNoPasswords(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	setupSynced(t, imap, smtp, c)
	data, err := os.ReadFile(exportTo(t, c))
	require.NoError(t, err)
	assert.NotContains(t, string(data), testPass)
	var b map[string]any
	require.NoError(t, json.Unmarshal(data, &b))
	assert.Equal(t, "postacontrol", b["app"])
	assert.Equal(t, "respaldo", b["kind"])
}

func TestBackup_IncludesLocalTasks(t *testing.T) {
	src, _, _, _ := newTestCore(t)
	ctx := context.Background()
	done := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	t1, err := src.SaveLocalTask(ctx, model.LocalTask{Title: "Pagar", Notes: "luz", DueDate: "2026-11-19", DueTime: "10:00", RemindBefore: ip(60),
		SourceMessageID: "<a@x>", SourceSubject: "Factura"})
	require.NoError(t, err)
	_, err = src.SaveLocalTask(ctx, model.LocalTask{Title: "Sin fecha"})
	require.NoError(t, err)
	require.NoError(t, src.store.SetLocalTaskDone(ctx, t1.ID, &done))
	path := exportTo(t, src)

	dst, _, _, _ := newTestCore(t)
	res, err := dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 2, res.TasksAdded)
	list, err := dst.ListLocalTasks(ctx, true)
	require.NoError(t, err)
	require.Len(t, list, 2)
	var got model.LocalTask
	for _, l := range list {
		if l.Title == "Pagar" {
			got = l
		}
	}
	assert.Equal(t, "luz", got.Notes)
	assert.Equal(t, "2026-11-19", got.DueDate)
	assert.Equal(t, "10:00", got.DueTime)
	require.NotNil(t, got.RemindBefore)
	assert.Equal(t, 60, *got.RemindBefore)
	assert.Equal(t, "<a@x>", got.SourceMessageID)
	require.NotNil(t, got.DoneAt)
	assert.Equal(t, done.Unix(), got.DoneAt.Unix())

	res, err = dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 0, res.TasksAdded)
	assert.Equal(t, 2, res.TasksSkipped)
	list, _ = dst.ListLocalTasks(ctx, true)
	assert.Len(t, list, 2)
}

func TestBackup_EventsRoundTrip(t *testing.T) {
	ctx := context.Background()
	src, _, _, _, _ := receiverCore(t, meEmail)
	p := writeFile(t, "g.ics", sampleICS(t, "google-export.ics"))
	_, err := src.ImportICS(ctx, []string{p})
	require.NoError(t, err)
	e := eventOf(t, src, "organiza-otro-planificacion@google.com")
	e.Notes, e.RemindBefore = "mis notas", ip(30)
	_, err = src.store.SaveEvent(ctx, e, seriesEnd(e))
	require.NoError(t, err)
	path := exportTo(t, src)

	dst, _, _, n, _ := receiverCore(t, meEmail)
	res, err := dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 5, res.EventsAdded)
	assert.Equal(t, 0, res.EventsSkipped)
	assert.Len(t, n.matching(EventCalendarChanged), 1)

	norm := func(c *Core) []model.CalendarEvent {
		all, err := c.store.AllEvents(ctx)
		require.NoError(t, err)
		for i := range all {
			all[i].ID = 0
		}
		slices.SortFunc(all, func(a, b model.CalendarEvent) int { return strings.Compare(a.UID, b.UID) })
		return all
	}
	want, got := norm(src), norm(dst)
	require.Len(t, got, 5)
	assert.Equal(t, want, got)
	assert.NotEmpty(t, got[0].RawICS+got[1].RawICS+got[2].RawICS+got[3].RawICS+got[4].RawICS, "el RawICS viaja")

	res, err = dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 0, res.EventsAdded)
	assert.Equal(t, 5, res.EventsSkipped)
	assert.Len(t, n.matching(EventCalendarChanged), 1, "no se avisa si no se sumó nada")
}

func TestBackup_OldFileWithoutEvents(t *testing.T) {
	dst, _, _, _ := newTestCore(t)
	p := filepath.Join(t.TempDir(), "viejo.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"app":"`+backupApp+`","kind":"`+backupKind+`","version":1}`), 0o600))
	res, err := dst.ImportBackup(context.Background(), p)
	require.NoError(t, err)
	assert.Equal(t, 0, res.EventsAdded)
}

// La foto de Taskapp del espejo viaja: tras restaurar, el primer refresco no
// lo marca cambiado si en Taskapp no cambió nada.
func TestBackup_RestoresTaskappSnapshot(t *testing.T) {
	ctx := context.Background()
	c, _, smtp, _, acc := meetCore(t)
	mirror := createdWithBea(t, c, smtp, acc)
	snap, err := c.store.TaskappSnapshot(ctx, mirror.ID)
	require.NoError(t, err)
	path := exportTo(t, c)
	require.NoError(t, c.store.DeleteEvent(ctx, mirror.ID))

	res, err := c.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 1, res.EventsAdded)
	restored := mirrorOf(t, c, 77)
	got, err := c.store.TaskappSnapshot(ctx, restored.ID)
	require.NoError(t, err)
	assert.Equal(t, snap, got)

	items := meetingItems(listRange(t, c))
	require.Len(t, items, 1)
	assert.Empty(t, items[0].TaskappChange)
	assert.Empty(t, mirrorOf(t, c, 77).TaskappChange)
}

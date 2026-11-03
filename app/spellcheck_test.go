package app

import (
	"context"
	"path/filepath"
	"testing"

	"postacontrol/core/mail"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
)

func TestAfterSettingsSaved(t *testing.T) {
	cases := []struct {
		name string
		st   model.Settings
		want bool
	}{
		{"prendido por defecto", model.Settings{}, true},
		{"apagado", model.Settings{DisableSpellcheck: true}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []bool
			afterSettingsSaved(c.st, func(on bool) { calls = append(calls, on) })
			if len(calls) != 1 || calls[0] != c.want {
				t.Fatalf("llamadas = %v, quería [%v]", calls, c.want)
			}
		})
	}
}

func TestAfterSettingsSavedSinCorrector(t *testing.T) {
	afterSettingsSaved(model.Settings{}, nil) // no debe entrar en pánico
}

func TestSpellLanguages(t *testing.T) {
	if len(spellLanguages) != 2 || spellLanguages[0] != "es_AR" || spellLanguages[1] != "en_US" {
		t.Fatalf("spellLanguages = %v", spellLanguages)
	}
}

func TestSetSettingsAplicaElCorrector(t *testing.T) {
	core, err := mail.New(mail.Config{DBPath: filepath.Join(t.TempDir(), "p.db"), Secrets: secrets.NewMemory()})
	if err != nil {
		t.Fatal(err)
	}
	var calls []bool
	svc := &MailService{core: core, spellcheck: func(on bool) { calls = append(calls, on) }}
	ctx := context.Background()

	if err := svc.SetSettings(ctx, model.Settings{DisableSpellcheck: true}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] {
		t.Fatalf("llamadas = %v, quería [false]", calls)
	}

	// Si el núcleo falla (base cerrada), el corrector no se toca.
	if err := core.Close(); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetSettings(ctx, model.Settings{}); err == nil {
		t.Fatal("quería un error con el núcleo cerrado")
	}
	if len(calls) != 1 {
		t.Fatalf("no debía llamar al corrector tras un error: %v", calls)
	}
}

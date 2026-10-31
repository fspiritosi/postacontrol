package app

import (
	"context"
	"testing"

	"postacontrol/core/mail"
	"postacontrol/core/model"
)

func TestHandleDrop_IgnoresOtherTargets(t *testing.T) {
	llamadas := 0
	stage := func(context.Context, []string) ([]model.StagedAttachment, error) {
		llamadas++
		return nil, nil
	}
	for _, target := range []string{"", "otro"} {
		if _, ok := handleDrop(context.Background(), stage, target, []string{"/tmp/a.txt"}); ok {
			t.Errorf("target %q: se esperaba ok=false", target)
		}
	}
	if _, ok := handleDrop(context.Background(), stage, ComposeDropTarget, nil); ok {
		t.Error("sin rutas: se esperaba ok=false")
	}
	if llamadas != 0 {
		t.Errorf("stage se llamó %d veces, se esperaba 0", llamadas)
	}
}

func TestHandleDrop_StagesAndReportsErrors(t *testing.T) {
	ok := func(_ context.Context, paths []string) ([]model.StagedAttachment, error) {
		return []model.StagedAttachment{{ID: 7, Filename: paths[0]}}, nil
	}
	p, hecho := handleDrop(context.Background(), ok, ComposeDropTarget, []string{"a.txt"})
	if !hecho || len(p.Attachments) != 1 || p.Attachments[0].ID != 7 || p.Error != "" {
		t.Fatalf("payload inesperado: %+v (hecho=%v)", p, hecho)
	}

	falla := func(context.Context, []string) ([]model.StagedAttachment, error) {
		return nil, &mail.UserError{Code: "x", Msg: "El archivo es demasiado grande."}
	}
	p, hecho = handleDrop(context.Background(), falla, ComposeDropTarget, []string{"a.txt"})
	if !hecho || p.Error != "El archivo es demasiado grande." || len(p.Attachments) != 0 {
		t.Fatalf("payload inesperado: %+v (hecho=%v)", p, hecho)
	}
}

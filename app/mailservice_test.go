package app

import (
	"context"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// En Linux el filtro del diálogo (gtk_file_filter_add_pattern) distingue
// mayúsculas: FOTO.JPG tiene que aparecer igual que foto.jpg.
func TestImagePatterns_BothCases(t *testing.T) {
	got := strings.Split(imagePatterns, ";")
	has := map[string]bool{}
	for _, p := range got {
		has[p] = true
	}
	for _, ext := range []string{"png", "jpg", "jpeg", "gif", "webp"} {
		for _, p := range []string{"*." + ext, "*." + strings.ToUpper(ext)} {
			if !has[p] {
				t.Errorf("falta %q en %q", p, imagePatterns)
			}
		}
	}
}

// Los diálogos se abren sobre la ventana desde la que se pidieron (un
// redactor), no sobre la que tiene el foco; sin ventana en la llamada, la
// activa.
func TestDialogParent(t *testing.T) {
	caller, active := &application.WebviewWindow{}, &application.WebviewWindow{}
	current := func() application.Window { return active }
	ctx := context.WithValue(context.Background(), application.WindowKey, application.Window(caller))
	if got := dialogParent(ctx, current); got != caller {
		t.Fatalf("con ventana en la llamada: got %p, want %p", got, caller)
	}
	if got := dialogParent(context.Background(), current); got != active {
		t.Fatalf("sin ventana en la llamada: got %p, want %p", got, active)
	}
	if got := dialogParent(context.Background(), func() application.Window { return nil }); got != nil {
		t.Fatalf("sin ninguna ventana: got %v", got)
	}
}

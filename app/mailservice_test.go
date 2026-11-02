package app

import (
	"strings"
	"testing"
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

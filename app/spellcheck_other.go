//go:build !linux || gtk3

package app

// applySpellcheck no hace nada fuera de Linux (macOS y Windows traen su
// corrector) ni con -tags gtk3, que usa webkit2gtk-4.1 en vez de webkitgtk-6.0.
func applySpellcheck(enabled bool) []string { return nil }

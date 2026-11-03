//go:build !linux

package app

import "github.com/wailsapp/wails/v3/pkg/application"

var invokeAsync = application.InvokeAsync

// applySpellcheck no hace nada fuera de Linux: macOS y Windows traen su corrector.
func applySpellcheck(enabled bool) []string { return nil }

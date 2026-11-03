package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"postacontrol/core/model"
)

// invokeAsync corre una función en el hilo de la UI sin esperarla.
var invokeAsync = application.InvokeAsync

// spellLanguages son los diccionarios que se piden a WebKit, en orden.
var spellLanguages = []string{"es_AR", "en_US"}

// ApplySpellcheck prende o apaga el corrector del webview. Se puede llamar
// desde cualquier goroutine: el trabajo corre en el hilo de GTK, sin esperarlo.
func ApplySpellcheck(enabled bool) {
	invokeAsync(func() { applySpellcheck(enabled) })
}

// afterSettingsSaved aplica lo que cambia de inmediato al guardar los ajustes.
func afterSettingsSaved(st model.Settings, spellcheck func(enabled bool)) {
	if spellcheck != nil {
		spellcheck(!st.DisableSpellcheck)
	}
}

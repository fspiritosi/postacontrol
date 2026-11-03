//go:build linux

package app

/*
#cgo pkg-config: webkitgtk-6.0
#include <stdlib.h>
#include <webkit/webkit.h>

// setSpellcheck configura el contexto por defecto, que usan los webviews de
// Wails. Debe correr en el hilo de GTK. Devuelve los idiomas cargados, como
// una lista terminada en NULL que se libera con g_strfreev.
static char **setSpellcheck(const char **langs, int enabled) {
	WebKitWebContext *ctx = webkit_web_context_get_default();
	webkit_web_context_set_spell_checking_languages(ctx, langs);
	webkit_web_context_set_spell_checking_enabled(ctx, enabled ? TRUE : FALSE);
	return g_strdupv((gchar **)webkit_web_context_get_spell_checking_languages(ctx));
}
*/
import "C"

import (
	"log/slog"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var invokeAsync = application.InvokeAsync

// applySpellcheck prende o apaga el corrector de WebKit con spellLanguages.
// Devuelve los idiomas que WebKit cargó, o nil si quedó apagado. Tiene que
// correr en el hilo de GTK (ver ApplySpellcheck).
func applySpellcheck(enabled bool) []string {
	langs := C.malloc(C.size_t(len(spellLanguages)+1) * C.size_t(unsafe.Sizeof(uintptr(0))))
	defer C.free(langs)
	arr := unsafe.Slice((**C.char)(langs), len(spellLanguages)+1)
	for i, l := range spellLanguages {
		arr[i] = C.CString(l)
	}
	arr[len(spellLanguages)] = nil
	defer func() {
		for _, p := range arr[:len(spellLanguages)] {
			C.free(unsafe.Pointer(p))
		}
	}()

	flag := C.int(0)
	if enabled {
		flag = 1
	}
	out := C.setSpellcheck((**C.char)(langs), flag)
	defer C.g_strfreev(out)
	var got []string
	for p := out; p != nil && *p != nil; p = (**C.char)(unsafe.Add(unsafe.Pointer(p), unsafe.Sizeof(uintptr(0)))) {
		got = append(got, C.GoString(*p))
	}
	if !enabled {
		slog.Info("corrector apagado")
		return nil
	}
	if len(got) < len(spellLanguages) {
		slog.Warn("corrector: faltan diccionarios", "cargados", got)
	} else {
		slog.Info("corrector", "idiomas", got)
	}
	return got
}

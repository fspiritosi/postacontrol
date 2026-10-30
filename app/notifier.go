package app

import "github.com/wailsapp/wails/v3/pkg/application"

// wailsNotifier traduce los eventos del núcleo a eventos de Wails. Emit no
// vuelve a llamar a Core, como exige el contrato de mail.Notifier.
type wailsNotifier struct {
	app *application.App
}

func (n wailsNotifier) Emit(event string, data any) {
	if data == nil {
		n.app.Event.Emit(event)
		return
	}
	n.app.Event.Emit(event, data)
}

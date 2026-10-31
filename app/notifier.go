package app

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"postacontrol/core/mail"
	"postacontrol/core/model"
)

// wailsNotifier traduce los eventos del núcleo a eventos de Wails. Emit no
// vuelve a llamar a Core, como exige el contrato de mail.Notifier.
type wailsNotifier struct {
	app *application.App
	hub *notifyHub
}

func (n wailsNotifier) Emit(event string, data any) {
	if ev, ok := data.(model.NewMail); ok && event == mail.EventNewMail {
		n.hub.newMail(ev)
	}
	if data == nil {
		n.app.Event.Emit(event)
		return
	}
	n.app.Event.Emit(event, data)
}

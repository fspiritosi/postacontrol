package app

import (
	"context"
	"errors"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"postacontrol/core/mail"
	"postacontrol/core/model"
)

const (
	// ComposeDropTarget es el id del elemento del redactor que acepta archivos.
	ComposeDropTarget = "compose-drop"
	// EventFilesStaged avisa al frontend que los archivos soltados ya se prepararon.
	EventFilesStaged = "files-staged"
)

// FilesStagedPayload es el contenido de EventFilesStaged. Si algo falló,
// Error lleva el mensaje para mostrar y Attachments queda vacío.
type FilesStagedPayload struct {
	Attachments []model.StagedAttachment `json:"attachments"`
	Error       string                   `json:"error"`
}

// handleDrop decide qué hacer con una soltada de archivos. Devuelve false si
// no cae sobre el redactor o no trae rutas (no hay nada que avisar).
func handleDrop(
	ctx context.Context,
	stage func(context.Context, []string) ([]model.StagedAttachment, error),
	targetID string,
	paths []string,
) (FilesStagedPayload, bool) {
	if targetID != ComposeDropTarget || len(paths) == 0 {
		return FilesStagedPayload{}, false
	}
	atts, err := stage(ctx, paths)
	if err != nil {
		var ue *mail.UserError
		if errors.As(err, &ue) {
			return FilesStagedPayload{Error: ue.Msg}, true
		}
		return FilesStagedPayload{Error: "No pudimos adjuntar los archivos."}, true
	}
	return FilesStagedPayload{Attachments: atts}, true
}

// SetupFileDrop registra la soltada de archivos del administrador de archivos
// sobre el redactor. El núcleo del servicio se crea en el arranque, antes de
// que pueda llegar una soltada; si aún no está, se ignora.
func SetupFileDrop(app *application.App, win *application.WebviewWindow, svc *MailService) {
	win.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		if svc.core == nil {
			return
		}
		paths := e.Context().DroppedFiles()
		target := ""
		if d := e.Context().DropTargetDetails(); d != nil {
			target = d.ElementID
		}
		// En una goroutine: copiar archivos grandes no debe frenar la UI.
		go func() {
			if p, ok := handleDrop(context.Background(), svc.core.StageFiles, target, paths); ok {
				app.Event.Emit(EventFilesStaged, p)
			}
		}()
	})
}

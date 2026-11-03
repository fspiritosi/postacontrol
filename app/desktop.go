package app

import (
	_ "embed"
	"slices"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed tray.png
var trayIcon []byte

// autostartID es el nombre del .desktop de inicio al abrir sesión.
const autostartID = "postacontrol"

// StartHidden dice si la app arrancó con --hidden (inicio al abrir sesión):
// en ese caso la ventana nace escondida y queda solo la bandeja. Si además
// viene un mailto:, el usuario quiere el redactor: la ventana no se esconde.
func StartHidden(args []string) bool {
	return slices.Contains(args, "--hidden") && len(MailtoArgs(args)) == 0
}

// trayTooltip es el texto fijo del ícono de la bandeja. En Linux, Wails 3.0.0
// no implementa SetTooltip (es un stub vacío), así que el contador de no
// leídos queda para cuando Wails lo soporte.
const trayTooltip = "PostaControl"

// Desktop agrupa la integración con el escritorio: ventana principal y
// bandeja del sistema. Lo crea SetupDesktop; la Task 5 (notificaciones) usa
// ShowWindow y WindowVisibleAndFocused.
type Desktop struct {
	app  *application.App
	win  *application.WebviewWindow
	tray *application.SystemTray
}

// ShowWindow muestra la ventana principal, la des-minimiza y le da foco.
func (d *Desktop) ShowWindow() {
	d.win.Show()
	if d.win.IsMinimised() {
		d.win.UnMinimise()
	}
	d.win.Focus()
}

func (d *Desktop) windowVisible() bool { return d.win.IsVisible() && !d.win.IsMinimised() }
func (d *Desktop) windowFocused() bool { return d.win.IsFocused() }

// WindowVisibleAndFocused dice si el usuario está viendo la ventana ahora.
func (d *Desktop) WindowVisibleAndFocused() bool {
	return d.win.IsVisible() && d.win.IsFocused() && !d.win.IsMinimised()
}

// QuitKeyBindings devuelve los atajos de la ventana principal (Ctrl+Q sale de
// verdad). Hay que pasarlos en WebviewWindowOptions.KeyBindings: solo esa vía
// normaliza el atajo como lo busca el manejador de teclas de Linux
// (app.KeyBinding.Add guarda el texto crudo y no coincide).
func QuitKeyBindings(quit func()) map[string]func(application.Window) {
	return map[string]func(application.Window){
		"ctrl+q": func(application.Window) { quit() },
	}
}

// SetupDesktop arma la bandeja (ícono, tooltip fijo, menú, clic) y hace que
// cerrar la ventana la esconda en vez de salir. "Salir" llama a quit, que
// espera a que los redactores guarden. Devuelve el *Desktop.
func SetupDesktop(app *application.App, win *application.WebviewWindow, quit func()) *Desktop {
	d := &Desktop{app: app, win: win}

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
	})

	d.tray = app.SystemTray.New()
	d.tray.SetIcon(trayIcon)
	d.tray.SetTooltip(trayTooltip)
	menu := app.NewMenu()
	menu.Add("Abrir PostaControl").OnClick(func(*application.Context) { d.ShowWindow() })
	menu.AddSeparator()
	menu.Add("Salir").OnClick(func(*application.Context) { quit() })
	d.tray.SetMenu(menu)
	d.tray.OnClick(d.ShowWindow)
	return d
}

// GetStartAtLogin dice si la app está registrada para abrirse al iniciar sesión.
func (s *MailService) GetStartAtLogin() (bool, error) {
	return s.app.Autostart.IsEnabled()
}

// SetStartAtLogin registra o quita el inicio al abrir sesión. La app arranca
// con --hidden para quedar solo en la bandeja.
func (s *MailService) SetStartAtLogin(enabled bool) error {
	if !enabled {
		return s.app.Autostart.Disable()
	}
	return s.app.Autostart.EnableWithOptions(application.AutostartOptions{
		Identifier: autostartID,
		Arguments:  []string{"--hidden"},
	})
}

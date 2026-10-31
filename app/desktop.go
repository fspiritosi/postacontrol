package app

import (
	"context"
	_ "embed"
	"fmt"
	"slices"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"postacontrol/core/mail"
)

//go:embed tray.png
var trayIcon []byte

// autostartID es el nombre del .desktop de inicio al abrir sesión.
const autostartID = "postacontrol"

// StartHidden dice si la app arrancó con --hidden (inicio al abrir sesión):
// en ese caso la ventana nace escondida y queda solo la bandeja.
func StartHidden(args []string) bool {
	return slices.Contains(args, "--hidden")
}

// TrayTooltip arma el texto del ícono de la bandeja según los no leídos.
func TrayTooltip(unread int) string {
	switch {
	case unread <= 0:
		return "PostaControl"
	case unread == 1:
		return "PostaControl — 1 correo sin leer"
	default:
		return fmt.Sprintf("PostaControl — %d correos sin leer", unread)
	}
}

// Desktop agrupa la integración con el escritorio: ventana principal y
// bandeja del sistema. Lo crea SetupDesktop; la Task 5 (notificaciones) usa
// ShowWindow y WindowVisibleAndFocused.
type Desktop struct {
	app  *application.App
	win  *application.WebviewWindow
	tray *application.SystemTray
	svc  *MailService
}

// ShowWindow muestra la ventana principal, la des-minimiza y le da foco.
func (d *Desktop) ShowWindow() {
	d.win.Show()
	if d.win.IsMinimised() {
		d.win.UnMinimise()
	}
	d.win.Focus()
}

// WindowVisibleAndFocused dice si el usuario está viendo la ventana ahora.
func (d *Desktop) WindowVisibleAndFocused() bool {
	return d.win.IsVisible() && d.win.IsFocused() && !d.win.IsMinimised()
}

// SetupDesktop arma la bandeja (ícono, tooltip, menú, clic), hace que cerrar
// la ventana la esconda en vez de salir, mapea Ctrl+Q a salir de verdad y
// mantiene el tooltip con los no leídos de la Entrada. Devuelve el *Desktop.
func SetupDesktop(app *application.App, win *application.WebviewWindow, svc *MailService) *Desktop {
	d := &Desktop{app: app, win: win, svc: svc}

	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
	})

	app.KeyBinding.Add("ctrl+q", func(application.Window) { app.Quit() })

	d.tray = app.SystemTray.New()
	d.tray.SetIcon(trayIcon)
	d.tray.SetTooltip(TrayTooltip(0))
	menu := app.NewMenu()
	menu.Add("Abrir PostaControl").OnClick(func(*application.Context) { d.ShowWindow() })
	menu.AddSeparator()
	menu.Add("Salir").OnClick(func(*application.Context) { app.Quit() })
	d.tray.SetMenu(menu)
	d.tray.OnClick(d.ShowWindow)

	refresh := func(*application.CustomEvent) { go d.refreshTooltip() }
	app.Event.On(mail.EventFolderUpdated, refresh)
	app.Event.On(mail.EventFoldersUpdated, refresh)
	// Tooltip inicial: ServiceStartup ya corrió cuando la app arranca.
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { go d.refreshTooltip() })
	return d
}

// refreshTooltip recalcula los no leídos de la Entrada y actualiza el tooltip.
func (d *Desktop) refreshTooltip() {
	if d.svc == nil || d.svc.core == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	n, err := d.svc.core.InboxUnreadTotal(ctx)
	if err != nil {
		return
	}
	d.tray.SetTooltip(TrayTooltip(n))
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

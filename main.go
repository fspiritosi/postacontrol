package main

import (
	"embed"
	"log"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"

	"postacontrol/app"
)

// Wails uses Go's `embed` package to embed the frontend files into the binary.
// Any files in the frontend/dist folder will be embedded into the binary and
// made available to the frontend.
// See https://pkg.go.dev/embed for more information.

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

// main inicializa la aplicación, crea la ventana principal y corre el loop de Wails.
func main() {

	// Create a new Wails application by providing the necessary options.
	// Variables 'Name' and 'Description' are for application metadata.
	// 'Assets' configures the asset server with the 'FS' variable pointing to the frontend files.
	// 'Bind' is a list of Go struct instances. The frontend has access to the methods of these instances.
	// 'Mac' options tailor the application when running an macOS.
	mailService := app.NewMailService()
	var desktop *app.Desktop
	notifier := notifications.New()
	wailsApp := application.New(application.Options{
		Name:        "PostaControl",
		Description: "Cliente de correo personal",
		// Explícito: en Debug, Wails registra los argumentos de cada llamada
		// enlazada, y AddAccount/TestAccount llevan la contraseña.
		LogLevel: slog.LevelInfo,
		Icon:     appIcon,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Services: []application.Service{
			application.NewService(mailService),
			application.NewService(notifier),
		},
		// Con Wayland, GNOME asocia la ventana al .desktop por este id.
		Linux: application.LinuxOptions{
			ApplicationID: "ar.com.codecontrol.postacontrol",
			ProgramName:   "postacontrol",
		},
		Mac: application.MacOptions{
			// Cerrar la ventana solo la esconde: la app vive en la bandeja.
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "ar.com.codecontrol.postacontrol",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if desktop != nil {
					desktop.ShowWindow()
				}
			},
		},
	})

	// Create a new window with the necessary options.
	// 'Title' is the title of the window.
	// 'Mac' options tailor the window when running on macOS.
	// 'BackgroundColour' is the background colour of the window.
	// 'URL' is the URL that will be loaded into the webview.
	win := wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "PostaControl",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		KeyBindings:      app.QuitKeyBindings(wailsApp.Quit),
		Hidden:           app.StartHidden(os.Args),
		BackgroundColour: application.NewRGB(6, 7, 15),
		URL:              "/",
	})

	desktop = app.SetupDesktop(wailsApp, win)
	app.SetupNotifications(wailsApp, mailService, desktop, notifier)

	// Run the application. This blocks until the application has been exited.
	err := wailsApp.Run()

	// If an error occurred while running the application, log it and exit.
	if err != nil {
		log.Fatal(err)
	}
}

package app

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"postacontrol/core/mail"
)

const (
	// EventComposeCloseRequest le pide a un redactor que cierre como con Esc
	// (guardar o preguntar): llegó la ✕ de su ventana.
	EventComposeCloseRequest = "compose:close-request"
	// EventComposeFlush le pide a un redactor que guarde todo antes de salir y
	// responda con ComposeFlushed.
	EventComposeFlush = "compose:flush"
	// EventComposeQuitBlocked avisa a un redactor que la salida se canceló por
	// él; data es el mensaje para mostrar.
	EventComposeQuitBlocked = "compose:quit-blocked"
	// EventComposeSendStarted avisa a todas las ventanas que se empezó a
	// enviar un correo; data es su id en la Bandeja de salida.
	EventComposeSendStarted = "compose:send-started"
	// EventComposeNotice le pasa a todas las ventanas un aviso de un redactor
	// que se está cerrando, para que lo muestre la principal; data es un
	// ComposeNoticeData.
	EventComposeNotice = "compose:notice"

	// composeQuitTimeout es cuánto espera la salida a que respondan los redactores.
	composeQuitTimeout = 10 * time.Second
	// msgFlushNoResponse es el aviso para un redactor que no respondió a tiempo.
	msgFlushNoResponse = "No pudimos guardar este borrador; la app sigue abierta."
)

// WindowBackground es el fondo de todas las ventanas, mientras carga el webview.
var WindowBackground = application.NewRGB(6, 7, 15)

// ComposeNoticeData es el aviso de compose:notice. Kind es "success" o "error".
type ComposeNoticeData struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// composeNotice arma el aviso; un kind desconocido o un mensaje vacío no avisan.
func composeNotice(kind, message string) (ComposeNoticeData, bool) {
	if (kind != "success" && kind != "error") || message == "" {
		return ComposeNoticeData{}, false
	}
	return ComposeNoticeData{Kind: kind, Message: message}, true
}

// composeWindow es lo que el registro necesita de una ventana del redactor.
type composeWindow interface {
	Focus()
	Close()
	// EmitEvent manda un evento solo a esta ventana.
	EmitEvent(name string, data ...any) bool
	// SetAllowClose decide si la ✕ cierra la ventana o le pide al redactor
	// que cierre.
	SetAllowClose(bool)
}

type composeEntry struct {
	win  composeWindow // nil mientras se crea
	init string
	key  string
}

type flushReply struct {
	id  int
	ok  bool
	msg string
}

// quitRound es una salida en curso: espera la respuesta de cada redactor.
type quitRound struct {
	pending map[int]bool
	replies chan flushReply
}

// ComposeWindows lleva las ventanas del redactor abiertas: qué datos inicia
// cada una, la clave que evita abrir dos veces el mismo borrador y la salida
// de la app, que espera a que todas guarden.
type ComposeWindows struct {
	create  func(id int) composeWindow
	quit    func()
	timeout time.Duration

	mu       sync.Mutex
	nextID   int
	wins     map[int]*composeEntry
	quitting *quitRound
}

func newComposeWindows(create func(id int) composeWindow, quit func(), timeout time.Duration) *ComposeWindows {
	return &ComposeWindows{create: create, quit: quit, timeout: timeout, wins: map[int]*composeEntry{}}
}

// Open abre un redactor con initJSON. Si key no está vacía y ya hay una
// ventana con esa clave, la trae adelante y devuelve su id.
func (r *ComposeWindows) Open(initJSON, key string) (int, error) {
	r.mu.Lock()
	if key != "" {
		for id, e := range r.wins {
			if e.key == key {
				win := e.win
				r.mu.Unlock()
				if win != nil {
					win.Focus()
				}
				return id, nil
			}
		}
	}
	r.nextID++
	id := r.nextID
	e := &composeEntry{init: initJSON, key: key}
	r.wins[id] = e
	r.mu.Unlock()

	// Sin el candado: crear la ventana pasa por el hilo de la UI.
	win := r.create(id)
	r.mu.Lock()
	alive := r.wins[id] == e
	if alive {
		e.win = win
	}
	r.mu.Unlock()
	if !alive {
		// Se cerró mientras se creaba.
		win.SetAllowClose(true)
		win.Close()
	}
	return id, nil
}

// Init devuelve los datos con que arranca el redactor de la ventana id.
func (r *ComposeWindows) Init(id int) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.wins[id]
	if !ok {
		return "", &mail.UserError{Code: "not_found", Msg: "Esta ventana del redactor ya no existe."}
	}
	return e.init, nil
}

// SetKey registra la clave de la ventana id (por ejemplo, después del primer
// autoguardado de un correo nuevo).
func (r *ComposeWindows) SetKey(id int, key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, ok := r.wins[id]; ok {
		e.key = key
	}
}

// Close cierra la ventana id sin volver a preguntarle al redactor y la saca
// del registro.
func (r *ComposeWindows) Close(id int) {
	win := r.forget(id)
	if win != nil {
		win.SetAllowClose(true)
		win.Close()
	}
}

// Closed saca del registro una ventana que ya no existe.
func (r *ComposeWindows) Closed(id int) { r.forget(id) }

// forget saca la ventana id del registro. Si hay una salida en curso, una
// ventana que se cerró cuenta como guardada.
func (r *ComposeWindows) forget(id int) composeWindow {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.wins[id]
	if !ok {
		return nil
	}
	delete(r.wins, id)
	r.replyLocked(flushReply{id: id, ok: true})
	return e.win
}

// Flushed es la respuesta de un redactor a EventComposeFlush.
func (r *ComposeWindows) Flushed(id int, ok bool, message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replyLocked(flushReply{id: id, ok: ok, msg: message})
}

func (r *ComposeWindows) replyLocked(rep flushReply) {
	q := r.quitting
	if q == nil || !q.pending[rep.id] {
		return
	}
	delete(q.pending, rep.id)
	q.replies <- rep // tiene lugar para una respuesta por ventana
}

// Quit sale de la app. Sin redactores abiertos sale enseguida; si no, les
// pide a todos que guarden y sale solo si todos responden bien antes del
// timeout. Si no, trae adelante el primero que falló o no respondió y le
// avisa por qué la app sigue abierta. No bloquea: la espera corre aparte.
func (r *ComposeWindows) Quit() {
	r.mu.Lock()
	if r.quitting != nil {
		r.mu.Unlock()
		return // ya hay una salida esperando
	}
	q := &quitRound{pending: map[int]bool{}}
	targets := map[int]composeWindow{}
	for id, e := range r.wins {
		if e.win != nil {
			q.pending[id] = true
			targets[id] = e.win
		}
	}
	if len(targets) == 0 {
		r.mu.Unlock()
		r.quit()
		return
	}
	q.replies = make(chan flushReply, len(targets))
	r.quitting = q
	r.mu.Unlock()

	go r.waitFlush(q, targets)
}

func (r *ComposeWindows) waitFlush(q *quitRound, targets map[int]composeWindow) {
	ids := make([]int, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		targets[id].EmitEvent(EventComposeFlush)
	}

	timer := time.NewTimer(r.timeout)
	defer timer.Stop()
	var failed *flushReply
wait:
	for answered := 0; answered < len(ids) && failed == nil; {
		select {
		case rep := <-q.replies:
			answered++
			if !rep.ok {
				failed = &rep
			}
		case <-timer.C:
			failed = r.firstPending(q, ids)
			break wait
		}
	}

	r.mu.Lock()
	r.quitting = nil
	r.mu.Unlock()

	if failed == nil {
		r.quit()
		return
	}
	win := targets[failed.id]
	win.Focus()
	win.EmitEvent(EventComposeQuitBlocked, failed.msg)
}

// firstPending es, al vencer el tiempo, el primer redactor que no respondió.
// Si respondieron todos justo al vencer, mira las respuestas que quedaron sin
// leer: nil si todas fueron bien.
func (r *ComposeWindows) firstPending(q *quitRound, ids []int) *flushReply {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, id := range ids {
		if q.pending[id] {
			return &flushReply{id: id, msg: msgFlushNoResponse}
		}
	}
	for {
		select {
		case rep := <-q.replies:
			if !rep.ok {
				return &rep
			}
		default:
			return nil
		}
	}
}

// wailsComposeWindow es una ventana del redactor de verdad.
type wailsComposeWindow struct {
	win        *application.WebviewWindow
	allowClose atomic.Bool
}

func (w *wailsComposeWindow) Focus() {
	w.win.Show()
	if w.win.IsMinimised() {
		w.win.UnMinimise()
	}
	w.win.Focus()
}

func (w *wailsComposeWindow) Close() { w.win.Close() }

func (w *wailsComposeWindow) SetAllowClose(v bool) { w.allowClose.Store(v) }

func (w *wailsComposeWindow) EmitEvent(name string, data ...any) bool {
	return emitToWindow(w.win, name, data...)
}

// emitToWindow manda un evento solo al webview de win. En Wails 3.0.0,
// WebviewWindow.EmitEvent lo manda a todas las ventanas (solo le pone el
// nombre de la ventana como Sender); DispatchWailsEvent es la entrega a una.
func emitToWindow(win *application.WebviewWindow, name string, data ...any) bool {
	event := &application.CustomEvent{Name: name, Sender: win.Name()}
	if len(data) == 1 {
		event.Data = data[0]
	} else if len(data) > 1 {
		event.Data = data
	}
	win.DispatchWailsEvent(event)
	return false
}

// SetupComposeWindows arma el registro de redactores con ventanas de Wails.
// Cada ventana carga /#/compose/<id>, recibe sus propios archivos soltados y
// pasa la ✕ al redactor, que decide si guardar o preguntar.
func SetupComposeWindows(app *application.App, svc *MailService) *ComposeWindows {
	var r *ComposeWindows
	r = newComposeWindows(func(id int) composeWindow {
		win := app.Window.NewWithOptions(application.WebviewWindowOptions{
			Name:             fmt.Sprintf("compose-%d", id),
			Title:            "Nuevo correo",
			Width:            900,
			Height:           720,
			MinWidth:         640,
			MinHeight:        480,
			KeyBindings:      QuitKeyBindings(r.Quit),
			BackgroundColour: WindowBackground,
			URL:              fmt.Sprintf("/#/compose/%d", id),
			EnableFileDrop:   true,
		})
		cw := &wailsComposeWindow{win: win}
		win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
			if !cw.allowClose.Load() {
				e.Cancel()
				cw.EmitEvent(EventComposeCloseRequest)
			}
		})
		SetupFileDrop(app, win, svc)
		// Wails 3.0.0 no tiene WindowClosed: los listeners de WindowClosing
		// corren solo si ningún hook lo canceló, es decir, cuando la ventana
		// se cierra de verdad (es la misma señal con que Wails la destruye).
		win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
			r.Closed(id)
		})
		return cw
	}, app.Quit, composeQuitTimeout)
	return r
}

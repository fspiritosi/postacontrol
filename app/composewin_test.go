package app

import (
	"errors"
	"sync"
	"testing"
	"time"

	"postacontrol/core/mail"
)

type emitted struct {
	name string
	data []any
}

// fakeComposeWindow registra lo que el registro le pide a una ventana.
type fakeComposeWindow struct {
	mu         sync.Mutex
	id         int
	focus      int
	closed     int
	allowClose bool
	events     []emitted
}

func (w *fakeComposeWindow) Focus() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.focus++
}

func (w *fakeComposeWindow) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed++
}

func (w *fakeComposeWindow) SetAllowClose(v bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.allowClose = v
}

func (w *fakeComposeWindow) EmitEvent(name string, data ...any) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.events = append(w.events, emitted{name, data})
	return false
}

func (w *fakeComposeWindow) focusCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.focus
}

func (w *fakeComposeWindow) named(name string) []emitted {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []emitted
	for _, e := range w.events {
		if e.name == name {
			out = append(out, e)
		}
	}
	return out
}

// composeHarness arma un registro con ventanas falsas y cuenta las salidas.
type composeHarness struct {
	r     *ComposeWindows
	mu    sync.Mutex
	wins  map[int]*fakeComposeWindow
	quits int
}

func newComposeHarness(timeout time.Duration) *composeHarness {
	h := &composeHarness{wins: map[int]*fakeComposeWindow{}}
	h.r = newComposeWindows(func(id int) composeWindow {
		w := &fakeComposeWindow{id: id}
		h.mu.Lock()
		h.wins[id] = w
		h.mu.Unlock()
		return w
	}, func() {
		h.mu.Lock()
		h.quits++
		h.mu.Unlock()
	}, timeout)
	return h
}

func (h *composeHarness) win(t *testing.T, id int) *fakeComposeWindow {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	w, ok := h.wins[id]
	if !ok {
		t.Fatalf("no se creó la ventana %d", id)
	}
	return w
}

func (h *composeHarness) created() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.wins)
}

func (h *composeHarness) quitCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.quits
}

func (h *composeHarness) open(t *testing.T, initJSON, key string) int {
	t.Helper()
	id, err := h.r.Open(initJSON, key)
	if err != nil {
		t.Fatalf("Open(%q, %q): %v", initJSON, key, err)
	}
	return id
}

// eventually espera hasta 2 s a que cond se cumpla.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("no pasó: %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestComposeWindows_OpenStoresInit(t *testing.T) {
	h := newComposeHarness(time.Second)
	id := h.open(t, `{"mode":"new"}`, "")
	got, err := h.r.Init(id)
	if err != nil || got != `{"mode":"new"}` {
		t.Fatalf("Init(%d) = %q, %v", id, got, err)
	}
	_, err = h.r.Init(id + 100)
	var ue *mail.UserError
	if !errors.As(err, &ue) || ue.Code != "not_found" {
		t.Fatalf("Init de un id desconocido: %v, quería not_found", err)
	}
}

func TestComposeWindows_DedupByKey(t *testing.T) {
	h := newComposeHarness(time.Second)
	id1 := h.open(t, "a", "draft:5")
	again := h.open(t, "b", "draft:5")
	if again != id1 || h.created() != 1 {
		t.Fatalf("misma clave: ids %d y %d, %d ventanas", id1, again, h.created())
	}
	if h.win(t, id1).focusCount() != 1 {
		t.Fatal("la segunda apertura no trajo adelante la ventana")
	}
	if got, _ := h.r.Init(id1); got != "a" {
		t.Fatalf("la segunda apertura pisó el init: %q", got)
	}

	id2 := h.open(t, "otro", "")
	h.r.SetKey(id2, "draft:9")
	if got := h.open(t, "c", "draft:9"); got != id2 {
		t.Fatalf("Open con la clave registrada después = %d, quería %d", got, id2)
	}
	if h.win(t, id2).focusCount() != 1 {
		t.Fatal("no enfocó la ventana con la clave nueva")
	}

	x1 := h.open(t, "x", "")
	x2 := h.open(t, "x", "")
	if x1 == x2 || h.created() != 4 {
		t.Fatalf("sin clave: ids %d y %d, %d ventanas", x1, x2, h.created())
	}
}

func TestComposeWindows_CloseAndClosedForget(t *testing.T) {
	h := newComposeHarness(time.Second)
	id := h.open(t, "a", "draft:1")
	h.r.Close(id)
	w := h.win(t, id)
	w.mu.Lock()
	allowed, closed := w.allowClose, w.closed
	w.mu.Unlock()
	if !allowed || closed != 1 {
		t.Fatalf("Close: allowClose=%v closed=%d", allowed, closed)
	}
	if _, err := h.r.Init(id); err == nil {
		t.Fatal("Close no sacó la ventana del registro")
	}
	reopened := h.open(t, "b", "draft:1")
	if reopened == id || h.created() != 2 {
		t.Fatalf("después de Close, la misma clave no creó otra ventana (id %d)", reopened)
	}

	h.r.Closed(reopened)
	third := h.open(t, "c", "draft:1")
	if third == reopened || h.created() != 3 {
		t.Fatalf("después de Closed, la misma clave no creó otra ventana (id %d)", third)
	}
}

func TestComposeWindows_QuitWithoutWindows(t *testing.T) {
	h := newComposeHarness(time.Second)
	h.r.Quit()
	eventually(t, "quit llamado", func() bool { return h.quitCount() == 1 })
	time.Sleep(20 * time.Millisecond)
	if n := h.quitCount(); n != 1 {
		t.Fatalf("quit se llamó %d veces", n)
	}
}

func TestComposeWindows_QuitAllOK(t *testing.T) {
	h := newComposeHarness(time.Second)
	a := h.open(t, "a", "")
	b := h.open(t, "b", "")
	h.r.Quit()
	for _, id := range []int{a, b} {
		w := h.win(t, id)
		eventually(t, "compose:flush emitido", func() bool { return len(w.named("compose:flush")) == 1 })
	}
	h.r.Flushed(a, true, "")
	time.Sleep(20 * time.Millisecond)
	if h.quitCount() != 0 {
		t.Fatal("salió antes de que respondieran todas")
	}
	h.r.Flushed(b, true, "")
	eventually(t, "quit llamado", func() bool { return h.quitCount() == 1 })
}

func TestComposeWindows_QuitBlocked(t *testing.T) {
	h := newComposeHarness(time.Second)
	a := h.open(t, "a", "")
	b := h.open(t, "b", "")
	h.r.Quit()
	wb := h.win(t, b)
	eventually(t, "compose:flush emitido", func() bool { return len(wb.named("compose:flush")) == 1 })
	h.r.Flushed(a, true, "")
	h.r.Flushed(b, false, "msg")
	eventually(t, "compose:quit-blocked", func() bool { return len(wb.named("compose:quit-blocked")) == 1 })
	blocked := wb.named("compose:quit-blocked")[0]
	if len(blocked.data) != 1 || blocked.data[0] != "msg" {
		t.Fatalf("compose:quit-blocked con %v, quería \"msg\"", blocked.data)
	}
	if wb.focusCount() != 1 {
		t.Fatal("no trajo adelante la ventana que falló")
	}
	if len(h.win(t, a).named("compose:quit-blocked")) != 0 {
		t.Fatal("avisó a la ventana que sí guardó")
	}
	time.Sleep(20 * time.Millisecond)
	if h.quitCount() != 0 {
		t.Fatal("salió aunque una ventana no pudo guardar")
	}
}

func TestComposeWindows_QuitTimeout(t *testing.T) {
	h := newComposeHarness(50 * time.Millisecond)
	a := h.open(t, "a", "")
	h.r.Quit()
	w := h.win(t, a)
	eventually(t, "compose:quit-blocked", func() bool { return len(w.named("compose:quit-blocked")) == 1 })
	blocked := w.named("compose:quit-blocked")[0]
	want := "No pudimos guardar este borrador; la app sigue abierta."
	if len(blocked.data) != 1 || blocked.data[0] != want {
		t.Fatalf("compose:quit-blocked con %v, quería %q", blocked.data, want)
	}
	if w.focusCount() != 1 {
		t.Fatal("no trajo adelante la ventana que no respondió")
	}
	if h.quitCount() != 0 {
		t.Fatal("salió aunque una ventana no respondió")
	}
	// Una respuesta tardía no hace salir: la salida ya se canceló.
	h.r.Flushed(a, true, "")
	time.Sleep(20 * time.Millisecond)
	if h.quitCount() != 0 {
		t.Fatal("una respuesta tardía hizo salir")
	}
}

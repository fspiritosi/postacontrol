package testenv

import (
	"bytes"
	"crypto/tls"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"

	"postacontrol/core/model"
)

// IMAP es un servidor IMAP en memoria con TLS implícito.
type IMAP struct {
	Addr     string
	Settings model.ServerSettings
	User     *imapmemserver.User
	// Close baja el servidor y corta las conexiones abiertas. Restart lo
	// vuelve a levantar en la misma dirección.
	Close func()

	mem       *imapmemserver.Server
	tlsServer *tls.Config
	caps      imap.CapSet

	mu     sync.Mutex
	srv    *imapserver.Server
	ln     net.Listener
	served chan struct{} // se cierra cuando Serve volvió

	sessions atomic.Int64

	// Lo que imapmemserver no tiene y se simula en session (ver noselect.go).
	simMu    sync.Mutex
	noselect map[string]bool
	ops      []string
	moveHook func(src string)
}

// Sessions devuelve cuántas conexiones aceptó el servidor desde que arrancó
// (Restart no lo reinicia). Sirve para verificar que algo no fue a la red.
func (s *IMAP) Sessions() int64 { return s.sessions.Load() }

// StartIMAP levanta el servidor con INBOX y "Sent" vacíos. Se cierra solo al
// terminar el test.
func StartIMAP(t testing.TB, tlsc TLS, username, password string) *IMAP {
	t.Helper()
	return startIMAP(t, tlsc, username, password, imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}})
}

// StartIMAPRev1 es como StartIMAP pero anuncia solo IMAP4rev1: sin MOVE ni
// UIDPLUS.
func StartIMAPRev1(t testing.TB, tlsc TLS, username, password string) *IMAP {
	t.Helper()
	return startIMAP(t, tlsc, username, password, imap.CapSet{imap.CapIMAP4rev1: {}})
}

// StartIMAPWithCaps es como StartIMAP pero anuncia exactamente las
// capacidades dadas.
func StartIMAPWithCaps(t testing.TB, tlsc TLS, username, password string, caps imap.CapSet) *IMAP {
	t.Helper()
	return startIMAP(t, tlsc, username, password, caps)
}

func startIMAP(t testing.TB, tlsc TLS, username, password string, caps imap.CapSet) *IMAP {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser(username, password)
	for _, name := range []string{"INBOX", "Sent"} {
		if err := user.Create(name, nil); err != nil {
			t.Fatalf("testenv: crear %s: %v", name, err)
		}
	}
	mem.AddUser(user)

	s := &IMAP{User: user, mem: mem, tlsServer: tlsc.Server, caps: caps}
	s.Close = s.stop
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsc.Server)
	if err != nil {
		t.Fatalf("testenv: escuchar IMAP: %v", err)
	}
	s.serve(ln)

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	s.Addr = ln.Addr().String()
	s.Settings = model.ServerSettings{Host: host, Port: port, Security: model.SecurityTLS}
	t.Cleanup(s.Close)
	return s
}

func (s *IMAP) serve(ln net.Listener) {
	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			s.sessions.Add(1)
			return &session{SessionIMAP4rev2: s.mem.NewSession().(imapserver.SessionIMAP4rev2), s: s}, nil, nil
		},
		Caps: s.caps,
		// El servidor no sabe que el listener ya es TLS, así que sin esto
		// rechazaría LOGIN. El cliente igual verifica el certificado.
		InsecureAuth: true,
	})
	served := make(chan struct{})
	s.mu.Lock()
	s.srv, s.ln, s.served = srv, ln, served
	s.mu.Unlock()
	go func() { _ = srv.Serve(ln); _ = ln.Close(); close(served) }()
}

func (s *IMAP) stop() {
	s.mu.Lock()
	srv, ln, served := s.srv, s.ln, s.served
	s.srv, s.ln, s.served = nil, nil, nil
	s.mu.Unlock()
	if srv == nil {
		return
	}
	// go-imap (v2.0.0-beta.8) hace el Add del WaitGroup de Serve fuera del
	// lock: si Close llega justo después de arrancar, su Wait compite con ese
	// Add. Cerrar el listener y esperar a que Serve vuelva lo evita; Close
	// después solo corta las conexiones abiertas.
	_ = ln.Close()
	<-served
	_ = srv.Close()
}

// Restart vuelve a levantar el servidor en la misma dirección, con los mismos
// buzones y mensajes. Si estaba levantado, primero lo baja.
func (s *IMAP) Restart(t testing.TB) {
	t.Helper()
	s.stop()
	var (
		ln  net.Listener
		err error
	)
	// El puerto recién liberado puede tardar un instante en volver a estar libre.
	for range 50 {
		if ln, err = tls.Listen("tcp", s.Addr, s.tlsServer); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("testenv: volver a escuchar IMAP en %s: %v", s.Addr, err)
	}
	s.serve(ln)
}

// Deliver agrega un mensaje crudo (RFC 5322, con CRLF) a un buzón, sin pasar
// por el protocolo.
func (s *IMAP) Deliver(t testing.TB, mailbox string, raw []byte, flags ...imap.Flag) {
	t.Helper()
	s.DeliverAt(t, mailbox, raw, time.Time{}, flags...)
}

// DeliverAt es como Deliver pero fija el INTERNALDATE (cero = ahora).
func (s *IMAP) DeliverAt(t testing.TB, mailbox string, raw []byte, when time.Time, flags ...imap.Flag) {
	t.Helper()
	lit := &literal{Reader: bytes.NewReader(raw), size: int64(len(raw))}
	if _, err := s.User.Append(mailbox, lit, &imap.AppendOptions{Flags: flags, Time: when}); err != nil {
		t.Fatalf("testenv: entregar a %s: %v", mailbox, err)
	}
}

// Expunge borra un mensaje del buzón como lo haría otro cliente (marca
// \Deleted y hace EXPUNGE), sin pasar por el protocolo.
func (s *IMAP) Expunge(t testing.TB, mailbox string, uid uint32) {
	t.Helper()
	sess := imapmemserver.NewUserSession(s.User)
	defer func() { _ = sess.Close() }()
	if _, err := sess.Select(mailbox, nil); err != nil {
		t.Fatalf("testenv: seleccionar %s: %v", mailbox, err)
	}
	set := imap.UIDSetNum(imap.UID(uid))
	del := &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}
	if err := sess.Store(nil, set, del, nil); err != nil {
		t.Fatalf("testenv: marcar UID %d como borrado: %v", uid, err)
	}
	if err := sess.Expunge(nil, &set); err != nil {
		t.Fatalf("testenv: expunge en %s: %v", mailbox, err)
	}
}

// RecreateMailbox borra el buzón y lo vuelve a crear vacío. El servidor en
// memoria no deja elegir el UIDVALIDITY, pero al recrear un buzón siempre le
// asigna uno nuevo (RFC 9051): devuelve ese valor.
func (s *IMAP) RecreateMailbox(t testing.TB, name string) uint32 {
	t.Helper()
	if err := s.User.Delete(name); err != nil {
		t.Fatalf("testenv: borrar %s: %v", name, err)
	}
	if err := s.User.Create(name, nil); err != nil {
		t.Fatalf("testenv: crear %s: %v", name, err)
	}
	st, err := s.User.Status(name, &imap.StatusOptions{UIDValidity: true})
	if err != nil {
		t.Fatalf("testenv: status %s: %v", name, err)
	}
	return st.UIDValidity
}

type literal struct {
	*bytes.Reader
	size int64
}

func (l *literal) Size() int64 { return l.size }

// DeleteMailbox borra el buzón (con sus mensajes) sin pasar por el protocolo.
func (s *IMAP) DeleteMailbox(t testing.TB, name string) {
	t.Helper()
	if err := s.User.Delete(name); err != nil {
		t.Fatalf("testenv: borrar %s: %v", name, err)
	}
}

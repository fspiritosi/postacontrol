package testenv

import (
	"bytes"
	"crypto/tls"
	"net"
	"strconv"
	"testing"

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
	Close    func()
}

// StartIMAP levanta el servidor con INBOX y "Sent" vacíos. Se cierra solo al
// terminar el test.
func StartIMAP(t testing.TB, tlsc TLS, username, password string) *IMAP {
	t.Helper()
	mem := imapmemserver.New()
	user := imapmemserver.NewUser(username, password)
	for _, name := range []string{"INBOX", "Sent"} {
		if err := user.Create(name, nil); err != nil {
			t.Fatalf("testenv: crear %s: %v", name, err)
		}
	}
	mem.AddUser(user)

	srv := imapserver.New(&imapserver.Options{
		NewSession: func(*imapserver.Conn) (imapserver.Session, *imapserver.GreetingData, error) {
			return mem.NewSession(), nil, nil
		},
		Caps: imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapIMAP4rev2: {}},
		// El servidor no sabe que el listener ya es TLS, así que sin esto
		// rechazaría LOGIN. El cliente igual verifica el certificado.
		InsecureAuth: true,
	})
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsc.Server)
	if err != nil {
		t.Fatalf("testenv: escuchar IMAP: %v", err)
	}
	go func() { _ = srv.Serve(ln); _ = ln.Close() }()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	s := &IMAP{
		Addr:     ln.Addr().String(),
		Settings: model.ServerSettings{Host: host, Port: port, Security: model.SecurityTLS},
		User:     user,
		Close:    func() { _ = srv.Close(); _ = ln.Close() },
	}
	t.Cleanup(s.Close)
	return s
}

// Deliver agrega un mensaje crudo (RFC 5322, con CRLF) a un buzón, sin pasar
// por el protocolo.
func (s *IMAP) Deliver(t testing.TB, mailbox string, raw []byte, flags ...imap.Flag) {
	t.Helper()
	lit := &literal{Reader: bytes.NewReader(raw), size: int64(len(raw))}
	if _, err := s.User.Append(mailbox, lit, &imap.AppendOptions{Flags: flags}); err != nil {
		t.Fatalf("testenv: entregar a %s: %v", mailbox, err)
	}
}

type literal struct {
	*bytes.Reader
	size int64
}

func (l *literal) Size() int64 { return l.size }

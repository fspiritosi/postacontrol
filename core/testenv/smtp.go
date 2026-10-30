package testenv

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"

	"postacontrol/core/model"
)

// RejectedRecipient es el destinatario que el servidor rechaza en RCPT con un
// error 5xx.
const RejectedRecipient = "rechazo@ej.com"

// Envelope es un mensaje recibido por el servidor SMTP de prueba.
type Envelope struct {
	From string
	To   []string
	Data []byte
}

// SMTP es un servidor SMTP con TLS implícito que exige AUTH PLAIN y guarda lo
// que recibe.
type SMTP struct {
	Addr     string
	Settings model.ServerSettings
	Received func() []Envelope
	Close    func()
}

type smtpBackend struct {
	username, password string
	mu                 sync.Mutex
	received           []Envelope
}

func (b *smtpBackend) NewSession(*smtp.Conn) (smtp.Session, error) {
	return &smtpSession{b: b}, nil
}

type smtpSession struct {
	b    *smtpBackend
	auth bool
	from string
	to   []string
}

func (s *smtpSession) AuthMechanisms() []string { return []string{sasl.Plain} }

func (s *smtpSession) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(identity, username, password string) error {
		if username != s.b.username || password != s.b.password {
			return smtp.ErrAuthFailed
		}
		s.auth = true
		return nil
	}), nil
}

func (s *smtpSession) Mail(from string, _ *smtp.MailOptions) error {
	if !s.auth {
		return smtp.ErrAuthRequired
	}
	s.from = from
	return nil
}

func (s *smtpSession) Rcpt(to string, _ *smtp.RcptOptions) error {
	if to == RejectedRecipient {
		return &smtp.SMTPError{Code: 550, EnhancedCode: smtp.EnhancedCode{5, 1, 1}, Message: "destinatario rechazado"}
	}
	s.to = append(s.to, to)
	return nil
}

func (s *smtpSession) Data(r io.Reader) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.b.mu.Lock()
	s.b.received = append(s.b.received, Envelope{From: s.from, To: append([]string(nil), s.to...), Data: data})
	s.b.mu.Unlock()
	return nil
}

func (s *smtpSession) Reset() { s.from, s.to = "", nil }

func (s *smtpSession) Logout() error { return nil }

// StartSMTP levanta el servidor. Se cierra solo al terminar el test.
func StartSMTP(t testing.TB, tlsc TLS, username, password string) *SMTP {
	t.Helper()
	b := &smtpBackend{username: username, password: password}
	srv := smtp.NewServer(b)
	srv.Domain = "localhost"
	srv.AllowInsecureAuth = true // el listener ya es TLS; el servidor no lo sabe
	srv.TLSConfig = tlsc.Server

	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsc.Server)
	if err != nil {
		t.Fatalf("testenv: escuchar SMTP: %v", err)
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, smtp.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			t.Logf("testenv: SMTP: %v", err)
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	s := &SMTP{
		Addr:     ln.Addr().String(),
		Settings: model.ServerSettings{Host: host, Port: port, Security: model.SecurityTLS},
		Received: func() []Envelope {
			b.mu.Lock()
			defer b.mu.Unlock()
			return append([]Envelope{}, b.received...)
		},
		Close: func() { _ = srv.Close(); _ = ln.Close() },
	}
	t.Cleanup(s.Close)
	return s
}

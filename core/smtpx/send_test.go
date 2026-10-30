package smtpx_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/smtpx"
	"postacontrol/core/testenv"
)

func setup(t *testing.T) (*testenv.SMTP, testenv.TLS) {
	t.Helper()
	tlsc := testenv.NewTLS(t)
	return testenv.StartSMTP(t, tlsc, "ana", "secreto"), tlsc
}

func TestSend_Delivers(t *testing.T) {
	srv, tlsc := setup(t)
	raw := []byte("From: ana@ej.com\r\nTo: beto@ej.com\r\nSubject: Hola\r\n\r\ncuerpo\r\n")
	err := smtpx.Send(context.Background(), srv.Settings, "ana", "secreto", "ana@ej.com", []string{"beto@ej.com", "carla@ej.com"}, raw, tlsc.Client)
	require.NoError(t, err)
	got := srv.Received()
	require.Len(t, got, 1)
	assert.Equal(t, "ana@ej.com", got[0].From)
	assert.Equal(t, []string{"beto@ej.com", "carla@ej.com"}, got[0].To)
	assert.Equal(t, raw, got[0].Data)
}

func TestSend_WrongPassword(t *testing.T) {
	srv, tlsc := setup(t)
	err := smtpx.Send(context.Background(), srv.Settings, "ana", "mala", "ana@ej.com", []string{"b@ej.com"}, []byte("x\r\n"), tlsc.Client)
	assert.ErrorIs(t, err, smtpx.ErrAuth)
	assert.Empty(t, srv.Received())
}

func TestSend_UntrustedCert(t *testing.T) {
	srv, _ := setup(t)
	err := smtpx.Send(context.Background(), srv.Settings, "ana", "secreto", "ana@ej.com", []string{"b@ej.com"}, []byte("x\r\n"), nil)
	assert.ErrorIs(t, err, smtpx.ErrTLS)
}

func TestCheck_OK(t *testing.T) {
	srv, tlsc := setup(t)
	require.NoError(t, smtpx.Check(context.Background(), srv.Settings, "ana", "secreto", tlsc.Client))
	assert.Empty(t, srv.Received())
}

func TestCheck_WrongPassword(t *testing.T) {
	srv, tlsc := setup(t)
	err := smtpx.Check(context.Background(), srv.Settings, "ana", "mala", tlsc.Client)
	assert.ErrorIs(t, err, smtpx.ErrAuth)
}

func TestSend_RejectedRecipient(t *testing.T) {
	srv, tlsc := setup(t)
	err := smtpx.Send(context.Background(), srv.Settings, "ana", "secreto", "ana@ej.com", []string{testenv.RejectedRecipient}, []byte("x\r\n"), tlsc.Client)
	require.ErrorIs(t, err, smtpx.ErrRejected)
	assert.Contains(t, err.Error(), "destinatario rechazado")
	assert.Empty(t, srv.Received())
}

func TestSend_ConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	require.NoError(t, ln.Close())
	s := model.ServerSettings{Host: "127.0.0.1", Port: port, Security: model.SecuritySTARTTLS}
	err = smtpx.Check(context.Background(), s, "a", "b", nil)
	assert.ErrorIs(t, err, smtpx.ErrNetwork)
}

func TestCheck_SilentServerHonorsContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() { // acepta y no dice nada
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	for _, sec := range []model.Security{model.SecurityTLS, model.SecuritySTARTTLS} {
		s := model.ServerSettings{Host: "127.0.0.1", Port: port, Security: sec}
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		start := time.Now()
		err = smtpx.Check(ctx, s, "a", "b", nil)
		cancel()
		assert.ErrorIs(t, err, smtpx.ErrNetwork, string(sec))
		assert.Less(t, time.Since(start), 3*time.Second, string(sec))
	}
}

// startSTARTTLS levanta un servidor SMTP en claro que ofrece STARTTLS solo si
// withTLS es true.
func startSTARTTLS(t *testing.T, tlsc testenv.TLS, withTLS bool) (*stBackend, model.ServerSettings) {
	t.Helper()
	b := &stBackend{}
	srv := smtp.NewServer(b)
	srv.Domain = "localhost"
	if withTLS {
		srv.TLSConfig = tlsc.Server
	} else {
		srv.AllowInsecureAuth = true
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return b, model.ServerSettings{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Security: model.SecuritySTARTTLS}
}

type stBackend struct {
	mu     sync.Mutex
	data   [][]byte
	hellos []string // nombre de cada EHLO/HELO, en orden
}

func (b *stBackend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	b.mu.Lock()
	b.hellos = append(b.hellos, c.Hostname())
	b.mu.Unlock()
	return &stSession{b: b}, nil
}

func (b *stBackend) lastHello() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.hellos) == 0 {
		return ""
	}
	return b.hellos[len(b.hellos)-1]
}

type stSession struct{ b *stBackend }

func (s *stSession) AuthMechanisms() []string { return []string{sasl.Plain} }
func (s *stSession) Auth(string) (sasl.Server, error) {
	return sasl.NewPlainServer(func(_, u, p string) error {
		if u != "ana" || p != "secreto" {
			return smtp.ErrAuthFailed
		}
		return nil
	}), nil
}
func (s *stSession) Mail(string, *smtp.MailOptions) error { return nil }
func (s *stSession) Rcpt(string, *smtp.RcptOptions) error { return nil }
func (s *stSession) Data(r io.Reader) error {
	d, err := io.ReadAll(r)
	s.b.mu.Lock()
	s.b.data = append(s.b.data, d)
	s.b.mu.Unlock()
	return err
}
func (s *stSession) Reset()        {}
func (s *stSession) Logout() error { return nil }

func TestSend_STARTTLS_Delivers(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	b, set := startSTARTTLS(t, tlsc, true)
	raw := []byte("Subject: hola\r\n\r\ncuerpo\r\n")
	require.NoError(t, smtpx.Send(context.Background(), set, "ana", "secreto", "ana@ej.com", []string{"b@ej.com"}, raw, tlsc.Client))
	b.mu.Lock()
	defer b.mu.Unlock()
	require.Len(t, b.data, 1)
	assert.Equal(t, raw, b.data[0])
}

func TestSend_STARTTLS_UntrustedCert(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	_, set := startSTARTTLS(t, tlsc, true)
	err := smtpx.Check(context.Background(), set, "ana", "secreto", nil)
	assert.ErrorIs(t, err, smtpx.ErrTLS)
}

func TestSend_STARTTLS_NotAdvertised(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	_, set := startSTARTTLS(t, tlsc, false)
	err := smtpx.Check(context.Background(), set, "ana", "secreto", tlsc.Client)
	assert.ErrorIs(t, err, smtpx.ErrTLS)
	assert.NotErrorIs(t, err, smtpx.ErrNetwork)
}

// startImplicitTLS levanta un servidor SMTP con TLS implícito (como el 465).
func startImplicitTLS(t *testing.T, tlsc testenv.TLS) (*stBackend, model.ServerSettings) {
	t.Helper()
	b := &stBackend{}
	srv := smtp.NewServer(b)
	srv.Domain = "localhost"
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsc.Server)
	require.NoError(t, err)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	return b, model.ServerSettings{Host: "127.0.0.1", Port: ln.Addr().(*net.TCPAddr).Port, Security: model.SecurityTLS}
}

// Un servidor que acepta TCP y nunca completa el handshake TLS no deja el
// envío colgado aunque el contexto no tenga plazo.
func TestCheck_StalledTLSHandshakeTimesOut(t *testing.T) {
	defer smtpx.SetHandshakeTimeout(200 * time.Millisecond)()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() { // acepta y no contesta el ClientHello
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	for _, sec := range []model.Security{model.SecurityTLS, model.SecuritySTARTTLS} {
		s := model.ServerSettings{Host: "127.0.0.1", Port: port, Security: sec}
		done := make(chan error, 1)
		start := time.Now()
		go func() { done <- smtpx.Check(context.Background(), s, "a", "b", nil) }()
		select {
		case err := <-done:
			assert.ErrorIs(t, err, smtpx.ErrNetwork, string(sec))
			assert.Less(t, time.Since(start), 2*time.Second, string(sec))
		case <-time.After(5 * time.Second):
			t.Fatalf("%s: sin plazo, el handshake quedó colgado", sec)
		}
	}
}

func TestSend_EHLOUsesHostname(t *testing.T) {
	want, err := os.Hostname()
	if err != nil || want == "" {
		want = "localhost"
	}
	assert.Equal(t, want, smtpx.LocalName())

	tlsc := testenv.NewTLS(t)
	for name, start := range map[string]func() (*stBackend, model.ServerSettings){
		"tls":      func() (*stBackend, model.ServerSettings) { return startImplicitTLS(t, tlsc) },
		"starttls": func() (*stBackend, model.ServerSettings) { return startSTARTTLS(t, tlsc, true) },
	} {
		b, s := start()
		raw := []byte("From: ana@ej.com\r\nTo: beto@ej.com\r\nSubject: Hola\r\n\r\ncuerpo\r\n")
		require.NoError(t, smtpx.Send(context.Background(), s, "ana", "secreto", "ana@ej.com", []string{"beto@ej.com"}, raw, tlsc.Client), name)
		assert.Equal(t, want, b.lastHello(), name)
	}
}

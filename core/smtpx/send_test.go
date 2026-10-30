package smtpx_test

import (
	"context"
	"net"
	"testing"
	"time"

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

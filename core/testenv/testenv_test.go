package testenv_test

import (
	"crypto/tls"
	"errors"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/testenv"
)

func dialSMTP(t *testing.T, s *testenv.SMTP, tlsc testenv.TLS) *smtp.Client {
	t.Helper()
	conn, err := tls.Dial("tcp", s.Addr, tlsc.Client)
	require.NoError(t, err)
	c := smtp.NewClient(conn)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestSMTP_AuthAndReceive(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	s := testenv.StartSMTP(t, tlsc, "ana", "pw")
	c := dialSMTP(t, s, tlsc)
	require.NoError(t, c.Auth(sasl.NewPlainClient("", "ana", "pw")))
	require.NoError(t, c.SendMail("ana@ej.com", []string{"bea@ej.com"}, strings.NewReader("Subject: x\r\n\r\nhola\r\n")))

	got := s.Received()
	require.Len(t, got, 1)
	assert.Equal(t, "ana@ej.com", got[0].From)
	assert.Equal(t, []string{"bea@ej.com"}, got[0].To)
	assert.Contains(t, string(got[0].Data), "hola")
}

func TestSMTP_WrongPasswordAndNoAuth(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	s := testenv.StartSMTP(t, tlsc, "ana", "pw")
	assert.Error(t, dialSMTP(t, s, tlsc).Auth(sasl.NewPlainClient("", "ana", "mal")))
	assert.Error(t, dialSMTP(t, s, tlsc).Mail("ana@ej.com", nil))
	assert.Empty(t, s.Received())
}

func TestSMTP_RejectedRecipient(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	s := testenv.StartSMTP(t, tlsc, "ana", "pw")
	c := dialSMTP(t, s, tlsc)
	require.NoError(t, c.Auth(sasl.NewPlainClient("", "ana", "pw")))
	require.NoError(t, c.Mail("ana@ej.com", nil))
	err := c.Rcpt(testenv.RejectedRecipient, nil)
	var se *smtp.SMTPError
	require.True(t, errors.As(err, &se))
	assert.Equal(t, 550, se.Code)
}

func imapCount(t *testing.T, s *testenv.IMAP, mailbox string) uint32 {
	t.Helper()
	st, err := s.User.Status(mailbox, &imap.StatusOptions{NumMessages: true})
	require.NoError(t, err)
	return *st.NumMessages
}

func TestIMAP_Expunge(t *testing.T) {
	s := testenv.StartIMAP(t, testenv.NewTLS(t), "ana", "pw")
	for range 3 {
		s.Deliver(t, "INBOX", []byte("Subject: x\r\n\r\nhola\r\n"))
	}
	s.Expunge(t, "INBOX", 2)
	assert.Equal(t, uint32(2), imapCount(t, s, "INBOX"))
}

func TestIMAP_RecreateMailboxChangesUIDValidity(t *testing.T) {
	s := testenv.StartIMAP(t, testenv.NewTLS(t), "ana", "pw")
	s.Deliver(t, "INBOX", []byte("Subject: x\r\n\r\nhola\r\n"))
	before, err := s.User.Status("INBOX", &imap.StatusOptions{UIDValidity: true})
	require.NoError(t, err)
	after := s.RecreateMailbox(t, "INBOX")
	assert.NotEqual(t, before.UIDValidity, after)
	assert.Equal(t, uint32(0), imapCount(t, s, "INBOX"))
}

func TestIMAP_CloseAndRestartSameAddress(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	s := testenv.StartIMAP(t, tlsc, "ana", "pw")
	s.Deliver(t, "INBOX", []byte("Subject: x\r\n\r\nhola\r\n"))
	addr := s.Addr
	s.Close()
	_, err := tls.Dial("tcp", addr, tlsc.Client)
	require.Error(t, err)

	s.Restart(t)
	assert.Equal(t, addr, s.Addr)
	conn, err := tls.Dial("tcp", addr, tlsc.Client)
	require.NoError(t, err)
	_ = conn.Close()
	assert.Equal(t, uint32(1), imapCount(t, s, "INBOX"), "los datos sobreviven al reinicio")
}

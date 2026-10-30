package imapx_test

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/testenv"
)

const (
	user = "ana@ej.com"
	pass = "secreto"
)

func setup(t *testing.T) (*testenv.IMAP, testenv.TLS) {
	t.Helper()
	tlsc := testenv.NewTLS(t)
	return testenv.StartIMAP(t, tlsc, user, pass), tlsc
}

func dial(t *testing.T, s *testenv.IMAP, tlsc testenv.TLS) *imapx.Conn {
	t.Helper()
	c, err := imapx.Dial(context.Background(), s.Settings, user, pass, tlsc.Client)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func plainMsg(subject, extra string) []byte {
	return []byte("From: Bea <bea@ej.com>\r\nTo: ana@ej.com\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <" + subject + "@ej.com>\r\n" + extra +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nhola\r\n")
}

func TestDial_OK(t *testing.T) {
	s, tlsc := setup(t)
	c, err := imapx.Dial(context.Background(), s.Settings, user, pass, tlsc.Client)
	require.NoError(t, err)
	require.NoError(t, c.Close())
}

func TestDial_WrongPassword(t *testing.T) {
	s, tlsc := setup(t)
	_, err := imapx.Dial(context.Background(), s.Settings, user, "mala", tlsc.Client)
	require.Error(t, err)
	assert.True(t, errors.Is(err, imapx.ErrAuth), "got %v", err)
}

func TestDial_UntrustedCert(t *testing.T) {
	s, _ := setup(t)
	_, err := imapx.Dial(context.Background(), s.Settings, user, pass, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, imapx.ErrTLS), "got %v", err)
}

func TestDial_NetworkError(t *testing.T) {
	s, tlsc := setup(t)
	bad := s.Settings
	s.Close()
	_, err := imapx.Dial(context.Background(), bad, user, pass, tlsc.Client)
	require.Error(t, err)
	assert.True(t, errors.Is(err, imapx.ErrNetwork), "got %v", err)
	assert.False(t, errors.Is(err, imapx.ErrTLS))
}

func TestDial_CancelledContext(t *testing.T) {
	s, tlsc := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := imapx.Dial(ctx, s.Settings, user, pass, tlsc.Client)
	assert.True(t, errors.Is(err, imapx.ErrNetwork), "got %v", err)
}

func TestListFolders_RolesAndCounts(t *testing.T) {
	s, tlsc := setup(t)
	require.NoError(t, s.User.Create("INBOX.Borradores", nil))
	s.Deliver(t, "INBOX", plainMsg("uno", ""), imap.FlagSeen)
	s.Deliver(t, "INBOX", plainMsg("dos", ""))
	c := dial(t, s, tlsc)

	folders, err := c.ListFolders()
	require.NoError(t, err)
	by := map[string]imapx.RemoteFolder{}
	for _, f := range folders {
		by[f.Name] = f
	}
	assert.Equal(t, model.FolderRole("inbox"), by["INBOX"].Role)
	assert.Equal(t, 2, by["INBOX"].Total)
	assert.Equal(t, 1, by["INBOX"].Unread)
	assert.True(t, by["INBOX"].Selectable)
	assert.Equal(t, model.FolderRole("sent"), by["Sent"].Role)
	assert.Equal(t, model.FolderRole("drafts"), by["INBOX.Borradores"].Role)
}

func TestDetectRole_Table(t *testing.T) {
	cases := []struct {
		name, delim string
		attrs       []imap.MailboxAttr
		want        model.FolderRole
	}{
		{"INBOX", "/", nil, "inbox"},
		{"inbox", ".", nil, "inbox"},
		{"Sent", "/", nil, "sent"},
		{"Sent Items", "/", nil, "sent"},
		{"Sent Messages", "/", nil, "sent"},
		{"Enviados", "/", nil, "sent"},
		{"INBOX.Sent", ".", nil, "sent"},
		{"INBOX/Sent", "/", nil, "sent"},
		{"Drafts", "/", nil, "drafts"},
		{"Borradores", "/", nil, "drafts"},
		{"Trash", "/", nil, "trash"},
		{"Deleted Items", "/", nil, "trash"},
		{"Papelera", "/", nil, "trash"},
		{"Junk", "/", nil, "junk"},
		{"SPAM", "/", nil, "junk"},
		{"Archive", "/", nil, "archive"},
		{"Archivo", "/", nil, "archive"},
		{"Facturas", "/", nil, ""},
		{"[Gmail]/Enviados", "/", nil, "sent"},
		{"[Gmail]/Mis cosas", "/", []imap.MailboxAttr{imap.MailboxAttrSent}, "sent"},
		{"Lo que sea", "/", []imap.MailboxAttr{imap.MailboxAttrDrafts}, "drafts"},
		{"Lo que sea", "/", []imap.MailboxAttr{imap.MailboxAttrTrash}, "trash"},
		{"Lo que sea", "/", []imap.MailboxAttr{imap.MailboxAttrJunk}, "junk"},
		{"Lo que sea", "/", []imap.MailboxAttr{imap.MailboxAttrArchive}, "archive"},
		// el atributo gana sobre el nombre
		{"Sent", "/", []imap.MailboxAttr{imap.MailboxAttrArchive}, "archive"},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s|%s", tc.name, tc.want), func(t *testing.T) {
			assert.Equal(t, tc.want, imapx.DetectRole(tc.name, tc.delim, tc.attrs))
		})
	}
}

func multipartWithPDF(pdf []byte) []byte {
	b64 := base64.StdEncoding.EncodeToString(pdf)
	return []byte("From: Bea <bea@ej.com>\r\nTo: ana@ej.com\r\nSubject: conadjunto\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <adj@ej.com>\r\n" +
		"References: <a@x> <b@x>\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=XX\r\n\r\n" +
		"--XX\r\nContent-Type: text/plain; charset=utf-8\r\n\r\ntexto del cuerpo\r\n" +
		"--XX\r\nContent-Type: application/pdf; name=\"f.pdf\"\r\nContent-Disposition: attachment; filename=\"f.pdf\"\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" + b64 + "\r\n--XX--\r\n")
}

func TestFetchHeaders_EnvelopeReferencesAttachments(t *testing.T) {
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", multipartWithPDF([]byte("%PDF-1.4 fake")))
	s.Deliver(t, "INBOX", plainMsg("simple", ""))
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)

	uids, err := c.AllUIDs()
	require.NoError(t, err)
	require.Len(t, uids, 2)
	assert.Less(t, uids[0], uids[1])

	hs, err := c.FetchHeaders(uids)
	require.NoError(t, err)
	require.Len(t, hs, 2)
	assert.Equal(t, uids[0], hs[0].UID)
	assert.Equal(t, []string{"<a@x>", "<b@x>"}, hs[0].References)
	assert.True(t, hs[0].HasAttachments)
	require.NotNil(t, hs[0].Envelope)
	assert.Equal(t, "conadjunto", hs[0].Envelope.Subject)
	assert.Equal(t, "bea", hs[0].Envelope.From[0].Mailbox)
	assert.Greater(t, hs[0].Size, int64(0))
	assert.False(t, hs[0].Date.IsZero())

	assert.Empty(t, hs[1].References)
	assert.NotNil(t, hs[1].References)
	assert.False(t, hs[1].HasAttachments)
	assert.Equal(t, "simple", hs[1].Envelope.Subject)
}

func TestFetchHeaders_Empty(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	hs, err := c.FetchHeaders(nil)
	require.NoError(t, err)
	assert.Empty(t, hs)
}

func TestFetchPart_TextAndAttachment(t *testing.T) {
	pdf := []byte("%PDF-1.4\x00\x01\x02 binario \xff\xfe")
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", multipartWithPDF(pdf))
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)
	uids, err := c.AllUIDs()
	require.NoError(t, err)

	bs, err := c.FetchStructure(uids[0])
	require.NoError(t, err)
	assert.Equal(t, "multipart/mixed", bs.MediaType())

	hdr, body, err := c.FetchPart(uids[0], []int{1})
	require.NoError(t, err)
	assert.Contains(t, string(hdr), "text/plain")
	assert.Contains(t, string(body), "texto del cuerpo")

	hdr, body, err = c.FetchPart(uids[0], []int{2})
	require.NoError(t, err)
	assert.Contains(t, string(hdr), "application/pdf")
	got, err := base64.StdEncoding.DecodeString(string(body))
	require.NoError(t, err)
	assert.Equal(t, pdf, got)

	// Peek: pedir partes no marca el mensaje como leído.
	fl, err := c.FetchFlags(uids)
	require.NoError(t, err)
	assert.NotContains(t, fl[uids[0]], imap.FlagSeen)

	// Parte vacía: HEADER + TEXT del mensaje completo.
	hdr, body, err = c.FetchPart(uids[0], nil)
	require.NoError(t, err)
	assert.Contains(t, string(hdr), "Subject: conadjunto")
	assert.Contains(t, string(body), "texto del cuerpo")
}

func TestAddFlagsAndAppend(t *testing.T) {
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	c := dial(t, s, tlsc)

	_, err := c.SelectRW("INBOX")
	require.NoError(t, err)
	uids, err := c.AllUIDs()
	require.NoError(t, err)
	require.Len(t, uids, 1)

	fl, err := c.FetchFlags(uids)
	require.NoError(t, err)
	assert.NotContains(t, fl[uids[0]], imap.FlagSeen)

	require.NoError(t, c.AddFlags(uids[0], imap.FlagSeen))
	fl, err = c.FetchFlags(uids)
	require.NoError(t, err)
	assert.Contains(t, fl[uids[0]], imap.FlagSeen)

	require.NoError(t, c.Append("Sent", plainMsg("enviado", ""), imap.FlagSeen))
	info, err := c.Select("Sent")
	require.NoError(t, err)
	assert.Equal(t, uint32(1), info.NumMessages)
	assert.NotZero(t, info.UIDValidity)
	assert.NotZero(t, info.UIDNext)

	sentUIDs, err := c.AllUIDs()
	require.NoError(t, err)
	fl, err = c.FetchFlags(sentUIDs)
	require.NoError(t, err)
	assert.Contains(t, fl[sentUIDs[0]], imap.FlagSeen)
}

func TestSelect_Missing(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	_, err := c.Select("NoExiste")
	assert.Error(t, err)
}

func rawMsg(subject, headers, body string) []byte {
	return []byte("From: Bea <bea@ej.com>\r\nTo: ana@ej.com\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <" + subject + "@ej.com>\r\nMIME-Version: 1.0\r\n" +
		headers + "\r\n" + body)
}

func TestFetchHeaders_HasAttachmentsRules(t *testing.T) {
	s, tlsc := setup(t)
	// (a) text/plain con disposición attachment
	s.Deliver(t, "INBOX", rawMsg("a", "Content-Type: multipart/mixed; boundary=B\r\n",
		"--B\r\nContent-Type: text/plain\r\n\r\ncuerpo\r\n"+
			"--B\r\nContent-Type: text/plain\r\nContent-Disposition: attachment; filename=nota.txt\r\n\r\nnota\r\n--B--\r\n"))
	// (b) pdf con filename solo en la disposición, sin name=
	s.Deliver(t, "INBOX", rawMsg("b", "Content-Type: multipart/mixed; boundary=B\r\n",
		"--B\r\nContent-Type: text/plain\r\n\r\ncuerpo\r\n"+
			"--B\r\nContent-Type: application/pdf\r\nContent-Disposition: inline; filename=f.pdf\r\nContent-Transfer-Encoding: base64\r\n\r\nJVBERg==\r\n--B--\r\n"))
	// (c) imagen inline con Content-ID y sin filename
	s.Deliver(t, "INBOX", rawMsg("c", "Content-Type: multipart/related; boundary=R\r\n",
		"--R\r\nContent-Type: text/html\r\n\r\n<img src=\"cid:x\">\r\n"+
			"--R\r\nContent-Type: image/png\r\nContent-ID: <x>\r\nContent-Disposition: inline\r\nContent-Transfer-Encoding: base64\r\n\r\niVBORw==\r\n--R--\r\n"))
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)
	uids, err := c.AllUIDs()
	require.NoError(t, err)
	hs, err := c.FetchHeaders(uids)
	require.NoError(t, err)
	require.Len(t, hs, 3)
	assert.True(t, hs[0].HasAttachments, "a: text/plain attachment")
	assert.True(t, hs[1].HasAttachments, "b: pdf con filename de disposición")
	assert.False(t, hs[2].HasAttachments, "c: imagen inline sin filename")
}

func TestSelect_IsReadOnly(t *testing.T) {
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)
	uids, err := c.AllUIDs()
	require.NoError(t, err)

	err = c.AddFlags(uids[0], imap.FlagSeen)
	assert.ErrorIs(t, err, imapx.ErrReadOnly)
	fl, err := c.FetchFlags(uids)
	require.NoError(t, err)
	assert.NotContains(t, fl[uids[0]], imap.FlagSeen)

	// Volver a Select tras SelectRW también deja el modo de solo lectura.
	_, err = c.SelectRW("INBOX")
	require.NoError(t, err)
	_, err = c.Select("INBOX")
	require.NoError(t, err)
	assert.ErrorIs(t, c.AddFlags(uids[0], imap.FlagSeen), imapx.ErrReadOnly)
}

func TestDial_StalledServerHonorsContext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	p, _ := strconv.Atoi(port)
	for _, sec := range []model.Security{model.SecurityTLS, model.SecuritySTARTTLS} {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		start := time.Now()
		_, err = imapx.Dial(ctx, model.ServerSettings{Host: "127.0.0.1", Port: p, Security: sec}, user, pass, nil)
		cancel()
		assert.True(t, errors.Is(err, imapx.ErrNetwork), "%s: got %v", sec, err)
		assert.Less(t, time.Since(start), time.Second, sec)
	}
}

package imapx_test

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
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

func TestOps_ConnectionDropIsNetworkError(t *testing.T) {
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)
	s.Close()

	_, err = c.AllUIDs()
	assert.ErrorIs(t, err, imapx.ErrNetwork)
	_, err = c.FetchHeaders([]uint32{1})
	assert.ErrorIs(t, err, imapx.ErrNetwork)
	_, err = c.FetchFlags([]uint32{1})
	assert.ErrorIs(t, err, imapx.ErrNetwork)
	_, err = c.Select("INBOX")
	assert.ErrorIs(t, err, imapx.ErrNetwork)
	_, err = c.ListFolders()
	assert.ErrorIs(t, err, imapx.ErrNetwork)
	assert.ErrorIs(t, c.Noop(), imapx.ErrNetwork)
}

func TestNoop_LiveSession(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	require.NoError(t, c.Noop())
}

func TestOps_ServerRefusalIsNotNetworkError(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	_, err := c.Select("NoExiste")
	require.Error(t, err)
	assert.NotErrorIs(t, err, imapx.ErrNetwork)
}

func TestAbort_ClosesWithoutLogout(t *testing.T) {
	s, tlsc := setup(t)
	c, err := imapx.Dial(context.Background(), s.Settings, user, pass, tlsc.Client)
	require.NoError(t, err)
	require.NoError(t, c.Abort())
	_, err = c.Select("INBOX")
	assert.ErrorIs(t, err, imapx.ErrNetwork)
}

// scriptedIMAP es un servidor mínimo que contesta a cualquier UID FETCH con
// fetchResp. Sirve para probar respuestas que el servidor en memoria no
// genera (por ejemplo, un ENVELOPE con palabras codificadas sin decodificar,
// que es lo que mandan Dovecot y compañía).
func scriptedIMAP(t *testing.T, tlsc testenv.TLS, fetchResp string) model.ServerSettings {
	t.Helper()
	return scriptedIMAPWith(t, tlsc, script{fetch: fetchResp})
}

// script dice cómo contesta scriptedIMAPWith.
type script struct {
	fetch string // respuesta a UID FETCH (sin la línea final etiquetada)
	// login, si no es vacío, reemplaza el "OK" de LOGIN (por ejemplo
	// "NO [UNAVAILABLE] probá más tarde").
	login string
	// dropOnCapability corta la conexión al recibir CAPABILITY (el saludo ya
	// la anuncia, así que el cliente solo la pide después del LOGIN).
	dropOnCapability bool
}

func scriptedIMAPWith(t *testing.T, tlsc testenv.TLS, sc script) model.ServerSettings {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsc.Server)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.WriteString(conn, "* OK [CAPABILITY IMAP4rev1] listo\r\n")
				lines := bufio.NewScanner(conn)
				for lines.Scan() {
					tag, cmd, _ := strings.Cut(lines.Text(), " ")
					cmd = strings.ToUpper(cmd)
					switch {
					case strings.HasPrefix(cmd, "CAPABILITY"):
						if sc.dropOnCapability {
							return
						}
						_, _ = io.WriteString(conn, "* CAPABILITY IMAP4rev1\r\n")
					case strings.HasPrefix(cmd, "LOGIN") && sc.login != "":
						_, _ = io.WriteString(conn, tag+" "+sc.login+"\r\n")
						continue
					case strings.HasPrefix(cmd, "UID FETCH"):
						_, _ = io.WriteString(conn, sc.fetch)
					case strings.HasPrefix(cmd, "LOGOUT"):
						_, _ = io.WriteString(conn, "* BYE chau\r\n"+tag+" OK listo\r\n")
						return
					}
					_, _ = io.WriteString(conn, tag+" OK listo\r\n")
				}
			}()
		}
	}()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return model.ServerSettings{Host: host, Port: port, Security: model.SecurityTLS}
}

// Falla segura: solo los códigos de falla temporal del servidor ([UNAVAILABLE],
// [SERVERBUG], [INUSE], [LIMIT]) se reintentan con espera, como la red; todo
// otro NO (sin código, credenciales, [EXPIRED], [CONTACTADMIN], [ALERT],
// códigos desconocidos) pausa la cuenta para no acumular LOGINs fallidos.
func TestDial_LoginNOClassification(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	cases := []struct {
		name  string
		login string
		want  error
	}{
		{"sin código", "NO usuario o contraseña inválidos", imapx.ErrAuth},
		{"AUTHENTICATIONFAILED", "NO [AUTHENTICATIONFAILED] Authentication failed.", imapx.ErrAuth},
		{"AUTHORIZATIONFAILED", "NO [AUTHORIZATIONFAILED] no autorizado", imapx.ErrAuth},
		{"EXPIRED", "NO [EXPIRED] la contraseña venció", imapx.ErrAuth},
		{"CONTACTADMIN", "NO [CONTACTADMIN] contactá al administrador", imapx.ErrAuth},
		{"PRIVACYREQUIRED", "NO [PRIVACYREQUIRED] se requiere TLS", imapx.ErrAuth},
		{"ALERT", "NO [ALERT] cuenta bloqueada", imapx.ErrAuth},
		{"código desconocido", "NO [XFOOBAR] motivo desconocido", imapx.ErrAuth},
		{"UNAVAILABLE", "NO [UNAVAILABLE] Temporary authentication failure.", imapx.ErrNetwork},
		{"SERVERBUG", "NO [SERVERBUG] algo se rompió", imapx.ErrNetwork},
		{"INUSE", "NO [INUSE] buzón en uso", imapx.ErrNetwork},
		{"LIMIT", "NO [LIMIT] demasiadas conexiones", imapx.ErrNetwork},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := scriptedIMAPWith(t, tlsc, script{login: tc.login})
			_, err := imapx.Dial(context.Background(), srv, user, pass, tlsc.Client)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.want)
			other := imapx.ErrNetwork
			if tc.want == imapx.ErrNetwork {
				other = imapx.ErrAuth
			}
			assert.NotErrorIs(t, err, other)
		})
	}
}

func TestFetchHeaders_DecodesLegacyCharsetsInEnvelope(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	resp := `* 1 FETCH (UID 7 FLAGS (\Seen) INTERNALDATE "02-Jan-2006 15:04:05 +0000" RFC822.SIZE 120 ` +
		`ENVELOPE ("Mon, 02 Jan 2006 15:04:05 +0000" "=?windows-1252?Q?Informaci=F3n?=" ` +
		`(("=?windows-1252?Q?Pe=F1a?=" NIL "p" "x.com")) NIL NIL NIL NIL NIL NIL "<m@x.com>") ` +
		`BODYSTRUCTURE ("text" "plain" ("charset" "utf-8") NIL NIL "7bit" 5 1 NIL NIL NIL NIL) ` +
		"BODY[HEADER.FIELDS (REFERENCES)] {2}\r\n\r\n)\r\n"
	srv := scriptedIMAP(t, tlsc, resp)
	c, err := imapx.Dial(context.Background(), srv, user, pass, tlsc.Client)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })

	hs, err := c.FetchHeaders([]uint32{7})
	require.NoError(t, err)
	require.Len(t, hs, 1)
	require.NotNil(t, hs[0].Envelope)
	assert.Equal(t, "Información", hs[0].Envelope.Subject)
	require.Len(t, hs[0].Envelope.From, 1)
	assert.Equal(t, "Peña", hs[0].Envelope.From[0].Name)
}

func seedTwo(t *testing.T, s *testenv.IMAP, c *imapx.Conn) []uint32 {
	t.Helper()
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	s.Deliver(t, "INBOX", plainMsg("dos", ""))
	_, err := c.SelectRW("INBOX")
	require.NoError(t, err)
	uids, err := c.AllUIDs()
	require.NoError(t, err)
	require.Len(t, uids, 2)
	return uids
}

func TestStoreFlags_AddAndRemove(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	uids := seedTwo(t, s, c)

	require.NoError(t, c.StoreFlags(uids, true, imap.FlagFlagged))
	fl, err := c.FetchFlags(uids)
	require.NoError(t, err)
	assert.Contains(t, fl[uids[0]], imap.FlagFlagged)
	assert.Contains(t, fl[uids[1]], imap.FlagFlagged)

	require.NoError(t, c.StoreFlags(uids[:1], false, imap.FlagFlagged))
	fl, err = c.FetchFlags(uids)
	require.NoError(t, err)
	assert.NotContains(t, fl[uids[0]], imap.FlagFlagged)
	assert.Contains(t, fl[uids[1]], imap.FlagFlagged)
}

func TestStoreFlags_ReadOnlyFails(t *testing.T) {
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)
	err = c.StoreFlags([]uint32{1}, true, imap.FlagSeen)
	assert.ErrorIs(t, err, imapx.ErrReadOnly)
}

func TestMove_MovesToDestination(t *testing.T) {
	s, tlsc := setup(t)
	require.NoError(t, s.User.Create("Trash", nil))
	c := dial(t, s, tlsc)
	uids := seedTwo(t, s, c)

	require.NoError(t, c.Move(uids[:1], "Trash"))
	left, err := c.AllUIDs()
	require.NoError(t, err)
	assert.Equal(t, uids[1:], left)

	info, err := c.Select("Trash")
	require.NoError(t, err)
	assert.Equal(t, uint32(1), info.NumMessages)
}

func TestExpungeUIDs_OnlyGivenUIDs(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	uids := seedTwo(t, s, c)

	// Otro cliente marca u2 como \Deleted sin expungearlo.
	sess := imapmemserver.NewUserSession(s.User)
	defer func() { _ = sess.Close() }()
	_, err := sess.Select("INBOX", nil)
	require.NoError(t, err)
	del := &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}
	require.NoError(t, sess.Store(nil, imap.UIDSetNum(imap.UID(uids[1])), del, nil))

	fl, err := c.FetchFlags(uids[1:])
	require.NoError(t, err)
	require.Contains(t, fl[uids[1]], imap.FlagDeleted)

	require.NoError(t, c.ExpungeUIDs(uids[:1]))
	left, err := c.AllUIDs()
	require.NoError(t, err)
	assert.Equal(t, uids[1:], left)
}

func TestMove_Rev1ServerIsUnsupported(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	s := testenv.StartIMAPRev1(t, tlsc, user, pass)
	require.NoError(t, s.User.Create("Trash", nil))
	c := dial(t, s, tlsc)
	uids := seedTwo(t, s, c)

	assert.ErrorIs(t, c.Move(uids[:1], "Trash"), imapx.ErrUnsupported)
	assert.ErrorIs(t, c.ExpungeUIDs(uids[:1]), imapx.ErrUnsupported)
	left, err := c.AllUIDs()
	require.NoError(t, err)
	assert.Equal(t, uids, left)
}

func uidplusOnly(t *testing.T) (*testenv.IMAP, testenv.TLS) {
	t.Helper()
	tlsc := testenv.NewTLS(t)
	caps := imap.CapSet{imap.CapIMAP4rev1: {}, imap.CapUIDPlus: {}}
	return testenv.StartIMAPWithCaps(t, tlsc, user, pass, caps), tlsc
}

func TestMove_UIDPlusOnlyMoves(t *testing.T) {
	s, tlsc := uidplusOnly(t)
	require.NoError(t, s.User.Create("Trash", nil))
	c := dial(t, s, tlsc)
	uids := seedTwo(t, s, c)

	require.NoError(t, c.Move(uids[:1], "Trash"))
	left, err := c.AllUIDs()
	require.NoError(t, err)
	assert.Equal(t, uids[1:], left)
	info, err := c.Select("Trash")
	require.NoError(t, err)
	assert.Equal(t, uint32(1), info.NumMessages)
}

func TestMove_UIDPlusOnlyCopyFailureKeepsOriginals(t *testing.T) {
	s, tlsc := uidplusOnly(t)
	c := dial(t, s, tlsc)
	uids := seedTwo(t, s, c)

	require.Error(t, c.Move(uids[:1], "NoExiste"))
	left, err := c.AllUIDs()
	require.NoError(t, err)
	assert.Equal(t, uids, left)
	fl, err := c.FetchFlags(uids)
	require.NoError(t, err)
	assert.NotContains(t, fl[uids[0]], imap.FlagDeleted)
	require.NoError(t, c.Noop())
}

func TestWrites_EmptyUIDsAreNoOps(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	seedTwo(t, s, c)
	require.NoError(t, c.StoreFlags(nil, true, imap.FlagSeen))
	require.NoError(t, c.Move(nil, "Sent"))
	require.NoError(t, c.ExpungeUIDs(nil))
}

func TestListFolders_OneFolderPerRole(t *testing.T) {
	s, tlsc := setup(t)
	for _, name := range []string{"Trash", "Clientes", "Clientes/Papelera", "Proyectos", "Proyectos/Trash", "Proyectos/Sent"} {
		require.NoError(t, s.User.Create(name, nil))
	}
	c := dial(t, s, tlsc)

	folders, err := c.ListFolders()
	require.NoError(t, err)
	by := map[string]imapx.RemoteFolder{}
	for _, f := range folders {
		by[f.Name] = f
	}
	assert.Equal(t, model.FolderRole("trash"), by["Trash"].Role)
	assert.Equal(t, model.FolderRole(""), by["Clientes/Papelera"].Role)
	assert.Equal(t, model.FolderRole(""), by["Proyectos/Trash"].Role)
	assert.Equal(t, model.FolderRole("sent"), by["Sent"].Role)
	assert.Equal(t, model.FolderRole(""), by["Proyectos/Sent"].Role)
}

func TestUniqueRoles(t *testing.T) {
	f := func(name string, role model.FolderRole, special bool) imapx.RemoteFolder {
		return imapx.RemoteFolder{Name: name, Delimiter: ".", Role: role, SpecialUse: special, Selectable: true}
	}
	roles := func(fs []imapx.RemoteFolder) map[string]model.FolderRole {
		out := map[string]model.FolderRole{}
		for _, x := range fs {
			out[x.Name] = x.Role
		}
		return out
	}
	cases := []struct {
		name string
		in   []imapx.RemoteFolder
		want map[string]model.FolderRole
	}{
		{"la menos profunda gana", []imapx.RemoteFolder{
			f("INBOX.Clientes.Papelera", "trash", false), f("INBOX.Trash", "trash", false), f("INBOX", "inbox", false),
		}, map[string]model.FolderRole{"INBOX.Clientes.Papelera": "", "INBOX.Trash": "trash", "INBOX": "inbox"}},
		{"empate: la primera por nombre", []imapx.RemoteFolder{
			f("Trash", "trash", false), f("Deleted Items", "trash", false),
		}, map[string]model.FolderRole{"Trash": "", "Deleted Items": "trash"}},
		{"special-use gana aunque esté más adentro", []imapx.RemoteFolder{
			f("Trash", "trash", false), f("INBOX.Cosas.Borrados", "trash", true),
		}, map[string]model.FolderRole{"Trash": "", "INBOX.Cosas.Borrados": "trash"}},
		{"una no seleccionable no gana", []imapx.RemoteFolder{
			{Name: "Trash", Delimiter: ".", Role: "trash"}, f("INBOX.Trash", "trash", false),
		}, map[string]model.FolderRole{"Trash": "", "INBOX.Trash": "trash"}},
		{"roles distintos no se pisan", []imapx.RemoteFolder{
			f("Sent", "sent", false), f("Trash", "trash", false), f("Facturas", "", false),
		}, map[string]model.FolderRole{"Sent": "sent", "Trash": "trash", "Facturas": ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, roles(imapx.UniqueRoles(tc.in)))
		})
	}
}

func dialWatch(t *testing.T, s *testenv.IMAP, tlsc testenv.TLS, onChange func()) *imapx.Conn {
	t.Helper()
	c, err := imapx.DialWatch(context.Background(), s.Settings, user, pass, tlsc.Client, onChange)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Abort() })
	return c
}

func TestIdleWatch_NotifiesOnNewMessage(t *testing.T) {
	s, tlsc := setup(t)
	changed := make(chan struct{}, 8)
	c := dialWatch(t, s, tlsc, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- c.IdleWatch(ctx, "INBOX") }()

	waitChange(t, s, changed)

	cancel()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("IdleWatch no volvió tras cancelar")
	}
}

func TestIdleWatch_ServerGoneIsNetworkError(t *testing.T) {
	s, tlsc := setup(t)
	changed := make(chan struct{}, 8)
	c := dialWatch(t, s, tlsc, func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	done := make(chan error, 1)
	go func() { done <- c.IdleWatch(context.Background(), "INBOX") }()
	waitChange(t, s, changed) // el IDLE ya está activo
	s.Close()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, imapx.ErrNetwork)
	case <-time.After(5 * time.Second):
		t.Fatal("IdleWatch no detectó el corte")
	}
}

func TestIdleWatch_NoIdleCapIsUnsupported(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	// El servidor en memoria siempre anuncia IDLE; uno con guion no.
	settings := scriptedIMAP(t, tlsc, "")
	c, err := imapx.DialWatch(context.Background(), settings, user, pass, tlsc.Client, func() {})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Abort() })
	done := make(chan error, 1)
	go func() { done <- c.IdleWatch(context.Background(), "INBOX") }()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, imapx.ErrUnsupported)
	case <-time.After(2 * time.Second):
		t.Fatal("debía volver enseguida")
	}
}

// Si la conexión se corta al pedir las capacidades después del LOGIN, no se
// sabe si hay IDLE: es un corte de red (el vigía reconecta), no "no soportado".
func TestIdleWatch_CapsLostIsNetworkError(t *testing.T) {
	tlsc := testenv.NewTLS(t)
	settings := scriptedIMAPWith(t, tlsc, script{dropOnCapability: true})
	c, err := imapx.DialWatch(context.Background(), settings, user, pass, tlsc.Client, func() {})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Abort() })
	done := make(chan error, 1)
	go func() { done <- c.IdleWatch(context.Background(), "INBOX") }()
	select {
	case err := <-done:
		assert.ErrorIs(t, err, imapx.ErrNetwork)
		assert.NotErrorIs(t, err, imapx.ErrUnsupported)
	case <-time.After(5 * time.Second):
		t.Fatal("debía volver enseguida")
	}
}

// waitChange entrega correos hasta que onChange avisa (el IDLE está activo).
func waitChange(t *testing.T, s *testenv.IMAP, changed <-chan struct{}) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		s.Deliver(t, "INBOX", plainMsg("sonda", ""))
		select {
		case <-changed:
			return
		case <-deadline:
			t.Fatal("no llegó el aviso de correo nuevo")
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestSearchText_FindsInBody(t *testing.T) {
	s, tlsc := setup(t)
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	s.Deliver(t, "INBOX", []byte("From: Bea <bea@ej.com>\r\nTo: ana@ej.com\r\nSubject: dos\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <dos@ej.com>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nadjunto la factura del mes\r\n"))
	s.Deliver(t, "INBOX", plainMsg("tres", ""))
	c := dial(t, s, tlsc)
	_, err := c.Select("INBOX")
	require.NoError(t, err)

	uids, err := c.SearchText("factura")
	require.NoError(t, err)
	assert.Equal(t, []uint32{2}, uids)

	uids, err = c.SearchText("inexistente")
	require.NoError(t, err)
	assert.Empty(t, uids)
}

func folderNames(t *testing.T, c *imapx.Conn) map[string]imapx.RemoteFolder {
	t.Helper()
	fs, err := c.ListFolders()
	require.NoError(t, err)
	by := map[string]imapx.RemoteFolder{}
	for _, f := range fs {
		by[f.Name] = f
	}
	return by
}

func TestMailbox_CreateRenameDelete(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)

	require.NoError(t, c.CreateMailbox("Facturación 2026"))
	assert.Contains(t, folderNames(t, c), "Facturación 2026")

	require.NoError(t, c.RenameMailbox("Facturación 2026", "Clientes"))
	by := folderNames(t, c)
	assert.Contains(t, by, "Clientes")
	assert.NotContains(t, by, "Facturación 2026")

	require.NoError(t, c.DeleteMailbox("Clientes"))
	assert.NotContains(t, folderNames(t, c), "Clientes")

	// imapmemserver no renombra las inferiores: acá solo se cubre la hoja.
	assert.Error(t, c.DeleteMailbox("NoExiste"))
}

func TestMailbox_SubscribeUnsubscribe(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	require.NoError(t, c.CreateMailbox("Clientes"))
	require.NoError(t, c.Subscribe("Clientes"))
	require.NoError(t, c.Unsubscribe("Clientes"))
	assert.Error(t, c.Subscribe("NoExiste"))
}

func TestMailboxTotal(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	s.Deliver(t, "INBOX", plainMsg("uno", ""))
	s.Deliver(t, "INBOX", plainMsg("dos", ""))
	n, err := c.MailboxTotal("INBOX")
	require.NoError(t, err)
	assert.Equal(t, 2, n)
	_, err = c.MailboxTotal("NoExiste")
	assert.Error(t, err)
}

func TestListUnder(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	require.NoError(t, c.CreateMailbox("Clientes"))
	require.NoError(t, c.CreateMailbox("Clientes/Acme"))
	require.NoError(t, c.CreateMailbox("ClientesViejos"))
	by := folderNames(t, c)
	delim := by["Clientes/Acme"].Delimiter
	require.NotEmpty(t, delim)

	got, err := c.ListUnder("Clientes", delim)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Clientes"+delim+"Acme", got[0].Name)
	assert.Equal(t, delim, got[0].Delimiter)
	assert.True(t, got[0].Selectable)
}

func TestListUnder_WildcardInNameDoesNotOverMatch(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	for _, n := range []string{"100%", "100%/x", "1000", "1000/x", "100abc/x"} {
		require.NoError(t, c.CreateMailbox(n))
	}
	got, err := c.ListUnder("100%", "/")
	require.NoError(t, err)
	names := []string{}
	for _, g := range got {
		names = append(names, g.Name)
	}
	assert.Equal(t, []string{"100%/x"}, names)
}

func TestPersonalPrefix(t *testing.T) {
	s, tlsc := setup(t)
	c := dial(t, s, tlsc)
	// imapmemserver informa NAMESPACE con prefijo vacío.
	p, err := c.PersonalPrefix()
	require.NoError(t, err)
	assert.Equal(t, "", p)

	// Sin la capacidad no se manda nada y el prefijo es "".
	s2 := testenv.StartIMAPRev1(t, tlsc, user, pass)
	c2 := dial(t, s2, tlsc)
	p, err = c2.PersonalPrefix()
	require.NoError(t, err)
	assert.Equal(t, "", p)
}

package mail

import (
	"context"
	"encoding/base64"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/store"
	"postacontrol/core/testenv"
)

// newDraftsCore arma un núcleo con una cuenta ya sincronizada. Con caps nil el
// servidor anuncia IMAP4rev2 (UIDPLUS incluido). Con withDrafts el servidor
// tiene la carpeta "Drafts".
func newDraftsCore(t *testing.T, caps goimap.CapSet, withDrafts bool) (*Core, *testenv.IMAP, *testenv.SMTP, *recNotifier, model.Account) {
	t.Helper()
	tlsEnv := testenv.NewTLS(t)
	var srv *testenv.IMAP
	if caps == nil {
		srv = testenv.StartIMAP(t, tlsEnv, testUser, testPass)
	} else {
		srv = testenv.StartIMAPWithCaps(t, tlsEnv, testUser, testPass, caps)
	}
	if withDrafts {
		require.NoError(t, srv.User.Create("Drafts", nil))
	}
	smtp := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	n := &recNotifier{}
	c, err := New(Config{
		DBPath:          t.TempDir() + "/test.db",
		Secrets:         secrets.NewMemory(),
		Notifier:        n,
		TLS:             tlsEnv.Client,
		SkipInitialSync: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	acc, _ := setupSynced(t, srv, smtp, c)
	return c, srv, smtp, n, acc
}

// serverDrafts devuelve las cabeceras de los mensajes de "Drafts", consultando
// directo al servidor con una conexión aparte.
func serverDrafts(t *testing.T, c *Core, srv *testenv.IMAP) []imapx.RemoteHeader {
	t.Helper()
	conn, err := imapx.Dial(context.Background(), srv.Settings, testUser, testPass, c.cfg.TLS)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	_, err = conn.Select("Drafts")
	require.NoError(t, err)
	uids, err := conn.AllUIDs()
	require.NoError(t, err)
	hs, err := conn.FetchHeaders(uids)
	require.NoError(t, err)
	return hs
}

func draftReq(acc model.Account, subject string) model.SendRequest {
	return model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Cc: []model.Address{}, Bcc: []model.Address{},
		Subject: subject, BodyHTML: "<p>" + subject + "</p>", Attachments: []int64{},
	}
}

func webmailDraft(messageID, subject string) []byte {
	return []byte("From: " + testUser + "\r\nTo: d@x\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: " + messageID + "\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nhola\r\n")
}

func TestDrafts_SaveIsLocalOnly(t *testing.T) {
	c, srv, _, n, acc := newDraftsCore(t, nil, true)
	srv.Close() // sin servidor: guardar no toca la red
	ctx := context.Background()

	// Un borrador puede no tener destinatarios.
	req := model.SendRequest{AccountID: acc.ID, Subject: "Sin para", BodyHTML: "<p>hola</p>"}
	d, err := c.SaveDraft(ctx, 0, req)
	require.NoError(t, err)
	require.NotZero(t, d.ID)
	assert.True(t, d.NeedsUpload)
	assert.Regexp(t, `^<.+@ej\.com>$`, d.MessageID)
	assert.Equal(t, "Sin para", d.Request.Subject)
	assert.Equal(t, d.ID, d.Request.ComposeDraftID)
	assert.Equal(t, d.MessageID, d.Request.DraftMessageID)
	assert.NotEmpty(t, n.matching(EventDraftsChanged))

	// Guardar otra vez el mismo borrador conserva id y Message-ID, aunque el
	// pedido traiga otro.
	req.Subject = "Segunda"
	req.DraftMessageID = "<otro@x>"
	d2, err := c.SaveDraft(ctx, d.ID, req)
	require.NoError(t, err)
	assert.Equal(t, d.ID, d2.ID)
	assert.Equal(t, d.MessageID, d2.MessageID)
	assert.Equal(t, "Segunda", d2.Request.Subject)

	pending, err := c.ListPendingDrafts(ctx)
	require.NoError(t, err)
	require.Len(t, pending, 1)
	assert.Equal(t, d.ID, pending[0].ID)

	// Un borrador que ya no existe no se recrea.
	_, err = c.SaveDraft(ctx, d.ID+100, req)
	requireUserError(t, err, "not_found")

	// La cuenta tiene que existir.
	_, err = c.SaveDraft(ctx, 0, model.SendRequest{AccountID: acc.ID + 100})
	requireUserError(t, err, "not_found")
}

func TestDrafts_UploadAppendsWithDraftFlagAndBcc(t *testing.T) {
	c, srv, _, n, acc := newDraftsCore(t, nil, true)
	ctx := context.Background()
	req := draftReq(acc, "Borrador")
	req.Bcc = addrs("oculto@x")
	req.DraftMessageID = "<de-la-ui@x>" // el Message-ID sale de la fila, no del pedido
	d, err := c.SaveDraft(ctx, 0, req)
	require.NoError(t, err)

	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	assert.False(t, up.NeedsUpload)
	assert.Empty(t, up.UploadError)

	hs := serverDrafts(t, c, srv)
	require.Len(t, hs, 1)
	assert.True(t, hasFlag(hs[0].Flags, goimap.FlagDraft))
	assert.True(t, hasFlag(hs[0].Flags, goimap.FlagSeen))
	assert.Equal(t, d.MessageID, bracketMsgID(hs[0].Envelope.MessageID))
	require.Len(t, hs[0].Envelope.Bcc, 1)
	assert.Equal(t, "oculto@x", hs[0].Envelope.Bcc[0].Addr())

	// La carpeta Borradores local quedó sincronizada y avisada.
	draftsID := folderID(t, c, acc.ID, "Drafts")
	assert.Equal(t, []string{"Borrador"}, subjects(listAll(t, c, draftsID)))
	found := false
	for _, e := range n.matching(EventFolderUpdated) {
		if e["folderId"] == draftsID {
			found = true
		}
	}
	assert.True(t, found)
	assert.NotEmpty(t, n.matching(EventDraftsChanged))
}

func TestDrafts_ReuploadReplacesPreviousVersion(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, true)
	// Un borrador ajeno (otro Message-ID) no se toca.
	srv.Deliver(t, "Drafts", webmailDraft("<ajeno@ej.com>", "Ajeno"), goimap.FlagDraft)
	ctx := context.Background()

	var id int64
	for _, s := range []string{"v1", "v2", "v3"} {
		d, err := c.SaveDraft(ctx, id, draftReq(acc, s))
		require.NoError(t, err)
		id = d.ID
		up, err := c.UploadDraft(ctx, id)
		require.NoError(t, err)
		require.False(t, up.NeedsUpload, up.UploadError)
	}
	hs := serverDrafts(t, c, srv)
	got := []string{}
	for _, h := range hs {
		got = append(got, h.Envelope.Subject)
	}
	assert.ElementsMatch(t, []string{"Ajeno", "v3"}, got)
}

func TestDrafts_UploadWithoutUIDPlusOnlyFlagsDeleted(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, goimap.CapSet{goimap.CapIMAP4rev1: {}}, true)
	ctx := context.Background()
	d, err := c.SaveDraft(ctx, 0, draftReq(acc, "v1"))
	require.NoError(t, err)
	_, err = c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	_, err = c.SaveDraft(ctx, d.ID, draftReq(acc, "v2"))
	require.NoError(t, err)
	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	assert.False(t, up.NeedsUpload, up.UploadError)

	// Sin UIDPLUS no hay EXPUNGE: la versión vieja sigue, marcada \Deleted.
	hs := serverDrafts(t, c, srv)
	require.Len(t, hs, 2)
	assert.Equal(t, "v1", hs[0].Envelope.Subject)
	assert.True(t, hasFlag(hs[0].Flags, goimap.FlagDeleted))
	assert.Equal(t, "v2", hs[1].Envelope.Subject)
	assert.False(t, hasFlag(hs[1].Flags, goimap.FlagDeleted))
}

func TestDrafts_UploadOfflineMarksFailed(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, true)
	ctx := context.Background()
	d, err := c.SaveDraft(ctx, 0, draftReq(acc, "Sin red"))
	require.NoError(t, err)
	srv.Close()

	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err) // autoguardado: la UI solo muestra el estado
	assert.True(t, up.NeedsUpload)
	assert.Equal(t, "No pudimos conectarnos al servidor. Revisá el host, el puerto y tu conexión.", up.UploadError)

	// Un borrador que ya no existe no va a la red.
	_, err = c.UploadDraft(ctx, d.ID+100)
	requireUserError(t, err, "not_found")
}

// Con la cuenta en pausa por contraseña rechazada, el autoguardado no vuelve
// a intentar el LOGIN.
func TestDrafts_UploadSkipsPausedAccount(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, true)
	ctx := context.Background()
	d, err := c.SaveDraft(ctx, 0, draftReq(acc, "En pausa"))
	require.NoError(t, err)
	c.noteAuthFailure(acc.ID)

	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	assert.True(t, up.NeedsUpload)
	assert.Equal(t, "El servidor rechazó el usuario o la contraseña.", up.UploadError)
	assert.Empty(t, serverDrafts(t, c, srv))
}

func TestDrafts_UploadPendingRetriesInBackground(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, true)
	ctx := context.Background()
	d, err := c.SaveDraft(ctx, 0, draftReq(acc, "Pendiente"))
	require.NoError(t, err)
	srv.Close()
	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	require.True(t, up.NeedsUpload)

	srv.Restart(t)
	c.backgroundPass(ctx)

	pending, err := c.ListPendingDrafts(ctx)
	require.NoError(t, err)
	assert.Empty(t, pending)
	got, err := c.store.GetComposeDraft(ctx, d.ID)
	require.NoError(t, err)
	assert.False(t, got.NeedsUpload)
	assert.Empty(t, got.UploadError)
	hs := serverDrafts(t, c, srv)
	require.Len(t, hs, 1)
	assert.Equal(t, "Pendiente", hs[0].Envelope.Subject)

	// Una segunda pasada no vuelve a subirlo.
	c.backgroundPass(ctx)
	assert.Len(t, serverDrafts(t, c, srv), 1)
}

func TestDrafts_SaveDuringUploadStaysPending(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, true)
	ctx := context.Background()
	d, err := c.SaveDraft(ctx, 0, draftReq(acc, "v1"))
	require.NoError(t, err)

	// Mientras se sube v1, el redactor guarda v2.
	c.afterDraftAppend = func(id int64) {
		c.afterDraftAppend = nil
		_, err := c.SaveDraft(ctx, id, draftReq(acc, "v2"))
		require.NoError(t, err)
	}
	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	assert.True(t, up.NeedsUpload, "v2 todavía no está en el servidor")
	assert.Equal(t, "v2", up.Request.Subject)

	up, err = c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	assert.False(t, up.NeedsUpload)
	hs := serverDrafts(t, c, srv)
	require.Len(t, hs, 1)
	assert.Equal(t, "v2", hs[0].Envelope.Subject)
}

func TestDrafts_NoDraftsFolderKeepsLocal(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, false)
	ctx := context.Background()
	d, err := c.SaveDraft(ctx, 0, draftReq(acc, "Local"))
	require.NoError(t, err)

	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	assert.True(t, up.NeedsUpload)
	assert.Equal(t, "Esta cuenta no tiene carpeta Borradores en el servidor.", up.UploadError)
	assert.Empty(t, serverSubjects(t, c, srv, testUser, "INBOX"))
	assert.Empty(t, serverSubjects(t, c, srv, testUser, "Sent"))
}

func TestDeleteDraft_RemovesServerCopiesAndStaged(t *testing.T) {
	c, srv, _, n, acc := newDraftsCore(t, nil, true)
	srv.Deliver(t, "Drafts", webmailDraft("<ajeno@ej.com>", "Ajeno"), goimap.FlagDraft)
	ctx := context.Background()

	only, err := c.store.StageAttachment(ctx, "solo.txt", "text/plain", []byte("solo"))
	require.NoError(t, err)
	shared, err := c.store.StageAttachment(ctx, "compartido.txt", "text/plain", []byte("compartido"))
	require.NoError(t, err)
	// El adjunto compartido también lo usa un correo de la bandeja de salida.
	_, err = c.store.QueueLocalDraft(ctx, 0, model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Attachments: []int64{shared.ID}})
	require.NoError(t, err)

	req := draftReq(acc, "Descartar")
	req.Attachments = []int64{only.ID, shared.ID}
	d, err := c.SaveDraft(ctx, 0, req)
	require.NoError(t, err)
	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	require.False(t, up.NeedsUpload, up.UploadError)
	require.Len(t, serverDrafts(t, c, srv), 2)
	before := len(n.matching(EventDraftsChanged))

	require.NoError(t, c.DeleteDraft(ctx, d.ID))
	_, err = c.store.GetComposeDraft(ctx, d.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	info, err := c.StagedInfo(ctx, []int64{only.ID, shared.ID})
	require.NoError(t, err)
	require.Len(t, info, 1)
	assert.Equal(t, shared.ID, info[0].ID)
	hs := serverDrafts(t, c, srv)
	require.Len(t, hs, 1)
	assert.Equal(t, "Ajeno", hs[0].Envelope.Subject)
	assert.Greater(t, len(n.matching(EventDraftsChanged)), before)

	// Sin servidor se borra lo local igual y se avisa.
	d2, err := c.SaveDraft(ctx, 0, draftReq(acc, "Sin red"))
	require.NoError(t, err)
	srv.Close()
	ue := requireUserError(t, c.DeleteDraft(ctx, d2.ID), "draft")
	assert.Equal(t, "Borramos el borrador de esta compu, pero no pudimos borrarlo del servidor.", ue.Msg)
	_, err = c.store.GetComposeDraft(ctx, d2.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestOpenDraftMessage_OwnDraftReturnsLocal(t *testing.T) {
	c, _, _, _, acc := newDraftsCore(t, nil, true)
	ctx := context.Background()
	d, err := c.SaveDraft(ctx, 0, draftReq(acc, "Mío"))
	require.NoError(t, err)
	_, err = c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)

	m := firstMessage(t, c, folderID(t, c, acc.ID, "Drafts"))
	got, err := c.OpenDraftMessage(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, d.ID, got.ID)
	assert.Equal(t, d.MessageID, got.MessageID)
	assert.Equal(t, "Mío", got.Request.Subject)
	assert.Equal(t, d.ID, got.Request.ComposeDraftID)
}

func TestOpenDraftMessage_ForeignDraftKeepsBccAndAttachments(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, true)
	raw := []byte("From: " + testUser + "\r\nTo: Dana <d@x>\r\nCc: c@x\r\nBcc: oculto@x\r\nSubject: Del webmail\r\n" +
		"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <web@ej.com>\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: multipart/mixed; boundary=b\r\n\r\n" +
		"--b\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<p>Hola <b>webmail</b></p>\r\n" +
		"--b\r\nContent-Type: application/pdf\r\nContent-Transfer-Encoding: base64\r\n" +
		"Content-Disposition: attachment; filename=\"factura.pdf\"\r\n\r\n" + wrap76(base64.StdEncoding.EncodeToString(pdfBytes)) +
		"--b--\r\n")
	srv.Deliver(t, "Drafts", raw, goimap.FlagDraft, goimap.FlagSeen)
	ctx := context.Background()
	draftsID := folderID(t, c, acc.ID, "Drafts")
	require.NoError(t, c.SyncFolder(ctx, draftsID))
	m := firstMessage(t, c, draftsID)

	d, err := c.OpenDraftMessage(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, "<web@ej.com>", d.MessageID)
	assert.Equal(t, acc.ID, d.Request.AccountID)
	assert.Equal(t, []model.Address{{Name: "Dana", Email: "d@x"}}, d.Request.To)
	assert.Equal(t, addrs("c@x"), d.Request.Cc)
	assert.Equal(t, addrs("oculto@x"), d.Request.Bcc)
	assert.Equal(t, "Del webmail", d.Request.Subject)
	assert.Contains(t, d.Request.BodyHTML, "webmail")
	assert.Nil(t, d.Request.Reply)
	require.Len(t, d.Request.Attachments, 1)
	files, err := c.store.StagedAttachments(ctx, d.Request.Attachments)
	require.NoError(t, err)
	assert.Equal(t, "factura.pdf", files[0].Filename)
	assert.Equal(t, pdfBytes, files[0].Data)
	assert.False(t, d.NeedsUpload, "es la copia que ya está en el servidor")

	// Abrirlo otra vez devuelve el mismo borrador.
	again, err := c.OpenDraftMessage(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, d.ID, again.ID)

	// Al guardarlo y subirlo reemplaza al original.
	req := d.Request
	req.Subject = "Editado"
	_, err = c.SaveDraft(ctx, d.ID, req)
	require.NoError(t, err)
	up, err := c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	require.False(t, up.NeedsUpload, up.UploadError)
	hs := serverDrafts(t, c, srv)
	require.Len(t, hs, 1)
	assert.Equal(t, "Editado", hs[0].Envelope.Subject)
	assert.Equal(t, "<web@ej.com>", bracketMsgID(hs[0].Envelope.MessageID))
	require.Len(t, hs[0].Envelope.Bcc, 1)
	assert.True(t, hs[0].HasAttachments)
}

func TestOpenDraftMessage_RejectsNonDraftFolder(t *testing.T) {
	c, srv, _, _, acc := newDraftsCore(t, nil, true)
	srv.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "Recibido"))
	ctx := context.Background()
	inboxID := folderID(t, c, acc.ID, "INBOX")
	require.NoError(t, c.SyncFolder(ctx, inboxID))

	_, err := c.OpenDraftMessage(ctx, firstMessage(t, c, inboxID).ID)
	ue := requireUserError(t, err, "invalid")
	assert.Equal(t, "Ese correo no es un borrador.", ue.Msg)
}

func TestSend_FromDraftRemovesDraftKeepsStaged(t *testing.T) {
	c, srv, smtp, _, acc := newDraftsCore(t, nil, true)
	srv.Deliver(t, "Drafts", webmailDraft("<ajeno@ej.com>", "Ajeno"), goimap.FlagDraft)
	ctx := context.Background()
	st, err := c.store.StageAttachment(ctx, "nota.txt", "text/plain", []byte("contenido"))
	require.NoError(t, err)
	req := draftReq(acc, "Para enviar")
	req.Attachments = []int64{st.ID}
	d, err := c.SaveDraft(ctx, 0, req)
	require.NoError(t, err)
	_, err = c.UploadDraft(ctx, d.ID)
	require.NoError(t, err)
	require.Len(t, serverDrafts(t, c, srv), 2)

	send := d.Request // trae ComposeDraftID y DraftMessageID
	id, _, send, err := c.enqueue(ctx, send)
	require.NoError(t, err)
	// Encolado: el borrador ya no está, pero su adjunto sigue para el envío.
	_, err = c.store.GetComposeDraft(ctx, d.ID)
	require.ErrorIs(t, err, store.ErrNotFound)
	info, err := c.StagedInfo(ctx, []int64{st.ID})
	require.NoError(t, err)
	require.Len(t, info, 1)

	_, err = c.deliver(ctx, id, acc, send)
	require.NoError(t, err)
	envs := smtp.Received()
	require.Len(t, envs, 1)
	assert.Equal(t, []byte("contenido"), attachmentsOf(t, envs[0])["nota.txt"])
	hs := serverDrafts(t, c, srv)
	require.Len(t, hs, 1)
	assert.Equal(t, "Ajeno", hs[0].Envelope.Subject)
}

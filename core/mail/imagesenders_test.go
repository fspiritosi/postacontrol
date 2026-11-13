package mail

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const remoteImageMail = "From: Banco <Avisos@Banco.com>\r\nTo: " + testUser + "\r\nSubject: resumen\r\n" +
	"Date: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <img@ej.com>\r\nContent-Type: text/html; charset=utf-8\r\n\r\n" +
	"<p>hola</p><img src=\"https://banco.com/logo.png\">\r\n"

func TestImageSenders_TrustedSenderShowsRemoteImages(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	imap.Deliver(t, "INBOX", []byte(remoteImageMail))
	_, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	m := listAll(t, c, inboxID)[0]

	d, err := c.GetMessage(ctx, m.ID)
	require.NoError(t, err)
	require.True(t, d.HasRemoteImages)
	assert.False(t, d.RemoteImagesTrusted)

	require.NoError(t, c.TrustImageSender(ctx, " avisos@banco.COM "))
	assert.NotEmpty(t, n.matching(EventImageSendersChanged))
	d, err = c.GetMessage(ctx, m.ID)
	require.NoError(t, err)
	assert.True(t, d.RemoteImagesTrusted, "sin distinguir mayúsculas")

	list, err := c.ListImageSenders(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"avisos@banco.com"}, list)

	require.NoError(t, c.UntrustImageSender(ctx, "AVISOS@banco.com"))
	d, err = c.GetMessage(ctx, m.ID)
	require.NoError(t, err)
	assert.False(t, d.RemoteImagesTrusted)
}

func TestImageSenders_RejectsWhatIsNotAnAddress(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	var ue *UserError
	require.ErrorAs(t, c.TrustImageSender(context.Background(), "no es un correo"), &ue)
	require.ErrorAs(t, c.TrustImageSender(context.Background(), ""), &ue)
}

package mail

import (
	"context"
	"encoding/base64"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestSend_SignatureBetweenBodyAndQuote(t *testing.T) {
	c, _, smtp, acc, _, m := prepared(t, threadMail())
	sig, err := c.SaveSignature(context.Background(), model.Signature{Name: "Firma", HTML: "<p>Saludos, Fabricio</p>"})
	require.NoError(t, err)
	d, err := c.PrepareCompose(context.Background(), m.ID, "reply")
	require.NoError(t, err)
	_, err = c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: d.To, Subject: d.Subject, BodyHTML: "<p>MI-CUERPO</p>", SignatureID: sig.ID,
		Reply: &model.ReplyContext{Mode: "reply", OriginalMessageID: m.ID},
	})
	require.NoError(t, err)
	env := smtp.Received()[0]
	html := envelopePart(t, env, "text/html")
	b, s, q := strings.Index(html, "MI-CUERPO"), strings.Index(html, "Saludos, Fabricio"), strings.Index(html, "escribió")
	require.True(t, b >= 0 && s > b && q > s, html)
	assert.Contains(t, html, `<div style="margin-top:16px">`)
	text := envelopePart(t, env, "text/plain")
	b, s, q = strings.Index(text, "MI-CUERPO"), strings.Index(text, "Saludos, Fabricio"), strings.Index(text, "escribió")
	require.True(t, b >= 0 && s > b && q > s, text)
	assert.NotContains(t, text, "-- ")
	assert.Contains(t, strings.ReplaceAll(text, "\r\n", "\n"), "MI-CUERPO\n\nSaludos, Fabricio")
}

func TestSend_SignatureLogoTravelsAsCID(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG-fake"))
	sig, err := c.SaveSignature(context.Background(), model.Signature{Name: "Logo", HTML: `<p>Chau</p><img src="` + uri + `">`})
	require.NoError(t, err)
	_, err = c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s", SignatureID: sig.ID,
		BodyHTML: `<p>mirá</p><img src="` + uri + `">`,
	})
	require.NoError(t, err)
	env := smtp.Received()[0]
	html := envelopePart(t, env, "text/html")
	assert.NotContains(t, html, "data:")
	assert.Len(t, regexp.MustCompile(`src="cid:([^"]+)"`).FindAllString(html, -1), 2)
	assert.Equal(t, 1, strings.Count(string(env.Data), "Content-Id: <"))
}

func TestSend_MissingSignatureSendsWithout(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(context.Background(), model.SendRequest{
		AccountID: acc.ID, To: addrs("d@x"), Subject: "s", BodyHTML: "<p>hola</p>", SignatureID: 9999,
	})
	require.NoError(t, err)
	html := envelopePart(t, smtp.Received()[0], "text/html")
	assert.Contains(t, html, "hola")
	assert.NotContains(t, html, "margin-top:16px")
}

func TestSaveSignature_ValidatesAndSanitizes(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	_, err := c.SaveSignature(ctx, model.Signature{Name: "  ", HTML: "<p>x</p>"})
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
	assert.Equal(t, "Poné un nombre para la firma.", ue.Msg)

	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("\x89PNG-fake"))
	got, err := c.SaveSignature(ctx, model.Signature{Name: "F", HTML: `<p>hola</p><script>alert(1)</script><img src="` + uri + `">`})
	require.NoError(t, err)
	assert.NotContains(t, got.HTML, "script")
	assert.Contains(t, got.HTML, uri)
	assert.NotEmpty(t, n.matching(EventSignaturesChanged))

	list, err := c.ListSignatures(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.NoError(t, c.DeleteSignature(ctx, got.ID))
	list, _ = c.ListSignatures(ctx)
	assert.Empty(t, list)
}

func TestPrepareCompose_UsesAccountDefaultSignature(t *testing.T) {
	c, _, _, acc, _, m := prepared(t, threadMail())
	ctx := context.Background()
	d, err := c.PrepareCompose(ctx, m.ID, "reply")
	require.NoError(t, err)
	assert.Zero(t, d.SignatureID)
	sig, err := c.SaveSignature(ctx, model.Signature{Name: "F", HTML: "<p>x</p>"})
	require.NoError(t, err)
	require.NoError(t, c.SetAccountSignature(ctx, acc.ID, sig.ID))
	d, err = c.PrepareCompose(ctx, m.ID, "reply")
	require.NoError(t, err)
	assert.Equal(t, sig.ID, d.SignatureID)
}

func TestBuildOutgoing_ForDraftKeepsBccAndMessageID(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	req := model.SendRequest{AccountID: acc.ID, To: addrs("d@x"), Bcc: addrs("oculto@x"), Subject: "s",
		BodyHTML: "<p>hola</p>", DraftMessageID: "<borrador@x>"}
	out, err := c.buildOutgoing(context.Background(), acc, req, true)
	require.NoError(t, err)
	assert.True(t, out.IncludeBcc)
	assert.Equal(t, "<borrador@x>", out.MessageID)
	out, err = c.buildOutgoing(context.Background(), acc, req, false)
	require.NoError(t, err)
	assert.False(t, out.IncludeBcc)
	assert.NotEqual(t, "<borrador@x>", out.MessageID)
}

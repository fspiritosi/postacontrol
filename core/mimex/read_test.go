package mimex_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/mimex"
	"postacontrol/core/testenv"
)

const (
	user = "ana@ej.com"
	pass = "secreto"
)

// load entrega el .eml a un IMAP en memoria y devuelve la conexión y el UID,
// para trabajar con la misma BODYSTRUCTURE que en producción.
func load(t *testing.T, name string) (*imapx.Conn, uint32) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	tlsc := testenv.NewTLS(t)
	srv := testenv.StartIMAP(t, tlsc, user, pass)
	srv.Deliver(t, "INBOX", raw)
	c, err := imapx.Dial(context.Background(), srv.Settings, user, pass, tlsc.Client)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	_, err = c.Select("INBOX")
	require.NoError(t, err)
	uids, err := c.AllUIDs()
	require.NoError(t, err)
	require.Len(t, uids, 1)
	return c, uids[0]
}

func structure(t *testing.T, name string) (imap.BodyStructure, *imapx.Conn, uint32) {
	t.Helper()
	c, uid := load(t, name)
	bs, err := c.FetchStructure(uid)
	require.NoError(t, err)
	return bs, c, uid
}

func TestPlan_AlternativeWithAttachment(t *testing.T) {
	bs, _, _ := structure(t, "alt_attachment.eml")
	p := mimex.Plan(bs)
	assert.Equal(t, []int{1, 1}, p.TextPart)
	assert.Equal(t, []int{1, 2}, p.HTMLPart)
	assert.Empty(t, p.Inline)
	require.Len(t, p.Attachments, 1)
	a := p.Attachments[0]
	assert.Equal(t, "factura.pdf", a.Filename)
	assert.Equal(t, []int{2}, a.Part)
	assert.Equal(t, 0, a.Index)
	assert.Equal(t, "application/pdf", a.MIMEType)
}

func TestPlan_RelatedInlineImage(t *testing.T) {
	bs, _, _ := structure(t, "related_inline.eml")
	p := mimex.Plan(bs)
	assert.Equal(t, []int{1}, p.HTMLPart)
	require.Len(t, p.Inline, 1)
	assert.Equal(t, "logo@x", p.Inline[0].ContentID)
	assert.Equal(t, "image/png", p.Inline[0].MIMEType)
	assert.Equal(t, []int{2}, p.Inline[0].Part)
	assert.Empty(t, p.Attachments)
}

func TestPlan_OnlyTextPlain(t *testing.T) {
	bs, _, _ := structure(t, "only_text.eml")
	p := mimex.Plan(bs)
	assert.Equal(t, []int{1}, p.TextPart)
	assert.Nil(t, p.HTMLPart)
}

func TestPlan_OnlyHTML(t *testing.T) {
	bs, _, _ := structure(t, "only_html.eml")
	p := mimex.Plan(bs)
	assert.Equal(t, []int{1}, p.HTMLPart)
	assert.Nil(t, p.TextPart)
}

func TestPlan_NilStructure(t *testing.T) {
	p := mimex.Plan(nil)
	assert.Nil(t, p.TextPart)
	assert.Nil(t, p.HTMLPart)
	assert.NotNil(t, p.Attachments)
	assert.NotNil(t, p.Inline)
}

func TestParse_Latin1AndEncodedWords(t *testing.T) {
	bs, c, uid := structure(t, "latin1.eml")
	p := mimex.Plan(bs)
	require.NotNil(t, p.TextPart)
	hdr, body, err := c.FetchPart(uid, p.TextPart)
	require.NoError(t, err)
	txt, err := mimex.DecodeText(hdr, body)
	require.NoError(t, err)
	assert.Equal(t, "Información", trimNL(txt))

	assert.Equal(t, "Peña", mimex.DecodeHeaderWord("=?windows-1252?Q?Pe=F1a?="))
	assert.Equal(t, "sin codificar", mimex.DecodeHeaderWord("sin codificar"))

	require.Len(t, p.Attachments, 1)
	assert.Equal(t, "presupuestoñ.pdf", p.Attachments[0].Filename)
}

// El IMAP en memoria (como net/mime) descarta los filename* con charsets que
// no son UTF-8; servidores reales como Dovecot los entregan crudos en Params.
func TestPlan_RFC2231FilenameRawParams(t *testing.T) {
	part := func(params map[string]string) *imap.BodyStructureSinglePart {
		return &imap.BodyStructureSinglePart{
			Type: "application", Subtype: "pdf", Size: 10,
			Extended: &imap.BodyStructureSinglePartExt{
				Disposition: &imap.BodyStructureDisposition{Value: "attachment", Params: params},
			},
		}
	}
	cases := map[string]map[string]string{
		"presupuestoñ.pdf": {"filename*": "iso-8859-1''presupuesto%F1.pdf"},
		"año nuevo.pdf":    {"filename*0*": "utf-8''a%C3%B1o%20", "filename*1*": "nuevo.pdf"},
		"Peña.pdf":         {"filename": "=?windows-1252?Q?Pe=F1a?=.pdf"},
	}
	for want, params := range cases {
		p := mimex.Plan(part(params))
		require.Len(t, p.Attachments, 1, want)
		assert.Equal(t, want, p.Attachments[0].Filename)
		assert.Equal(t, []int{1}, p.Attachments[0].Part)
	}
}

func TestDecodeBinary_Base64(t *testing.T) {
	bs, c, uid := structure(t, "alt_attachment.eml")
	p := mimex.Plan(bs)
	hdr, body, err := c.FetchPart(uid, p.Attachments[0].Part)
	require.NoError(t, err)
	data, err := mimex.DecodeBinary(hdr, body)
	require.NoError(t, err)
	assert.Equal(t, "%PDF-1.4\n", string(data))
}

func TestDecodeText_WholeMessageHeader(t *testing.T) {
	// Mensaje no multipart: Task 12 baja HEADER y TEXT con FetchPart(uid, nil).
	c, uid := load(t, "only_text.eml")
	hdr, body, err := c.FetchPart(uid, nil)
	require.NoError(t, err)
	txt, err := mimex.DecodeText(hdr, body)
	require.NoError(t, err)
	assert.Equal(t, "Solo texto", trimNL(txt))
}

func TestParse_MalformedFallsBackToRaw(t *testing.T) {
	c, uid := load(t, "malformed.eml")
	hdr, body, err := c.FetchPart(uid, nil)
	require.NoError(t, err)
	txt, err := mimex.DecodeText(hdr, body)
	require.NoError(t, err)
	assert.Contains(t, txt, "cuerpo crudo sin cierre")
}

func TestDecodeText_UnknownCharsetAndNoCharset(t *testing.T) {
	txt, err := mimex.DecodeText([]byte("Content-Type: text/plain; charset=x-inventado\r\n\r\n"), []byte("hola"))
	require.NoError(t, err)
	assert.Equal(t, "hola", txt)
	// Bytes de Windows-1252 sin charset declarado
	txt, err = mimex.DecodeText([]byte("Content-Type: text/plain\r\n\r\n"), []byte("Informaci\xf3n"))
	require.NoError(t, err)
	assert.Equal(t, "Información", txt)
}

func TestResolveCID(t *testing.T) {
	in := `<img src="cid:logo@x"> <img src='cid:otro'>`
	out := mimex.ResolveCID(in, map[string]string{"logo@x": "data:image/png;base64,AAA"})
	assert.Equal(t, `<img src="data:image/png;base64,AAA"> <img src='cid:otro'>`, out)
}

func trimNL(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

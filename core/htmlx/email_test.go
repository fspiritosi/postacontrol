package htmlx

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmailHTML_InlineStylesPerElement(t *testing.T) {
	cases := map[string]string{
		"<h1>x</h1>":                  `<h1 style="margin:16px 0 8px;font-size:24px;line-height:1.25;font-weight:bold">x</h1>`,
		"<h2>x</h2>":                  `<h2 style="margin:14px 0 6px;font-size:20px;line-height:1.3;font-weight:bold">x</h2>`,
		"<h3>x</h3>":                  `<h3 style="margin:12px 0 4px;font-size:16px;line-height:1.4;font-weight:bold">x</h3>`,
		"<blockquote>x</blockquote>":  `<blockquote style="margin:8px 0;padding:0 0 0 12px;border-left:3px solid #d0d0d0;color:#555555">x</blockquote>`,
		"<ul><li>x</li></ul>":         `<ul style="margin:4px 0;padding-left:24px"><li>x</li></ul>`,
		"<ol><li>x</li></ol>":         `<ol style="margin:4px 0;padding-left:24px"><li>x</li></ol>`,
		"<hr>":                        `<hr style="border:0;border-top:1px solid #d0d0d0;margin:12px 0"/>`,
		`<img src="a.png">`:           `<img src="a.png" style="max-width:100%;height:auto"/>`,
		`<a href="https://x.y">x</a>`: `<a href="https://x.y" style="color:#2563eb;text-decoration:underline">x</a>`,
		"<table><tbody><tr><th>h</th><td>c</td></tr></tbody></table>": `<table style="border-collapse:collapse;border:1px solid #d0d0d0;margin:8px 0"><tbody><tr>` +
			`<th style="border:1px solid #d0d0d0;padding:6px 10px;vertical-align:top;text-align:left;font-weight:bold;background-color:#f3f3f3">h</th>` +
			`<td style="border:1px solid #d0d0d0;padding:6px 10px;vertical-align:top;text-align:left">c</td></tr></tbody></table>`,
	}
	for in, want := range cases {
		assert.Equal(t, want, EmailHTML(in), in)
	}
}

func TestEmailHTML_ExistingStylesWin(t *testing.T) {
	assert.Equal(t, `<p style="margin:0;color:red">x</p>`, EmailHTML(`<p style="color:red">x</p>`))
	assert.Equal(t,
		`<h1 style="margin:16px 0 8px;font-size:24px;line-height:1.25;font-weight:bold;font-size:30px">x</h1>`,
		EmailHTML(`<h1 style="font-size:30px">x</h1>`))
}

func TestEmailHTML_KeepsTightParagraphs(t *testing.T) {
	cases := map[string]string{
		"<p>uno</p><p>dos</p>":        `<p style="margin:0">uno</p><p style="margin:0">dos</p>`,
		"<p>uno</p><p></p><p>dos</p>": `<p style="margin:0">uno</p><p style="margin:0"><br/></p><p style="margin:0">dos</p>`,
		"<p> </p>":                    `<p style="margin:0"><br/></p>`,
		"<p><br></p>":                 `<p style="margin:0"><br/></p>`,
		"":                            "",
	}
	for in, want := range cases {
		assert.Equal(t, want, EmailHTML(in), in)
	}
}

func TestEmailHTML_DropsColgroup(t *testing.T) {
	got := EmailHTML(`<table><colgroup><col style="width:50px"></colgroup><tbody><tr><td>c</td></tr></tbody></table>`)
	assert.NotContains(t, got, "colgroup")
	assert.NotContains(t, got, "<col")
	assert.Contains(t, got, "<td")
}

func dataURI(mime string, payload []byte) string {
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(payload)
}

func counterCID() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("cid%d@postacontrol", n) }
}

func TestExtractInlineImages_ReplacesDataURIs(t *testing.T) {
	png, jpg := []byte("png-bytes"), []byte("jpg-bytes")
	in := `<p>a</p><img src="` + dataURI("image/png", png) + `"><p><img alt="b" src="` + dataURI("image/jpeg", jpg) + `"></p>`
	out, imgs := ExtractInlineImages(in, counterCID())
	require.Len(t, imgs, 2)
	assert.Equal(t, InlineImage{CID: "cid1@postacontrol", MIMEType: "image/png", Data: png}, imgs[0])
	assert.Equal(t, InlineImage{CID: "cid2@postacontrol", MIMEType: "image/jpeg", Data: jpg}, imgs[1])
	assert.Equal(t, `<p>a</p><img src="cid:cid1@postacontrol"/><p><img alt="b" src="cid:cid2@postacontrol"/></p>`, out)
}

func TestExtractInlineImages_DedupesIdenticalImages(t *testing.T) {
	u := dataURI("image/gif", []byte("same"))
	out, imgs := ExtractInlineImages(`<img src="`+u+`"><img src="`+u+`">`, counterCID())
	require.Len(t, imgs, 1)
	assert.Equal(t, `<img src="cid:cid1@postacontrol"/><img src="cid:cid1@postacontrol"/>`, out)
}

func TestExtractInlineImages_DropsBadDataImages(t *testing.T) {
	in := `<p>x</p><img src="data:text/html;base64,PGI+"><img src="` + dataURI("image/svg+xml", []byte("<svg/>")) +
		`"><img src="data:image/png;base64,@@no-es-base64@@"><img src="data:image/png,crudo"><p>y</p>`
	out, imgs := ExtractInlineImages(in, counterCID())
	assert.Empty(t, imgs)
	assert.Equal(t, `<p>x</p><p>y</p>`, out)
}

func TestExtractInlineImages_LeavesRemoteAndCID(t *testing.T) {
	in := `<img src="https://x.y/a.png"><img src="http://x.y/b.png"><img src="cid:abc">`
	out, imgs := ExtractInlineImages(in, counterCID())
	assert.Empty(t, imgs)
	assert.Equal(t, `<img src="https://x.y/a.png"/><img src="http://x.y/b.png"/><img src="cid:abc"/>`, out)
}

func pngDataURI(t *testing.T, w int) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, 2))))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestExtractInlineImages_WideImagesGetWidth(t *testing.T) {
	n := 0
	cid := func() string { n++; return fmt.Sprintf("c%d", n) }
	out, _ := ExtractInlineImages(`<img src="`+pngDataURI(t, 1200)+`">`, cid)
	assert.Contains(t, out, `width="600"`)
	out, _ = ExtractInlineImages(`<img src="`+pngDataURI(t, 300)+`">`, cid)
	assert.NotContains(t, out, "width=")
	out, _ = ExtractInlineImages(`<img width="100" src="`+pngDataURI(t, 1200)+`">`, cid)
	assert.Contains(t, out, `width="100"`)
	assert.NotContains(t, out, `width="600"`)
	// Con alto o con ancho en el estilo, ya tiene tamaño: no se agrega width.
	out, _ = ExtractInlineImages(`<img height="80" src="`+pngDataURI(t, 1200)+`">`, cid)
	assert.NotContains(t, out, `width="600"`)
	out, _ = ExtractInlineImages(`<img style="border:0; Width: 200px" src="`+pngDataURI(t, 1200)+`">`, cid)
	assert.NotContains(t, out, `width="600"`)
	// max-width en el estilo no es un ancho: Outlook lo ignora.
	out, _ = ExtractInlineImages(`<img style="max-width:100%" src="`+pngDataURI(t, 1200)+`">`, cid)
	assert.Contains(t, out, `width="600"`)
}

package htmlx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitize_StripsScriptsAndHandlers(t *testing.T) {
	out, _ := Sanitize(`<script>alert(1)</script><a href="javascript:x" onclick="y">a</a>`)
	assert.NotContains(t, out, "script")
	assert.NotContains(t, out, "javascript:")
	assert.NotContains(t, out, "onclick")
}

func TestSanitize_StripsDangerousElementsAndSchemes(t *testing.T) {
	in := `<iframe src="https://e.example"></iframe><object data="x"></object><embed src="x">` +
		`<form action="x"><input name="a"></form><meta http-equiv="refresh" content="0;url=x"><base href="https://e.example/">` +
		`<img src="javascript:alert(1)" onerror="x"><a href="data:text/html,<b>x</b>">d</a><a href="JaVaScRiPt:x">j</a>` +
		`<style>p{color:red}</style><p onmouseover="x" style="color:red">ok</p>`
	out, _ := Sanitize(in)
	for _, bad := range []string{"<iframe", "<object", "<embed", "<form", "<input", "<meta", "<base", "javascript:", "onerror", "onmouseover", "data:text"} {
		assert.NotContains(t, out, bad)
	}
	assert.Contains(t, out, "<style>")
	assert.Contains(t, out, `style="color:red"`)
}

func TestSanitize_JavascriptInCSSAndProtocolRelativeImages(t *testing.T) {
	out, has := Sanitize(`<style>p{background:url(javascript:alert(1))}</style><p style="background:url(JavaScript:x)">x</p><img src="//t.example/x.gif">`)
	assert.NotContains(t, strings.ToLower(out), "javascript:")
	assert.Contains(t, out, `data-remote-src="//t.example/x.gif"`)
	assert.NotContains(t, out, ` src=`)
	assert.True(t, has)
}

func TestSanitize_BlocksRemoteImages(t *testing.T) {
	out, has := Sanitize(`<img src="https://t.example/p.gif">`)
	assert.Contains(t, out, `data-remote-src="https://t.example/p.gif"`)
	assert.NotContains(t, out, ` src=`)
	assert.True(t, has)
}

func TestSanitize_KeepsDataAndCidImagesAndTables(t *testing.T) {
	out, has := Sanitize(`<img src="data:image/png;base64,AAA"><img src="cid:logo@x"><table><tr><td bgcolor="#fff" style="color:red">x</td></tr></table>`)
	assert.Contains(t, out, `src="data:image/png;base64,AAA"`)
	assert.Contains(t, out, `src="cid:logo@x"`)
	assert.Contains(t, out, `bgcolor="#fff"`)
	assert.Contains(t, out, `style="color:red"`)
	assert.False(t, has)
}

func TestRestoreRemoteImages(t *testing.T) {
	out, _ := Sanitize(`<img src="https://t.example/p.gif">`)
	got := RestoreRemoteImages(out)
	assert.Contains(t, got, `src="https://t.example/p.gif"`)
	assert.NotContains(t, got, "data-remote-src")
}

func TestToText_Structure(t *testing.T) {
	in := `<p>Hola <b>Peña</b></p><ul><li>uno</li></ul><blockquote><p>cita</p></blockquote><a href="https://x.y">sitio</a>`
	assert.Equal(t, "Hola Peña\n\n- uno\n\n> cita\n\nsitio (https://x.y)", ToText(in))
}

func TestToText_EntitiesAndLinks(t *testing.T) {
	assert.Equal(t, "a & b <c>", ToText(`<p>a &amp; b &lt;c&gt;</p>`))
	assert.Equal(t, "https://x.y", ToText(`<a href="https://x.y">https://x.y</a>`))
	assert.Equal(t, "uno\ndos", ToText(`uno<br>dos`))
	assert.Equal(t, "a\n\nb", ToText(`<p>a</p><p></p><p></p><p>b</p>`))
}

func TestSnippet_CutsOnWord(t *testing.T) {
	assert.Equal(t, "hola mundo…", Snippet("hola  mundo\nqué tal", 12))
	assert.Equal(t, "corto", Snippet(" corto ", 12))
}

func TestToText_TableCells(t *testing.T) {
	assert.Equal(t, "a b\nc d", ToText(`<table><tr><td>a</td><td>b</td></tr><tr><td>c</td><td>d</td></tr></table>`))
}

func TestToText_PreKeepsLayout(t *testing.T) {
	assert.Equal(t, "l1\n  l2", ToText("<pre>l1\n  l2</pre>"))
}

func TestSanitize_ImgSrcAllowlist(t *testing.T) {
	for _, src := range []string{"x.gif", "//h/x.gif", "http://h/x.gif"} {
		out, has := Sanitize(`<img src="` + src + `">`)
		assert.Contains(t, out, `data-remote-src="`+src+`"`, src)
		assert.NotContains(t, out, ` src=`, src)
		assert.True(t, has, src)
		assert.Contains(t, RestoreRemoteImages(out), ` src="`+src+`"`, src)
	}
	// bluemonday descarta de entrada el <img> con esquema desconocido (ftp): nada se carga.
	out, has := Sanitize(`<img src="ftp://h/x.gif">`)
	assert.NotContains(t, out, "ftp")
	assert.False(t, has)
	out, has = Sanitize(`<img src=""><img src="javascript:x"><img src="data:text/html,x">`)
	assert.NotContains(t, out, "src")
	assert.False(t, has)
}

func TestSanitize_RemoteCSSSetsFlag(t *testing.T) {
	for _, in := range []string{
		`<style>p{background:url("https://t.example/a.png")}</style>`,
		`<style>@import "//t.example/a.css";</style>`,
		`<style>@import url(http://t.example/a.css);</style>`,
		`<p style="background:URL( 'https://t.example/a.png')">x</p>`,
	} {
		out, has := Sanitize(in)
		assert.True(t, has, in)
		assert.Contains(t, out, "t.example", in) // el CSS no se modifica
	}
	_, has := Sanitize(`<p style="background:url(data:image/png;base64,AAA)">x</p>`)
	assert.False(t, has)
}

func TestTightParagraphs(t *testing.T) {
	cases := map[string]string{
		"<p>uno</p><p>dos</p>":                      `<p style="margin:0">uno</p><p style="margin:0">dos</p>`,
		"<p>uno</p><p></p><p>dos</p>":               `<p style="margin:0">uno</p><p style="margin:0"><br/></p><p style="margin:0">dos</p>`,
		"<p> </p>":                                  `<p style="margin:0"><br/></p>`,
		"<p><br></p>":                               `<p style="margin:0"><br/></p>`,
		`<p style="color:red">x</p>`:                `<p style="margin:0;color:red">x</p>`,
		"<ul><li><p>a</p></li></ul><p><b>b</b></p>": `<ul><li><p style="margin:0">a</p></li></ul><p style="margin:0"><b>b</b></p>`,
		"": "",
	}
	for in, want := range cases {
		assert.Equal(t, want, TightParagraphs(in), in)
	}
}

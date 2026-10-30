// Package htmlx sanea el HTML de los correos recibidos y lo convierte a texto plano.
package htmlx

import (
	"net/url"
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

var policy = newPolicy()

func newPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col", "center")
	p.AllowAttrs("width", "height", "align", "valign", "bgcolor", "border", "cellpadding", "cellspacing", "colspan", "rowspan").
		OnElements("table", "thead", "tbody", "tfoot", "tr", "td", "th", "colgroup", "col")
	p.AllowElements("font")
	p.AllowAttrs("color", "face", "size").OnElements("font")
	p.AllowAttrs("style").Globally()

	// bluemonday exige AllowUnsafe para permitir <style>. Es aceptable porque
	// esta política es solo la primera barrera: la real es el iframe con sandbox
	// y CSP (Task 16). Lo que sigue prohibido: <script>, on*, javascript:.
	p.AllowUnsafe(true)
	p.AllowElements("style")

	// cid: y data: se permiten como esquema; data: se restringe a <img src> en
	// restrictURLs, porque bluemonday valida esquemas sin mirar el elemento.
	p.AllowURLSchemes("http", "https", "mailto", "tel", "cid", "data")
	return p
}

// Sanitize limpia el HTML recibido. Las imágenes http(s) pasan de src a
// data-remote-src; hasRemoteImages indica si hubo alguna.
func Sanitize(in string) (clean string, hasRemoteImages bool) {
	nodes, err := parseFragment(policy.Sanitize(in))
	if err != nil {
		return "", false
	}
	for _, n := range nodes {
		if rewrite(n) {
			hasRemoteImages = true
		}
	}
	return render(nodes), hasRemoteImages
}

// RestoreRemoteImages devuelve data-remote-src a src (para citar el original al responder).
func RestoreRemoteImages(clean string) string {
	nodes, err := parseFragment(clean)
	if err != nil {
		return clean
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Img {
			for i, a := range n.Attr {
				if a.Key == "data-remote-src" {
					n.Attr[i].Key = "src"
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	return render(nodes)
}

func parseFragment(s string) ([]*html.Node, error) {
	return html.ParseFragment(strings.NewReader(s), &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body})
}

func render(nodes []*html.Node) string {
	var sb strings.Builder
	for _, n := range nodes {
		_ = html.Render(&sb, n)
	}
	return sb.String()
}

// rewrite aplica las reglas sobre el árbol y dice si encontró imágenes remotas.
func rewrite(n *html.Node) (remote bool) {
	if n.Type == html.ElementNode {
		kept := n.Attr[:0]
		for _, a := range n.Attr {
			switch a.Key {
			case "style":
				if hasJSScheme(a.Val) {
					continue
				}
			case "src", "href":
				scheme := schemeOf(a.Val)
				if scheme == "data" && !(n.DataAtom == atom.Img && a.Key == "src") {
					continue // data: solo vale para imágenes
				}
				if n.DataAtom == atom.Img && a.Key == "src" && (scheme == "http" || scheme == "https" || strings.HasPrefix(strings.TrimSpace(a.Val), "//")) {
					a.Key = "data-remote-src"
					remote = true
				}
			}
			kept = append(kept, a)
		}
		n.Attr = kept
	}
	if n.Type == html.TextNode && n.Parent != nil && n.Parent.DataAtom == atom.Style && hasJSScheme(n.Data) {
		n.Data = ""
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if rewrite(c) {
			remote = true
		}
	}
	return remote
}

func schemeOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Scheme)
}

// hasJSScheme detecta "javascript:" dentro de CSS. Los navegadores modernos no
// lo ejecutan en url(), pero se descarta el valor completo por defensa en
// profundidad (no cubre variantes ofuscadas con escapes; ahí rige el CSP).
func hasJSScheme(css string) bool {
	return strings.Contains(strings.ToLower(css), "javascript:")
}

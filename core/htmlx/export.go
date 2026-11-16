package htmlx

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var (
	exportPolicy  = newExportPolicy()
	exportDataImg = regexp.MustCompile(`(?i)^data:image/[a-z0-9.+-]+;base64,[a-z0-9+/=]+$`)
	exportEmptyIm = regexp.MustCompile(`<img\s*/?>`)
)

func newExportPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()
	p.AllowElements("p", "br", "div", "span", "h1", "h2", "h3", "h4", "h5", "h6",
		"ul", "ol", "li", "table", "thead", "tbody", "tfoot", "tr", "td", "th",
		"b", "i", "strong", "em", "u", "blockquote", "pre", "code", "hr")
	p.AllowAttrs("colspan", "rowspan").OnElements("td", "th")
	p.AllowAttrs("href").OnElements("a")
	p.AllowURLSchemes("http", "https", "mailto")
	// bluemonday valida el esquema de src sin mirar el elemento: data: se
	// acepta solo si es una imagen; el src de <img> además debe cumplir el regexp.
	p.AllowURLSchemeWithCustomPolicy("data", func(u *url.URL) bool {
		return strings.HasPrefix(strings.ToLower(u.Opaque), "image/")
	})
	p.AddTargetBlankToFullyQualifiedLinks(true) // agrega rel="noopener"
	p.AllowElements("img")
	p.AllowAttrs("src").Matching(exportDataImg).OnElements("img")
	return p
}

// SanitizeForExport limpia HTML para mandarlo a un sistema que lo muestra sin
// sandbox: sin <style>, sin atributos style, sin imágenes remotas (solo
// data:image) y solo enlaces http, https y mailto.
func SanitizeForExport(in string) string {
	return exportEmptyIm.ReplaceAllString(exportPolicy.Sanitize(in), "")
}

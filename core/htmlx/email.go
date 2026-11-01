package htmlx

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

const (
	borderGris = "1px solid #d0d0d0"
	celdaBase  = "border:" + borderGris + ";padding:6px 10px;vertical-align:top;text-align:left"
)

// emailStyles son los estilos en línea que llevan los elementos del redactor:
// los clientes de correo no ven las hojas de estilo del editor.
var emailStyles = map[atom.Atom]string{
	atom.H1:         "margin:16px 0 8px;font-size:24px;line-height:1.25;font-weight:bold",
	atom.H2:         "margin:14px 0 6px;font-size:20px;line-height:1.3;font-weight:bold",
	atom.H3:         "margin:12px 0 4px;font-size:16px;line-height:1.4;font-weight:bold",
	atom.Blockquote: "margin:8px 0;padding:0 0 0 12px;border-left:3px solid #d0d0d0;color:#555555",
	atom.Ul:         "margin:4px 0;padding-left:24px",
	atom.Ol:         "margin:4px 0;padding-left:24px",
	atom.Table:      "border-collapse:collapse;border:" + borderGris + ";margin:8px 0",
	atom.Td:         celdaBase,
	atom.Th:         celdaBase + ";font-weight:bold;background-color:#f3f3f3",
	atom.Hr:         "border:0;border-top:" + borderGris + ";margin:12px 0",
	atom.Img:        "max-width:100%;height:auto",
	atom.A:          "color:#2563eb;text-decoration:underline",
}

// imageFloats son los estilos de una imagen con align="left" o "right": el
// texto que sigue la rodea. Outlook ignora float pero respeta el align.
var imageFloats = map[string]string{
	"left":  "float:left;margin:0 12px 4px 0",
	"right": "float:right;margin:0 0 4px 12px",
}

// clearFloats cierra un fragmento con imágenes flotantes, para que no
// invadan lo que viene después (la firma o la cita).
const clearFloats = `<div style="clear:both"></div>`

// EmailHTML prepara el HTML del redactor para enviarlo: párrafos ajustados
// (ver TightParagraphs), estilos en línea por elemento (antepuestos al style
// existente, así el del usuario gana), imágenes alineadas que flotan y sin
// los <colgroup> de las tablas de TipTap, que algunos clientes dibujan mal.
func EmailHTML(fragment string) string {
	nodes, err := parseFragment(fragment)
	if err != nil {
		return fragment
	}
	floated := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode && c.DataAtom == atom.Colgroup {
				n.RemoveChild(c)
			} else {
				walk(c)
			}
			c = next
		}
		if n.Type != html.ElementNode {
			return
		}
		if n.DataAtom == atom.P {
			tightParagraph(n)
		} else if st, ok := emailStyles[n.DataAtom]; ok {
			if n.DataAtom == atom.Img {
				if fl, ok := imageFloats[strings.ToLower(attr(n, "align"))]; ok {
					st += ";" + fl
					floated = true
				}
			}
			prependStyle(n, st)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
	if floated {
		return render(nodes) + clearFloats
	}
	return render(nodes)
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func prependStyle(n *html.Node, style string) {
	for i, a := range n.Attr {
		if a.Key == "style" {
			n.Attr[i].Val = style + ";" + a.Val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: "style", Val: style})
}

// InlineImage es una imagen embebida del cuerpo, para enviarla como parte
// multipart/related referenciada por cid:.
type InlineImage struct {
	CID      string
	MIMEType string
	Data     []byte
}

var inlineImageTypes = map[string]bool{"png": true, "jpeg": true, "gif": true, "webp": true}

// ExtractInlineImages reemplaza cada <img> con data URI de imagen válida por
// una referencia cid: (newCID da el identificador; imágenes con los mismos
// bytes comparten uno). Un <img> con otro data: o con base64 inválido se
// quita. Las imágenes http(s) y cid: quedan como están.
func ExtractInlineImages(fragment string, newCID func() string) (string, []InlineImage) {
	nodes, err := parseFragment(fragment)
	if err != nil {
		return fragment, nil
	}
	var images []InlineImage
	cids := map[[sha256.Size]byte]string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		for c := n.FirstChild; c != nil; {
			next := c.NextSibling
			if c.Type == html.ElementNode && c.DataAtom == atom.Img {
				if !inlineImg(c, &images, cids, newCID) {
					n.RemoveChild(c)
				}
			} else {
				walk(c)
			}
			c = next
		}
	}
	root := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	for _, n := range nodes {
		root.AppendChild(n)
	}
	walk(root)
	var kept []*html.Node
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		kept = append(kept, c)
	}
	return render(kept), images
}

// inlineImg reescribe el src de img; devuelve false si hay que quitar la imagen.
func inlineImg(img *html.Node, images *[]InlineImage, cids map[[sha256.Size]byte]string, newCID func() string) bool {
	for i, a := range img.Attr {
		if a.Key != "src" {
			continue
		}
		val := strings.TrimSpace(a.Val)
		if len(val) < 5 || !strings.EqualFold(val[:5], "data:") {
			return true
		}
		mime, data, ok := parseImageDataURI(val)
		if !ok {
			return false
		}
		sum := sha256.Sum256(data)
		cid, seen := cids[sum]
		if !seen {
			cid = newCID()
			cids[sum] = cid
			*images = append(*images, InlineImage{CID: cid, MIMEType: mime, Data: data})
		}
		img.Attr[i].Val = "cid:" + cid
		capWidth(img, mime, data)
		return true
	}
	return true
}

// maxInlineWidth es el ancho al que se limitan las imágenes anchas: Outlook
// ignora max-width y las dibuja a su tamaño natural.
const maxInlineWidth = 600

// capWidth agrega width="600" a las imágenes PNG, JPEG o GIF más anchas que eso
// y que no traen tamaño (ni width, ni height, ni width en el estilo): un width
// junto a un height propio la deformaría. WebP queda como está (no hay
// decodificador estándar).
func capWidth(img *html.Node, mime string, data []byte) {
	if mime == "image/webp" {
		return
	}
	for _, a := range img.Attr {
		if a.Key == "width" || a.Key == "height" || (a.Key == "style" && styleHasWidth(a.Val)) {
			return
		}
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= maxInlineWidth {
		return
	}
	img.Attr = append(img.Attr, html.Attribute{Key: "width", Val: strconv.Itoa(maxInlineWidth)})
}

// styleHasWidth dice si el estilo en línea declara width (max-width y
// min-width no cuentan).
func styleHasWidth(style string) bool {
	for _, decl := range strings.Split(style, ";") {
		prop, _, ok := strings.Cut(decl, ":")
		if ok && strings.EqualFold(strings.TrimSpace(prop), "width") {
			return true
		}
	}
	return false
}

// parseImageDataURI acepta solo data:image/(png|jpeg|gif|webp);base64,<datos>.
func parseImageDataURI(uri string) (mime string, data []byte, ok bool) {
	head, payload, found := strings.Cut(uri, ",")
	if !found {
		return "", nil, false
	}
	head = strings.ToLower(head)
	kind, isB64 := strings.CutSuffix(strings.TrimPrefix(head, "data:image/"), ";base64")
	if !strings.HasPrefix(head, "data:image/") || !isB64 || !inlineImageTypes[kind] {
		return "", nil, false
	}
	payload = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, payload)
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(bytes.TrimSpace(data)) == 0 {
		return "", nil, false
	}
	return "image/" + kind, data, true
}

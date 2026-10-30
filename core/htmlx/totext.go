package htmlx

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// ToText convierte HTML a texto plano (parte text/plain del envío y snippet).
func ToText(in string) string {
	nodes, err := parseFragment(in)
	if err != nil {
		return ""
	}
	w := &textWriter{}
	for _, n := range nodes {
		w.node(n)
	}
	return w.String()
}

// Snippet colapsa espacios y corta en n runas sin partir palabras.
func Snippet(text string, n int) string {
	s := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	cut := string([]rune(s)[:n])
	if i := strings.LastIndexByte(cut, ' '); i > 0 && s[len(cut)] != ' ' {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ") + "…"
}

// textWriter solo agrega al final de sb, así que ToText es lineal. El único
// espacio que podría quedar al final de una línea es el que sigue a un
// marcador de lista ("- "): se debe (owedSpace) y se escribe recién con el
// próximo texto, o se descarta si antes viene un salto de línea.
type textWriter struct {
	sb        strings.Builder
	pending   int  // saltos de línea pendientes antes del próximo texto (0, 1 o 2)
	space     bool // hubo un espacio entre inlines que todavía no se escribió
	owedSpace bool // falta escribir el espacio que sigue a un marcador de lista
}

func (w *textWriter) String() string { return strings.TrimSpace(w.sb.String()) }

func (w *textWriter) need(n int) {
	if n > w.pending {
		w.pending = n
	}
}

func (w *textWriter) empty() bool { return w.sb.Len() == 0 && !w.owedSpace }

// breakOrJoin escribe los saltos pendientes (descartando el espacio debido,
// que quedaría al final de la línea) o, si no hay, el espacio debido.
func (w *textWriter) breakOrJoin() {
	if w.pending > 0 {
		w.owedSpace = false
		for range w.pending {
			w.sb.WriteByte('\n')
		}
		w.pending = 0
		return
	}
	if w.owedSpace {
		w.sb.WriteByte(' ')
		w.owedSpace = false
	}
}

// text escribe s (con espacios ya normalizados; puede empezar o terminar en un espacio).
func (w *textWriter) text(s string) {
	core := strings.TrimSpace(s)
	if core == "" {
		w.space = w.space || s != ""
		return
	}
	if w.empty() {
		w.pending = 0
	}
	hadBreak := w.pending > 0
	w.breakOrJoin()
	if !hadBreak && (w.space || strings.HasPrefix(s, " ")) && w.sb.Len() > 0 && !strings.HasSuffix(w.sb.String(), "\n") {
		w.sb.WriteByte(' ')
	}
	w.sb.WriteString(core)
	w.space = strings.HasSuffix(s, " ")
}

// prefix escribe un marcador de línea (como "- ") sin espacio extra después.
func (w *textWriter) prefix(s string) {
	w.text(strings.TrimRight(s, " "))
	w.owedSpace = true
	w.space = false
}

// raw escribe s sin colapsar espacios (para <pre>).
func (w *textWriter) raw(s string) {
	s = strings.TrimRight(strings.TrimLeft(s, "\r\n"), " \t\r\n")
	if strings.TrimSpace(s) == "" {
		return
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if w.empty() {
		w.pending = 0
	}
	w.breakOrJoin()
	w.sb.WriteString(s)
	w.space = false
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.DataAtom == atom.Br {
			sb.WriteByte('\n')
		}
		sb.WriteString(textContent(c))
	}
	return sb.String()
}

func (w *textWriter) children(n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.node(c)
	}
}

func (w *textWriter) node(n *html.Node) {
	switch n.Type {
	case html.TextNode:
		// los saltos de línea del código fuente son espacios; los espacios finales se conservan para separar inlines
		w.text(normalizeSpaces(n.Data))
	case html.ElementNode:
		w.element(n)
	default:
		w.children(n)
	}
}

func normalizeSpaces(s string) string {
	s = strings.NewReplacer("\n", " ", "\r", " ", "\t", " ").Replace(s)
	trailing := strings.HasSuffix(s, " ")
	if strings.TrimSpace(s) == "" {
		return " "
	}
	out := strings.Join(strings.Fields(s), " ")
	if strings.HasPrefix(s, " ") {
		out = " " + out
	}
	if trailing {
		out += " "
	}
	return out
}

func (w *textWriter) element(n *html.Node) {
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Head, atom.Title:
		return
	case atom.Br:
		w.need(1)
	case atom.P, atom.Div, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol, atom.Table:
		w.need(2)
		w.children(n)
		w.need(2)
	case atom.Tr:
		w.need(1)
		w.children(n)
		w.need(1)
	case atom.Td, atom.Th:
		w.children(n)
		for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
			if sib.Type == html.ElementNode {
				if sib.DataAtom == atom.Td || sib.DataAtom == atom.Th {
					w.space = true
				}
				break
			}
		}
	case atom.Pre:
		w.need(2)
		w.raw(textContent(n))
		w.need(2)
	case atom.Li:
		w.need(1)
		w.prefix("- ")
		w.children(n)
		w.need(1)
	case atom.Blockquote:
		inner := &textWriter{}
		inner.children(n)
		var lines []string
		for _, l := range strings.Split(inner.String(), "\n") {
			if l == "" {
				lines = append(lines, ">")
			} else {
				lines = append(lines, "> "+l)
			}
		}
		w.need(2)
		w.text(strings.Join(lines, "\n"))
		w.need(2)
	case atom.A:
		inner := &textWriter{}
		inner.children(n)
		label, href := inner.String(), ""
		for _, a := range n.Attr {
			if a.Key == "href" {
				href = strings.TrimSpace(a.Val)
			}
		}
		switch {
		case href == "" || label == href:
			w.text(label)
		case label == "":
			w.text(href)
		default:
			w.text(label + " (" + href + ")")
		}
	default:
		w.children(n)
	}
}

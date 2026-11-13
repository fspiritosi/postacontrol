package store

import (
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// SearchTerms es lo que pide el texto de una búsqueda: palabras sueltas (en
// cualquier lugar) y filtros. Sintaxis, sin distinguir mayúsculas ni tildes:
//
//	de:ana  para:juan  asunto:"factura marzo"
//	tiene:adjunto  es:no-leído  es:destacado
//	desde:2026-10-01  hasta:31/10/2026 (incluye ese día)
//
// También valen from:, to:, subject:, has:attachment, is:unread, is:starred,
// after:/since: y before: (excluye ese día). Lo que no se entiende como
// filtro (nota:x, desde:ayer, http://…) se busca como texto común.
type SearchTerms struct {
	Words         []string
	From          []string
	To            []string
	Subject       []string
	HasAttachment bool
	Unread        bool
	Flagged       bool
	Since         time.Time // incluido; cero = sin límite
	Before        time.Time // excluido; cero = sin límite
}

// HasFilters dice si hay algo más que palabras sueltas.
func (t SearchTerms) HasFilters() bool {
	return len(t.From) > 0 || len(t.To) > 0 || len(t.Subject) > 0 || t.HasAttachment || t.Unread ||
		t.Flagged || !t.Since.IsZero() || !t.Before.IsZero()
}

var stripMarks = runes.Remove(runes.In(unicode.Mn))

// fold pasa a minúsculas y sin tildes.
func fold(s string) string {
	out, _, err := transform.String(transform.Chain(norm.NFD, stripMarks, norm.NFC), strings.ToLower(s))
	if err != nil {
		return strings.ToLower(s)
	}
	return out
}

// splitSearch parte por espacios, salvo dentro de comillas ("hola mundo" o
// asunto:"hola mundo"). Las comillas no quedan.
func splitSearch(text string) []string {
	var out []string
	var cur strings.Builder
	quoted, started := false, false
	for _, r := range text {
		switch {
		case r == '"':
			quoted = !quoted
			started = true
		case unicode.IsSpace(r) && !quoted:
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if started {
		out = append(out, cur.String())
	}
	return out
}

func parseSearchDate(s string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", "02/01/2006", "2/1/2006"} {
		if d, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return d, true
		}
	}
	return time.Time{}, false
}

// ParseSearch separa el texto de la búsqueda en palabras y filtros.
func ParseSearch(text string) SearchTerms {
	var t SearchTerms
	for _, tok := range splitSearch(text) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok || val == "" {
			if strings.TrimSpace(tok) != "" {
				t.Words = append(t.Words, tok)
			}
			continue
		}
		used := true
		switch k, v := fold(key), fold(val); k {
		case "de", "from":
			t.From = append(t.From, val)
		case "para", "to":
			t.To = append(t.To, val)
		case "asunto", "subject":
			t.Subject = append(t.Subject, val)
		case "tiene", "has":
			used = v == "adjunto" || v == "adjuntos" || v == "attachment"
			t.HasAttachment = t.HasAttachment || used
		case "es", "is":
			switch v {
			case "no-leido", "noleido", "sin-leer", "unread":
				t.Unread = true
			case "destacado", "starred", "flagged":
				t.Flagged = true
			default:
				used = false
			}
		case "desde", "after", "since":
			d, ok := parseSearchDate(val)
			if used = ok; ok {
				t.Since = d
			}
		case "hasta":
			d, ok := parseSearchDate(val)
			if used = ok; ok {
				t.Before = d.AddDate(0, 0, 1)
			}
		case "antes", "before":
			d, ok := parseSearchDate(val)
			if used = ok; ok {
				t.Before = d
			}
		default:
			used = false
		}
		if !used {
			t.Words = append(t.Words, tok)
		}
	}
	return t
}

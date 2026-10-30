package mimex

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/charset" // registra los charsets legacy
	"github.com/emersion/go-message/textproto"
	"golang.org/x/text/encoding/charmap"
)

var cidRef = regexp.MustCompile(`(?i)cid:[^"'\s)>]+`)

var wordDecoder = &mime.WordDecoder{CharsetReader: charset.Reader}

// DecodeHeaderWord decodifica palabras codificadas RFC 2047, también con
// charsets legacy. Si no puede, devuelve el texto original.
func DecodeHeaderWord(s string) string {
	out, err := wordDecoder.DecodeHeader(s)
	if err != nil {
		return s
	}
	return out
}

func readHeader(mimeHeader []byte) textproto.Header {
	h, err := textproto.ReadHeader(bufio.NewReader(bytes.NewReader(mimeHeader)))
	if err != nil {
		return textproto.Header{} // cabecera ilegible: se asume text/plain sin codificar
	}
	return h
}

// DecodeText aplica el Content-Transfer-Encoding y el charset de mimeHeader a
// body y devuelve UTF-8. mimeHeader puede ser la cabecera MIME de una parte o
// la cabecera completa del mensaje. Ante un contenido que no se puede
// interpretar devuelve el cuerpo crudo, sin error.
func DecodeText(mimeHeader, body []byte) (string, error) {
	hdr := readHeader(mimeHeader)
	ent, err := message.New(message.Header{Header: hdr}, bytes.NewReader(body))
	if ent == nil { // solo falla de verdad con errores que no son de charset
		_ = err
		return toUTF8(body), nil
	}
	// Con charset desconocido message.New devuelve la entidad con el cuerpo
	// sin convertir (y un error); el multipart roto también cae acá.
	out, rerr := io.ReadAll(ent.Body)
	if rerr != nil && len(out) == 0 {
		return toUTF8(body), nil
	}
	return toUTF8(out), nil
}

// toUTF8 devuelve b como string; si no es UTF-8 válido, lo interpreta como
// Windows-1252, que es lo habitual en correo español sin charset declarado.
func toUTF8(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	if out, err := charmap.Windows1252.NewDecoder().Bytes(b); err == nil {
		return string(out)
	}
	return strings.ToValidUTF8(string(b), "�")
}

// DecodeBinary aplica solo el Content-Transfer-Encoding (base64,
// quoted-printable o identidad); nunca convierte charsets.
func DecodeBinary(mimeHeader, body []byte) ([]byte, error) {
	hdr := readHeader(mimeHeader)
	switch strings.ToLower(strings.TrimSpace(hdr.Get("Content-Transfer-Encoding"))) {
	case "base64":
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, body)
		clean = bytes.TrimRight(clean, "=")
		return base64.RawStdEncoding.DecodeString(string(clean))
	case "quoted-printable":
		return io.ReadAll(quotedprintable.NewReader(bytes.NewReader(body)))
	default:
		return append([]byte{}, body...), nil
	}
}

// ResolveCID reemplaza las referencias cid:<id> del HTML por el data URI que
// parts asocia a ese Content-ID (sin <>).
func ResolveCID(html string, parts map[string]string) string {
	return cidRef.ReplaceAllStringFunc(html, func(m string) string {
		id := m[len("cid:"):]
		if raw, err := url.PathUnescape(id); err == nil {
			if uri, ok := parts[raw]; ok {
				return uri
			}
		}
		if uri, ok := parts[id]; ok {
			return uri
		}
		return m
	})
}

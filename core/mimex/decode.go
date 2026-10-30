package mimex

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
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

// readHeader parsea la cabecera conservando los campos leídos aunque falle
// más adelante (por ejemplo, si falta la línea en blanco final).
func readHeader(mimeHeader []byte) textproto.Header {
	h, _ := textproto.ReadHeader(bufio.NewReader(bytes.NewReader(mimeHeader)))
	return h
}

// DecodeText devuelve el texto de una parte (o de un mensaje no multipart) en
// UTF-8. mimeHeader puede ser la cabecera MIME de la parte o la cabecera
// completa del mensaje; solo se usan Content-Type y Content-Transfer-Encoding.
//
// Es de mejor esfuerzo: el lector nunca debe romperse por un mensaje mal
// formado, así que ante contenido no interpretable devuelve texto y error nil:
//   - multipart/* (por ejemplo, un boundary que no cierra): el cuerpo crudo.
//   - transfer-encoding que falla al decodificar: el cuerpo crudo, nunca una
//     decodificación a medias. Un encoding desconocido se trata como identidad.
//   - charset desconocido o ausente: los bytes tal cual, como UTF-8 si son
//     válidos y si no como Windows-1252.
//   - lectura parcial al convertir el charset: los bytes leídos hasta ahí.
//
// Solo devuelve error si la conversión de charset falla sin producir ningún
// byte a partir de un cuerpo no vacío.
func DecodeText(mimeHeader, body []byte) (string, error) {
	hdr := message.Header{Header: readHeader(mimeHeader)}
	mediaType, params, _ := hdr.ContentType()
	if strings.HasPrefix(mediaType, "multipart/") {
		return toUTF8(body), nil
	}
	data, err := decodeTransfer(hdr.Header, body)
	if err != nil {
		return toUTF8(body), nil
	}
	cs := params["charset"]
	if !strings.HasPrefix(mediaType, "text/") || cs == "" {
		return toUTF8(data), nil
	}
	r, err := charset.Reader(cs, bytes.NewReader(data))
	if err != nil { // charset desconocido
		return toUTF8(data), nil
	}
	out, rerr := io.ReadAll(r)
	if rerr != nil && len(out) == 0 && len(data) > 0 {
		return "", fmt.Errorf("mimex: convertir charset %q: %w", cs, rerr)
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
	return decodeTransfer(readHeader(mimeHeader), body)
}

func decodeTransfer(hdr textproto.Header, body []byte) ([]byte, error) {
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

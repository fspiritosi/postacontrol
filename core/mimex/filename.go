package mimex

import (
	"bytes"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-message/charset"
)

// partFilename devuelve el nombre de archivo de una parte, ya decodificado.
// Mira el filename de Content-Disposition y, si falta, el name de
// Content-Type. Entiende RFC 2231 (filename*=cs”%XX y continuaciones
// filename*0*, filename*1*) y palabras codificadas RFC 2047.
func partFilename(b *imap.BodyStructureSinglePart) string {
	if d := b.Disposition(); d != nil {
		if v := paramValue(d.Params, "filename"); v != "" {
			return v
		}
	}
	return paramValue(b.Params, "name")
}

func paramValue(params map[string]string, key string) string {
	if v, ok := params[key+"*"]; ok { // RFC 2231 con charset
		if dec, ok := decode2231(v); ok {
			return dec
		}
	}
	if v := continuation(params, key); v != "" {
		return v
	}
	if v, ok := params[key]; ok {
		return DecodeHeaderWord(v)
	}
	return ""
}

// continuation une key*0, key*1, ... (con o sin "*" final de codificación).
func continuation(params map[string]string, key string) string {
	var sb strings.Builder
	encoded := false
	for i := 0; ; i++ {
		n := key + "*" + strconv.Itoa(i)
		if v, ok := params[n+"*"]; ok {
			encoded = true
			sb.WriteString(v)
		} else if v, ok := params[n]; ok {
			sb.WriteString(v)
		} else {
			break
		}
	}
	if sb.Len() == 0 {
		return ""
	}
	if encoded {
		if dec, ok := decode2231(sb.String()); ok {
			return dec
		}
	}
	return sb.String()
}

// decode2231 decodifica "charset'lang'valor%XX". Sin los apóstrofes, el valor
// se toma como UTF-8 con escapes.
func decode2231(v string) (string, bool) {
	cs, rest := "utf-8", v
	if parts := strings.SplitN(v, "'", 3); len(parts) == 3 {
		if parts[0] != "" {
			cs = parts[0]
		}
		rest = parts[2]
	}
	raw, err := url.PathUnescape(rest)
	if err != nil {
		return "", false
	}
	r, err := charset.Reader(cs, bytes.NewReader([]byte(raw)))
	if err != nil {
		return raw, true // charset desconocido: dejamos los bytes tal cual
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return "", false
	}
	return string(out), true
}

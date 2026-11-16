package mimex

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/emersion/go-message"
	"github.com/emersion/go-message/textproto"
)

// DroppedPart describe una parte que StripAttachments sacó del mensaje.
type DroppedPart struct {
	Filename string // vacío si la parte no traía nombre
	Size     int64  // bytes ya decodificados (sin base64 ni quoted-printable)
}

// StripAttachments devuelve el mensaje con los encabezados, el texto y el HTML,
// sin las partes adjuntas (Content-Disposition attachment o con filename,
// salvo text/plain y text/html sin nombre). Las imágenes inline (cid) y
// cualquier otra parte hoja que no sea cuerpo también se van. Los multipart
// quedan con los hijos que sobreviven; si uno queda vacío se descarta.
//
// Si no se sacó nada, devuelve los bytes originales sin tocar. Si el mensaje
// entero es una sola parte que se descartaría, queda con los encabezados y un
// text/plain vacío. Ante un mensaje mal formado devuelve el error.
func StripAttachments(raw []byte) ([]byte, []DroppedPart, error) {
	br := bufio.NewReader(bytes.NewReader(raw))
	hdr, err := textproto.ReadHeader(br)
	if err != nil {
		return nil, nil, fmt.Errorf("mimex: leer encabezados: %w", err)
	}
	body, err := io.ReadAll(br)
	if err != nil {
		return nil, nil, fmt.Errorf("mimex: leer cuerpo: %w", err)
	}

	var dropped []DroppedPart
	newBody, keep, err := stripEntity(hdr, body, &dropped)
	if err != nil {
		return nil, nil, err
	}
	if len(dropped) == 0 {
		return raw, nil, nil
	}

	var out bytes.Buffer
	if !keep {
		// El mensaje entero se descartó: queda un text/plain vacío.
		hdr = hdr.Copy()
		hdr.Set("Content-Type", "text/plain; charset=utf-8")
		hdr.Del("Content-Transfer-Encoding")
		hdr.Del("Content-Disposition")
		hdr.Del("Content-ID")
		newBody = nil
	}
	if err := textproto.WriteHeader(&out, hdr); err != nil {
		return nil, nil, err
	}
	out.Write(newBody)
	return out.Bytes(), dropped, nil
}

// stripEntity procesa una entidad (mensaje o parte). Devuelve el cuerpo a
// escribir y si la entidad se conserva; si no, la anota en dropped.
func stripEntity(hdr textproto.Header, body []byte, dropped *[]DroppedPart) ([]byte, bool, error) {
	mh := message.Header{Header: hdr}
	mediaType, params, err := mh.ContentType()
	if err != nil && mh.Get("Content-Type") != "" {
		return nil, false, fmt.Errorf("mimex: Content-Type inválido: %w", err)
	}
	if strings.HasPrefix(mediaType, "multipart/") {
		return stripMultipart(body, params["boundary"], dropped)
	}

	name := partName(mh)
	disp, _, _ := mh.ContentDisposition()
	isBody := (mediaType == "" || mediaType == "text/plain" || mediaType == "text/html") &&
		name == "" && disp != "attachment"
	if isBody {
		return body, true, nil
	}

	// Un transfer-encoding roto no impide listar la parte: se cuenta cruda.
	size := int64(len(body))
	if data, derr := decodeTransfer(hdr, body); derr == nil {
		size = int64(len(data))
	}
	*dropped = append(*dropped, DroppedPart{Filename: name, Size: size})
	return nil, false, nil
}

// stripMultipart reescribe un multipart conservando solo los hijos que quedan.
func stripMultipart(body []byte, boundary string, dropped *[]DroppedPart) ([]byte, bool, error) {
	if boundary == "" {
		return nil, false, errors.New("mimex: multipart sin boundary")
	}
	mr := textproto.NewMultipartReader(bytes.NewReader(body), boundary)
	var out bytes.Buffer
	mw := textproto.NewMultipartWriter(&out)
	if err := mw.SetBoundary(boundary); err != nil {
		return nil, false, fmt.Errorf("mimex: boundary inválido: %w", err)
	}
	kept := 0
	for {
		p, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, fmt.Errorf("mimex: leer multipart: %w", err)
		}
		pb, err := io.ReadAll(p)
		if err != nil {
			return nil, false, fmt.Errorf("mimex: leer parte: %w", err)
		}
		nb, keep, err := stripEntity(p.Header, pb, dropped)
		if err != nil {
			return nil, false, err
		}
		if !keep {
			continue
		}
		w, err := mw.CreatePart(p.Header)
		if err != nil {
			return nil, false, err
		}
		if _, err := w.Write(nb); err != nil {
			return nil, false, err
		}
		kept++
	}
	if kept == 0 {
		return nil, false, nil
	}
	if err := mw.Close(); err != nil {
		return nil, false, err
	}
	return out.Bytes(), true, nil
}

// partName devuelve el nombre de archivo de la parte: el filename del
// Content-Disposition o, si no hay, el name del Content-Type, ya decodificado.
func partName(h message.Header) string {
	_, dp, _ := h.ContentDisposition()
	name := dp["filename"]
	if name == "" {
		_, cp, _ := h.ContentType()
		name = cp["name"]
	}
	return DecodeHeaderWord(name)
}

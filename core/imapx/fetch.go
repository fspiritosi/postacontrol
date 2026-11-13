package imapx

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"net/textproto"
	"slices"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"
)

// ErrNoMessage: el UID ya no existe en la carpeta del servidor.
var ErrNoMessage = errors.New("imapx: mensaje inexistente")

type RemoteHeader struct {
	UID            uint32
	Flags          []imap.Flag
	Date           time.Time // INTERNALDATE; si falta, la fecha del ENVELOPE
	Size           int64
	Envelope       *imap.Envelope
	References     []string
	HasAttachments bool
}

// AllUIDs devuelve los UID de la carpeta seleccionada (UID SEARCH ALL), en
// orden ascendente.
func (c *Conn) AllUIDs() ([]uint32, error) {
	data, err := c.c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
	if err != nil {
		return nil, opErr("UID SEARCH", err)
	}
	uids := data.AllUIDs()
	out := make([]uint32, len(uids))
	for i, u := range uids {
		out[i] = uint32(u)
	}
	// El servidor debería devolverlos ordenados, pero no lo damos por sentado.
	slices.Sort(out)
	return out, nil
}

// FetchHeaders pide UID, FLAGS, ENVELOPE, INTERNALDATE, RFC822.SIZE,
// BODYSTRUCTURE y la cabecera References de cada UID.
func (c *Conn) FetchHeaders(uids []uint32) ([]RemoteHeader, error) {
	if len(uids) == 0 {
		return []RemoteHeader{}, nil
	}
	refs := &imap.FetchItemBodySection{
		Specifier:    imap.PartSpecifierHeader,
		HeaderFields: []string{"References"},
		Peek:         true,
	}
	msgs, err := c.c.Fetch(toUIDs(uids), &imap.FetchOptions{
		UID: true, Flags: true, Envelope: true, InternalDate: true, RFC822Size: true,
		BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
		BodySection:   []*imap.FetchItemBodySection{refs},
	}).Collect()
	if err != nil {
		return nil, opErr("UID FETCH headers", err)
	}
	out := make([]RemoteHeader, 0, len(msgs))
	for _, m := range msgs {
		h := RemoteHeader{
			UID:            uint32(m.UID),
			Flags:          m.Flags,
			Date:           m.InternalDate,
			Size:           m.RFC822Size,
			Envelope:       m.Envelope,
			References:     []string{},
			HasAttachments: hasAttachments(m.BodyStructure),
		}
		if h.Date.IsZero() && m.Envelope != nil {
			h.Date = m.Envelope.Date
		}
		for _, bs := range m.BodySection {
			h.References = parseReferences(bs.Bytes)
		}
		out = append(out, h)
	}
	return out, nil
}

func parseReferences(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	hdr, _ := textproto.NewReader(bufio.NewReader(bytes.NewReader(raw))).ReadMIMEHeader()
	return append([]string{}, strings.Fields(strings.Join(hdr.Values("References"), " "))...)
}

func hasAttachments(bs imap.BodyStructure) bool {
	if bs == nil {
		return false
	}
	found := false
	bs.Walk(func(_ []int, p imap.BodyStructure) bool {
		sp, ok := p.(*imap.BodyStructureSinglePart)
		if !ok {
			return true
		}
		if d := sp.Disposition(); d != nil && strings.EqualFold(d.Value, "attachment") {
			found = true
		} else if !strings.HasPrefix(sp.MediaType(), "text/") && sp.Filename() != "" {
			found = true
		}
		return true
	})
	return found
}

// FetchFlags devuelve los flags actuales de cada UID.
func (c *Conn) FetchFlags(uids []uint32) (map[uint32][]imap.Flag, error) {
	out := make(map[uint32][]imap.Flag, len(uids))
	if len(uids) == 0 {
		return out, nil
	}
	msgs, err := c.c.Fetch(toUIDs(uids), &imap.FetchOptions{UID: true, Flags: true}).Collect()
	if err != nil {
		return nil, opErr("UID FETCH flags", err)
	}
	for _, m := range msgs {
		out[uint32(m.UID)] = m.Flags
	}
	return out, nil
}

// FetchStructure devuelve la BODYSTRUCTURE del mensaje.
func (c *Conn) FetchStructure(uid uint32) (imap.BodyStructure, error) {
	msgs, err := c.c.Fetch(toUIDs([]uint32{uid}), &imap.FetchOptions{
		UID: true, BodyStructure: &imap.FetchItemBodyStructure{Extended: true},
	}).Collect()
	if err != nil {
		return nil, opErr("UID FETCH bodystructure", err)
	}
	if len(msgs) == 0 || msgs[0].BodyStructure == nil {
		return nil, fmt.Errorf("%w: UID %d", ErrNoMessage, uid)
	}
	return msgs[0].BodyStructure, nil
}

// FetchPart baja la cabecera MIME y el cuerpo (sin decodificar) de una parte,
// sin marcar el mensaje como leído. Con part vacío baja HEADER y TEXT del
// mensaje completo.
func (c *Conn) FetchPart(uid uint32, part []int) (mimeHeader []byte, body []byte, err error) {
	hdrSpec, bodySpec := imap.PartSpecifierMIME, imap.PartSpecifierNone
	if len(part) == 0 {
		hdrSpec, bodySpec = imap.PartSpecifierHeader, imap.PartSpecifierText
	}
	hs := &imap.FetchItemBodySection{Specifier: hdrSpec, Part: part, Peek: true}
	bs := &imap.FetchItemBodySection{Specifier: bodySpec, Part: part, Peek: true}
	msgs, err := c.c.Fetch(toUIDs([]uint32{uid}), &imap.FetchOptions{
		UID: true, BodySection: []*imap.FetchItemBodySection{hs, bs},
	}).Collect()
	if err != nil {
		return nil, nil, opErr(fmt.Sprintf("UID FETCH parte %v", part), err)
	}
	if len(msgs) == 0 {
		return nil, nil, fmt.Errorf("%w: UID %d", ErrNoMessage, uid)
	}
	var gotB bool
	for _, s := range msgs[0].BodySection {
		switch s.Section.Specifier {
		case hdrSpec:
			mimeHeader = s.Bytes
		case bodySpec:
			body, gotB = s.Bytes, true
		}
	}
	if !gotB {
		return nil, nil, errors.New("imapx: el servidor no devolvió la parte pedida")
	}
	if mimeHeader == nil {
		mimeHeader = []byte{}
	}
	return mimeHeader, body, nil
}

// FetchHeaderFields baja solo esos encabezados del mensaje, sin marcarlo leído
// (BODY.PEEK[HEADER.FIELDS (...)]). Devuelve el bloque de encabezados tal
// como viene, vacío si el mensaje no tiene ninguno.
func (c *Conn) FetchHeaderFields(uid uint32, fields ...string) ([]byte, error) {
	sec := &imap.FetchItemBodySection{Specifier: imap.PartSpecifierHeader, HeaderFields: fields, Peek: true}
	msgs, err := c.c.Fetch(toUIDs([]uint32{uid}), &imap.FetchOptions{
		UID: true, BodySection: []*imap.FetchItemBodySection{sec},
	}).Collect()
	if err != nil {
		return nil, opErr("UID FETCH encabezados", err)
	}
	if len(msgs) == 0 {
		return nil, fmt.Errorf("%w: UID %d", ErrNoMessage, uid)
	}
	for _, s := range msgs[0].BodySection {
		if s.Section.Specifier == imap.PartSpecifierHeader {
			return s.Bytes, nil
		}
	}
	return []byte{}, nil
}

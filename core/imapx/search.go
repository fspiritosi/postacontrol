package imapx

import (
	"slices"

	"github.com/emersion/go-imap/v2"
)

// SearchText busca en la carpeta ya abierta los mensajes que tienen todas las
// palabras, en cualquier lugar (UID SEARCH TEXT w1 TEXT w2…: cabeceras y
// cuerpo, lo resuelve el servidor). Devuelve los UID en orden ascendente. Quien
// llama hace antes el Select (EXAMINE).
func (c *Conn) SearchText(words ...string) ([]uint32, error) {
	data, err := c.c.UIDSearch(&imap.SearchCriteria{Text: words}, nil).Wait()
	if err != nil {
		return nil, opErr("UID SEARCH", err)
	}
	uids := data.AllUIDs()
	out := make([]uint32, len(uids))
	for i, u := range uids {
		out[i] = uint32(u)
	}
	slices.Sort(out)
	return out, nil
}

// SearchHeader busca en la carpeta ya abierta los mensajes cuyo encabezado
// name contiene value (UID SEARCH HEADER name value). Devuelve los UID en
// orden ascendente. Con value vacío el servidor devolvería todos los que
// tienen el encabezado, así que no busca y devuelve una lista vacía.
func (c *Conn) SearchHeader(name, value string) ([]uint32, error) {
	if value == "" {
		return []uint32{}, nil
	}
	crit := &imap.SearchCriteria{Header: []imap.SearchCriteriaHeaderField{{Key: name, Value: value}}}
	data, err := c.c.UIDSearch(crit, nil).Wait()
	if err != nil {
		return nil, opErr("UID SEARCH HEADER", err)
	}
	uids := data.AllUIDs()
	out := make([]uint32, len(uids))
	for i, u := range uids {
		out[i] = uint32(u)
	}
	slices.Sort(out)
	return out, nil
}

// SearchKeyword busca en la carpeta ya abierta los mensajes que tienen el
// keyword kw (UID SEARCH KEYWORD kw). Devuelve los UID en orden ascendente.
// Quien llama hace antes el Select.
func (c *Conn) SearchKeyword(kw string) ([]uint32, error) {
	crit := &imap.SearchCriteria{Flag: []imap.Flag{imap.Flag(kw)}}
	data, err := c.c.UIDSearch(crit, nil).Wait()
	if err != nil {
		return nil, opErr("UID SEARCH KEYWORD", err)
	}
	uids := data.AllUIDs()
	out := make([]uint32, len(uids))
	for i, u := range uids {
		out[i] = uint32(u)
	}
	slices.Sort(out)
	return out, nil
}

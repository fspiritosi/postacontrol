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

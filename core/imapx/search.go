package imapx

import (
	"slices"

	"github.com/emersion/go-imap/v2"
)

// SearchText busca text en la carpeta ya abierta (UID SEARCH TEXT: cabeceras y
// cuerpo, lo resuelve el servidor). Devuelve los UID en orden ascendente. Quien
// llama hace antes el Select (EXAMINE).
func (c *Conn) SearchText(text string) ([]uint32, error) {
	data, err := c.c.UIDSearch(&imap.SearchCriteria{Text: []string{text}}, nil).Wait()
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

package imapx

import (
	"fmt"
	"slices"
	"strings"

	"github.com/emersion/go-imap/v2"

	"postacontrol/core/model"
)

type RemoteFolder struct {
	Name, Delimiter string
	Role            model.FolderRole
	Selectable      bool
	Unread, Total   int
}

// ListFolders hace LIST "" "*" y, de cada carpeta seleccionable, STATUS
// (MESSAGES UNSEEN).
func (c *Conn) ListFolders() ([]RemoteFolder, error) {
	var opts *imap.ListOptions
	caps := c.c.Caps()
	if caps.Has(imap.CapSpecialUse) && caps.Has(imap.CapListExtended) {
		opts = &imap.ListOptions{ReturnSpecialUse: true}
	}
	items, err := c.c.List("", "*", opts).Collect()
	if err != nil {
		return nil, opErr("LIST", err)
	}
	out := make([]RemoteFolder, 0, len(items))
	for _, it := range items {
		if slices.Contains(it.Attrs, imap.MailboxAttrNonExistent) {
			continue
		}
		delim := ""
		if it.Delim != 0 {
			delim = string(it.Delim)
		}
		f := RemoteFolder{
			Name:       it.Mailbox,
			Delimiter:  delim,
			Role:       DetectRole(it.Mailbox, delim, it.Attrs),
			Selectable: !slices.Contains(it.Attrs, imap.MailboxAttrNoSelect),
		}
		if f.Selectable {
			st, err := c.c.Status(it.Mailbox, &imap.StatusOptions{NumMessages: true, NumUnseen: true}).Wait()
			if err != nil {
				return nil, opErr(fmt.Sprintf("STATUS %q", it.Mailbox), err)
			}
			if st.NumMessages != nil {
				f.Total = int(*st.NumMessages)
			}
			if st.NumUnseen != nil {
				f.Unread = int(*st.NumUnseen)
			}
		}
		out = append(out, f)
	}
	return out, nil
}

type SelectInfo struct {
	UIDValidity, UIDNext uint32
	NumMessages          uint32
}

// Select abre la carpeta en solo lectura (EXAMINE).
func (c *Conn) Select(name string) (SelectInfo, error) { return c.selectMailbox(name, true) }

// SelectRW abre la carpeta con permiso de escritura (SELECT).
func (c *Conn) SelectRW(name string) (SelectInfo, error) { return c.selectMailbox(name, false) }

func (c *Conn) selectMailbox(name string, readOnly bool) (SelectInfo, error) {
	d, err := c.c.Select(name, &imap.SelectOptions{ReadOnly: readOnly}).Wait()
	if err != nil {
		c.rw = false
		return SelectInfo{}, opErr(fmt.Sprintf("select %q", name), err)
	}
	c.rw = !readOnly
	return SelectInfo{UIDValidity: d.UIDValidity, UIDNext: uint32(d.UIDNext), NumMessages: d.NumMessages}, nil
}

var roleByName = map[string]model.FolderRole{
	"sent": "sent", "sent items": "sent", "sent messages": "sent", "enviados": "sent",
	"drafts": "drafts", "borradores": "drafts",
	"trash": "trash", "deleted items": "trash", "papelera": "trash",
	"junk": "junk", "spam": "junk",
	"archive": "archive", "archivo": "archive",
}

// DetectRole decide el rol de una carpeta: primero por atributos special-use,
// después INBOX, y por último por el último segmento del nombre.
func DetectRole(name, delim string, attrs []imap.MailboxAttr) model.FolderRole {
	for _, a := range attrs {
		switch a {
		case imap.MailboxAttrSent:
			return "sent"
		case imap.MailboxAttrDrafts:
			return "drafts"
		case imap.MailboxAttrTrash:
			return "trash"
		case imap.MailboxAttrJunk:
			return "junk"
		case imap.MailboxAttrArchive:
			return "archive"
		}
	}
	if strings.EqualFold(name, "INBOX") {
		return "inbox"
	}
	last := name
	if delim != "" {
		if i := strings.LastIndex(last, delim); i >= 0 {
			last = last[i+len(delim):]
		}
	}
	// Algunos servidores usan "." aunque el delimitador reportado sea otro.
	if i := strings.LastIndexAny(last, "./"); i >= 0 {
		last = last[i+1:]
	}
	return roleByName[strings.ToLower(strings.TrimSpace(last))]
}

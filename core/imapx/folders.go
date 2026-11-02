package imapx

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/emersion/go-imap/v2"

	"postacontrol/core/model"
)

type RemoteFolder struct {
	Name, Delimiter string
	Role            model.FolderRole
	// SpecialUse dice si el rol salió de un atributo special-use del servidor
	// (y no del nombre).
	SpecialUse    bool
	Selectable    bool
	Unread, Total int
}

// ListFolders hace LIST "" "*" y, de cada carpeta seleccionable, STATUS
// (MESSAGES UNSEEN). Cada rol queda en una sola carpeta (ver UniqueRoles).
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
			SpecialUse: specialUseRole(it.Attrs) != "",
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
	return UniqueRoles(out), nil
}

// UniqueRoles deja cada rol en una sola carpeta de la cuenta; las demás
// candidatas quedan sin rol. Sin esto, una subcarpeta "Clientes.Papelera" o
// "Proyectos/Trash" también sería la Papelera y borrar, enviar o la vista
// unificada podrían usar la equivocada. Gana, en orden: la seleccionable, la
// que el servidor marca con special-use, la menos profunda (menos segmentos)
// y, si empatan, la primera por nombre. Modifica fs y lo devuelve.
func UniqueRoles(fs []RemoteFolder) []RemoteFolder {
	winner := map[model.FolderRole]int{}
	for i, f := range fs {
		if f.Role == "" {
			continue
		}
		if j, ok := winner[f.Role]; !ok || betterRoleCandidate(f, fs[j]) {
			winner[f.Role] = i
		}
	}
	for i := range fs {
		if r := fs[i].Role; r != "" && winner[r] != i {
			fs[i].Role = ""
			fs[i].SpecialUse = false
		}
	}
	return fs
}

func betterRoleCandidate(a, b RemoteFolder) bool {
	if a.Selectable != b.Selectable {
		return a.Selectable
	}
	if a.SpecialUse != b.SpecialUse {
		return a.SpecialUse
	}
	if da, db := folderDepth(a), folderDepth(b); da != db {
		return da < db
	}
	return a.Name < b.Name
}

func folderDepth(f RemoteFolder) int {
	if f.Delimiter == "" {
		return 1
	}
	return strings.Count(f.Name, f.Delimiter) + 1
}

type SelectInfo struct {
	UIDValidity, UIDNext uint32
	NumMessages          uint32
	// KeywordsOK: PERMANENTFLAGS trae \*, así que el servidor guarda keywords
	// nuevos. Solo es confiable después de SelectRW: con EXAMINE muchos
	// servidores no informan ningún flag permanente.
	KeywordsOK bool
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
	return SelectInfo{
		UIDValidity: d.UIDValidity, UIDNext: uint32(d.UIDNext), NumMessages: d.NumMessages,
		KeywordsOK: slices.Contains(d.PermanentFlags, imap.FlagWildcard),
	}, nil
}

var roleByName = map[string]model.FolderRole{
	"sent": "sent", "sent items": "sent", "sent messages": "sent", "enviados": "sent",
	"drafts": "drafts", "borradores": "drafts",
	"trash": "trash", "deleted items": "trash", "papelera": "trash",
	"junk": "junk", "spam": "junk",
	"archive": "archive", "archivo": "archive",
}

// DetectRole decide el rol de una carpeta: primero por atributos special-use,
// después INBOX, y por último por el último segmento del nombre. El nombre
// solo cuenta en el primer nivel o justo debajo de INBOX (el prefijo personal
// de Dovecot, «INBOX.»): una subcarpeta que el usuario llamó "Archivo" o
// "Spam" adentro de otra no es especial aunque la cuenta no tenga esa
// carpeta. Más adentro, el rol sale solo de special-use.
func DetectRole(name, delim string, attrs []imap.MailboxAttr) model.FolderRole {
	if r := specialUseRole(attrs); r != "" {
		return r
	}
	if strings.EqualFold(name, "INBOX") {
		return "inbox"
	}
	cut, sep := -1, 0
	if delim != "" {
		if i := strings.LastIndex(name, delim); i >= 0 {
			cut, sep = i, len(delim)
		}
	}
	// Algunos servidores usan "." aunque el delimitador reportado sea otro.
	if i := strings.LastIndexAny(name, "./"); i > cut {
		cut, sep = i, 1
	}
	last := name
	if cut >= 0 {
		if !strings.EqualFold(name[:cut], "INBOX") {
			return ""
		}
		last = name[cut+sep:]
	}
	return roleByName[strings.ToLower(strings.TrimSpace(last))]
}

// specialUseRole devuelve el rol que indica un atributo special-use, o "".
func specialUseRole(attrs []imap.MailboxAttr) model.FolderRole {
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
	return ""
}

// CreateMailbox crea un buzón (CREATE). El nombre ya viene completo, con el
// prefijo y el delimitador de la cuenta.
func (c *Conn) CreateMailbox(name string) error {
	if err := c.c.Create(name, nil).Wait(); err != nil {
		return opErr(fmt.Sprintf("CREATE %q", name), err)
	}
	return nil
}

// RenameMailbox renombra un buzón (RENAME). El servidor renombra también las
// inferiores.
func (c *Conn) RenameMailbox(oldName, newName string) error {
	if err := c.c.Rename(oldName, newName, nil).Wait(); err != nil {
		return opErr(fmt.Sprintf("RENAME %q", oldName), err)
	}
	return nil
}

// DeleteMailbox borra un buzón (DELETE).
func (c *Conn) DeleteMailbox(name string) error {
	if err := c.c.Delete(name).Wait(); err != nil {
		return opErr(fmt.Sprintf("DELETE %q", name), err)
	}
	return nil
}

// Subscribe suscribe el buzón (SUBSCRIBE).
func (c *Conn) Subscribe(name string) error {
	if err := c.c.Subscribe(name).Wait(); err != nil {
		return opErr(fmt.Sprintf("SUBSCRIBE %q", name), err)
	}
	return nil
}

// Unsubscribe quita la suscripción del buzón (UNSUBSCRIBE).
func (c *Conn) Unsubscribe(name string) error {
	if err := c.c.Unsubscribe(name).Wait(); err != nil {
		return opErr(fmt.Sprintf("UNSUBSCRIBE %q", name), err)
	}
	return nil
}

// PersonalPrefix devuelve el prefijo del primer namespace personal
// (NAMESPACE), por ejemplo "INBOX." en Dovecot. Sin la capacidad, o si el
// servidor no informa ninguno, devuelve "" sin mandar nada.
func (c *Conn) PersonalPrefix() (string, error) {
	caps := c.c.Caps()
	if !caps.Has(imap.CapNamespace) && !caps.Has(imap.CapIMAP4rev2) {
		return "", nil
	}
	data, err := c.c.Namespace().Wait()
	if err != nil {
		return "", opErr("NAMESPACE", err)
	}
	if len(data.Personal) == 0 {
		return "", nil
	}
	return data.Personal[0].Prefix, nil
}

// ErrNoMessageCount: la respuesta de STATUS no trajo MESSAGES.
var ErrNoMessageCount = errors.New("el servidor no informó MESSAGES")

// MailboxTotal devuelve la cantidad de mensajes del buzón (STATUS (MESSAGES)).
// Si la respuesta no trae MESSAGES (el servidor no lo informa, o go-imap la
// descarta porque el nombre que devuelve no coincide exacto) es un error
// (ErrNoMessageCount) y no 0: con esto se decide si una carpeta quedó vacía
// para borrarla.
func (c *Conn) MailboxTotal(name string) (int, error) {
	st, err := c.c.Status(name, &imap.StatusOptions{NumMessages: true}).Wait()
	if err != nil {
		return 0, opErr(fmt.Sprintf("STATUS %q", name), err)
	}
	if st.NumMessages == nil {
		return 0, fmt.Errorf("imapx: STATUS %q: %w", name, ErrNoMessageCount)
	}
	return int(*st.NumMessages), nil
}

// ListUnder lista los buzones que cuelgan de name (LIST "" "name<delim>*"), sin
// STATUS y sin incluir a name. Los contadores quedan en cero.
func (c *Conn) ListUnder(name, delim string) ([]RemoteFolder, error) {
	items, err := c.c.List("", name+delim+"*", nil).Collect()
	if err != nil {
		return nil, opErr(fmt.Sprintf("LIST %q", name), err)
	}
	out := []RemoteFolder{}
	for _, it := range items {
		if it.Mailbox == name || slices.Contains(it.Attrs, imap.MailboxAttrNonExistent) {
			continue
		}
		// Si name trae «*» o «%», el patrón matchea de más: solo valen los que
		// de verdad cuelgan de name.
		if !strings.HasPrefix(it.Mailbox, name+delim) {
			continue
		}
		out = append(out, listedFolder(it))
	}
	return out, nil
}

// Lookup busca un buzón por su nombre exacto (LIST "" name), sin STATUS. ok es
// false si el servidor no lo lista.
func (c *Conn) Lookup(name string) (f RemoteFolder, ok bool, err error) {
	items, err := c.c.List("", name, nil).Collect()
	if err != nil {
		return RemoteFolder{}, false, opErr(fmt.Sprintf("LIST %q", name), err)
	}
	for _, it := range items {
		// Con «*» o «%» en el nombre el patrón matchea de más.
		if it.Mailbox == name && !slices.Contains(it.Attrs, imap.MailboxAttrNonExistent) {
			return listedFolder(it), true, nil
		}
	}
	return RemoteFolder{}, false, nil
}

// listedFolder arma la carpeta de una respuesta de LIST, sin contadores.
func listedFolder(it *imap.ListData) RemoteFolder {
	d := ""
	if it.Delim != 0 {
		d = string(it.Delim)
	}
	return RemoteFolder{
		Name:       it.Mailbox,
		Delimiter:  d,
		Role:       DetectRole(it.Mailbox, d, it.Attrs),
		SpecialUse: specialUseRole(it.Attrs) != "",
		Selectable: !slices.Contains(it.Attrs, imap.MailboxAttrNoSelect),
	}
}

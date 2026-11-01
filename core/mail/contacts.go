package mail

import (
	"context"
	"errors"
	"net/mail"
	"sort"
	"strings"

	"postacontrol/core/model"
	"postacontrol/core/store"
)

// suggestLimit es el máximo de sugerencias de destinatarios.
const suggestLimit = 8

// displayName es el nombre que se muestra de una ficha: el nombre o, si no
// tiene, su primer correo.
func displayName(c model.Contact) string {
	if c.Name != "" {
		return c.Name
	}
	if len(c.Emails) > 0 {
		return c.Emails[0]
	}
	return ""
}

// stripPhone deja el teléfono sin espacios ni guiones, para comparar.
func stripPhone(s string) string {
	return strings.NewReplacer(" ", "", "-", "").Replace(s)
}

// ListContacts devuelve las fichas ordenadas por el nombre que se muestra. Con
// query, solo las que lo contienen (sin acentos ni mayúsculas) en el nombre, la
// empresa o algún correo, o en algún teléfono sin espacios ni guiones.
func (c *Core) ListContacts(ctx context.Context, query string) ([]model.Contact, error) {
	list, err := c.store.ListContacts(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	q := fold(strings.TrimSpace(query))
	if q != "" {
		qp := stripPhone(q)
		kept := list[:0]
		for _, ct := range list {
			if contactMatches(ct, q, qp) {
				kept = append(kept, ct)
			}
		}
		list = kept
	}
	sort.SliceStable(list, func(i, j int) bool {
		return fold(displayName(list[i])) < fold(displayName(list[j]))
	})
	return list, nil
}

func contactMatches(ct model.Contact, q, qPhone string) bool {
	if strings.Contains(fold(ct.Name), q) || strings.Contains(fold(ct.Company), q) {
		return true
	}
	for _, e := range ct.Emails {
		if strings.Contains(fold(e), q) {
			return true
		}
	}
	if qPhone != "" {
		for _, p := range ct.Phones {
			if strings.Contains(stripPhone(p), qPhone) {
				return true
			}
		}
	}
	return false
}

// GetContact devuelve la ficha; si no existe, un not_found.
func (c *Core) GetContact(ctx context.Context, id int64) (model.Contact, error) {
	ct, err := c.store.GetContact(ctx, id)
	if err != nil {
		return model.Contact{}, c.ctxOrUserError(ctx, err)
	}
	return ct, nil
}

// SaveContact crea la ficha (ID 0) o actualiza la existente. Recorta los
// textos, quita correos y teléfonos vacíos y deduplica los correos sin
// distinguir mayúsculas.
func (c *Core) SaveContact(ctx context.Context, ct model.Contact) (model.Contact, error) {
	ct.Name = strings.TrimSpace(ct.Name)
	ct.Company = strings.TrimSpace(ct.Company)
	ct.Notes = strings.TrimSpace(ct.Notes)
	ct.Emails = normalizeEmails(ct.Emails)
	phones := []string{}
	for _, p := range ct.Phones {
		if p = strings.TrimSpace(p); p != "" {
			phones = append(phones, p)
		}
	}
	ct.Phones = phones
	if ct.Name == "" && len(ct.Emails) == 0 {
		return model.Contact{}, &UserError{Code: "invalid", Msg: "Poné un nombre o un correo."}
	}
	for _, e := range ct.Emails {
		if a, err := mail.ParseAddress(e); err != nil || a.Address != e {
			return model.Contact{}, &UserError{Code: "invalid", Msg: "«" + e + "» no es un correo válido."}
		}
	}
	saved, err := c.store.SaveContact(ctx, ct)
	var taken store.ErrEmailTaken
	if errors.As(err, &taken) {
		name := taken.Email
		if other, gerr := c.store.GetContact(ctx, taken.ContactID); gerr == nil {
			name = displayName(other)
		}
		return model.Contact{}, &UserError{Code: "duplicate", Msg: "«" + emailAsTyped(ct.Emails, taken.Email) + "» ya está en la ficha de " + name + ".", Err: err}
	}
	if err != nil {
		return model.Contact{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventContactsChanged, nil)
	return saved, nil
}

// normalizeEmails recorta, descarta los vacíos y deduplica sin distinguir
// mayúsculas, conservando el primero.
func normalizeEmails(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, e := range in {
		e = strings.TrimSpace(e)
		k := strings.ToLower(e)
		if e == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, e)
	}
	return out
}

// emailAsTyped devuelve el correo tal como lo escribió el usuario (el store
// puede devolverlo con otra capitalización).
func emailAsTyped(typed []string, email string) string {
	for _, e := range typed {
		if strings.EqualFold(e, email) {
			return e
		}
	}
	return email
}

// DeleteContact borra la ficha. Los correos no se tocan.
func (c *Core) DeleteContact(ctx context.Context, id int64) error {
	if err := c.store.DeleteContact(ctx, id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventContactsChanged, nil)
	return nil
}

// ContactForEmail devuelve la ficha dueña del correo, o nil si nadie lo tiene.
func (c *Core) ContactForEmail(ctx context.Context, email string) (*model.Contact, error) {
	id, err := c.store.ContactIDForEmail(ctx, strings.TrimSpace(email))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	ct, err := c.store.GetContact(ctx, id)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return &ct, nil
}

// AddSenderToContacts devuelve la ficha del remitente; si todavía no tiene,
// la crea con su nombre y su correo.
func (c *Core) AddSenderToContacts(ctx context.Context, addr model.Address) (model.Contact, error) {
	existing, err := c.ContactForEmail(ctx, addr.Email)
	if err != nil {
		return model.Contact{}, err
	}
	if existing != nil {
		return *existing, nil
	}
	return c.SaveContact(ctx, model.Contact{Name: strings.TrimSpace(addr.Name), Emails: []string{addr.Email}})
}

// SuggestRecipients sugiere direcciones para lo escrito en un campo de
// destinatarios: primero los contactos y después el resto de los destinatarios
// usados; en cada grupo, antes los que empiezan con lo escrito. exclude son los
// correos que ya están en el campo.
func (c *Core) SuggestRecipients(ctx context.Context, query string, exclude []string) ([]model.Suggestion, error) {
	q := fold(strings.TrimSpace(query))
	if q == "" {
		return []model.Suggestion{}, nil
	}
	skip := map[string]bool{}
	for _, e := range exclude {
		skip[strings.ToLower(strings.TrimSpace(e))] = true
	}
	contacts, err := c.store.ListContacts(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	recips, err := c.store.ListRecipients(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}

	type cand struct {
		model.Suggestion
		prefix bool
	}
	var fromContacts, fromRecips []cand
	sort.SliceStable(contacts, func(i, j int) bool {
		return fold(displayName(contacts[i])) < fold(displayName(contacts[j]))
	})
	inBook := map[string]bool{} // todos los correos de las fichas, aunque no coincidan
	for _, ct := range contacts {
		for _, e := range ct.Emails {
			inBook[strings.ToLower(e)] = true
		}
	}
	for _, ct := range contacts {
		for _, e := range ct.Emails {
			if skip[strings.ToLower(e)] {
				continue
			}
			if ok, prefix := suggestMatch(q, ct.Name, e); ok {
				fromContacts = append(fromContacts, cand{model.Suggestion{Name: ct.Name, Email: e, ContactID: ct.ID}, prefix})
			}
		}
	}
	sort.SliceStable(recips, func(i, j int) bool { // manda el uso
		if recips[i].Times != recips[j].Times {
			return recips[i].Times > recips[j].Times
		}
		return recips[i].LastUsed > recips[j].LastUsed
	})
	for _, r := range recips {
		k := strings.ToLower(r.Email)
		if skip[k] || inBook[k] {
			continue
		}
		if ok, prefix := suggestMatch(q, r.Name, r.Email); ok {
			fromRecips = append(fromRecips, cand{model.Suggestion{Name: r.Name, Email: r.Email}, prefix})
		}
	}

	out := []model.Suggestion{}
	for _, group := range [][]cand{fromContacts, fromRecips} {
		for _, pass := range []bool{true, false} {
			for _, x := range group {
				if x.prefix == pass && len(out) < suggestLimit {
					out = append(out, x.Suggestion)
				}
			}
		}
	}
	return out, nil
}

// suggestMatch dice si q (ya plegado) está en el nombre o el correo, y si
// empieza el correo o alguna palabra del nombre.
func suggestMatch(q, name, email string) (match, prefix bool) {
	fn, fe := fold(name), fold(email)
	if !strings.Contains(fn, q) && !strings.Contains(fe, q) {
		return false, false
	}
	if strings.HasPrefix(fe, q) {
		return true, true
	}
	for _, w := range strings.Fields(fn) {
		if strings.HasPrefix(w, q) {
			return true, true
		}
	}
	return true, false
}

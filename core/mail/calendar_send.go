package mail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"postacontrol/core/icalx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	msgEventNoAccount   = "Elegí desde qué cuenta mandar las invitaciones."
	msgInviteSendFailed = "El evento se guardó, pero no pudimos mandar las invitaciones: "
	msgCancelSendFailed = "El evento se borró, pero no pudimos mandar las cancelaciones: "
)

// invitation es un aviso por mandar a los invitados de un evento.
type invitation struct {
	event  model.CalendarEvent
	method string // "REQUEST" o "CANCEL"
	to     []model.Address
	update bool      // REQUEST de algo que el invitado ya tenía
	when   time.Time // inicio que va en el asunto; cero = el del evento
	bumps  bool      // es de prev editado, que sube de SEQUENCE
}

// notices dice qué hay que mandarles a los invitados por lo que dejó ed. A
// los que siguen invitados les va la serie entera y a los quitados la
// cancelación, con SEQUENCE+1, solo si cambió algo que les importa; a un
// alta (o a la serie nueva de "following"), la invitación.
func (ed eventEdit) notices() []invitation {
	if ed.updated == nil && ed.created == nil {
		if len(ed.prev.Attendees) == 0 {
			return nil
		}
		gone := ed.prev
		gone.Sequence++
		return []invitation{{event: gone, method: "CANCEL", to: attendeeAddrs(gone.Attendees)}}
	}
	var out []invitation
	if ed.updated != nil && (len(ed.prev.Attendees) > 0 || len(ed.updated.Attendees) > 0) && mattersChanged(ed.prev, *ed.updated) {
		next := *ed.updated
		next.Sequence = ed.prev.Sequence + 1
		kept, added, removed := diffAttendees(ed.prev.Attendees, next.Attendees)
		if len(kept) > 0 {
			out = append(out, invitation{event: next, method: "REQUEST", to: kept, update: true, when: ed.when, bumps: true})
		}
		if len(added) > 0 {
			out = append(out, invitation{event: next, method: "REQUEST", to: added, when: ed.when, bumps: true})
		}
		if len(removed) > 0 {
			gone := next
			gone.Attendees = removed
			out = append(out, invitation{event: gone, method: "CANCEL", to: attendeeAddrs(removed), when: ed.when, bumps: true})
		}
	}
	if ed.created != nil && len(ed.created.Attendees) > 0 {
		out = append(out, invitation{event: *ed.created, method: "REQUEST", to: attendeeAddrs(ed.created.Attendees)})
	}
	return out
}

// preview resume ed para avisarle al usuario antes de guardar o borrar.
func (ed eventEdit) preview() model.SavePreview {
	p := model.SavePreview{NotifyNeeded: len(ed.notices()) > 0}
	after := ed.updated
	if ed.created != nil {
		after = ed.created
	}
	if after != nil {
		_, added, removed := diffAttendees(ed.prev.Attendees, after.Attendees)
		p.Added, p.Removed = len(added), len(removed)
	}
	return p
}

// prepareNotices calcula los avisos de ed (si notify), trae la cuenta desde
// la que salen, la pone de organizador y sube la SEQUENCE de lo editado si le
// sale algo. No guarda nada.
func (c *Core) prepareNotices(ctx context.Context, ed *eventEdit, notify bool) (model.Account, []invitation, error) {
	if !notify || len(ed.notices()) == 0 {
		return model.Account{}, nil, nil
	}
	acc, err := c.inviteAccount(ctx, ed.accountID())
	if err != nil {
		return model.Account{}, nil, err
	}
	org := model.Address{Name: acc.DisplayName, Email: acc.Email}
	for _, e := range []*model.CalendarEvent{&ed.prev, ed.updated, ed.created} {
		if e != nil {
			e.Organizer = org
		}
	}
	notices := ed.notices()
	for _, n := range notices {
		if n.bumps {
			ed.updated.Sequence = ed.prev.Sequence + 1
		}
	}
	return acc, notices, nil
}

// sendNotices encola los avisos, uno por invitado. Se llama con calMu tomado
// y después de guardar ed, así dos guardados no mandan lo mismo dos veces. Si
// encolar falla, lo guardado queda (con la SEQUENCE de antes si no salió nada
// de lo editado) y el error empieza con failed.
func (c *Core) sendNotices(ctx context.Context, ed *eventEdit, acc model.Account, notices []invitation, failed string) error {
	bumpedSent := false
	for _, n := range notices {
		for _, to := range n.to {
			err := c.queueInvitationAt(ctx, acc, n.event, n.method, []model.Address{to}, n.update, n.when)
			if err == nil {
				bumpedSent = bumpedSent || n.bumps
				continue
			}
			if ed.updated != nil && ed.updated.Sequence != ed.prev.Sequence && !bumpedSent {
				rev := *ed.updated
				rev.Sequence = ed.prev.Sequence
				if saved, rerr := c.saveEvent(context.WithoutCancel(ctx), rev); rerr == nil {
					ed.updated = &saved
				}
			}
			return &UserError{Code: "send", Msg: failed + c.ctxOrUserError(ctx, err).Error(), Err: err}
		}
	}
	return nil
}

// planEventSave prepara el guardado del evento e (ya validado) pedido en req:
// saca de los invitados la dirección de la cuenta del evento (nadie se invita
// a sí mismo) y, si quedan invitados, pone esa cuenta de organizador.
func (c *Core) planEventSave(ctx context.Context, e model.CalendarEvent, req model.EventSave) (eventEdit, error) {
	acc, err := c.eventAccount(ctx, e.AccountID)
	if err != nil {
		return eventEdit{}, err
	}
	if acc != nil {
		e.Attendees = withoutEmail(e.Attendees, acc.Email)
	}
	var ed eventEdit
	if e.ID == 0 {
		ed = planCreate(e)
	} else if ed, err = c.planSave(ctx, e, req.Scope, req.OccurrenceAt.UTC()); err != nil {
		return eventEdit{}, err
	}
	if acc != nil {
		org := model.Address{Name: acc.DisplayName, Email: acc.Email}
		for _, ev := range []*model.CalendarEvent{ed.updated, ed.created} {
			if ev != nil && ev.AccountID == acc.ID && len(ev.Attendees) > 0 {
				ev.Organizer = org
			}
		}
	}
	return ed, nil
}

// eventAccount trae la cuenta del evento; nil si no tiene o ya no existe.
func (c *Core) eventAccount(ctx context.Context, id int64) (*model.Account, error) {
	if id == 0 {
		return nil, nil
	}
	acc, err := c.store.GetAccount(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return &acc, nil
}

// withoutEmail saca de los invitados la dirección email (sin distinguir mayúsculas).
func withoutEmail(as []model.Attendee, email string) []model.Attendee {
	out := make([]model.Attendee, 0, len(as))
	for _, a := range as {
		if !strings.EqualFold(a.Email, email) {
			out = append(out, a)
		}
	}
	return out
}

// inviteAccount trae la cuenta desde la que salen las invitaciones.
func (c *Core) inviteAccount(ctx context.Context, id int64) (model.Account, error) {
	if id == 0 {
		return model.Account{}, &UserError{Code: "invalid", Msg: msgEventNoAccount}
	}
	acc, err := c.store.GetAccount(ctx, id)
	if err != nil {
		return model.Account{}, c.ctxOrUserError(ctx, err)
	}
	return acc, nil
}

// PreviewEventSave dice, sin guardar nada, si guardar req les cambia algo a
// los invitados y a cuántos se agrega o se quita.
func (c *Core) PreviewEventSave(ctx context.Context, req model.EventSave) (model.SavePreview, error) {
	e := req.Event
	if err := validateEvent(&e); err != nil {
		return model.SavePreview{}, err
	}
	ed, err := c.planEventSave(ctx, e, req)
	if err != nil {
		return model.SavePreview{}, err
	}
	return ed.preview(), nil
}

// PreviewEventDelete dice, sin borrar nada, si borrar req les cambia algo a
// los invitados.
func (c *Core) PreviewEventDelete(ctx context.Context, req model.EventDelete) (model.SavePreview, error) {
	ed, err := c.planDelete(ctx, req)
	if err != nil {
		return model.SavePreview{}, err
	}
	return ed.preview(), nil
}

// queueInvitation encola, desde acc, un correo por destinatario de to con el
// evento e como iMIP (method REQUEST o CANCEL). update marca un REQUEST de
// algo que el destinatario ya tenía. Para en el primer error.
func (c *Core) queueInvitation(ctx context.Context, acc model.Account, e model.CalendarEvent, method string, to []model.Address, update bool) error {
	return c.queueInvitationAt(ctx, acc, e, method, to, update, time.Time{})
}

// queueInvitationAt es queueInvitation con el inicio que va en el asunto
// (cero = el del evento).
func (c *Core) queueInvitationAt(ctx context.Context, acc model.Account, e model.CalendarEvent, method string, to []model.Address, update bool, when time.Time) error {
	ics, err := icalx.Build(method, eventToSeries(e), time.Now())
	if err != nil {
		return err
	}
	subject := invitationSubject(e, method, update, when)
	body := invitationHTML(e, time.Local)
	for _, a := range to {
		_, err := c.QueueSend(ctx, model.SendRequest{AccountID: acc.ID, To: []model.Address{a}, Subject: subject,
			BodyHTML: body, Calendar: &model.CalendarPart{Method: method, ICS: string(ics)}})
		if err != nil {
			return err
		}
	}
	return nil
}

// invitationSubject arma "Invitación: <título> — <cuándo>" (o "Invitación
// actualizada:" o "Cancelado:").
func invitationSubject(e model.CalendarEvent, method string, update bool, when time.Time) string {
	prefix := "Invitación: "
	switch {
	case method == "CANCEL":
		prefix = "Cancelado: "
	case update:
		prefix = "Invitación actualizada: "
	}
	if when.IsZero() {
		when = e.Start
	}
	cuando := formatDateES(when)
	if e.AllDay {
		cuando = dateES(when.UTC())
	}
	return prefix + e.Title + " — " + cuando
}

// invitationHTML es el cuerpo de la invitación: una tabla con qué, cuándo
// (en loc), dónde y la descripción. Las notas nunca salen.
func invitationHTML(e model.CalendarEvent, loc *time.Location) string {
	var b strings.Builder
	b.WriteString(`<table cellpadding="4" style="border-collapse:collapse">`)
	row := func(label, value string) {
		fmt.Fprintf(&b, `<tr><th align="left" valign="top" style="padding-right:16px">%s</th><td>%s</td></tr>`, label, value)
	}
	row("Qué", html.EscapeString(e.Title))
	cuando := whenRange(e, loc)
	if e.RRule != "" {
		cuando += " · se repite"
	}
	row("Cuándo", html.EscapeString(cuando))
	if e.Location != "" {
		row("Dónde", html.EscapeString(e.Location))
	}
	if e.Description != "" {
		desc := html.EscapeString(strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(e.Description))
		row("Descripción", strings.ReplaceAll(desc, "\n", "<br>"))
	}
	b.WriteString(`</table>`)
	return b.String()
}

// whenRange es "inicio – fin": con hora, en loc (sin repetir la fecha si es
// el mismo día); de día completo, las fechas sin convertir (el fin es
// exclusivo).
func whenRange(e model.CalendarEvent, loc *time.Location) string {
	if e.AllDay {
		first, last := e.Start.UTC(), e.End.UTC().AddDate(0, 0, -1)
		if !last.After(first) {
			return dateES(first)
		}
		return dateES(first) + " – " + dateES(last)
	}
	s, end := e.Start.In(loc), e.End.In(loc)
	if dateES(s) == dateES(end) {
		return fmt.Sprintf("%s – %02d:%02d", formatDateIn(s, loc), end.Hour(), end.Minute())
	}
	return formatDateIn(s, loc) + " – " + formatDateIn(end, loc)
}

// eventToSeries traduce un evento a iCalendar: las excepciones canceladas van
// como EXDATE y las cambiadas como VEVENT con RECURRENCE-ID. Las notas no.
func eventToSeries(e model.CalendarEvent) icalx.Series {
	status := "CONFIRMED"
	if e.Status == "cancelled" {
		status = "CANCELLED"
	}
	atts := make([]icalx.Attendee, 0, len(e.Attendees))
	for _, a := range e.Attendees {
		atts = append(atts, icalx.Attendee{Email: a.Email, Name: a.Name, Partstat: a.Partstat})
	}
	base := icalx.VEvent{UID: e.UID, Sequence: e.Sequence, Status: status, AllDay: e.AllDay,
		Organizer: e.Organizer, Attendees: atts}
	s := icalx.Series{Master: base}
	m := &s.Master
	m.Summary, m.Description, m.Location = e.Title, e.Description, e.Location
	m.Start, m.End, m.RRule, m.TZID = e.Start.UTC(), e.End.UTC(), e.RRule, e.TZID

	xs := append([]model.EventException(nil), e.Exceptions...)
	sort.Slice(xs, func(i, j int) bool { return xs[i].RecurrenceAt.Before(xs[j].RecurrenceAt) })
	for _, x := range xs {
		if x.Cancelled {
			m.ExDates = append(m.ExDates, x.RecurrenceAt.UTC())
			continue
		}
		ov := base
		ov.Summary, ov.Description, ov.Location = x.Title, x.Description, x.Location
		ov.Start, ov.End, ov.RecurrenceID = x.Start.UTC(), x.End.UTC(), x.RecurrenceAt.UTC()
		s.Overrides = append(s.Overrides, ov)
	}
	return s
}

// mattersChanged dice si de a a b cambió algo que les importa a los
// invitados (no las notas, el aviso, la cuenta ni el nombre o la respuesta de
// un invitado).
func mattersChanged(a, b model.CalendarEvent) bool {
	return a.Title != b.Title || !a.Start.Equal(b.Start) || !a.End.Equal(b.End) || a.AllDay != b.AllDay ||
		a.Location != b.Location || a.Description != b.Description || a.RRule != b.RRule || a.Status != b.Status ||
		!sameStrings(exceptionKeys(a.Exceptions), exceptionKeys(b.Exceptions)) ||
		!sameStrings(attendeeEmails(a.Attendees), attendeeEmails(b.Attendees))
}

func exceptionKeys(xs []model.EventException) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, fmt.Sprintf("%d|%t|%d|%d|%q|%q|%q", x.RecurrenceAt.Unix(), x.Cancelled, x.Start.Unix(), x.End.Unix(),
			x.Title, x.Location, x.Description))
	}
	return out
}

func attendeeEmails(as []model.Attendee) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, a := range as {
		if e := strings.ToLower(a.Email); !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	return out
}

// sameStrings compara dos listas como conjuntos.
func sameStrings(a, b []string) bool {
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}

// diffAttendees separa los invitados de after en los que ya estaban en before
// y los agregados, y devuelve los de before que ya no están.
func diffAttendees(before, after []model.Attendee) (kept, added []model.Address, removed []model.Attendee) {
	was := map[string]bool{}
	for _, a := range before {
		was[strings.ToLower(a.Email)] = true
	}
	is := map[string]bool{}
	for _, a := range after {
		e := strings.ToLower(a.Email)
		if is[e] {
			continue
		}
		is[e] = true
		if was[e] {
			kept = append(kept, model.Address{Name: a.Name, Email: a.Email})
		} else {
			added = append(added, model.Address{Name: a.Name, Email: a.Email})
		}
	}
	for _, a := range before {
		if !is[strings.ToLower(a.Email)] {
			removed = append(removed, a)
		}
	}
	return kept, added, removed
}

func attendeeAddrs(as []model.Attendee) []model.Address {
	out := make([]model.Address, 0, len(as))
	for _, a := range as {
		out = append(out, model.Address{Name: a.Name, Email: a.Email})
	}
	return out
}

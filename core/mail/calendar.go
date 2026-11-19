package mail

import (
	"context"
	"fmt"
	"net/mail"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"postacontrol/core/icalx"
	"postacontrol/core/model"
)

const (
	msgEventNoTitle     = "Poné un título."
	msgEventEndBefore   = "El fin tiene que ser después del inicio."
	msgEventBadReminder = "Esa anticipación de aviso no es válida."
	msgEventBadInvitee  = "Ese invitado no tiene una dirección válida: %s"
	msgEventNotLocal    = "Este evento no se edita desde acá."
	msgEventBadScope    = "No sabemos qué parte de la serie cambiar."
	taskBlock           = 30 * time.Minute
)

// occurrence es una ocurrencia de un evento, con las excepciones aplicadas.
type occurrence struct {
	Original    time.Time // inicio original (UTC)
	Start, End  time.Time
	Title       string
	Location    string
	Description string
}

// overlaps dice si [s, e) se superpone con [from, to); un evento sin duración
// cuenta si empieza dentro del rango (igual que icalx.Expand).
func overlaps(s, e, from, to time.Time) bool {
	if !s.Before(to) {
		return false
	}
	if !e.After(s) {
		return !s.Before(from)
	}
	return e.After(from)
}

func expand(e model.CalendarEvent, from, to time.Time) []time.Time {
	return icalx.Expand(e.Start, e.End, e.AllDay, e.TZID, e.RRule, nil, from, to)
}

// isOccurrence dice si t es el inicio original de una ocurrencia del evento.
func isOccurrence(e model.CalendarEvent, t time.Time) bool {
	for _, s := range expand(e, t, t.Add(time.Second)) {
		if s.Equal(t) {
			return true
		}
	}
	return false
}

// countBefore cuenta las ocurrencias de la serie que empiezan antes de t.
func countBefore(e model.CalendarEvent, t time.Time) int {
	n, from := 0, e.Start
	for {
		fresh := 0
		var last time.Time
		for _, s := range expand(e, from, t) {
			if !s.Before(from) { // Expand también trae las que empezaron antes y siguen
				n, fresh, last = n+1, fresh+1, s
			}
		}
		if fresh == 0 {
			return n
		}
		from = last.Add(time.Second)
	}
}

// occurrences devuelve las ocurrencias del evento que se superponen con
// [from, to), ordenadas: una excepción cancelada saca la ocurrencia y una
// movida reemplaza su hora y sus textos (y cuenta aunque el inicio original
// quede fuera del rango).
func (c *Core) occurrences(e model.CalendarEvent, from, to time.Time) []occurrence {
	dur := e.End.Sub(e.Start)
	exc := map[int64]model.EventException{}
	if e.RRule != "" {
		for _, x := range e.Exceptions {
			exc[x.RecurrenceAt.Unix()] = x
		}
	}
	out := []occurrence{}
	for _, s := range expand(e, from, to) {
		if _, ok := exc[s.Unix()]; ok {
			continue
		}
		out = append(out, occurrence{Original: s, Start: s, End: s.Add(dur), Title: e.Title, Location: e.Location, Description: e.Description})
	}
	for _, x := range exc {
		if x.Cancelled || !overlaps(x.Start, x.End, from, to) || !isOccurrence(e, x.RecurrenceAt) {
			continue
		}
		out = append(out, occurrence{Original: x.RecurrenceAt.UTC(), Start: x.Start.UTC(), End: x.End.UTC(),
			Title: x.Title, Location: x.Location, Description: x.Description})
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Start.Equal(out[j].Start) {
			return out[i].Start.Before(out[j].Start)
		}
		return out[i].Original.Before(out[j].Original)
	})
	return out
}

// ListCalendar devuelve lo que hay en [from, to): los eventos locales y las
// invitaciones (salvo las rechazadas) y las tareas locales pendientes con fecha.
func (c *Core) ListCalendar(ctx context.Context, from, to time.Time) (model.CalendarRange, error) {
	evs, err := c.store.EventsInRange(ctx, from, to)
	if err != nil {
		return model.CalendarRange{}, c.ctxOrUserError(ctx, err)
	}
	items := []model.CalendarItem{}
	for _, e := range evs {
		if (e.Kind != "local" && e.Kind != "invite") || (e.Kind == "invite" && e.MyPartstat == "DECLINED") {
			continue
		}
		confirmed := 0
		for _, a := range e.Attendees {
			if a.Partstat == "ACCEPTED" {
				confirmed++
			}
		}
		partstat := ""
		if e.Kind == "invite" {
			partstat = e.MyPartstat
		}
		for _, o := range c.occurrences(e, from, to) {
			items = append(items, model.CalendarItem{
				Key: fmt.Sprintf("e:%d:%d", e.ID, o.Original.Unix()), Source: e.Kind, EventID: e.ID,
				Title: o.Title, Location: o.Location, Start: o.Start, End: o.End, AllDay: e.AllDay,
				Recurring: e.RRule != "", OccurrenceAt: o.Original, Partstat: partstat, NeedsReview: e.NeedsReview,
				Cancelled: e.Status == "cancelled", Attendees: len(e.Attendees), Confirmed: confirmed,
			})
		}
	}
	tasks, err := c.store.ListLocalTasks(ctx, false)
	if err != nil {
		return model.CalendarRange{}, c.ctxOrUserError(ctx, err)
	}
	for _, t := range tasks {
		it, ok := taskItem(t)
		if ok && overlaps(it.Start, it.End, from, to) {
			items = append(items, it)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if !items[i].Start.Equal(items[j].Start) {
			return items[i].Start.Before(items[j].Start)
		}
		return items[i].Key < items[j].Key
	})
	return model.CalendarRange{Items: items}, nil
}

// taskItem arma el ítem de una tarea con fecha: con hora, un bloque de media
// hora en la zona local; sin hora, de día completo. false si no tiene fecha.
func taskItem(t model.LocalTask) (model.CalendarItem, bool) {
	if t.DueDate == "" || t.DoneAt != nil {
		return model.CalendarItem{}, false
	}
	it := model.CalendarItem{Key: fmt.Sprintf("t:%d", t.ID), Source: "task", TaskID: t.ID, Title: t.Title}
	if t.DueTime != "" {
		start, err := time.ParseInLocation("2006-01-02 15:04", t.DueDate+" "+t.DueTime, time.Local)
		if err != nil {
			return model.CalendarItem{}, false
		}
		it.Start, it.End = start, start.Add(taskBlock)
		return it, true
	}
	day, err := time.ParseInLocation("2006-01-02", t.DueDate, time.UTC)
	if err != nil {
		return model.CalendarItem{}, false
	}
	it.Start, it.End, it.AllDay = day, day.AddDate(0, 0, 1), true
	return it, true
}

// GetEvent devuelve el evento con sus invitados y excepciones.
func (c *Core) GetEvent(ctx context.Context, id int64) (model.CalendarEvent, error) {
	e, err := c.store.GetEvent(ctx, id)
	if err != nil {
		return model.CalendarEvent{}, c.ctxOrUserError(ctx, err)
	}
	return e, nil
}

// validateEvent normaliza y valida lo que se puede cambiar de un evento.
func validateEvent(e *model.CalendarEvent) error {
	e.Title = strings.TrimSpace(e.Title)
	if e.Title == "" {
		return &UserError{Code: "invalid", Msg: msgEventNoTitle}
	}
	if e.AllDay {
		// Día completo: medianoche UTC de la fecha, fin exclusivo.
		e.Start, e.End = utcDate(e.Start), utcDate(e.End)
		if !e.End.After(e.Start) {
			e.End = e.Start.AddDate(0, 0, 1)
		}
	}
	if !e.End.After(e.Start) {
		return &UserError{Code: "invalid", Msg: msgEventEndBefore}
	}
	if e.RemindBefore != nil && !validRemindMinutes(*e.RemindBefore) {
		return &UserError{Code: "invalid", Msg: msgEventBadReminder}
	}
	// Sin repetidos: por email, gana el último y queda en el lugar del primero.
	atts := make([]model.Attendee, 0, len(e.Attendees))
	pos := map[string]int{}
	for _, a := range e.Attendees {
		addr, err := mail.ParseAddress(strings.TrimSpace(a.Email))
		if err != nil || !strings.Contains(addr.Address, "@") {
			return &UserError{Code: "invalid", Msg: fmt.Sprintf(msgEventBadInvitee, a.Email)}
		}
		a.Email = strings.ToLower(addr.Address)
		if strings.TrimSpace(a.Name) == "" {
			a.Name = addr.Name
		}
		if a.Partstat == "" {
			a.Partstat = "NEEDS-ACTION"
		}
		if i, ok := pos[a.Email]; ok {
			atts[i] = a
			continue
		}
		pos[a.Email] = len(atts)
		atts = append(atts, a)
	}
	e.Attendees = atts
	e.Start, e.End = e.Start.UTC(), e.End.UTC()
	return nil
}

// utcDate es la medianoche UTC de la fecha (UTC) de t.
func utcDate(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// seriesEnd es el fin de la última ocurrencia (nil si la serie no termina),
// estirado si alguna ocurrencia se movió más allá.
func seriesEnd(e model.CalendarEvent) *time.Time {
	end := e.End
	if e.RRule != "" {
		t, ok := icalx.SeriesEnd(e.Start, e.End, e.AllDay, e.TZID, e.RRule)
		if !ok {
			return nil
		}
		end = t
	}
	for _, x := range e.Exceptions {
		if !x.Cancelled && x.End.After(end) {
			end = x.End
		}
	}
	end = end.UTC()
	return &end
}

func (c *Core) saveEvent(ctx context.Context, e model.CalendarEvent) (model.CalendarEvent, error) {
	saved, err := c.store.SaveEvent(ctx, e, seriesEnd(e))
	if err != nil {
		return model.CalendarEvent{}, c.ctxOrUserError(ctx, err)
	}
	return saved, nil
}

// localEvent trae el evento a editar; solo los locales se editan desde acá.
func (c *Core) localEvent(ctx context.Context, id int64) (model.CalendarEvent, error) {
	prev, err := c.store.GetEvent(ctx, id)
	if err != nil {
		return model.CalendarEvent{}, c.ctxOrUserError(ctx, err)
	}
	if prev.Kind != "local" {
		return model.CalendarEvent{}, &UserError{Code: "invalid", Msg: msgEventNotLocal}
	}
	return prev, nil
}

// effectiveScope traduce el alcance pedido: sin serie o desde la primera
// ocurrencia, "following" equivale a toda la serie.
func effectiveScope(prev model.CalendarEvent, scope string, at time.Time) (string, error) {
	switch scope {
	case "", "this", "following":
	default:
		return "", &UserError{Code: "invalid", Msg: msgEventBadScope}
	}
	if scope == "" || prev.RRule == "" {
		return "", nil
	}
	if !isOccurrence(prev, at) {
		return "", &UserError{Code: "not_found", Msg: msgNotFound}
	}
	if scope == "following" && countBefore(prev, at) == 0 {
		return "", nil
	}
	return scope, nil
}

// eventEdit es lo que deja un alta, una edición o un borrado, todavía sin
// guardar. Sin updated ni created, se borra prev entero.
type eventEdit struct {
	prev    model.CalendarEvent  // el evento como estaba (cero en un alta)
	updated *model.CalendarEvent // prev con los cambios (mismo ID)
	created *model.CalendarEvent // el alta, o la serie nueva que deja "following"
	when    time.Time            // inicio de la ocurrencia que cambia ("this"); cero = el del evento
}

// accountID es la cuenta desde la que se avisa a los invitados.
func (ed eventEdit) accountID() int64 {
	switch {
	case ed.created != nil:
		return ed.created.AccountID
	case ed.updated != nil:
		return ed.updated.AccountID
	}
	return ed.prev.AccountID
}

// planCreate prepara el alta de un evento local ya validado.
func planCreate(e model.CalendarEvent) eventEdit {
	e.UID = uuid.NewString() + "@postacontrol"
	e.Kind, e.Status, e.Sequence = "local", "confirmed", 0
	if e.TZID == "" {
		e.TZID = localTZID()
	}
	e.Exceptions = []model.EventException{}
	return eventEdit{created: &e}
}

// planSave prepara la edición de un evento local ya validado: entero, solo una
// ocurrencia ("this") o desde una ocurrencia en adelante ("following").
func (c *Core) planSave(ctx context.Context, e model.CalendarEvent, scope string, at time.Time) (eventEdit, error) {
	prev, err := c.localEvent(ctx, e.ID)
	if err != nil {
		return eventEdit{}, err
	}
	if scope, err = effectiveScope(prev, scope, at); err != nil {
		return eventEdit{}, err
	}
	ed := eventEdit{prev: prev}
	switch scope {
	case "this":
		next := prev
		next.Exceptions = withException(prev.Exceptions, model.EventException{RecurrenceAt: at, Start: e.Start, End: e.End,
			Title: e.Title, Location: e.Location, Description: e.Description})
		ed.updated, ed.when = &next, e.Start
	case "following":
		next, cut := splitSeries(prev, e, at), cutSeries(prev, at)
		ed.created, ed.updated = &next, &cut
	default:
		next := edited(prev, e)
		next.Exceptions = shiftExceptions(prev.Exceptions, next.Start.Sub(prev.Start))
		next.Exceptions = validExceptions(next, next.Exceptions)
		ed.updated = &next
	}
	return ed, nil
}

// planDelete prepara el borrado de un evento local entero, de una ocurrencia
// ("this") o desde una ocurrencia en adelante ("following").
func (c *Core) planDelete(ctx context.Context, req model.EventDelete) (eventEdit, error) {
	prev, err := c.localEvent(ctx, req.ID)
	if err != nil {
		return eventEdit{}, err
	}
	at := req.OccurrenceAt.UTC()
	scope, err := effectiveScope(prev, req.Scope, at)
	if err != nil {
		return eventEdit{}, err
	}
	ed := eventEdit{prev: prev}
	switch scope {
	case "this":
		next := prev
		next.Exceptions = withException(prev.Exceptions, model.EventException{RecurrenceAt: at, Cancelled: true,
			Start: at, End: at.Add(prev.End.Sub(prev.Start))})
		ed.updated, ed.when = &next, at
	case "following":
		cut := cutSeries(prev, at)
		ed.updated = &cut
	}
	return ed, nil
}

// applyEdit guarda lo preparado y deja en ed lo que quedó guardado. Con una
// serie cortada, primero el alta y después el corte: si algo falla, a lo sumo
// queda una ocurrencia repetida, nunca se pierde una.
func (c *Core) applyEdit(ctx context.Context, ed *eventEdit) error {
	if ed.created == nil && ed.updated == nil {
		if err := c.store.DeleteEvent(ctx, ed.prev.ID); err != nil {
			return c.ctxOrUserError(ctx, err)
		}
		return nil
	}
	if ed.created != nil {
		saved, err := c.saveEvent(ctx, *ed.created)
		if err != nil {
			return err
		}
		ed.created = &saved
	}
	if ed.updated != nil {
		saved, err := c.saveEvent(ctx, *ed.updated)
		if err != nil {
			if ed.created != nil {
				_ = c.store.DeleteEvent(context.WithoutCancel(ctx), ed.created.ID)
			}
			return err
		}
		ed.updated = &saved
	}
	return nil
}

// SaveEvent da de alta un evento local (ID 0) o lo edita: entero, solo una
// ocurrencia ("this") o desde una ocurrencia en adelante ("following"). Un
// alta con invitados les manda la invitación; una edición, si req.Notify, les
// avisa lo que cambió.
func (c *Core) SaveEvent(ctx context.Context, req model.EventSave) (model.CalendarEvent, error) {
	e := req.Event
	if err := validateEvent(&e); err != nil {
		return model.CalendarEvent{}, err
	}
	c.calMu.Lock()
	defer c.calMu.Unlock()
	ed, err := c.planEventSave(ctx, e, req)
	if err != nil {
		return model.CalendarEvent{}, err
	}
	acc, notices, err := c.prepareNotices(ctx, &ed, req.Notify || e.ID == 0)
	if err != nil {
		return model.CalendarEvent{}, err
	}
	if err := c.applyEdit(ctx, &ed); err != nil {
		return model.CalendarEvent{}, err
	}
	c.emit(EventCalendarChanged, nil)
	err = c.sendNotices(ctx, &ed, acc, notices, msgInviteSendFailed)
	if ed.created != nil {
		return *ed.created, err
	}
	return *ed.updated, err
}

// edited aplica sobre prev los campos que se cambian desde el editor; la
// identidad (UID, tipo, estado, secuencia) queda como estaba.
func edited(prev, e model.CalendarEvent) model.CalendarEvent {
	prev.Title, prev.Description, prev.Notes, prev.Location = e.Title, e.Description, e.Notes, e.Location
	prev.AllDay, prev.Start, prev.End, prev.RRule = e.AllDay, e.Start, e.End, e.RRule
	prev.RemindBefore, prev.AccountID, prev.Attendees = e.RemindBefore, e.AccountID, e.Attendees
	if e.TZID != "" {
		prev.TZID = e.TZID
	}
	return prev
}

// withException agrega la excepción o reemplaza la de la misma ocurrencia.
func withException(xs []model.EventException, x model.EventException) []model.EventException {
	out := make([]model.EventException, 0, len(xs)+1)
	for _, old := range xs {
		if !old.RecurrenceAt.Equal(x.RecurrenceAt) {
			out = append(out, old)
		}
	}
	return append(out, x)
}

// shiftExceptions corre las excepciones (ocurrencia original y horas) en d.
func shiftExceptions(xs []model.EventException, d time.Duration) []model.EventException {
	out := make([]model.EventException, 0, len(xs))
	for _, x := range xs {
		x.RecurrenceAt, x.Start, x.End = x.RecurrenceAt.Add(d), x.Start.Add(d), x.End.Add(d)
		out = append(out, x)
	}
	return out
}

// validExceptions se queda con las excepciones de ocurrencias que siguen
// existiendo en la serie (un cambio de regla puede dejarlas sueltas).
func validExceptions(e model.CalendarEvent, xs []model.EventException) []model.EventException {
	out := []model.EventException{}
	if e.RRule == "" {
		return out
	}
	for _, x := range xs {
		if isOccurrence(e, x.RecurrenceAt) {
			out = append(out, x)
		}
	}
	return out
}

// ruleParts separa la regla en sus partes, sin las claves indicadas, y
// devuelve aparte el valor de COUNT (0 si no tiene).
func ruleParts(rule string, drop ...string) (parts []string, count int) {
	for _, p := range strings.Split(rule, ";") {
		key, val, _ := strings.Cut(p, "=")
		key = strings.ToUpper(strings.TrimSpace(key))
		if key == "COUNT" {
			count, _ = strconv.Atoi(strings.TrimSpace(val))
		}
		dropped := p == ""
		for _, d := range drop {
			dropped = dropped || key == d
		}
		if !dropped {
			parts = append(parts, p)
		}
	}
	return parts, count
}

// cutSeries termina la serie justo antes de la ocurrencia at (UNTIL, sin
// COUNT) y le saca las excepciones desde at.
func cutSeries(e model.CalendarEvent, at time.Time) model.CalendarEvent {
	parts, _ := ruleParts(e.RRule, "COUNT", "UNTIL")
	until := at.Add(-time.Second).UTC().Format("20060102T150405Z")
	if e.AllDay {
		until = at.UTC().AddDate(0, 0, -1).Format("20060102")
	}
	e.RRule = strings.Join(append(parts, "UNTIL="+until), ";")
	kept := []model.EventException{}
	for _, x := range e.Exceptions {
		if x.RecurrenceAt.Before(at) {
			kept = append(kept, x)
		}
	}
	e.Exceptions = kept
	return e
}

// splitSeries arma la serie, con otro UID, que empieza en la ocurrencia at
// con los campos nuevos (la vieja se corta con cutSeries). Si la regla no
// cambió, la nueva sigue con la misma (con las repeticiones que le quedaban,
// si tenía COUNT).
func splitSeries(prev, e model.CalendarEvent, at time.Time) model.CalendarEvent {
	next := edited(prev, e)
	next.ID, next.UID, next.Status, next.Sequence = 0, uuid.NewString()+"@postacontrol", "confirmed", 0
	if e.RRule == prev.RRule {
		parts, count := ruleParts(prev.RRule, "COUNT")
		if count > 0 {
			parts = append(parts, fmt.Sprintf("COUNT=%d", count-countBefore(prev, at)))
		}
		next.RRule = strings.Join(parts, ";")
	}
	var moved []model.EventException
	for _, x := range prev.Exceptions {
		if x.RecurrenceAt.After(at) { // la de at la reemplaza lo que se editó
			moved = append(moved, x)
		}
	}
	next.Exceptions = validExceptions(next, shiftExceptions(moved, next.Start.Sub(at)))
	return next
}

// DeleteEvent borra un evento local entero, solo una ocurrencia ("this") o
// desde una ocurrencia en adelante ("following"). Con req.Notify les avisa a
// los invitados: la cancelación o la serie que queda.
func (c *Core) DeleteEvent(ctx context.Context, req model.EventDelete) error {
	c.calMu.Lock()
	defer c.calMu.Unlock()
	ed, err := c.planDelete(ctx, req)
	if err != nil {
		return err
	}
	acc, notices, err := c.prepareNotices(ctx, &ed, req.Notify)
	if err != nil {
		return err
	}
	if err := c.applyEdit(ctx, &ed); err != nil {
		return err
	}
	c.emit(EventCalendarChanged, nil)
	failed := msgInviteSendFailed
	if ed.updated == nil {
		failed = msgCancelSendFailed
	}
	return c.sendNotices(ctx, &ed, acc, notices, failed)
}

// localTZID es el nombre IANA de la zona local, o "" si no se sabe: en Linux
// sin TZ, time.Local se llama "Local" y hay que mirar TZ o /etc/localtime.
func localTZID() string {
	link, _ := os.Readlink("/etc/localtime")
	return localTZIDFrom(time.Local.String(), os.Getenv("TZ"), link)
}

// localTZIDFrom elige la zona entre el nombre de time.Local, el valor de TZ y
// el destino del enlace /etc/localtime, en ese orden; vale el primero que
// time.LoadLocation acepta.
func localTZIDFrom(local, env, link string) string {
	valid := func(name string) bool {
		if name == "" || name == "Local" {
			return false
		}
		_, err := time.LoadLocation(name)
		return err == nil
	}
	if valid(local) {
		return local
	}
	if env = strings.TrimPrefix(env, ":"); valid(env) {
		return env
	}
	if _, name, ok := strings.Cut(link, "zoneinfo/"); ok && valid(name) {
		return name
	}
	return ""
}

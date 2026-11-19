package mail

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"postacontrol/core/icalx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	msgInviteBadPartstat = "Esa respuesta no es válida."
	msgNotInvite         = "Este evento no es una invitación."
	msgInviteCancelled   = "Esta invitación fue cancelada."
	msgNotCancelled      = "Solo se quitan invitaciones canceladas."
	msgAliasReply        = "Ninguna de tus cuentas figura como invitada; respondimos desde %s."
	msgNoOrganizer       = "Esta invitación no dice a quién responderle."
	msgNoReplyAccount    = "Ninguna de tus cuentas puede responder esta invitación."
)

// Resultados de procesar un correo con invitación (invite_messages.outcome).
const (
	outcomeCreated   = "created"
	outcomeUpdated   = "updated"
	outcomeOutdated  = "outdated"  // REQUEST con una SEQUENCE que no es más nueva
	outcomeDuplicate = "duplicate" // misma UID, SEQUENCE, remitente y método que otro correo
	outcomePast      = "past"
	outcomeOwn       = "own" // lo organiza una de las cuentas
	outcomeCancelled = "cancelled"
	outcomeReply     = "reply"
	outcomeNoEvent   = "noevent" // CANCEL o REPLY de un evento que no tenemos
	// El .ics del correo no es una invitación (PUBLISH, ilegible): se anota
	// para no volver a bajarlo, pero no muestra tarjeta.
	outcomeNotInvite = "notinvite"
)

// ignoredInviteRoles son las carpetas cuyas invitaciones no se procesan.
var ignoredInviteRoles = map[model.FolderRole]bool{"sent": true, "drafts": true, "trash": true, "junk": true}

// noteInvite procesa la invitación (iTIP) que trae el correo d: crea, actualiza
// o cancela la invitación en el calendario, o anota la respuesta de un
// invitado a un evento propio. Nunca falla el abrir o indexar el correo: los
// errores van al log.
func (c *Core) noteInvite(ctx context.Context, d model.MessageDetail, ics []byte) {
	if err := c.processInvite(ctx, d, ics); err != nil {
		slog.Warn("invitaciones: no se pudo procesar la invitación del correo", "mensaje", d.ID, "err", err)
	}
}

func (c *Core) processInvite(ctx context.Context, d model.MessageDetail, ics []byte) error {
	sender := strings.ToLower(strings.TrimSpace(d.From.Email))
	cal, err := icalx.Parse(ics)
	method := cal.Method
	if err == nil && len(cal.Series) == 0 {
		err = errors.New("no trae eventos")
	}
	if err == nil && method != "REQUEST" && method != "CANCEL" && method != "REPLY" {
		err = fmt.Errorf("método %q", method)
	}
	if err != nil {
		slog.Debug("invitaciones: el calendario del correo no es una invitación", "mensaje", d.ID, "err", err)
		return c.store.SaveInviteRecord(ctx, store.InviteRecord{MessageID: d.ID, Method: method, Sender: sender,
			Outcome: outcomeNotInvite})
	}
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return err
	}
	if ignored, err := c.inviteFolderIgnored(ctx, d); err != nil || ignored {
		return err
	}
	// Lo que manda una de las cuentas es una copia de lo propio; salvo una
	// respuesta: la cuenta puede estar invitada a un evento de otra.
	if method != "REPLY" && accountByEmail(accs, d.From.Email) != nil {
		return nil
	}

	c.calMu.Lock()
	defer c.calMu.Unlock()
	if _, ok, err := c.store.InviteRecord(ctx, d.ID); err != nil || ok {
		return err
	}
	m := cal.Series[0].Master
	rec := store.InviteRecord{MessageID: d.ID, Method: method, UID: recordUID(cal.Series[0]), Sequence: m.Sequence,
		Sender: sender, ICS: string(cal.Raw)}
	// Un REQUEST repetido (el mismo correo en dos carpetas) lo descarta
	// applyRequest por SEQUENCE y DTSTAMP. Una respuesta nueva puede venir con
	// la misma SEQUENCE (el invitado cambió de idea): las respuestas solo se
	// cuidan por correo.
	seen := false
	if method == "CANCEL" {
		if seen, err = c.store.InviteSeen(ctx, rec.UID, rec.Sequence, rec.Sender, rec.Method); err != nil {
			return err
		}
	}
	changed := false
	switch {
	case seen:
		rec.Outcome = outcomeDuplicate
	case method == "REQUEST":
		rec.Outcome, changed, err = c.applyRequest(ctx, d, cal, accs)
	case method == "CANCEL":
		rec.Outcome, changed, err = c.applyCancel(ctx, cal.Series[0])
	default:
		rec.Outcome, changed, err = c.applyReply(ctx, m)
	}
	if err != nil {
		return err
	}
	if changed {
		c.emit(EventCalendarChanged, nil)
	}
	return c.store.SaveInviteRecord(ctx, rec)
}

// recordUID es la UID con la que se registra el correo: la de la serie o, si
// solo trae ocurrencias sueltas, "<uid>#<inicios originales unix, ordenados>",
// así dos correos de ocurrencias distintas con la misma SEQUENCE no se toman
// por repetidos.
func recordUID(s icalx.Series) string {
	if s.Master.RecurrenceID.IsZero() {
		return s.Master.UID
	}
	ids := make([]int64, 0, len(s.Overrides))
	for _, ov := range s.Overrides {
		ids = append(ids, ov.RecurrenceID.Unix())
	}
	slices.Sort(ids)
	parts := make([]string, 0, len(ids))
	for _, id := range slices.Compact(ids) {
		parts = append(parts, strconv.FormatInt(id, 10))
	}
	return s.Master.UID + "#" + strings.Join(parts, ",")
}

// inviteFolderIgnored dice si la invitación del correo no se procesa por la
// carpeta: Enviados, Borradores, Papelera o Spam.
func (c *Core) inviteFolderIgnored(ctx context.Context, d model.MessageDetail) (bool, error) {
	f, _, err := c.store.GetFolder(ctx, d.FolderID)
	if err != nil {
		return false, err
	}
	return ignoredInviteRoles[f.Role], nil
}

// accountByEmail es la cuenta con esa dirección (sin distinguir mayúsculas).
func accountByEmail(accs []model.Account, email string) *model.Account {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	for i := range accs {
		if strings.EqualFold(accs[i].Email, email) {
			return &accs[i]
		}
	}
	return nil
}

// applyRequest crea o actualiza la invitación con la serie del REQUEST.
func (c *Core) applyRequest(ctx context.Context, d model.MessageDetail, cal icalx.Calendar, accs []model.Account) (string, bool, error) {
	s := cal.Series[0]
	m := s.Master
	if accountByEmail(accs, m.Organizer.Email) != nil {
		return outcomeOwn, false, nil
	}
	prev, err := c.store.EventByUID(ctx, m.UID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return "", false, err
	}
	next := seriesEvent(s, cal.Raw)
	if subDailyRule(next.RRule) {
		slog.Info("invitaciones: se ignora una repetición de menos de un día", "uid", m.UID, "rrule", next.RRule)
		next.RRule, next.Exceptions = "", []model.EventException{}
	}
	if found := err == nil; !found {
		// Una invitación quitada del calendario no vuelve por una copia del
		// correo (otra carpeta): tiene que ser más nueva que lo ya visto.
		if outcome, err := c.seenInvite(ctx, s); err != nil || outcome != "" {
			return outcome, false, err
		}
		if eventPast(next, c.now()) {
			return outcomePast, false, nil
		}
		setMyAttendee(&next, m.Attendees, accs, d.AccountID, append(append([]model.Address(nil), d.To...), d.Cc...))
		_, err := c.store.SaveEvent(ctx, next, seriesEnd(next))
		return outcomeCreated, err == nil, err
	}
	if prev.Kind != "invite" {
		return outcomeOwn, false, nil
	}
	// Las ocurrencias sueltas valen con la misma SEQUENCE de la serie (cada
	// una es de otra ocurrencia); la serie entera tiene que ser más nueva o,
	// con la misma SEQUENCE, tener un DTSTAMP posterior (RFC 5546 §2.1.5).
	occurrences := !m.RecurrenceID.IsZero() && prev.RRule != ""
	if m.Sequence < prev.Sequence {
		return outcomeOutdated, false, nil
	}
	if m.Sequence == prev.Sequence && !occurrences {
		switch stamp := storedStamp(prev); {
		case m.DTStamp.Equal(stamp):
			return outcomeDuplicate, false, nil
		case !m.DTStamp.After(stamp):
			return outcomeOutdated, false, nil
		}
	}
	if occurrences {
		next = withOverrides(prev, s.Overrides)
		next.Sequence = max(prev.Sequence, m.Sequence)
		if sameInvite(prev, next) { // la misma ocurrencia otra vez
			return outcomeDuplicate, false, nil
		}
	} else {
		next.ID, next.Notes, next.RemindBefore = prev.ID, prev.Notes, prev.RemindBefore
		next.AccountID, next.MyEmail, next.AliasReply = prev.AccountID, prev.MyEmail, prev.AliasReply
		next.MyPartstat, next.NeedsReview = prev.MyPartstat, prev.NeedsReview
		if !next.Start.Equal(prev.Start) || !next.End.Equal(prev.End) || next.AllDay != prev.AllDay || next.RRule != prev.RRule {
			next.NeedsReview = true
			if next.MyPartstat != "DECLINED" {
				next.MyPartstat = "NEEDS-ACTION"
			}
		}
	}
	_, err = c.store.SaveEvent(ctx, next, seriesEnd(next))
	return outcomeUpdated, err == nil, err
}

// seenInvite compara el REQUEST de la serie s con los correos ya procesados
// de esa UID (REQUEST y CANCEL): "duplicate" si es copia de un REQUEST visto
// (misma SEQUENCE y DTSTAMP); "outdated" si no es más nuevo que el último
// visto (SEQUENCE mayor o, con la misma, DTSTAMP posterior); "" si lo es (o
// no se vio nada).
func (c *Core) seenInvite(ctx context.Context, s icalx.Series) (string, error) {
	recs, err := c.store.InviteRecordsByUID(ctx, recordUID(s))
	if err != nil || len(recs) == 0 {
		return "", err
	}
	m := s.Master
	maxSeq, stamp, copied := -1, time.Time{}, false
	for _, r := range recs {
		var st time.Time
		if cal, err := icalx.Parse([]byte(r.ICS)); err == nil && len(cal.Series) > 0 {
			st = cal.Series[0].Master.DTStamp
		}
		copied = copied || (r.Method == "REQUEST" && r.Sequence == m.Sequence && st.Equal(m.DTStamp))
		if r.Sequence > maxSeq || (r.Sequence == maxSeq && st.After(stamp)) {
			maxSeq, stamp = r.Sequence, st
		}
	}
	switch {
	case copied:
		return outcomeDuplicate, nil
	case m.Sequence > maxSeq || (m.Sequence == maxSeq && m.DTStamp.After(stamp)):
		return "", nil
	}
	return outcomeOutdated, nil
}

// storedStamp es el DTSTAMP del maestro del iCalendar guardado (cero si no hay).
func storedStamp(e model.CalendarEvent) time.Time {
	cal, err := icalx.Parse([]byte(e.RawICS))
	if err != nil || len(cal.Series) == 0 {
		return time.Time{}
	}
	return cal.Series[0].Master.DTStamp
}

// sameInvite dice si sumar ocurrencias no cambió nada de la invitación.
func sameInvite(a, b model.CalendarEvent) bool {
	return a.Sequence == b.Sequence && a.NeedsReview == b.NeedsReview && a.MyPartstat == b.MyPartstat &&
		slices.EqualFunc(a.Exceptions, b.Exceptions, func(x, y model.EventException) bool {
			return x.RecurrenceAt.Equal(y.RecurrenceAt) && x.Cancelled == y.Cancelled && x.Start.Equal(y.Start) &&
				x.End.Equal(y.End) && x.Title == y.Title && x.Location == y.Location && x.Description == y.Description
		})
}

// subDailyRule dice si la regla se repite más de una vez por día
// (FREQ=SECONDLY, MINUTELY u HOURLY).
func subDailyRule(rule string) bool {
	for _, part := range strings.Split(rule, ";") {
		k, v, _ := strings.Cut(part, "=")
		if strings.EqualFold(strings.TrimSpace(k), "FREQ") {
			switch strings.ToUpper(strings.TrimSpace(v)) {
			case "SECONDLY", "MINUTELY", "HOURLY":
				return true
			}
		}
	}
	return false
}

// withOverrides suma a la serie las ocurrencias cambiadas. Si alguna cambió
// de hora, la invitación queda para revisar (y sin respuesta, salvo que la
// hubiera rechazado).
func withOverrides(e model.CalendarEvent, ovs []icalx.VEvent) model.CalendarEvent {
	dur := e.End.Sub(e.Start)
	cur := map[int64]model.EventException{}
	for _, x := range e.Exceptions {
		cur[x.RecurrenceAt.Unix()] = x
	}
	moved := false
	for _, ov := range ovs {
		x := overrideException(ov)
		start, end := x.RecurrenceAt, x.RecurrenceAt.Add(dur)
		if old, ok := cur[x.RecurrenceAt.Unix()]; ok && !old.Cancelled {
			start, end = old.Start, old.End
		}
		moved = moved || (!x.Cancelled && (!x.Start.Equal(start) || !x.End.Equal(end)))
		e.Exceptions = withException(e.Exceptions, x)
	}
	e.Exceptions = validExceptions(e, e.Exceptions)
	if moved {
		e.NeedsReview = true
		if e.MyPartstat != "DECLINED" {
			e.MyPartstat = "NEEDS-ACTION"
		}
	}
	return e
}

// setMyAttendee elige con qué cuenta se responde: la del primer ATTENDEE que
// coincide con una cuenta; si ninguno, la que recibió el correo, con la
// dirección del ATTENDEE que figura entre los destinatarios del correo
// (recipients: To y Cc) o, si tampoco, con la de la cuenta (AliasReply).
func setMyAttendee(e *model.CalendarEvent, atts []icalx.Attendee, accs []model.Account, receiver int64, recipients []model.Address) {
	partstat := func(a icalx.Attendee) string {
		if a.Partstat == "" {
			return "NEEDS-ACTION"
		}
		return a.Partstat
	}
	for _, a := range atts {
		if acc := accountByEmail(accs, a.Email); acc != nil {
			e.AccountID, e.MyEmail, e.MyPartstat = acc.ID, acc.Email, partstat(a)
			return
		}
	}
	for _, a := range atts {
		for _, r := range recipients {
			if a.Email != "" && strings.EqualFold(strings.TrimSpace(r.Email), a.Email) {
				e.AccountID, e.MyEmail, e.MyPartstat = receiver, a.Email, partstat(a)
				return
			}
		}
	}
	e.AccountID, e.AliasReply, e.MyPartstat = receiver, true, "NEEDS-ACTION"
	for _, acc := range accs {
		if acc.ID == receiver {
			e.MyEmail = acc.Email
		}
	}
}

// seriesEvent traduce la serie de una invitación a un evento "invite" (sin
// las respuestas propias): las ocurrencias cambiadas y las canceladas (EXDATE
// o STATUS:CANCELLED) van como excepciones.
func seriesEvent(s icalx.Series, raw []byte) model.CalendarEvent {
	m := s.Master
	e := model.CalendarEvent{UID: m.UID, Kind: "invite", Title: m.Summary, Description: m.Description,
		Location: m.Location, AllDay: m.AllDay, Start: m.Start.UTC(), End: m.End.UTC(), TZID: m.TZID, RRule: m.RRule,
		Organizer: m.Organizer, Sequence: m.Sequence, Status: "confirmed", RawICS: string(raw),
		Attendees: []model.Attendee{}, Exceptions: []model.EventException{}}
	if m.Status == "CANCELLED" {
		e.Status = "cancelled"
	}
	for _, a := range m.Attendees {
		p := a.Partstat
		if p == "" {
			p = "NEEDS-ACTION"
		}
		e.Attendees = append(e.Attendees, model.Attendee{Email: a.Email, Name: a.Name, Partstat: p})
	}
	if !m.RecurrenceID.IsZero() || m.RRule == "" {
		return e
	}
	dur := e.End.Sub(e.Start)
	xs := []model.EventException{}
	for _, x := range m.ExDates {
		x = x.UTC()
		xs = append(xs, model.EventException{RecurrenceAt: x, Cancelled: true, Start: x, End: x.Add(dur)})
	}
	for _, ov := range s.Overrides {
		xs = withException(xs, overrideException(ov))
	}
	e.Exceptions = validExceptions(e, xs)
	return e
}

// overrideException traduce una ocurrencia cambiada (con RECURRENCE-ID).
func overrideException(ov icalx.VEvent) model.EventException {
	x := model.EventException{RecurrenceAt: ov.RecurrenceID.UTC(), Start: ov.Start.UTC(), End: ov.End.UTC(),
		Title: ov.Summary, Location: ov.Location, Description: ov.Description}
	x.Cancelled = ov.Status == "CANCELLED"
	return x
}

// eventPast dice si el evento (o la serie entera) ya terminó; una serie sin
// fin nunca terminó. Uno de día completo termina al final de su último día en
// la zona de now (la local: c.now es time.Now).
func eventPast(e model.CalendarEvent, now time.Time) bool {
	end := seriesEnd(e)
	if end == nil {
		return false
	}
	last := *end
	if e.AllDay {
		y, m, d := last.UTC().Date()
		last = time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	}
	return !last.After(now)
}

// applyCancel cancela la invitación entera o, con RECURRENCE-ID, esas ocurrencias.
func (c *Core) applyCancel(ctx context.Context, s icalx.Series) (string, bool, error) {
	m := s.Master
	prev, err := c.store.EventByUID(ctx, m.UID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && prev.Kind != "invite") {
		return outcomeNoEvent, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if m.Sequence < prev.Sequence { // una cancelación vieja, de antes de un cambio
		return outcomeOutdated, false, nil
	}
	next := prev
	if m.RecurrenceID.IsZero() {
		next.Status = "cancelled"
	} else {
		dur := prev.End.Sub(prev.Start)
		for _, ov := range s.Overrides {
			at := ov.RecurrenceID.UTC()
			next.Exceptions = withException(next.Exceptions, model.EventException{RecurrenceAt: at, Cancelled: true, Start: at, End: at.Add(dur)})
		}
	}
	next.Sequence = max(next.Sequence, m.Sequence)
	_, err = c.store.SaveEvent(ctx, next, seriesEnd(next))
	return outcomeCancelled, err == nil, err
}

// applyReply anota la respuesta de un invitado a un evento propio (local o de
// Taskapp), solo si ya estaba invitado.
func (c *Core) applyReply(ctx context.Context, m icalx.VEvent) (string, bool, error) {
	prev, err := c.store.EventByUID(ctx, m.UID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && prev.Kind != "local" && prev.Kind != "taskapp") {
		return outcomeNoEvent, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if len(m.Attendees) == 0 || m.Attendees[0].Partstat == "" {
		return outcomeReply, false, nil
	}
	a := m.Attendees[0]
	for _, old := range prev.Attendees {
		if strings.EqualFold(old.Email, a.Email) && old.Partstat != a.Partstat {
			ok, err := c.store.SetAttendeePartstat(ctx, prev.ID, a.Email, a.Partstat)
			return outcomeReply, ok, err
		}
	}
	return outcomeReply, false, nil
}

// InviteForMessage arma la tarjeta de invitación del correo. Un correo sin
// registro que tiene un .ics adjunto de hasta 1 MB (los cacheados antes de
// que se miraran las invitaciones) se procesa ahora. Un .ics que no es una
// invitación no muestra tarjeta.
func (c *Core) InviteForMessage(ctx context.Context, messageID int64) (model.MessageInvite, error) {
	rec, ok, err := c.store.InviteRecord(ctx, messageID)
	if err != nil {
		return model.MessageInvite{}, c.ctxOrUserError(ctx, err)
	}
	if !ok {
		noted, err := c.noteLegacyInvite(ctx, messageID)
		if err != nil || !noted {
			return model.MessageInvite{}, err
		}
		if rec, ok, err = c.store.InviteRecord(ctx, messageID); err != nil {
			return model.MessageInvite{}, c.ctxOrUserError(ctx, err)
		}
		if !ok {
			return model.MessageInvite{}, nil
		}
	}
	if rec.Outcome == outcomeNotInvite {
		return model.MessageInvite{}, nil
	}
	return c.inviteView(ctx, rec)
}

// noteLegacyInvite baja el .ics adjunto del correo y lo procesa. false si no
// hay nada que procesar.
func (c *Core) noteLegacyInvite(ctx context.Context, messageID int64) (bool, error) {
	d, _, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return false, c.ctxOrUserError(ctx, err)
	}
	att, ok := calendarAttachment(d.Attachments)
	if !ok {
		return false, nil
	}
	ignored, err := c.inviteFolderIgnored(ctx, d)
	if err != nil {
		return false, c.ctxOrUserError(ctx, err)
	}
	if ignored {
		return false, nil
	}
	_, data, err := c.attachmentBytes(ctx, messageID, att.Index)
	if err != nil {
		return false, err
	}
	c.noteInvite(ctx, d, data)
	return true, nil
}

// inviteView arma la tarjeta con el registro y, si sigue en el calendario, el
// evento (salvo un correo de una sola ocurrencia, que muestra la suya).
func (c *Core) inviteView(ctx context.Context, rec store.InviteRecord) (model.MessageInvite, error) {
	v := model.MessageInvite{Found: true, Method: rec.Method}
	cal, err := icalx.Parse([]byte(rec.ICS))
	if err != nil || len(cal.Series) == 0 {
		return v, nil
	}
	m := cal.Series[0].Master
	shown := seriesEvent(cal.Series[0], cal.Raw)
	v.Cancelled = rec.Method == "CANCEL" || shown.Status == "cancelled"
	ev, err := c.store.EventByUID(ctx, m.UID)
	switch {
	case err == nil:
		v.EventID = ev.ID
		if m.RecurrenceID.IsZero() {
			shown = ev
		}
		v.Cancelled = v.Cancelled || ev.Status == "cancelled"
		if ev.Kind == "invite" {
			v.Partstat, v.NeedsReview, v.AliasReply = ev.MyPartstat, ev.NeedsReview, ev.AliasReply
			v.Outdated = rec.Method == "REQUEST" && ev.Sequence > rec.Sequence
		}
	case !errors.Is(err, store.ErrNotFound):
		return model.MessageInvite{}, c.ctxOrUserError(ctx, err)
	}
	v.Title, v.Location, v.Organizer = shown.Title, shown.Location, shown.Organizer
	v.Start, v.End, v.AllDay, v.Recurring = shown.Start, shown.End, shown.AllDay, shown.RRule != ""
	v.Past = eventPast(shown, c.now())
	if rec.Method == "REPLY" && len(m.Attendees) > 0 {
		a := m.Attendees[0]
		v.ReplyFrom, v.ReplyStatus = model.Address{Name: a.Name, Email: a.Email}, a.Partstat
	}
	return v, nil
}

// RespondInvite responde la invitación (ACCEPTED, TENTATIVE o DECLINED): le
// manda el REPLY al organizador por la bandeja de salida, desde la cuenta
// invitada, y anota la respuesta.
func (c *Core) RespondInvite(ctx context.Context, eventID int64, partstat string) (model.RespondResult, error) {
	partstat = strings.ToUpper(strings.TrimSpace(partstat))
	if _, ok := replyVerbs[partstat]; !ok {
		return model.RespondResult{}, &UserError{Code: "invalid", Msg: msgInviteBadPartstat}
	}
	c.calMu.Lock()
	defer c.calMu.Unlock()
	e, err := c.store.GetEvent(ctx, eventID)
	if err != nil {
		return model.RespondResult{}, c.ctxOrUserError(ctx, err)
	}
	switch {
	case e.Kind != "invite":
		return model.RespondResult{}, &UserError{Code: "invalid", Msg: msgNotInvite}
	case e.Status == "cancelled":
		return model.RespondResult{}, &UserError{Code: "invalid", Msg: msgInviteCancelled}
	case e.Organizer.Email == "":
		return model.RespondResult{}, &UserError{Code: "invalid", Msg: msgNoOrganizer}
	}
	acc, err := c.replyAccount(ctx, e)
	if err != nil {
		return model.RespondResult{}, err
	}
	me := e.MyEmail
	if me == "" {
		me = acc.Email
	}
	orig, err := icalx.Parse([]byte(e.RawICS))
	if err != nil {
		return model.RespondResult{}, &UserError{Code: "invalid", Msg: msgNotInvite, Err: err}
	}
	ics, err := icalx.BuildReply(orig, e.UID, me, partstat, e.Sequence, c.now())
	if err != nil {
		return model.RespondResult{}, &UserError{Code: "invalid", Msg: msgNotInvite, Err: err}
	}
	name := acc.DisplayName
	if name == "" {
		name = acc.Email
	}
	verb := replyVerbs[partstat]
	body := "<p>" + html.EscapeString(name+" "+verb.body+" «"+e.Title+"».") + "</p>"
	if _, err := c.QueueSend(ctx, model.SendRequest{AccountID: acc.ID, To: []model.Address{e.Organizer},
		Subject: verb.subject + e.Title, BodyHTML: body,
		Calendar: &model.CalendarPart{Method: "REPLY", ICS: string(ics)}}); err != nil {
		return model.RespondResult{}, c.ctxOrUserError(ctx, err)
	}

	e.MyPartstat, e.NeedsReview = partstat, false
	for i, a := range e.Attendees {
		if strings.EqualFold(a.Email, me) {
			e.Attendees[i].Partstat = partstat
		}
	}
	if _, err := c.saveEvent(ctx, e); err != nil {
		return model.RespondResult{}, err
	}
	c.emit(EventCalendarChanged, nil)
	res := model.RespondResult{}
	if e.AliasReply {
		res.Warning = fmt.Sprintf(msgAliasReply, acc.Email)
	}
	return res, nil
}

// replyVerbs es, por respuesta, el prefijo del asunto y lo que dice el cuerpo.
var replyVerbs = map[string]struct{ subject, body string }{
	"ACCEPTED":  {"Aceptado: ", "aceptó la invitación a"},
	"TENTATIVE": {"Tal vez: ", "respondió que tal vez vaya a"},
	"DECLINED":  {"Rechazado: ", "rechazó la invitación a"},
}

// replyAccount es la cuenta que responde: la de MyEmail o, si ya no está, la
// del evento.
func (c *Core) replyAccount(ctx context.Context, e model.CalendarEvent) (model.Account, error) {
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return model.Account{}, c.ctxOrUserError(ctx, err)
	}
	if acc := accountByEmail(accs, e.MyEmail); acc != nil {
		return *acc, nil
	}
	for _, acc := range accs {
		if acc.ID == e.AccountID && e.AccountID != 0 {
			return acc, nil
		}
	}
	return model.Account{}, &UserError{Code: "invalid", Msg: msgNoReplyAccount}
}

// RemoveCancelledEvent saca del calendario una invitación cancelada. Los
// registros de los correos quedan.
func (c *Core) RemoveCancelledEvent(ctx context.Context, eventID int64) error {
	c.calMu.Lock()
	defer c.calMu.Unlock()
	e, err := c.store.GetEvent(ctx, eventID)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	if e.Kind != "invite" {
		return &UserError{Code: "invalid", Msg: msgNotInvite}
	}
	if e.Status != "cancelled" {
		return &UserError{Code: "invalid", Msg: msgNotCancelled}
	}
	if err := c.store.DeleteEvent(ctx, e.ID); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventCalendarChanged, nil)
	return nil
}

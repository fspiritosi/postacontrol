package icalx

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/emersion/go-ical"
)

// ProdID identifica a PostaControl en los iCalendar que arma.
const ProdID = "-//PostaControl//PostaControl//ES"

// Build arma un iCalendar con el METHOD dado (REQUEST, CANCEL o PUBLISH).
// Las horas van en UTC (sin VTIMEZONE) y los días completos como VALUE=DATE,
// salvo en una serie con repetición y zona IANA: ahí DTSTART, DTEND, EXDATE y
// RECURRENCE-ID van en hora local de esa zona con su VTIMEZONE, para que quien
// la recibe expanda BYDAY/BYMONTHDAY en el mismo día local.
func Build(method string, s Series, now time.Time) ([]byte, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	switch method {
	case "REQUEST", "CANCEL", "PUBLISH":
	default:
		return nil, fmt.Errorf("icalx: método desconocido %q", method)
	}
	if s.Master.UID == "" {
		return nil, errors.New("icalx: la serie no tiene UID")
	}

	cal := newCalendar(method)
	loc := seriesZone(s.Master)
	if loc != nil {
		cal.Children = append(cal.Children, vtimezone(loc, s.Master.Start))
	}
	cal.Children = append(cal.Children, buildEvent(s.Master, method, now, loc))
	for _, ov := range s.Overrides {
		ov.UID = s.Master.UID
		cal.Children = append(cal.Children, buildEvent(ov, method, now, loc))
	}
	return encode(cal)
}

// seriesZone es la zona en la que se escriben las horas de la serie: la del
// maestro si se repite, tiene hora y su TZID carga; si no, nil (UTC).
func seriesZone(m VEvent) *time.Location {
	if m.RRule == "" || m.AllDay || m.TZID == "" || m.TZID == "UTC" {
		return nil
	}
	loc, err := time.LoadLocation(m.TZID)
	if err != nil {
		return nil
	}
	return loc
}

func newCalendar(method string) *ical.Calendar {
	cal := ical.NewCalendar()
	cal.Props.SetText("PRODID", ProdID)
	cal.Props.SetText("VERSION", "2.0")
	cal.Props.SetText("CALSCALE", "GREGORIAN")
	cal.Props.SetText("METHOD", method)
	return cal
}

func encode(cal *ical.Calendar) ([]byte, error) {
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		return nil, fmt.Errorf("icalx: no se pudo armar el iCalendar: %w", err)
	}
	return fold(buf.Bytes()), nil
}

// buildEvent arma un VEVENT; con loc, las horas van en hora local de esa zona.
func buildEvent(ev VEvent, method string, now time.Time, loc *time.Location) *ical.Component {
	c := ical.NewComponent("VEVENT")
	c.Props.SetText("UID", ev.UID)
	c.Props.SetDateTime("DTSTAMP", now.UTC())
	setTime(c, "DTSTART", ev.Start, ev.AllDay, loc)
	if !ev.End.IsZero() {
		setTime(c, "DTEND", ev.End, ev.AllDay, loc)
	}
	if !ev.RecurrenceID.IsZero() {
		setTime(c, "RECURRENCE-ID", ev.RecurrenceID, ev.AllDay, loc)
	}
	seq := ical.NewProp("SEQUENCE") // entero: sin VALUE=TEXT
	seq.Value = strconv.Itoa(ev.Sequence)
	c.Props.Set(seq)
	status := ev.Status
	if method == "CANCEL" {
		status = "CANCELLED"
	}
	if status != "" {
		c.Props.SetText("STATUS", status)
	}
	if ev.Summary != "" {
		c.Props.SetText("SUMMARY", cleanText(ev.Summary))
	}
	if ev.Description != "" {
		c.Props.SetText("DESCRIPTION", cleanText(ev.Description))
	}
	if ev.Location != "" {
		c.Props.SetText("LOCATION", cleanText(ev.Location))
	}
	if ev.RRule != "" {
		p := ical.NewProp("RRULE")
		p.Value = strings.TrimPrefix(ev.RRule, "RRULE:")
		c.Props.Set(p)
	}
	if len(ev.ExDates) > 0 {
		p := ical.NewProp("EXDATE")
		vals := make([]string, len(ev.ExDates))
		for i, t := range ev.ExDates {
			switch {
			case ev.AllDay:
				vals[i] = t.UTC().Format("20060102")
			case loc != nil:
				vals[i] = t.In(loc).Format("20060102T150405")
			default:
				vals[i] = t.UTC().Format("20060102T150405Z")
			}
		}
		switch {
		case ev.AllDay:
			p.Params.Set("VALUE", "DATE")
		case loc != nil:
			p.Params.Set("TZID", loc.String())
		}
		p.Value = strings.Join(vals, ",")
		c.Props.Set(p)
	}
	if ev.Organizer.Email != "" {
		p := ical.NewProp("ORGANIZER")
		if cn := cleanParam(ev.Organizer.Name); cn != "" {
			p.Params.Set("CN", cn)
		}
		p.Value = "mailto:" + ev.Organizer.Email
		c.Props.Set(p)
	}
	for _, a := range ev.Attendees {
		if a.Email == "" {
			continue
		}
		c.Props.Add(attendeeProp(a.Email, a.Name, a.Partstat))
	}
	return c
}

func attendeeProp(email, name, partstat string) *ical.Prop {
	p := ical.NewProp("ATTENDEE")
	if cn := cleanParam(name); cn != "" {
		p.Params.Set("CN", cn)
	}
	p.Params.Set("ROLE", "REQ-PARTICIPANT")
	if partstat == "" {
		partstat = "NEEDS-ACTION"
	}
	p.Params.Set("PARTSTAT", partstat)
	p.Params.Set("RSVP", "TRUE")
	p.Value = "mailto:" + email
	return p
}

// cleanText unifica los saltos de línea a "\n": el encoder rechaza CR y LF crudos.
func cleanText(v string) string {
	return strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(v)
}

// cleanParam saca lo que no puede ir en un parámetro (comillas y saltos).
func cleanParam(v string) string {
	v = strings.NewReplacer("\"", "", "\r", " ", "\n", " ").Replace(v)
	return strings.TrimSpace(v)
}

func setTime(c *ical.Component, name string, t time.Time, allDay bool, loc *time.Location) {
	p := ical.NewProp(name)
	switch {
	case allDay:
		p.SetDate(t.UTC())
	case loc != nil:
		p.Params.Set("TZID", loc.String())
		p.Value = t.In(loc).Format("20060102T150405")
	default:
		p.SetDateTime(t.UTC())
	}
	c.Props.Set(p)
}

// BuildReply arma la respuesta (METHOD:REPLY) de attendee a la invitación orig
// para el evento uid. Copia horas y VTIMEZONE del original y deja solo al
// invitado que responde.
func BuildReply(orig Calendar, uid, attendee, partstat string, now time.Time) ([]byte, error) {
	partstat = strings.ToUpper(strings.TrimSpace(partstat))
	switch partstat {
	case "ACCEPTED", "TENTATIVE", "DECLINED":
	default:
		return nil, fmt.Errorf("icalx: respuesta inválida %q", partstat)
	}
	email := cleanEmail(attendee)
	if email == "" {
		return nil, errors.New("icalx: falta el invitado que responde")
	}
	src, err := ical.NewDecoder(bytes.NewReader(orig.Raw)).Decode()
	if err != nil {
		return nil, fmt.Errorf("icalx: iCalendar original inválido: %w", err)
	}

	var master *ical.Component
	var zones []*ical.Component
	for _, c := range src.Children {
		switch c.Name {
		case "VTIMEZONE":
			zones = append(zones, c)
		case "VEVENT":
			if master == nil && textProp(c, "UID") == uid && c.Props.Get("RECURRENCE-ID") == nil {
				master = c
			}
		}
	}
	if master == nil {
		return nil, fmt.Errorf("icalx: el evento %q no está en el original", uid)
	}

	ev := ical.NewComponent("VEVENT")
	for name, props := range master.Props {
		switch name {
		case "ATTENDEE", "DESCRIPTION", "DTSTAMP":
			continue
		}
		ev.Props[name] = append([]ical.Prop(nil), props...)
	}
	ev.Props.SetDateTime("DTSTAMP", now.UTC())
	var cn string
	for _, p := range master.Props.Values("ATTENDEE") {
		if cleanEmail(p.Value) == email {
			cn = p.Params.Get("CN")
			break
		}
	}
	ev.Props.Add(attendeeProp(email, cn, partstat))

	cal := newCalendar("REPLY")
	cal.Children = append(cal.Children, zones...)
	cal.Children = append(cal.Children, ev)
	return encode(cal)
}

// fold parte las líneas de más de 75 octetos (RFC 5545 §3.1) sin cortar un
// carácter UTF-8 y garantiza CRLF en todas.
func fold(b []byte) []byte {
	var out bytes.Buffer
	for _, line := range strings.SplitAfter(string(b), "\n") {
		if line == "" {
			continue
		}
		line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		limit := 75
		for len(line) > limit {
			cut := limit
			for cut > 0 && !utf8.RuneStart(line[cut]) {
				cut--
			}
			out.WriteString(line[:cut])
			out.WriteString("\r\n ")
			line = line[cut:]
			limit = 74 // el espacio de continuación ocupa un octeto
		}
		out.WriteString(line)
		out.WriteString("\r\n")
	}
	return out.Bytes()
}

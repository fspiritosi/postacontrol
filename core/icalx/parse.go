// Package icalx lee iCalendar (invitaciones de Google/Outlook y la
// exportación de Google Calendar) y lo traduce a tipos propios.
package icalx

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/emersion/go-ical"

	"postacontrol/core/model"
)

// Attendee es un invitado. Email va en minúscula y sin "mailto:".
type Attendee struct{ Email, Name, Partstat string }

// VEvent es un evento ya normalizado.
type VEvent struct {
	UID                            string
	Sequence                       int
	Status                         string // "CONFIRMED", "TENTATIVE" o "CANCELLED"; "" = CONFIRMED
	Summary, Description, Location string
	Start, End                     time.Time // con hora: en UTC; día completo: medianoche UTC, fin exclusivo
	AllDay                         bool
	TZID                           string // IANA de la zona del original; "" si era UTC o flotante
	RRule                          string // sin "RRULE:"; "" = no se repite
	ExDates                        []time.Time
	RecurrenceID                   time.Time // cero en el maestro
	Organizer                      model.Address
	Attendees                      []Attendee
}

// Series agrupa un evento maestro con sus excepciones (por RECURRENCE-ID).
type Series struct {
	Master    VEvent
	Overrides []VEvent
}

// Calendar es el archivo iCalendar completo.
type Calendar struct {
	Method string   // "REQUEST", "REPLY", "CANCEL", "PUBLISH" o ""
	Name   string   // X-WR-CALNAME
	Series []Series // agrupado por UID, en el orden del archivo
	Raw    []byte   // el archivo tal cual
	// Skipped lista los VEVENT ilegibles que se salteó: "<UID o #n>: <motivo>".
	Skipped []string
}

// Parse lee un iCalendar; las horas flotantes se toman en time.Local.
func Parse(data []byte) (Calendar, error) { return ParseInLocation(data, time.Local) }

// ParseInLocation lee un iCalendar usando floating para las horas sin zona.
func ParseInLocation(data []byte, floating *time.Location) (Calendar, error) {
	if floating == nil {
		floating = time.Local
	}
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		return Calendar{}, fmt.Errorf("icalx: iCalendar inválido: %w", err)
	}
	out := Calendar{Raw: data}
	if p := cal.Props.Get("METHOD"); p != nil {
		out.Method = strings.ToUpper(strings.TrimSpace(p.Value))
	}
	if p := cal.Props.Get("X-WR-CALNAME"); p != nil {
		out.Name = textOf(p)
	}

	vtz := map[string]*ical.Component{}
	for _, c := range cal.Children {
		if c.Name == "VTIMEZONE" {
			if p := c.Props.Get("TZID"); p != nil {
				vtz[p.Value] = c
			}
		}
	}

	index := map[string]int{}
	nEvents := 0
	for _, c := range cal.Children {
		if c.Name != "VEVENT" {
			continue
		}
		nEvents++
		ev, err := parseEvent(c, vtz, floating)
		if err != nil {
			id := textProp(c, "UID")
			if id == "" {
				id = "#" + strconv.Itoa(nEvents)
			}
			out.Skipped = append(out.Skipped, id+": "+err.Error())
			continue
		}
		i, ok := index[ev.UID]
		if !ok {
			i = len(out.Series)
			index[ev.UID] = i
			out.Series = append(out.Series, Series{})
			out.Series[i].Master = VEvent{}
		}
		s := &out.Series[i]
		switch {
		case !ev.RecurrenceID.IsZero():
			s.Overrides = append(s.Overrides, ev)
		case s.Master.UID == "":
			s.Master = ev
		}
	}
	if nEvents > 0 && len(out.Series) == 0 {
		return Calendar{}, fmt.Errorf("icalx: no se pudo leer ningún evento (%s)", out.Skipped[0])
	}
	// Serie con solo excepciones: la primera hace de maestro.
	for i := range out.Series {
		s := &out.Series[i]
		if s.Master.UID == "" && len(s.Overrides) > 0 {
			s.Master = s.Overrides[0]
		}
	}
	return out, nil
}

func parseEvent(c *ical.Component, vtz map[string]*ical.Component, floating *time.Location) (VEvent, error) {
	var ev VEvent
	ev.UID = textProp(c, "UID")
	if ev.UID == "" {
		return ev, errors.New("falta el UID")
	}
	ev.Sequence, _ = strconv.Atoi(strings.TrimSpace(textProp(c, "SEQUENCE")))
	ev.Status = strings.ToUpper(strings.TrimSpace(textProp(c, "STATUS")))
	ev.Summary = textProp(c, "SUMMARY")
	ev.Description = textProp(c, "DESCRIPTION")
	ev.Location = textProp(c, "LOCATION")

	startProp := c.Props.Get("DTSTART")
	if startProp == nil {
		return ev, errors.New("falta DTSTART")
	}
	start, allDay, tzid, err := parseTime(startProp, vtz, floating)
	if err != nil {
		return ev, err
	}
	ev.Start, ev.AllDay, ev.TZID = start, allDay, tzid

	switch {
	case c.Props.Get("DTEND") != nil:
		end, _, _, err := parseTime(c.Props.Get("DTEND"), vtz, floating)
		if err != nil {
			return ev, err
		}
		ev.End = end
	case c.Props.Get("DURATION") != nil:
		d, err := parseDuration(c.Props.Get("DURATION").Value)
		if err != nil {
			return ev, err
		}
		ev.End = ev.Start.Add(d)
	case allDay:
		ev.End = ev.Start.AddDate(0, 0, 1)
	default:
		ev.End = ev.Start
	}

	if p := c.Props.Get("RRULE"); p != nil {
		ev.RRule = strings.TrimSpace(strings.TrimPrefix(p.Value, "RRULE:"))
	}
	for _, p := range c.Props.Values("EXDATE") {
		for _, v := range strings.Split(p.Value, ",") {
			q := p
			q.Value = strings.TrimSpace(v)
			if q.Value == "" {
				continue
			}
			t, _, _, err := parseTime(&q, vtz, floating)
			if err != nil {
				return ev, err
			}
			ev.ExDates = append(ev.ExDates, t)
		}
	}
	if p := c.Props.Get("RECURRENCE-ID"); p != nil {
		t, _, _, err := parseTime(p, vtz, floating)
		if err != nil {
			return ev, err
		}
		ev.RecurrenceID = t
	}

	if p := c.Props.Get("ORGANIZER"); p != nil {
		ev.Organizer = model.Address{Name: p.Params.Get("CN"), Email: cleanEmail(p.Value)}
	}
	for _, p := range c.Props.Values("ATTENDEE") {
		a := Attendee{
			Email:    cleanEmail(p.Value),
			Name:     p.Params.Get("CN"),
			Partstat: strings.ToUpper(p.Params.Get("PARTSTAT")),
		}
		ev.Attendees = append(ev.Attendees, a)
	}
	return ev, nil
}

func textOf(p *ical.Prop) string {
	if t, err := p.Text(); err == nil {
		return t
	}
	return p.Value
}

func textProp(c *ical.Component, name string) string {
	if p := c.Props.Get(name); p != nil {
		return textOf(p)
	}
	return ""
}

func cleanEmail(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 7 && strings.EqualFold(v[:7], "mailto:") {
		v = v[7:]
	}
	return strings.ToLower(strings.TrimSpace(v))
}

// parseTime lee un valor DATE o DATE-TIME. La zona sale del parámetro TZID.
func parseTime(p *ical.Prop, vtz map[string]*ical.Component, floating *time.Location) (t time.Time, allDay bool, tzid string, err error) {
	v := strings.TrimSpace(p.Value)
	if strings.EqualFold(p.Params.Get("VALUE"), "DATE") || len(v) == 8 {
		t, err = time.ParseInLocation("20060102", v, time.UTC)
		if err != nil {
			return t, false, "", fmt.Errorf("icalx: fecha inválida %q", v)
		}
		return t, true, "", nil
	}
	if strings.HasSuffix(v, "Z") {
		t, err = time.ParseInLocation("20060102T150405Z", v, time.UTC)
		if err != nil {
			return t, false, "", fmt.Errorf("icalx: fecha y hora inválida %q", v)
		}
		return t, false, "", nil
	}
	loc := floating
	if name := p.Params.Get("TZID"); name != "" {
		loc = resolveTZ(name, vtz)
		if s := loc.String(); s != "" && s != "UTC" {
			tzid = s
		}
	}
	t, err = time.ParseInLocation("20060102T150405", v, loc)
	if err != nil {
		return t, false, "", fmt.Errorf("icalx: fecha y hora inválida %q", v)
	}
	return t.UTC(), false, tzid, nil
}

// parseDuration lee una duración iCalendar (P1D, PT1H30M, P1W...).
func parseDuration(s string) (time.Duration, error) {
	orig := s
	s = strings.TrimSpace(s)
	neg := false
	if strings.HasPrefix(s, "-") {
		neg, s = true, s[1:]
	} else {
		s = strings.TrimPrefix(s, "+")
	}
	if !strings.HasPrefix(s, "P") {
		return 0, fmt.Errorf("icalx: duración inválida %q", orig)
	}
	s = s[1:]
	var d time.Duration
	inTime := false
	num := ""
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			num += string(c)
		case c == 'T':
			inTime = true
		default:
			n, err := strconv.Atoi(num)
			if err != nil {
				return 0, fmt.Errorf("icalx: duración inválida %q", orig)
			}
			num = ""
			switch {
			case c == 'W':
				d += time.Duration(n) * 7 * 24 * time.Hour
			case c == 'D':
				d += time.Duration(n) * 24 * time.Hour
			case c == 'H' && inTime:
				d += time.Duration(n) * time.Hour
			case c == 'M' && inTime:
				d += time.Duration(n) * time.Minute
			case c == 'S' && inTime:
				d += time.Duration(n) * time.Second
			default:
				return 0, fmt.Errorf("icalx: duración inválida %q", orig)
			}
		}
	}
	if num != "" {
		return 0, fmt.Errorf("icalx: duración inválida %q", orig)
	}
	if neg {
		d = -d
	}
	return d, nil
}

package icalx

import (
	"fmt"
	"time"

	"github.com/emersion/go-ical"
)

// transition es un cambio de horario: el instante, el offset de antes y el
// de después, y la zona que rige desde ahí.
type transition struct {
	at       time.Time
	from, to int
	name     string
	dst      bool
}

// vtimezone arma el VTIMEZONE de loc desde la base de zonas de Go con los
// cambios de horario del año de start. Sin cambios, un solo STANDARD; con
// horario de verano, STANDARD y DAYLIGHT con su regla anual. Si start es
// anterior al primer cambio de su año se usan los del año anterior, para que
// las observancias empiecen antes que el evento (RFC 5545 §3.6.5).
func vtimezone(loc *time.Location, start time.Time) *ical.Component {
	tz := ical.NewComponent("VTIMEZONE")
	tz.Props.Set(rawProp("TZID", loc.String()))
	year := start.In(loc).Year()
	trans := yearTransitions(loc, year)
	if len(trans) > 0 && start.Before(trans[0].at) {
		year--
		trans = yearTransitions(loc, year)
	}
	if len(trans) == 0 {
		name, off := time.Date(year, 1, 1, 0, 0, 0, 0, loc).Zone()
		tz.Children = append(tz.Children, observance("STANDARD", "19700101T000000", off, off, name, ""))
		return tz
	}
	// La regla anual solo vale para el caso común: un cambio de ida y uno de vuelta.
	yearly := len(trans) == 2 && trans[0].dst != trans[1].dst
	for _, tr := range trans {
		local := tr.at.Add(time.Duration(tr.from) * time.Second).UTC() // hora local antes del cambio
		kind := "STANDARD"
		if tr.dst {
			kind = "DAYLIGHT"
		}
		rule := ""
		if yearly {
			rule = fmt.Sprintf("FREQ=YEARLY;BYMONTH=%d;BYDAY=%s", int(local.Month()), weekOfMonth(local))
		}
		tz.Children = append(tz.Children, observance(kind, local.Format("20060102T150405"), tr.from, tr.to, tr.name, rule))
	}
	return tz
}

// yearTransitions lista los cambios de offset de loc durante el año.
func yearTransitions(loc *time.Location, year int) []transition {
	end := time.Date(year+1, 1, 1, 0, 0, 0, 0, loc)
	var out []transition
	t := time.Date(year, 1, 1, 0, 0, 0, 0, loc)
	for {
		_, next := t.ZoneBounds()
		if next.IsZero() || !next.Before(end) {
			return out
		}
		_, from := next.Add(-time.Second).Zone()
		name, to := next.Zone()
		if from != to {
			out = append(out, transition{at: next, from: from, to: to, name: name, dst: next.IsDST()})
		}
		t = next
	}
}

// weekOfMonth es el BYDAY de la fecha: "2SU", o "-1SU" si es la última semana.
func weekOfMonth(d time.Time) string {
	days := [...]string{"SU", "MO", "TU", "WE", "TH", "FR", "SA"}
	inMonth := time.Date(d.Year(), d.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	n := (d.Day()-1)/7 + 1
	if d.Day()+7 > inMonth {
		n = -1
	}
	return fmt.Sprintf("%d%s", n, days[d.Weekday()])
}

func observance(kind, dtstart string, from, to int, name, rule string) *ical.Component {
	c := ical.NewComponent(kind)
	c.Props.Set(rawProp("DTSTART", dtstart))
	c.Props.Set(rawProp("TZOFFSETFROM", formatOffset(from)))
	c.Props.Set(rawProp("TZOFFSETTO", formatOffset(to)))
	if name != "" {
		c.Props.SetText("TZNAME", name)
	}
	if rule != "" {
		c.Props.Set(rawProp("RRULE", rule))
	}
	return c
}

func rawProp(name, value string) *ical.Prop {
	p := ical.NewProp(name)
	p.Value = value
	return p
}

// formatOffset escribe un offset en segundos como "-0300" (o "+053028").
func formatOffset(sec int) string {
	sign := '+'
	if sec < 0 {
		sign, sec = '-', -sec
	}
	h, m, s := sec/3600, sec/60%60, sec%60
	if s != 0 {
		return fmt.Sprintf("%c%02d%02d%02d", sign, h, m, s)
	}
	return fmt.Sprintf("%c%02d%02d", sign, h, m)
}

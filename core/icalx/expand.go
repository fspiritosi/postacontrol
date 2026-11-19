package icalx

import (
	"sort"
	"time"

	"github.com/teambition/rrule-go"
)

// maxOccurrences limita cuántas ocurrencias devuelve Expand.
const maxOccurrences = 500

// maxScan limita cuántas ocurrencias se recorren antes de llegar a from.
const maxScan = 200000

// Expand devuelve los inicios (en UTC, ascendentes) de las ocurrencias que se
// superponen con [from, to). La regla se expande en la zona tzid (time.Local si
// está vacía o no carga; UTC en día completo) para que la hora local se
// mantenga al cruzar un cambio de horario. exdates son instantes UTC. Sin
// regla, o con una regla ilegible, devuelve solo start si se superpone.
func Expand(start, end time.Time, allDay bool, tzid, rule string, exdates []time.Time, from, to time.Time) []time.Time {
	dur := end.Sub(start)
	if dur < 0 {
		dur = 0
	}
	overlaps := func(s time.Time) bool {
		if !s.Before(to) {
			return false
		}
		if dur == 0 {
			return !s.Before(from) // evento puntual
		}
		return s.Add(dur).After(from)
	}
	single := func() []time.Time {
		if overlaps(start) {
			return []time.Time{start.UTC()}
		}
		return []time.Time{}
	}
	if rule == "" {
		return single()
	}

	loc := ruleLocation(allDay, tzid)
	opt, err := rrule.StrToROptionInLocation(rule, loc)
	if err != nil {
		return single()
	}
	opt.Dtstart = start.In(loc)
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return single()
	}
	set := &rrule.Set{}
	set.RRule(r)
	set.DTStart(opt.Dtstart)
	for _, e := range exdates {
		set.ExDate(e)
	}

	out := []time.Time{}
	next := set.Iterator()
	for i := 0; i < maxScan && len(out) < maxOccurrences; i++ {
		t, ok := next()
		if !ok || !t.Before(to) {
			break
		}
		if overlaps(t) {
			out = append(out, t.UTC())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// ruleLocation es la zona en la que se expande una regla: UTC en día completo,
// tzid si carga y, si no, time.Local.
func ruleLocation(allDay bool, tzid string) *time.Location {
	switch {
	case allDay:
		return time.UTC
	case tzid != "":
		if l, err := time.LoadLocation(tzid); err == nil {
			return l
		}
	}
	return time.Local
}

// SeriesEnd devuelve el fin (UTC) de la última ocurrencia de una serie con
// UNTIL o COUNT. false si la regla no termina, no se entiende o tiene más
// ocurrencias de las que se recorren (maxScan). No usa el tope de Expand.
func SeriesEnd(start, end time.Time, allDay bool, tzid, rule string) (time.Time, bool) {
	if rule == "" {
		return time.Time{}, false
	}
	loc := ruleLocation(allDay, tzid)
	opt, err := rrule.StrToROptionInLocation(rule, loc)
	if err != nil || (opt.Count == 0 && opt.Until.IsZero()) {
		return time.Time{}, false
	}
	opt.Dtstart = start.In(loc)
	r, err := rrule.NewRRule(*opt)
	if err != nil {
		return time.Time{}, false
	}
	dur := end.Sub(start)
	if dur < 0 {
		dur = 0
	}
	last := start
	next := r.Iterator()
	for i := 0; ; i++ {
		if i == maxScan {
			return time.Time{}, false
		}
		t, ok := next()
		if !ok {
			break
		}
		last = t
	}
	return last.Add(dur).UTC(), true
}

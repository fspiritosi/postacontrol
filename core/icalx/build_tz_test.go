package icalx

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/emersion/go-ical"
	"github.com/teambition/rrule-go"
)

func loadZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skip("sin base de zonas horarias")
	}
	return loc
}

// zoneComponent devuelve el VTIMEZONE del iCalendar armado.
func zoneComponent(t *testing.T, data []byte) *ical.Component {
	t.Helper()
	cal, err := ical.NewDecoder(bytes.NewReader(data)).Decode()
	if err != nil {
		t.Fatal(err)
	}
	var zones []*ical.Component
	for i, c := range cal.Children {
		if c.Name == "VTIMEZONE" {
			zones = append(zones, c)
			for _, d := range cal.Children[:i] {
				if d.Name == "VEVENT" {
					t.Error("el VTIMEZONE debe ir antes de los VEVENT")
				}
			}
		}
	}
	if len(zones) != 1 {
		t.Fatalf("VTIMEZONE = %d:\n%s", len(zones), data)
	}
	return zones[0]
}

// zoneOffset aplica las reglas de un VTIMEZONE (RFC 5545 §3.6.5) al instante
// at: la observancia que empezó más tarde sin pasarse define el offset.
func zoneOffset(t *testing.T, tz *ical.Component, at time.Time) int {
	t.Helper()
	best, bestOff := time.Time{}, 0
	for _, obs := range tz.Children {
		from, ok := parseOffset(obs.Props.Get("TZOFFSETFROM").Value)
		if !ok {
			t.Fatalf("TZOFFSETFROM inválido en %s", obs.Name)
		}
		to, ok := parseOffset(obs.Props.Get("TZOFFSETTO").Value)
		if !ok {
			t.Fatalf("TZOFFSETTO inválido en %s", obs.Name)
		}
		local, err := time.ParseInLocation("20060102T150405", obs.Props.Get("DTSTART").Value, time.UTC)
		if err != nil {
			t.Fatalf("DTSTART de %s: %v", obs.Name, err)
		}
		onsets := []time.Time{local}
		if p := obs.Props.Get("RRULE"); p != nil {
			opt, err := rrule.StrToROptionInLocation(p.Value, time.UTC)
			if err != nil {
				t.Fatalf("RRULE de %s: %v", obs.Name, err)
			}
			opt.Dtstart = local
			r, err := rrule.NewRRule(*opt)
			if err != nil {
				t.Fatal(err)
			}
			onsets = r.Between(local, at.Add(400*24*time.Hour), true)
		}
		for _, o := range onsets {
			// DTSTART es hora local con el offset anterior al cambio.
			u := o.Add(-time.Duration(from) * time.Second)
			if !u.After(at) && u.After(best) {
				best, bestOff = u, to
			}
		}
	}
	if best.IsZero() {
		t.Fatalf("ninguna observancia cubre %v", at)
	}
	return bestOff
}

func TestBuild_RecurringUsesTZID(t *testing.T) {
	ba := loadZone(t, "America/Argentina/Buenos_Aires")
	s := sampleSeries()
	m := &s.Master
	m.Start = time.Date(2026, 11, 23, 22, 0, 0, 0, ba).UTC() // lunes 22:00 = martes 01:00Z
	m.End = m.Start.Add(time.Hour)
	m.TZID = "America/Argentina/Buenos_Aires"
	m.RRule = "FREQ=WEEKLY;BYDAY=MO;UNTIL=20261222T025959Z"
	ex := time.Date(2026, 11, 30, 22, 0, 0, 0, ba).UTC()
	m.ExDates = []time.Time{ex}
	ov := *m
	ov.RRule, ov.ExDates = "", nil
	ov.RecurrenceID = time.Date(2026, 12, 7, 22, 0, 0, 0, ba).UTC()
	ov.Start = time.Date(2026, 12, 7, 23, 0, 0, 0, ba).UTC()
	ov.End = ov.Start.Add(time.Hour)
	s.Overrides = []VEvent{ov}

	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	txt := string(data)
	for _, want := range []string{
		"DTSTART;TZID=America/Argentina/Buenos_Aires:20261123T220000",
		"DTEND;TZID=America/Argentina/Buenos_Aires:20261123T230000",
		"EXDATE;TZID=America/Argentina/Buenos_Aires:20261130T220000",
		"RECURRENCE-ID;TZID=America/Argentina/Buenos_Aires:20261207T220000",
		"UNTIL=20261222T025959Z",
		"TZID:America/Argentina/Buenos_Aires",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("falta %q en:\n%s", want, txt)
		}
	}
	tz := zoneComponent(t, data)
	if len(tz.Children) != 1 || tz.Children[0].Name != "STANDARD" {
		t.Fatalf("observancias: %+v", tz.Children)
	}
	st := tz.Children[0]
	if st.Props.Get("DTSTART").Value != "19700101T000000" || st.Props.Get("TZOFFSETFROM").Value != "-0300" ||
		st.Props.Get("TZOFFSETTO").Value != "-0300" || st.Props.Get("TZNAME") == nil || st.Props.Get("RRULE") != nil {
		t.Errorf("STANDARD: %+v", st.Props)
	}

	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got := cal.Series[0].Master
	if got.TZID != "America/Argentina/Buenos_Aires" || !got.Start.Equal(m.Start) || len(got.ExDates) != 1 || !got.ExDates[0].Equal(ex) {
		t.Fatalf("round trip: %+v", got)
	}
	if len(cal.Series[0].Overrides) != 1 || !cal.Series[0].Overrides[0].RecurrenceID.Equal(ov.RecurrenceID) {
		t.Errorf("excepción: %+v", cal.Series[0].Overrides)
	}
	occ := Expand(got.Start, got.End, false, got.TZID, got.RRule, got.ExDates, utc(2026, 11, 1, 0, 0), utc(2027, 1, 1, 0, 0))
	if len(occ) != 4 { // 23/11, 7/12, 14/12 y 21/12 (el 30/11 está excluido)
		t.Fatalf("ocurrencias: %v", occ)
	}
	for _, o := range occ {
		l := o.In(ba)
		if l.Weekday() != time.Monday || l.Hour() != 22 {
			t.Errorf("%v cae %v a las %d", o, l.Weekday(), l.Hour())
		}
	}
}

func TestBuild_VTimezoneWithDST(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := sampleSeries()
	m := &s.Master
	m.Start = time.Date(2026, 10, 19, 9, 0, 0, 0, ny).UTC()
	m.End = m.Start.Add(time.Hour)
	m.TZID = "America/New_York"
	m.RRule = "FREQ=WEEKLY;COUNT=6"
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "DTSTART;TZID=America/New_York:20261019T090000") {
		t.Errorf("DTSTART:\n%s", data)
	}
	tz := zoneComponent(t, data)
	byName := map[string]*ical.Component{}
	for _, c := range tz.Children {
		byName[c.Name] = c
	}
	want := map[string][5]string{
		"DAYLIGHT": {"20260308T020000", "-0500", "-0400", "EDT", "FREQ=YEARLY;BYMONTH=3;BYDAY=2SU"},
		"STANDARD": {"20261101T020000", "-0400", "-0500", "EST", "FREQ=YEARLY;BYMONTH=11;BYDAY=1SU"},
	}
	if len(tz.Children) != 2 {
		t.Fatalf("observancias: %d", len(tz.Children))
	}
	for name, w := range want {
		c := byName[name]
		if c == nil {
			t.Fatalf("falta %s", name)
		}
		got := [5]string{c.Props.Get("DTSTART").Value, c.Props.Get("TZOFFSETFROM").Value, c.Props.Get("TZOFFSETTO").Value,
			c.Props.Get("TZNAME").Value, c.Props.Get("RRULE").Value}
		if got != w {
			t.Errorf("%s = %v, quiero %v", name, got, w)
		}
	}
	// Desde el inicio de la serie, las reglas dan el mismo offset que la base
	// de zonas, también otros años.
	for at := m.Start; at.Before(utc(2029, 1, 1, 0, 0)); at = at.Add(7 * time.Hour) {
		_, off := at.In(ny).Zone()
		if got := zoneOffset(t, tz, at); got != off {
			t.Fatalf("%v: offset %d, quiero %d", at, got, off)
		}
	}

	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got := cal.Series[0].Master
	occ := Expand(got.Start, got.End, false, got.TZID, got.RRule, nil, utc(2026, 10, 1, 0, 0), utc(2027, 1, 1, 0, 0))
	if len(occ) != 6 {
		t.Fatalf("ocurrencias: %v", occ)
	}
	for _, o := range occ {
		if h := o.In(ny).Hour(); h != 9 {
			t.Errorf("%v → %d h locales", o, h)
		}
	}
	if occ[0].Hour() != 13 || occ[5].Hour() != 14 {
		t.Errorf("antes y después del cambio: %v %v", occ[0], occ[5])
	}
}

func TestBuild_LastWeekRule(t *testing.T) {
	ber := loadZone(t, "Europe/Berlin")
	s := sampleSeries()
	s.Master.Start = time.Date(2026, 6, 1, 10, 0, 0, 0, ber).UTC()
	s.Master.End = s.Master.Start.Add(time.Hour)
	s.Master.TZID = "Europe/Berlin"
	s.Master.RRule = "FREQ=DAILY;COUNT=3"
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	tz := zoneComponent(t, data)
	rules := map[string]string{}
	for _, c := range tz.Children {
		rules[c.Name] = c.Props.Get("RRULE").Value
	}
	if rules["DAYLIGHT"] != "FREQ=YEARLY;BYMONTH=3;BYDAY=-1SU" || rules["STANDARD"] != "FREQ=YEARLY;BYMONTH=10;BYDAY=-1SU" {
		t.Errorf("reglas: %v", rules)
	}
	for at := s.Master.Start; at.Before(utc(2028, 1, 1, 0, 0)); at = at.Add(5 * time.Hour) {
		_, off := at.In(ber).Zone()
		if got := zoneOffset(t, tz, at); got != off {
			t.Fatalf("%v: offset %d, quiero %d", at, got, off)
		}
	}
}

// Una serie que empieza antes del primer cambio de su año usa los cambios del
// año anterior: así las observancias empiezan antes que el evento.
func TestBuild_VTimezoneBeforeFirstChange(t *testing.T) {
	ny := loadZone(t, "America/New_York")
	s := sampleSeries()
	s.Master.Start = time.Date(2026, 1, 12, 9, 0, 0, 0, ny).UTC()
	s.Master.End = s.Master.Start.Add(time.Hour)
	s.Master.TZID = "America/New_York"
	s.Master.RRule = "FREQ=WEEKLY"
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	tz := zoneComponent(t, data)
	for _, c := range tz.Children {
		if v := c.Props.Get("DTSTART").Value; v[:4] != "2025" {
			t.Errorf("%s empieza %s", c.Name, v)
		}
	}
	for at := s.Master.Start; at.Before(utc(2027, 6, 1, 0, 0)); at = at.Add(7 * time.Hour) {
		_, off := at.In(ny).Zone()
		if got := zoneOffset(t, tz, at); got != off {
			t.Fatalf("%v: offset %d, quiero %d", at, got, off)
		}
	}
}

func TestBuild_UTCWithoutRuleOrZone(t *testing.T) {
	loadZone(t, "America/Argentina/Buenos_Aires")
	cases := map[string]func(*VEvent){
		"sin regla":     func(v *VEvent) { v.TZID = "America/Argentina/Buenos_Aires" },
		"sin zona":      func(v *VEvent) { v.RRule = "FREQ=WEEKLY" },
		"zona inválida": func(v *VEvent) { v.RRule = "FREQ=WEEKLY"; v.TZID = "Marte/Olympus" },
	}
	for name, mod := range cases {
		s := sampleSeries()
		mod(&s.Master)
		data, err := Build("REQUEST", s, now0)
		if err != nil {
			t.Fatal(err)
		}
		txt := string(data)
		if strings.Contains(txt, "VTIMEZONE") || !strings.Contains(txt, "DTSTART:20261203T150000Z") {
			t.Errorf("%s:\n%s", name, txt)
		}
	}
	s := sampleSeries()
	s.Master.AllDay, s.Master.RRule, s.Master.TZID = true, "FREQ=WEEKLY", "America/Argentina/Buenos_Aires"
	s.Master.Start, s.Master.End = utc(2026, 12, 3, 0, 0), utc(2026, 12, 4, 0, 0)
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	if txt := string(data); strings.Contains(txt, "VTIMEZONE") || !strings.Contains(txt, "DTSTART;VALUE=DATE:20261203") {
		t.Errorf("día completo:\n%s", txt)
	}
}

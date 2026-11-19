package icalx

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"postacontrol/core/model"
)

var now0 = time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)

func sampleSeries() Series {
	return Series{Master: VEvent{
		UID: "uid-1@postacontrol", Sequence: 2, Summary: "Reunión; con, comas", Description: "Línea 1\nLínea 2",
		Location:  "Sala 3",
		Start:     time.Date(2026, 12, 3, 15, 0, 0, 0, time.UTC),
		End:       time.Date(2026, 12, 3, 16, 0, 0, 0, time.UTC),
		Organizer: model.Address{Name: "Org Ejemplo", Email: "org@example.com"},
		Attendees: []Attendee{
			{Email: "a@example.com", Name: "Ana", Partstat: "ACCEPTED"},
			{Email: "b@example.com", Name: "Beto"},
		},
	}}
}

func TestBuild_RequestRoundTrip(t *testing.T) {
	s := sampleSeries()
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Method != "REQUEST" || len(cal.Series) != 1 {
		t.Fatalf("método/series: %q %d", cal.Method, len(cal.Series))
	}
	m := cal.Series[0].Master
	if m.UID != s.Master.UID || m.Sequence != 2 || m.Organizer != s.Master.Organizer {
		t.Errorf("maestro: %+v", m)
	}
	if !m.Start.Equal(s.Master.Start) || !m.End.Equal(s.Master.End) {
		t.Errorf("horas: %v %v", m.Start, m.End)
	}
	if m.Summary != s.Master.Summary || m.Description != s.Master.Description {
		t.Errorf("textos: %q %q", m.Summary, m.Description)
	}
	if len(m.Attendees) != 2 || m.Attendees[0].Partstat != "ACCEPTED" || m.Attendees[1].Partstat != "NEEDS-ACTION" ||
		m.Attendees[1].Email != "b@example.com" || m.Attendees[1].Name != "Beto" {
		t.Errorf("invitados: %+v", m.Attendees)
	}
	txt := string(data)
	for _, want := range []string{"PRODID:-//PostaControl//PostaControl//ES", "VERSION:2.0", "CALSCALE:GREGORIAN", "DTSTAMP:20261119T120000Z", "DTSTART:20261203T150000Z"} {
		if !strings.Contains(txt, want) {
			t.Errorf("falta %q", want)
		}
	}
	if strings.Contains(txt, "VTIMEZONE") {
		t.Error("no debería haber VTIMEZONE")
	}
}

func TestBuild_Cancel(t *testing.T) {
	data, err := Build("CANCEL", sampleSeries(), now0)
	if err != nil {
		t.Fatal(err)
	}
	txt := string(data)
	if !strings.Contains(txt, "STATUS:CANCELLED") || !strings.Contains(txt, "SEQUENCE:2") || !strings.Contains(txt, "METHOD:CANCEL") {
		t.Errorf("CANCEL incompleto:\n%s", txt)
	}
}

func TestBuild_Errors(t *testing.T) {
	if _, err := Build("FOO", sampleSeries(), now0); err == nil {
		t.Error("método desconocido debería fallar")
	}
	s := sampleSeries()
	s.Master.UID = ""
	if _, err := Build("REQUEST", s, now0); err == nil {
		t.Error("sin UID debería fallar")
	}
}

func TestBuild_AllDayValueDate(t *testing.T) {
	s := sampleSeries()
	s.Master.AllDay = true
	s.Master.Start = time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
	s.Master.End = time.Date(2026, 11, 21, 0, 0, 0, 0, time.UTC)
	data, err := Build("PUBLISH", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	txt := string(data)
	if !strings.Contains(txt, "DTSTART;VALUE=DATE:20261120") || !strings.Contains(txt, "DTEND;VALUE=DATE:20261121") {
		t.Errorf("falta VALUE=DATE:\n%s", txt)
	}
	cal, err := Parse(data)
	if err != nil || !cal.Series[0].Master.AllDay {
		t.Fatalf("round trip: %v", err)
	}
}

func TestBuild_OverrideAndExdate(t *testing.T) {
	s := sampleSeries()
	s.Master.RRule = "FREQ=WEEKLY;COUNT=5"
	ex := time.Date(2026, 12, 10, 15, 0, 0, 0, time.UTC)
	s.Master.ExDates = []time.Time{ex}
	ov := s.Master
	ov.RRule, ov.ExDates = "", nil
	ov.RecurrenceID = time.Date(2026, 12, 17, 15, 0, 0, 0, time.UTC)
	ov.Start = time.Date(2026, 12, 17, 18, 0, 0, 0, time.UTC)
	ov.End = time.Date(2026, 12, 17, 19, 0, 0, 0, time.UTC)
	s.Overrides = []VEvent{ov}
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	txt := string(data)
	if n := strings.Count(txt, "BEGIN:VEVENT"); n != 2 {
		t.Fatalf("VEVENT = %d", n)
	}
	if strings.Count(txt, "RECURRENCE-ID") != 1 || !strings.Contains(txt, "RRULE:FREQ=WEEKLY;COUNT=5") || !strings.Contains(txt, "EXDATE:20261210T150000Z") {
		t.Errorf("contenido:\n%s", txt)
	}
	if strings.Index(txt, "RECURRENCE-ID") < strings.LastIndex(txt, "BEGIN:VEVENT") {
		t.Error("RECURRENCE-ID debe estar en el segundo VEVENT")
	}
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	got := cal.Series[0]
	if len(got.Overrides) != 1 || !got.Overrides[0].RecurrenceID.Equal(ov.RecurrenceID) ||
		len(got.Master.ExDates) != 1 || !got.Master.ExDates[0].Equal(ex) {
		t.Errorf("round trip: %+v", got)
	}
}

func TestBuild_LinesFolded(t *testing.T) {
	s := sampleSeries()
	s.Master.Description = strings.Repeat("ñandú largo ", 40)
	s.Master.Summary = strings.Repeat("á", 100)
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte("\r\n")) {
		t.Error("no termina en CRLF")
	}
	for _, l := range strings.SplitAfter(string(data), "\r\n") {
		if l == "" {
			continue
		}
		if !strings.HasSuffix(l, "\r\n") || strings.Contains(strings.TrimSuffix(l, "\r\n"), "\n") {
			t.Fatalf("línea sin CRLF: %q", l)
		}
		if len(l)-2 > 75 {
			t.Fatalf("línea de %d octetos: %q", len(l)-2, l)
		}
	}
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if cal.Series[0].Master.Description != s.Master.Description || cal.Series[0].Master.Summary != s.Master.Summary {
		t.Error("el plegado corrompió el texto")
	}
}

func readSample(t *testing.T, name string) Calendar {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := Parse(b)
	if err != nil {
		t.Fatal(err)
	}
	return cal
}

func TestBuildReply_OnlyMyAttendee(t *testing.T) {
	orig := readSample(t, "google-request.ics")
	uid := orig.Series[0].Master.UID
	data, err := BuildReply(orig, uid, "Fabricio@Example.com", "ACCEPTED", now0)
	if err != nil {
		t.Fatal(err)
	}
	txt := string(data)
	if !strings.Contains(txt, "METHOD:REPLY") || strings.Contains(txt, "VALARM") || strings.Contains(txt, "DESCRIPTION") {
		t.Errorf("REPLY:\n%s", txt)
	}
	if strings.Count(txt, "ATTENDEE") != 1 || !strings.Contains(txt, "PARTSTAT=ACCEPTED") {
		t.Errorf("invitados:\n%s", txt)
	}
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	o, r := orig.Series[0].Master, cal.Series[0].Master
	if cal.Method != "REPLY" || r.UID != o.UID || r.Sequence != o.Sequence || r.Organizer != o.Organizer ||
		len(r.Attendees) != 1 || r.Attendees[0].Email != "fabricio@example.com" || r.Attendees[0].Partstat != "ACCEPTED" ||
		!r.Start.Equal(o.Start) {
		t.Errorf("reply: %+v", r)
	}
	if !strings.Contains(txt, "DTSTAMP:20261119T120000Z") {
		t.Error("DTSTAMP")
	}
}

func TestBuildReply_KeepsVTimezone(t *testing.T) {
	orig := readSample(t, "outlook-request.ics")
	uid := orig.Series[0].Master.UID
	data, err := BuildReply(orig, uid, orig.Series[0].Master.Attendees[0].Email, "TENTATIVE", now0)
	if err != nil {
		t.Fatal(err)
	}
	txt := string(data)
	if !strings.Contains(txt, "BEGIN:VTIMEZONE") || !strings.Contains(txt, "TZID:Argentina Standard Time") ||
		!strings.Contains(txt, "DTSTART;TZID=Argentina Standard Time:") {
		t.Errorf("falta VTIMEZONE/TZID:\n%s", txt)
	}
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !cal.Series[0].Master.Start.Equal(orig.Series[0].Master.Start) {
		t.Error("cambió la hora")
	}
}

func TestBuildReply_Errors(t *testing.T) {
	orig := readSample(t, "google-request.ics")
	uid := orig.Series[0].Master.UID
	for _, c := range []struct{ uid, att, ps string }{
		{"otro", "a@b.com", "ACCEPTED"},
		{uid, "", "ACCEPTED"},
		{uid, "a@b.com", "NEEDS-ACTION"},
	} {
		if _, err := BuildReply(orig, c.uid, c.att, c.ps, now0); err == nil {
			t.Errorf("%+v debería fallar", c)
		}
	}
}

func TestBuild_NormalizesLineBreaks(t *testing.T) {
	s := sampleSeries()
	s.Master.Description = "a\r\nb"
	s.Master.Summary = "x\ry"
	s.Master.Location = "l1\r\nl2"
	data, err := Build("REQUEST", s, now0)
	if err != nil {
		t.Fatal(err)
	}
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	m := cal.Series[0].Master
	if m.Description != "a\nb" || m.Summary != "x\ny" || m.Location != "l1\nl2" {
		t.Errorf("got %q %q %q", m.Description, m.Summary, m.Location)
	}
}

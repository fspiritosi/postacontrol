package icalx

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func leer(t *testing.T, nombre string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", nombre))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func utc(y int, m time.Month, d, h, mi int) time.Time {
	return time.Date(y, m, d, h, mi, 0, 0, time.UTC)
}

func baires(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Skip("sin base de zonas horarias")
	}
	return loc
}

func ics(cuerpo string) []byte {
	return []byte("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:x\r\n" + cuerpo + "END:VCALENDAR\r\n")
}

func TestParse_GoogleRequest(t *testing.T) {
	cal, err := Parse(leer(t, "google-request.ics"))
	if err != nil {
		t.Fatal(err)
	}
	if cal.Method != "REQUEST" || len(cal.Series) != 1 {
		t.Fatalf("method=%q series=%d", cal.Method, len(cal.Series))
	}
	ev := cal.Series[0].Master
	if ev.UID != "7q3k9d2m5x8reunionproyecto@google.com" || ev.Sequence != 1 {
		t.Errorf("uid=%q seq=%d", ev.UID, ev.Sequence)
	}
	if ev.Organizer.Email != "organizador@example.com" || ev.Organizer.Name != "Organizador Ejemplo" {
		t.Errorf("organizador: %+v", ev.Organizer)
	}
	var found bool
	for _, a := range ev.Attendees {
		if a.Email == "fabricio@example.com" {
			found = true
			if a.Partstat != "NEEDS-ACTION" {
				t.Errorf("partstat=%q", a.Partstat)
			}
		}
	}
	if !found || len(ev.Attendees) != 3 {
		t.Errorf("asistentes: %+v", ev.Attendees)
	}
	if !ev.Start.Equal(utc(2026, 12, 3, 15, 0)) || !ev.End.Equal(utc(2026, 12, 3, 16, 0)) {
		t.Errorf("start=%v end=%v", ev.Start, ev.End)
	}
	if ev.Start.Location() != time.UTC || ev.TZID != "" {
		t.Errorf("zona: %v tzid=%q", ev.Start.Location(), ev.TZID)
	}
	if ev.Description != "Revision del avance del proyecto.\nAgenda: demo y proximos pasos." {
		t.Errorf("descripción: %q", ev.Description)
	}
}

func TestParse_OutlookWindowsTZID(t *testing.T) {
	baires(t)
	cal, err := Parse(leer(t, "outlook-request.ics"))
	if err != nil {
		t.Fatal(err)
	}
	ev := cal.Series[0].Master
	if !ev.Start.Equal(utc(2026, 12, 10, 13, 0)) || !ev.End.Equal(utc(2026, 12, 10, 14, 0)) {
		t.Errorf("start=%v end=%v", ev.Start, ev.End)
	}
	if ev.TZID != "America/Argentina/Buenos_Aires" {
		t.Errorf("tzid=%q", ev.TZID)
	}
	if ev.Sequence != 0 || ev.Summary != "Seguimiento Outlook" {
		t.Errorf("seq=%d summary=%q", ev.Sequence, ev.Summary)
	}
}

func TestParse_UnknownTZIDUsesVTimezoneOffset(t *testing.T) {
	data := ics("BEGIN:VTIMEZONE\r\nTZID:Zona Inventada\r\nBEGIN:STANDARD\r\nDTSTART:16010101T000000\r\nTZOFFSETFROM:-0300\r\nTZOFFSETTO:-0300\r\nEND:STANDARD\r\nEND:VTIMEZONE\r\n" +
		"BEGIN:VEVENT\r\nUID:a\r\nDTSTART;TZID=Zona Inventada:20261120T100000\r\nEND:VEVENT\r\n")
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if got := cal.Series[0].Master.Start; !got.Equal(utc(2026, 11, 20, 13, 0)) {
		t.Errorf("start=%v", got)
	}
}

func TestParse_Floating(t *testing.T) {
	loc := baires(t)
	cal, err := ParseInLocation(ics("BEGIN:VEVENT\r\nUID:a\r\nDTSTART:20261120T100000\r\nEND:VEVENT\r\n"), loc)
	if err != nil {
		t.Fatal(err)
	}
	ev := cal.Series[0].Master
	if !ev.Start.Equal(utc(2026, 11, 20, 13, 0)) || ev.TZID != "" {
		t.Errorf("start=%v tzid=%q", ev.Start, ev.TZID)
	}
	// Sin DTEND ni DURATION con hora: el fin es el inicio.
	if !ev.End.Equal(ev.Start) {
		t.Errorf("end=%v", ev.End)
	}
}

func TestParse_AllDay(t *testing.T) {
	cal, err := Parse(ics("BEGIN:VEVENT\r\nUID:a\r\nDTSTART;VALUE=DATE:20261120\r\nEND:VEVENT\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	ev := cal.Series[0].Master
	if !ev.AllDay || !ev.Start.Equal(utc(2026, 11, 20, 0, 0)) || !ev.End.Equal(utc(2026, 11, 21, 0, 0)) {
		t.Errorf("%+v", ev)
	}
}

func TestParse_Duration(t *testing.T) {
	cal, err := Parse(ics("BEGIN:VEVENT\r\nUID:a\r\nDTSTART:20261120T100000Z\r\nDURATION:PT1H30M\r\nEND:VEVENT\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cal.Series[0].Master.End; !got.Equal(utc(2026, 11, 20, 11, 30)) {
		t.Errorf("end=%v", got)
	}
}

func TestParse_RecurringWithOverride(t *testing.T) {
	baires(t)
	cal, err := Parse(leer(t, "google-export.ics"))
	if err != nil {
		t.Fatal(err)
	}
	var s *Series
	for i := range cal.Series {
		if cal.Series[i].Master.UID == "serie-semanal-daily@google.com" {
			s = &cal.Series[i]
		}
	}
	if s == nil {
		t.Fatal("no está la serie semanal")
	}
	if s.Master.RRule != "FREQ=WEEKLY;COUNT=10;BYDAY=MO,WE" {
		t.Errorf("rrule=%q", s.Master.RRule)
	}
	if len(s.Master.ExDates) != 1 || !s.Master.ExDates[0].Equal(utc(2026, 11, 30, 13, 0)) {
		t.Errorf("exdates=%v", s.Master.ExDates)
	}
	if len(s.Overrides) != 1 {
		t.Fatalf("overrides=%d", len(s.Overrides))
	}
	o := s.Overrides[0]
	if !o.RecurrenceID.Equal(utc(2026, 12, 2, 13, 0)) || !o.Start.Equal(utc(2026, 12, 2, 18, 0)) {
		t.Errorf("recurrence-id=%v start=%v", o.RecurrenceID, o.Start)
	}
	if !s.Master.RecurrenceID.IsZero() {
		t.Error("el maestro no debería tener RECURRENCE-ID")
	}
}

func TestParse_OnlyOverrideBecomesMaster(t *testing.T) {
	data := ics("BEGIN:VEVENT\r\nUID:a\r\nRECURRENCE-ID:20261120T100000Z\r\nDTSTART:20261120T120000Z\r\nEND:VEVENT\r\n")
	cal, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	s := cal.Series[0]
	if s.Master.UID != "a" || len(s.Overrides) != 1 {
		t.Errorf("%+v", s)
	}
}

func TestParse_Cancel(t *testing.T) {
	cal, err := Parse(leer(t, "google-cancel.ics"))
	if err != nil {
		t.Fatal(err)
	}
	if cal.Method != "CANCEL" || cal.Series[0].Master.Status != "CANCELLED" {
		t.Errorf("method=%q status=%q", cal.Method, cal.Series[0].Master.Status)
	}
	if cal.Series[0].Master.Sequence != 3 {
		t.Errorf("seq=%d", cal.Series[0].Master.Sequence)
	}
}

func TestParse_AttendeeCase(t *testing.T) {
	cal, err := Parse(ics("BEGIN:VEVENT\r\nUID:a\r\nDTSTART:20261120T100000Z\r\nATTENDEE;CN=Juan:MAILTO:Juan@X.com\r\nEND:VEVENT\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	a := cal.Series[0].Master.Attendees[0]
	if a.Email != "juan@x.com" || a.Name != "Juan" {
		t.Errorf("%+v", a)
	}
}

func TestParse_ExportName(t *testing.T) {
	cal, err := Parse(leer(t, "google-export.ics"))
	if err != nil {
		t.Fatal(err)
	}
	if cal.Name != "fabricio@example.com" || cal.Method != "PUBLISH" {
		t.Errorf("name=%q method=%q", cal.Name, cal.Method)
	}
	if len(cal.Series) != 5 {
		t.Errorf("series=%d", len(cal.Series))
	}
	if len(cal.Raw) == 0 {
		t.Error("Raw vacío")
	}
}

func TestParse_Garbage(t *testing.T) {
	if _, err := Parse([]byte("hola")); err == nil {
		t.Error("se esperaba un error")
	}
}

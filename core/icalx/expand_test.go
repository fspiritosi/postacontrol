package icalx

import (
	"testing"
	"time"
)

func TestExpand_WeeklyExdate(t *testing.T) {
	start := utc(2026, 11, 2, 10, 0) // lunes
	end := start.Add(time.Hour)
	ex := []time.Time{utc(2026, 11, 11, 10, 0)}
	got := Expand(start, end, false, "UTC", "FREQ=WEEKLY;BYDAY=MO,WE", ex, utc(2026, 11, 1, 0, 0), utc(2026, 11, 29, 0, 0))
	if len(got) != 7 {
		t.Fatalf("got %d: %v", len(got), got)
	}
	for i := 1; i < len(got); i++ {
		if !got[i].After(got[i-1]) {
			t.Errorf("no ordenado: %v", got)
		}
	}
	for _, g := range got {
		if g.Equal(ex[0]) || g.Location() != time.UTC {
			t.Errorf("exdate o zona: %v", g)
		}
	}
}

func TestExpand_UntilAndCount(t *testing.T) {
	start := utc(2026, 11, 2, 10, 0)
	end := start.Add(time.Hour)
	from, to := utc(2026, 1, 1, 0, 0), utc(2027, 12, 31, 0, 0)
	if got := Expand(start, end, false, "UTC", "FREQ=DAILY;COUNT=3", nil, from, to); len(got) != 3 {
		t.Errorf("COUNT: %d", len(got))
	}
	if got := Expand(start, end, false, "UTC", "FREQ=DAILY;UNTIL=20261105T100000Z", nil, from, to); len(got) != 4 {
		t.Errorf("UNTIL: %d", len(got))
	}
}

func TestExpand_NoRule(t *testing.T) {
	start := utc(2026, 11, 2, 10, 0)
	end := start.Add(time.Hour)
	if got := Expand(start, end, false, "", "", nil, utc(2026, 11, 2, 0, 0), utc(2026, 11, 3, 0, 0)); len(got) != 1 || !got[0].Equal(start) {
		t.Errorf("got %v", got)
	}
	if got := Expand(start, end, false, "", "", nil, utc(2026, 11, 3, 0, 0), utc(2026, 11, 4, 0, 0)); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestExpand_BadRule(t *testing.T) {
	start := utc(2026, 11, 2, 10, 0)
	got := Expand(start, start.Add(time.Hour), false, "UTC", "FREQ=NOPE", nil, utc(2026, 11, 1, 0, 0), utc(2026, 11, 5, 0, 0))
	if len(got) != 1 || !got[0].Equal(start) {
		t.Errorf("got %v", got)
	}
}

func TestExpand_OverlapNotJustStart(t *testing.T) {
	start := utc(2026, 11, 2, 23, 0)
	end := utc(2026, 11, 3, 1, 0)
	got := Expand(start, end, false, "UTC", "FREQ=DAILY;COUNT=3", nil, utc(2026, 11, 3, 0, 0), utc(2026, 11, 4, 0, 0))
	// ocurrencias del 2 (hasta 01:00 del 3, se superpone) y del 3 (23:00)
	if len(got) != 2 || !got[0].Equal(start) {
		t.Errorf("got %v", got)
	}
}

func TestExpand_DSTKeepsWallTime(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("sin tzdata")
	}
	start := time.Date(2026, 10, 22, 9, 0, 0, 0, ny).UTC()
	got := Expand(start, start.Add(time.Hour), false, "America/New_York", "FREQ=WEEKLY;COUNT=4", nil, utc(2026, 10, 1, 0, 0), utc(2026, 12, 31, 0, 0))
	if len(got) != 4 {
		t.Fatalf("got %v", got)
	}
	for _, g := range got {
		if h := g.In(ny).Hour(); h != 9 {
			t.Errorf("%v → %d h locales", g, h)
		}
	}
	if got[3].UTC().Hour() != 14 {
		t.Errorf("después del cambio debería ser 14Z: %v", got[3])
	}
}

func TestExpand_AllDay(t *testing.T) {
	start := utc(2026, 11, 20, 0, 0)
	got := Expand(start, start.AddDate(0, 0, 1), true, "America/New_York", "FREQ=DAILY;COUNT=3", []time.Time{utc(2026, 11, 21, 0, 0)}, utc(2026, 11, 1, 0, 0), utc(2026, 12, 1, 0, 0))
	if len(got) != 2 || !got[0].Equal(start) || !got[1].Equal(utc(2026, 11, 22, 0, 0)) {
		t.Errorf("got %v", got)
	}
}

func TestExpand_Cap500(t *testing.T) {
	start := utc(2026, 1, 1, 10, 0)
	got := Expand(start, start.Add(time.Hour), false, "UTC", "FREQ=DAILY", nil, utc(2026, 1, 1, 0, 0), utc(2036, 1, 1, 0, 0))
	if len(got) != 500 {
		t.Errorf("got %d", len(got))
	}
}

func TestExpand_ZeroDurationAtFrom(t *testing.T) {
	start := utc(2026, 11, 2, 10, 0)
	got := Expand(start, start, false, "UTC", "", nil, start, start.Add(time.Hour))
	if len(got) != 1 || !got[0].Equal(start) {
		t.Errorf("got %v", got)
	}
	if got := Expand(start, start, false, "UTC", "", nil, start.Add(-time.Hour), start); len(got) != 0 {
		t.Errorf("start == to no debería entrar: %v", got)
	}
}

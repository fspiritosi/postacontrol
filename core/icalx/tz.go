package icalx

import (
	"strings"
	"time"

	"github.com/emersion/go-ical"
)

// windowsZones traduce los nombres de zona de Windows (Outlook/Exchange) a IANA.
var windowsZones = map[string]string{
	"Argentina Standard Time":        "America/Argentina/Buenos_Aires",
	"SA Eastern Standard Time":       "America/Cayenne",
	"E. South America Standard Time": "America/Sao_Paulo",
	"Pacific SA Standard Time":       "America/Santiago",
	"SA Pacific Standard Time":       "America/Bogota",
	"Central Standard Time (Mexico)": "America/Mexico_City",
	"Eastern Standard Time":          "America/New_York",
	"Central Standard Time":          "America/Chicago",
	"Pacific Standard Time":          "America/Los_Angeles",
	"Mountain Standard Time":         "America/Denver",
	"GMT Standard Time":              "Europe/London",
	"W. Europe Standard Time":        "Europe/Berlin",
	"Romance Standard Time":          "Europe/Paris",
	"UTC":                            "UTC",
}

// resolveTZ devuelve la zona de un TZID: IANA, nombre de Windows, el offset
// del VTIMEZONE del propio archivo o, en última instancia, UTC.
func resolveTZ(tzid string, vtz map[string]*ical.Component) *time.Location {
	if tzid == "" {
		return time.UTC
	}
	if loc, err := time.LoadLocation(tzid); err == nil {
		return loc
	}
	if iana, ok := windowsZones[tzid]; ok {
		if loc, err := time.LoadLocation(iana); err == nil {
			return loc
		}
	}
	if comp := vtz[tzid]; comp != nil {
		for _, child := range comp.Children {
			if child.Name != "STANDARD" {
				continue
			}
			if p := child.Props.Get("TZOFFSETTO"); p != nil {
				if off, ok := parseOffset(p.Value); ok {
					return time.FixedZone("", off)
				}
			}
		}
	}
	return time.UTC
}

// parseOffset lee "+hhmm", "-hhmm" o con segundos ("-hhmmss") en segundos.
func parseOffset(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if len(s) < 5 || (s[0] != '+' && s[0] != '-') {
		return 0, false
	}
	digits := s[1:]
	if len(digits) != 4 && len(digits) != 6 {
		return 0, false
	}
	n := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	var h, m, sec int
	if len(digits) == 4 {
		h, m = n/100, n%100
	} else {
		h, m, sec = n/10000, n/100%100, n%100
	}
	off := h*3600 + m*60 + sec
	if s[0] == '-' {
		off = -off
	}
	return off, true
}

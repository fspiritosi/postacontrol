package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestParseSearch(t *testing.T) {
	got := ParseSearch(`presupuesto De:Ana para:"Juan Pérez" asunto:"factura marzo" tiene:adjunto es:no-leído es:destacado desde:2026-10-01 hasta:31/10/2026 http://x.com`)
	assert.Equal(t, []string{"presupuesto", "http://x.com"}, got.Words)
	assert.Equal(t, []string{"Ana"}, got.From)
	assert.Equal(t, []string{"Juan Pérez"}, got.To)
	assert.Equal(t, []string{"factura marzo"}, got.Subject)
	assert.True(t, got.HasAttachment)
	assert.True(t, got.Unread)
	assert.True(t, got.Flagged)
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.Local), got.Since)
	// hasta: es el día entero.
	assert.Equal(t, time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local), got.Before)
	assert.True(t, got.HasFilters())
}

func TestParseSearch_UnknownOrBrokenStaysText(t *testing.T) {
	got := ParseSearch(`nota:importante desde:ayer es:raro de:`)
	assert.Equal(t, []string{"nota:importante", "desde:ayer", "es:raro", "de:"}, got.Words)
	assert.False(t, got.HasFilters())
	// Variantes sin tilde y en inglés.
	got = ParseSearch(`from:a is:unread has:attachment es:noleido before:2026-01-02 after:2026-01-01`)
	assert.Equal(t, []string{"a"}, got.From)
	assert.True(t, got.Unread)
	assert.True(t, got.HasAttachment)
	assert.False(t, got.Since.IsZero())
	assert.Equal(t, time.Date(2026, 1, 2, 0, 0, 0, 0, time.Local), got.Before, "before: es excluyente")
}

func TestSearch_Filters(t *testing.T) {
	s := newStore(t)
	_, f := seedFolder(t, s)
	ctx := context.Background()
	add := func(uid uint32, subject, fromName, fromEmail string, date time.Time, seen, flagged, attach bool, to string) {
		h := hdr(uid, date, seen)
		h.Subject = subject
		h.From = model.Address{Name: fromName, Email: fromEmail}
		h.Flagged, h.HasAttachments = flagged, attach
		h.To = []model.Address{{Email: to}}
		require.NoError(t, s.UpsertHeaders(ctx, f, []HeaderRow{h}))
	}
	oct := time.Date(2026, 10, 15, 12, 0, 0, 0, time.Local)
	add(1, "Factura marzo", "Ana", "ana@x.com", oct, true, false, true, "yo@x.com")
	add(2, "Hola Ana", "Beto", "beto@x.com", oct.AddDate(0, 1, 0), false, true, false, "ana@x.com")
	add(3, "Reunión", "Ana", "ana@x.com", oct.AddDate(0, -2, 0), false, false, false, "yo@x.com")

	q := func(text string) []string { return subjects(search(t, s, text, 0)) }
	// de: mira solo el remitente (el asunto "Hola Ana" no cuenta).
	assert.ElementsMatch(t, []string{"Factura marzo", "Reunión"}, q("de:ana"))
	assert.Equal(t, []string{"Hola Ana"}, q("para:ana"))
	assert.Equal(t, []string{"Factura marzo"}, q(`asunto:"factura marzo"`))
	// Solo filtros, sin palabras.
	assert.Equal(t, []string{"Factura marzo"}, q("tiene:adjunto"))
	assert.ElementsMatch(t, []string{"Hola Ana", "Reunión"}, q("es:no-leído"))
	assert.Equal(t, []string{"Hola Ana"}, q("es:destacado"))
	assert.Equal(t, []string{"Factura marzo"}, q("desde:2026-10-01 hasta:2026-10-31"))
	assert.Equal(t, []string{"Reunión"}, q("de:ana es:no-leído"))
	assert.Empty(t, q("   "))
}

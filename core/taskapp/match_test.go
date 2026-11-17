package taskapp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// proyectosDemo arma un set de proyectos para los tests de sugerencia.
var proyectosDemo = []Project{
	{ID: 1, Name: "Sinergia", Status: "active", ApproverEmails: []string{"pmolina@sinergiambiental.com.ar"}},
	{ID: 2, Name: "Otro", Status: "active", ApproverEmails: []string{"yordanpz@hotmail.com"}},
}

func TestSuggest_ExactEmail(t *testing.T) {
	assert.Equal(t, int64(1), SuggestProject("pmolina@sinergiambiental.com.ar", proyectosDemo))
}

func TestSuggest_CaseInsensitive(t *testing.T) {
	assert.Equal(t, int64(1), SuggestProject("PMolina@SinergiAmbiental.com.ar", proyectosDemo))
}

func TestSuggest_Domain(t *testing.T) {
	assert.Equal(t, int64(1), SuggestProject("otro@sinergiambiental.com.ar", proyectosDemo))
}

func TestSuggest_SubdomainDoesNotMatch(t *testing.T) {
	assert.Equal(t, int64(0), SuggestProject("x@mail.sinergiambiental.com.ar", proyectosDemo))
}

func TestSuggest_PublicDomainNever(t *testing.T) {
	assert.Equal(t, int64(0), SuggestProject("otro@hotmail.com", proyectosDemo))
	// La misma dirección exacta sí es coincidencia.
	assert.Equal(t, int64(2), SuggestProject("yordanpz@hotmail.com", proyectosDemo))
}

func TestSuggest_AmbiguousIsEmpty(t *testing.T) {
	proyectos := []Project{
		{ID: 1, Status: "active", ApproverEmails: []string{"a@sinergiambiental.com.ar"}},
		{ID: 2, Status: "active", ApproverEmails: []string{"b@sinergiambiental.com.ar"}},
	}
	assert.Equal(t, int64(0), SuggestProject("otro@sinergiambiental.com.ar", proyectos))
}

func TestSuggest_AmbiguousExactEmailDoesNotFallBackToDomain(t *testing.T) {
	proyectos := []Project{
		{ID: 1, Status: "active", ApproverEmails: []string{"pm@sinergiambiental.com.ar"}},
		{ID: 2, Status: "active", ApproverEmails: []string{"pm@sinergiambiental.com.ar"}},
		{ID: 3, Status: "active", ApproverEmails: []string{"otro@sinergiambiental.com.ar"}},
	}
	assert.Equal(t, int64(0), SuggestProject("pm@sinergiambiental.com.ar", proyectos))
}

func TestSuggest_IgnoresInactive(t *testing.T) {
	proyectos := []Project{
		{ID: 1, Status: "completed", ApproverEmails: []string{"pm@sinergiambiental.com.ar"}},
	}
	assert.Equal(t, int64(0), SuggestProject("pm@sinergiambiental.com.ar", proyectos))
	assert.Equal(t, int64(0), SuggestProject("otro@sinergiambiental.com.ar", proyectos))
}

func TestSuggest_TrimsSpacesAndRejectsInvalid(t *testing.T) {
	assert.Equal(t, int64(1), SuggestProject("  pmolina@sinergiambiental.com.ar  ", proyectosDemo))
	assert.Equal(t, int64(0), SuggestProject("", proyectosDemo))
	assert.Equal(t, int64(0), SuggestProject("sin-arroba", proyectosDemo))
}

func TestIsPublicDomain(t *testing.T) {
	assert.True(t, IsPublicDomain("gmail.com"))
	assert.True(t, IsPublicDomain("Gmail.COM"))
	assert.True(t, IsPublicDomain("fibertel.com.ar"))
	assert.False(t, IsPublicDomain("sinergiambiental.com.ar"))
	assert.False(t, IsPublicDomain("mail.gmail.com"))
}

func TestCleanSubject(t *testing.T) {
	cases := map[string]string{
		"Re: RV: Fwd:  Error al exportar": "Error al exportar",
		"RE:FW: x":                        "x",
		"Respuesta pendiente":             "Respuesta pendiente",
		"":                                "",
		"  Re: Hola  ":                    "Hola",
		"Reenviar: Tr: Consulta":          "Consulta",
	}
	for in, want := range cases {
		assert.Equal(t, want, CleanSubject(in), "entrada %q", in)
	}
}

func TestSuggest_EmptyDomainNeverMatches(t *testing.T) {
	proyectos := []Project{
		{ID: 1, Status: "active", ApproverEmails: []string{""}},
		{ID: 2, Status: "inactive"},
	}
	assert.Equal(t, int64(0), SuggestProject("x@", proyectos))
	proyectos = []Project{{ID: 3, Status: "active", ApproverEmails: []string{"sin-arroba"}}}
	assert.Equal(t, int64(0), SuggestProject("x@", proyectos))
}

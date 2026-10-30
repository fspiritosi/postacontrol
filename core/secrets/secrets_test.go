package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemory_SetGetDelete(t *testing.T) {
	s := NewMemory()
	const email = "fabricio@example.com"

	require.NoError(t, s.Set(email, "clave-1"))
	got, err := s.Get(email)
	require.NoError(t, err)
	assert.Equal(t, "clave-1", got)

	require.NoError(t, s.Set(email, "clave-2"))
	got, err = s.Get(email)
	require.NoError(t, err)
	assert.Equal(t, "clave-2", got, "Set debe reemplazar la contraseña previa")

	require.NoError(t, s.Delete(email))
	_, err = s.Get(email)
	assert.ErrorIs(t, err, ErrNotFound)

	assert.NoError(t, s.Delete(email), "borrar algo inexistente no es error")
	assert.NoError(t, s.Delete("nunca@existio.invalid"))
}

func TestKeyring_RoundTrip(t *testing.T) {
	if os.Getenv("POSTACONTROL_KEYRING_TEST") != "1" {
		t.Skip("requiere el llavero real: definí POSTACONTROL_KEYRING_TEST=1")
	}

	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	email := "test+" + hex.EncodeToString(buf) + "@postacontrol.invalid"

	s := NewKeyring()
	t.Cleanup(func() { _ = s.Delete(email) })

	_, err = s.Get(email)
	assert.ErrorIs(t, err, ErrNotFound, "un correo nuevo no tiene contraseña")

	const password = "clave-de-prueba-no-real"
	require.NoError(t, s.Set(email, password))

	got, err := s.Get(email)
	require.NoError(t, err)
	assert.Equal(t, password, got)

	require.NoError(t, s.Delete(email))
	_, err = s.Get(email)
	assert.ErrorIs(t, err, ErrNotFound)

	assert.NoError(t, s.Delete(email), "borrar dos veces no es error")
}

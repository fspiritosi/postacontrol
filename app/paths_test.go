package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDataDir_UsesXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_DATA_HOME", tmp)
	dir, err := DataDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(tmp, "postacontrol"), dir)
}

func TestDataDir_FallbackHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", home)
	dir, err := DataDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".local", "share", "postacontrol"), dir)
}

func TestValidateExternalURL(t *testing.T) {
	for _, ok := range []string{"http://a.com", "https://a.com/x?y=1", "mailto:a@b.com", "HTTPS://A.COM"} {
		assert.NoError(t, validateExternalURL(ok), ok)
	}
	for _, bad := range []string{"", "file:///etc/passwd", "javascript:alert(1)", "ftp://a.com", "data:text/html,x", "a.com", "http://", "https:///x", "mailto:"} {
		assert.Error(t, validateExternalURL(bad), bad)
	}
}

func TestSuggestedFilename(t *testing.T) {
	assert.Equal(t, "adjunto", suggestedFilename(""))
	assert.Equal(t, "factura.pdf", suggestedFilename("factura.pdf"))
}

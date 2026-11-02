package mail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

var pngSig = []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}

func writePNG(t *testing.T, name string, extra int) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, append(append([]byte{}, pngSig...), make([]byte, extra)...), 0o600))
	return p
}

func TestSetBackground_CopiesAndServesDataURL(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	src := writePNG(t, "fondo.png", 64)
	require.NoError(t, c.SetBackground(ctx, src))

	st, err := c.GetSettings(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, st.Background)
	assert.FileExists(t, filepath.Join(c.dataDir, st.Background))

	require.NoError(t, os.Remove(src))
	url, err := c.BackgroundImage(ctx)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(url, "data:image/png;base64,"), url)
	assert.Contains(t, n.names(), EventSettingsChanged)
}

func TestSetBackground_ReplacesAndRemovesOld(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	require.NoError(t, c.SetBackground(ctx, writePNG(t, "a.png", 8)))
	first, _ := c.GetSettings(ctx)
	require.NoError(t, c.SetBackground(ctx, writePNG(t, "b.png", 16)))
	second, _ := c.GetSettings(ctx)
	assert.NotEqual(t, first.Background, second.Background)
	assert.NoFileExists(t, filepath.Join(c.dataDir, first.Background))
	assert.FileExists(t, filepath.Join(c.dataDir, second.Background))
}

func TestSetBackground_RejectsNonImage(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	p := filepath.Join(t.TempDir(), "x.txt")
	require.NoError(t, os.WriteFile(p, []byte("hola"), 0o600))
	err := c.SetBackground(ctx, p)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "background", ue.Code)
	assert.Equal(t, "Solo se pueden usar imágenes PNG, JPG, GIF o WebP.", ue.Msg)
	st, _ := c.GetSettings(ctx)
	assert.Empty(t, st.Background)
}

func TestSetBackground_RejectsTooBig(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	p := writePNG(t, "big.png", MaxBackgroundBytes+1-len(pngSig))
	err := c.SetBackground(context.Background(), p)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "background", ue.Code)
	assert.Contains(t, ue.Msg, "20 MB")
}

func TestClearBackground_RemovesFile(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	require.NoError(t, c.SetBackground(ctx, writePNG(t, "a.png", 8)))
	st, _ := c.GetSettings(ctx)
	require.NoError(t, c.ClearBackground(ctx))
	got, _ := c.GetSettings(ctx)
	assert.Empty(t, got.Background)
	assert.NoFileExists(t, filepath.Join(c.dataDir, st.Background))
}

func TestBackgroundImage_MissingFileIsEmpty(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	require.NoError(t, c.SetBackground(ctx, writePNG(t, "a.png", 8)))
	st, _ := c.GetSettings(ctx)
	require.NoError(t, os.Remove(filepath.Join(c.dataDir, st.Background)))
	url, err := c.BackgroundImage(ctx)
	require.NoError(t, err)
	assert.Equal(t, "", url)
}

func TestBackground_ConcurrentWithSetSettings(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	src := writePNG(t, "a.png", 8)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			assert.NoError(t, c.SetBackground(ctx, src))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 30; i++ {
			assert.NoError(t, c.SetSettings(ctx, model.Settings{ReadingPane: "top"}))
		}
	}()
	wg.Wait()
	st, err := c.GetSettings(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, st.Background)
	assert.FileExists(t, filepath.Join(c.dataDir, st.Background))
	files, _ := filepath.Glob(filepath.Join(c.dataDir, "background-*"))
	assert.Len(t, files, 1)
}

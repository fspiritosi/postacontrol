package mail

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func autoFiles(t *testing.T, dir string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(dir, autoBackupPrefix+"*.json"))
	require.NoError(t, err)
	out := []string{}
	for _, p := range m {
		out = append(out, filepath.Base(p))
	}
	slices.Sort(out)
	return out
}

func TestAutoBackup_DirIsValidatedAndKept(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	var ue *UserError
	require.ErrorAs(t, c.SetAutoBackupDir(ctx, filepath.Join(t.TempDir(), "no-existe")), &ue)

	dir := t.TempDir()
	require.NoError(t, c.SetAutoBackupDir(ctx, dir))
	assert.NotEmpty(t, n.matching(EventSettingsChanged))
	// Guardar otras opciones no la pisa (la UI manda la configuración entera).
	require.NoError(t, c.SetSettings(ctx, model.Settings{UnifiedInbox: true}))
	st, err := c.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, dir, st.AutoBackupDir)
	assert.True(t, st.UnifiedInbox)

	require.NoError(t, c.SetAutoBackupDir(ctx, ""))
	st, err = c.GetSettings(ctx)
	require.NoError(t, err)
	assert.Empty(t, st.AutoBackupDir)
}

func TestAutoBackup_WeeklyAndKeepsTheLastOnes(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	_, err := c.SaveContact(ctx, model.Contact{Name: "Ana", Emails: []string{"ana@x.com"}})
	require.NoError(t, err)

	// Sin carpeta elegida no hace nada.
	done, err := c.RunAutoBackup(ctx, false)
	require.NoError(t, err)
	assert.False(t, done)

	dir := t.TempDir()
	// Copias viejas de otras semanas, y un archivo ajeno que no se toca.
	now := time.Now()
	for i := 1; i <= 9; i++ {
		day := now.AddDate(0, 0, -7*i-1).Format("2006-01-02")
		require.NoError(t, os.WriteFile(filepath.Join(dir, autoBackupPrefix+day+".json"), []byte("{}"), 0o600))
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "otra-cosa.json"), []byte("{}"), 0o600))
	require.NoError(t, c.SetAutoBackupDir(ctx, dir))

	done, err = c.RunAutoBackup(ctx, false)
	require.NoError(t, err)
	assert.True(t, done, "la última tiene más de una semana")
	files := autoFiles(t, dir)
	require.Len(t, files, autoBackupKeep)
	today := autoBackupPrefix + now.Format("2006-01-02") + ".json"
	assert.Equal(t, today, files[len(files)-1])
	assert.FileExists(t, filepath.Join(dir, "otra-cosa.json"))

	data, err := os.ReadFile(filepath.Join(dir, today))
	require.NoError(t, err)
	var b backupFile
	require.NoError(t, json.Unmarshal(data, &b))
	require.Len(t, b.Contacts, 1)

	// Ya hay una de esta semana: no repite, salvo que se pida.
	done, err = c.RunAutoBackup(ctx, false)
	require.NoError(t, err)
	assert.False(t, done)
	done, err = c.RunAutoBackup(ctx, true)
	require.NoError(t, err)
	assert.True(t, done)
	assert.Len(t, autoFiles(t, dir), autoBackupKeep)

	info, err := c.AutoBackupInfo(ctx)
	require.NoError(t, err)
	assert.Equal(t, dir, info.Dir)
	assert.Equal(t, now.Format("2006-01-02"), info.LastDate)
}

func TestAutoBackup_MissingDirReportsError(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "pendrive")
	require.NoError(t, os.Mkdir(dir, 0o700))
	require.NoError(t, c.SetAutoBackupDir(ctx, dir))
	require.NoError(t, os.Remove(dir)) // se desconectó el pendrive

	_, err := c.RunAutoBackup(ctx, false)
	var ue *UserError
	require.ErrorAs(t, err, &ue)
}

func TestBackup_IncludesImageSenders(t *testing.T) {
	src, _, _, _ := newTestCore(t)
	ctx := context.Background()
	require.NoError(t, src.TrustImageSender(ctx, "avisos@banco.com"))
	require.NoError(t, src.TrustImageSender(ctx, "news@tienda.com"))
	path := exportTo(t, src)

	dst, _, _, _ := newTestCore(t)
	require.NoError(t, dst.TrustImageSender(ctx, "news@tienda.com"))
	got, err := dst.ImportBackup(ctx, path)
	require.NoError(t, err)
	assert.Equal(t, 1, got.ImageSendersAdded)
	assert.Equal(t, 1, got.ImageSendersSkipped)
	list, err := dst.ListImageSenders(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{"avisos@banco.com", "news@tienda.com"}, list)
}

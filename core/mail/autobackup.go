package mail

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"postacontrol/core/model"
)

// Respaldo automático: una vez por semana se guarda el respaldo (ver
// backup.go) en la carpeta que se eligió, como
// postacontrol-respaldo-auto-AAAA-MM-DD.json, y se conservan los últimos
// autoBackupKeep. La fecha de la última copia sale de los nombres de los
// archivos: no hace falta guardar nada más.

const (
	autoBackupPrefix = "postacontrol-respaldo-auto-"
	autoBackupKeep   = 8
	autoBackupEvery  = 7 * 24 * time.Hour

	msgAutoBackupDir   = "No se puede guardar en esa carpeta. Elegí otra."
	msgAutoBackupWrite = "No pudimos guardar el respaldo automático en la carpeta elegida. Revisá que esté conectada o elegí otra en Configuración → Respaldo."
)

// SetAutoBackupDir elige la carpeta del respaldo automático ("" lo apaga).
// Tiene que existir y dejar escribir.
func (c *Core) SetAutoBackupDir(ctx context.Context, dir string) error {
	if dir != "" {
		if !filepath.IsAbs(dir) || !writableDir(dir) {
			return &UserError{Code: "invalid", Msg: msgAutoBackupDir}
		}
	}
	if err := c.saveAutoBackupDir(ctx, dir); err != nil {
		return err
	}
	c.emit(EventSettingsChanged, nil)
	return nil
}

func (c *Core) saveAutoBackupDir(ctx context.Context, dir string) error {
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	st.AutoBackupDir = dir
	if err := c.store.SaveSettings(ctx, st); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	return nil
}

// writableDir dice si dir es una carpeta donde se puede crear un archivo.
func writableDir(dir string) bool {
	fi, err := os.Stat(dir)
	if err != nil || !fi.IsDir() {
		return false
	}
	f, err := os.CreateTemp(dir, ".postacontrol-prueba-*")
	if err != nil {
		return false
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return true
}

// autoBackupDates devuelve las fechas (AAAA-MM-DD) de las copias automáticas
// que hay en dir, de la más vieja a la más nueva.
func autoBackupDates(dir string) []string {
	matches, _ := filepath.Glob(filepath.Join(dir, autoBackupPrefix+"*.json"))
	dates := []string{}
	for _, m := range matches {
		d := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), autoBackupPrefix), ".json")
		if _, err := time.Parse(time.DateOnly, d); err == nil {
			dates = append(dates, d)
		}
	}
	slices.Sort(dates)
	return dates
}

// AutoBackupInfo devuelve la carpeta elegida y la fecha de la última copia.
func (c *Core) AutoBackupInfo(ctx context.Context) (model.AutoBackupInfo, error) {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return model.AutoBackupInfo{}, c.ctxOrUserError(ctx, err)
	}
	info := model.AutoBackupInfo{Dir: st.AutoBackupDir}
	if info.Dir != "" {
		if dates := autoBackupDates(info.Dir); len(dates) > 0 {
			info.LastDate = dates[len(dates)-1]
		}
	}
	return info, nil
}

// RunAutoBackup guarda la copia si hay carpeta elegida y la última tiene una
// semana o más (o siempre, con force), y borra las que pasan de
// autoBackupKeep. Devuelve si guardó.
func (c *Core) RunAutoBackup(ctx context.Context, force bool) (bool, error) {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return false, c.ctxOrUserError(ctx, err)
	}
	dir := st.AutoBackupDir
	if dir == "" {
		return false, nil
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		return false, &UserError{Code: "io", Msg: msgAutoBackupWrite, Err: err}
	}
	now := time.Now()
	dates := autoBackupDates(dir)
	if !force && len(dates) > 0 {
		last, _ := time.ParseInLocation(time.DateOnly, dates[len(dates)-1], now.Location())
		if now.Sub(last) < autoBackupEvery {
			return false, nil
		}
	}
	dest := filepath.Join(dir, autoBackupPrefix+now.Format(time.DateOnly)+".json")
	if err := c.ExportBackup(ctx, dest); err != nil {
		return false, &UserError{Code: "io", Msg: msgAutoBackupWrite, Err: err}
	}
	dates = autoBackupDates(dir)
	for len(dates) > autoBackupKeep {
		_ = os.Remove(filepath.Join(dir, autoBackupPrefix+dates[0]+".json"))
		dates = dates[1:]
	}
	return true, nil
}

// StartAutoBackup revisa al arrancar y después cada every si toca la copia
// semanal. Si falla lo avisa con EventAutoBackupFailed, una vez por arranque:
// el pendrive desconectado no tiene que avisar cada every.
func (c *Core) StartAutoBackup(ctx context.Context, every time.Duration) {
	if every <= 0 {
		return
	}
	c.spawn(func(life context.Context) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		defer context.AfterFunc(life, cancel)()
		warned := false
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			if _, err := c.RunAutoBackup(ctx, false); err != nil && ctx.Err() == nil && !warned {
				warned = true
				c.emit(EventAutoBackupFailed, map[string]any{"message": asUserError(err).Error()})
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	})
}

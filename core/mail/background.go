package mail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"postacontrol/core/model"
)

// MaxBackgroundBytes es el tope de la imagen de fondo.
const MaxBackgroundBytes = 20 << 20

func backgroundErr(msg string, err error) error {
	return &UserError{Code: "background", Msg: msg, Err: err}
}

var backgroundExt = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
}

// SetBackground copia la imagen de srcPath a la carpeta de datos y la deja como
// fondo. El archivo anterior se borra recién después de guardar la configuración.
func (c *Core) SetBackground(ctx context.Context, srcPath string) error {
	if err := c.setBackgroundLocked(ctx, srcPath); err != nil {
		return err
	}
	c.emit(EventSettingsChanged, nil)
	return nil
}

func (c *Core) setBackgroundLocked(ctx context.Context, srcPath string) error {
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	name := filepath.Base(srcPath)
	fi, err := os.Stat(srcPath)
	if err != nil {
		return backgroundErr("No pudimos leer «"+name+"».", err)
	}
	if fi.IsDir() {
		return backgroundErr("«"+name+"» es una carpeta; elegí una imagen.", nil)
	}
	if fi.Size() > MaxBackgroundBytes {
		// El tope es redondo: se dice "20 MB", no "20.0 MB".
		max := fmt.Sprintf("%d MB", MaxBackgroundBytes>>20)
		if sameSize(fi.Size(), MaxBackgroundBytes) {
			return backgroundErr("La imagen supera el máximo de "+max+".", nil)
		}
		return backgroundErr("La imagen pesa "+formatSize(fi.Size())+"; el máximo es "+max+".", nil)
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return backgroundErr("No pudimos leer «"+name+"».", err)
	}
	ext, ok := backgroundExt[http.DetectContentType(data)]
	if !ok {
		return backgroundErr("Solo se pueden usar imágenes PNG, JPG, GIF o WebP.", nil)
	}
	file := fmt.Sprintf("background-%d%s", c.now().UnixNano(), ext)
	dest := filepath.Join(c.dataDir, file)
	if err := writeFileAtomic(dest, data, 0o600); err != nil {
		return backgroundErr("No pudimos guardar el fondo.", err)
	}
	old, err := c.saveBackground(ctx, file)
	if err != nil {
		_ = os.Remove(dest)
		return err
	}
	c.removeBackgroundFile(old, file)
	return nil
}

// ClearBackground quita el fondo y borra su archivo.
func (c *Core) ClearBackground(ctx context.Context) error {
	if err := c.clearBackgroundLocked(ctx); err != nil {
		return err
	}
	c.emit(EventSettingsChanged, nil)
	return nil
}

func (c *Core) clearBackgroundLocked(ctx context.Context) error {
	c.settingsMu.Lock()
	defer c.settingsMu.Unlock()
	old, err := c.saveBackground(ctx, "")
	if err != nil {
		return err
	}
	c.removeBackgroundFile(old, "")
	return nil
}

// saveBackground guarda file como fondo conservando el resto de la configuración
// y devuelve el nombre anterior.
func (c *Core) saveBackground(ctx context.Context, file string) (string, error) {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return "", c.ctxOrUserError(ctx, err)
	}
	old := st.Background
	st.Background = file
	if err := c.store.SaveSettings(ctx, st); err != nil {
		return "", c.ctxOrUserError(ctx, err)
	}
	return old, nil
}

func (c *Core) removeBackgroundFile(old, keep string) {
	// Solo nombres simples: nunca salir de la carpeta de datos.
	if old == "" || old == keep || filepath.Base(old) != old {
		return
	}
	_ = os.Remove(filepath.Join(c.dataDir, old))
}

// BackgroundImage devuelve el fondo como data URL, o "" si no hay fondo o su
// archivo ya no existe.
func (c *Core) BackgroundImage(ctx context.Context) (string, error) {
	st, err := c.store.GetSettings(ctx)
	if err != nil {
		return "", c.ctxOrUserError(ctx, err)
	}
	return c.readBackground(st)
}

func (c *Core) readBackground(st model.Settings) (string, error) {
	if st.Background == "" || filepath.Base(st.Background) != st.Background {
		return "", nil
	}
	data, err := os.ReadFile(filepath.Join(c.dataDir, st.Background))
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", backgroundErr("No pudimos leer el fondo.", err)
	}
	return "data:" + http.DetectContentType(data) + ";base64," + base64.StdEncoding.EncodeToString(data), nil
}

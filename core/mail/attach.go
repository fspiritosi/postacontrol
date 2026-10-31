package mail

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"

	"postacontrol/core/model"
)

const (
	// MaxMessageBytes es el tope de un correo: adjuntos más imágenes pegadas.
	MaxMessageBytes = 25 << 20
	// MaxInlineImageBytes es el tope de una imagen insertada en el cuerpo.
	MaxInlineImageBytes = 5 << 20
)

// formatSize repite el formato del frontend (lib/format.ts), así los números
// de los mensajes coinciden con los de la pantalla.
func formatSize(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%d KB", int64(float64(n)/1024+0.5))
	}
	return fmt.Sprintf("%.1f MB", float64(n)/1024/1024)
}

// sameSize dice si n y max se muestran iguales (25.0 MB contra 25.0 MB): ahí
// "pesa X; el máximo es X" confunde y el mensaje usa otra redacción.
func sameSize(n, max int64) bool { return formatSize(n) == formatSize(max) }

func attachErr(msg string) error { return &UserError{Code: "attach", Msg: msg} }

// StageFiles guarda los archivos de paths como adjuntos a la espera del envío.
// Valida todos antes de guardar ninguno: o se adjuntan todos, o ninguno.
func (c *Core) StageFiles(ctx context.Context, paths []string) ([]model.StagedAttachment, error) {
	var total int64
	for _, p := range paths {
		name := filepath.Base(p)
		fi, err := os.Stat(p)
		switch {
		case err != nil:
			return nil, &UserError{Code: "attach", Msg: "No pudimos leer «" + name + "».", Err: err}
		case fi.IsDir():
			return nil, attachErr("«" + name + "» es una carpeta; solo se pueden adjuntar archivos.")
		case fi.Size() > MaxMessageBytes && sameSize(fi.Size(), MaxMessageBytes):
			return nil, attachErr("«" + name + "» supera el máximo de " + formatSize(MaxMessageBytes) + " por correo.")
		case fi.Size() > MaxMessageBytes:
			return nil, attachErr("«" + name + "» pesa " + formatSize(fi.Size()) + "; el máximo por correo es " + formatSize(MaxMessageBytes) + ".")
		}
		total += fi.Size()
	}
	if total > MaxMessageBytes {
		return nil, attachErr("Los archivos suman " + formatSize(total) + "; el máximo por correo es " + formatSize(MaxMessageBytes) + ".")
	}
	out := make([]model.StagedAttachment, 0, len(paths))
	rollback := func() {
		ids := make([]int64, len(out))
		for i, st := range out {
			ids[i] = st.ID
		}
		_ = c.store.DeleteStagedAttachments(context.WithoutCancel(ctx), ids)
	}
	for _, p := range paths {
		name := filepath.Base(p)
		data, err := os.ReadFile(p)
		if err != nil {
			rollback()
			return nil, &UserError{Code: "attach", Msg: "No pudimos leer «" + name + "».", Err: err}
		}
		st, err := c.store.StageAttachment(ctx, name, mimeFor(name, data), data)
		if err != nil {
			rollback()
			return nil, c.ctxOrUserError(ctx, err)
		}
		out = append(out, st)
	}
	return out, nil
}

// mimeFor deduce el tipo por la extensión y, si no la conoce, por el contenido.
func mimeFor(name string, data []byte) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return http.DetectContentType(data)
}

// LoadImage lee una imagen del disco para insertarla en el cuerpo. El tipo
// sale del contenido, no de la extensión.
func (c *Core) LoadImage(ctx context.Context, path string) (model.PickedImage, error) {
	name := filepath.Base(path)
	fi, err := os.Stat(path)
	if err != nil {
		return model.PickedImage{}, &UserError{Code: "image", Msg: "No pudimos leer «" + name + "».", Err: err}
	}
	if fi.IsDir() {
		return model.PickedImage{}, &UserError{Code: "image", Msg: "«" + name + "» es una carpeta; elegí una imagen."}
	}
	if fi.Size() > MaxInlineImageBytes {
		return model.PickedImage{}, imageTooBig(fi.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return model.PickedImage{}, &UserError{Code: "image", Msg: "No pudimos leer «" + name + "».", Err: err}
	}
	switch ct := http.DetectContentType(data); ct {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return model.PickedImage{DataURL: "data:" + ct + ";base64," + base64.StdEncoding.EncodeToString(data), Filename: name}, nil
	}
	return model.PickedImage{}, &UserError{Code: "image", Msg: "Solo se pueden insertar imágenes PNG, JPG, GIF o WebP."}
}

func imageTooBig(n int64) error {
	if sameSize(n, MaxInlineImageBytes) {
		return &UserError{Code: "image", Msg: "La imagen supera el máximo de " + formatSize(MaxInlineImageBytes) + "."}
	}
	return &UserError{Code: "image", Msg: "La imagen pesa " + formatSize(n) + "; el máximo es " + formatSize(MaxInlineImageBytes) + "."}
}

// StagedInfo devuelve los datos de los adjuntos preparados ids (sin contenido).
func (c *Core) StagedInfo(ctx context.Context, ids []int64) ([]model.StagedAttachment, error) {
	out, err := c.store.StagedInfo(ctx, ids)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return out, nil
}

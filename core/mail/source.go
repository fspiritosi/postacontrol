package mail

import (
	"context"
	"strings"
	"unicode/utf8"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
)

// Para ver el código fuente alcanza con el principio: los adjuntos en base64
// pueden ser megas de texto que nadie lee.
const maxSourceView = 1 << 20

// rawMessage baja el mensaje entero del servidor (sin marcarlo leído).
func (c *Core) rawMessage(ctx context.Context, messageID int64) ([]byte, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	d, _, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	f, last, err := c.store.GetFolder(ctx, d.FolderID)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	var raw []byte
	err = c.withConn(ctx, d.AccountID, func(_ context.Context, conn *imapx.Conn) error {
		if err := selectChecked(conn, f, last, false); err != nil {
			return err
		}
		var err error
		raw, err = conn.FetchRaw(d.UID)
		return err
	})
	return raw, err
}

// MessageSource devuelve el mensaje tal como está en el servidor, para verlo.
func (c *Core) MessageSource(ctx context.Context, messageID int64) (model.MessageSource, error) {
	raw, err := c.rawMessage(ctx, messageID)
	if err != nil {
		return model.MessageSource{}, err
	}
	src := model.MessageSource{}
	if len(raw) > maxSourceView {
		raw, src.Truncated = raw[:maxSourceView], true
	}
	// Un correo en otra codificación (latin1) no es UTF-8 válido: se reemplaza
	// lo que no se puede mostrar en vez de mandar texto roto a la UI.
	src.Text = strings.ToValidUTF8(string(raw), string(utf8.RuneError))
	return src, nil
}

// SaveMessageSource guarda el mensaje entero como .eml en dest.
func (c *Core) SaveMessageSource(ctx context.Context, messageID int64, dest string) error {
	raw, err := c.rawMessage(ctx, messageID)
	if err != nil {
		return err
	}
	if err := writeFileAtomic(dest, raw, 0o644); err != nil {
		return &UserError{Code: "io", Msg: "No pudimos guardar el archivo en esa ubicación.", Err: err}
	}
	return nil
}

// emlFilename sugiere el nombre del archivo a partir del asunto.
func emlFilename(subject string) string {
	name := strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\':
			return '_'
		case ':', '*', '?', '"', '<', '>', '|':
			return '-'
		}
		if r < 32 {
			return -1
		}
		return r
	}, strings.TrimSpace(subject))
	if r := []rune(name); len(r) > 80 {
		name = string(r[:80])
	}
	if name == "" {
		name = "correo"
	}
	return name + ".eml"
}

// EmlFilename es el nombre sugerido para guardar el mensaje.
func (c *Core) EmlFilename(ctx context.Context, messageID int64) string {
	d, _, err := c.store.GetMessage(ctx, messageID)
	if err != nil {
		return emlFilename("")
	}
	return emlFilename(d.Subject)
}

package mail

import (
	"context"
	"errors"
	"strings"

	"postacontrol/core/htmlx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	msgTemplateNoName  = "Poné un nombre para la plantilla."
	msgTemplateMissing = "Esa plantilla ya no existe."
)

// ListTemplates devuelve las plantillas por nombre.
func (c *Core) ListTemplates(ctx context.Context) ([]model.Template, error) {
	list, err := c.store.ListTemplates(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return list, nil
}

// SaveTemplate crea la plantilla (ID 0) o actualiza la existente. El HTML se
// sanea antes de guardarlo, como el de las firmas.
func (c *Core) SaveTemplate(ctx context.Context, t model.Template) (model.Template, error) {
	t.Name = strings.TrimSpace(t.Name)
	t.Subject = strings.TrimSpace(t.Subject)
	if t.Name == "" {
		return model.Template{}, &UserError{Code: "invalid", Msg: msgTemplateNoName}
	}
	t.HTML, _ = htmlx.Sanitize(t.HTML)
	saved, err := c.store.SaveTemplate(ctx, t)
	if errors.Is(err, store.ErrNotFound) {
		return model.Template{}, &UserError{Code: "notfound", Msg: msgTemplateMissing}
	}
	if err != nil {
		return model.Template{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventTemplatesChanged, nil)
	return saved, nil
}

// DeleteTemplate borra la plantilla.
func (c *Core) DeleteTemplate(ctx context.Context, id int64) error {
	if err := c.store.DeleteTemplate(ctx, id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventTemplatesChanged, nil)
	return nil
}

package mail

import (
	"context"
	"net/mail"
	"strings"
)

const msgBadImageSender = "Esa no es una dirección de correo válida."

// TrustImageSender hace que los correos de email muestren siempre sus
// imágenes remotas. Sin distinguir mayúsculas.
func (c *Core) TrustImageSender(ctx context.Context, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if a, err := mail.ParseAddress(email); err != nil || a.Address != email {
		return &UserError{Code: "invalid", Msg: msgBadImageSender}
	}
	if err := c.store.TrustImageSender(ctx, email); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventImageSendersChanged, nil)
	return nil
}

// UntrustImageSender vuelve a bloquear las imágenes remotas de email.
func (c *Core) UntrustImageSender(ctx context.Context, email string) error {
	if err := c.store.UntrustImageSender(ctx, strings.TrimSpace(email)); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventImageSendersChanged, nil)
	return nil
}

// ListImageSenders devuelve los remitentes con imágenes permitidas, en orden.
func (c *Core) ListImageSenders(ctx context.Context) ([]string, error) {
	list, err := c.store.ListImageSenders(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return list, nil
}

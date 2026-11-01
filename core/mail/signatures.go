package mail

import (
	"context"
	"strings"

	"postacontrol/core/htmlx"
	"postacontrol/core/model"
)

// ListSignatures devuelve las firmas, compartidas por todas las cuentas.
func (c *Core) ListSignatures(ctx context.Context) ([]model.Signature, error) {
	list, err := c.store.ListSignatures(ctx)
	if err != nil {
		return nil, c.ctxOrUserError(ctx, err)
	}
	return list, nil
}

// SaveSignature crea la firma (ID 0) o actualiza la existente. El HTML se sanea
// antes de guardarlo.
func (c *Core) SaveSignature(ctx context.Context, s model.Signature) (model.Signature, error) {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return model.Signature{}, &UserError{Code: "invalid", Msg: "Poné un nombre para la firma."}
	}
	s.HTML, _ = htmlx.Sanitize(s.HTML)
	saved, err := c.store.SaveSignature(ctx, s)
	if err != nil {
		return model.Signature{}, c.ctxOrUserError(ctx, err)
	}
	c.emit(EventSignaturesChanged, nil)
	return saved, nil
}

// DeleteSignature borra la firma; las cuentas que la usaban quedan sin firma.
func (c *Core) DeleteSignature(ctx context.Context, id int64) error {
	if err := c.store.DeleteSignature(ctx, id); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventSignaturesChanged, nil)
	c.emit(EventAccountsChanged, nil)
	return nil
}

// SetAccountSignature fija la firma por defecto de la cuenta (0 = sin firma).
func (c *Core) SetAccountSignature(ctx context.Context, accountID, signatureID int64) error {
	if err := c.store.SetAccountSignature(ctx, accountID, signatureID); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventSignaturesChanged, nil)
	c.emit(EventAccountsChanged, nil)
	return nil
}

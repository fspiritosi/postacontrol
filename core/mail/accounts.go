package mail

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/smtpx"
)

var palette = []string{"#2563eb", "#16a34a", "#ea580c", "#9333ea"}

// parseEmail valida y devuelve la dirección pelada.
func parseEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", errors.New("el correo está vacío")
	}
	a, err := mail.ParseAddress(s)
	if err != nil {
		return "", fmt.Errorf("el correo no es válido: %w", err)
	}
	return a.Address, nil
}

// TestAccount verifica las credenciales contra IMAP y SMTP sin guardar nada.
func (c *Core) TestAccount(ctx context.Context, in model.AccountInput) error {
	user := in.Username
	if user == "" {
		user = in.Email
	}
	conn, err := imapx.Dial(ctx, in.IMAP, user, in.Password, c.cfg.TLS)
	if err != nil {
		return asUserError(err)
	}
	_ = conn.Close()
	return asUserError(smtpx.Check(ctx, in.SMTP, user, in.Password, c.cfg.TLS))
}

func (c *Core) AddAccount(ctx context.Context, in model.AccountInput) (model.Account, error) {
	addr, err := parseEmail(in.Email)
	if err != nil {
		return model.Account{}, err
	}
	in.Email = addr
	if in.Username == "" {
		in.Username = addr
	}
	if in.DisplayName == "" {
		in.DisplayName = addr[:strings.LastIndex(addr, "@")]
	}
	if err := c.TestAccount(ctx, in); err != nil {
		return model.Account{}, err
	}
	existing, err := c.store.ListAccounts(ctx)
	if err != nil {
		return model.Account{}, err
	}
	acc := in.Account
	acc.Color = palette[len(existing)%len(palette)]
	acc.SaveSentCopy = true
	id, err := c.store.InsertAccount(ctx, acc)
	if err != nil {
		return model.Account{}, err
	}
	acc.ID = id
	if err := c.cfg.Secrets.Set(addr, in.Password); err != nil {
		_ = c.store.DeleteAccount(context.WithoutCancel(ctx), id)
		return model.Account{}, err
	}
	c.emit(EventAccountsChanged, nil)
	if !c.cfg.SkipInitialSync {
		go func() { _ = c.SyncAccount(context.Background(), id) }()
	}
	return acc, nil
}

func (c *Core) ListAccounts(ctx context.Context) ([]model.Account, error) {
	return c.store.ListAccounts(ctx)
}

// RemoveAccount borra la caché local y el secreto. No toca el servidor.
func (c *Core) RemoveAccount(ctx context.Context, id int64) error {
	acc, err := c.store.GetAccount(ctx, id)
	if err != nil {
		return asUserError(err)
	}
	if err := c.store.DeleteAccount(ctx, id); err != nil {
		return asUserError(err)
	}
	if err := c.cfg.Secrets.Delete(acc.Email); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return err
	}
	c.emit(EventAccountsChanged, nil)
	return nil
}

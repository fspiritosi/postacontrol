package mail

import (
	"context"
	"errors"
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
	a, err := mail.ParseAddress(strings.TrimSpace(s))
	if err != nil || !strings.Contains(a.Address, "@") {
		return "", &UserError{Code: "invalid", Msg: "La dirección de correo no es válida.", Err: err}
	}
	return strings.ToLower(a.Address), nil
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
	if in.Password == "" {
		return model.Account{}, &UserError{Code: "invalid", Msg: "Ingresá la contraseña."}
	}
	existing, err := c.store.ListAccounts(ctx)
	if err != nil {
		return model.Account{}, err
	}
	used := map[string]bool{}
	for _, a := range existing {
		if strings.EqualFold(a.Email, addr) {
			return model.Account{}, &UserError{Code: "duplicate", Msg: "Esa cuenta ya está configurada."}
		}
		used[a.Color] = true
	}
	if in.Username == "" {
		in.Username = addr
	}
	if in.DisplayName == "" {
		in.DisplayName = addr[:strings.LastIndex(addr, "@")]
	}
	if err := c.TestAccount(ctx, in); err != nil {
		return model.Account{}, err
	}
	acc := in.Account
	acc.Color = palette[len(existing)%len(palette)]
	for _, col := range palette {
		if !used[col] {
			acc.Color = col
			break
		}
	}
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
		c.spawn(func(ctx context.Context) { _ = c.SyncAccount(ctx, id) })
	}
	// Si ya hay vigías, la cuenta nueva tiene el suyo (espera a que la
	// primera sincronización cree la Entrada).
	c.startWatcher(id)
	return acc, nil
}

func (c *Core) ListAccounts(ctx context.Context) ([]model.Account, error) {
	return c.store.ListAccounts(ctx)
}

// SetSaveSentCopy cambia si, al enviar, se guarda una copia en Enviados (por
// ejemplo, si el servidor ya la guarda solo).
func (c *Core) SetSaveSentCopy(ctx context.Context, accountID int64, save bool) error {
	if err := c.store.SetSaveSentCopy(ctx, accountID, save); err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	c.emit(EventAccountsChanged, nil)
	return nil
}

// RemoveAccount frena el vigía de la cuenta y borra el secreto y la caché
// local. No toca el servidor.
func (c *Core) RemoveAccount(ctx context.Context, id int64) error {
	acc, err := c.store.GetAccount(ctx, id)
	if err != nil {
		return asUserError(err)
	}
	// Primero el vigía: si reconectara después de borrar el secreto, avisaría
	// que falta la contraseña de una cuenta que se está borrando.
	c.stopWatcher(id)
	if err := c.cfg.Secrets.Delete(acc.Email); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		c.startWatcher(id) // la cuenta queda: vuelve su vigía
		return err
	}
	if err := c.store.DeleteAccount(ctx, id); err != nil {
		return asUserError(err)
	}
	c.dropConn(id)
	c.emit(EventAccountsChanged, nil)
	return nil
}

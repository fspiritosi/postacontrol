package mail

import (
	"context"
	"errors"
	"net/mail"
	"regexp"
	"strings"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/smtpx"
)

var palette = []string{"#2563eb", "#16a34a", "#ea580c", "#9333ea"}

// colorRe es el formato de color que acepta UpdateAccount (#rrggbb).
var colorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

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

// UpdateAccount cambia los datos de la cuenta (todos los de model.Account
// salvo el id). Si cambia la conexión (usuario, IMAP, SMTP o viene una
// contraseña nueva) la prueba antes de guardar, con la contraseña nueva o la
// del llavero; si la prueba falla no cambia nada. La contraseña queda en el
// llavero bajo la dirección de la cuenta: si la dirección cambia se mueve,
// pero la vieja se borra recién cuando el guardado en la base salió bien (si
// falla, el llavero vuelve a como estaba).
//
// Si cambia el buzón (el host IMAP o el usuario) borra la caché local de la
// cuenta y la vuelve a sincronizar de cero; las definiciones de etiquetas y
// los borradores locales quedan. Con cualquier cambio de conexión descarta la
// conexión del pool, levanta la pausa por contraseña rechazada, reinicia el
// vigía y sincroniza en segundo plano.
func (c *Core) UpdateAccount(ctx context.Context, id int64, in model.AccountInput) (model.Account, error) {
	// 1. Validar.
	old, err := c.store.GetAccount(ctx, id)
	if err != nil {
		return model.Account{}, c.ctxOrUserError(ctx, err)
	}
	addr, err := parseEmail(in.Email)
	if err != nil {
		return model.Account{}, err
	}
	if !colorRe.MatchString(in.Color) {
		return model.Account{}, &UserError{Code: "invalid", Msg: "El color no es válido."}
	}
	existing, err := c.store.ListAccounts(ctx)
	if err != nil {
		return model.Account{}, c.ctxOrUserError(ctx, err)
	}
	for _, a := range existing {
		if a.ID != id && strings.EqualFold(a.Email, addr) {
			return model.Account{}, &UserError{Code: "duplicate", Msg: "Esa cuenta ya está configurada."}
		}
	}
	acc := in.Account
	acc.ID, acc.Email = id, addr
	if acc.Username == "" {
		acc.Username = addr
	}
	if acc.DisplayName == "" {
		acc.DisplayName = addr[:strings.LastIndex(addr, "@")]
	}
	oldUser := old.Username
	if oldUser == "" {
		oldUser = old.Email
	}
	mailboxChanged := acc.Username != oldUser || !strings.EqualFold(acc.IMAP.Host, old.IMAP.Host)
	connChanged := mailboxChanged || in.Password != "" ||
		!sameServer(acc.IMAP, old.IMAP) || !sameServer(acc.SMTP, old.SMTP)
	emailChanged := addr != old.Email

	// 2. Probar. La contraseña es la nueva o la del llavero.
	pw := in.Password
	if pw == "" && (connChanged || emailChanged) {
		if pw, err = c.cfg.Secrets.Get(old.Email); err != nil {
			return model.Account{}, &UserError{Code: "invalid", Msg: "Ingresá la contraseña.", Err: err}
		}
	}
	if connChanged {
		if err := c.TestAccount(ctx, model.AccountInput{Account: acc, Password: pw}); err != nil {
			return model.Account{}, err
		}
	}

	// 3. Llavero. Con la misma dirección, se guarda la contraseña de antes
	// para poder volver atrás.
	secretChanged := emailChanged || in.Password != ""
	var restore func()
	if secretChanged {
		if emailChanged {
			restore = func() { _ = c.cfg.Secrets.Delete(addr) }
		} else if prev, err := c.cfg.Secrets.Get(old.Email); err == nil {
			restore = func() { _ = c.cfg.Secrets.Set(old.Email, prev) }
		} else if errors.Is(err, secrets.ErrNotFound) {
			restore = func() { _ = c.cfg.Secrets.Delete(old.Email) }
		}
		if err := c.cfg.Secrets.Set(addr, pw); err != nil {
			return model.Account{}, err
		}
	}

	// 4. Base.
	if err := c.store.UpdateAccount(ctx, acc); err != nil {
		if restore != nil {
			restore()
		}
		return model.Account{}, c.ctxOrUserError(ctx, err)
	}

	// 5. La contraseña ya está bajo la dirección nueva: la vieja sobra. Si no
	// se puede borrar queda una entrada huérfana en el llavero, sin efecto
	// sobre la cuenta, así que no se informa.
	if emailChanged {
		_ = c.cfg.Secrets.Delete(old.Email)
	}

	// 6. Otro buzón: lo que hay en la caché es del viejo. Subir la época
	// antes de borrar hace que una sincronización en curso no escriba nada
	// del buzón viejo después del borrado.
	watcherStopped := false
	if mailboxChanged {
		c.stopWatcher(id)
		watcherStopped = true
		c.bumpAccountEpoch(id)
		c.dropConn(id)
		// La cuenta ya se guardó: el borrado no se corta aunque se cancele ctx.
		if err := c.store.ResetAccountCache(context.WithoutCancel(ctx), id); err != nil {
			c.emit(EventAccountsChanged, nil)
			return model.Account{}, c.ctxOrUserError(ctx, err)
		}
		c.emit(EventFoldersUpdated, map[string]any{"accountId": id})
	}

	// 7. Conexión nueva: como UpdatePassword, más el vigía.
	if connChanged {
		c.dropConn(id)
		c.clearAuthFailure(id)
		if !watcherStopped {
			c.stopWatcher(id)
		}
		c.startWatcher(id)
		if !c.cfg.SkipInitialSync {
			c.spawn(func(ctx context.Context) { _ = c.SyncAccount(ctx, id) })
		}
	}

	// 8. Avisar.
	c.emit(EventAccountsChanged, nil)
	saved, err := c.store.GetAccount(ctx, id)
	if err != nil {
		return acc, nil
	}
	return saved, nil
}

// sameServer dice si dos configuraciones de servidor apuntan a lo mismo (el
// host sin distinguir mayúsculas).
func sameServer(a, b model.ServerSettings) bool {
	return strings.EqualFold(a.Host, b.Host) && a.Port == b.Port && a.Security == b.Security
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

package mail

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

func userErrCode(t *testing.T, err error) string {
	t.Helper()
	var ue *UserError
	require.True(t, errors.As(err, &ue), "se esperaba UserError, fue %v", err)
	return ue.Code
}

func TestAddAccount_OK(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	assert.Equal(t, "#2563eb", acc.Color)
	assert.Equal(t, testUser, acc.Username)
	assert.Equal(t, "f", acc.DisplayName)
	assert.True(t, acc.SaveSentCopy)
	pw, err := c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, testPass, pw)
	assert.Contains(t, n.names(), EventAccountsChanged)
	list, err := c.ListAccounts(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "#2563eb", list[0].Color)
}

func TestAddAccount_WrongPasswordPersistsNothing(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	_, err := c.AddAccount(context.Background(), inputFor(imap, smtp, "mala"))
	require.Error(t, err)
	assert.Equal(t, "auth", userErrCode(t, err))
	list, _ := c.ListAccounts(context.Background())
	assert.Empty(t, list)
	_, err = c.cfg.Secrets.Get(testUser)
	assert.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestAddAccount_UntrustedCertIsTLSError(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	c.cfg.TLS = nil
	_, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.Error(t, err)
	assert.Equal(t, "tls", userErrCode(t, err))
	list, _ := c.ListAccounts(context.Background())
	assert.Empty(t, list)
	_, err = c.cfg.Secrets.Get(testUser)
	assert.ErrorIs(t, err, secrets.ErrNotFound)
}

func TestAddAccount_UnreachableIsNetworkError(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	in := inputFor(imap, smtp, testPass)
	in.IMAP.Port = 1
	_, err := c.AddAccount(context.Background(), in)
	require.Error(t, err)
	assert.Equal(t, "network", userErrCode(t, err))
}

func inputWithEmail(c *Core, imap *testenv.IMAP, smtp *testenv.SMTP, email string) model.AccountInput {
	in := inputFor(imap, smtp, testPass)
	in.Email = email
	in.Username = testUser
	return in
}

func TestAddAccount_InvalidInputRejectedBeforeNetwork(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	for _, e := range []string{"", "   ", "no-es-un-correo"} {
		in := inputFor(imap, smtp, testPass)
		in.Email = e
		in.IMAP.Port = 1 // si tocara la red daría "network"
		_, err := c.AddAccount(context.Background(), in)
		require.Error(t, err, "email %q", e)
		assert.Equal(t, "invalid", userErrCode(t, err))
		assert.Equal(t, "La dirección de correo no es válida.", err.Error())
	}
	_, err := c.AddAccount(context.Background(), inputFor(imap, smtp, ""))
	assert.Equal(t, "invalid", userErrCode(t, err))
	assert.Equal(t, "Ingresá la contraseña.", err.Error())
	list, _ := c.ListAccounts(context.Background())
	assert.Empty(t, list)
}

func TestAddAccount_DuplicateRejected(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	_, err := c.AddAccount(context.Background(), inputWithEmail(c, imap, smtp, "Ana@Ej.com"))
	require.NoError(t, err)
	in := inputWithEmail(c, imap, smtp, "ana@ej.COM")
	in.IMAP.Port = 1 // debe fallar antes de la red
	_, err = c.AddAccount(context.Background(), in)
	assert.Equal(t, "duplicate", userErrCode(t, err))
	assert.Equal(t, "Esa cuenta ya está configurada.", err.Error())
	list, _ := c.ListAccounts(context.Background())
	require.Len(t, list, 1)
	assert.Equal(t, "ana@ej.com", list[0].Email)
	pw, err := c.cfg.Secrets.Get("ana@ej.com")
	require.NoError(t, err)
	assert.Equal(t, testPass, pw)
}

func TestAddAccount_ColorCyclesThroughPalette(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	a1, err := c.AddAccount(context.Background(), inputWithEmail(c, imap, smtp, "a@ej.com"))
	require.NoError(t, err)
	a2, err := c.AddAccount(context.Background(), inputWithEmail(c, imap, smtp, "b@ej.com"))
	require.NoError(t, err)
	assert.Equal(t, "#2563eb", a1.Color)
	assert.Equal(t, "#16a34a", a2.Color)
}

func TestAddAccount_ReusesFreedColor(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	a, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, "a@ej.com"))
	require.NoError(t, err)
	b, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, "b@ej.com"))
	require.NoError(t, err)
	assert.Equal(t, "#16a34a", b.Color)
	require.NoError(t, c.RemoveAccount(ctx, a.ID))
	cc, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, "c@ej.com"))
	require.NoError(t, err)
	assert.Equal(t, "#2563eb", cc.Color)
}

type failDeleteSecrets struct{ secrets.Store }

func (failDeleteSecrets) Delete(string) error { return errors.New("llavero bloqueado") }

func TestRemoveAccount_SecretFailureKeepsRow(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	c.cfg.Secrets = failDeleteSecrets{c.cfg.Secrets}
	require.Error(t, c.RemoveAccount(ctx, acc.ID))
	list, _ := c.ListAccounts(ctx)
	assert.Len(t, list, 1)
}

func TestRemoveAccount_ClearsCacheAndSecret(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", []byte("From: a@b.com\r\nSubject: hola\r\n\r\ncuerpo\r\n"))
	acc, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.RemoveAccount(context.Background(), acc.ID))

	list, err := c.ListAccounts(context.Background())
	require.NoError(t, err)
	assert.Empty(t, list)
	_, err = c.cfg.Secrets.Get(testUser)
	assert.ErrorIs(t, err, secrets.ErrNotFound)

	st, err := imap.User.Status("INBOX", &goimap.StatusOptions{NumMessages: true})
	require.NoError(t, err)
	require.NotNil(t, st.NumMessages)
	assert.Equal(t, uint32(1), *st.NumMessages)

	err = c.RemoveAccount(context.Background(), acc.ID)
	assert.Equal(t, "not_found", userErrCode(t, err))
}

func TestSetSaveSentCopy_DisablesSentCopy(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.True(t, acc.SaveSentCopy)
	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	before := len(n.matching(EventAccountsChanged))

	require.NoError(t, c.SetSaveSentCopy(ctx, acc.ID, false))
	assert.Len(t, n.matching(EventAccountsChanged), before+1)
	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.False(t, list[0].SaveSentCopy)

	res, err := c.Send(ctx, model.SendRequest{AccountID: acc.ID, To: []model.Address{{Email: "d@x"}}, Subject: "s", BodyHTML: "<p>x</p>"})
	require.NoError(t, err)
	assert.False(t, res.SentCopySaved)
	assert.Empty(t, res.Warning, "sin copia pedida no hay aviso")
	st, err := imap.User.Status("Sent", &goimap.StatusOptions{NumMessages: true})
	require.NoError(t, err)
	assert.Equal(t, uint32(0), *st.NumMessages)

	assert.Equal(t, "not_found", userErrCode(t, c.SetSaveSentCopy(ctx, acc.ID+99, true)))
}

// editInput arma el pedido de edición con los datos guardados de la cuenta.
func editInput(acc model.Account) model.AccountInput {
	return model.AccountInput{Account: acc}
}

func TestUpdateAccount_NameAndColor(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	before := len(n.matching(EventAccountsChanged))
	foldersBefore := len(n.matching(EventFoldersUpdated))

	in := editInput(acc)
	in.DisplayName, in.Color = "Fabri", "#dc2626"
	got, err := c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "Fabri", got.DisplayName)
	assert.Equal(t, "#dc2626", got.Color)

	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "Fabri", list[0].DisplayName)
	assert.Equal(t, "#dc2626", list[0].Color)
	assert.Equal(t, acc.Email, list[0].Email)
	fs, err := c.ListFolders(ctx, acc.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, fs, "la caché sigue")
	assert.Len(t, n.matching(EventAccountsChanged), before+1)
	assert.Len(t, n.matching(EventFoldersUpdated), foldersBefore)

	// Un nombre vacío se completa con la parte antes de @.
	in.DisplayName = ""
	got, err = c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "f", got.DisplayName)

	_, err = c.UpdateAccount(ctx, acc.ID+99, in)
	assert.Equal(t, "not_found", userErrCode(t, err))
}

func TestUpdateAccount_BadColor(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	before := n.count()

	for _, col := range []string{"rojo", "#dc262", "#dc2626ff", "dc2626", ""} {
		in := editInput(acc)
		in.DisplayName, in.Color = "Otro", col
		_, err = c.UpdateAccount(ctx, acc.ID, in)
		assert.Equal(t, "invalid", userErrCode(t, err), "color %q", col)
		assert.Equal(t, "El color no es válido.", err.Error())
	}
	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, acc, list[0])
	assert.Equal(t, before, n.count())
}

func TestUpdateAccount_DuplicateEmail(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	a, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, "a@ej.com"))
	require.NoError(t, err)
	b, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, "b@ej.com"))
	require.NoError(t, err)

	in := editInput(b)
	in.Email = "A@Ej.com"
	_, err = c.UpdateAccount(ctx, b.ID, in)
	assert.Equal(t, "duplicate", userErrCode(t, err))
	assert.Equal(t, "Esa cuenta ya está configurada.", err.Error())

	// Su propia dirección no cuenta como repetida.
	in = editInput(a)
	in.Email = "A@ej.com"
	got, err := c.UpdateAccount(ctx, a.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "a@ej.com", got.Email)

	in.Email = "no-es-un-correo"
	_, err = c.UpdateAccount(ctx, a.ID, in)
	assert.Equal(t, "La dirección de correo no es válida.", err.Error())
	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, []model.Account{a, b}, list)
}

func TestUpdateAccount_WrongPasswordChangesNothing(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	before := len(n.matching(EventAccountsChanged))

	in := editInput(acc)
	in.DisplayName, in.Password = "Fabri", "mala"
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	assert.Equal(t, "auth", userErrCode(t, err))

	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, acc, list[0])
	pw, err := c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, testPass, pw)
	assert.Len(t, n.matching(EventAccountsChanged), before)
}

func TestUpdateAccount_ConnectionChangeNeedsStoredPassword(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.cfg.Secrets.Delete(testUser))

	in := editInput(acc)
	in.IMAP.Security = model.SecuritySTARTTLS
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	assert.Equal(t, "invalid", userErrCode(t, err))
	assert.Equal(t, "Ingresá la contraseña.", err.Error())
	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, acc, list[0])
}

func TestUpdateAccount_NewPasswordIsStored(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.cfg.Secrets.Set(testUser, "vieja"))
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	c.now = func() time.Time { return now }
	c.noteAuthFailure(acc.ID)

	in := editInput(acc)
	in.Password = testPass
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	pw, err := c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, testPass, pw)
	assert.False(t, c.authPaused(acc.ID), "se levanta la pausa por contraseña rechazada")
}

func TestUpdateAccount_EmailMovesSecret(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	foldersBefore := len(n.matching(EventFoldersUpdated))

	in := editInput(acc)
	in.Email = "nuevo@ej.com"
	in.Username = testUser
	got, err := c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	assert.Equal(t, "nuevo@ej.com", got.Email)
	assert.Equal(t, testUser, got.Username)

	pw, err := c.cfg.Secrets.Get("nuevo@ej.com")
	require.NoError(t, err)
	assert.Equal(t, testPass, pw)
	_, err = c.cfg.Secrets.Get(testUser)
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	fs, err := c.ListFolders(ctx, acc.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, fs, "cambiar solo la dirección no borra la caché")
	assert.Len(t, n.matching(EventFoldersUpdated), foldersBefore)
	require.NoError(t, c.SyncAccount(ctx, acc.ID), "conecta con la contraseña movida")
}

// failAccountUpdates hace fallar todo UPDATE de accounts con un trigger
// creado a mano en la base, como si el guardado fallara después de validar.
func failAccountUpdates(t *testing.T, c *Core) {
	t.Helper()
	db, err := sql.Open("sqlite", c.cfg.DBPath)
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TRIGGER fail_account_update BEFORE UPDATE ON accounts
 BEGIN SELECT RAISE(ABORT, 'guardado roto'); END`)
	require.NoError(t, err)
}

func TestUpdateAccount_StoreFailureRestoresSecret(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	failAccountUpdates(t, c)
	before := len(n.matching(EventAccountsChanged))

	// Dirección nueva: el secreto copiado se borra.
	in := editInput(acc)
	in.Email = "nuevo@ej.com"
	in.Username = testUser
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	require.Error(t, err)
	_, err = c.cfg.Secrets.Get("nuevo@ej.com")
	assert.ErrorIs(t, err, secrets.ErrNotFound)
	pw, err := c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, testPass, pw)

	// Misma dirección con contraseña nueva: vuelve la vieja.
	require.NoError(t, c.cfg.Secrets.Set(testUser, "vieja"))
	in = editInput(acc)
	in.Password = testPass
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	require.Error(t, err)
	pw, err = c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, "vieja", pw)

	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, acc, list[0])
	assert.Len(t, n.matching(EventAccountsChanged), before)
}

func TestUpdateAccount_MailboxChangeResetsCache(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	imap.Deliver(t, "INBOX", []byte("From: a@b.com\r\nSubject: viejo\r\n\r\ncuerpo\r\n"))
	require.NoError(t, imap.User.Create("Viejo", nil))
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	_, err = c.store.SetAccountLabelsLocal(ctx, acc.ID)
	require.NoError(t, err)

	const otro = "otro@ej.com"
	other := imap.AddUser(t, otro, testPass)
	smtp.AddUser(otro, testPass)
	require.NoError(t, other.Create("Nuevo", nil))
	before := len(n.matching(EventFoldersUpdated))

	in := editInput(acc)
	in.Username = otro
	got, err := c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	assert.Equal(t, otro, got.Username)

	evs := n.matching(EventFoldersUpdated)
	require.Len(t, evs, before+1)
	assert.Equal(t, acc.ID, evs[before]["accountId"])
	fs, err := c.ListFolders(ctx, acc.ID)
	require.NoError(t, err)
	assert.Empty(t, fs, "se borró la caché del buzón viejo")
	local, err := c.store.AccountLabelsLocal(ctx, acc.ID)
	require.NoError(t, err)
	assert.False(t, local)
	pw, err := c.cfg.Secrets.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, testPass, pw)

	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	fs, err = c.ListFolders(ctx, acc.ID)
	require.NoError(t, err)
	names := []string{}
	var inbox model.Folder
	for _, f := range fs {
		names = append(names, f.Name)
		if f.Role == "inbox" {
			inbox = f
		}
	}
	assert.ElementsMatch(t, []string{"INBOX", "Sent", "Nuevo"}, names)
	ms, err := c.ListMessages(ctx, inbox.ID, model.ListQuery{})
	require.NoError(t, err)
	assert.Empty(t, ms, "la Entrada del buzón nuevo está vacía")
}

func TestUpdateAccount_SignatureAndSaveSent(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	sig, err := c.SaveSignature(ctx, model.Signature{Name: "Trabajo", HTML: "<p>Fabri</p>"})
	require.NoError(t, err)

	in := editInput(acc)
	in.DefaultSignatureID, in.SaveSentCopy = sig.ID, false
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, sig.ID, list[0].DefaultSignatureID)
	assert.False(t, list[0].SaveSentCopy)

	in.DefaultSignatureID, in.SaveSentCopy = 0, true
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	list, err = c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), list[0].DefaultSignatureID)
	assert.True(t, list[0].SaveSentCopy)
}

func TestSyncAccount_StaleEpochWritesNothing(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	// La cuenta cambia de buzón entre el LIST y el guardado de las carpetas.
	c.afterFolderList = func(id int64) { c.bumpAccountEpoch(id) }

	require.NoError(t, c.SyncAccount(ctx, acc.ID), "el corte no se informa")
	fs, err := c.ListFolders(ctx, acc.ID)
	require.NoError(t, err)
	assert.Empty(t, fs)
	assert.Empty(t, n.matching(EventSyncError))
	assert.Empty(t, n.matching(EventFoldersUpdated))

	// Con la época al día, sincroniza normalmente.
	c.afterFolderList = nil
	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	fs, err = c.ListFolders(ctx, acc.ID)
	require.NoError(t, err)
	assert.NotEmpty(t, fs)
}

func TestSyncFolder_StaleEpochStopsBetweenBatches(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(ctx, acc.ID))
	for i := range headerBatch + 1 {
		imap.Deliver(t, "INBOX", []byte(fmt.Sprintf("From: a@b.com\r\nSubject: %d\r\n\r\ncuerpo\r\n", i)))
	}
	inbox := inboxOf(t, c, acc.ID)
	// La cuenta cambia de buzón después del primer lote.
	n.setHook(func(event string, _ any) {
		if event == EventSyncProgress {
			c.bumpAccountEpoch(acc.ID)
		}
	})
	updatedBefore := len(n.matching(EventFolderUpdated))

	require.NoError(t, c.SyncFolder(ctx, inbox), "el corte no se informa")
	ms, err := c.ListMessages(ctx, inbox, model.ListQuery{Limit: 1000})
	require.NoError(t, err)
	assert.Len(t, ms, headerBatch, "el segundo lote no se escribió")
	assert.Len(t, n.matching(EventSyncProgress), 1)
	assert.Len(t, n.matching(EventFolderUpdated), updatedBefore)
	assert.Empty(t, n.matching(EventSyncError))
}

func inboxOf(t *testing.T, c *Core, accountID int64) int64 {
	t.Helper()
	fs, err := c.ListFolders(context.Background(), accountID)
	require.NoError(t, err)
	for _, f := range fs {
		if f.Role == "inbox" {
			return f.ID
		}
	}
	t.Fatal("no hay Entrada")
	return 0
}

func accountOrder(t *testing.T, c *Core) []string {
	t.Helper()
	list, err := c.ListAccounts(context.Background())
	require.NoError(t, err)
	out := []string{}
	for _, a := range list {
		out = append(out, a.Email)
	}
	return out
}

func TestReorderAccounts(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	for _, e := range []string{"a@ej.com", "b@ej.com", "c@ej.com"} {
		_, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, e))
		require.NoError(t, err)
	}
	before := len(n.matching(EventAccountsChanged))
	require.NoError(t, c.ReorderAccounts(ctx, []int64{3, 1, 2}))
	assert.Equal(t, []string{"c@ej.com", "a@ej.com", "b@ej.com"}, accountOrder(t, c))
	assert.Len(t, n.matching(EventAccountsChanged), before+1)

	_, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, "d@ej.com"))
	require.NoError(t, err)
	assert.Equal(t, []string{"c@ej.com", "a@ej.com", "b@ej.com", "d@ej.com"}, accountOrder(t, c))
}

func TestReorderAccounts_Stale(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	for _, e := range []string{"a@ej.com", "b@ej.com", "c@ej.com"} {
		_, err := c.AddAccount(ctx, inputWithEmail(c, imap, smtp, e))
		require.NoError(t, err)
	}
	before := len(n.matching(EventAccountsChanged))
	for _, ids := range [][]int64{{1, 2}, {1, 1, 2}, {1, 2, 99}} {
		err := c.ReorderAccounts(ctx, ids)
		assert.Equal(t, "invalid", userErrCode(t, err))
		assert.Equal(t, "La lista de cuentas cambió. Probá de nuevo.", err.Error())
	}
	assert.Equal(t, []string{"a@ej.com", "b@ej.com", "c@ej.com"}, accountOrder(t, c))
	assert.Len(t, n.matching(EventAccountsChanged), before)
}

// failingSecrets es un llavero en memoria cuyo Get o Delete pueden fallar con
// un error que no es ErrNotFound (llavero bloqueado, D-Bus caído).
type failingSecrets struct {
	secrets.Store
	getErr, deleteErr error
}

func (f *failingSecrets) Get(email string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.Store.Get(email)
}

func (f *failingSecrets) Delete(email string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.Store.Delete(email)
}

func TestUpdateAccount_UnreadableKeyringKeepsOldPassword(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	mem := c.cfg.Secrets
	require.NoError(t, mem.Set(testUser, "vieja"))
	c.cfg.Secrets = &failingSecrets{Store: mem, getErr: errors.New("llavero bloqueado")}
	before := len(n.matching(EventAccountsChanged))

	// Misma dirección con contraseña nueva: sin poder leer la vieja no hay
	// cómo volver atrás, así que no se toca el llavero.
	in := editInput(acc)
	in.Password = testPass
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	require.Error(t, err)
	var ue *UserError
	require.True(t, errors.As(err, &ue), "se esperaba UserError, fue %v", err)
	assert.NotContains(t, err.Error(), testPass)

	pw, err := mem.Get(testUser)
	require.NoError(t, err)
	assert.Equal(t, "vieja", pw)
	list, err := c.ListAccounts(ctx)
	require.NoError(t, err)
	assert.Equal(t, acc, list[0])
	assert.Len(t, n.matching(EventAccountsChanged), before)
}

func TestUpdateAccount_OldSecretDeleteFailureIsLogged(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	mem := c.cfg.Secrets
	c.cfg.Secrets = &failingSecrets{Store: mem, deleteErr: errors.New("llavero bloqueado")}

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	in := editInput(acc)
	in.Email = "nuevo@ej.com"
	in.Username = testUser
	got, err := c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err, "no poder borrar el secreto viejo no corta la edición")
	assert.Equal(t, "nuevo@ej.com", got.Email)
	assert.Contains(t, logs.String(), "cuentas: no se pudo borrar el secreto viejo")
	assert.Contains(t, logs.String(), "llavero bloqueado")
	assert.NotContains(t, logs.String(), testPass)
}

func TestUpdateAccount_OldSecretAlreadyGoneIsNotLogged(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, err := c.AddAccount(ctx, inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	mem := c.cfg.Secrets
	c.cfg.Secrets = &failingSecrets{Store: mem, deleteErr: secrets.ErrNotFound}

	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	in := editInput(acc)
	in.Email = "nuevo@ej.com"
	in.Username = testUser
	_, err = c.UpdateAccount(ctx, acc.ID, in)
	require.NoError(t, err)
	assert.NotContains(t, logs.String(), "secreto viejo")
}

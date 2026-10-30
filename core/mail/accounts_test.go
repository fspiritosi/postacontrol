package mail

import (
	"context"
	"errors"
	"testing"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/secrets"
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

func TestAddAccount_InvalidEmailRejectedBeforeNetwork(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	for _, e := range []string{"", "   ", "no-es-un-correo"} {
		in := inputFor(imap, smtp, testPass)
		in.Email = e
		_, err := c.AddAccount(context.Background(), in)
		assert.Error(t, err, "email %q", e)
	}
	list, _ := c.ListAccounts(context.Background())
	assert.Empty(t, list)
}

func TestAddAccount_ColorCyclesThroughPalette(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	in := inputFor(imap, smtp, testPass)
	a1, err := c.AddAccount(context.Background(), in)
	require.NoError(t, err)
	a2, err := c.AddAccount(context.Background(), in)
	require.NoError(t, err)
	assert.Equal(t, "#2563eb", a1.Color)
	assert.Equal(t, "#16a34a", a2.Color)
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

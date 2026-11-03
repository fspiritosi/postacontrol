package app

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/mail"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

func TestParseMailto_Full(t *testing.T) {
	f, err := parseMailto("MAILTO:a@x.com,Bob%20%3Cb@x.com%3E,malo?to=c@x.com&cc=d@x.com,e@x.com&cc=f@x.com&bcc=g@x.com&subject=Hola%20mundo&body=l1%0D%0Al2%20%3Cb%3E")
	require.NoError(t, err)
	assert.Equal(t, []model.Address{{Email: "a@x.com"}, {Name: "Bob", Email: "b@x.com"}, {Email: "c@x.com"}}, f.To)
	assert.Equal(t, []model.Address{{Email: "d@x.com"}, {Email: "e@x.com"}, {Email: "f@x.com"}}, f.Cc)
	assert.Equal(t, []model.Address{{Email: "g@x.com"}}, f.Bcc)
	assert.Equal(t, "Hola mundo", f.Subject)
	assert.Equal(t, "l1\r\nl2 <b>", f.Body)
}

func TestParseMailto_IgnoresAttach(t *testing.T) {
	f, err := parseMailto("mailto:a@x.com?attach=/etc/passwd&attachment=x&in-reply-to=y")
	require.NoError(t, err)
	assert.Equal(t, []model.Address{{Email: "a@x.com"}}, f.To)
	assert.Empty(t, f.Cc)
	assert.Empty(t, f.Bcc)
	assert.Empty(t, f.Subject)
	assert.Empty(t, f.Body)
}

func TestParseMailto_Rejects(t *testing.T) {
	for _, bad := range []string{"https://x.com", "javascript:alert(1)", "mailto:" + strings.Repeat("a", 9000)} {
		_, err := parseMailto(bad)
		assert.Error(t, err, bad[:min(len(bad), 30)])
	}
}

func TestParseMailto_EmptyIsOK(t *testing.T) {
	f, err := parseMailto("mailto:")
	require.NoError(t, err)
	assert.Empty(t, f.To)
	assert.Empty(t, f.Subject)
}

func TestMailtoInitJSON_Keys(t *testing.T) {
	js, err := mailtoInitJSON(mailtoFields{Body: "a<b\r\nc\nd"}, model.Account{ID: 7, DefaultSignatureID: 3})
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(js), &m))
	keys := []string{"accountId", "to", "cc", "bcc", "subject", "bodyHtml", "quotedHtml", "reply", "attachments", "originalAttachments", "skipOriginalAttachments", "signatureId"}
	assert.Len(t, m, len(keys))
	for _, k := range keys {
		assert.Contains(t, m, k)
	}
	for _, k := range []string{"to", "cc", "bcc", "attachments", "originalAttachments", "skipOriginalAttachments"} {
		assert.IsType(t, []any{}, m[k], k)
	}
	assert.Nil(t, m["reply"])
	assert.EqualValues(t, 3, m["signatureId"])
	assert.EqualValues(t, 7, m["accountId"])
	assert.Equal(t, "a&lt;b<br>c<br>d", m["bodyHtml"])
	assert.Equal(t, "", m["quotedHtml"])
}

func TestPickMailtoAccount(t *testing.T) {
	accs := []model.Account{{ID: 4}, {ID: 9}}
	a, ok := pickMailtoAccount(accs, 9)
	assert.True(t, ok)
	assert.EqualValues(t, 9, a.ID)
	for _, id := range []int64{0, 55} {
		a, ok = pickMailtoAccount(accs, id)
		assert.True(t, ok)
		assert.EqualValues(t, 4, a.ID)
	}
	_, ok = pickMailtoAccount(nil, 1)
	assert.False(t, ok)
}

func TestMailto_ParseThenInitJSON(t *testing.T) {
	f, err := parseMailto("mailto:a@x.com,b@y.com?cc=c@z.com&subject=Hola%20qu%C3%A9%20tal&body=l1%0D%0Al2%3Cb%3E")
	require.NoError(t, err)
	js, err := mailtoInitJSON(f, model.Account{ID: 1})
	require.NoError(t, err)
	var got struct {
		To, Cc, Bcc []model.Address
		Subject     string
		BodyHTML    string `json:"bodyHtml"`
	}
	require.NoError(t, json.Unmarshal([]byte(js), &got))
	assert.Equal(t, []model.Address{{Email: "a@x.com"}, {Email: "b@y.com"}}, got.To)
	assert.Equal(t, []model.Address{{Email: "c@z.com"}}, got.Cc)
	assert.Equal(t, []model.Address{}, got.Bcc)
	assert.Equal(t, "Hola qué tal", got.Subject)
	assert.Equal(t, "l1<br>l2&lt;b&gt;", got.BodyHTML)
}

func TestOpenMailto_SinCuentas(t *testing.T) {
	core, err := mail.New(mail.Config{DBPath: filepath.Join(t.TempDir(), "p.db"), Secrets: secrets.NewMemory()})
	require.NoError(t, err)
	defer core.Close()
	svc := &MailService{core: core}
	err = svc.OpenMailto(context.Background(), "mailto:a@x.com", 0)
	var ue *mail.UserError
	require.True(t, errors.As(err, &ue))
	assert.Equal(t, "Primero agregá una cuenta.", ue.Msg)
}

func TestMailtoArgs(t *testing.T) {
	long := "mailto:" + strings.Repeat("a", 9000)
	got := MailtoArgs([]string{"--hidden", "MAILTO:a@x.com", "x", long})
	if len(got) != 1 || got[0] != "MAILTO:a@x.com" {
		t.Fatalf("MailtoArgs = %v", got)
	}
}

func TestOpenMailto_SinNucleo(t *testing.T) {
	svc := &MailService{}
	var err error
	require.NotPanics(t, func() { err = svc.OpenMailto(context.Background(), "mailto:a@x.com", 0) })
	var ue *mail.UserError
	require.True(t, errors.As(err, &ue), "se esperaba UserError, fue %v", err)
	assert.Equal(t, "unavailable", ue.Code)
}

// newCoreConCuenta arma un núcleo con una cuenta contra servidores de prueba.
func newCoreConCuenta(t *testing.T) *mail.Core {
	t.Helper()
	const user, pass = "f@ej.com", "secreta"
	tlsEnv := testenv.NewTLS(t)
	imap := testenv.StartIMAP(t, tlsEnv, user, pass)
	smtp := testenv.StartSMTP(t, tlsEnv, user, pass)
	core, err := mail.New(mail.Config{
		DBPath:          filepath.Join(t.TempDir(), "p.db"),
		Secrets:         secrets.NewMemory(),
		TLS:             tlsEnv.Client,
		SkipInitialSync: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = core.Close() })
	_, err = core.AddAccount(context.Background(), model.AccountInput{
		Account:  model.Account{Email: user, IMAP: imap.Settings, SMTP: smtp.Settings},
		Password: pass,
	})
	require.NoError(t, err)
	return core
}

func TestOpenMailtoArgs_EsperaAlArranque(t *testing.T) {
	svc := NewMailService()
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.OpenMailtoArgs([]string{"--hidden", "mailto:a@x.com?subject=Hola"})
	}()
	select {
	case <-done:
		t.Fatal("OpenMailtoArgs no esperó al arranque")
	case <-time.After(50 * time.Millisecond):
	}

	// Arranque: como en main, primero el núcleo y los redactores, después la señal.
	h := newComposeHarness(time.Second)
	svc.core = newCoreConCuenta(t)
	svc.SetComposeWindows(h.r)
	svc.MarkStarted()
	svc.MarkStarted() // otra vez no hace nada

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OpenMailtoArgs no siguió después del arranque")
	}
	require.Equal(t, 1, h.created())
	init, err := h.r.Init(1)
	require.NoError(t, err)
	var got mailtoInit
	require.NoError(t, json.Unmarshal([]byte(init), &got))
	assert.Equal(t, []model.Address{{Email: "a@x.com"}}, got.To)
	assert.Equal(t, "Hola", got.Subject)
}

func TestOpenMailtoArgs_SinNucleoNoEntraEnPanico(t *testing.T) {
	svc := NewMailService()
	svc.MarkStarted()
	require.NotPanics(t, func() { svc.OpenMailtoArgs([]string{"mailto:a@x.com"}) })
}

func TestOpenMailtoArgs_SinMailtoNoEspera(t *testing.T) {
	svc := NewMailService() // nunca arranca
	svc.OpenMailtoArgs([]string{"--hidden"})
}

func TestParseMailto_PlusIsLiteral(t *testing.T) {
	f, err := parseMailto("mailto:?cc=a+tag@x.com&subject=1+1")
	require.NoError(t, err)
	assert.Empty(t, f.To)
	assert.Equal(t, []model.Address{{Email: "a+tag@x.com"}}, f.Cc)
	assert.Equal(t, "1+1", f.Subject)
}

func TestParseMailto_KeysIgnoreCaseAndBadEscapes(t *testing.T) {
	f, err := parseMailto("mailto:a@x.com?SUBJECT=Hola%20che&Body=x%ZZ&body=bien&CC=b@x.com&=nada&solo")
	require.NoError(t, err)
	assert.Equal(t, "Hola che", f.Subject)
	assert.Equal(t, "bien", f.Body, "el valor mal escapado se saltea")
	assert.Equal(t, []model.Address{{Email: "b@x.com"}}, f.Cc)
}

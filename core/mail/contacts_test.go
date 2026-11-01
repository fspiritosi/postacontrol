package mail

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/testenv"
)

func mustSave(t *testing.T, c *Core, ct model.Contact) model.Contact {
	t.Helper()
	saved, err := c.SaveContact(context.Background(), ct)
	require.NoError(t, err)
	return saved
}

func TestSaveContact_ValidatesAndNormalizes(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()

	_, err := c.SaveContact(ctx, model.Contact{})
	ue := userErr(t, err)
	assert.Equal(t, "invalid", ue.Code)
	assert.Equal(t, "Poné un nombre o un correo.", ue.Msg)

	_, err = c.SaveContact(ctx, model.Contact{Emails: []string{"no-es-correo"}})
	ue = userErr(t, err)
	assert.Equal(t, "invalid", ue.Code)
	assert.Equal(t, "«no-es-correo» no es un correo válido.", ue.Msg)

	_, err = c.SaveContact(ctx, model.Contact{Emails: []string{"Ana <a@x.com>"}})
	assert.Equal(t, "invalid", userErr(t, err).Code)
	assert.Zero(t, n.count(), "los rechazos no emiten eventos")

	saved, err := c.SaveContact(ctx, model.Contact{
		Name: " Ana ", Emails: []string{" a@x.com ", "A@X.com", ""}, Phones: []string{" ", ""},
	})
	require.NoError(t, err)
	assert.NotZero(t, saved.ID)
	assert.Equal(t, "Ana", saved.Name)
	assert.Equal(t, []string{"a@x.com"}, saved.Emails)
	assert.Equal(t, []string{}, saved.Phones)
	assert.Equal(t, []string{EventContactsChanged}, n.names())
}

func TestSaveContact_EmailUniqueIgnoringCase(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	mustSave(t, c, model.Contact{Name: "Ana", Emails: []string{"juan@x.com"}})
	_, err := c.SaveContact(context.Background(), model.Contact{Name: "Otro", Emails: []string{"Juan@X.com"}})
	ue := userErr(t, err)
	assert.Equal(t, "duplicate", ue.Code)
	assert.Equal(t, "«Juan@X.com» ya está en la ficha de Ana.", ue.Msg)
}

func TestListContacts_FiltersFoldedAndSorts(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	mustSave(t, c, model.Contact{Name: "Ñandú"})
	mustSave(t, c, model.Contact{Name: "ana"})
	mustSave(t, c, model.Contact{Name: "José", Company: "Acme", Phones: []string{"11 5555-1234"}})
	mustSave(t, c, model.Contact{Emails: []string{"zz@x.com"}})

	names := func(q string) []string {
		list, err := c.ListContacts(ctx, q)
		require.NoError(t, err)
		out := []string{}
		for _, ct := range list {
			out = append(out, ct.Name+"|"+fmt.Sprint(ct.Emails))
		}
		return out
	}
	assert.Equal(t, []string{"ana|[]", "José|[]", "Ñandú|[]", "|[zz@x.com]"}, names(""))
	assert.Equal(t, []string{"José|[]"}, names("acme"))
	assert.Equal(t, []string{"Ñandú|[]"}, names("NAND"))
	assert.Equal(t, []string{"|[zz@x.com]"}, names("ZZ@x"))
	assert.Equal(t, []string{"José|[]"}, names("555-12"))
	assert.Equal(t, []string{"José|[]"}, names("1155551234"))
	assert.Empty(t, names("nada"))
}

func TestAddSenderToContacts_CreatesOrReturnsExisting(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	first, err := c.AddSenderToContacts(ctx, model.Address{Name: " Ana ", Email: "ana@x.com"})
	require.NoError(t, err)
	assert.Equal(t, "Ana", first.Name)
	assert.Equal(t, []string{"ana@x.com"}, first.Emails)
	assert.Equal(t, []string{EventContactsChanged}, n.names())

	again, err := c.AddSenderToContacts(ctx, model.Address{Name: "Otra", Email: "Ana@X.com"})
	require.NoError(t, err)
	assert.Equal(t, first, again)
	assert.Len(t, n.names(), 1, "la segunda llamada no emite")
}

func TestContactForEmail_NilWhenMissing(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	got, err := c.ContactForEmail(ctx, "nadie@x.com")
	require.NoError(t, err)
	assert.Nil(t, got)

	saved := mustSave(t, c, model.Contact{Name: "Ana", Emails: []string{"ana@x.com"}})
	got, err = c.ContactForEmail(ctx, "ANA@x.com")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, saved.ID, got.ID)
}

func TestGetAndDeleteContact(t *testing.T) {
	c, _, _, n := newTestCore(t)
	ctx := context.Background()
	_, err := c.GetContact(ctx, 99)
	ue := userErr(t, err)
	assert.Equal(t, "not_found", ue.Code)
	assert.Equal(t, msgNotFound, ue.Msg)

	saved := mustSave(t, c, model.Contact{Name: "Ana"})
	got, err := c.GetContact(ctx, saved.ID)
	require.NoError(t, err)
	assert.Equal(t, saved, got)

	require.NoError(t, c.DeleteContact(ctx, saved.ID))
	assert.Equal(t, []string{EventContactsChanged, EventContactsChanged}, n.names())
	_, err = c.GetContact(ctx, saved.ID)
	assert.Equal(t, "not_found", userErr(t, err).Code)
}

func emails(s []model.Suggestion) []string {
	out := []string{}
	for _, x := range s {
		out = append(out, x.Email)
	}
	return out
}

func record(t *testing.T, c *Core, times int, addrs ...model.Address) {
	t.Helper()
	for i := 0; i < times; i++ {
		require.NoError(t, c.store.RecordRecipients(context.Background(), addrs, time.Unix(1000+int64(i), 0)))
	}
}

func TestSuggest_FoldsAccentsAndCase(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	mustSave(t, c, model.Contact{Name: "Peña", Emails: []string{"pp@x.com"}})
	mustSave(t, c, model.Contact{Name: "José", Emails: []string{"js@x.com"}})
	for _, q := range []string{"pena", "PEÑ"} {
		got, err := c.SuggestRecipients(context.Background(), q, nil)
		require.NoError(t, err)
		require.Len(t, got, 1, q)
		assert.Equal(t, "pp@x.com", got[0].Email)
		assert.Equal(t, "Peña", got[0].Name)
		assert.NotZero(t, got[0].ContactID)
	}
	got, err := c.SuggestRecipients(context.Background(), "JOSE", nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "José", got[0].Name)
}

func TestSuggest_OrderAndLimit(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	mustSave(t, c, model.Contact{Name: "Juan Pérez", Emails: []string{"jp@x.com"}})
	record(t, c, 50, model.Address{Email: "juana@y.com"})
	record(t, c, 2, model.Address{Email: "juanb@y.com"})
	// Más usado que juanb: solo queda atrás porque no empieza con "juan".
	record(t, c, 10, model.Address{Email: "x-juan@z.com"})
	got, err := c.SuggestRecipients(ctx, "juan", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"jp@x.com", "juana@y.com", "juanb@y.com", "x-juan@z.com"}, emails(got))

	// Cualquier palabra del nombre cuenta como prefijo.
	mustSave(t, c, model.Contact{Name: "Maria Perez", Emails: []string{"mp@x.com"}})
	mustSave(t, c, model.Contact{Name: "Alperez", Emails: []string{"al@x.com"}})
	got, err = c.SuggestRecipients(ctx, "perez", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"jp@x.com", "mp@x.com", "al@x.com"}, emails(got))

	for i := 0; i < 12; i++ {
		record(t, c, 1, model.Address{Email: fmt.Sprintf("lim%02d@y.com", i)})
	}
	got, err = c.SuggestRecipients(ctx, "lim", nil)
	require.NoError(t, err)
	assert.Len(t, got, 8)
}

// Una dirección escrita entera va primero, antes que los contactos que solo la
// contienen: así Enter no la cambia por la de otra persona.
func TestSuggest_ExactEmailFirst(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	mustSave(t, c, model.Contact{Name: "Mariana", Emails: []string{"mariana@x.com"}})
	record(t, c, 1, model.Address{Email: "ana@x.com"})
	got, err := c.SuggestRecipients(ctx, " ANA@x.com ", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"ana@x.com", "mariana@x.com"}, emails(got))

	// También entre contactos: el exacto va antes que el que lo extiende, aunque
	// por nombre vaya después.
	mustSave(t, c, model.Contact{Name: "Ana", Emails: []string{"bea@x.com.ar"}})
	mustSave(t, c, model.Contact{Name: "Zoe", Emails: []string{"bea@x.com"}})
	got, err = c.SuggestRecipients(ctx, "bea@x.com", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"bea@x.com", "bea@x.com.ar"}, emails(got))
}

func TestSuggest_SkipsContactDuplicatesAndExcluded(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	ctx := context.Background()
	saved := mustSave(t, c, model.Contact{Name: "Ana", Emails: []string{"ana@x.com"}})
	record(t, c, 3, model.Address{Name: "Ana", Email: "ANA@x.com"}, model.Address{Email: "anita@y.com"}, model.Address{Email: "anton@y.com"})

	got, err := c.SuggestRecipients(ctx, "an", []string{"Anton@Y.com"})
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "ana@x.com", got[0].Email)
	assert.Equal(t, saved.ID, got[0].ContactID)
	assert.Equal(t, "anita@y.com", got[1].Email)
	assert.Zero(t, got[1].ContactID)

	got, err = c.SuggestRecipients(ctx, "  ", nil)
	require.NoError(t, err)
	assert.Equal(t, []model.Suggestion{}, got)
}

func TestSend_RecordsRecipients(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, _ := setupSynced(t, imap, smtp, c)

	_, err := c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs("a@x"), Cc: addrs("b@x"), Bcc: addrs("c@x"),
		Subject: "Hola", BodyHTML: "<p>x</p>",
	})
	require.NoError(t, err)
	rs, err := c.store.ListRecipients(ctx)
	require.NoError(t, err)
	got := map[string]int{}
	for _, r := range rs {
		got[r.Email] = r.Times
	}
	assert.Equal(t, map[string]int{"a@x": 1, "b@x": 1, "c@x": 1}, got)
}

func TestSend_FailureRecordsNoRecipients(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	ctx := context.Background()
	acc, _ := setupSynced(t, imap, smtp, c)
	_, err := c.Send(ctx, model.SendRequest{
		AccountID: acc.ID, To: addrs(testenv.RejectedRecipient), Subject: "Fallido", BodyHTML: "<p>x</p>",
	})
	require.Error(t, err)
	rs, err := c.store.ListRecipients(ctx)
	require.NoError(t, err)
	assert.Empty(t, rs)
}

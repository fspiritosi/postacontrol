package mail

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageSource_ViewAndSave(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "fuente"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()
	m := listAll(t, c, inboxID)[0]

	src, err := c.MessageSource(ctx, m.ID)
	require.NoError(t, err)
	assert.Contains(t, src.Text, "Subject: fuente\r\n")
	assert.Contains(t, src.Text, "Message-ID: <fuente@ej.com>")
	assert.False(t, src.Truncated)
	assert.False(t, listAll(t, c, inboxID)[0].Seen, "verlo no lo marca leído")

	dest := filepath.Join(t.TempDir(), "fuente.eml")
	require.NoError(t, c.SaveMessageSource(ctx, m.ID, dest))
	data, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "From: Bea <bea@ej.com>"))
}

func TestMessageSource_LongIsTruncatedForViewing(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	big := "From: a@ej.com\r\nSubject: grande\r\nMessage-ID: <g@ej.com>\r\n\r\n" + strings.Repeat("x", maxSourceView+100) + "\r\n"
	imap.Deliver(t, "INBOX", []byte(big))
	_, inboxID := setupSynced(t, imap, smtp, c)

	src, err := c.MessageSource(context.Background(), listAll(t, c, inboxID)[0].ID)
	require.NoError(t, err)
	assert.True(t, src.Truncated)
	assert.Len(t, src.Text, maxSourceView)
}

func TestEmlFilename(t *testing.T) {
	assert.Equal(t, "Factura marzo.eml", emlFilename("Factura marzo"))
	assert.Equal(t, "Re- a_b.eml", emlFilename("Re: a/b"))
	assert.Equal(t, "correo.eml", emlFilename("   "))
}

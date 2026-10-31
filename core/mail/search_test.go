package mail

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func TestSearch_ReturnsPending(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", datedMail("presupuesto", time.Now(), "hola"))
	imap.Deliver(t, "INBOX", datedMail("otro", time.Now(), "hola"))
	_, inboxID := setupSynced(t, imap, smtp, c)

	res, err := c.Search(context.Background(), model.SearchQuery{Text: "presupuesto"})
	require.NoError(t, err)
	require.Len(t, res.Messages, 1)
	assert.Equal(t, "presupuesto", res.Messages[0].Subject)
	assert.Equal(t, 2, res.Pending)

	res, err = c.Search(context.Background(), model.SearchQuery{Text: "presupuesto", FolderID: inboxID})
	require.NoError(t, err)
	assert.Equal(t, 2, res.Pending)
}

func TestSearchServer_FindsUnindexedBody(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", datedMail("uno", time.Now(), "nada"))
	imap.Deliver(t, "INBOX", datedMail("dos", time.Now(), "adjunto la cotizacion del mes"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	ctx := context.Background()

	local, err := c.Search(ctx, model.SearchQuery{Text: "cotizacion"})
	require.NoError(t, err)
	assert.Empty(t, local.Messages)

	// Un mensaje nuevo en el servidor que la caché todavía no conoce.
	imap.Deliver(t, "INBOX", datedMail("tres", time.Now(), "otra cotizacion"))
	ms, err := c.SearchServer(ctx, inboxID, "cotizacion")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dos", "tres"}, subjects(ms))
}

func TestSearchServer_NonASCII(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", datedMail("uno", time.Now(), "nada"))
	imap.Deliver(t, "INBOX", datedMail("dos", time.Now(), "la cotización"))
	_, inboxID := setupSynced(t, imap, smtp, c)
	ms, err := c.SearchServer(context.Background(), inboxID, "cotización")
	require.NoError(t, err)
	assert.Equal(t, []string{"dos"}, subjects(ms))
}

func TestSearchServer_EmptyTextInvalid(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	_, inboxID := setupSynced(t, imap, smtp, c)
	_, err := c.SearchServer(context.Background(), inboxID, "   ")
	var ue *UserError
	require.ErrorAs(t, err, &ue)
	assert.Equal(t, "invalid", ue.Code)
}

func TestListThreads_DefaultLimit(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	for i := range 150 {
		imap.Deliver(t, "INBOX", datedMail(fmt.Sprintf("h-%d", i), time.Now(), "hola"))
	}
	_, inboxID := setupSynced(t, imap, smtp, c)
	ts, err := c.ListThreads(context.Background(), inboxID, model.ListQuery{})
	require.NoError(t, err)
	assert.Len(t, ts, 100)

	msgs, err := c.GetThread(context.Background(), inboxID, ts[0].ThreadRoot)
	require.NoError(t, err)
	assert.Len(t, msgs, 1)
}

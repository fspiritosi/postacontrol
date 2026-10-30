package mail

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	goimap "github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver/imapmemserver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/imapx"
	"postacontrol/core/model"
	"postacontrol/core/testenv"
)

func rawMail(from, subject string) []byte {
	return []byte("From: " + from + "\r\nTo: " + testUser + "\r\nSubject: " + subject +
		"\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\nMessage-ID: <" + subject + "@ej.com>\r\n" +
		"Content-Type: text/plain; charset=utf-8\r\n\r\nhola\r\n")
}

func deliverN(t *testing.T, imap *testenv.IMAP, n int, prefix string) {
	t.Helper()
	for i := 1; i <= n; i++ {
		imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", fmt.Sprintf("%s-%d", prefix, i)))
	}
}

// setupSynced crea el núcleo, agrega la cuenta y hace un SyncAccount. Devuelve
// el ID de INBOX.
func setupSynced(t *testing.T, imap *testenv.IMAP, smtp *testenv.SMTP, c *Core) (model.Account, int64) {
	t.Helper()
	acc, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.NoError(t, err)
	require.NoError(t, c.SyncAccount(context.Background(), acc.ID))
	return acc, folderID(t, c, acc.ID, "INBOX")
}

func folderID(t *testing.T, c *Core, accountID int64, name string) int64 {
	t.Helper()
	fs, err := c.ListFolders(context.Background(), accountID)
	require.NoError(t, err)
	for _, f := range fs {
		if f.Name == name {
			return f.ID
		}
	}
	t.Fatalf("no está la carpeta %s en %v", name, fs)
	return 0
}

func listAll(t *testing.T, c *Core, folderID int64) []model.MessageSummary {
	t.Helper()
	ms, err := c.ListMessages(context.Background(), folderID, model.ListQuery{Limit: 10000})
	require.NoError(t, err)
	return ms
}

func subjects(ms []model.MessageSummary) []string {
	out := []string{}
	for _, m := range ms {
		out = append(out, m.Subject)
	}
	return out
}

func TestSyncAccount_FoldersAndInbox(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	day := func(d int) time.Time { return time.Date(2026, 10, d, 12, 0, 0, 0, time.UTC) }
	// Las fechas no siguen el orden de los UID: la lista se ordena por fecha.
	imap.DeliverAt(t, "INBOX", rawMail("Bea <bea@ej.com>", "uno"), day(2))
	imap.DeliverAt(t, "INBOX", rawMail("Ciro <ciro@ej.com>", "dos"), day(4), goimap.FlagSeen)
	imap.DeliverAt(t, "INBOX", rawMail("Dana <dana@ej.com>", "tres"), day(3))

	acc, inboxID := setupSynced(t, imap, smtp, c)

	fs, err := c.ListFolders(context.Background(), acc.ID)
	require.NoError(t, err)
	by := map[string]model.Folder{}
	for _, f := range fs {
		by[f.Name] = f
	}
	require.Contains(t, by, "INBOX")
	require.Contains(t, by, "Sent")
	assert.Equal(t, model.FolderRole("inbox"), by["INBOX"].Role)
	assert.Equal(t, 3, by["INBOX"].Total)
	assert.Equal(t, 2, by["INBOX"].Unread)
	assert.Equal(t, model.FolderRole("sent"), by["Sent"].Role)

	ms, err := c.ListMessages(context.Background(), inboxID, model.ListQuery{})
	require.NoError(t, err)
	require.Len(t, ms, 3)
	assert.Equal(t, []string{"dos", "tres", "uno"}, subjects(ms))
	assert.Equal(t, model.Address{Name: "Ciro", Email: "ciro@ej.com"}, ms[0].From)
	assert.Equal(t, model.Address{Name: "Bea", Email: "bea@ej.com"}, ms[2].From)
	assert.True(t, ms[0].Seen)
	assert.False(t, ms[1].Seen)
	assert.Equal(t, day(4), ms[0].Date)
	assert.Equal(t, acc.ID, ms[0].AccountID)

	assert.Contains(t, n.matching(EventFoldersUpdated), map[string]any{"accountId": acc.ID})
	assert.Contains(t, n.matching(EventFolderUpdated), map[string]any{"folderId": inboxID})
	assert.Empty(t, n.matching(EventSyncError))

	// La cabecera completa queda en la caché para responder después.
	d, _, err := c.store.GetMessage(context.Background(), ms[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "<dos@ej.com>", d.MessageID)
	assert.Equal(t, []model.Address{{Email: testUser}}, d.To)
}

func TestSyncFolder_Incremental(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 3, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)
	require.Len(t, listAll(t, c, inboxID), 3)

	imap.Deliver(t, "INBOX", rawMail("Bea <bea@ej.com>", "nuevo"))
	require.NoError(t, c.SyncFolder(context.Background(), inboxID))

	ms := listAll(t, c, inboxID)
	assert.Len(t, ms, 4)
	assert.Equal(t, []map[string]any{
		{"folderId": inboxID, "done": 3, "total": 3},
		{"folderId": inboxID, "done": 1, "total": 1},
	}, n.matching(EventSyncProgress), "la segunda pasada solo pide el UID nuevo")

	// Sin novedades no hay lotes, pero la carpeta se actualiza igual.
	require.NoError(t, c.SyncFolder(context.Background(), inboxID))
	assert.Len(t, n.matching(EventSyncProgress), 2)
}

func TestSyncFolder_UpdatesFlagsFromServer(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 2, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	require.Equal(t, 2, folderByID(t, c, acc.ID, inboxID).Unread)

	// Otro cliente marca el UID 1 como leído y destacado.
	sess := imapAsOtherClient(t, imap)
	set := goimap.UIDSetNum(1)
	require.NoError(t, sess.Store(nil, set, &goimap.StoreFlags{
		Op: goimap.StoreFlagsAdd, Silent: true, Flags: []goimap.Flag{goimap.FlagSeen, goimap.FlagFlagged},
	}, nil))

	require.NoError(t, c.SyncFolder(context.Background(), inboxID))
	for _, m := range listAll(t, c, inboxID) {
		if m.UID == 1 {
			assert.True(t, m.Seen)
			assert.True(t, m.Flagged)
		} else {
			assert.False(t, m.Seen)
		}
	}
	assert.Equal(t, 1, folderByID(t, c, acc.ID, inboxID).Unread)
}

// imapAsOtherClient abre una sesión directa sobre el INBOX del servidor, como
// si fuera otro cliente de correo.
func imapAsOtherClient(t *testing.T, imap *testenv.IMAP) *imapmemserver.UserSession {
	t.Helper()
	sess := imapmemserver.NewUserSession(imap.User)
	t.Cleanup(func() { _ = sess.Close() })
	_, err := sess.Select("INBOX", nil)
	require.NoError(t, err)
	return sess
}

func folderByID(t *testing.T, c *Core, accountID, id int64) model.Folder {
	t.Helper()
	fs, err := c.ListFolders(context.Background(), accountID)
	require.NoError(t, err)
	for _, f := range fs {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no está la carpeta %d", id)
	return model.Folder{}
}

func TestSyncFolder_RemovesExpunged(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 3, "m")
	acc, inboxID := setupSynced(t, imap, smtp, c)
	require.Len(t, listAll(t, c, inboxID), 3)

	imap.Expunge(t, "INBOX", 2)
	require.NoError(t, c.SyncFolder(context.Background(), inboxID))

	ms := listAll(t, c, inboxID)
	assert.ElementsMatch(t, []string{"m-1", "m-3"}, subjects(ms))
	assert.Equal(t, 2, folderByID(t, c, acc.ID, inboxID).Total)
}

func TestSyncFolder_UIDValidityChangeResets(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	deliverN(t, imap, 3, "viejo")
	_, inboxID := setupSynced(t, imap, smtp, c)
	_, before, err := c.store.GetFolder(context.Background(), inboxID)
	require.NoError(t, err)

	// El buzón se recrea: los UID 1 y 2 vuelven a existir, pero son otros mensajes.
	newValidity := imap.RecreateMailbox(t, "INBOX")
	require.NotEqual(t, before.UIDValidity, newValidity)
	deliverN(t, imap, 2, "nuevo")
	require.NoError(t, c.SyncFolder(context.Background(), inboxID))

	ms := listAll(t, c, inboxID)
	assert.ElementsMatch(t, []string{"nuevo-1", "nuevo-2"}, subjects(ms))
	_, after, err := c.store.GetFolder(context.Background(), inboxID)
	require.NoError(t, err)
	assert.Equal(t, newValidity, after.UIDValidity)
}

func TestSyncFolder_LargeFolderBatchesNewestFirst(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 450, "m")

	var afterFirst []uint32
	var once sync.Once
	n.setHook(func(event string, data any) {
		if event != EventSyncProgress {
			return
		}
		once.Do(func() {
			fid := data.(map[string]any)["folderId"].(int64)
			afterFirst, _ = c.store.KnownUIDs(context.Background(), fid)
		})
	})
	_, inboxID := setupSynced(t, imap, smtp, c)

	assert.Equal(t, []map[string]any{
		{"folderId": inboxID, "done": 200, "total": 450},
		{"folderId": inboxID, "done": 400, "total": 450},
		{"folderId": inboxID, "done": 450, "total": 450},
	}, n.matching(EventSyncProgress))
	require.Len(t, afterFirst, 200)
	assert.Equal(t, uint32(251), afterFirst[0], "el primer lote son los 200 UID más altos")
	assert.Equal(t, uint32(450), afterFirst[len(afterFirst)-1])

	assert.Len(t, listAll(t, c, inboxID), 450)
	ms, err := c.ListMessages(context.Background(), inboxID, model.ListQuery{})
	require.NoError(t, err)
	assert.Len(t, ms, 100, "Limit 0 devuelve 100")
}

func TestSyncFolder_RecoversAfterConnectionDrop(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 450, "m")
	acc, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.NoError(t, err)

	var once sync.Once
	n.setHook(func(event string, _ any) {
		if event == EventSyncProgress {
			once.Do(imap.Close) // corta el servidor después del primer lote
		}
	})
	syncErr := c.SyncAccount(context.Background(), acc.ID)
	require.Error(t, syncErr)
	assert.Equal(t, "network", userErrCode(t, syncErr))

	inboxID := folderID(t, c, acc.ID, "INBOX")
	_, fsync, err := c.store.GetFolder(context.Background(), inboxID)
	require.NoError(t, err)
	assert.NotZero(t, fsync.UIDValidity, "UIDVALIDITY se guarda apenas se abre la carpeta (R19)")
	assert.Zero(t, fsync.UIDNext, "UIDNext 0 = sincronización incompleta; se escribe solo al final")
	known, err := c.store.KnownUIDs(context.Background(), inboxID)
	require.NoError(t, err)
	assert.Len(t, known, 200, "solo el lote completo, sin filas a medio escribir")
	errs := n.matching(EventSyncError)
	require.Len(t, errs, 1)
	assert.Equal(t, map[string]any{"accountId": acc.ID, "message": syncErr.Error()}, errs[0])
	assert.NotContains(t, fmt.Sprint(errs[0]), testPass)

	n.setHook(nil)
	imap.Restart(t)
	require.NoError(t, c.SyncFolder(context.Background(), inboxID))
	assert.Len(t, listAll(t, c, inboxID), 450)
	progress := n.matching(EventSyncProgress)
	assert.Equal(t, map[string]any{"folderId": inboxID, "done": 200, "total": 250}, progress[len(progress)-2],
		"la segunda pasada es incremental: solo los 250 que faltaban")
	assert.Equal(t, map[string]any{"folderId": inboxID, "done": 250, "total": 250}, progress[len(progress)-1])
	_, fsync, err = c.store.GetFolder(context.Background(), inboxID)
	require.NoError(t, err)
	assert.NotZero(t, fsync.UIDValidity)
	assert.Equal(t, uint32(451), fsync.UIDNext)
}

// R19: si la primera sincronización se corta y después cambia el UIDVALIDITY,
// la caché a medio llenar no se mezcla con los mensajes del buzón nuevo.
func TestSyncFolder_UIDValidityChangeAfterInterruptedFirstSync(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 450, "viejo")
	acc, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
	require.NoError(t, err)

	var once sync.Once
	n.setHook(func(event string, _ any) {
		if event == EventSyncProgress {
			once.Do(imap.Close)
		}
	})
	require.Error(t, c.SyncAccount(context.Background(), acc.ID))
	n.setHook(nil)
	inboxID := folderID(t, c, acc.ID, "INBOX")
	require.Len(t, listAll(t, c, inboxID), 200, "quedaron los UID 251-450 del buzón viejo")

	// El buzón se recrea con otros 450 mensajes: los UID 251-450 vuelven a
	// existir pero son otros.
	imap.RecreateMailbox(t, "INBOX")
	deliverN(t, imap, 450, "nuevo")
	imap.Restart(t)
	require.NoError(t, c.SyncFolder(context.Background(), inboxID))

	ms := listAll(t, c, inboxID)
	assert.Len(t, ms, 450)
	stale := []string{}
	for _, m := range ms {
		if !strings.HasPrefix(m.Subject, "nuevo-") {
			stale = append(stale, m.Subject)
		}
	}
	assert.Empty(t, stale, "filas del buzón viejo")
}

func TestWithConn_RedialsOnceWhenPooledConnDied(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)

	// La conexión del pool queda muerta; el servidor vuelve enseguida.
	imap.Close()
	imap.Restart(t)
	deliverN(t, imap, 1, "otro")

	require.NoError(t, c.SyncFolder(context.Background(), inboxID))
	assert.Len(t, listAll(t, c, inboxID), 2)
	assert.Empty(t, n.matching(EventSyncError))
}

func TestSyncFolder_AuthFailureEmitsSyncError(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	_, inboxID := setupSynced(t, imap, smtp, c)
	c.closeConns() // obliga a un dial nuevo
	require.NoError(t, c.cfg.Secrets.Set(testUser, "cambiada"))

	err := c.SyncFolder(context.Background(), inboxID)
	assert.Equal(t, "auth", userErrCode(t, err))
	errs := n.matching(EventSyncError)
	require.Len(t, errs, 1)
	assert.Equal(t, err.Error(), errs[0]["message"])
	assert.NotContains(t, fmt.Sprint(errs[0]), "cambiada")
}

func TestSyncFolder_UnknownFolder(t *testing.T) {
	c, _, _, _ := newTestCore(t)
	err := c.SyncFolder(context.Background(), 999)
	assert.Equal(t, "not_found", userErrCode(t, err))
}

func TestSync_DecodesLegacyCharsetHeaders(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	imap.Deliver(t, "INBOX", []byte("From: =?iso-8859-1?Q?Pe=F1a?= <p@x.com>\r\nTo: "+testUser+
		"\r\nSubject: =?windows-1252?Q?Informaci=F3n?=\r\nDate: Mon, 02 Jan 2006 15:04:05 +0000\r\n"+
		"Message-ID: <legacy@x.com>\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nhola\r\n"))
	_, inboxID := setupSynced(t, imap, smtp, c)

	ms := listAll(t, c, inboxID)
	require.Len(t, ms, 1)
	assert.Equal(t, "Información", ms[0].Subject)
	assert.Equal(t, model.Address{Name: "Peña", Email: "p@x.com"}, ms[0].From)
}

func TestStartBackgroundSync_Ticks(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	deliverN(t, imap, 1, "m")
	_, inboxID := setupSynced(t, imap, smtp, c)

	ctx, cancel := context.WithCancel(context.Background())
	c.StartBackgroundSync(ctx, 50*time.Millisecond)
	deliverN(t, imap, 1, "otro")
	assert.Eventually(t, func() bool { return len(listAll(t, c, inboxID)) == 2 }, time.Second, 10*time.Millisecond)

	cancel()
	time.Sleep(100 * time.Millisecond) // deja terminar una pasada en curso
	before := len(n.matching(EventFolderUpdated))
	time.Sleep(250 * time.Millisecond)
	assert.Equal(t, before, len(n.matching(EventFolderUpdated)), "después de cancelar no hay más pasadas")
}

// La sincronización inicial que lanza AddAccount está atada al ciclo de vida:
// Close la cancela, espera a que termine y recién después cierra el store.
// Se prueba con Close llegando en dos momentos: entre dos usos de la conexión
// (solo el WaitGroup la protege) y en medio de los lotes de una carpeta.
func TestClose_WaitsForInitialSyncAndStopsIt(t *testing.T) {
	cases := []struct {
		name         string
		trigger      string
		wantProgress int
	}{
		{"entre carpetas y cabeceras", EventFoldersUpdated, 0},
		{"en medio de los lotes", EventSyncProgress, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, imap, smtp, n := newTestCore(t)
			c.cfg.SkipInitialSync = false
			deliverN(t, imap, 450, "m")

			started := make(chan struct{})
			var hookDone atomic.Bool
			var once sync.Once
			n.setHook(func(event string, _ any) {
				if event != tc.trigger {
					return
				}
				once.Do(func() {
					close(started)
					time.Sleep(150 * time.Millisecond) // la sincronización sigue en curso
					hookDone.Store(true)
				})
			})
			_, err := c.AddAccount(context.Background(), inputFor(imap, smtp, testPass))
			require.NoError(t, err)

			select {
			case <-started:
			case <-time.After(5 * time.Second):
				t.Fatal("la sincronización inicial no arrancó")
			}
			t0 := time.Now()
			require.NoError(t, c.Close())
			assert.Less(t, time.Since(t0), 2*time.Second)
			assert.True(t, hookDone.Load(), "Close esperó a la goroutine de sincronización")

			events := n.count()
			time.Sleep(200 * time.Millisecond)
			assert.Equal(t, events, n.count(), "nadie emite (ni toca el store) después de Close")
			assert.Len(t, n.matching(EventSyncProgress), tc.wantProgress, "después de cancelar no hay más lotes")
			assert.Empty(t, n.matching(EventSyncError), "cancelar no es un error para el usuario")

			// Después de Close no se arrancan sincronizaciones nuevas.
			c.StartBackgroundSync(context.Background(), 10*time.Millisecond)
			time.Sleep(50 * time.Millisecond)
			assert.Equal(t, events, n.count())
		})
	}
}

func TestWithConnNoRetry_DoesNotRetry(t *testing.T) {
	c, imap, smtp, n := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)

	// La conexión del pool queda muerta aunque el servidor vuelva enseguida.
	imap.Close()
	imap.Restart(t)

	calls := 0
	err := c.withConnNoRetry(context.Background(), acc.ID, func(_ context.Context, conn *imapx.Conn) error {
		calls++
		_, err := conn.Select("INBOX")
		return err
	})
	assert.Equal(t, "network", userErrCode(t, err))
	assert.Equal(t, 1, calls, "no reconecta ni reintenta")
	assert.Len(t, n.matching(EventSyncError), 1)

	// La conexión rota se descartó: la próxima operación conecta de nuevo.
	calls = 0
	require.NoError(t, c.withConnNoRetry(context.Background(), acc.ID, func(_ context.Context, conn *imapx.Conn) error {
		calls++
		_, err := conn.Select("INBOX")
		return err
	}))
	assert.Equal(t, 1, calls)
}

func TestSync_CancelledContextIsNotUserError(t *testing.T) {
	c, imap, smtp, _ := newTestCore(t)
	acc, _ := setupSynced(t, imap, smtp, c)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.ErrorIs(t, c.SyncAccount(ctx, acc.ID), context.Canceled)

	// Con el núcleo cerrado tampoco aparece "sql: database is closed".
	require.NoError(t, c.Close())
	err := c.SyncAccount(context.Background(), acc.ID)
	assert.ErrorIs(t, err, context.Canceled)
}

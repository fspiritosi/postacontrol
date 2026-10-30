package mail

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
	"postacontrol/core/secrets"
	"postacontrol/core/testenv"
)

const (
	testUser = "f@ej.com"
	testPass = "secreta"
)

type recEvent struct {
	Name string
	Data any
}

type recNotifier struct {
	mu     sync.Mutex
	events []recEvent
}

func (r *recNotifier) Emit(event string, data any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recEvent{event, data})
}

func (r *recNotifier) names() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := []string{}
	for _, e := range r.events {
		out = append(out, e.Name)
	}
	return out
}

func newTestCore(t *testing.T) (*Core, *testenv.IMAP, *testenv.SMTP, *recNotifier) {
	t.Helper()
	tlsEnv := testenv.NewTLS(t)
	imap := testenv.StartIMAP(t, tlsEnv, testUser, testPass)
	smtp := testenv.StartSMTP(t, tlsEnv, testUser, testPass)
	n := &recNotifier{}
	c, err := New(Config{
		DBPath:          t.TempDir() + "/test.db",
		Secrets:         secrets.NewMemory(),
		Notifier:        n,
		TLS:             tlsEnv.Client,
		SkipInitialSync: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c, imap, smtp, n
}

func inputFor(imap *testenv.IMAP, smtp *testenv.SMTP, password string) model.AccountInput {
	return model.AccountInput{
		Account:  model.Account{Email: testUser, IMAP: imap.Settings, SMTP: smtp.Settings},
		Password: password,
	}
}

package taskapp

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// get hace el GET del callback y devuelve el status.
func get(t *testing.T, raw string) int {
	t.Helper()
	resp, err := http.Get(raw)
	require.NoError(t, err)
	defer resp.Body.Close()
	return resp.StatusCode
}

// openWith arma un open falso que parsea la URL, y llama a fn con cb y state.
func openWith(t *testing.T, fn func(cb, state string)) func(string) error {
	return openWithDone(t, fn, nil)
}

func openWithDone(t *testing.T, fn func(cb, state string), fin chan struct{}) func(string) error {
	return func(raw string) error {
		u, err := url.Parse(raw)
		require.NoError(t, err)
		assert.Equal(t, "/cli-auth", u.Path)
		go func() {
			if fin != nil {
				defer close(fin)
			}
			fn(u.Query().Get("cb"), u.Query().Get("state"))
		}()
		return nil
	}
}

func TestLogin_ReturnsToken(t *testing.T) {
	open := openWith(t, func(cb, state string) {
		q := url.Values{"token": {"tok123"}, "expires_at": {"2027-01-02T03:04:05.123Z"}, "state": {state}}
		get(t, cb+"?"+q.Encode())
	})
	g, err := Login(context.Background(), "http://web.test/", open)
	require.NoError(t, err)
	assert.Equal(t, "tok123", g.Token)
	assert.Equal(t, 2027, g.ExpiresAt.Year())
}

func TestLogin_BadExpiresStillSucceeds(t *testing.T) {
	open := openWith(t, func(cb, state string) {
		get(t, cb+"?token=x&expires_at=nope&state="+state)
	})
	g, err := Login(context.Background(), "http://web.test", open)
	require.NoError(t, err)
	assert.Equal(t, "x", g.Token)
	assert.True(t, g.ExpiresAt.IsZero())
}

func TestLogin_WrongStateIgnored(t *testing.T) {
	var bad, bad2, good int
	fin := make(chan struct{})
	open := openWithDone(t, func(cb, state string) {
		bad = get(t, cb+"?token=evil&state=otro")
		bad2 = get(t, cb+"?error=user_cancelled&state=otro")
		good = get(t, cb+"?token=ok&state="+state)
	}, fin)
	g, err := Login(context.Background(), "http://web.test", open)
	require.NoError(t, err)
	assert.Equal(t, "ok", g.Token)
	<-fin
	assert.Equal(t, 400, bad)
	assert.Equal(t, 400, bad2)
	assert.Equal(t, 200, good)
}

func TestLogin_Denied(t *testing.T) {
	open := openWith(t, func(cb, state string) {
		get(t, cb+"?error=user_cancelled&state="+state)
	})
	_, err := Login(context.Background(), "http://web.test", open)
	assert.ErrorIs(t, err, ErrDenied)
}

func TestLogin_NoTokenNoErrorKeepsWaiting(t *testing.T) {
	var st int
	fin := make(chan struct{})
	open := openWithDone(t, func(cb, state string) {
		st = get(t, cb+"?state="+state)
		get(t, cb+"?token=ok&state="+state)
	}, fin)
	g, err := Login(context.Background(), "http://web.test", open)
	require.NoError(t, err)
	assert.Equal(t, "ok", g.Token)
	<-fin
	assert.Equal(t, 400, st)
}

func TestLogin_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var addr string
	open := func(raw string) error {
		u, _ := url.Parse(raw)
		cb, _ := url.Parse(u.Query().Get("cb"))
		addr = cb.Host
		return nil
	}
	_, err := Login(ctx, "http://web.test", open)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	l, lerr := net.Listen("tcp", addr)
	require.NoError(t, lerr)
	l.Close()
}

func TestLogin_OpenErrorReturnsAndCloses(t *testing.T) {
	var addr string
	open := func(raw string) error {
		u, _ := url.Parse(raw)
		cb, _ := url.Parse(u.Query().Get("cb"))
		addr = cb.Host
		return assert.AnError
	}
	_, err := Login(context.Background(), "http://web.test", open)
	assert.ErrorIs(t, err, assert.AnError)
	l, lerr := net.Listen("tcp", addr)
	require.NoError(t, lerr)
	l.Close()
}

func TestLogin_CallbackIsLoopback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	var cb, state string
	open := func(raw string) error {
		u, _ := url.Parse(raw)
		cb, state = u.Query().Get("cb"), u.Query().Get("state")
		assert.True(t, strings.HasPrefix(raw, "http://web.test/cli-auth?"))
		return nil
	}
	_, _ = Login(ctx, "http://web.test/", open)
	assert.True(t, strings.HasPrefix(cb, "http://127.0.0.1:"))
	assert.True(t, strings.HasSuffix(cb, "/callback"))
	assert.Len(t, state, 32)
}

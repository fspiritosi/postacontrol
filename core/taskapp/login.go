package taskapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Grant es el token que entrega Taskapp al autorizar desde el navegador.
type Grant struct {
	Token     string
	ExpiresAt time.Time // cero si Taskapp no mandó una fecha válida
}

var ErrDenied = errors.New("taskapp: autorización rechazada")

const (
	pagOK     = "<!doctype html><meta charset=\"utf-8\"><title>PostaControl</title><p>Listo. Ya podés volver a PostaControl.</p>"
	pagDenied = "<!doctype html><meta charset=\"utf-8\"><title>PostaControl</title><p>No se autorizó la conexión.</p>"
)

// Login escucha en 127.0.0.1:<libre>, abre <webURL>/cli-auth?cb=…&state=… con
// open y espera el callback hasta que ctx venza. El servidor queda cerrado al volver.
func Login(ctx context.Context, webURL string, open func(url string) error) (Grant, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return Grant{}, err
	}
	state := hex.EncodeToString(buf)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Grant{}, err
	}

	type result struct {
		grant Grant
		err   error
	}
	// Con buffer 1 y envío no bloqueante, un segundo callback no cuelga el handler.
	done := make(chan result, 1)
	finish := func(r result) {
		select {
		case done <- r:
		default:
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		q := r.URL.Query()
		if q.Get("state") != state {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if q.Get("error") != "" {
			fmt.Fprint(w, pagDenied)
			finish(result{err: ErrDenied})
			return
		}
		token := q.Get("token")
		if token == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		g := Grant{Token: token}
		if t, perr := time.Parse(time.RFC3339Nano, q.Get("expires_at")); perr == nil {
			g.ExpiresAt = t
		}
		fmt.Fprint(w, pagOK)
		finish(result{grant: g})
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	defer func() {
		// Shutdown deja terminar la respuesta en vuelo; Close es el respaldo.
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if srv.Shutdown(sctx) != nil {
			srv.Close()
		}
		// Si Serve todavía no arrancó, Shutdown no conoce el listener: se cierra acá.
		ln.Close()
	}()

	cb := "http://" + ln.Addr().String() + "/callback"
	q := url.Values{"cb": {cb}, "state": {state}}
	if err := open(strings.TrimRight(webURL, "/") + "/cli-auth?" + q.Encode()); err != nil {
		return Grant{}, err
	}

	select {
	case r := <-done:
		return r.grant, r.err
	case <-ctx.Done():
		return Grant{}, ctx.Err()
	}
}

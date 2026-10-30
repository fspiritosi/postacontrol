package autoconfig

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"postacontrol/core/model"
)

func mx(host string) func(context.Context, string) ([]*net.MX, error) {
	return func(context.Context, string) ([]*net.MX, error) {
		return []*net.MX{{Host: host, Pref: 10}}, nil
	}
}

func serveXML(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDetect_HostingerByMX(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("no debía haber peticiones HTTP: %s", r.URL)
	}))
	defer srv.Close()

	s, err := Detect(context.Background(), "yo@midominio.com", Options{
		LookupMX:            mx("mx1.hostinger.com."),
		ISPDBBase:           srv.URL + "/",
		DomainAutoconfigURL: func(string) string { return srv.URL + "/auto" },
	})
	require.NoError(t, err)
	assert.Equal(t, model.ServerSettings{Host: "imap.hostinger.com", Port: 993, Security: model.SecurityTLS}, s.IMAP)
	assert.Equal(t, model.ServerSettings{Host: "smtp.hostinger.com", Port: 465, Security: model.SecurityTLS}, s.SMTP)
	assert.Equal(t, "known", s.Source)
	assert.Equal(t, "yo@midominio.com", s.Username)
}

func TestDetect_ISPDB(t *testing.T) {
	data, err := os.ReadFile("testdata/ispdb-example.xml")
	require.NoError(t, err)
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		w.Write(data)
	}))
	defer srv.Close()

	s, err := Detect(context.Background(), "ana@example.com", Options{LookupMX: mx("mail.otro.net."), ISPDBBase: srv.URL + "/v1.1/"})
	require.NoError(t, err)
	assert.Equal(t, "/v1.1/example.com", path)
	assert.Equal(t, model.ServerSettings{Host: "imap.example.com", Port: 993, Security: model.SecurityTLS}, s.IMAP)
	assert.Equal(t, model.ServerSettings{Host: "smtp.example.com", Port: 587, Security: model.SecuritySTARTTLS}, s.SMTP)
	assert.Equal(t, "ana", s.Username)
	assert.Equal(t, "ispdb", s.Source)
}

func TestDetect_DomainAutoconfig(t *testing.T) {
	data, err := os.ReadFile("testdata/ispdb-example.xml")
	require.NoError(t, err)
	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()
	srv := serveXML(t, string(data))

	var gotDomain string
	s, err := Detect(context.Background(), "ana@example.com", Options{
		LookupMX:            mx("mail.otro.net."),
		ISPDBBase:           notFound.URL + "/",
		DomainAutoconfigURL: func(d string) string { gotDomain = d; return srv.URL },
	})
	require.NoError(t, err)
	assert.Equal(t, "example.com", gotDomain)
	assert.Equal(t, "autoconfig", s.Source)
	assert.Equal(t, "imap.example.com", s.IMAP.Host)
}

func TestDetect_SkipsPlainSocket(t *testing.T) {
	srv := serveXML(t, `<clientConfig><emailProvider id="x">
<incomingServer type="imap"><hostname>plain.x.com</hostname><port>143</port><socketType>plain</socketType><username>%EMAILADDRESS%</username></incomingServer>
<incomingServer type="imap"><hostname>imap.x.com</hostname><port>993</port><socketType>SSL</socketType><username>%EMAILADDRESS%</username></incomingServer>
<outgoingServer type="smtp"><hostname>plain.x.com</hostname><port>25</port><socketType>plain</socketType></outgoingServer>
<outgoingServer type="smtp"><hostname>smtp.x.com</hostname><port>465</port><socketType>SSL</socketType></outgoingServer>
</emailProvider></clientConfig>`)

	s, err := Detect(context.Background(), "a@x.com", Options{LookupMX: mx("m.y."), ISPDBBase: srv.URL + "/"})
	require.NoError(t, err)
	assert.Equal(t, "imap.x.com", s.IMAP.Host)
	assert.Equal(t, "smtp.x.com", s.SMTP.Host)
	assert.Equal(t, "a@x.com", s.Username)
}

func TestDetect_NothingFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	_, err := Detect(context.Background(), "a@x.com", Options{
		LookupMX:            func(context.Context, string) ([]*net.MX, error) { return nil, errors.New("dns caído") },
		ISPDBBase:           srv.URL + "/",
		DomainAutoconfigURL: func(string) string { return srv.URL + "/auto" },
	})
	assert.ErrorIs(t, err, ErrNotDetected)
}

func TestDetect_InvalidEmail(t *testing.T) {
	for _, e := range []string{"sin-arroba", "a@", "@x.com", "a@x.com/../y", "a@x y.com"} {
		_, err := Detect(context.Background(), e, Options{LookupMX: mx("m.")})
		require.Error(t, err, e)
		assert.False(t, errors.Is(err, ErrNotDetected), e)
	}
}

func TestSettingsJSONTags(t *testing.T) {
	out := jsonOf(t, Settings{})
	for _, k := range []string{`"imap"`, `"smtp"`, `"username"`, `"source"`} {
		assert.Contains(t, out, k)
	}
}

func jsonOf(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func TestDetect_RedirectDowngradeRejected(t *testing.T) {
	data, err := os.ReadFile("testdata/ispdb-example.xml")
	require.NoError(t, err)
	plain := serveXML(t, string(data))
	tlsSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	defer tlsSrv.Close()
	client := tlsSrv.Client()
	client.CheckRedirect = checkRedirect

	_, err = Detect(context.Background(), "ana@example.com", Options{
		LookupMX:            mx("m.y."),
		HTTPClient:          client,
		ISPDBBase:           tlsSrv.URL + "/",
		DomainAutoconfigURL: func(string) string { return tlsSrv.URL + "/auto" },
	})
	assert.ErrorIs(t, err, ErrNotDetected)
}

func TestDetect_UnknownPlaceholderFallsBackToEmail(t *testing.T) {
	srv := serveXML(t, `<clientConfig><emailProvider id="x">
<incomingServer type="imap"><hostname>imap.x.com</hostname><port>993</port><socketType>SSL</socketType><username>%FOO%</username></incomingServer>
<outgoingServer type="smtp"><hostname>smtp.x.com</hostname><port>465</port><socketType>SSL</socketType></outgoingServer>
</emailProvider></clientConfig>`)
	s, err := Detect(context.Background(), "a@x.com", Options{LookupMX: mx("m.y."), ISPDBBase: srv.URL + "/"})
	require.NoError(t, err)
	assert.Equal(t, "a@x.com", s.Username)
}

func TestDetect_EmailDomainPlaceholder(t *testing.T) {
	srv := serveXML(t, `<clientConfig><emailProvider id="x">
<incomingServer type="imap"><hostname>imap.x.com</hostname><port>993</port><socketType>SSL</socketType><username>%EMAILLOCALPART%.%EMAILDOMAIN%</username></incomingServer>
<outgoingServer type="smtp"><hostname>smtp.x.com</hostname><port>465</port><socketType>SSL</socketType></outgoingServer>
</emailProvider></clientConfig>`)
	s, err := Detect(context.Background(), "a@x.com", Options{LookupMX: mx("m.y."), ISPDBBase: srv.URL + "/"})
	require.NoError(t, err)
	assert.Equal(t, "a.x.com", s.Username)
}

func TestDetect_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := Detect(ctx, "a@x.com", Options{LookupMX: mx("m.y.")})
	assert.ErrorIs(t, err, context.Canceled)
	assert.False(t, errors.Is(err, ErrNotDetected))
}

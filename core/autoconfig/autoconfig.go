// Package autoconfig detecta la configuración IMAP/SMTP a partir de un email.
package autoconfig

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"postacontrol/core/model"
)

var ErrNotDetected = errors.New("no pudimos detectar los servidores; cargalos a mano")

// Settings es el resultado de la detección. Source: "known" | "ispdb" | "autoconfig".
type Settings struct {
	IMAP     model.ServerSettings `json:"imap"`
	SMTP     model.ServerSettings `json:"smtp"`
	Username string               `json:"username"`
	Source   string               `json:"source"`
}

type Options struct {
	LookupMX   func(ctx context.Context, domain string) ([]*net.MX, error) // default: net.DefaultResolver.LookupMX
	HTTPClient *http.Client                                                // default: timeout 8s
	ISPDBBase  string                                                      // default "https://autoconfig.thunderbird.net/v1.1/"
	// DomainAutoconfigURL arma la URL del paso 3 (default
	// "https://autoconfig.<dominio>/mail/config-v1.1.xml"). Permite inyectar un servidor de prueba.
	DomainAutoconfigURL func(domain string) string
}

const defaultISPDBBase = "https://autoconfig.thunderbird.net/v1.1/"

var domainRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

func (o Options) withDefaults() Options {
	if o.LookupMX == nil {
		o.LookupMX = net.DefaultResolver.LookupMX
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 8 * time.Second}
	}
	if o.ISPDBBase == "" {
		o.ISPDBBase = defaultISPDBBase
	}
	if o.DomainAutoconfigURL == nil {
		o.DomainAutoconfigURL = func(d string) string {
			return "https://autoconfig." + d + "/mail/config-v1.1.xml"
		}
	}
	return o
}

// Detect intenta, en orden: proveedor conocido por MX, ISPDB y autoconfig del dominio.
func Detect(ctx context.Context, email string, opt Options) (Settings, error) {
	local, domain, err := splitEmail(email)
	if err != nil {
		return Settings{}, err
	}
	opt = opt.withDefaults()

	if s, ok := detectKnown(ctx, opt, domain); ok {
		s.Username = email
		return s, nil
	}
	if s, ok := fetchConfig(ctx, opt, strings.TrimRight(opt.ISPDBBase, "/")+"/"+domain, email, local); ok {
		s.Source = "ispdb"
		return s, nil
	}
	if s, ok := fetchConfig(ctx, opt, opt.DomainAutoconfigURL(domain), email, local); ok {
		s.Source = "autoconfig"
		return s, nil
	}
	return Settings{}, ErrNotDetected
}

func splitEmail(email string) (local, domain string, err error) {
	i := strings.LastIndex(email, "@")
	if i <= 0 || i == len(email)-1 {
		return "", "", fmt.Errorf("email inválido: %q", email)
	}
	local, domain = email[:i], strings.ToLower(email[i+1:])
	if strings.ContainsAny(local, " \t/\\") || !domainRe.MatchString(domain) {
		return "", "", fmt.Errorf("email inválido: %q", email)
	}
	return local, domain, nil
}

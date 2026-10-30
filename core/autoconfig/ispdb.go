package autoconfig

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"strings"

	"postacontrol/core/model"
)

type xmlServer struct {
	Type       string `xml:"type,attr"`
	Hostname   string `xml:"hostname"`
	Port       int    `xml:"port"`
	SocketType string `xml:"socketType"`
	Username   string `xml:"username"`
}

type clientConfig struct {
	Provider struct {
		Incoming []xmlServer `xml:"incomingServer"`
		Outgoing []xmlServer `xml:"outgoingServer"`
	} `xml:"emailProvider"`
}

const maxConfigSize = 1 << 20

// fetchConfig descarga y parsea un XML en formato autoconfig de Mozilla.
func fetchConfig(ctx context.Context, opt Options, url, email, local string) (Settings, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Settings{}, false
	}
	resp, err := opt.HTTPClient.Do(req)
	if err != nil {
		return Settings{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Settings{}, false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxConfigSize))
	if err != nil {
		return Settings{}, false
	}
	return parseConfig(body, email, local)
}

func parseConfig(data []byte, email, local string) (Settings, bool) {
	var cfg clientConfig
	if err := xml.Unmarshal(data, &cfg); err != nil {
		return Settings{}, false
	}
	imap, imapUser, ok := pick(cfg.Provider.Incoming, "imap")
	if !ok {
		return Settings{}, false
	}
	smtp, smtpUser, ok := pick(cfg.Provider.Outgoing, "smtp")
	if !ok {
		return Settings{}, false
	}
	user := imapUser
	if user == "" {
		user = smtpUser
	}
	user = strings.NewReplacer("%EMAILADDRESS%", email, "%EMAILLOCALPART%", local).Replace(user)
	if user == "" {
		user = email
	}
	return Settings{IMAP: imap, SMTP: smtp, Username: user}, true
}

// pick elige el primer servidor del tipo dado con socket SSL o STARTTLS.
func pick(servers []xmlServer, typ string) (model.ServerSettings, string, bool) {
	for _, s := range servers {
		if !strings.EqualFold(s.Type, typ) || s.Hostname == "" || s.Port <= 0 {
			continue
		}
		var sec model.Security
		switch strings.ToUpper(strings.TrimSpace(s.SocketType)) {
		case "SSL":
			sec = model.SecurityTLS
		case "STARTTLS":
			sec = model.SecuritySTARTTLS
		default:
			continue
		}
		return model.ServerSettings{Host: strings.TrimSpace(s.Hostname), Port: s.Port, Security: sec}, strings.TrimSpace(s.Username), true
	}
	return model.ServerSettings{}, "", false
}

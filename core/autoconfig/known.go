package autoconfig

import (
	"context"
	"strings"

	"postacontrol/core/model"
)

func detectKnown(ctx context.Context, opt Options, domain string) (Settings, bool) {
	mxs, err := opt.LookupMX(ctx, domain)
	if err != nil {
		return Settings{}, false
	}
	for _, mx := range mxs {
		host := strings.ToLower(strings.TrimSuffix(mx.Host, "."))
		if strings.HasSuffix(host, ".hostinger.com") {
			return Settings{
				IMAP:   model.ServerSettings{Host: "imap.hostinger.com", Port: 993, Security: model.SecurityTLS},
				SMTP:   model.ServerSettings{Host: "smtp.hostinger.com", Port: 465, Security: model.SecurityTLS},
				Source: "known",
			}, true
		}
	}
	return Settings{}, false
}

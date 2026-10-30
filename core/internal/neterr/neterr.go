// Package neterr agrupa la clasificación de errores de red compartida por
// imapx y smtpx.
package neterr

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strings"
)

// IsTLSError informa si err viene de la negociación TLS o de la verificación
// del certificado (y no de la red).
func IsTLSError(err error) bool {
	if err == nil {
		return false
	}
	var (
		certInvalid  x509.CertificateInvalidError
		unknownAuth  x509.UnknownAuthorityError
		hostErr      x509.HostnameError
		verifyErr    *tls.CertificateVerificationError
		alertErr     tls.AlertError
		recordHdrErr tls.RecordHeaderError
	)
	if errors.As(err, &certInvalid) || errors.As(err, &unknownAuth) || errors.As(err, &hostErr) ||
		errors.As(err, &verifyErr) || errors.As(err, &alertErr) || errors.As(err, &recordHdrErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:")
}

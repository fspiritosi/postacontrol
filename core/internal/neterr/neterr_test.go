package neterr_test

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"postacontrol/core/internal/neterr"
)

func TestIsTLSError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"x509", fmt.Errorf("dial: %w", x509.UnknownAuthorityError{}), true},
		{"record header", fmt.Errorf("dial: %w", tls.RecordHeaderError{Msg: "x"}), true},
		{"net.OpError", &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}, false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) { assert.Equal(t, c.want, neterr.IsTLSError(c.err)) })
	}
}

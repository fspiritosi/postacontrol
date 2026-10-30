package smtpx

import (
	"context"
	"errors"
	"testing"

	"github.com/emersion/go-smtp"
	"github.com/stretchr/testify/assert"
)

func TestClassifyAuth(t *testing.T) {
	cases := []struct {
		code int
		auth bool
	}{{535, true}, {534, true}, {538, true}, {500, true}, {421, false}, {454, false}}
	for _, c := range cases {
		err := classifyAuth(context.Background(), &smtp.SMTPError{Code: c.code, Message: "x"})
		assert.Equal(t, c.auth, errorsIs(err, ErrAuth), "code %d", c.code)
		assert.Equal(t, !c.auth, errorsIs(err, ErrNetwork), "code %d", c.code)
	}
}

func errorsIs(err, target error) bool { return errors.Is(err, target) }

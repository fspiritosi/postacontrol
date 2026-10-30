package imapx

import (
	"errors"
	"fmt"

	"github.com/emersion/go-imap/v2"
)

// ErrReadOnly: la carpeta seleccionada no se abrió para escritura.
var ErrReadOnly = errors.New("imapx: carpeta abierta en solo lectura")

// AddFlags agrega flags a un mensaje (STORE +FLAGS). Requiere SelectRW.
func (c *Conn) AddFlags(uid uint32, flags ...imap.Flag) error {
	if !c.rw {
		return ErrReadOnly
	}
	err := c.c.Store(toUIDs([]uint32{uid}), &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: flags}, nil).Close()
	if err != nil {
		return opErr("UID STORE", err)
	}
	return nil
}

// Append sube un mensaje crudo (RFC 5322, con CRLF) a un buzón.
func (c *Conn) Append(mailbox string, raw []byte, flags ...imap.Flag) error {
	cmd := c.c.Append(mailbox, int64(len(raw)), &imap.AppendOptions{Flags: flags})
	if _, err := cmd.Write(raw); err != nil {
		_ = cmd.Close()
		return opErr(fmt.Sprintf("APPEND %q", mailbox), err)
	}
	if err := cmd.Close(); err != nil {
		return opErr(fmt.Sprintf("APPEND %q", mailbox), err)
	}
	if _, err := cmd.Wait(); err != nil {
		return opErr(fmt.Sprintf("APPEND %q", mailbox), err)
	}
	return nil
}

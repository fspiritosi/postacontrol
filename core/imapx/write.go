package imapx

import (
	"errors"
	"fmt"

	"github.com/emersion/go-imap/v2"
)

// ErrReadOnly: la carpeta seleccionada no se abrió para escritura.
var ErrReadOnly = errors.New("imapx: carpeta abierta en solo lectura")

// ErrUnsupported: el servidor no anuncia la capacidad necesaria.
var ErrUnsupported = errors.New("imapx: el servidor no lo soporta")

// StoreFlags agrega o quita flags en lote (STORE +FLAGS.SILENT / -FLAGS.SILENT).
// Requiere SelectRW.
func (c *Conn) StoreFlags(uids []uint32, add bool, flags ...imap.Flag) error {
	if !c.rw {
		return ErrReadOnly
	}
	op := imap.StoreFlagsDel
	if add {
		op = imap.StoreFlagsAdd
	}
	err := c.c.Store(toUIDs(uids), &imap.StoreFlags{Op: op, Silent: true, Flags: flags}, nil).Close()
	if err != nil {
		return opErr("UID STORE", err)
	}
	return nil
}

// AddFlags agrega flags a un mensaje (STORE +FLAGS). Requiere SelectRW.
func (c *Conn) AddFlags(uid uint32, flags ...imap.Flag) error {
	return c.StoreFlags([]uint32{uid}, true, flags...)
}

// Move mueve mensajes a otro buzón. Sin MOVE ni UIDPLUS devuelve
// ErrUnsupported sin mandar nada: el respaldo de la librería sería un EXPUNGE
// que borraría también lo que otro cliente marcó como \Deleted. Requiere SelectRW.
func (c *Conn) Move(uids []uint32, dest string) error {
	if !c.rw {
		return ErrReadOnly
	}
	caps := c.c.Caps()
	if !caps.Has(imap.CapMove) && !caps.Has(imap.CapUIDPlus) {
		return ErrUnsupported
	}
	if _, err := c.c.Move(toUIDs(uids), dest).Wait(); err != nil {
		return opErr(fmt.Sprintf("UID MOVE %q", dest), err)
	}
	return nil
}

// ExpungeUIDs marca \Deleted y borra solo esos UIDs (UID EXPUNGE). Exige
// UIDPLUS o IMAP4rev2. Requiere SelectRW.
func (c *Conn) ExpungeUIDs(uids []uint32) error {
	if !c.rw {
		return ErrReadOnly
	}
	if !c.c.Caps().Has(imap.CapUIDPlus) {
		return ErrUnsupported
	}
	if err := c.StoreFlags(uids, true, imap.FlagDeleted); err != nil {
		return err
	}
	if err := c.c.UIDExpunge(toUIDs(uids)).Close(); err != nil {
		return opErr("UID EXPUNGE", err)
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

// Noop manda NOOP. Sirve para comprobar que la sesión sigue viva antes de un
// comando que no se puede reintentar.
func (c *Conn) Noop() error {
	if err := c.c.Noop().Wait(); err != nil {
		return opErr("NOOP", err)
	}
	return nil
}

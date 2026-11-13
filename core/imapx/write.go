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
	if len(uids) == 0 {
		return nil
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

// Move mueve mensajes a otro buzón. Con MOVE lo hace el servidor. Con UIDPLUS
// sin MOVE se hace a mano y en orden (COPY, recién después \Deleted y UID
// EXPUNGE), porque el respaldo de la librería encadena los tres comandos sin
// esperar el resultado del COPY y perdería los originales si falla. Sin
// ninguna de las dos devuelve ErrUnsupported sin mandar nada: nunca se usa un
// EXPUNGE sin UID. Requiere SelectRW.
func (c *Conn) Move(uids []uint32, dest string) error {
	_, err := c.MoveUIDs(uids, dest)
	return err
}

// MoveResult es lo que informó el servidor sobre un MOVE (COPYUID, con
// UIDPLUS): la UIDVALIDITY del destino y los UID nuevos ahí. Dest queda vacío
// si el servidor no lo dijo.
type MoveResult struct {
	UIDValidity uint32
	Dest        []uint32
}

// MoveUIDs es Move pero además devuelve los UID que quedaron en el destino.
func (c *Conn) MoveUIDs(uids []uint32, dest string) (MoveResult, error) {
	if !c.rw {
		return MoveResult{}, ErrReadOnly
	}
	if len(uids) == 0 {
		return MoveResult{}, nil
	}
	caps := c.c.Caps()
	set := toUIDs(uids)
	switch {
	case caps.Has(imap.CapMove):
		data, err := c.c.Move(set, dest).Wait()
		if err != nil {
			return MoveResult{}, opErr(fmt.Sprintf("UID MOVE %q", dest), err)
		}
		var res MoveResult
		if data != nil {
			dst, _ := data.DestUIDs.(imap.UIDSet)
			res = MoveResult{UIDValidity: data.UIDValidity, Dest: fromUIDSet(dst)}
		}
		return res, nil
	case caps.Has(imap.CapUIDPlus):
		data, err := c.c.Copy(set, dest).Wait()
		if err != nil {
			return MoveResult{}, opErr(fmt.Sprintf("UID COPY %q", dest), err)
		}
		if err := c.StoreFlags(uids, true, imap.FlagDeleted); err != nil {
			return MoveResult{}, err
		}
		if err := c.c.UIDExpunge(set).Close(); err != nil {
			return MoveResult{}, opErr("UID EXPUNGE", err)
		}
		var res MoveResult
		if data != nil {
			res = MoveResult{UIDValidity: data.UIDValidity, Dest: fromUIDSet(data.DestUIDs)}
		}
		return res, nil
	}
	return MoveResult{}, ErrUnsupported
}

// fromUIDSet pasa un conjunto de UID a una lista. Un conjunto dinámico (con
// "*") o vacío da nil.
func fromUIDSet(s imap.UIDSet) []uint32 {
	if len(s) == 0 {
		return nil
	}
	nums, ok := s.Nums()
	if !ok {
		return nil
	}
	out := make([]uint32, len(nums))
	for i, n := range nums {
		out[i] = uint32(n)
	}
	return out
}

// ExpungeUIDs marca \Deleted y borra solo esos UIDs (UID EXPUNGE). Exige
// UIDPLUS o IMAP4rev2. Si el UID EXPUNGE falla después del STORE, los mensajes
// quedan marcados \Deleted. Requiere SelectRW.
func (c *Conn) ExpungeUIDs(uids []uint32) error {
	if !c.rw {
		return ErrReadOnly
	}
	if len(uids) == 0 {
		return nil
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

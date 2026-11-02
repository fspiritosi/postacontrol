package testenv

import (
	"slices"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapserver"
)

// mailboxDelim es el delimitador de imapmemserver.
const mailboxDelim = '/'

// AddNoselect agrega un buzón \Noselect: LIST lo informa así, SELECT, EXAMINE
// y STATUS fallan, y DELETE lo saca. imapmemserver no tiene buzones así (un
// padre que solo existe porque tiene hijos, como en Dovecot).
func (s *IMAP) AddNoselect(name string) {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	if s.noselect == nil {
		s.noselect = map[string]bool{}
	}
	s.noselect[name] = true
}

// Ops devuelve, en orden, los SELECT, EXAMINE y DELETE que recibió el
// servidor, como "SELECT <buzón>".
func (s *IMAP) Ops() []string {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	return slices.Clone(s.ops)
}

// SetMoveHook hace que, después de cada MOVE que sale bien, se llame a fn con
// el buzón de origen (en la goroutine del servidor). nil lo saca.
func (s *IMAP) SetMoveHook(fn func(src string)) {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	s.moveHook = fn
}

func (s *IMAP) record(op string) {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	s.ops = append(s.ops, op)
}

func (s *IMAP) isNoselect(name string) bool {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	return s.noselect[name]
}

var errNoselect = &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Mailbox is \\Noselect"}

// session envuelve la sesión de imapmemserver para simular los buzones
// \Noselect y registrar comandos. Sin nada configurado se comporta igual.
type session struct {
	imapserver.SessionIMAP4rev2
	s        *IMAP
	selected string
}

func (sess *session) Select(name string, options *imap.SelectOptions) (*imap.SelectData, error) {
	op := "SELECT "
	if options != nil && options.ReadOnly {
		op = "EXAMINE "
	}
	sess.s.record(op + name)
	if sess.s.isNoselect(name) {
		return nil, errNoselect
	}
	data, err := sess.SessionIMAP4rev2.Select(name, options)
	if err == nil {
		sess.selected = name
	}
	return data, err
}

func (sess *session) Status(name string, options *imap.StatusOptions) (*imap.StatusData, error) {
	if sess.s.isNoselect(name) {
		return nil, errNoselect
	}
	return sess.SessionIMAP4rev2.Status(name, options)
}

func (sess *session) Delete(name string) error {
	sess.s.record("DELETE " + name)
	sess.s.simMu.Lock()
	if sess.s.noselect[name] {
		delete(sess.s.noselect, name)
		sess.s.simMu.Unlock()
		return nil
	}
	sess.s.simMu.Unlock()
	return sess.SessionIMAP4rev2.Delete(name)
}

func (sess *session) List(w *imapserver.ListWriter, ref string, patterns []string, options *imap.ListOptions) error {
	if len(patterns) > 0 && (options == nil || !options.SelectSubscribed) {
		sess.s.simMu.Lock()
		var names []string
		for name := range sess.s.noselect {
			if slices.ContainsFunc(patterns, func(p string) bool {
				return imapserver.MatchList(name, mailboxDelim, ref, p)
			}) {
				names = append(names, name)
			}
		}
		sess.s.simMu.Unlock()
		slices.Sort(names)
		for _, name := range names {
			data := imap.ListData{
				Attrs:   []imap.MailboxAttr{imap.MailboxAttrNoSelect, imap.MailboxAttrHasChildren},
				Delim:   mailboxDelim,
				Mailbox: name,
			}
			if err := w.WriteList(&data); err != nil {
				return err
			}
		}
	}
	return sess.SessionIMAP4rev2.List(w, ref, patterns, options)
}

func (sess *session) Move(w *imapserver.MoveWriter, numSet imap.NumSet, dest string) error {
	if err := sess.SessionIMAP4rev2.Move(w, numSet, dest); err != nil {
		return err
	}
	sess.s.simMu.Lock()
	hook := sess.s.moveHook
	sess.s.simMu.Unlock()
	if hook != nil {
		hook(sess.selected)
	}
	return nil
}

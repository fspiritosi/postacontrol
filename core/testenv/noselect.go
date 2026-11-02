package testenv

import (
	"slices"
	"strings"

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

// RemoveNoselect saca el buzón \Noselect agregado con AddNoselect.
func (s *IMAP) RemoveNoselect(name string) {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	delete(s.noselect, name)
}

// OmitStatusMessages hace que STATUS del buzón responda sin MESSAGES, como un
// servidor que no lo informa.
func (s *IMAP) OmitStatusMessages(name string) {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	if s.noCount == nil {
		s.noCount = map[string]bool{}
	}
	s.noCount[name] = true
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

// NoKeywords simula un servidor que no guarda keywords: desde ahora SELECT y
// EXAMINE anuncian en PERMANENTFLAGS solo los flags de sistema (sin \*) y un
// STORE que incluya algún keyword (un flag que no empieza con «\») responde NO.
func (s *IMAP) NoKeywords() {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	s.noKeywords, s.rejectKeywords = true, true
}

// RejectKeywordStores hace que un STORE que incluya algún keyword responda NO,
// aunque PERMANENTFLAGS siga anunciando \*: un rechazo que el cliente no
// puede prever.
func (s *IMAP) RejectKeywordStores() {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	s.rejectKeywords = true
}

// OmitPermanentFlags hace que SELECT y EXAMINE respondan PERMANENTFLAGS
// vacío. go-imap no distingue eso de un servidor que no manda PERMANENTFLAGS:
// para el cliente, no se sabe qué flags guarda. Tiene prioridad sobre
// NoKeywords.
func (s *IMAP) OmitPermanentFlags() {
	s.simMu.Lock()
	defer s.simMu.Unlock()
	s.noPermFlags = true
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
// \Noselect y los servidores sin keywords, y registrar comandos. Sin nada
// configurado se comporta igual.
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
	if err != nil {
		return data, err
	}
	sess.selected = name
	sess.s.simMu.Lock()
	noKeywords, noPermFlags := sess.s.noKeywords, sess.s.noPermFlags
	sess.s.simMu.Unlock()
	switch {
	case noPermFlags:
		data.PermanentFlags = nil
	case noKeywords:
		// imapmemserver anuncia los flags en uso más \*: sin \* la lista
		// puede quedar vacía, que es otra cosa (no se sabe). Un servidor sin
		// keywords anuncia los flags de sistema.
		data.PermanentFlags = []imap.Flag{imap.FlagSeen, imap.FlagAnswered, imap.FlagFlagged, imap.FlagDeleted, imap.FlagDraft}
	}
	return data, nil
}

var errNoKeywords = &imap.Error{Type: imap.StatusResponseTypeNo, Text: "Keywords are not supported"}

func (sess *session) Store(w *imapserver.FetchWriter, numSet imap.NumSet, flags *imap.StoreFlags, options *imap.StoreOptions) error {
	sess.s.simMu.Lock()
	reject := sess.s.rejectKeywords
	sess.s.simMu.Unlock()
	if reject && slices.ContainsFunc(flags.Flags, func(f imap.Flag) bool { return !strings.HasPrefix(string(f), `\`) }) {
		return errNoKeywords
	}
	return sess.SessionIMAP4rev2.Store(w, numSet, flags, options)
}

func (sess *session) Status(name string, options *imap.StatusOptions) (*imap.StatusData, error) {
	if sess.s.isNoselect(name) {
		return nil, errNoselect
	}
	sess.s.simMu.Lock()
	omit := sess.s.noCount[name]
	sess.s.simMu.Unlock()
	if omit && options != nil {
		// El servidor arma la respuesta con estas mismas opciones: sin
		// MESSAGES, la respuesta no lo trae.
		options.NumMessages = false
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

package mail

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"postacontrol/core/icalx"
	"postacontrol/core/model"
	"postacontrol/core/store"
)

const (
	maxICSBytes   = 50 << 20 // un archivo, o una entrada de un zip descomprimida
	maxZipEntries = 50

	msgICSTooBig     = "Ese archivo es demasiado grande para importar."
	msgICSUnreadable = "No pudimos leer %s."
)

func icsTooBig() error { return &UserError{Code: "invalid", Msg: msgICSTooBig} }

func icsUnreadable(name string, err error) error {
	return &UserError{Code: "invalid", Msg: fmt.Sprintf(msgICSUnreadable, name), Err: err}
}

// icsSource es un calendario .ics ya leído de un archivo o de un zip.
type icsSource struct {
	id   string
	name string // nombre del archivo o entrada, sin la carpeta
	data []byte
}

func isZip(data []byte) bool { return bytes.HasPrefix(data, []byte("PK\x03\x04")) }

func isICSName(name string) bool { return strings.EqualFold(path.Ext(name), ".ics") }

// readICSFile lee un archivo del disco (hasta maxICSBytes).
func readICSFile(p string) ([]byte, error) {
	f, err := os.Open(p)
	if err != nil {
		return nil, icsUnreadable(filepath.Base(p), err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return nil, icsUnreadable(filepath.Base(p), err)
	}
	if st.Size() > maxICSBytes {
		return nil, icsTooBig()
	}
	data, err := io.ReadAll(io.LimitReader(f, maxICSBytes+1))
	if err != nil {
		return nil, icsUnreadable(filepath.Base(p), err)
	}
	if len(data) > maxICSBytes {
		return nil, icsTooBig()
	}
	return data, nil
}

// openZip abre el zip data y comprueba la cantidad de entradas.
func openZip(name string, data []byte) (*zip.Reader, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, icsUnreadable(name, err)
	}
	if len(zr.File) > maxZipEntries {
		return nil, icsTooBig()
	}
	return zr, nil
}

// readZipEntry lee una entrada .ics: el tamaño declarado y el real no pasan de maxICSBytes.
func readZipEntry(f *zip.File) ([]byte, error) {
	name := path.Base(f.Name)
	if f.UncompressedSize64 > maxICSBytes {
		return nil, icsTooBig()
	}
	rc, err := f.Open()
	if err != nil {
		return nil, icsUnreadable(name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxICSBytes+1))
	if err != nil {
		return nil, icsUnreadable(name, err)
	}
	if len(data) > maxICSBytes {
		return nil, icsTooBig()
	}
	return data, nil
}

// loadICSID lee lo que designa id: una ruta, o "ruta#entrada" de un zip.
func loadICSID(id string) (icsSource, error) {
	if _, err := os.Stat(id); err == nil {
		data, err := readICSFile(id)
		if err != nil {
			return icsSource{}, err
		}
		if isZip(data) { // un zip entero no es un calendario
			return icsSource{}, icsUnreadable(filepath.Base(id), nil)
		}
		return icsSource{id: id, name: filepath.Base(id), data: data}, nil
	}
	i := strings.LastIndex(id, "#")
	if i < 0 {
		return icsSource{}, icsUnreadable(filepath.Base(id), nil)
	}
	zp, entry := id[:i], id[i+1:]
	data, err := readICSFile(zp)
	if err != nil {
		return icsSource{}, err
	}
	if !isZip(data) {
		return icsSource{}, icsUnreadable(path.Base(entry), nil)
	}
	zr, err := openZip(filepath.Base(zp), data)
	if err != nil {
		return icsSource{}, err
	}
	for _, f := range zr.File {
		if f.Name != entry || !isICSName(f.Name) {
			continue
		}
		body, err := readZipEntry(f)
		if err != nil {
			return icsSource{}, err
		}
		return icsSource{id: id, name: path.Base(entry), data: body}, nil
	}
	return icsSource{}, icsUnreadable(path.Base(entry), nil)
}

func trimExt(name string) string { return strings.TrimSuffix(name, path.Ext(name)) }

// PreviewICSImport lista los calendarios que traen los archivos .ics o .zip
// (el de Google): uno por .ics suelto y uno por cada .ics de un zip.
func (c *Core) PreviewICSImport(ctx context.Context, paths []string) ([]model.ICSCalendar, error) {
	out := []model.ICSCalendar{}
	add := func(s icsSource) error {
		cal, err := icalx.Parse(s.data)
		if err != nil {
			return icsUnreadable(s.name, err)
		}
		name := cal.Name
		if name == "" {
			name = trimExt(s.name)
		}
		out = append(out, model.ICSCalendar{ID: s.id, Name: name, Events: len(cal.Series)})
		return nil
	}
	for _, p := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		data, err := readICSFile(p)
		if err != nil {
			return nil, err
		}
		if !isZip(data) {
			if err := add(icsSource{id: p, name: filepath.Base(p), data: data}); err != nil {
				return nil, err
			}
			continue
		}
		zr, err := openZip(filepath.Base(p), data)
		if err != nil {
			return nil, err
		}
		for _, f := range zr.File {
			if f.FileInfo().IsDir() || !isICSName(f.Name) {
				continue
			}
			body, err := readZipEntry(f)
			if err != nil {
				return nil, err
			}
			if err := add(icsSource{id: p + "#" + f.Name, name: path.Base(f.Name), data: body}); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// ImportICS suma al calendario los eventos de los calendarios elegidos (los ids
// de PreviewICSImport), que se vuelven a leer del disco. Nunca manda correos.
// Un evento cuya UID ya está se saltea, salvo una invitación con una SEQUENCE
// más nueva, que se actualiza (conservando las notas y el aviso propios).
func (c *Core) ImportICS(ctx context.Context, ids []string) (model.ICSImportResult, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	var res model.ICSImportResult
	cals := make([]icalx.Calendar, 0, len(ids))
	for _, id := range ids {
		s, err := loadICSID(id)
		if err != nil {
			return res, err
		}
		cal, err := icalx.Parse(s.data)
		if err != nil {
			return res, icsUnreadable(s.name, err)
		}
		cals = append(cals, cal)
	}
	accs, err := c.store.ListAccounts(ctx)
	if err != nil {
		return res, c.ctxOrUserError(ctx, err)
	}

	c.calMu.Lock()
	defer c.calMu.Unlock()
	seen := map[string]bool{}
	for _, cal := range cals {
		res.Skipped += len(cal.Skipped)
		for _, s := range cal.Series {
			if err := c.importSeries(ctx, s, accs, seen, &res); err != nil {
				if res.Added+res.Updated > 0 {
					c.emit(EventCalendarChanged, nil)
				}
				return res, c.ctxOrUserError(ctx, err)
			}
		}
	}
	if res.Added+res.Updated > 0 {
		c.emit(EventCalendarChanged, nil)
	}
	return res, nil
}

// importedEvent arma el evento de la serie s según quién lo organiza: uno de
// otro es una invitación ("invite") y uno propio o sin organizador, un evento
// "local" (sin invitados propios). Conserva la SEQUENCE.
func (c *Core) importedEvent(s icalx.Series, accs []model.Account) (model.CalendarEvent, error) {
	m := s.Master
	e := seriesEvent(s, nil)
	if e.TZID == "" {
		e.TZID = localTZID()
	}
	org := accountByEmail(accs, m.Organizer.Email)
	if strings.TrimSpace(m.Organizer.Email) != "" && org == nil {
		raw, err := icalx.Build("PUBLISH", s, c.now())
		if err != nil {
			return e, err
		}
		e.RawICS = string(raw)
		var receiver int64
		for _, a := range accs {
			if receiver == 0 || a.ID < receiver {
				receiver = a.ID
			}
		}
		setMyAttendee(&e, m.Attendees, accs, receiver)
		return e, nil
	}
	e.Kind = "local"
	e.Organizer = model.Address{}
	if org != nil {
		e.AccountID, e.Organizer = org.ID, m.Organizer
	}
	others := []model.Attendee{}
	for _, a := range e.Attendees {
		if accountByEmail(accs, a.Email) == nil {
			others = append(others, a)
		}
	}
	e.Attendees = others
	return e, nil
}

// importSeries guarda la serie s (con calMu tomado) y la cuenta en res.
func (c *Core) importSeries(ctx context.Context, s icalx.Series, accs []model.Account, seen map[string]bool, res *model.ICSImportResult) error {
	uid := s.Master.UID
	if seen[uid] {
		res.Skipped++
		return nil
	}
	seen[uid] = true
	e, err := c.importedEvent(s, accs)
	if err != nil {
		res.Skipped++
		return nil
	}
	prev, err := c.store.EventByUID(ctx, uid)
	switch {
	case errors.Is(err, store.ErrNotFound):
		if _, err := c.store.SaveEvent(ctx, e, seriesEnd(e)); err != nil {
			return err
		}
		res.Added++
	case err != nil:
		return err
	case prev.Kind == "invite" && e.Kind == "invite" && e.Sequence > prev.Sequence:
		e.ID, e.Notes, e.RemindBefore = prev.ID, prev.Notes, prev.RemindBefore
		if _, err := c.store.SaveEvent(ctx, e, seriesEnd(e)); err != nil {
			return err
		}
		res.Updated++
	default:
		res.Skipped++
	}
	return nil
}

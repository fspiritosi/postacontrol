package mail

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"postacontrol/core/model"
	"postacontrol/core/store"
)

// Respaldo de lo que vive solo en esta PC: contactos, firmas, etiquetas,
// reglas y la firma por defecto de cada cuenta. El correo no va: su fuente de
// verdad es el servidor. Tampoco van las cuentas ni sus contraseñas (las
// contraseñas están en el llavero y no salen de ahí).
//
// Las referencias entre datos van por algo estable y no por los ids de esta
// base: la cuenta por su dirección, la carpeta por su nombre en el servidor, la
// etiqueta por su keyword IMAP y la firma por su posición en el archivo.

const (
	backupApp     = "postacontrol"
	backupKind    = "respaldo"
	backupVersion = 1
	// Un respaldo de verdad pesa unos pocos KB (las firmas con imágenes, algo
	// más): el tope solo evita leer a memoria un archivo cualquiera enorme.
	maxBackupBytes = 50 << 20

	msgBackupRead    = "No pudimos leer ese archivo."
	msgBackupInvalid = "Ese archivo no es un respaldo de PostaControl."
	msgBackupNewer   = "Ese respaldo es de una versión más nueva de PostaControl. Actualizá la app para importarlo."
	msgBackupWrite   = "No pudimos guardar el respaldo en esa ubicación."
)

// Los keywords que crea la app; uno distinto en un respaldo no se usa tal cual.
var labelKeywordRE = regexp.MustCompile(`^\$pc_[0-9a-f]{8}$`)

type backupFile struct {
	App        string            `json:"app"`
	Kind       string            `json:"kind"`
	Version    int               `json:"version"`
	CreatedAt  time.Time         `json:"createdAt"`
	Contacts   []backupContact   `json:"contacts"`
	Signatures []backupSignature `json:"signatures"`
	Labels     []backupLabel     `json:"labels"`
	Rules      []backupRule      `json:"rules"`
	Accounts   []backupAccount   `json:"accounts"`
}

type backupContact struct {
	Name    string   `json:"name"`
	Emails  []string `json:"emails"`
	Phones  []string `json:"phones"`
	Company string   `json:"company"`
	Notes   string   `json:"notes"`
}

// backupSignature: Ref es su número dentro del archivo (las cuentas la nombran así).
type backupSignature struct {
	Ref  int    `json:"ref"`
	Name string `json:"name"`
	HTML string `json:"html"`
}

type backupLabel struct {
	Keyword string `json:"keyword"`
	Name    string `json:"name"`
	Color   string `json:"color"`
}

type backupRule struct {
	Account    string                `json:"account"` // dirección de la cuenta
	Name       string                `json:"name"`
	Enabled    bool                  `json:"enabled"`
	MatchAll   bool                  `json:"matchAll"`
	Conditions []model.RuleCondition `json:"conditions"`
	Actions    []backupRuleAction    `json:"actions"`
}

type backupRuleAction struct {
	Type   string `json:"type"`
	Folder string `json:"folder,omitempty"` // nombre de la carpeta en el servidor
	Label  string `json:"label,omitempty"`  // keyword de la etiqueta
}

// backupAccount guarda solo lo local de una cuenta: su firma por defecto.
type backupAccount struct {
	Email            string `json:"email"`
	DefaultSignature int    `json:"defaultSignature,omitempty"` // Ref; 0 = sin firma
}

// ExportBackup escribe el respaldo en dest. Si algo falla a mitad de camino,
// dest no queda a medio escribir.
func (c *Core) ExportBackup(ctx context.Context, dest string) error {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	b, err := c.buildBackup(ctx)
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return c.ctxOrUserError(ctx, err)
	}
	// 0600: lleva datos personales (contactos y notas).
	if err := writeFileAtomic(dest, append(data, '\n'), 0o600); err != nil {
		return &UserError{Code: "io", Msg: msgBackupWrite, Err: err}
	}
	return nil
}

func (c *Core) buildBackup(ctx context.Context) (backupFile, error) {
	b := backupFile{App: backupApp, Kind: backupKind, Version: backupVersion, CreatedAt: time.Now().UTC(),
		Contacts: []backupContact{}, Signatures: []backupSignature{}, Labels: []backupLabel{},
		Rules: []backupRule{}, Accounts: []backupAccount{}}

	contacts, err := c.store.ListContacts(ctx)
	if err != nil {
		return b, err
	}
	for _, ct := range contacts {
		b.Contacts = append(b.Contacts, backupContact{Name: ct.Name, Emails: ct.Emails, Phones: ct.Phones,
			Company: ct.Company, Notes: ct.Notes})
	}

	sigs, err := c.store.ListSignatures(ctx)
	if err != nil {
		return b, err
	}
	sigRef := map[int64]int{}
	for i, s := range sigs {
		sigRef[s.ID] = i + 1
		b.Signatures = append(b.Signatures, backupSignature{Ref: i + 1, Name: s.Name, HTML: s.HTML})
	}

	labels, err := c.store.ListLabels(ctx)
	if err != nil {
		return b, err
	}
	byKw, err := c.store.LabelByKeyword(ctx)
	if err != nil {
		return b, err
	}
	kwOf := map[int64]string{}
	for kw, id := range byKw {
		kwOf[id] = kw
	}
	for _, l := range labels {
		b.Labels = append(b.Labels, backupLabel{Keyword: kwOf[l.ID], Name: l.Name, Color: l.Color})
	}

	accounts, err := c.store.ListAccounts(ctx)
	if err != nil {
		return b, err
	}
	for _, a := range accounts {
		b.Accounts = append(b.Accounts, backupAccount{Email: a.Email, DefaultSignature: sigRef[a.DefaultSignatureID]})
		folders, err := c.store.ListFolders(ctx, a.ID)
		if err != nil {
			return b, err
		}
		folderName := map[int64]string{}
		for _, f := range folders {
			folderName[f.ID] = f.Name
		}
		rules, err := c.store.ListRules(ctx, a.ID)
		if err != nil {
			return b, err
		}
		for _, r := range rules {
			br := backupRule{Account: a.Email, Name: r.Name, Enabled: r.Enabled, MatchAll: r.MatchAll,
				Conditions: r.Conditions, Actions: []backupRuleAction{}}
			for _, ac := range r.Actions {
				ba := backupRuleAction{Type: ac.Type}
				switch ac.Type {
				case "move":
					ba.Folder = folderName[ac.FolderID]
				case "label":
					ba.Label = kwOf[ac.LabelID]
				}
				br.Actions = append(br.Actions, ba)
			}
			b.Rules = append(b.Rules, br)
		}
	}
	return b, nil
}

// ImportBackup suma a esta instalación lo que trae el respaldo de src, sin
// pisar ni borrar nada de lo que ya hay: una ficha con alguno de sus correos ya
// en la libreta, una firma o una regla con el mismo nombre y una etiqueta con
// el mismo keyword o nombre se saltean. Una cuenta que ya tiene firma por
// defecto la conserva.
func (c *Core) ImportBackup(ctx context.Context, src string) (model.BackupImport, error) {
	ctx, cancel := c.lifeCtx(ctx)
	defer cancel()
	b, err := readBackup(src)
	if err != nil {
		return model.BackupImport{}, err
	}
	var res model.BackupImport
	err = c.applyBackup(ctx, b, &res)
	// Lo que se llegó a importar queda: se avisa aunque haya fallado después.
	if res.ContactsAdded > 0 {
		c.emit(EventContactsChanged, nil)
	}
	if res.SignaturesAdded > 0 {
		c.emit(EventSignaturesChanged, nil)
		c.emit(EventAccountsChanged, nil)
	}
	if res.LabelsAdded > 0 {
		c.emit(EventLabelsChanged, nil)
	}
	if err != nil {
		return res, c.ctxOrUserError(ctx, err)
	}
	return res, nil
}

func readBackup(src string) (backupFile, error) {
	f, err := os.Open(src)
	if err != nil {
		return backupFile{}, &UserError{Code: "io", Msg: msgBackupRead, Err: err}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxBackupBytes+1))
	if err != nil {
		return backupFile{}, &UserError{Code: "io", Msg: msgBackupRead, Err: err}
	}
	if len(data) > maxBackupBytes {
		return backupFile{}, &UserError{Code: "invalid", Msg: msgBackupInvalid}
	}
	var b backupFile
	if err := json.Unmarshal(data, &b); err != nil || b.App != backupApp || b.Kind != backupKind || b.Version < 1 {
		return backupFile{}, &UserError{Code: "invalid", Msg: msgBackupInvalid, Err: err}
	}
	if b.Version > backupVersion {
		return backupFile{}, &UserError{Code: "invalid", Msg: msgBackupNewer}
	}
	return b, nil
}

func (c *Core) applyBackup(ctx context.Context, b backupFile, res *model.BackupImport) error {
	if err := c.importContacts(ctx, b.Contacts, res); err != nil {
		return err
	}
	sigIDs, err := c.importSignatures(ctx, b.Signatures, res)
	if err != nil {
		return err
	}
	labelIDs, newLabels, err := c.importLabels(ctx, b.Labels, res)
	if err != nil {
		return err
	}
	accounts, err := c.store.ListAccounts(ctx)
	if err != nil {
		return err
	}
	accByEmail := map[string]model.Account{}
	for _, a := range accounts {
		accByEmail[strings.ToLower(a.Email)] = a
	}
	if err := c.importRules(ctx, b.Rules, accByEmail, labelIDs, res); err != nil {
		return err
	}
	for _, ba := range b.Accounts {
		a, ok := accByEmail[strings.ToLower(ba.Email)]
		sigID := sigIDs[ba.DefaultSignature]
		if !ok || a.DefaultSignatureID != 0 || sigID == 0 {
			continue
		}
		if err := c.store.SetAccountSignature(ctx, a.ID, sigID); err != nil {
			return err
		}
		c.emit(EventAccountsChanged, nil)
	}
	// Las etiquetas restauradas vuelven a juntar sus correos desde el servidor.
	for _, id := range newLabels {
		c.spawn(func(ctx context.Context) { _ = c.RefreshLabel(ctx, id, 0) })
	}
	return nil
}

func (c *Core) importContacts(ctx context.Context, list []backupContact, res *model.BackupImport) error {
	existing, err := c.store.ListContacts(ctx)
	if err != nil {
		return err
	}
	// Una ficha sin correos se reconoce por el nombre.
	namesWithoutEmail := map[string]bool{}
	for _, ct := range existing {
		if len(ct.Emails) == 0 {
			namesWithoutEmail[fold(ct.Name)] = true
		}
	}
	for _, bc := range list {
		ct := model.Contact{Name: bc.Name, Emails: nonNil(bc.Emails), Phones: nonNil(bc.Phones),
			Company: bc.Company, Notes: bc.Notes}
		present, err := c.contactPresent(ctx, ct, namesWithoutEmail)
		if err != nil {
			return err
		}
		if present {
			res.ContactsSkipped++
			continue
		}
		if _, err := c.SaveContact(ctx, ct); err != nil {
			var ue *UserError
			if errors.As(err, &ue) {
				// Una ficha que no pasa la validación (un correo roto, por
				// ejemplo) no corta el resto.
				res.ContactsSkipped++
				continue
			}
			return err
		}
		res.ContactsAdded++
		if len(ct.Emails) == 0 {
			namesWithoutEmail[fold(strings.TrimSpace(ct.Name))] = true
		}
	}
	return nil
}

func (c *Core) contactPresent(ctx context.Context, ct model.Contact, namesWithoutEmail map[string]bool) (bool, error) {
	if len(ct.Emails) == 0 {
		return namesWithoutEmail[fold(strings.TrimSpace(ct.Name))], nil
	}
	for _, e := range ct.Emails {
		_, err := c.store.ContactIDForEmail(ctx, strings.TrimSpace(e))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, store.ErrNotFound) {
			return false, err
		}
	}
	return false, nil
}

// importSignatures devuelve el id local de cada Ref del archivo (la nueva o la
// que ya estaba con ese nombre).
func (c *Core) importSignatures(ctx context.Context, list []backupSignature, res *model.BackupImport) (map[int]int64, error) {
	existing, err := c.store.ListSignatures(ctx)
	if err != nil {
		return nil, err
	}
	byName := map[string]int64{}
	for _, s := range existing {
		byName[fold(s.Name)] = s.ID
	}
	ids := map[int]int64{}
	for _, bs := range list {
		key := fold(strings.TrimSpace(bs.Name))
		if id, ok := byName[key]; ok {
			ids[bs.Ref] = id
			res.SignaturesSkipped++
			continue
		}
		s, err := c.SaveSignature(ctx, model.Signature{Name: bs.Name, HTML: bs.HTML})
		if err != nil {
			var ue *UserError
			if errors.As(err, &ue) {
				res.SignaturesSkipped++
				continue
			}
			return nil, err
		}
		byName[key] = s.ID
		ids[bs.Ref] = s.ID
		res.SignaturesAdded++
	}
	return ids, nil
}

// importLabels devuelve el id local de cada keyword del archivo y los ids de
// las etiquetas creadas.
func (c *Core) importLabels(ctx context.Context, list []backupLabel, res *model.BackupImport) (map[string]int64, []int64, error) {
	existing, err := c.store.ListLabels(ctx)
	if err != nil {
		return nil, nil, err
	}
	taken, err := c.store.LabelByKeyword(ctx)
	if err != nil {
		return nil, nil, err
	}
	ids := map[string]int64{}
	var created []int64
	for _, bl := range list {
		if id, ok := taken[bl.Keyword]; ok {
			ids[bl.Keyword] = id
			res.LabelsSkipped++
			continue
		}
		name, err := cleanLabel(bl.Name, bl.Color, existing)
		if err != nil {
			// Mismo nombre que una que ya está: las reglas usan esa.
			if i := labelNamed(existing, bl.Name); i >= 0 {
				ids[bl.Keyword] = existing[i].ID
				res.LabelsSkipped++
			} else {
				res.LabelsFailed++
			}
			continue
		}
		if len(existing) >= maxLabels {
			res.LabelsFailed++
			continue
		}
		kw := bl.Keyword
		if !labelKeywordRE.MatchString(kw) {
			if kw, err = newLabelKeyword(taken); err != nil {
				return nil, nil, err
			}
		}
		l, err := c.store.RestoreLabel(ctx, name, bl.Color, kw)
		if err != nil {
			return nil, nil, err
		}
		taken[kw] = l.ID
		ids[bl.Keyword] = l.ID
		existing = append(existing, l)
		created = append(created, l.ID)
		res.LabelsAdded++
	}
	return ids, created, nil
}

func labelNamed(list []model.Label, name string) int {
	name = strings.TrimSpace(name)
	for i, l := range list {
		if strings.EqualFold(l.Name, name) {
			return i
		}
	}
	return -1
}

func (c *Core) importRules(ctx context.Context, list []backupRule, accByEmail map[string]model.Account,
	labelIDs map[string]int64, res *model.BackupImport) error {
	folders := map[int64]map[string]int64{} // por cuenta: nombre → id
	ruleNames := map[int64]map[string]bool{}
	for _, br := range list {
		acc, ok := accByEmail[strings.ToLower(br.Account)]
		if !ok {
			res.RulesFailed++
			continue
		}
		if _, ok := folders[acc.ID]; !ok {
			fs, err := c.store.ListFolders(ctx, acc.ID)
			if err != nil {
				return err
			}
			byName := map[string]int64{}
			for _, f := range fs {
				byName[f.Name] = f.ID
			}
			folders[acc.ID] = byName
			rs, err := c.store.ListRules(ctx, acc.ID)
			if err != nil {
				return err
			}
			names := map[string]bool{}
			for _, r := range rs {
				names[fold(r.Name)] = true
			}
			ruleNames[acc.ID] = names
		}
		if ruleNames[acc.ID][fold(strings.TrimSpace(br.Name))] {
			res.RulesSkipped++
			continue
		}
		r := model.Rule{AccountID: acc.ID, Name: br.Name, Enabled: br.Enabled, MatchAll: br.MatchAll,
			Conditions: nonNil(br.Conditions), Actions: []model.RuleAction{}}
		missing := false
		for _, ba := range br.Actions {
			a := model.RuleAction{Type: ba.Type}
			switch ba.Type {
			case "move":
				a.FolderID, ok = folders[acc.ID][ba.Folder]
				missing = missing || !ok
			case "label":
				a.LabelID, ok = labelIDs[ba.Label]
				missing = missing || !ok
			}
			r.Actions = append(r.Actions, a)
		}
		if missing {
			res.RulesFailed++
			continue
		}
		saved, err := c.SaveRule(ctx, r)
		if err != nil {
			var ue *UserError
			if errors.As(err, &ue) {
				res.RulesFailed++
				continue
			}
			return err
		}
		ruleNames[acc.ID][fold(saved.Name)] = true
		res.RulesAdded++
	}
	return nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

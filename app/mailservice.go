package app

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"postacontrol/core/autoconfig"
	"postacontrol/core/mail"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
)

const syncEvery = 5 * time.Minute

// MailService es el servicio que ve el frontend. Delega en mail.Core; los
// errores del núcleo (*mail.UserError) llegan a JS con su mensaje en español,
// porque Wails usa err.Error().
type MailService struct {
	app  *application.App
	core *mail.Core
}

// SetApp se llama una vez, después de application.New y antes de Run: el
// servicio se registra en las opciones de la app, así que la app no existe
// todavía cuando se crea.
func (s *MailService) SetApp(app *application.App) { s.app = app }

func (s *MailService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	dir, err := DataDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("app: no se pudo crear %s: %w", dir, err)
	}
	core, err := mail.New(mail.Config{
		DBPath:   filepath.Join(dir, "postacontrol.db"),
		Secrets:  secrets.NewKeyring(),
		Notifier: wailsNotifier{app: s.app},
	})
	if err != nil {
		return err
	}
	s.core = core
	core.StartBackgroundSync(ctx, syncEvery)
	return nil
}

func (s *MailService) ServiceShutdown() error {
	if s.core == nil {
		return nil
	}
	return s.core.Close()
}

func (s *MailService) DetectSettings(ctx context.Context, email string) (autoconfig.Settings, error) {
	return s.core.DetectSettings(ctx, email)
}

func (s *MailService) TestAccount(ctx context.Context, in model.AccountInput) error {
	return s.core.TestAccount(ctx, in)
}

func (s *MailService) AddAccount(ctx context.Context, in model.AccountInput) (model.Account, error) {
	return s.core.AddAccount(ctx, in)
}

func (s *MailService) ListAccounts(ctx context.Context) ([]model.Account, error) {
	return s.core.ListAccounts(ctx)
}

func (s *MailService) RemoveAccount(ctx context.Context, id int64) error {
	return s.core.RemoveAccount(ctx, id)
}

func (s *MailService) ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error) {
	return s.core.ListFolders(ctx, accountID)
}

func (s *MailService) SyncAccount(ctx context.Context, accountID int64) error {
	return s.core.SyncAccount(ctx, accountID)
}

func (s *MailService) SyncFolder(ctx context.Context, folderID int64) error {
	return s.core.SyncFolder(ctx, folderID)
}

func (s *MailService) ListMessages(ctx context.Context, folderID int64, q model.ListQuery) ([]model.MessageSummary, error) {
	return s.core.ListMessages(ctx, folderID, q)
}

func (s *MailService) GetMessage(ctx context.Context, id int64) (model.MessageDetail, error) {
	return s.core.GetMessage(ctx, id)
}

// SaveAttachment abre el diálogo nativo "Guardar como" con el nombre del
// adjunto sugerido. Devuelve saved=false (sin error) si se cancela.
func (s *MailService) SaveAttachment(ctx context.Context, messageID int64, index int) (saved bool, err error) {
	d, err := s.core.GetMessage(ctx, messageID)
	if err != nil {
		return false, err
	}
	var att *model.Attachment
	for i := range d.Attachments {
		if d.Attachments[i].Index == index {
			att = &d.Attachments[i]
			break
		}
	}
	if att == nil {
		return false, errors.New("No encontramos ese adjunto.")
	}
	dest, err := s.app.Dialog.SaveFile().SetFilename(suggestedFilename(att.Filename)).PromptForSingleSelection()
	if err != nil {
		return false, err
	}
	if dest == "" {
		return false, nil
	}
	if err := s.core.SaveAttachment(ctx, messageID, index, dest); err != nil {
		return false, err
	}
	return true, nil
}

func suggestedFilename(name string) string {
	if name == "" {
		return "adjunto"
	}
	return name
}

// OpenExternal abre una URL en el navegador del sistema. Solo http, https y mailto.
func (s *MailService) OpenExternal(link string) error {
	if err := validateExternalURL(link); err != nil {
		return err
	}
	return s.app.Browser.OpenURL(link)
}

// validateExternalURL acepta solo http, https y mailto (con host o destinatario).
func validateExternalURL(link string) error {
	u, err := url.Parse(link)
	if err != nil {
		return errors.New("El enlace no es válido.")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return errors.New("El enlace no es válido.")
		}
	case "mailto":
		if u.Opaque == "" && u.Path == "" && u.RawQuery == "" {
			return errors.New("El enlace no es válido.")
		}
	default:
		return errors.New("Solo se pueden abrir enlaces http, https y mailto.")
	}
	return nil
}

func (s *MailService) PrepareCompose(ctx context.Context, messageID int64, mode model.ComposeMode) (model.ComposeDraft, error) {
	return s.core.PrepareCompose(ctx, messageID, mode)
}

func (s *MailService) Send(ctx context.Context, req model.SendRequest) (model.SendResult, error) {
	return s.core.Send(ctx, req)
}

func (s *MailService) ListLocalDrafts(ctx context.Context) ([]model.LocalDraft, error) {
	return s.core.ListLocalDrafts(ctx)
}

func (s *MailService) DeleteLocalDraft(ctx context.Context, id int64) error {
	return s.core.DeleteLocalDraft(ctx, id)
}

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
	hub  notifyHub
}

func NewMailService() *MailService { return &MailService{} }

func (s *MailService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.app = application.Get()
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
		Notifier: wailsNotifier{app: s.app, hub: &s.hub},
	})
	if err != nil {
		return err
	}
	s.core = core
	core.StartBackgroundSync(ctx, syncEvery)
	core.StartWatching(ctx)
	core.StartIndexing(ctx)
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

// SetSaveSentCopy cambia si se guarda la copia de lo enviado en Enviados.
func (s *MailService) SetSaveSentCopy(ctx context.Context, accountID int64, save bool) error {
	return s.core.SetSaveSentCopy(ctx, accountID, save)
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
	// El núcleo indexa los adjuntos por posición. Si el índice no existe, el
	// propio núcleo devuelve su not_found (sin escribir nada: destino vacío).
	if index < 0 || index >= len(d.Attachments) {
		return false, s.core.SaveAttachment(ctx, messageID, index, "")
	}
	dlg := s.app.Dialog.SaveFile().SetFilename(suggestedFilename(d.Attachments[index].Filename))
	if w := s.app.Window.Current(); w != nil {
		dlg.AttachToWindow(w) // modal respecto de la ventana
	}
	dest, err := dlg.PromptForSingleSelection()
	if err != nil {
		return false, &mail.UserError{Code: "ui", Msg: "No pudimos abrir el diálogo.", Err: err}
	}
	if dest == "" {
		return false, nil
	}
	if err := s.core.SaveAttachment(ctx, messageID, index, dest); err != nil {
		return false, err
	}
	return true, nil
}

// PickAttachments abre el diálogo nativo para elegir archivos a adjuntar y los
// deja preparados. Devuelve una lista vacía (sin error) si se cancela.
func (s *MailService) PickAttachments(ctx context.Context) ([]model.StagedAttachment, error) {
	dlg := s.app.Dialog.OpenFile().CanChooseFiles(true).SetTitle("Adjuntar archivos")
	if w := s.app.Window.Current(); w != nil {
		dlg.AttachToWindow(w)
	}
	paths, err := dlg.PromptForMultipleSelection()
	if err != nil {
		return nil, &mail.UserError{Code: "ui", Msg: "No pudimos abrir el diálogo.", Err: err}
	}
	if len(paths) == 0 {
		return []model.StagedAttachment{}, nil
	}
	return s.core.StageFiles(ctx, paths)
}

// PickImage abre el diálogo nativo para elegir una imagen a insertar en el
// cuerpo. Devuelve un PickedImage vacío (sin error) si se cancela.
func (s *MailService) PickImage(ctx context.Context) (model.PickedImage, error) {
	dlg := s.app.Dialog.OpenFile().CanChooseFiles(true).SetTitle("Insertar imagen").
		AddFilter("Imágenes", "*.png;*.jpg;*.jpeg;*.gif;*.webp")
	if w := s.app.Window.Current(); w != nil {
		dlg.AttachToWindow(w)
	}
	path, err := dlg.PromptForSingleSelection()
	if err != nil {
		return model.PickedImage{}, &mail.UserError{Code: "ui", Msg: "No pudimos abrir el diálogo.", Err: err}
	}
	if path == "" {
		return model.PickedImage{}, nil
	}
	return s.core.LoadImage(ctx, path)
}

// StagedInfo devuelve los datos de los adjuntos preparados ids.
func (s *MailService) StagedInfo(ctx context.Context, ids []int64) ([]model.StagedAttachment, error) {
	return s.core.StagedInfo(ctx, ids)
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
	if err := s.app.Browser.OpenURL(link); err != nil {
		return &mail.UserError{Code: "ui", Msg: "No pudimos abrir el enlace.", Err: err}
	}
	return nil
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

// QueueSend pone el correo en la bandeja de salida y lo envía en segundo
// plano; el resultado llega por el evento mail:send-finished.
func (s *MailService) QueueSend(ctx context.Context, req model.SendRequest) (int64, error) {
	return s.core.QueueSend(ctx, req)
}

func (s *MailService) ListLocalDrafts(ctx context.Context) ([]model.LocalDraft, error) {
	return s.core.ListLocalDrafts(ctx)
}

func (s *MailService) DeleteLocalDraft(ctx context.Context, id int64) error {
	return s.core.DeleteLocalDraft(ctx, id)
}

func (s *MailService) GetSettings(ctx context.Context) (model.Settings, error) {
	return s.core.GetSettings(ctx)
}

func (s *MailService) SetSettings(ctx context.Context, st model.Settings) error {
	return s.core.SetSettings(ctx, st)
}

func (s *MailService) SetSeen(ctx context.Context, ids []int64, seen bool) error {
	return s.core.SetSeen(ctx, ids, seen)
}

func (s *MailService) SetFlagged(ctx context.Context, ids []int64, flagged bool) error {
	return s.core.SetFlagged(ctx, ids, flagged)
}

func (s *MailService) MoveMessages(ctx context.Context, ids []int64, destFolderID int64) error {
	return s.core.MoveMessages(ctx, ids, destFolderID)
}

func (s *MailService) DeleteMessages(ctx context.Context, ids []int64) error {
	return s.core.DeleteMessages(ctx, ids)
}

func (s *MailService) ListUnified(ctx context.Context, view model.UnifiedView, q model.ListQuery) ([]model.MessageSummary, error) {
	return s.core.ListUnified(ctx, view, q)
}

func (s *MailService) SyncUnified(ctx context.Context, view model.UnifiedView) error {
	return s.core.SyncUnified(ctx, view)
}

func (s *MailService) Search(ctx context.Context, q model.SearchQuery) (model.SearchResult, error) {
	return s.core.Search(ctx, q)
}

func (s *MailService) SearchServer(ctx context.Context, folderID int64, text string) ([]model.MessageSummary, error) {
	return s.core.SearchServer(ctx, folderID, text)
}

func (s *MailService) ListThreads(ctx context.Context, folderID int64, q model.ListQuery) ([]model.ThreadSummary, error) {
	return s.core.ListThreads(ctx, folderID, q)
}

func (s *MailService) GetThread(ctx context.Context, folderID int64, threadRoot string) ([]model.MessageSummary, error) {
	return s.core.GetThread(ctx, folderID, threadRoot)
}

func (s *MailService) UpdatePassword(ctx context.Context, accountID int64, password string) error {
	return s.core.UpdatePassword(ctx, accountID, password)
}

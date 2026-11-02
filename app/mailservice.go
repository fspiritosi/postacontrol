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

func (s *MailService) ListSignatures(ctx context.Context) ([]model.Signature, error) {
	return s.core.ListSignatures(ctx)
}

func (s *MailService) SaveSignature(ctx context.Context, sig model.Signature) (model.Signature, error) {
	return s.core.SaveSignature(ctx, sig)
}

func (s *MailService) DeleteSignature(ctx context.Context, id int64) error {
	return s.core.DeleteSignature(ctx, id)
}

// SetAccountSignature fija la firma por defecto de la cuenta (0 = sin firma).
func (s *MailService) SetAccountSignature(ctx context.Context, accountID, signatureID int64) error {
	return s.core.SetAccountSignature(ctx, accountID, signatureID)
}

// ListContacts devuelve las fichas de la libreta; con query, las que coinciden.
func (s *MailService) ListContacts(ctx context.Context, query string) ([]model.Contact, error) {
	return s.core.ListContacts(ctx, query)
}

func (s *MailService) GetContact(ctx context.Context, id int64) (model.Contact, error) {
	return s.core.GetContact(ctx, id)
}

func (s *MailService) SaveContact(ctx context.Context, c model.Contact) (model.Contact, error) {
	return s.core.SaveContact(ctx, c)
}

func (s *MailService) DeleteContact(ctx context.Context, id int64) error {
	return s.core.DeleteContact(ctx, id)
}

// ContactForEmail devuelve la ficha dueña del correo, o nil si nadie lo tiene.
func (s *MailService) ContactForEmail(ctx context.Context, email string) (*model.Contact, error) {
	return s.core.ContactForEmail(ctx, email)
}

// AddSenderToContacts crea la ficha del remitente o devuelve la que ya tiene.
func (s *MailService) AddSenderToContacts(ctx context.Context, addr model.Address) (model.Contact, error) {
	return s.core.AddSenderToContacts(ctx, addr)
}

// SuggestRecipients sugiere direcciones; exclude son las que ya están en el campo.
func (s *MailService) SuggestRecipients(ctx context.Context, query string, exclude []string) ([]model.Suggestion, error) {
	return s.core.SuggestRecipients(ctx, query, exclude)
}

func (s *MailService) ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error) {
	return s.core.ListFolders(ctx, accountID)
}

// CreateFolder crea una carpeta; parentFolderID 0 la deja en el primer nivel.
func (s *MailService) CreateFolder(ctx context.Context, accountID, parentFolderID int64, name string) (model.Folder, error) {
	return s.core.CreateFolder(ctx, accountID, parentFolderID, name)
}

// RenameFolder cambia el último segmento del nombre de la carpeta.
func (s *MailService) RenameFolder(ctx context.Context, folderID int64, name string) (model.Folder, error) {
	return s.core.RenameFolder(ctx, folderID, name)
}

// MoveFolder mueve la carpeta bajo otro padre; 0 es el primer nivel.
func (s *MailService) MoveFolder(ctx context.Context, folderID, newParentFolderID int64) (model.Folder, error) {
	return s.core.MoveFolder(ctx, folderID, newParentFolderID)
}

// DeleteFolderPreview cuenta en el servidor los correos de la carpeta y de sus
// subcarpetas, para el aviso antes de eliminarla.
func (s *MailService) DeleteFolderPreview(ctx context.Context, folderID int64) (model.FolderDeleteInfo, error) {
	return s.core.DeleteFolderPreview(ctx, folderID)
}

// DeleteFolder elimina la carpeta y sus subcarpetas, mandando sus correos a la
// Papelera. Devuelve cuántos correos movió.
func (s *MailService) DeleteFolder(ctx context.Context, folderID int64) (int, error) {
	return s.core.DeleteFolder(ctx, folderID)
}

// SetFolderOrder guarda el orden manual de las carpetas de la cuenta.
func (s *MailService) SetFolderOrder(ctx context.Context, accountID int64, folderIDs []int64) error {
	return s.core.SetFolderOrder(ctx, accountID, folderIDs)
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

// AttachmentData devuelve los bytes del adjunto para el visor (tope de 50 MB).
func (s *MailService) AttachmentData(ctx context.Context, messageID int64, index int) (model.AttachmentData, error) {
	return s.core.AttachmentData(ctx, messageID, index)
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

// imagePatterns es el filtro de los diálogos de imagen. En Linux GTK compara
// los patrones distinguiendo mayúsculas: van las dos formas. El núcleo valida
// por el contenido, no por la extensión.
const imagePatterns = "*.png;*.jpg;*.jpeg;*.gif;*.webp;*.PNG;*.JPG;*.JPEG;*.GIF;*.WEBP"

// PickImage abre el diálogo nativo para elegir una imagen a insertar en el
// cuerpo. Devuelve un PickedImage vacío (sin error) si se cancela.
func (s *MailService) PickImage(ctx context.Context) (model.PickedImage, error) {
	dlg := s.app.Dialog.OpenFile().CanChooseFiles(true).SetTitle("Insertar imagen").
		AddFilter("Imágenes", imagePatterns)
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

// PickBackground abre el diálogo nativo para elegir la imagen de fondo. Devuelve
// false (sin error) si se cancela.
func (s *MailService) PickBackground(ctx context.Context) (bool, error) {
	dlg := s.app.Dialog.OpenFile().CanChooseFiles(true).SetTitle("Elegir fondo").
		AddFilter("Imágenes", imagePatterns)
	if w := s.app.Window.Current(); w != nil {
		dlg.AttachToWindow(w)
	}
	path, err := dlg.PromptForSingleSelection()
	if err != nil {
		return false, &mail.UserError{Code: "ui", Msg: "No pudimos abrir el diálogo.", Err: err}
	}
	if path == "" {
		return false, nil
	}
	return true, s.core.SetBackground(ctx, path)
}

// ClearBackground quita la imagen de fondo.
func (s *MailService) ClearBackground(ctx context.Context) error {
	return s.core.ClearBackground(ctx)
}

// BackgroundImage devuelve el fondo como data URL, o "" si no hay.
func (s *MailService) BackgroundImage(ctx context.Context) (string, error) {
	return s.core.BackgroundImage(ctx)
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

// SaveDraft guarda el borrador del redactor en la compu (id 0 lo crea), sin
// tocar la red.
func (s *MailService) SaveDraft(ctx context.Context, id int64, req model.SendRequest) (model.Draft, error) {
	return s.core.SaveDraft(ctx, id, req)
}

// UploadDraft sube el borrador a la carpeta Borradores del servidor. Si no se
// pudo, el motivo queda en uploadError y needsUpload sigue en true.
func (s *MailService) UploadDraft(ctx context.Context, id int64) (model.Draft, error) {
	return s.core.UploadDraft(ctx, id)
}

// DeleteDraft descarta el borrador, también de la carpeta Borradores del servidor.
func (s *MailService) DeleteDraft(ctx context.Context, id int64) error {
	return s.core.DeleteDraft(ctx, id)
}

// OpenDraftMessage abre para seguir editando un mensaje de la carpeta Borradores.
func (s *MailService) OpenDraftMessage(ctx context.Context, messageID int64) (model.Draft, error) {
	return s.core.OpenDraftMessage(ctx, messageID)
}

// ListPendingDrafts devuelve los borradores que todavía no se subieron al servidor.
func (s *MailService) ListPendingDrafts(ctx context.Context) ([]model.Draft, error) {
	return s.core.ListPendingDrafts(ctx)
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

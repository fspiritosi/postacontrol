package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"postacontrol/core/autoconfig"
	"postacontrol/core/mail"
	"postacontrol/core/model"
	"postacontrol/core/secrets"
)

const syncEvery = 5 * time.Minute

// undoSendDelay es cuánto espera un correo antes de salir, para poder deshacer
// el envío.
const undoSendDelay = 10 * time.Second

// MailService es el servicio que ve el frontend. Delega en mail.Core; los
// errores del núcleo (*mail.UserError) llegan a JS con su mensaje en español,
// porque Wails usa err.Error().
type MailService struct {
	app      *application.App
	core     *mail.Core
	hub      notifyHub
	composes *ComposeWindows
	// spellcheck prende o apaga el corrector; en tests se reemplaza.
	spellcheck func(enabled bool)
	// started se cierra cuando la app terminó de arrancar (core y composes
	// ya puestos). Una segunda instancia puede llegar antes: sus mailto:
	// esperan acá. Leer core y composes después de esperar es seguro.
	started   chan struct{}
	startOnce sync.Once
}

func NewMailService() *MailService {
	return &MailService{spellcheck: ApplySpellcheck, started: make(chan struct{})}
}

// MarkStarted avisa que la app terminó de arrancar: libera los mailto: que
// llegaron antes. Se llama desde ApplicationStarted; llamarlo de nuevo no hace nada.
//
//wails:ignore
func (s *MailService) MarkStarted() {
	s.startOnce.Do(func() {
		if s.started != nil {
			close(s.started)
		}
	})
}

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
		// Los segundos para deshacer un envío (se puede apagar en Redacción).
		UndoSendDelay: undoSendDelay,
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

// UpdateAccount edita una cuenta existente. Contraseña vacía = conservar la guardada.
func (s *MailService) UpdateAccount(ctx context.Context, id int64, in model.AccountInput) (model.Account, error) {
	return s.core.UpdateAccount(ctx, id, in)
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

// dialogParent es la ventana sobre la que se abre un diálogo: la que hizo la
// llamada (Wails la pone en el contexto) o, si no viene, la activa.
func dialogParent(ctx context.Context, current func() application.Window) application.Window {
	if w, ok := ctx.Value(application.WindowKey).(application.Window); ok && w != nil {
		return w
	}
	return current()
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
	if w := dialogParent(ctx, s.app.Window.Current); w != nil {
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
	if w := dialogParent(ctx, s.app.Window.Current); w != nil {
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
	if w := dialogParent(ctx, s.app.Window.Current); w != nil {
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
	if w := dialogParent(ctx, s.app.Window.Current); w != nil {
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

// OpenMailto abre el redactor de PostaControl con los datos de un enlace
// mailto:. accountID es la cuenta remitente; 0 o inexistente, la primera.
func (s *MailService) OpenMailto(ctx context.Context, link string, accountID int64) error {
	f, err := parseMailto(link)
	if err != nil {
		return &mail.UserError{Code: "invalid", Msg: "El enlace no es válido.", Err: err}
	}
	if s.core == nil {
		return &mail.UserError{Code: "unavailable", Msg: "PostaControl todavía está arrancando. Probá de nuevo en un momento."}
	}
	accs, err := s.core.ListAccounts(ctx)
	if err != nil {
		return err
	}
	acc, ok := pickMailtoAccount(accs, accountID)
	if !ok {
		return &mail.UserError{Code: "invalid", Msg: "Primero agregá una cuenta."}
	}
	initJSON, err := mailtoInitJSON(f, acc)
	if err != nil {
		return err
	}
	if s.composes == nil {
		return &mail.UserError{Code: "unavailable", Msg: "No pudimos abrir el redactor."}
	}
	_, err = s.composes.Open(initJSON, "")
	return err
}

// OpenMailtoArgs abre un redactor por cada mailto: de la línea de comandos
// (arranque en frío o segunda instancia). Si la app todavía no arrancó,
// espera a MarkStarted. Los errores se registran sin el enlace, que puede
// traer direcciones y texto del correo.
func (s *MailService) OpenMailtoArgs(args []string) {
	links := MailtoArgs(args)
	if len(links) == 0 {
		return
	}
	if s.started != nil {
		<-s.started
	}
	for _, a := range links {
		if err := s.OpenMailto(context.Background(), a, 0); err != nil {
			slog.Warn("mailto: no se pudo abrir", "err", err)
		}
	}
}

// OpenExternal abre una URL en el navegador del sistema. Solo http y https.
func (s *MailService) OpenExternal(link string) error {
	if err := validateExternalURL(link); err != nil {
		return err
	}
	if err := s.app.Browser.OpenURL(link); err != nil {
		return &mail.UserError{Code: "ui", Msg: "No pudimos abrir el enlace.", Err: err}
	}
	return nil
}

// validateExternalURL acepta solo http y https con host (los mailto: van por OpenMailto).
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
	default:
		return errors.New("Solo se pueden abrir enlaces http y https.")
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

// CancelSend frena un correo que todavía espera para salir: queda en la
// Bandeja de salida para seguir editándolo.
func (s *MailService) CancelSend(ctx context.Context, draftID int64) error {
	return s.core.CancelSend(ctx, draftID)
}

// TrustImageSender muestra siempre las imágenes remotas de ese remitente.
func (s *MailService) TrustImageSender(ctx context.Context, email string) error {
	return s.core.TrustImageSender(ctx, email)
}

// UntrustImageSender vuelve a bloquear las imágenes remotas de ese remitente.
func (s *MailService) UntrustImageSender(ctx context.Context, email string) error {
	return s.core.UntrustImageSender(ctx, email)
}

// ListImageSenders devuelve los remitentes con imágenes remotas permitidas.
func (s *MailService) ListImageSenders(ctx context.Context) ([]string, error) {
	return s.core.ListImageSenders(ctx)
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

// SetComposeWindows le da al servicio el registro de ventanas del redactor.
// Se llama en main, antes de que arranque la app.
//
//wails:ignore
func (s *MailService) SetComposeWindows(r *ComposeWindows) { s.composes = r }

// OpenComposeWindow abre un redactor en su propia ventana con initJSON (lo
// lee con ComposeInit). Si key no está vacía y ya hay una ventana con esa
// clave (por ejemplo "draft:5"), la trae adelante en vez de abrir otra.
func (s *MailService) OpenComposeWindow(initJSON string, key string) error {
	if s.composes == nil {
		return &mail.UserError{Code: "unavailable", Msg: "No pudimos abrir el redactor."}
	}
	_, err := s.composes.Open(initJSON, key)
	return err
}

// ComposeInit devuelve los datos con que arranca el redactor de la ventana id.
func (s *MailService) ComposeInit(id int) (string, error) {
	if s.composes == nil {
		return "", &mail.UserError{Code: "not_found", Msg: "Esta ventana del redactor ya no existe."}
	}
	return s.composes.Init(id)
}

// SetComposeKey registra la clave de la ventana id, para no abrir dos veces
// el mismo borrador.
func (s *MailService) SetComposeKey(id int, key string) {
	if s.composes != nil {
		s.composes.SetKey(id, key)
	}
}

// CloseComposeWindow cierra la ventana del redactor id sin volver a preguntar.
func (s *MailService) CloseComposeWindow(id int) {
	if s.composes != nil {
		s.composes.Close(id)
	}
}

// ComposeFlushed es la respuesta del redactor id a compose:flush: ok si
// guardó todo; si no, message dice por qué.
func (s *MailService) ComposeFlushed(id int, ok bool, message string) {
	if s.composes != nil {
		s.composes.Flushed(id, ok, message)
	}
}

// ComposeSendStarted avisa a todas las ventanas que se empezó a enviar el
// correo draftID de la Bandeja de salida.
func (s *MailService) ComposeSendStarted(draftID int64) {
	if s.app != nil {
		s.app.Event.Emit(EventComposeSendStarted, draftID)
	}
}

// ComposeNotice pasa a todas las ventanas un aviso ("success" o "error") de
// un redactor que se cierra: su propia ventana desaparece antes de mostrarlo.
func (s *MailService) ComposeNotice(kind string, message string) {
	n, ok := composeNotice(kind, message)
	if ok && s.app != nil {
		s.app.Event.Emit(EventComposeNotice, n)
	}
}

func (s *MailService) GetSettings(ctx context.Context) (model.Settings, error) {
	return s.core.GetSettings(ctx)
}

func (s *MailService) SetSettings(ctx context.Context, st model.Settings) error {
	if err := s.core.SetSettings(ctx, st); err != nil {
		return err
	}
	afterSettingsSaved(st, s.spellcheck)
	return nil
}

// SpellcheckEnabled dice si el corrector va prendido; si no se puede leer la
// configuración, prendido (es el valor de fábrica).
func (s *MailService) SpellcheckEnabled() bool {
	if s.core == nil {
		return true
	}
	st, err := s.core.GetSettings(context.Background())
	return err != nil || !st.DisableSpellcheck
}

func (s *MailService) SetSeen(ctx context.Context, ids []int64, seen bool) error {
	return s.core.SetSeen(ctx, ids, seen)
}

func (s *MailService) SetFlagged(ctx context.Context, ids []int64, flagged bool) error {
	return s.core.SetFlagged(ctx, ids, flagged)
}

// Etiquetas: delegan en el núcleo.

func (s *MailService) ListLabels(ctx context.Context) ([]model.Label, error) {
	return s.core.ListLabels(ctx)
}

func (s *MailService) CreateLabel(ctx context.Context, name, color string) (model.Label, error) {
	return s.core.CreateLabel(ctx, name, color)
}

func (s *MailService) UpdateLabel(ctx context.Context, l model.Label) (model.Label, error) {
	return s.core.UpdateLabel(ctx, l)
}

func (s *MailService) ReorderAccounts(ctx context.Context, ids []int64) error {
	return s.core.ReorderAccounts(ctx, ids)
}

func (s *MailService) ReorderLabels(ctx context.Context, ids []int64) error {
	return s.core.ReorderLabels(ctx, ids)
}

func (s *MailService) LabelUsage(ctx context.Context, id int64) (int, error) {
	return s.core.LabelUsage(ctx, id)
}

func (s *MailService) DeleteLabel(ctx context.Context, id int64) error {
	return s.core.DeleteLabel(ctx, id)
}

func (s *MailService) SetLabel(ctx context.Context, ids []int64, labelID int64, on bool) error {
	return s.core.SetLabel(ctx, ids, labelID, on)
}

func (s *MailService) ListLabeled(ctx context.Context, labelID, accountID int64, q model.ListQuery) ([]model.MessageSummary, error) {
	return s.core.ListLabeled(ctx, labelID, accountID, q)
}

func (s *MailService) RefreshLabel(ctx context.Context, labelID, accountID int64) error {
	return s.core.RefreshLabel(ctx, labelID, accountID)
}

// ListRules devuelve las reglas de la cuenta, en el orden en que se aplican.
func (s *MailService) ListRules(ctx context.Context, accountID int64) ([]model.Rule, error) {
	return s.core.ListRules(ctx, accountID)
}

// SaveRule crea (ID 0) o actualiza una regla.
func (s *MailService) SaveRule(ctx context.Context, r model.Rule) (model.Rule, error) {
	return s.core.SaveRule(ctx, r)
}

func (s *MailService) DeleteRule(ctx context.Context, id int64) error {
	return s.core.DeleteRule(ctx, id)
}

func (s *MailService) ReorderRules(ctx context.Context, accountID int64, ids []int64) error {
	return s.core.ReorderRules(ctx, accountID, ids)
}

// RunRules aplica ya las reglas de la cuenta a todos los correos de la carpeta.
func (s *MailService) RunRules(ctx context.Context, folderID int64) (model.RuleRun, error) {
	return s.core.RunRules(ctx, folderID)
}

// MoveMessages mueve los mensajes a otra carpeta y devuelve con qué
// deshacerlo (UndoMove; 0 = nada que deshacer).
func (s *MailService) MoveMessages(ctx context.Context, ids []int64, destFolderID int64) (int64, error) {
	return s.core.MoveMessagesUndoable(ctx, ids, destFolderID)
}

// DeleteMessages manda los mensajes a la Papelera (los que ya estaban ahí se
// borran definitivamente) y devuelve con qué deshacer lo que fue a la Papelera.
func (s *MailService) DeleteMessages(ctx context.Context, ids []int64) (int64, error) {
	return s.core.DeleteMessagesUndoable(ctx, ids)
}

// ArchiveMessages mueve los mensajes a la carpeta Archivo de su cuenta (la
// crea si no hay) y devuelve con qué deshacerlo.
func (s *MailService) ArchiveMessages(ctx context.Context, ids []int64) (int64, error) {
	return s.core.ArchiveMessagesUndoable(ctx, ids)
}

// UndoMove devuelve a su carpeta lo que movió una de las acciones de arriba.
func (s *MailService) UndoMove(ctx context.Context, undoID int64) error {
	return s.core.UndoMove(ctx, undoID)
}

// EmptyTrash borra definitivamente todos los correos de la Papelera y
// devuelve cuántos borró.
func (s *MailService) EmptyTrash(ctx context.Context, folderID int64) (int, error) {
	return s.core.EmptyTrash(ctx, folderID)
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

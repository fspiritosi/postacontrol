// Package model define los tipos de dominio compartidos por todo el núcleo.
// No depende de Wails ni de ningún otro paquete del proyecto.
package model

import "time"

type Security string // "tls" | "starttls"

const (
	SecurityTLS      Security = "tls"
	SecuritySTARTTLS Security = "starttls"
)

type ServerSettings struct {
	Host     string   `json:"host"`
	Port     int      `json:"port"`
	Security Security `json:"security"`
}

type Address struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Account struct {
	ID           int64          `json:"id"`
	Email        string         `json:"email"`
	DisplayName  string         `json:"displayName"`
	Username     string         `json:"username"`
	Color        string         `json:"color"`
	IMAP         ServerSettings `json:"imap"`
	SMTP         ServerSettings `json:"smtp"`
	SaveSentCopy bool           `json:"saveSentCopy"`
	// DefaultSignatureID es la firma que se pone sola al redactar (0 = sin firma).
	DefaultSignatureID int64 `json:"defaultSignatureId"`
}

// Signature es una firma reutilizable, en HTML.
type Signature struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	HTML string `json:"html"`
}

// Contact es una ficha de la libreta local.
type Contact struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Emails  []string `json:"emails"` // en orden; nunca nil
	Phones  []string `json:"phones"` // nunca nil
	Company string   `json:"company"`
	Notes   string   `json:"notes"`
}

// Suggestion es una dirección sugerida al escribir un destinatario.
type Suggestion struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	ContactID int64  `json:"contactId"` // 0 = no es contacto
}

// AccountInput es lo que llega desde la UI. Account.ID se ignora.
type AccountInput struct {
	Account
	Password string `json:"password"`
}

type FolderRole string // "inbox","sent","drafts","trash","junk","archive",""

// UnifiedView es una vista que junta carpetas de todas las cuentas.
type UnifiedView string

const (
	UnifiedInbox   UnifiedView = "inbox"
	UnifiedFlagged UnifiedView = "flagged"
	UnifiedSent    UnifiedView = "sent"
	UnifiedDrafts  UnifiedView = "drafts"
	UnifiedTrash   UnifiedView = "trash"
)

type Folder struct {
	ID         int64      `json:"id"`
	AccountID  int64      `json:"accountId"`
	Name       string     `json:"name"`
	Delimiter  string     `json:"delimiter"`
	Role       FolderRole `json:"role"`
	Unread     int        `json:"unread"`
	Total      int        `json:"total"`
	Selectable bool       `json:"selectable"`
	// Position es el orden manual entre hermanos; 0 = sin posición.
	Position int `json:"position"`
}

// FolderDeleteInfo es lo que se avisa antes de eliminar una carpeta, contado
// en el servidor. Total es solo el de la carpeta; el total general lo suma la
// UI.
type FolderDeleteInfo struct {
	Total      int              `json:"total"`
	Subfolders []SubfolderCount `json:"subfolders"`
}

// SubfolderCount es una subcarpeta que se elimina con su padre. Name es la
// ruta debajo del padre (por ejemplo "Acme" o "Acme/2025").
type SubfolderCount struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
}

type MessageSummary struct {
	ID             int64     `json:"id"`
	AccountID      int64     `json:"accountId"`
	FolderID       int64     `json:"folderId"`
	UID            uint32    `json:"uid"`
	From           Address   `json:"from"`
	Subject        string    `json:"subject"`
	Snippet        string    `json:"snippet"`
	Date           time.Time `json:"date"`
	Seen           bool      `json:"seen"`
	Flagged        bool      `json:"flagged"`
	Answered       bool      `json:"answered"`
	HasAttachments bool      `json:"hasAttachments"`
}

// NewMailItem es un correo nuevo dentro de un aviso NewMail.
type NewMailItem struct {
	MessageID int64  `json:"messageId"`
	FromName  string `json:"fromName"`
	FromEmail string `json:"fromEmail"`
	Subject   string `json:"subject"`
}

// NewMail es el aviso de correo nuevo no leído en una Entrada. Count es el
// total; Items lleva como mucho 3, los más nuevos primero.
type NewMail struct {
	AccountID int64         `json:"accountId"`
	FolderID  int64         `json:"folderId"`
	Count     int           `json:"count"`
	Items     []NewMailItem `json:"items"`
}

// AttachmentData es el contenido de un adjunto para mostrarlo en el visor.
type AttachmentData struct {
	Filename string `json:"filename"`
	MIMEType string `json:"mimeType"`
	Data     []byte `json:"data"`
}

type Attachment struct {
	Index    int    `json:"index"`
	Filename string `json:"filename"`
	MIMEType string `json:"mimeType"`
	Size     int64  `json:"size"`
	Part     []int  `json:"part"`
}

type MessageDetail struct {
	MessageSummary
	MessageID  string    `json:"messageId"`
	InReplyTo  string    `json:"inReplyTo"`
	References []string  `json:"references"`
	To         []Address `json:"to"`
	Cc         []Address `json:"cc"`
	ReplyTo    []Address `json:"replyTo"`
	// HTML ya viene sanitizado (imágenes remotas en data-remote-src).
	HTML            string       `json:"html"`
	Text            string       `json:"text"`
	HasRemoteImages bool         `json:"hasRemoteImages"`
	Attachments     []Attachment `json:"attachments"`
}

type ComposeMode string // "new","reply","replyAll","forward"

type ReplyContext struct {
	Mode              ComposeMode `json:"mode"`
	OriginalMessageID int64       `json:"originalMessageId"`
	// InReplyTo y References son los encabezados de hilo de la respuesta
	// (vacíos al reenviar). Los arma PrepareCompose y viajan con el pedido:
	// si el original ya no está (se movió o se borró) la respuesta sale igual,
	// sin la cita pero en el mismo hilo.
	InReplyTo  string   `json:"inReplyTo"`
	References []string `json:"references"`
}

type SendRequest struct {
	AccountID int64         `json:"accountId"`
	To        []Address     `json:"to"`
	Cc        []Address     `json:"cc"`
	Bcc       []Address     `json:"bcc"`
	Subject   string        `json:"subject"`
	BodyHTML  string        `json:"bodyHtml"`
	Reply     *ReplyContext `json:"reply"` // nil si es un correo nuevo
	// DraftID es el correo de la bandeja de salida que se reintenta (0 si es nuevo).
	DraftID int64 `json:"draftId,omitempty"`
	// Attachments son los ids de los adjuntos preparados (StageFiles) que viajan.
	Attachments []int64 `json:"attachments"`
	// SkipOriginalAttachments son los índices de los adjuntos del original que
	// un reenvío no incluye.
	SkipOriginalAttachments []int `json:"skipOriginalAttachments"`
	// SignatureID es la firma elegida en el redactor (0 = ninguna).
	SignatureID int64 `json:"signatureId"`
	// ComposeDraftID es el borrador autoguardado del que sale el envío (0 si no hay).
	ComposeDraftID int64 `json:"composeDraftId"`
	// DraftMessageID es el Message-ID del borrador del que viene el envío.
	DraftMessageID string `json:"draftMessageId"`
}

// Draft es un borrador autoguardado del redactor: se sube a la carpeta
// Borradores del servidor mientras NeedsUpload sea true.
type Draft struct {
	ID          int64       `json:"id"`
	Request     SendRequest `json:"request"`
	MessageID   string      `json:"messageId"`
	NeedsUpload bool        `json:"needsUpload"`
	UploadError string      `json:"uploadError"`
	UpdatedAt   time.Time   `json:"updatedAt"`
}

// StagedAttachment es un archivo elegido para adjuntar y guardado a la espera
// del envío.
type StagedAttachment struct {
	ID       int64  `json:"id"`
	Filename string `json:"filename"`
	MIMEType string `json:"mimeType"`
	Size     int64  `json:"size"`
}

// PickedImage es una imagen elegida del disco, lista para insertar en el
// cuerpo. DataURL queda vacío si el usuario canceló.
type PickedImage struct {
	DataURL  string `json:"dataUrl"`
	Filename string `json:"filename"`
}

type ComposeDraft struct {
	AccountID  int64        `json:"accountId"`
	To         []Address    `json:"to"`
	Cc         []Address    `json:"cc"`
	Subject    string       `json:"subject"`
	QuotedHTML string       `json:"quotedHtml"` // se muestra debajo del editor; no editable
	Reply      ReplyContext `json:"reply"`
	// SignatureID es la firma por defecto de la cuenta (0 = sin firma).
	SignatureID int64 `json:"signatureId"`
	// OriginalAttachments son los adjuntos del original; solo al reenviar (en
	// los otros modos va vacío).
	OriginalAttachments []Attachment `json:"originalAttachments"`
}

type SendResult struct {
	SentCopySaved bool   `json:"sentCopySaved"`
	Warning       string `json:"warning"`
}

// Estados de un correo de la bandeja de salida.
const (
	DraftSending = "sending" // en camino; si la app se cierra, pasa a failed al volver
	DraftFailed  = "failed"
)

// LocalDraft es un correo de la bandeja de salida: entra antes de enviarse y
// sale cuando el servidor lo acepta.
type LocalDraft struct {
	ID        int64       `json:"id"`
	CreatedAt time.Time   `json:"createdAt"`
	Request   SendRequest `json:"request"`
	Status    string      `json:"status"` // DraftSending o DraftFailed
	Error     string      `json:"error"`
}

type ListQuery struct {
	UnreadOnly bool `json:"unreadOnly"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
}

// ThreadSummary es un hilo de conversación dentro de una carpeta. Latest es el
// mensaje más nuevo de la carpeta; Count y Unread suman también los de Enviados.
type ThreadSummary struct {
	ThreadRoot   string         `json:"threadRoot"`
	Latest       MessageSummary `json:"latest"`
	Count        int            `json:"count"`
	Unread       int            `json:"unread"`
	Participants []string       `json:"participants"`
}

// SearchQuery es una búsqueda en el índice local. FolderID 0 = todas las
// carpetas de todas las cuentas.
type SearchQuery struct {
	Text     string `json:"text"`
	FolderID int64  `json:"folderId"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

// SearchResult agrupa los resultados y cuántos mensajes del alcance todavía no
// tienen el cuerpo indexado.
type SearchResult struct {
	Messages []MessageSummary `json:"messages"`
	Pending  int              `json:"pending"`
}

// Settings es la configuración de la app. Se guarda como JSON, así sumar
// opciones no exige migrar.
type Settings struct {
	UnifiedInbox bool `json:"unifiedInbox"`
	// DisableNotifications apaga los avisos de escritorio de correo nuevo (cero = activados).
	DisableNotifications bool `json:"disableNotifications"`
	// ThreadedList agrupa la lista de mensajes por conversación (cero = lista plana).
	ThreadedList bool `json:"threadedList"`
	// ReadingPane es dónde se muestra el correo abierto: "right", "bottom" o "top".
	// Vacío (configuración vieja) se lee como "bottom".
	ReadingPane string `json:"readingPane"`
	// Background es el nombre del archivo de fondo dentro de la carpeta de datos; vacío = sin fondo.
	Background string `json:"background"`
}

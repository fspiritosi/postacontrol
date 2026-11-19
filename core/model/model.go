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
	// Color es el color elegido para la carpeta (#rrggbb); vacío = sin color. Es solo local.
	Color string `json:"color"`
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
	// Labels son los ids de las etiquetas del mensaje, por posición; nunca nil.
	Labels []int64 `json:"labels"`
}

// Label es una etiqueta de usuario. Su keyword IMAP ($pc_…) no se expone.
type Label struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Color    string `json:"color"`
	Position int    `json:"position"`
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
	HTML            string `json:"html"`
	Text            string `json:"text"`
	HasRemoteImages bool   `json:"hasRemoteImages"`
	// RemoteImagesTrusted: el remitente está entre los que se eligió mostrarle
	// siempre las imágenes remotas.
	RemoteImagesTrusted bool         `json:"remoteImagesTrusted"`
	Attachments         []Attachment `json:"attachments"`
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

// CalendarPart es la invitación iMIP que viaja con un correo.
type CalendarPart struct {
	Method string `json:"method"`
	ICS    string `json:"ics"`
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
	// SendAt programa el envío para esa hora (segundos unix); 0 o una hora
	// que ya pasó = ahora.
	SendAt int64 `json:"sendAt,omitempty"`
	// Calendar, si viene, manda el correo como invitación (parte text/calendar).
	Calendar *CalendarPart `json:"calendar,omitempty"`
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
	DraftSending   = "sending" // en camino; si la app se cierra, pasa a failed al volver
	DraftFailed    = "failed"
	DraftScheduled = "scheduled" // espera su hora (Request.SendAt); sobrevive a cerrar la app
)

// LocalDraft es un correo de la bandeja de salida: entra antes de enviarse y
// sale cuando el servidor lo acepta.
type LocalDraft struct {
	ID        int64       `json:"id"`
	CreatedAt time.Time   `json:"createdAt"`
	Request   SendRequest `json:"request"`
	Status    string      `json:"status"` // DraftSending, DraftFailed o DraftScheduled
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
	// Labels es la unión de las etiquetas de los mensajes del hilo, por posición; nunca nil.
	Labels []int64 `json:"labels"`
}

// SearchQuery es una búsqueda en el índice local. FolderID 0 = todas las
// carpetas de todas las cuentas.
type SearchQuery struct {
	Text     string `json:"text"`
	FolderID int64  `json:"folderId"`
	LabelID  int64  `json:"labelId"` // 0 = sin filtro por etiqueta
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
	// DisableSpellcheck apaga el corrector ortográfico del redactor (cero = prendido).
	DisableSpellcheck bool `json:"disableSpellcheck"`
	// ThreadedList agrupa la lista de mensajes por conversación (cero = lista plana).
	ThreadedList bool `json:"threadedList"`
	// ReadingPane es dónde se muestra el correo abierto: "right", "bottom" o "top".
	// Vacío (configuración vieja) se lee como "bottom".
	ReadingPane string `json:"readingPane"`
	// Background es el nombre del archivo de fondo dentro de la carpeta de datos; vacío = sin fondo.
	Background string `json:"background"`
	// DisableUndoSend manda los correos enseguida, sin los segundos para
	// deshacer el envío (cero = se puede deshacer).
	DisableUndoSend bool `json:"disableUndoSend"`
	// AutoBackupDir es la carpeta del respaldo automático semanal; vacía =
	// apagado. Se cambia solo con SetAutoBackupDir.
	AutoBackupDir string `json:"autoBackupDir"`
	// Theme es el tema de la UI: "light", "dark" o "system". Vacío
	// (configuración vieja) se lee como "system".
	Theme string `json:"theme"`
	// TaskReminderMinutes es la anticipación por defecto del aviso de una
	// tarea con hora; nil = 15.
	TaskReminderMinutes *int `json:"taskReminderMinutes"`
	// EventReminderMinutes es la anticipación por defecto del aviso de un
	// evento; nil = 15.
	EventReminderMinutes *int `json:"eventReminderMinutes"`
	// TaskappAPIURL y TaskappWebURL son las direcciones de Taskapp; vacías =
	// las de producción.
	TaskappAPIURL string `json:"taskappApiUrl"`
	TaskappWebURL string `json:"taskappWebUrl"`
}

// TaskappStatus dice si hay una cuenta de Taskapp conectada y de quién.
type TaskappStatus struct {
	Connected bool   `json:"connected"`
	UserID    int64  `json:"userId"`
	UserName  string `json:"userName"`
	UserEmail string `json:"userEmail"`
}

// TaskappProject es un proyecto activo de Taskapp.
type TaskappProject struct {
	ID          int64  `json:"id"`
	WorkspaceID int64  `json:"workspaceId"`
	Name        string `json:"name"`
	Color       string `json:"color"`
}

// TaskappWorkspace es un workspace de Taskapp.
type TaskappWorkspace struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// TaskappMember es un miembro de un workspace de Taskapp.
type TaskappMember struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// TaskappMeetingCreate es el pedido de crear una reunión en Taskapp desde el
// calendario; External son los invitados de afuera, que reciben un .ics.
type TaskappMeetingCreate struct {
	WorkspaceID int64     `json:"workspaceId"`
	ProjectID   int64     `json:"projectId"` // 0 = sin proyecto
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	AllDay      bool      `json:"allDay"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AttendeeIDs []int64   `json:"attendeeIds"`
	External    []Address `json:"external"`
	AccountID   int64     `json:"accountId"` // cuenta que manda a los externos
}

// TaskappTask es una tarea o ticket pendiente asignado al usuario.
type TaskappTask struct {
	ID           int64  `json:"id"`
	Ticket       bool   `json:"ticket"`
	Title        string `json:"title"`
	Priority     string `json:"priority"`
	DueDate      string `json:"dueDate"`
	StatusName   string `json:"statusName"`
	ProjectID    int64  `json:"projectId"`
	ProjectName  string `json:"projectName"`
	ProjectColor string `json:"projectColor"`
	URL          string `json:"url"`
	Description  string `json:"description"`
}

// TaskappList es la lista de pendientes; Stale indica que viene de la caché
// porque Taskapp no respondió (Error dice por qué). Tasks nunca es nil.
type TaskappList struct {
	Tasks     []TaskappTask `json:"tasks"`
	FetchedAt time.Time     `json:"fetchedAt"`
	Stale     bool          `json:"stale"`
	Error     string        `json:"error"`
}

// Rule es una regla de una cuenta, como los filtros de Thunderbird: si un
// correo cumple las condiciones (todas o alguna, según MatchAll), se le aplican
// las acciones. Las reglas de una cuenta se evalúan en orden de Position.
type Rule struct {
	ID        int64  `json:"id"`
	AccountID int64  `json:"accountId"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	// MatchAll: true = todas las condiciones; false = alcanza con una.
	MatchAll   bool            `json:"matchAll"`
	Conditions []RuleCondition `json:"conditions"` // nunca nil
	Actions    []RuleAction    `json:"actions"`    // nunca nil
	Position   int             `json:"position"`
}

// RuleCondition compara un campo del correo con un texto, sin distinguir
// mayúsculas.
//
// Field: "from", "to", "cc", "toOrCc" o "subject".
// Op: "contains", "notContains", "is", "isNot", "beginsWith" o "endsWith".
type RuleCondition struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Value string `json:"value"`
}

// RuleAction es lo que se hace con un correo que cumple la regla.
//
// Type: "move" (a FolderID), "markRead", "flag", "label" (LabelID), "delete"
// (a la Papelera) o "stop" (no evaluar las reglas que siguen).
type RuleAction struct {
	Type     string `json:"type"`
	FolderID int64  `json:"folderId"`
	LabelID  int64  `json:"labelId"`
}

// RuleRun es el resultado de aplicar las reglas a mano sobre una carpeta.
type RuleRun struct {
	// Matched es cuántos correos cumplieron al menos una regla.
	Matched int `json:"matched"`
}

// BackupImport es el resumen de importar un respaldo. Lo que ya estaba se
// saltea (Skipped) y no se pisa; Failed es lo que no se pudo restaurar porque
// falta algo de lo que depende (la cuenta, la carpeta o la etiqueta) o porque
// se llegó al máximo.
type BackupImport struct {
	ContactsAdded     int `json:"contactsAdded"`
	ContactsSkipped   int `json:"contactsSkipped"`
	SignaturesAdded   int `json:"signaturesAdded"`
	SignaturesSkipped int `json:"signaturesSkipped"`
	LabelsAdded       int `json:"labelsAdded"`
	LabelsSkipped     int `json:"labelsSkipped"`
	LabelsFailed      int `json:"labelsFailed"`
	RulesAdded        int `json:"rulesAdded"`
	RulesSkipped      int `json:"rulesSkipped"`
	RulesFailed       int `json:"rulesFailed"`
	// Remitentes con imágenes remotas permitidas.
	ImageSendersAdded   int `json:"imageSendersAdded"`
	ImageSendersSkipped int `json:"imageSendersSkipped"`
	// Plantillas del redactor (las de mismo nombre se saltean).
	TemplatesAdded   int `json:"templatesAdded"`
	TemplatesSkipped int `json:"templatesSkipped"`
	// Tareas locales (las de mismo título, fecha y hora se saltean).
	TasksAdded   int `json:"tasksAdded"`
	TasksSkipped int `json:"tasksSkipped"`
	// Eventos del calendario (los de una UID que ya está se saltean).
	EventsAdded   int `json:"eventsAdded"`
	EventsSkipped int `json:"eventsSkipped"`
}

// ICSCalendar es un calendario de un archivo .ics (o de una entrada de un zip)
// listo para importar.
type ICSCalendar struct {
	ID     string `json:"id"`     // ruta, o "ruta#entrada" para un zip
	Name   string `json:"name"`   // X-WR-CALNAME, o el nombre del archivo
	Events int    `json:"events"` // cantidad de series
}

// ICSImportResult cuenta lo que dejó importar .ics.
type ICSImportResult struct{ Added, Updated, Skipped int }

// TaskReminder es el aviso de una tarea próxima; At es su fecha y hora, no el
// momento del aviso.
type TaskReminder struct {
	TaskID int64     `json:"taskId"`
	Title  string    `json:"title"`
	At     time.Time `json:"at"`
}

// EventReminder es el aviso de un evento o reunión próximos; At es el inicio de
// la ocurrencia (el movido, si se movió), no el momento del aviso.
type EventReminder struct {
	Key    string    `json:"key"` // la Key del CalendarItem
	Title  string    `json:"title"`
	At     time.Time `json:"at"`
	AllDay bool      `json:"allDay"`
}

// AutoBackupInfo es el estado del respaldo automático: la carpeta elegida
// (vacía = apagado) y la fecha (AAAA-MM-DD) de la última copia que hay ahí.
type AutoBackupInfo struct {
	Dir      string `json:"dir"`
	LastDate string `json:"lastDate"`
}

// Unsubscribe es cómo desuscribirse de un correo, según sus encabezados
// List-Unsubscribe. OneClick: el remitente acepta la baja con un POST a URL
// (RFC 8058). URL es la página (https o http) y Mailto la dirección de baja.
type Unsubscribe struct {
	Available bool   `json:"available"`
	OneClick  bool   `json:"oneClick"`
	URL       string `json:"url"`
	Mailto    string `json:"mailto"`
}

// UnsubscribeResult es cómo terminó la baja. Method: "post" (confirmada con un
// clic), "mail" (se mandó el correo de baja) o "web" (hay que terminarla en
// la página URL, que abre el usuario).
type UnsubscribeResult struct {
	Method string `json:"method"`
	URL    string `json:"url"`
}

// Snooze es un correo pospuesto: vuelve a ReturnFolder a la hora Until
// (segundos unix). MessageID es su encabezado Message-ID.
type Snooze struct {
	ID           int64  `json:"id"`
	AccountID    int64  `json:"accountId"`
	MessageID    string `json:"messageId"`
	ReturnFolder string `json:"returnFolder"`
	Until        int64  `json:"until"`
}

// Template es una plantilla (o respuesta rápida) del redactor. Subject, si no
// está vacío, completa el asunto cuando el correo todavía no tiene.
type Template struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
}

// MessageSource es el mensaje tal como está en el servidor, para verlo. Si es
// muy largo, Text trae solo el principio y Truncated lo dice.
type MessageSource struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

// LocalTask es una tarea propia de Postacontrol (no vive en Taskapp).
type LocalTask struct {
	ID              int64      `json:"id"`
	Title           string     `json:"title"`
	Notes           string     `json:"notes"`
	DueDate         string     `json:"dueDate"`         // "AAAA-MM-DD" o ""
	DueTime         string     `json:"dueTime"`         // "HH:MM" o ""; solo con DueDate
	RemindBefore    *int       `json:"remindBefore"`    // minutos; nil = el de Configuración
	SourceMessageID string     `json:"sourceMessageId"` // Message-ID del correo de origen o ""
	SourceSubject   string     `json:"sourceSubject"`
	DoneAt          *time.Time `json:"doneAt"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// TaskLink vincula un hilo de correo con una tarea o ticket de Taskapp.
type TaskLink struct {
	ID            int64     `json:"id"`
	ThreadRoot    string    `json:"threadRoot"`
	TaskID        int64     `json:"taskId"`
	Ticket        bool      `json:"ticket"`
	ProjectName   string    `json:"projectName"`
	Title         string    `json:"title"`
	AttachPending bool      `json:"attachPending"` // la tarea existe pero el .eml no se adjuntó
	CreatedAt     time.Time `json:"createdAt"`
	// Los completa core/mail, no el store:
	URL          string `json:"url"`
	AlreadyAdded bool   `json:"alreadyAdded"` // el correo consultado ya se sumó
	Completed    bool   `json:"completed"`
	StateKnown   bool   `json:"stateKnown"` // false = no se pudo preguntar a Taskapp
}

// TaskappDraft es lo que se propone al crear una tarea o ticket desde un correo.
type TaskappDraft struct {
	MessageID       int64  `json:"messageId"`
	Title           string `json:"title"`
	DescriptionHTML string `json:"descriptionHtml"`
	ProjectID       int64  `json:"projectId"` // 0 = sin sugerencia
	Oversize        bool   `json:"oversize"`  // el .eml no entra en MaxUpload: se adjunta sin adjuntos
}

// TaskappCreate es el pedido de crear una tarea o ticket desde un correo.
type TaskappCreate struct {
	MessageID       int64  `json:"messageId"`
	Ticket          bool   `json:"ticket"`
	ProjectID       int64  `json:"projectId"`
	Title           string `json:"title"`
	DescriptionHTML string `json:"descriptionHtml"`
	Priority        string `json:"priority"` // none|low|medium|high|urgent
	DueDate         string `json:"dueDate"`  // solo tarea
}

// Attendee es un invitado de un evento.
type Attendee struct {
	Email    string `json:"email"` // minúscula
	Name     string `json:"name"`
	Partstat string `json:"partstat"` // NEEDS-ACTION, ACCEPTED, TENTATIVE o DECLINED
}

// EventException modifica o cancela una ocurrencia de una serie.
type EventException struct {
	RecurrenceAt time.Time `json:"recurrenceAt"` // inicio original de la ocurrencia
	Cancelled    bool      `json:"cancelled"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Title        string    `json:"title"`
	Location     string    `json:"location"`
	Description  string    `json:"description"`
}

// CalendarEvent es un evento del calendario (local, invitación o espejo de Taskapp).
type CalendarEvent struct {
	ID               int64            `json:"id"`
	UID              string           `json:"uid"`
	Kind             string           `json:"kind"` // "local", "invite" o "taskapp" (espejo de externos)
	Title            string           `json:"title"`
	Description      string           `json:"description"` // viaja a los invitados
	Notes            string           `json:"notes"`       // privadas, nunca salen
	Location         string           `json:"location"`
	AllDay           bool             `json:"allDay"`
	Start            time.Time        `json:"start"`
	End              time.Time        `json:"end"` // exclusivo
	TZID             string           `json:"tzid"`
	RRule            string           `json:"rrule"`
	RemindBefore     *int             `json:"remindBefore"` // nil = el de Configuración
	AccountID        int64            `json:"accountId"`    // cuenta que manda o responde; 0 = ninguna
	Organizer        Address          `json:"organizer"`
	Sequence         int              `json:"sequence"`
	Status           string           `json:"status"`     // "confirmed" o "cancelled"
	MyPartstat       string           `json:"myPartstat"` // solo en "invite"
	MyEmail          string           `json:"myEmail"`
	AliasReply       bool             `json:"aliasReply"` // ninguna cuenta coincide con el ATTENDEE
	NeedsReview      bool             `json:"needsReview"`
	RawICS           string           `json:"-"`
	TaskappMeetingID int64            `json:"taskappMeetingId"`
	TaskappChange    string           `json:"taskappChange"` // "", "changed", "cancelled" o "deleted"
	Attendees        []Attendee       `json:"attendees"`
	Exceptions       []EventException `json:"exceptions"`
}

// CalendarItem es una ocurrencia de lo que se muestra en el calendario.
type CalendarItem struct {
	Key           string    `json:"key"`    // "e:<id>:<inicio original unix>", "m:<reunión>" o "t:<tarea>"
	Source        string    `json:"source"` // "local", "invite", "taskapp" o "task"
	EventID       int64     `json:"eventId"`
	MeetingID     int64     `json:"meetingId"`
	TaskID        int64     `json:"taskId"`
	Title         string    `json:"title"`
	Location      string    `json:"location"`
	Start         time.Time `json:"start"`
	End           time.Time `json:"end"`
	AllDay        bool      `json:"allDay"`
	Recurring     bool      `json:"recurring"`
	OccurrenceAt  time.Time `json:"occurrenceAt"` // inicio original (para "solo este")
	Color         string    `json:"color"`        // color del proyecto (Taskapp)
	ProjectName   string    `json:"projectName"`
	Partstat      string    `json:"partstat"`
	NeedsReview   bool      `json:"needsReview"`
	Cancelled     bool      `json:"cancelled"`
	Attendees     int       `json:"attendees"`
	Confirmed     int       `json:"confirmed"`
	TaskappChange string    `json:"taskappChange"`
	URL           string    `json:"url"` // Taskapp
}

// CalendarRange es lo que se muestra en el calendario para un rango.
type CalendarRange struct {
	Items            []CalendarItem `json:"items"` // nunca nil, ordenado por Start
	TaskappStale     bool           `json:"taskappStale"`
	TaskappFetchedAt time.Time      `json:"taskappFetchedAt"`
	TaskappError     string         `json:"taskappError"`
}

// EventSave es el pedido de guardar un evento.
type EventSave struct {
	Event        CalendarEvent `json:"event"`        // ID 0 = nuevo
	Scope        string        `json:"scope"`        // "" = todo el evento o la serie, "this" o "following"
	OccurrenceAt time.Time     `json:"occurrenceAt"` // la ocurrencia, si Scope != ""
	Notify       bool          `json:"notify"`
}

// EventDelete es el pedido de borrar un evento o parte de una serie.
type EventDelete struct {
	ID           int64     `json:"id"`
	Scope        string    `json:"scope"`
	OccurrenceAt time.Time `json:"occurrenceAt"`
	Notify       bool      `json:"notify"`
}

// SavePreview dice, antes de guardar o borrar un evento, si hay que avisarles
// a los invitados y a cuántos se agrega o se quita.
type SavePreview struct {
	NotifyNeeded bool `json:"notifyNeeded"` // hay invitados y cambió algo que les importa
	Added        int  `json:"added"`
	Removed      int  `json:"removed"`
}

// MessageInvite es lo que muestra la tarjeta de invitación de un correo.
type MessageInvite struct {
	Found       bool      `json:"found"`
	Method      string    `json:"method"`  // REQUEST, CANCEL o REPLY
	EventID     int64     `json:"eventId"` // 0 si no quedó en el calendario
	Title       string    `json:"title"`
	Location    string    `json:"location"`
	Organizer   Address   `json:"organizer"`
	Start       time.Time `json:"start"`
	End         time.Time `json:"end"`
	AllDay      bool      `json:"allDay"`
	Recurring   bool      `json:"recurring"`
	Partstat    string    `json:"partstat"`
	NeedsReview bool      `json:"needsReview"`
	Cancelled   bool      `json:"cancelled"`
	Past        bool      `json:"past"`
	Outdated    bool      `json:"outdated"` // el calendario tiene una versión más nueva
	AliasReply  bool      `json:"aliasReply"`
	ReplyFrom   Address   `json:"replyFrom"` // en un REPLY: quién respondió
	ReplyStatus string    `json:"replyStatus"`
}

// RespondResult es lo que deja responder una invitación.
type RespondResult struct {
	Warning string `json:"warning"`
}

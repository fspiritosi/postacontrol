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
}

// AccountInput es lo que llega desde la UI. Account.ID se ignora.
type AccountInput struct {
	Account
	Password string `json:"password"`
}

type FolderRole string // "inbox","sent","drafts","trash","junk","archive",""

type Folder struct {
	ID         int64      `json:"id"`
	AccountID  int64      `json:"accountId"`
	Name       string     `json:"name"`
	Delimiter  string     `json:"delimiter"`
	Role       FolderRole `json:"role"`
	Unread     int        `json:"unread"`
	Total      int        `json:"total"`
	Selectable bool       `json:"selectable"`
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
}

type SendRequest struct {
	AccountID int64         `json:"accountId"`
	To        []Address     `json:"to"`
	Cc        []Address     `json:"cc"`
	Bcc       []Address     `json:"bcc"`
	Subject   string        `json:"subject"`
	BodyHTML  string        `json:"bodyHtml"`
	Reply     *ReplyContext `json:"reply"` // nil si es un correo nuevo
}

type ComposeDraft struct {
	AccountID  int64        `json:"accountId"`
	To         []Address    `json:"to"`
	Cc         []Address    `json:"cc"`
	Subject    string       `json:"subject"`
	QuotedHTML string       `json:"quotedHtml"` // se muestra debajo del editor; no editable
	Reply      ReplyContext `json:"reply"`
}

type SendResult struct {
	SentCopySaved bool   `json:"sentCopySaved"`
	Warning       string `json:"warning"`
}

type LocalDraft struct {
	ID        int64       `json:"id"`
	CreatedAt time.Time   `json:"createdAt"`
	Request   SendRequest `json:"request"`
	Error     string      `json:"error"`
}

type ListQuery struct {
	UnreadOnly bool `json:"unreadOnly"`
	Offset     int  `json:"offset"`
	Limit      int  `json:"limit"`
}

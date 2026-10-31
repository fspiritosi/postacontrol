package mail

// Nombres de eventos que el núcleo emite por Notifier.
const (
	EventAccountsChanged = "mail:accounts-changed" // data: nil
	EventFoldersUpdated  = "mail:folders-updated"  // data: {"accountId": int64}
	EventFolderUpdated   = "mail:folder-updated"   // data: {"folderId": int64}
	EventSyncProgress    = "mail:sync-progress"    // data: {"folderId": int64, "done": int, "total": int}
	EventSyncError       = "mail:sync-error"       // data: {"accountId": int64, "message": string}
	// EventNewMail: llegó correo no leído a una Entrada (nunca en la primera sincronización).
	EventNewMail         = "mail:new-mail"         // data: model.NewMail
	EventSettingsChanged = "mail:settings-changed" // data: nil
	EventOutboxUpdated   = "mail:outbox-updated"   // data: nil
	// EventSendFinished: terminó un envío. message es el motivo si falló y
	// warning un aviso si salió (p. ej. sin copia en Enviados).
	EventSendFinished = "mail:send-finished" // data: {"draftId": int64, "ok": bool, "message": string, "warning": string}
)

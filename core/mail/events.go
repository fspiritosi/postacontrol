package mail

// Nombres de eventos que el núcleo emite por Notifier.
const (
	EventAccountsChanged = "mail:accounts-changed" // data: nil
	EventFoldersUpdated  = "mail:folders-updated"  // data: {"accountId": int64}
	EventFolderUpdated   = "mail:folder-updated"   // data: {"folderId": int64}
	EventSyncProgress    = "mail:sync-progress"    // data: {"folderId": int64, "done": int, "total": int}
	EventSyncError       = "mail:sync-error"       // data: {"accountId": int64, "message": string}
)

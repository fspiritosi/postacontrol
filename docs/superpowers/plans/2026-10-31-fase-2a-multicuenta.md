# PostaControl — Fase 2A "Multicuenta y acciones" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Fabricio use sus 4 cuentas en PostaControl. Cada cuenta tiene su árbol de carpetas, hay una bandeja unificada opcional y se puede mover, borrar, destacar y marcar como leído o no leído.

**Architecture:** Se extiende lo que ya existe sin cambiar la forma.
- **Núcleo:** `core/mail.Core` suma las acciones sobre mensajes (con un `UIDVALIDITY` verificado antes de escribir), las consultas unificadas por rol, la configuración y una pausa para las cuentas con la contraseña rechazada. Las escrituras IMAP nuevas viven en `core/imapx`.
- **Frontend:** el estado de navegación pasa de "carpeta + bandeja de salida" a una sola `View`, que puede ser una carpeta, una vista unificada o la bandeja de salida. La barra lateral muestra una sección por cuenta.

**Tech Stack:** el de la Fase 1 (ver `docs/superpowers/plans/2026-10-30-fase-1-primer-correo.md`). Se suman los componentes shadcn `switch` y `collapsible`.

**Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md` (secciones "Pantalla principal", "Fase 2").

**La Fase 2 se partió en tres planes:**
- **2A (este):** multicuenta, bandeja unificada, acciones y configuración.
- **2B:** IMAP IDLE, notificaciones, bandeja del sistema e inicio al abrir sesión.
- **2C:** búsqueda local (FTS5) y conversaciones.

Cada uno deja la app usable. En 2A la sincronización periódica cada 5 minutos sigue reemplazando a IDLE.

## Global Constraints

- Todas las de la Fase 1 siguen vigentes, salvo la lista de escrituras permitidas, que se amplía abajo. Entre ellas: `core/` sin imports de Wails, contraseñas solo en el llavero, TLS siempre verificado, tests solo contra `core/testenv`, textos en español rioplatense con voseo, `task check` verde antes de cada commit y commits convencionales en español con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Escrituras al servidor permitidas en 2A**, todas por acción explícita del usuario: `STORE ±\Seen`, `STORE ±\Flagged`, `MOVE` (o `COPY`+`STORE \Deleted`+`UID EXPUNGE` solo si hay UIDPLUS) y `UID EXPUNGE` de mensajes que ya están en la Papelera. Se suman a las de la Fase 1.
- **Nunca se usa `EXPUNGE` sin UID.** Borraría también lo que otro cliente marcó como `\Deleted`. Si el servidor no tiene `MOVE` ni `UIDPLUS` (ni IMAP4rev2), mover y borrar devuelven un error claro y no tocan nada.
- **Antes de cualquier escritura por UID** se abre la carpeta con `selectChecked(conn, f, last, true)`. Si el `UIDVALIDITY` cambió, se devuelve "stale" sin escribir.
- **Bandeja unificada** desactivada por defecto (spec). La configuración vive en SQLite (`settings`). La última vista abierta y las secciones plegadas se guardan en `localStorage`, igual que el tamaño de los paneles (`pc:layout`).
- **Bindings:** se regeneran con `task common:generate:bindings` (TypeScript), nunca con `wails3 generate bindings` a secas, que genera JS.

## Review Focus

1. **Mover o borrar con la carpeta cambiada en el servidor:** otro cliente la recreó o ya movió el mensaje. Se espera un error "stale" sin tocar ningún otro mensaje, y que la próxima sincronización ponga la caché al día. Lo cubre `TestMoveMessages_StaleFolderRefuses` (Task 4).
2. **Servidor sin `MOVE` ni `UIDPLUS`:** se espera que mover y borrar se rechacen con un mensaje en español y sin `EXPUNGE` general. Lo cubre `TestMove_Rev1ServerIsUnsupported` (Task 2).
3. **Cuenta sin carpeta Papelera:** "Borrar" no puede convertirse en un borrado definitivo. Se espera el error `no_trash`. Lo cubre `TestDeleteMessages_NoTrashErrors` (Task 4).
4. **Una de las 4 cuentas con la contraseña cambiada:** las otras tres siguen sincronizando, y la rechazada no se reintenta cada 5 minutos (riesgo de bloqueo en Hostinger). Lo cubre `TestBackgroundSync_SkipsAccountInAuthPause` (Task 6).
5. **La última vista guardada apunta a una cuenta o carpeta que ya no existe** (cuenta quitada, carpeta renombrada), o a una vista unificada con la opción apagada. Se espera caer en la Entrada de la primera cuenta, sin una lista vacía rota. Lo cubre `viewIsValid` en `view.test.ts` (Task 8).

---

## File Structure

```
core/
  model/model.go            # + Settings, UnifiedView
  store/
    schema_v3.sql           # tabla settings (Task 1)
    settings.go             # GetSettings / SaveSettings (Task 1)
    messages.go             # + MessageRefs, DeleteMessages, RecountFolder, ListByRole, ListFlagged (Tasks 3-5)
  imapx/write.go            # + StoreFlags, Move, ExpungeUIDs, ErrUnsupported (Task 2)
  testenv/imap.go           # + StartIMAPRev1 (Task 2)
  mail/
    settings.go             # Core.GetSettings / SetSettings (Task 1)
    actions.go              # SetSeen, SetFlagged, MoveMessages, DeleteMessages (Tasks 3-4)
    unified.go              # ListUnified, SyncUnified (Task 5)
    authpause.go            # pausa por credenciales rechazadas + UpdatePassword (Task 6)
app/mailservice.go          # bindings nuevos (Task 7)
frontend/src/
  lib/view.ts               # View, guardar/restaurar, validar (Task 8)
  lib/queries.ts            # useMessages(view), useSettings, useAllFolders (Task 8)
  features/layout/MailShell.tsx      # estado = View (Task 8)
  features/folders/
    FolderSidebar.tsx       # barra lateral: unificada, salida, una sección por cuenta (Task 9)
    AccountSection.tsx      # cabecera + menú + árbol de una cuenta (Task 9)
  features/settings/SettingsDialog.tsx   # Configuración (Task 9)
  features/messages/MessageList.tsx      # estrella por fila, color por cuenta (Tasks 8, 10)
  features/reader/MessageActions.tsx     # Destacar, No leído, Mover a…, Borrar (Task 10)
  features/accounts/PasswordDialog.tsx   # Cambiar contraseña (Task 11)
```

---

### Task 1: Configuración en el núcleo

**Files:**
- Create: `core/store/schema_v3.sql`, `core/store/settings.go`, `core/mail/settings.go`
- Modify: `core/store/migrate.go` (agregar `schemaV3`), `core/model/model.go`, `core/mail/events.go`
- Test: `core/store/store_test.go`, `core/mail/settings_test.go`

**Interfaces:**
- Produces:
  - `type Settings struct { UnifiedInbox bool \`json:"unifiedInbox"\` }` en `model`.
  - `(*store.Store).GetSettings(ctx) (model.Settings, error)`, que devuelve el valor cero si no hay fila.
  - `(*store.Store).SaveSettings(ctx, model.Settings) error`.
  - `(*mail.Core).GetSettings(ctx) (model.Settings, error)`.
  - `(*mail.Core).SetSettings(ctx, model.Settings) error`, que emite `EventSettingsChanged = "mail:settings-changed"` (data nil).

- [ ] **Step 1: Tests que fallan**

```go
// core/store/store_test.go
func TestSettings_DefaultsAndRoundTrip(t *testing.T) {
	s := newStore(t)
	ctx := context.Background()
	got, err := s.GetSettings(ctx)
	require.NoError(t, err)
	assert.Equal(t, model.Settings{}, got) // bandeja unificada apagada por defecto
	require.NoError(t, s.SaveSettings(ctx, model.Settings{UnifiedInbox: true}))
	require.NoError(t, s.SaveSettings(ctx, model.Settings{UnifiedInbox: true})) // upsert, no duplica
	got, err = s.GetSettings(ctx)
	require.NoError(t, err)
	assert.True(t, got.UnifiedInbox)
}

// core/mail/settings_test.go
func TestSetSettings_PersistsAndNotifies(t *testing.T) {
	c, _, _, n := newTestCore(t)
	require.NoError(t, c.SetSettings(context.Background(), model.Settings{UnifiedInbox: true}))
	got, err := c.GetSettings(context.Background())
	require.NoError(t, err)
	assert.True(t, got.UnifiedInbox)
	assert.Contains(t, n.names(), EventSettingsChanged)
}
```

- [ ] **Step 2:** `go test ./core/store/ ./core/mail/ -run 'Settings'`. Debe fallar porque las funciones no existen.
- [ ] **Step 3: Implementar.**
  - `schema_v3.sql`: `CREATE TABLE settings (id INTEGER PRIMARY KEY CHECK (id = 1), json TEXT NOT NULL);`
  - La configuración se guarda como JSON en la fila 1, así sumar opciones en 2B no exige migrar.
  - `TestOpen_MigratesAndIsIdempotent` ya compara contra `len(migrations)`.
- [ ] **Step 4:** `go test ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): configuración persistente con la bandeja unificada`

---

### Task 2: Escrituras IMAP: flags en lote, mover y expunge por UID

**Files:**
- Modify: `core/imapx/write.go`, `core/testenv/imap.go`
- Test: `core/imapx/imapx_test.go`

**Interfaces:**
- Produces, en `imapx`:
  - `var ErrUnsupported = errors.New("imapx: el servidor no lo soporta")`
  - `(*Conn).StoreFlags(uids []uint32, add bool, flags ...imap.Flag) error`: `+FLAGS.SILENT` o `-FLAGS.SILENT`. Requiere `SelectRW` y si no, `ErrReadOnly`. `AddFlags(uid, ...)` pasa a llamar a `StoreFlags([]uint32{uid}, true, ...)`.
  - `(*Conn).Move(uids []uint32, dest string) error`: usa `imapclient.Move` solo si las capacidades tienen `MOVE`, `IMAP4rev2` o `UIDPLUS`; si no, `ErrUnsupported`. Requiere `SelectRW`.
  - `(*Conn).ExpungeUIDs(uids []uint32) error`: `STORE +\Deleted` y luego `UID EXPUNGE`. Exige `UIDPLUS` o `IMAP4rev2`; si no, `ErrUnsupported`.
- Produces, en `testenv`: `StartIMAPRev1(t, tlsc, user, pass) *IMAP`. Es igual a `StartIMAP` pero anuncia solo `IMAP4rev1`.

- [ ] **Step 1: Tests que fallan**

```go
func TestStoreFlags_AddAndRemove(t *testing.T) // 2 mensajes en INBOX; SelectRW; StoreFlags(both,true,\Flagged) → FetchFlags tiene \Flagged en los dos; StoreFlags([u1],false,\Flagged) → u1 sin, u2 con.
func TestStoreFlags_ReadOnlyFails(t *testing.T) // Select (EXAMINE) → ErrorIs(err, ErrReadOnly)
func TestMove_MovesToDestination(t *testing.T) // crea "Trash"; Move([u1], "Trash") → AllUIDs(INBOX) sin u1; Select("Trash") tiene 1 mensaje
func TestExpungeUIDs_OnlyGivenUIDs(t *testing.T) // u1,u2 en INBOX; otro cliente marca u2 \Deleted (imapmemserver session); ExpungeUIDs([u1]) → AllUIDs == [u2] (u2 sobrevive aunque tenga \Deleted)
func TestMove_Rev1ServerIsUnsupported(t *testing.T) // StartIMAPRev1; Move y ExpungeUIDs → ErrorIs(err, ErrUnsupported); el mensaje sigue en INBOX
```

- [ ] **Step 2:** `go test ./core/imapx/ -run 'StoreFlags|Move|Expunge'`. Debe fallar porque las funciones no existen.
- [ ] **Step 3: Implementar.** Las capacidades salen de `c.c.Caps()`. Los errores del servidor se envuelven con `opErr` como el resto del paquete.
- [ ] **Step 4:** `go test ./core/imapx/ ./core/testenv/`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(imapx): flags en lote, mover y expunge por UID`

---

### Task 3: Marcar leído/no leído y destacar

**Files:**
- Create: `core/mail/actions.go`, `core/mail/actions_test.go`
- Modify: `core/store/messages.go`
- Test: `core/store/store_test.go`

**Interfaces:**
- Consumes: `imapx.StoreFlags` (Task 2) y `selectChecked` (`read.go`).
- Produces, en `store`:
  - `type MessageRef struct { ID, AccountID, FolderID int64; UID uint32; Seen, Flagged, Answered bool }`
  - `(*Store).MessageRefs(ctx, ids []int64) ([]MessageRef, error)`: devuelve `ErrNotFound` si falta alguno.
  - `(*Store).RecountFolder(ctx, folderID int64) error`: recalcula unread y total desde la caché.
- Produces, en `mail`:
  - `(*Core).SetSeen(ctx, ids []int64, seen bool) error`
  - `(*Core).SetFlagged(ctx, ids []int64, flagged bool) error`
  - helper interno `groupByFolder(refs []store.MessageRef) (accountID int64, byFolder map[int64][]store.MessageRef, err error)`, que también usa la Task 4.
- **Reglas:**
  - Todos los ids tienen que ser de la misma cuenta. Si no, `UserError{Code:"invalid", Msg:"Elegí correos de una sola cuenta."}`.
  - Para cada carpeta: `selectChecked(..., true)`, luego `StoreFlags`, luego `SetFlags` en la caché, `RecountFolder` y `EventFolderUpdated`.
  - La escritura es idempotente, así que se usa `withConn`.
  - Con `ids` vacío, devuelve nil sin conectar.

- [ ] **Step 1: Tests que fallan**

```go
func TestSetSeen_MarksUnreadOnServerAndCache(t *testing.T) // 2 mensajes leídos; SetSeen([m1], false) → servidor sin \Seen en m1, caché Seen=false, Folder.Unread == 1, evento folder-updated
func TestSetFlagged_RoundTrip(t *testing.T)                // SetFlagged([m1,m2], true) → servidor y caché con \Flagged; SetFlagged([m1], false) → solo m2
func TestSetSeen_MixedAccountsRefused(t *testing.T)         // segunda cuenta (servidores testenv nuevos, usuario "g@ej.com"); ids de las dos → Code "invalid", sin cambios en ninguno
func TestSetFlagged_StaleFolderRefuses(t *testing.T)        // imap.RecreateMailbox("INBOX") tras sincronizar → Code "stale", el servidor no recibe STORE
```

- [ ] **Step 2:** `go test ./core/mail/ -run 'SetSeen|SetFlagged'`. Debe fallar porque las funciones no existen.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test -race ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): marcar leído/no leído y destacar`

---

### Task 4: Mover y borrar

**Files:**
- Modify: `core/mail/actions.go`, `core/store/messages.go`, `core/mail/errors.go` (traducir `imapx.ErrUnsupported`)
- Test: `core/mail/actions_test.go`, `core/store/store_test.go`

**Interfaces:**
- Consumes: `imapx.Move`, `imapx.ExpungeUIDs`, `groupByFolder` y `MessageRefs` (Tasks 2 y 3).
- Produces:
  - `(*store.Store).DeleteMessages(ctx, ids []int64) error`
  - `(*Core).MoveMessages(ctx, ids []int64, destFolderID int64) error`
  - `(*Core).DeleteMessages(ctx, ids []int64) error`
- **Reglas:**
  - **La carpeta destino tiene que ser de la misma cuenta y seleccionable.** Si no, `invalid`. Si los mensajes ya están en el destino, no se hace nada.
  - **`DeleteMessages`:**
    - Fuera de la Papelera, mueve a la carpeta con rol `trash` de la cuenta.
    - Dentro de la Papelera, borra definitivamente con `ExpungeUIDs`.
    - Si la cuenta no tiene Papelera: `UserError{Code:"no_trash", Msg:"Esta cuenta no tiene carpeta Papelera."}`.
  - **`ErrUnsupported`** se traduce a `UserError{Code:"unsupported", Msg:"El servidor no permite mover ni borrar correos de forma segura."}`.
  - **Mover y borrar no son idempotentes:** se usa `withConnNoRetry`, precedido de un `Noop` con `withConn`, como `appendSent`.
  - **Después de escribir en el servidor:**
    - Se borran las filas de la caché (`store.DeleteMessages`) y se hace `RecountFolder` del origen.
    - Se emite `EventFolderUpdated` para el origen y para el destino.
    - Se sincroniza el destino en segundo plano con `c.spawn(... SyncFolder(dest))`.

- [ ] **Step 1: Tests que fallan**

```go
func TestMoveMessages_MovesAndResyncsDestination(t *testing.T) // crea "Archivo"; MoveMessages([m1], archivo) → INBOX en servidor y caché sin m1; tras esperar la sync del destino, la caché de Archivo tiene el asunto de m1
func TestMoveMessages_OtherAccountFolderRefused(t *testing.T)  // destino de otra cuenta → "invalid", nada se mueve
func TestMoveMessages_StaleFolderRefuses(t *testing.T)         // RecreateMailbox("INBOX") tras sincronizar → "stale", la carpeta destino sigue vacía
func TestDeleteMessages_MovesToTrash(t *testing.T)             // crea "Trash" (rol trash); DeleteMessages([m1]) → m1 en Trash del servidor, fuera de INBOX
func TestDeleteMessages_InTrashIsPermanent(t *testing.T)       // m en Trash; DeleteMessages([m]) → Trash vacía en servidor y caché; ninguna otra carpeta lo recibe
func TestDeleteMessages_NoTrashErrors(t *testing.T)            // sin carpeta Trash → "no_trash"; el mensaje sigue en INBOX y no se borró
func TestMoveMessages_UnsupportedServer(t *testing.T)          // cuenta sobre StartIMAPRev1 → "unsupported"
```

- [ ] **Step 2:** `go test ./core/mail/ -run 'MoveMessages|DeleteMessages'`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test -race ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): mover y borrar correos (a Papelera o definitivo desde la Papelera)`

---

### Task 5: Vistas unificadas

**Files:**
- Create: `core/mail/unified.go`, `core/mail/unified_test.go`
- Modify: `core/store/messages.go` (un solo armador de consultas para `ListMessages`, `ListByRole` y `ListFlagged`), `core/model/model.go`
- Test: `core/store/store_test.go`

**Interfaces:**
- Produces:
  - `type UnifiedView string` en `model`, con las constantes `UnifiedInbox="inbox"`, `UnifiedFlagged="flagged"`, `UnifiedSent="sent"`, `UnifiedDrafts="drafts"`, `UnifiedTrash="trash"`. Es la lista del spec: Entrada, Destacados, Enviados, Borradores, Papelera.
  - `(*store.Store).ListByRole(ctx, role model.FolderRole, q model.ListQuery) ([]model.MessageSummary, error)`: junta las carpetas con ese rol de todas las cuentas.
  - `(*store.Store).ListFlagged(ctx, q model.ListQuery) ([]model.MessageSummary, error)`: destacados de cualquier carpeta y cuenta.
  - `(*Core).ListUnified(ctx, view model.UnifiedView, q model.ListQuery) ([]model.MessageSummary, error)`: una vista desconocida da `invalid` y Limit 0 da 100, como en `ListMessages`.
  - `(*Core).SyncUnified(ctx, view model.UnifiedView) error`:
    - Sincroniza en cada cuenta las carpetas de ese rol. Para `flagged`, sincroniza las Entradas.
    - Corre una cuenta tras otra.
    - Si una cuenta falla, sigue con la siguiente; el error ya lo avisa `withConn`.
    - Solo devuelve el error del contexto.

- [ ] **Step 1: Tests que fallan**

```go
func TestListByRole_MergesAccountsByDate(t *testing.T)  // store: 2 cuentas con inbox y fechas intercaladas → orden por fecha desc, AccountID correcto en cada uno; UnreadOnly filtra
func TestListFlagged_AcrossFolders(t *testing.T)        // flagged en INBOX de A y en "Varios" de B → ambos, y nada más
func TestListUnified_UnknownViewInvalid(t *testing.T)   // core: "spam" → Code "invalid"
func TestSyncUnified_OneAccountDownOthersSync(t *testing.T) // 2 cuentas; imap de A cerrado (Close); SyncUnified(inbox) → nil, la Entrada de B tiene sus mensajes, evento sync-error para A
```

- [ ] **Step 2:** `go test ./core/... -run 'ListByRole|ListFlagged|Unified'`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): vistas unificadas por rol y destacados`

---

### Task 6: Pausa por contraseña rechazada y cambio de contraseña

**Files:**
- Create: `core/mail/authpause.go`, `core/mail/authpause_test.go`
- Modify: `core/mail/core.go` (`useConn` registra la falla; campos nuevos), `core/mail/sync.go` (`StartBackgroundSync` saltea las cuentas en pausa)

**Interfaces:**
- Produces:
  - `const authPause = 30 * time.Minute`
  - `(*Core).UpdatePassword(ctx, accountID int64, password string) error`:
    - Prueba IMAP y SMTP con los servidores guardados de la cuenta, vía `TestAccount`.
    - Si pasa: guarda la contraseña en el llavero, descarta la conexión del pool (`dropConn`), levanta la pausa y lanza `SyncAccount` en segundo plano (salvo con `SkipInitialSync`).
    - Si la prueba falla, no cambia nada.
    - Con la contraseña vacía devuelve `invalid`.
- **Reglas:**
  - Cuando `useConn` termina con un `UserError` de código `auth`, se anota `authFailedAt[accountID] = now`.
  - La sincronización periódica saltea las cuentas con una falla de hace menos de `authPause`.
  - Lo que pide el usuario (`SyncAccount` desde la UI, abrir una carpeta) siempre intenta.
  - El reloj se inyecta con un campo `now func() time.Time` (por defecto `time.Now`) para testear.

- [ ] **Step 1: Tests que fallan**

```go
func TestBackgroundSync_SkipsAccountInAuthPause(t *testing.T) // 2 cuentas; A con contraseña mala en el llavero en memoria; c.now fijo; una pasada de sync en segundo plano (exponer la pasada como c.backgroundPass(ctx) para testearla sin ticker) → A intentó 1 vez (1 evento sync-error con accountId de A); segunda pasada → sigue habiendo 1 solo evento sync-error de A, y B sincronizó las dos veces; avanzar c.now 31 min → A vuelve a intentar
func TestUpdatePassword_WrongKeepsOld(t *testing.T)          // UpdatePassword con contraseña mala → Code "auth"; el llavero conserva la anterior
func TestUpdatePassword_ClearsPause(t *testing.T)            // A en pausa; UpdatePassword correcta → llavero actualizado y la siguiente backgroundPass sincroniza A
```

- [ ] **Step 2:** `go test ./core/mail/ -run 'AuthPause|UpdatePassword'`. Debe fallar.
- [ ] **Step 3: Implementar.** Se extrae el cuerpo del bucle de `StartBackgroundSync` a `func (c *Core) backgroundPass(ctx context.Context)`.
- [ ] **Step 4:** `go test -race ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): pausa de 30 min tras una contraseña rechazada y cambio de contraseña`

---

### Task 7: Exponer lo nuevo al frontend

**Files:**
- Modify: `app/mailservice.go`
- Regenerate: `frontend/bindings/` (`task common:generate:bindings`)

**Interfaces:**
- Produces: métodos de `MailService` que delegan uno a uno en `Core`:
  - `GetSettings`, `SetSettings`
  - `SetSeen(ids []int64, seen bool)`, `SetFlagged(ids []int64, flagged bool)`
  - `MoveMessages(ids []int64, destFolderID int64)`, `DeleteMessages(ids []int64)`
  - `ListUnified(view model.UnifiedView, q model.ListQuery)`, `SyncUnified(view model.UnifiedView)`
  - `UpdatePassword(accountID int64, password string)`
  - Todos reciben `ctx context.Context` primero, igual que los existentes.

- [ ] **Step 1:** Agregar los métodos y correr `go build . && task common:generate:bindings`.
- [ ] **Step 2:** `task check`. Debe dar PASS. `git diff --stat frontend/bindings` muestra solo `.ts`.
- [ ] **Step 3: Commit** `feat(app): exponer acciones, vistas unificadas, configuración y contraseña`

---

### Task 8: Navegación por `View`

**Files:**
- Create: `frontend/src/lib/view.ts`, `frontend/src/lib/view.test.ts`
- Modify:
  - `frontend/src/features/layout/MailShell.tsx`
  - `frontend/src/lib/queries.ts`
  - `frontend/src/lib/events.ts`
  - `frontend/src/App.tsx`
  - `frontend/src/features/messages/MessageList.tsx`
  - `frontend/src/features/reader/MessageView.tsx`
  - `frontend/src/features/outbox/OutboxList.tsx`
  - los tests que construyen `ShellState`

**Interfaces:**
- Produces, en `view.ts`:
  - `type UnifiedRole = "inbox" | "flagged" | "sent" | "drafts" | "trash"`
  - `type View = { kind: "folder"; folderId: number } | { kind: "unified"; role: UnifiedRole } | { kind: "outbox" }`
  - `UNIFIED_LABEL: Record<UnifiedRole, string>` = Entrada, Destacados, Enviados, Borradores, Papelera.
  - `loadView(): View | null` y `saveView(v: View): void`, sobre `localStorage` con la clave `"pc:view"`. Los dos van con try/catch, como `loadLayout`.
  - `viewIsValid(v: View, folders: Folder[], unifiedEnabled: boolean): boolean`
  - `defaultView(folders: Folder[]): View | null`, que da la Entrada seleccionable de la primera cuenta (por `accountId` menor).
- Produces, en `ShellState` (reemplaza a `accountId`, `folderId`, `outbox`, `selectFolder` y `openOutbox`):
  - `{ view: View | null; messageId: number | null; selectView(v: View): void; selectMessage(id: number | null): void }`
  - `selectView` guarda la vista y limpia `messageId`.
- Produces, en `queries.ts`:
  - `useAllFolders(accounts: Account[])`: con `useQueries`, devuelve `{ folders: Folder[]; pending: boolean }`.
  - `useSettings()`
  - `useMessages(view: View | null, unreadOnly)`, con la clave `["messages", folderId, unreadOnly]` o `["messages", "unified", role, unreadOnly]`.
- Produces, en `events.ts`:
  - `mail:folder-updated` invalida además `["messages", "unified"]`.
  - `mail:settings-changed` invalida `["settings"]`.
- **Comportamiento:**
  - **Al arrancar**, `MailShell` usa `loadView()` si `viewIsValid`; si no, `defaultView`.
  - **Si la vista actual deja de ser válida** (se quitó la cuenta, se apagó la unificada), vuelve a `defaultView`.
  - **`MessageList`:**
    - Muestra el color de la cuenta de cada mensaje (`m.accountId`) en lugar del de una cuenta fija.
    - El encabezado dice `UNIFIED_LABEL[role]` o el nombre de la carpeta.
  - **Al elegir una vista unificada** se llama a `svc.SyncUnified(role)`, sin await, igual que `syncFolder`.
  - **"Redactar"** usa la cuenta de la carpeta abierta, o la primera cuenta si la vista es unificada o la bandeja de salida.
  - **`MessageView`** busca la cuenta del mensaje en la lista de cuentas.

- [ ] **Step 1: Tests que fallan.** `view.test.ts`:
  - `viewIsValid` falla para una carpeta que no existe y para una unificada con `unifiedEnabled=false`.
  - `viewIsValid` pasa para `{kind:"outbox"}`.
  - `defaultView` elige la Entrada de la cuenta de id menor y devuelve null sin carpetas.
  - `loadView` devuelve null con JSON inválido o con `localStorage` que tira error.

  Además, en `MessageList.test.tsx`: "en vista unificada muestra el color de la cuenta de cada mensaje y el título Entrada".
- [ ] **Step 2:** `npx vitest run src/lib/view.test.ts src/features/messages`. Debe fallar.
- [ ] **Step 3: Implementar.** También se adaptan los tests existentes que arman `ShellState`.
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): navegación por vista (carpeta, unificada o bandeja de salida)`

---

### Task 9: Barra lateral multicuenta y Configuración

**Files:**
- Create:
  - `frontend/src/features/folders/AccountSection.tsx`
  - `frontend/src/features/settings/SettingsDialog.tsx`
  - `frontend/src/features/settings/SettingsDialog.test.tsx`
- Modify:
  - `frontend/src/features/folders/FolderSidebar.tsx` (pasa a recibir `accounts: Account[]`)
  - `frontend/src/features/folders/FolderSidebar.test.tsx`
  - `frontend/src/App.tsx`
- Add: `npx shadcn@latest add switch collapsible` (desde `frontend/`).

**Interfaces:**
- Consumes: `View`, `ShellState`, `useAllFolders`, `useSettings` (Task 8), `svc.SetSettings` (Task 7).
- **La barra lateral, de arriba abajo:**
  1. **"Bandeja de salida":** igual que ahora, pero con `selectView({kind:"outbox"})`.
  2. **"Todas las cuentas"** (solo con `unifiedInbox`): las 5 vistas unificadas.
     - Entrada muestra la suma de no leídos de las Entradas.
     - Se ocultan las vistas sin ninguna carpeta con ese rol, salvo Destacados.
  3. **Una `AccountSection` por cuenta:**
     - Cabecera plegable con el punto de color, el nombre y los no leídos de su Entrada.
     - Menú "…" con Actualizar, Guardar copia en Enviados, Cambiar contraseña (lo conecta la Task 11) y Quitar cuenta.
     - Debajo, su árbol de carpetas (el `FolderRow` actual).
     - Las secciones plegadas se recuerdan en `localStorage` (`"pc:collapsed"`, la lista de ids de cuenta).
     - **Con la unificada activada**, las secciones de cuenta arrancan plegadas.
  4. **Al pie:** botones "Agregar cuenta" (abre el `AccountSetupDialog` existente) y "Configuración" (abre `SettingsDialog`).
- **`SettingsDialog`:** un `Switch` "Bandeja unificada", con el texto de ayuda "Muestra arriba las carpetas de todas las cuentas juntas." Cambiarlo llama a `SetSettings` al instante, sin botón Guardar; el diálogo se cierra con "Cerrar".
- **Al quitar una cuenta**, si la vista era de esa cuenta, `MailShell` cae en `defaultView` (Task 8).

- [ ] **Step 1: Tests que fallan.**
  - `FolderSidebar.test.tsx`:
    - "muestra una sección por cuenta con su árbol"
    - "con la bandeja unificada muestra Todas las cuentas y suma los no leídos de las Entradas"
    - "plegar una cuenta oculta sus carpetas y se recuerda"
    - "Agregar cuenta abre el alta"
  - `SettingsDialog.test.tsx`: "el interruptor guarda la bandeja unificada".
  - Se adaptan los dos tests existentes del menú de la cuenta.
- [ ] **Step 2:** `npx vitest run src/features/folders src/features/settings`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): barra lateral con varias cuentas, bandeja unificada y Configuración`

---

### Task 10: Acciones sobre mensajes en el lector y en la lista

**Files:**
- Create: `frontend/src/features/reader/MessageActions.tsx`, `frontend/src/features/reader/MessageActions.test.tsx`
- Modify: `frontend/src/features/reader/MessageView.tsx` (barra de acciones), `frontend/src/features/messages/MessageList.tsx` (estrella por fila)

**Interfaces:**
- Consumes: `svc.SetSeen`, `SetFlagged`, `MoveMessages`, `DeleteMessages` (Task 7) y `useFolders` (Fase 1).
- **Barra del lector:** Responder, Responder a todos y Reenviar (como hoy), más:
  - **Destacar / Quitar destacado:** ícono de estrella con `aria-pressed`.
  - **Marcar como no leído:** después de la llamada hace `selectMessage(null)`. Si no, al volver a abrir el mensaje `GetMessage` lo marcaría leído de nuevo.
  - **Mover a…:** `DropdownMenu` con las carpetas seleccionables de la cuenta del mensaje, salvo la actual, con las etiquetas de `folderLabel`.
  - **Borrar:**
    - Fuera de la Papelera, sin confirmación; el mensaje va a la Papelera.
    - En una carpeta con rol `trash`, el botón dice "Eliminar definitivamente" y pide confirmación: "¿Eliminar definitivamente? No se puede deshacer."
    - Después de mover o borrar, `selectMessage(null)` y un toast: "Movido a <carpeta>" o "Movido a la Papelera".
- **Errores:** `toast.error(errorMessage(e))`.
- **Lista:** cada fila muestra una estrella clicable (`aria-label` "Destacar" o "Quitar destacado"). Llama a `SetFlagged` sin abrir el mensaje y con `stopPropagation`.

- [ ] **Step 1: Tests que fallan.** `MessageActions.test.tsx`:
  - "Marcar como no leído llama SetSeen(false) y cierra el lector"
  - "Mover a… lista las carpetas de la cuenta salvo la actual y mueve"
  - "Borrar fuera de Papelera no pide confirmación"
  - "en Papelera pide confirmación y dice Eliminar definitivamente"
  - "Destacar alterna"

  Además, en `MessageList.test.tsx`: "la estrella destaca sin abrir el mensaje".
- [ ] **Step 2:** `npx vitest run src/features/reader src/features/messages`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): destacar, marcar no leído, mover y borrar`

---

### Task 11: Cambiar contraseña desde la app

**Files:**
- Create: `frontend/src/features/accounts/PasswordDialog.tsx`, `frontend/src/features/accounts/PasswordDialog.test.tsx`
- Modify: `frontend/src/features/folders/AccountSection.tsx` (opción del menú), `frontend/src/lib/events.ts` (acción en el toast de error)

**Interfaces:**
- Consumes: `svc.UpdatePassword` (Task 7).
- **El diálogo:**
  - Título "Cambiar contraseña", con el correo de la cuenta como descripción.
  - Un campo de contraseña y el botón "Probar y guardar" (con "Probando…" mientras corre).
  - Si falla, el error queda adentro del diálogo; si sale bien, se cierra con el toast "Contraseña actualizada".
- **El toast de error de sincronización:** cuando el mensaje es el de credenciales rechazadas (`"El servidor rechazó el usuario o la contraseña."`), suma la acción "Cambiar contraseña", que abre el diálogo para esa cuenta.
  - Como el toast vive fuera del árbol de la barra lateral, `events.ts` expone `onPasswordRequest(cb)` / `requestPassword(accountId)`, un emisor simple de módulo que escucha `AccountSection`.

- [ ] **Step 1: Tests que fallan.**
  - `PasswordDialog.test.tsx`: "guarda y cierra" y "muestra el error sin cerrar".
  - En `events.test.ts`: "el error de credenciales ofrece Cambiar contraseña".
- [ ] **Step 2:** `npx vitest run src/features/accounts src/lib/events.test.ts`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): cambiar la contraseña de una cuenta`

---

### Task 12: Prueba de aceptación con las 4 cuentas reales (Fabricio)

Es manual, con `task dev` en la ventana nativa (no en el navegador).

- [ ] **Alta:** sumar las otras 3 cuentas, entre ellas la de "otro hosting". Cada una tiene su color y su árbol.
- [ ] **Configuración:** activar la bandeja unificada.
  - La Entrada muestra correos de todas las cuentas por fecha, con el punto de color correcto.
  - Apagarla vuelve al árbol por cuenta.
- [ ] **Reinicio:** cerrar y abrir la app. Vuelve a la misma vista.
- [ ] **Acciones:** sobre un correo de prueba propio:
  - destacar y quitar el destacado
  - marcar como no leído
  - moverlo a otra carpeta
  - borrarlo (a la Papelera)
  - en la Papelera, eliminarlo definitivamente

  Revisar en el webmail de Hostinger que cada cambio se haya reflejado.
- [ ] **Contraseña:** "Cambiar contraseña" con una contraseña mala. Da el error y la cuenta sigue andando con la anterior.
- [ ] **Entre carpetas:** responder desde la unificada. El correo sale desde la cuenta que recibió el original.

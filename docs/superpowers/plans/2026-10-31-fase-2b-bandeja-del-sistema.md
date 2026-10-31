# PostaControl — Fase 2B "Correo al instante y bandeja del sistema" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que el correo nuevo llegue al instante y avise con una notificación del escritorio, y que la app viva en la bandeja del sistema. Cerrar la ventana la esconde, "Salir" cierra de verdad y hay una opción de iniciar al abrir sesión.

**Architecture:**
- **Núcleo (`core/`, sin Wails):**
  - Un "vigía" por cuenta mantiene una conexión IMAP propia en IDLE sobre la Entrada. Es independiente de la conexión del pool.
  - Cuando el servidor avisa, sincroniza la Entrada por el pool y emite `mail:new-mail` con los correos nuevos no leídos.
  - La sincronización cada 5 minutos queda como respaldo.
- **Cáscara (`app/`):**
  - Traduce `mail:new-mail` en una notificación nativa (servicio `notifications` de Wails, D-Bus en Linux).
  - Arma la bandeja del sistema, esconde la ventana al cerrar y garantiza una sola instancia.
  - Expone el inicio al abrir sesión (`app.Autostart`).

**Tech Stack:** el de las fases 1 y 2A, más `github.com/wailsapp/wails/v3/pkg/services/notifications`, `application.SystemTray`, `application.SingleInstanceOptions`, `app.Autostart` (todo de Wails v3.0.0) e `imapclient.Client.Idle` de go-imap v2.

**Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`:
- "Ciclo de vida de la app"
- "Comportamiento": IDLE y notificaciones "desde la Fase 2"
- "Fase 2"

Este plan es 2B, la segunda de las tres partes en que se dividió la Fase 2 (ver `docs/superpowers/plans/2026-10-31-fase-2a-multicuenta.md`).

## Global Constraints

- **Siguen vigentes todas las de la Fase 1 y la 2A:**
  - `core/` sin imports de Wails
  - contraseñas solo en el llavero
  - TLS siempre verificado
  - tests solo contra `core/testenv`
  - textos en español rioplatense con voseo
  - `task check` verde antes de cada commit
  - commits convencionales en español con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`
  - bindings con `task common:generate:bindings`
- **IDLE no escribe nada en el servidor:** el vigía abre la Entrada en solo lectura (`EXAMINE`).
- **Conexión propia del vigía:** nunca usa ni bloquea la conexión del pool (`withConn`). Un IDLE ocupa la sesión entera.
- **Pausa por contraseña:** el vigía respeta la de la 2A (`authPause`). Con una contraseña rechazada no reintenta el login hasta que la pausa vence o el usuario cambia la contraseña.
- **Al cerrar la ventana,** la app no termina: se esconde y sigue sincronizando y avisando.
  - Se sale de verdad con "Salir" del menú de la bandeja o con Ctrl+Q en la ventana.
- **"Iniciar al abrir sesión"** viene desactivado por defecto (spec). Al iniciar con la sesión, la app arranca escondida en la bandeja.
- **Ids de la app:**
  - id de aplicación: `ar.com.codecontrol.postacontrol` (Task 0 de la 2A)
  - id de instancia única: el mismo
  - identificador del autostart: `postacontrol`

## Review Focus

1. **Primera sincronización de una carpeta grande, o de una cuenta recién agregada:** cientos de correos "nuevos" que no tienen que disparar ninguna notificación. Lo cubre `TestNewMail_NotOnFirstSync` (Task 2).
2. **Llegan 20 correos de golpe:** se espera una sola notificación ("20 correos nuevos"), no 20. Lo cubren `TestNewMail_SummarizesBurst` (Task 2) y `TestNewMailNotification_Summary` (Task 5).
3. **La conexión del IDLE se corta** (la notebook se suspende, cambia el wifi, el servidor reinicia): el vigía vuelve a conectar con espera creciente, sin loop de reconexión ni catarata de avisos de error. Lo cubre `TestWatcher_ReconnectsAfterServerRestart` (Task 3).
4. **Una cuenta con la contraseña rechazada:** el vigía no martilla el login (riesgo de bloqueo en Hostinger). Lo cubre `TestWatcher_RespectsAuthPause` (Task 3).
5. **Un correo que ya llega leído** (lo abriste en el celular antes): no se notifica. Lo cubre `TestNewMail_OnlyUnseen` (Task 2).

---

## File Structure

```
core/
  imapx/idle.go             # DialWatch, (*Conn).IdleWatch (Task 1)
  testenv/imap.go           # sin cambios esperados; usa Restart y StartIMAPRev1 existentes
  store/messages.go         # + MessagesByUIDs (Task 2)
  mail/
    events.go               # + EventNewMail (Task 2)
    sync.go                 # SyncFolder detecta lo nuevo (Task 2)
    watch.go                # vigías IDLE por cuenta (Task 3)
  model/model.go            # + NewMail, NewMailItem; Settings.DisableNotifications (Tasks 2, 5)
app/
  desktop.go                # bandeja, cerrar=esconder, instancia única, autostart, Ctrl+Q (Task 4)
  notify.go                 # mail:new-mail → notificación nativa; clic → abrir mensaje (Task 5)
main.go                     # arma la app con desktop.go (Task 4)
frontend/src/
  features/settings/SettingsDialog.tsx   # interruptores nuevos (Task 6)
  lib/events.ts                          # app:open-message → abrir el correo (Task 6)
```

---

### Task 1: IDLE en `imapx`

**Files:**
- Create: `core/imapx/idle.go`
- Modify: `core/imapx/conn.go` (el armado de opciones de `Dial` se extrae a un helper que comparten `Dial` y `DialWatch`)
- Test: `core/imapx/imapx_test.go`

**Interfaces:**
- Produces:
  - `func DialWatch(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config, onChange func()) (*Conn, error)`
    - Es igual a `Dial`, pero registra un `imapclient.UnilateralDataHandler` cuyo `Mailbox` llama a `onChange` cuando cambia `NumMessages`.
    - `Expunge` también llama a `onChange`.
    - Los errores se clasifican igual que en `Dial`: `ErrAuth`, `ErrNetwork` o `ErrTLS`.
  - `func (c *Conn) IdleWatch(ctx context.Context, mailbox string) error`
    - Abre `mailbox` en solo lectura (`Select`) y arranca IDLE.
    - Bloquea hasta que `ctx` se cancela, y entonces cierra el IDLE limpiamente y devuelve `ctx.Err()`; o hasta que la conexión se corta, y entonces devuelve un error que envuelve `ErrNetwork`.
    - Si el servidor no tiene IDLE (`!Caps().Has(imap.CapIdle)`, que ya contempla IMAP4rev2), devuelve `ErrUnsupported` sin mandar nada.

- [ ] **Step 1: Tests que fallan**

```go
func TestIdleWatch_NotifiesOnNewMessage(t *testing.T)  // DialWatch con un onChange que manda a un canal; IdleWatch(ctx, "INBOX") en una goroutine; otro cliente entrega un correo (imap.Deliver) → llega al canal antes de 5 s; cancel(ctx) → IdleWatch devuelve context.Canceled antes de 5 s
func TestIdleWatch_ServerGoneIsNetworkError(t *testing.T) // IdleWatch corriendo; imap.Close() → devuelve un error con ErrorIs(err, ErrNetwork) antes de 5 s
func TestIdleWatch_NoIdleCapIsUnsupported(t *testing.T)  // StartIMAPRev1 → ErrorIs(err, ErrUnsupported) y vuelve enseguida
```

- [ ] **Step 2:** `go test ./core/imapx/ -run IdleWatch`. Debe fallar porque las funciones no existen.
- [ ] **Step 3: Implementar.**
  - `imapclient` reinicia el IDLE solo, cada pocos minutos.
  - Para detectar el corte de la conexión, esperar en paralelo `ctx.Done()` y el cierre del cliente (`c.c.Closed()`).
- [ ] **Step 4:** `go test -race ./core/imapx/`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(imapx): IDLE con aviso de cambios en la carpeta`

---

### Task 2: Detectar correo nuevo al sincronizar

**Files:**
- Modify:
  - `core/mail/sync.go` (`SyncFolder`, `syncPlan`)
  - `core/mail/events.go`
  - `core/store/messages.go`
  - `core/model/model.go`
- Test: `core/mail/sync_test.go`, `core/store/store_test.go`

**Interfaces:**
- Produces:
  - En `model`:
    - `type NewMailItem struct { MessageID int64 \`json:"messageId"\`; FromName string \`json:"fromName"\`; FromEmail string \`json:"fromEmail"\`; Subject string \`json:"subject"\` }`
    - `type NewMail struct { AccountID int64 \`json:"accountId"\`; FolderID int64 \`json:"folderId"\`; Count int \`json:"count"\`; Items []NewMailItem \`json:"items"\` }`. `Items` lleva como mucho 3 correos, los más nuevos primero.
  - `const EventNewMail = "mail:new-mail"`, con data `model.NewMail` (el struct, no un map).
  - `(*store.Store).MessagesByUIDs(ctx, folderID int64, uids []uint32) ([]model.MessageSummary, error)`, ordenados por fecha descendente.
  - ~~`(*store.Store).InboxUnreadTotal` / `(*mail.Core).InboxUnreadTotal`~~: **eliminados** junto con el tooltip de la bandeja (descartado en Linux); quedaban sin uso. Se implementaron en `store/folders.go` y `mail/sync.go` y se borraron en la ola final de correcciones.
- **Reglas de `SyncFolder`:**
  - Al terminar con éxito, si la carpeta tiene rol `inbox`, la pasada no es la primera y hubo UIDs nuevos (`fresh`) no leídos, emite `EventNewMail`.
  - **"No es la primera"** quiere decir que `last.UIDNext != 0` antes de esta pasada. Es la marca que ya usa `prepareSync` (R19 de la Fase 1).
  - **No leídos** quiere decir sin `\Seen`, mirando las filas guardadas.
  - `Count` es la cantidad total de correos nuevos no leídos.

- [ ] **Step 1: Tests que fallan**

```go
func TestNewMail_NotOnFirstSync(t *testing.T)   // 50 correos en INBOX; primera SyncAccount → ningún evento mail:new-mail
func TestNewMail_AfterFirstSync(t *testing.T)   // sincronizada; Deliver 1 correo no leído; SyncFolder(inbox) → 1 evento con Count 1, AccountID/FolderID correctos y Items[0].Subject igual al asunto entregado
func TestNewMail_OnlyUnseen(t *testing.T)       // sincronizada; Deliver 1 con \Seen y 1 sin → Count 1
func TestNewMail_SummarizesBurst(t *testing.T)  // sincronizada; Deliver 20 → 1 solo evento con Count 20 y len(Items) == 3
func TestNewMail_NotForOtherFolders(t *testing.T) // carpeta "Varios" sincronizada; Deliver ahí y SyncFolder → ningún evento
// (TestInboxUnreadTotal_SumsAccounts se eliminó junto con InboxUnreadTotal.)
```

- [ ] **Step 2:** `go test ./core/... -run 'NewMail'`. Debe fallar.
- [ ] **Step 3: Implementar.** `syncPlan` suma `firstSync bool`, que se calcula en `prepareSync` antes de tocar `UIDNext`.
- [ ] **Step 4:** `go test -race ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): avisar correo nuevo en la Entrada (sin la primera sincronización)`

---

### Task 3: Vigías IDLE por cuenta

**Files:**
- Create: `core/mail/watch.go`, `core/mail/watch_test.go`
- Modify: `core/mail/accounts.go` (`AddAccount` / `RemoveAccount` avisan al administrador de vigías), `core/mail/core.go` (campos)

**Interfaces:**
- Consumes: `imapx.DialWatch`, `(*Conn).IdleWatch` (Task 1), `SyncFolder` (que ya emite `EventNewMail`, Task 2), y `noteAuthFailure`/`authPaused`/`clearAuthFailure` de `authpause.go` (2A).
- Produces:
  - `(*Core).StartWatching(ctx context.Context)`
    - Lanza con `c.spawn` un vigía por cuenta existente.
    - Después de esta llamada, `AddAccount` lanza el vigía de la cuenta nueva y `RemoveAccount` frena el de la cuenta quitada.
    - `Close` los frena a todos.
    - Es idempotente: llamarla dos veces no duplica vigías.
  - Para tests: `c.dialWatch` es un campo del `Core` con la firma de `imapx.DialWatch`, inicializado con `imapx.DialWatch`; los tests lo envuelven para contar intentos. `c.watcherCount() int` devuelve cuántos vigías hay vivos (solo para tests).
  - Para tests: `const watchDebounce = 2 * time.Second` y `watchBackoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}`. Los dos son campos del `Core` inicializados con esos valores, para que los tests los acorten.
- **Ciclo de un vigía:**
  1. Si la cuenta está en pausa por contraseña, espera a que venga (revisa cada `watchBackoff[0]`).
  2. Ubica la carpeta con rol `inbox`. Si no hay, espera `watchBackoff[0]` y reintenta (la primera sincronización todavía no la creó).
  3. Hace `DialWatch` con la contraseña del llavero.
     - Con un error `auth` anota la falla (`noteAuthFailure`) y vuelve al paso 1.
     - Con otros errores espera según `watchBackoff`, avanzando un escalón por falla seguida y quedándose en el último. Cada conexión exitosa resetea el escalón.
  4. Hace `IdleWatch(ctx, inbox.Name)`.
     - Cada `onChange` arranca o extiende una espera de `watchDebounce`; al vencer, hace `c.SyncFolder(ctx, inboxID)` (que pasa por el pool).
     - Si `IdleWatch` devuelve `ErrUnsupported`, el vigía de esa cuenta termina: queda la sincronización periódica.
  5. Con un corte de red, vuelve al paso 3 con espera.
- **Los vigías no emiten `EventSyncError`.** Los errores de conexión del vigía no deben inundar la UI de avisos; la sincronización periódica ya informa.

- [ ] **Step 1: Tests que fallan.** Usan `watchDebounce` de 50 ms y `watchBackoff` de 50 ms.

```go
func TestWatcher_NewMailWithoutManualSync(t *testing.T)      // cuenta sincronizada; StartWatching; Deliver 1 → llega mail:new-mail antes de 5 s sin llamar a SyncFolder
func TestWatcher_ReconnectsAfterServerRestart(t *testing.T) // vigía corriendo; imap.Restart(t); Deliver → llega mail:new-mail antes de 10 s; durante todo el test no hay eventos mail:sync-error del vigía
func TestWatcher_RespectsAuthPause(t *testing.T)            // contraseña mala en el llavero en memoria; StartWatching; esperar 500 ms → el contador de c.dialWatch (campo inyectable, ver abajo) registra 1 solo intento para esa cuenta
func TestWatcher_StopsOnRemoveAccount(t *testing.T)         // StartWatching; RemoveAccount → c.watcherCount() llega a 0 antes de 1 s; después Close vuelve antes de 1 s
func TestWatcher_StartWatchingIsIdempotent(t *testing.T)    // StartWatching dos veces; Deliver → un solo evento mail:new-mail
```

- [ ] **Step 2:** `go test ./core/mail/ -run Watcher`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test -race -count=3 ./core/mail/ -run Watcher` y luego `go test -race ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): vigías IDLE por cuenta para el correo al instante`

---

### Task 4: Vivir en la bandeja del sistema

**Files:**
- Create: `app/desktop.go`, `app/desktop_test.go`, `app/tray.png` (copia de `build/brand/png/icon-64.png`, embebida)
- Modify:
  - `main.go`
  - `app/mailservice.go` (llama a `core.StartWatching` en `ServiceStartup`; expone `GetStartAtLogin`, `SetStartAtLogin`)

**Interfaces:**
- Consumes: `StartWatching`.
- Produces:
  - `func StartHidden(args []string) bool`: true si `args` contiene `--hidden`.
  - `func SetupDesktop(app *application.App, win *application.WebviewWindow) *Desktop`, que arma:
    - **Bandeja del sistema:** ícono `tray.png`, tooltip fijo `"PostaControl"`.
      - Menú: "Abrir PostaControl", separador, "Salir".
      - Clic en el ícono: muestra la ventana y le da foco.
    - **Cerrar la ventana:** `RegisterHook(events.Common.WindowClosing, …)` la esconde y cancela el cierre, como en el ejemplo `hide-window` de Wails.
    - **Ctrl+Q en la ventana:** `app.Quit()`.
  - `(*MailService).GetStartAtLogin() (bool, error)` y `SetStartAtLogin(enabled bool) error`, sobre `app.Autostart`, con `AutostartOptions{Identifier: "postacontrol", Arguments: []string{"--hidden"}}`.
- **`main.go`:**
  - `SingleInstance: &application.SingleInstanceOptions{UniqueID: "ar.com.codecontrol.postacontrol", OnSecondInstanceLaunch: muestra la ventana y le da foco}`.
  - La ventana se crea con `Hidden: StartHidden(os.Args)`.
  - `Mac.ApplicationShouldTerminateAfterLastWindowClosed` pasa a `false`.
- **El servicio de notificaciones** se registra en la Task 5, no acá.

> **Limitación:** en Linux, Wails 3.0.0 no implementa el tooltip de la bandeja; el contador de no leídos queda para cuando Wails lo soporte.

- [ ] **Step 1: Tests que fallan.** En `app/desktop_test.go`:
  - `TestStartHidden`: `--hidden` presente da true; ausente da false; `--hiddenx` da false.
- [ ] **Step 2:** `go test ./app/ -run 'StartHidden'`. Debe fallar.
- [ ] **Step 3: Implementar.** La bandeja, el hook de cierre y la instancia única no tienen test automático: se verifican en la Task 7.
- [ ] **Step 4:** `task check` da PASS, y `go build .` compila.
- [ ] **Step 5: Commit** `feat(app): bandeja del sistema, cerrar esconde, una sola instancia e inicio al abrir sesión`

---

### Task 5: Notificaciones de correo nuevo

**Files:**
- Create: `app/notify.go`, `app/notify_test.go`
- Modify:
  - `app/notifier.go` (además de reenviar a Wails, le pasa `mail:new-mail` al notificador de escritorio)
  - `main.go` (registra `notifications.New()` como servicio)
  - `core/model/model.go`: `Settings` suma `DisableNotifications bool \`json:"disableNotifications"\``. El valor cero deja las notificaciones activadas, así que no hace falta migrar.
- Regenerate: bindings.

**Interfaces:**
- Consumes: `model.NewMail` (Task 2) y `(*mail.Core).GetSettings`.
- Produces:
  - `func NewMailNotification(ev model.NewMail, accountEmail string) notifications.NotificationOptions`
    - **Con `Count == 1`:**
      - `Title` = `FromName`, o `FromEmail` si no hay nombre.
      - `Body` = `Subject`, o `"(sin asunto)"` si está vacío.
      - `Data` = `{"messageId": Items[0].MessageID, "folderId": ev.FolderID}`.
    - **Con `Count > 1`:**
      - `Title` = `"<Count> correos nuevos"`.
      - `Body` = los remitentes de `Items` separados por ", ".
      - `Data` = `{"folderId": ev.FolderID}`.
    - **En los dos casos:**
      - `Subtitle` = `accountEmail`.
      - `ID` = `"new-mail-<AccountID>"`: reemplaza la notificación anterior de la misma cuenta en lugar de apilar.
  - **Cuándo se manda:** si `DisableNotifications` es false y la ventana no está visible o no tiene foco (`!win.IsVisible() || !win.IsFocused()`).
  - **Al hacer clic** (`OnNotificationResponse` con `DefaultActionIdentifier`):
    1. Muestra la ventana y le da foco.
    2. Emite al frontend `"app:open-message"` con `{folderId, messageId}` (`messageId` es 0 si es un resumen).

- [ ] **Step 1: Tests que fallan.** En `app/notify_test.go`:
  - `TestNewMailNotification_Single`, con nombre y sin nombre, y asunto vacío.
  - `TestNewMailNotification_Summary`: Count 20, Items de 3 remitentes, da `Title` "20 correos nuevos" y `Body` "Ana, Beto, Caro".
  - `TestNewMailNotification_ReplacesPerAccount`: el mismo `ID` para dos eventos de la misma cuenta.
  - Más `TestShouldNotify` para la decisión, extraída como `func ShouldNotify(s model.Settings, visible, focused bool) bool`, con la tabla: desactivadas, visible con foco, escondida, visible sin foco.
- [ ] **Step 2:** `go test ./app/ -run 'NewMailNotification|ShouldNotify'`. Debe fallar.
- [ ] **Step 3: Implementar** y regenerar los bindings.
- [ ] **Step 4:** `task check` da PASS.
- [ ] **Step 5: Commit** `feat(app): notificación del escritorio para el correo nuevo`

---

### Task 6: Configuración y abrir desde la notificación

**Files:**
- Modify:
  - `frontend/src/features/settings/SettingsDialog.tsx` y su test
  - `frontend/src/lib/events.ts` y su test
  - `frontend/src/features/layout/MailShell.tsx` (escucha el pedido de abrir)

**Interfaces:**
- Consumes:
  - `svc.GetStartAtLogin`, `svc.SetStartAtLogin` (Task 4)
  - `Settings.disableNotifications` (Task 5)
  - el evento Wails `"app:open-message"` `{folderId, messageId}` (Task 5)
- **`SettingsDialog` suma dos interruptores debajo de "Bandeja unificada":**
  - **"Notificaciones de correo nuevo":** activado cuando `!disableNotifications`. Ayuda: "Avisa con una notificación del escritorio cuando la ventana no está a la vista."
  - **"Iniciar al abrir sesión":** lee `GetStartAtLogin` y escribe `SetStartAtLogin`. Ayuda: "PostaControl arranca escondido en la bandeja del sistema." Si falla, muestra `toast.error(errorMessage(e))` y vuelve el interruptor a su estado.
- **`events.ts`:** expone `onOpenMessage(cb)`, un emisor de módulo como `onPasswordRequest`, alimentado por `Events.On("app:open-message")`.
- **`MailShell`** se suscribe:
  - hace `selectView({kind:"folder", folderId})`;
  - si `messageId > 0`, hace después `selectMessage(messageId)`.

- [ ] **Step 1: Tests que fallan.**
  - `SettingsDialog.test.tsx`:
    - "el interruptor de notificaciones guarda disableNotifications"
    - "Iniciar al abrir sesión llama SetStartAtLogin"
    - "si SetStartAtLogin falla, vuelve atrás y avisa"
  - `events.test.ts`: "app:open-message llega a onOpenMessage".
  - `MailShell.test.tsx`: "abrir desde la notificación selecciona la carpeta y el correo".
- [ ] **Step 2:** `npx vitest run src/features/settings src/lib/events.test.ts src/features/layout`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check` da PASS.
- [ ] **Step 5: Commit** `feat(ui): notificaciones e inicio al abrir sesión en Configuración; abrir el correo desde la notificación`

---

### Task 7: Prueba de aceptación (Fabricio)

Es manual, con la app instalada (`task install:desktop`), **no** con `task dev`. El inicio automático registra la ruta del ejecutable que lo activa, y tiene que ser `~/.local/bin/postacontrol`.

- [ ] **Bandeja del sistema:** al abrir PostaControl aparece el ícono en la bandeja. En Ubuntu lo muestra la extensión "Ubuntu AppIndicators", que viene activada.
- [ ] **Cerrar con la X:** la ventana se esconde. Clic en el ícono de la bandeja: vuelve.
- [ ] **Abrir de nuevo desde el lanzador:** aparece la misma ventana, no una segunda instancia.
- [ ] **Correo nuevo:** con la ventana escondida, mandate un correo desde el celular o el webmail.
  - En pocos segundos llega la notificación.
  - Al hacer clic se abre la app en ese correo.
- [ ] **Varios correos:** mandá 3 seguidos. Llega una notificación con "3 correos nuevos", o la última reemplaza a la anterior, no tres apiladas.
- [ ] **Configuración:**
  - Desactivá "Notificaciones de correo nuevo" y repetí: no llega nada.
  - Activá "Iniciar al abrir sesión", cerrá la sesión y volvé a entrar. PostaControl arranca escondido en la bandeja.
- [ ] **Salir:** "Salir" del menú de la bandeja cierra la app del todo. Ctrl+Q también.
- [ ] **Corte de red:** con la app abierta, apagá el wifi un minuto y prendelo. El correo nuevo vuelve a llegar al instante, sin una lluvia de avisos de error.

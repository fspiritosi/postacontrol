# PostaControl — Fase 1 "Primer correo" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Construir una app de escritorio con la que Fabricio configure una cuenta IMAP/SMTP real, reciba, lea, responda, reenvíe y envíe correo correctamente.

**Architecture:** El núcleo de correo vive en `core/` como paquetes Go sin dependencias de Wails. `core/mail.Core` es la fachada que orquesta store (SQLite), secrets (llavero), autoconfig, imapx, smtpx, mimex y htmlx. La cáscara Wails (`app/`) expone un servicio fino que delega en `Core` y traduce sus notificaciones a eventos Wails. El frontend React + shadcn consume los bindings generados con TanStack Query y escucha esos eventos para invalidar datos.

**Tech Stack:** Go (toolchain go1.27.3), Wails v3.0.0, `emersion/go-imap/v2` v2.0.0-beta.8 (incluye `imapserver/imapmemserver` para tests), `emersion/go-smtp` v0.25.0, `emersion/go-message`, `emersion/go-sasl`, `zalando/go-keyring`, `microcosm-cc/bluemonday`, `modernc.org/sqlite`, `golang.org/x/net/html`; React 19 + TypeScript + Vite + Tailwind v4 + shadcn/ui + TipTap v3 + `@tanstack/react-query` + Vitest.

**Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`

**Fuera de esta fase** (según el spec, van en la Fase 2): la descarga de cuerpos en segundo plano con índice FTS5 y la búsqueda, varias cuentas en la UI, la bandeja unificada, IMAP IDLE, las notificaciones, la bandeja del sistema y las conversaciones. En la Fase 1 los cuerpos se bajan al abrir cada correo y quedan en caché, y una sincronización periódica cada 5 minutos reemplaza a IDLE.

## Global Constraints

- `go.mod`: `module postacontrol` y `toolchain go1.27.3`. Todos los comandos Go se corren con esa toolchain (si el `go` del sistema es más viejo, `GOTOOLCHAIN=auto` la descarga sola).
- Wails **v3.0.0**, plantilla `react` (TypeScript). En Linux se usa GTK4 + WebKitGTK 6.0, que es el default; no se usa `-tags gtk3`.
- **Ningún paquete bajo `core/` importa `github.com/wailsapp/...`.** Lo verifica el test `TestCoreHasNoWailsImports` (Task 1).
- Las contraseñas **solo** van al llavero del sistema (servicio `postacontrol`). Nunca a SQLite, a logs ni a eventos.
- **TLS:** los certificados se verifican siempre. Los tests inyectan `RootCAs` con un certificado autofirmado. Está prohibido `InsecureSkipVerify` y `DialInsecure` fuera de `core/testenv`.
- La app **nunca** borra, mueve ni marca mensajes en el servidor salvo por una acción explícita. En Fase 1 las únicas escrituras al servidor son: `STORE +\Seen` al abrir un correo, `STORE +\Answered` al responder y `APPEND` en Enviados al enviar.
- Los tests **nunca** se conectan a cuentas reales. Usan `core/testenv`.
- **Textos de la interfaz:** en español rioplatense, con voseo ("Configurá tu cuenta"). Decisión mía.
- **Base de datos:** `$XDG_DATA_HOME/postacontrol/postacontrol.db`, o `~/.local/share/postacontrol/postacontrol.db` si `XDG_DATA_HOME` no está definida.
- Todos los structs de `core/model` y `autoconfig.Settings` llevan tags JSON en camelCase, para que los bindings de TypeScript tengan nombres predecibles: `AccountID`→`accountId`, `BodyHTML`→`bodyHtml`, `HTML`→`html`, `OriginalMessageID`→`originalMessageId`, `HasRemoteImages`→`hasRemoteImages`, `IMAP`→`imap`, `SMTP`→`smtp`.
- `task check` tiene que pasar antes de cada commit: `go vet ./...`, `go test ./...`, `tsc --noEmit`, `vitest run` y `eslint`.
- **Commits:** convencionales, con descripción en español (`feat(core): ...`), terminados con la línea `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.

## Review Focus

1. **Charsets no UTF-8 y encabezados RFC 2047** (ISO-8859-1, Windows-1252, `=?iso-8859-1?Q?Informaci=F3n?=`), muy comunes en correo en español. Se espera ver bien los acentos en el asunto, el remitente, el cuerpo y el nombre de los adjuntos. Lo cubre `TestParse_Latin1AndEncodedWords` (Task 7).
2. **MIME raro o incompleto:** solo texto plano, solo HTML, partes sin `Content-Type` o boundary mal formado. El lector muestra lo que pueda y no rompe. Lo cubre `TestPlan_OnlyTextPlain`, `TestPlan_OnlyHTML` y `TestParse_MalformedFallsBackToRaw` (Task 7).
3. **Contraseña incorrecta, servidor caído o certificado inválido al dar de alta la cuenta.** Se espera un error claro en español, que no se guarde nada en la base y que no quede ningún secreto en el llavero. Lo cubre `TestAddAccount_WrongPasswordPersistsNothing` y `TestAddAccount_UntrustedCertIsTLSError` (Task 10).
4. **La conexión se corta en medio de una sincronización.** La sincronización devuelve un error, la caché queda consistente (sin lotes a medias) y la siguiente sincronización se recupera. Lo cubre `TestSyncFolder_RecoversAfterConnectionDrop` (Task 11).
5. **Nombres y asuntos con acentos o ñ al enviar** ("Peña", "Información"). El correo sale con los encabezados codificados correctamente y el destinatario los ve bien. Lo cubre `TestBuild_NonASCIIHeaders` (Task 8) y `TestSend_EndToEnd` (Task 13).

---

## File Structure

```
postacontrol/
  go.mod  go.sum  main.go  Taskfile.yml  build/ (generado por wails3)
  arch_test.go                       # TestCoreHasNoWailsImports
  app/
    mailservice.go                   # servicio Wails: delega en core/mail.Core
    notifier.go                      # implementa mail.Notifier con app.Event.Emit
    paths.go                         # DataDir()
  core/
    model/model.go                   # tipos de dominio compartidos
    store/  store.go migrate.go schema.sql accounts.go folders.go messages.go drafts.go
    secrets/ secrets.go keyring.go memory.go
    autoconfig/ autoconfig.go ispdb.go known.go
    testenv/ tlscert.go imap.go smtp.go
    imapx/  conn.go folders.go fetch.go write.go
    smtpx/  send.go
    htmlx/  sanitize.go totext.go
    mimex/  plan.go decode.go build.go
    mail/   core.go errors.go events.go accounts.go sync.go read.go compose.go
  frontend/src/
    main.tsx  App.tsx  index.css
    lib/ utils.ts  queries.ts  events.ts  srcdoc.ts  folders.ts  recipients.ts
    components/ui/                   # generados por shadcn
    features/
      theme/ThemeProvider.tsx
      layout/MailShell.tsx
      accounts/AccountSetupDialog.tsx  Welcome.tsx
      folders/FolderSidebar.tsx
      messages/MessageList.tsx
      reader/MessageView.tsx  SafeHtmlFrame.tsx  AttachmentList.tsx
      compose/Composer.tsx  RichEditor.tsx  RecipientInput.tsx  ComposerContext.tsx
```

### Tipos compartidos (`core/model/model.go`) — los usan todas las tareas

```go
type Security string // "tls" | "starttls"
const (SecurityTLS Security = "tls"; SecuritySTARTTLS Security = "starttls")

type ServerSettings struct { Host string; Port int; Security Security }
type Address struct { Name, Email string }

type Account struct {
    ID int64; Email, DisplayName, Username, Color string
    IMAP, SMTP ServerSettings
    SaveSentCopy bool
}
type AccountInput struct { Account; Password string } // Account.ID se ignora

type FolderRole string // "inbox","sent","drafts","trash","junk","archive",""
type Folder struct {
    ID, AccountID int64; Name, Delimiter string; Role FolderRole
    Unread, Total int; Selectable bool
}

type MessageSummary struct {
    ID, AccountID, FolderID int64; UID uint32
    From Address; Subject, Snippet string; Date time.Time
    Seen, Flagged, Answered, HasAttachments bool
}
type Attachment struct { Index int; Filename, MIMEType string; Size int64; Part []int }
type MessageDetail struct {
    MessageSummary
    MessageID, InReplyTo string; References []string
    To, Cc, ReplyTo []Address
    HTML, Text string        // HTML ya sanitizado (imágenes remotas en data-remote-src)
    HasRemoteImages bool
    Attachments []Attachment
}

type ComposeMode string // "new","reply","replyAll","forward"
type ReplyContext struct { Mode ComposeMode; OriginalMessageID int64 }
type SendRequest struct {
    AccountID int64; To, Cc, Bcc []Address; Subject, BodyHTML string
    Reply *ReplyContext // nil si es un correo nuevo
}
type ComposeDraft struct {
    AccountID int64; To, Cc []Address; Subject string
    QuotedHTML string // para mostrar debajo del editor; no editable
    Reply ReplyContext
}
type SendResult struct { SentCopySaved bool; Warning string }
type LocalDraft struct { ID int64; CreatedAt time.Time; Request SendRequest; Error string }
type ListQuery struct { UnreadOnly bool; Offset, Limit int }
```

---

### Task 1: Scaffold del proyecto (Wails + React + Tailwind + shadcn + Taskfile check)

**Files:**
- Create: todo lo que genera `wails3 init`, `arch_test.go`, `core/model/model.go`
- Modify: `go.mod`, `Taskfile.yml`, `frontend/package.json`, `frontend/vite.config.ts`, `frontend/tsconfig*.json`, `.gitignore`

**Interfaces:**
- Produces: el módulo `postacontrol`, los tipos de `core/model` (bloque de arriba), `task check` y `task dev`, el alias `@/` → `frontend/src`, y el helper `cn()` en `frontend/src/lib/utils.ts`.

- [ ] **Step 1:** Generar el proyecto en una carpeta temporal y moverlo al repo, porque el repo no está vacío: `wails3 init -n postacontrol -t react -d /tmp/pc-init`. Copiar el contenido a la raíz del repo sin pisar `docs/` ni `.gitignore`, y sumar a `.gitignore` las entradas que trae la plantilla.
- [ ] **Step 2:** En `go.mod` fijar `module postacontrol`, `go 1.27` y `toolchain go1.27.3`. Correr `go mod tidy` y verificar que `wails3 dev` abra la ventana de ejemplo. Luego cerrarla.
- [ ] **Step 3:** En `frontend/`, instalar Tailwind v4 con el plugin de Vite y correr `npx shadcn@latest init` (estilo new-york, color base zinc, CSS variables, alias `@/`). Seguir la guía actual de shadcn para Vite (consultarla con ctx7). Agregar los componentes `button input label dialog resizable scroll-area separator badge tooltip dropdown-menu sonner avatar alert`. Instalar `@tanstack/react-query`, `@tiptap/react@^3`, `@tiptap/pm@^3`, `@tiptap/starter-kit@^3`, `lucide-react`, y como devDependencies `vitest`, `jsdom` y `@testing-library/react`. Si la plantilla no trae ESLint, agregar `eslint`, `typescript-eslint` y `eslint-plugin-react-hooks` con una config flat mínima.
- [ ] **Step 4:** Crear `core/model/model.go` con los tipos de la sección "Tipos compartidos" y escribir `arch_test.go` en el paquete `main`:

```go
func TestCoreHasNoWailsImports(t *testing.T) {
    out, err := exec.Command("go", "list", "-deps", "./core/...").Output()
    require.NoError(t, err)
    assert.NotContains(t, string(out), "github.com/wailsapp/")
}
```
(Usar `testify` para assert/require en todo el proyecto.)

- [ ] **Step 5:** Agregar al `Taskfile.yml` la tarea `check` con `go vet ./...`, `go test ./...` y, dentro de `frontend`, `npx tsc --noEmit`, `npx vitest run --passWithNoTests` y `npx eslint .`.
- [ ] **Step 6:** Correr `task check`. Debe salir en verde.
- [ ] **Step 7:** Commit: `chore: scaffold Wails v3 + React + shadcn`.

---

### Task 2: Store SQLite (`core/store`)

**Files:**
- Create: `core/store/{store.go,migrate.go,schema.sql,accounts.go,folders.go,messages.go,drafts.go}`, `core/store/store_test.go`

**Interfaces:**
- Consumes: `core/model`.
- Produces:
```go
func Open(path string) (*Store, error)             // crea el directorio, WAL, foreign_keys=ON, migra
func (s *Store) Close() error
// cuentas
func (s *Store) InsertAccount(ctx, a model.Account) (int64, error)
func (s *Store) GetAccount(ctx, id int64) (model.Account, error)   // ErrNotFound
func (s *Store) ListAccounts(ctx) ([]model.Account, error)
func (s *Store) DeleteAccount(ctx, id int64) error                 // cascada a carpetas, mensajes y borradores
// carpetas
type FolderSync struct { UIDValidity, UIDNext uint32 }
func (s *Store) ReplaceFolders(ctx, accountID int64, fs []model.Folder) error // upsert por (account_id,name); borra las que ya no están
func (s *Store) ListFolders(ctx, accountID int64) ([]model.Folder, error)     // Unread/Total del último STATUS guardado
func (s *Store) GetFolder(ctx, id int64) (model.Folder, FolderSync, error)
func (s *Store) SetFolderSync(ctx, folderID int64, fs FolderSync) error
func (s *Store) SetFolderCounts(ctx, folderID int64, unread, total int) error
func (s *Store) ResetFolder(ctx, folderID int64) error             // borra los mensajes y pone FolderSync en cero
// mensajes
type HeaderRow struct { model.MessageSummary; MessageID, InReplyTo string; References []string; To, Cc, ReplyTo []model.Address; Size int64 }
type BodyRow struct { HTML, Text, Snippet string; HasRemoteImages bool; Attachments []model.Attachment }
func (s *Store) UpsertHeaders(ctx, folderID int64, rows []HeaderRow) error  // una sola transacción por lote
func (s *Store) KnownUIDs(ctx, folderID int64) ([]uint32, error)            // orden ascendente
func (s *Store) DeleteUIDsNotIn(ctx, folderID int64, keep []uint32) error
func (s *Store) SetFlags(ctx, folderID int64, uid uint32, seen, flagged, answered bool) error
func (s *Store) ListMessages(ctx, folderID int64, q model.ListQuery) ([]model.MessageSummary, error) // por fecha desc
func (s *Store) GetMessage(ctx, id int64) (model.MessageDetail, bool /*bodyCached*/, error)
func (s *Store) SetBody(ctx, id int64, b BodyRow) error
// borradores locales
func (s *Store) SaveLocalDraft(ctx, req model.SendRequest, errMsg string) (int64, error)
func (s *Store) ListLocalDrafts(ctx) ([]model.LocalDraft, error)
func (s *Store) DeleteLocalDraft(ctx, id int64) error
var ErrNotFound = errors.New("no encontrado")
```

Esquema: tablas `accounts`, `folders` (UNIQUE `account_id,name`), `messages` (UNIQUE `folder_id,uid`; índices en `(folder_id,date DESC)`, `message_id` e `in_reply_to`; las listas de direcciones, `references` y `attachments` se guardan como JSON) y `local_drafts` (`request_json`, `error`, `created_at`). Las migraciones se versionan con `PRAGMA user_version`; `schema.sql` es la versión 1 y va embebido con `//go:embed`.

- [ ] **Step 1:** Escribir los tests en `store_test.go`. Cada uno usa un `Open(filepath.Join(t.TempDir(),"t.db"))` nuevo:
  - `TestOpen_MigratesAndIsIdempotent`: abrir, cerrar y volver a abrir el mismo archivo sin error; `PRAGMA user_version` vale 1.
  - `TestAccounts_CRUD`: insertar una cuenta (Hostinger, `imap.hostinger.com:993 tls` / `smtp.hostinger.com:465 tls`), que `GetAccount` la devuelva igual, que `ListAccounts` tenga 1 elemento, y que después de `DeleteAccount` devuelva `ErrNotFound`.
  - `TestReplaceFolders_UpsertsAndRemovesMissing`: `[INBOX, INBOX.Sent, Old]` y luego `[INBOX, INBOX.Sent]`. `Old` desaparece y los IDs de `INBOX` e `INBOX.Sent` no cambian.
  - `TestUpsertHeaders_ListOrderAndUnreadFilter`: 3 mensajes con fechas distintas, uno visto. `ListMessages` los devuelve por fecha descendente y con `UnreadOnly` devuelve 2.
  - `TestDeleteUIDsNotIn` y `TestResetFolder`: después de cada operación, `KnownUIDs` devuelve exactamente lo esperado.
  - `TestGetMessage_BodyCachedFlag`: `bodyCached` es false antes de `SetBody` y true después; References, To y Attachments sobreviven el ida y vuelta por JSON.
  - `TestDeleteAccount_Cascades`: después de borrar la cuenta no quedan carpetas ni mensajes.
  - `TestLocalDrafts_RoundTrip`: un `SendRequest` con `Reply` no nil se guarda, se lista y se borra.
- [ ] **Step 2:** `go test ./core/store/ -v`. Debe fallar porque el paquete no compila.
- [ ] **Step 3:** Implementar los archivos de `core/store` con `database/sql` + `modernc.org/sqlite` (driver `"sqlite"`, DSN con `_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)`).
- [ ] **Step 4:** `go test ./core/store/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): store SQLite con cuentas, carpetas, mensajes y borradores locales`.

---

### Task 3: Llavero (`core/secrets`)

**Files:**
- Create: `core/secrets/{secrets.go,keyring.go,memory.go}`, `core/secrets/secrets_test.go`

**Interfaces:**
- Produces:
```go
type Store interface {
    Set(email, password string) error
    Get(email string) (string, error) // ErrNotFound
    Delete(email string) error        // si no existe, no es error
}
var ErrNotFound = errors.New("contraseña no encontrada en el llavero")
func NewKeyring() Store   // go-keyring, servicio "postacontrol", usuario = email
func NewMemory() Store    // para tests; seguro para concurrencia
```

- [ ] **Step 1:** Escribir `TestMemory_SetGetDelete` (Get después de Delete devuelve `ErrNotFound`; borrar algo inexistente devuelve nil) y `TestKeyring_RoundTrip`. Este último corre solo si `POSTACONTROL_KEYRING_TEST=1` (si no, `t.Skip`) y usa el email `test+<random>@postacontrol.invalid`.
- [ ] **Step 2:** `go test ./core/secrets/ -v`. Debe fallar.
- [ ] **Step 3:** Implementar. `NewKeyring` traduce `keyring.ErrNotFound` a `ErrNotFound` y en `Delete` lo ignora.
- [ ] **Step 4:** `go test ./core/secrets/ -v`. Debe pasar. Después correr una vez `POSTACONTROL_KEYRING_TEST=1 go test ./core/secrets/ -run Keyring -v` contra el GNOME Keyring real. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): secretos en el llavero del sistema`.

---

### Task 4: Autoconfiguración (`core/autoconfig`)

**Files:**
- Create: `core/autoconfig/{autoconfig.go,ispdb.go,known.go}`, `core/autoconfig/autoconfig_test.go`, `core/autoconfig/testdata/ispdb-example.xml`

**Interfaces:**
- Consumes: `model.ServerSettings`, `model.Security`.
- Produces:
```go
type Settings struct { IMAP, SMTP model.ServerSettings; Username string; Source string } // Source: "known" | "ispdb" | "autoconfig"
type Options struct {
    LookupMX   func(ctx context.Context, domain string) ([]*net.MX, error) // default: net.DefaultResolver.LookupMX
    HTTPClient *http.Client                                                // default: timeout 8s
    ISPDBBase  string                                                      // default "https://autoconfig.thunderbird.net/v1.1/"
}
func Detect(ctx context.Context, email string, opt Options) (Settings, error) // ErrNotDetected si nada funciona
var ErrNotDetected = errors.New("no pudimos detectar los servidores; cargalos a mano")
```

Orden de detección:
1. **Proveedor conocido por MX:** si algún MX termina en `.hostinger.com.` o `.hostinger.com`, se usa IMAP `imap.hostinger.com:993 tls` y SMTP `smtp.hostinger.com:465 tls`.
2. **ISPDB:** `ISPDBBase + dominio`.
3. **Autoconfig del dominio:** `https://autoconfig.<dominio>/mail/config-v1.1.xml`.

En los XML se elige el primer `incomingServer type="imap"` y el primer `outgoingServer type="smtp"` cuyo `socketType` sea `SSL` (se mapea a `tls`) o `STARTTLS`. Los que tienen `plain` se ignoran. En el usuario, `%EMAILADDRESS%` se reemplaza por el email y `%EMAILLOCALPART%` por la parte local. Si no hay usuario, se usa el email.

- [ ] **Step 1:** Escribir los tests con `LookupMX` falso y un `httptest.Server` que hace de `ISPDBBase`:
  - `TestDetect_HostingerByMX`: MX `mx1.hostinger.com.` da los valores exactos de Hostinger, `Source=="known"` y `Username==email`. No se hace ninguna petición HTTP (el servidor de prueba falla el test si recibe una).
  - `TestDetect_ISPDB`: el MX no coincide y el XML de testdata tiene `imap.example.com:993 SSL`, `smtp.example.com:587 STARTTLS` y `%EMAILLOCALPART%`. Se esperan esos valores, `Security` mapeado y el usuario igual a la parte local.
  - `TestDetect_SkipsPlainSocket`: el primer servidor IMAP del XML es `plain` y el segundo `SSL`; se elige el segundo.
  - `TestDetect_NothingFound`: todo devuelve 404 o error, y `errors.Is(err, ErrNotDetected)`.
  - `TestDetect_InvalidEmail`: `"sin-arroba"` da un error que no es `ErrNotDetected`.
- [ ] **Step 2:** `go test ./core/autoconfig/ -v`. Debe fallar.
- [ ] **Step 3:** Implementar con `encoding/xml` y los defaults de `Options`. En el paso 3 de la detección, el dominio sale del email; nunca de la entrada del usuario sin validar.
- [ ] **Step 4:** `go test ./core/autoconfig/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): autodetección de servidores IMAP/SMTP`.

---

### Task 5: Entorno de pruebas y conexión IMAP (`core/testenv`, `core/imapx`)

**Files:**
- Create: `core/testenv/{tlscert.go,imap.go,smtp.go}`, `core/imapx/{conn.go,folders.go,fetch.go,write.go}`, `core/imapx/imapx_test.go`

**Interfaces:**
- Consumes: `model.ServerSettings`, `model.Folder`, `model.Address`.
- Produces (`core/testenv`, solo para tests):
```go
type TLS struct { Server *tls.Config; Client *tls.Config } // cert autofirmado para "127.0.0.1"; Client solo tiene RootCAs
func NewTLS(t testing.TB) TLS
type IMAP struct { Addr string; Settings model.ServerSettings; User *imapmemserver.User; Close func() }
func StartIMAP(t testing.TB, tlsc TLS, username, password string) *IMAP // implicit TLS; crea INBOX y "Sent" vacíos
func (s *IMAP) Deliver(t testing.TB, mailbox string, raw []byte, flags ...imap.Flag) // APPEND directo al User
type SMTP struct { Addr string; Settings model.ServerSettings; Received func() []Envelope; Close func() }
type Envelope struct { From string; To []string; Data []byte }
func StartSMTP(t testing.TB, tlsc TLS, username, password string) *SMTP // implicit TLS, AUTH PLAIN, guarda lo recibido
```
- Produces (`core/imapx`):
```go
type Conn struct { /* *imapclient.Client */ }
func Dial(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config) (*Conn, error)
func (c *Conn) Close() error
type RemoteFolder struct { Name, Delimiter string; Role model.FolderRole; Selectable bool; Unread, Total int }
func (c *Conn) ListFolders() ([]RemoteFolder, error)        // LIST "" "*" + STATUS (MESSAGES UNSEEN) de cada carpeta seleccionable
type SelectInfo struct { UIDValidity, UIDNext uint32; NumMessages uint32 }
func (c *Conn) Select(name string) (SelectInfo, error)       // read-only (EXAMINE) salvo que se pida escritura
func (c *Conn) SelectRW(name string) (SelectInfo, error)
func (c *Conn) AllUIDs() ([]uint32, error)                   // UID SEARCH ALL, ascendente
type RemoteHeader struct { UID uint32; Flags []imap.Flag; Date time.Time; Size int64; Envelope *imap.Envelope; References []string; HasAttachments bool }
func (c *Conn) FetchHeaders(uids []uint32) ([]RemoteHeader, error) // UID, FLAGS, ENVELOPE, INTERNALDATE, RFC822.SIZE, BODYSTRUCTURE, BODY.PEEK[HEADER.FIELDS (REFERENCES)]
func (c *Conn) FetchFlags(uids []uint32) (map[uint32][]imap.Flag, error)
func (c *Conn) FetchStructure(uid uint32) (imap.BodyStructure, error)
func (c *Conn) FetchPart(uid uint32, part []int) (mimeHeader []byte, body []byte, err error) // BODY.PEEK[part.MIME] y BODY.PEEK[part]; si part es vacío, BODY.PEEK[HEADER] y BODY.PEEK[TEXT]
func (c *Conn) AddFlags(uid uint32, flags ...imap.Flag) error // requiere SelectRW
func (c *Conn) Append(mailbox string, raw []byte, flags ...imap.Flag) error
func DetectRole(name, delim string, attrs []imap.MailboxAttr) model.FolderRole
```

`DetectRole`: primero los atributos special-use (`\Sent`, `\Drafts`, `\Trash`, `\Junk`, `\Archive`). `INBOX` (sin distinguir mayúsculas) es `inbox`. Si no hay atributo, se mira el último segmento del nombre, sin distinguir mayúsculas: `Sent`, `Sent Items`, `Sent Messages` y `Enviados` son `sent`; `Drafts` y `Borradores` son `drafts`; `Trash`, `Deleted Items` y `Papelera` son `trash`; `Junk` y `Spam` son `junk`; `Archive` y `Archivo` son `archive`.

`HasAttachments` es true si la `BodyStructure` tiene alguna parte con disposición `attachment`, o alguna parte que no sea `text/*` ni `multipart/*` y tenga filename.

`Dial`: con `tls` usa `imapclient.DialTLS` y con `starttls` usa `imapclient.DialStartTLS`, en ambos casos con `Options.TLSConfig = tlsCfg` (nil significa las raíces del sistema). Después hace `Login`. Los errores se envuelven en tres variables exportadas: `ErrAuth` (credenciales rechazadas), `ErrNetwork` (dial o timeout) y `ErrTLS` (cualquier `x509`/`tls` error), para que `core/mail` los traduzca.

- [ ] **Step 1:** Escribir los tests. Cada uno levanta `NewTLS` + `StartIMAP`:
  - `TestDial_OK` y `TestDial_WrongPassword` (`errors.Is(err, ErrAuth)`).
  - `TestDial_UntrustedCert`: se pasa `tlsCfg` nil, así que el cert autofirmado no es confiable para el sistema, y `errors.Is(err, ErrTLS)`.
  - `TestListFolders_RolesAndCounts`: se crea `Sent` y `INBOX.Borradores`, se entregan 2 mensajes a INBOX (1 con `\Seen`). INBOX queda con `inbox`, `Total=2`, `Unread=1`; `Sent` con `sent`; `INBOX.Borradores` con `drafts`.
  - `TestDetectRole_Table`: tabla con los casos de la regla de arriba, incluido `INBOX.Sent` con delimitador `.`.
  - `TestFetchHeaders_EnvelopeReferencesAttachments`: se entrega un mensaje con `References: <a@x> <b@x>` y un adjunto PDF, y otro sin adjuntos. Se espera `References == ["<a@x>","<b@x>"]` y `HasAttachments` true/false según corresponda.
  - `TestFetchPart_TextAndAttachment`: se pide la parte del texto y la del adjunto; el adjunto se compara byte a byte (después de decodificar base64 en el test).
  - `TestAddFlagsAndAppend`: `AddFlags(uid, \Seen)` hace que `FetchFlags` incluya `\Seen`; `Append("Sent", raw, \Seen)` hace que `Select("Sent").NumMessages == 1`.
- [ ] **Step 2:** `go test ./core/imapx/ -v`. Debe fallar.
- [ ] **Step 3:** Implementar `core/testenv`. En `StartIMAP`: `imapserver.New(&imapserver.Options{NewSession: ..., Caps: IMAP4rev1 + IMAP4rev2, TLSConfig: nil})` sobre un `tls.Listen("tcp","127.0.0.1:0", tlsc.Server)`. En `StartSMTP`: `smtp.NewServer` con un backend que exige AUTH PLAIN (vía `go-sasl`) y guarda los `Envelope`. Después implementar `core/imapx`.
- [ ] **Step 4:** `go test ./core/imapx/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): cliente IMAP y entorno de pruebas con servidores en memoria`.

---

### Task 6: HTML seguro y texto plano (`core/htmlx`)

**Files:**
- Create: `core/htmlx/{sanitize.go,totext.go}`, `core/htmlx/htmlx_test.go`

**Interfaces:**
- Produces:
```go
func Sanitize(html string) (clean string, hasRemoteImages bool)
func RestoreRemoteImages(clean string) string      // data-remote-src → src (para citar el original al responder)
func ToText(html string) string                     // para la parte text/plain del envío y el snippet
func Snippet(text string, n int) string              // colapsa espacios y corta en n runas sin partir palabras
```

`Sanitize`:
- **Política:** bluemonday `UGCPolicy()` con estos agregados: tablas (`table thead tbody tfoot tr td th colgroup col` con `width height align valign bgcolor border cellpadding cellspacing colspan rowspan`), `font` (`color face size`), `center`, el atributo `style` global y el elemento `<style>`.
- **Imágenes remotas:** todo `<img src="http(s)://…">` pasa a `data-remote-src="…"` sin `src`. Se mantienen `data:` y `cid:`.
- **Lo que se elimina:** `<script>`, atributos `on*`, `javascript:` en `href` o `src`, `<iframe>`, `<object>`, `<embed>`, `<form>`, `<input>`, `<meta http-equiv=refresh>` y `<base>`.

La política es una primera barrera. La barrera real es el iframe con sandbox y CSP de la Task 16.

`ToText`: `p`, `div`, `br`, `li` (con prefijo `- `), `h1`–`h6`, `blockquote` (con prefijo `> ` en cada línea) y `a` (`texto (url)` cuando el texto es distinto del href). Se colapsan las líneas vacías que se repiten y se decodifican las entidades HTML.

- [ ] **Step 1:** Escribir los tests:
  - `TestSanitize_StripsScriptsAndHandlers`: `<script>alert(1)</script><a href="javascript:x" onclick="y">a</a>` no contiene `script`, `javascript:` ni `onclick`.
  - `TestSanitize_BlocksRemoteImages`: `<img src="https://t.example/p.gif">` da `data-remote-src="https://t.example/p.gif"`, sin `src=` y con `hasRemoteImages == true`.
  - `TestSanitize_KeepsDataAndCidImagesAndTables`: se conservan `src="data:image/png;base64,AAA"`, `<td bgcolor="#fff">` y `style="color:red"`, con `hasRemoteImages == false`.
  - `TestRestoreRemoteImages`: aplicada a la salida del test anterior, el resultado vuelve a tener `src="https://t.example/p.gif"`.
  - `TestToText_Structure`: `<p>Hola <b>Peña</b></p><ul><li>uno</li></ul><blockquote><p>cita</p></blockquote><a href="https://x.y">sitio</a>` da `"Hola Peña\n\n- uno\n\n> cita\n\nsitio (https://x.y)"`.
  - `TestSnippet_CutsOnWord`: `Snippet("hola  mundo\nqué tal", 12) == "hola mundo…"`.
- [ ] **Step 2:** `go test ./core/htmlx/ -v`. Debe fallar.
- [ ] **Step 3:** Implementar. El renombrado de imágenes y `ToText` usan `golang.org/x/net/html` sobre el árbol, nunca expresiones regulares.
- [ ] **Step 4:** `go test ./core/htmlx/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): sanitizado de HTML y conversión a texto plano`.

---

### Task 7: Plan de partes y decodificación MIME (`core/mimex` lectura)

**Files:**
- Create: `core/mimex/{plan.go,decode.go}`, `core/mimex/read_test.go`, `core/mimex/testdata/*.eml`

**Interfaces:**
- Consumes: `imap.BodyStructure` (de `imapx.FetchStructure`), `model.Attachment`.
- Produces:
```go
type InlinePart struct { Part []int; ContentID, MIMEType string }
type PartPlan struct {
    TextPart, HTMLPart []int   // nil si no existe; si el mensaje no es multipart, la ruta es []int{1}
    Inline      []InlinePart   // partes con Content-ID referenciadas como cid:
    Attachments []model.Attachment
}
func Plan(bs imap.BodyStructure) PartPlan
func DecodeText(mimeHeader, body []byte) (string, error)  // transfer-encoding + charset → UTF-8
func DecodeBinary(mimeHeader, body []byte) ([]byte, error)
func ResolveCID(html string, parts map[string]string) string // cid:<id> → data:URI; parts mapea contentID (sin <>) → data URI
func DecodeHeaderWord(s string) string                       // RFC 2047 con charsets legacy
```

`Plan`:
- En `multipart/alternative` se prefiere el último `text/html` y el último `text/plain`.
- En `multipart/related` las partes con Content-ID van a `Inline`.
- Toda parte con disposición `attachment`, o que no sea `text/*` y tenga nombre, va a `Attachments`, con `Index` correlativo desde 0.
- `Filename` se decodifica con `DecodeHeaderWord`, y también con RFC 2231 si viene en ese formato.

Los charsets se resuelven con `github.com/emersion/go-message/charset` (importado por su efecto colateral) usando `message.New`.

- [ ] **Step 1:** Escribir los tests. Los `.eml` de testdata se arman a mano y su `BodyStructure` se obtiene entregándolos a `testenv.StartIMAP` y llamando a `imapx.FetchStructure`, para usar la misma estructura que en producción:
  - `TestPlan_AlternativeWithAttachment` (`text/plain` + `text/html` + PDF `factura.pdf`): `TextPart=[1,1]`, `HTMLPart=[1,2]` y 1 adjunto con `Filename=="factura.pdf"` y `Part==[2]`.
  - `TestPlan_RelatedInlineImage`: hay un `Inline` con el `ContentID` correcto y la imagen no aparece en `Attachments`.
  - `TestPlan_OnlyTextPlain`: el mensaje no es multipart, `TextPart==[]int{1}` y `HTMLPart==nil`.
  - `TestPlan_OnlyHTML`: el mensaje no es multipart, `HTMLPart==[]int{1}` y `TextPart==nil`.
  - `TestParse_Latin1AndEncodedWords`: un cuerpo `quoted-printable` en `iso-8859-1` con `Informaci=F3n` decodifica a `Información`; `DecodeHeaderWord("=?windows-1252?Q?Pe=F1a?=") == "Peña"`; un adjunto con nombre RFC 2231 `filename*=iso-8859-1''presupuesto%F1.pdf` da `presupuestoñ.pdf`.
  - `TestParse_MalformedFallsBackToRaw`: con un boundary que no cierra, `DecodeText` sobre la parte vacía (header+text) no devuelve error y el texto contiene el cuerpo crudo.
  - `TestResolveCID`: `<img src="cid:logo@x">` con el mapa `{"logo@x":"data:image/png;base64,AAA"}` da `src="data:image/png;base64,AAA"`.
- [ ] **Step 2:** `go test ./core/mimex/ -run 'Plan|Parse|Resolve' -v`. Debe fallar.
- [ ] **Step 3:** Implementar `plan.go` y `decode.go`. `Plan` recorre la estructura con `BodyStructure.Walk` y arma las rutas de parte.
- [ ] **Step 4:** `go test ./core/mimex/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): plan de partes MIME y decodificación de charsets`.

---

### Task 8: Construcción del mensaje saliente (`core/mimex` escritura)

**Files:**
- Create: `core/mimex/build.go`, `core/mimex/build_test.go`

**Interfaces:**
- Consumes: `model.Address`.
- Produces:
```go
type OutAttachment struct { Filename, MIMEType string; Data []byte }
type Outgoing struct {
    From model.Address; To, Cc, Bcc []model.Address
    Subject, HTML, Text string
    InReplyTo string; References []string
    Attachments []OutAttachment
    Date time.Time; MessageID string // MessageID con <>, lo genera el que llama
}
func Build(o Outgoing) ([]byte, error)
func NewMessageID(fromEmail string) string // "<uuid@dominio-del-from>"
func Recipients(o Outgoing) []string        // To+Cc+Bcc, sin duplicados, en minúscula
```

**Estructura:**
- Sin adjuntos: `multipart/alternative` con `text/plain` y `text/html` en UTF-8 y `quoted-printable`.
- Con adjuntos: `multipart/mixed` que contiene ese `alternative` más un adjunto en base64 por cada `OutAttachment`, con `Content-Disposition: attachment` y filename codificado según RFC 2231 si no es ASCII.

**Encabezados:**
- Se escriben `From`, `To`, `Cc`, `Subject` (RFC 2047 si no es ASCII), `Date` (RFC 5322), `Message-ID`, `MIME-Version`, `In-Reply-To` y `References` (estos dos solo si vienen).
- **`Bcc` nunca va en los encabezados.**
- Se usa `go-message/mail` (`mail.CreateWriter`, `CreateInline`, `CreateAttachment`).

- [ ] **Step 1:** Escribir los tests. Cada uno parsea la salida con `go-message/mail.CreateReader` para verificar:
  - `TestBuild_AlternativeOnly`: el tipo de contenido es `multipart/alternative` y tiene 2 partes con el HTML y el texto esperados.
  - `TestBuild_WithAttachments`: el tipo es `multipart/mixed`, el adjunto sale con el filename y los bytes esperados, y el `alternative` está adentro.
  - `TestBuild_NonASCIIHeaders`: con From `{"Fabricio Peña","f@ej.com"}` y asunto `Información de octubre`, el encabezado crudo es ASCII puro y, decodificado, da exactamente esos textos. Un filename `presupuesto ñ.pdf` vuelve igual.
  - `TestBuild_BccNotInHeaders`: el crudo no contiene `Bcc` ni la dirección de bcc, y `Recipients` sí la incluye.
  - `TestBuild_ReplyHeaders`: `In-Reply-To: <a@x>` y `References: <r@x> <a@x>`.
  - `TestNewMessageID_Format`: coincide con `^<[0-9a-f-]{36}@ej\.com>$` para `f@ej.com`.
- [ ] **Step 2:** `go test ./core/mimex/ -run 'Build|MessageID' -v`. Debe fallar.
- [ ] **Step 3:** Implementar `build.go`.
- [ ] **Step 4:** `go test ./core/mimex/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): construcción de mensajes MIME salientes`.

---

### Task 9: Envío SMTP (`core/smtpx`)

**Files:**
- Create: `core/smtpx/send.go`, `core/smtpx/send_test.go`

**Interfaces:**
- Consumes: `model.ServerSettings`, `testenv.StartSMTP`.
- Produces:
```go
func Send(ctx context.Context, s model.ServerSettings, username, password, from string, rcpts []string, raw []byte, tlsCfg *tls.Config) error
func Check(ctx context.Context, s model.ServerSettings, username, password string, tlsCfg *tls.Config) error // conecta + AUTH + QUIT
var ErrAuth, ErrNetwork, ErrTLS, ErrRejected error // ErrRejected: el servidor rechazó un remitente, un destinatario o el DATA
```

Con `tls` se usa `smtp.DialTLS` y con `starttls` se usa `smtp.DialStartTLS`. Después va `Auth(sasl.NewPlainClient("", username, password))`, `Mail`, `Rcpt` por cada destinatario, `Data` y `Quit`. El dial respeta `ctx` (si vence, se cierra la conexión).

- [ ] **Step 1:** Escribir los tests: `TestSend_Delivers` (el `Envelope` recibido tiene From, To y los bytes exactos), `TestSend_WrongPassword` (`ErrAuth`), `TestSend_UntrustedCert` (`tlsCfg` nil da `ErrTLS`), `TestCheck_OK` y `TestSend_RejectedRecipient`. Para este último, el backend de testenv rechaza `rechazo@ej.com` y se espera `ErrRejected`. Si hace falta, sumar a `StartSMTP` la opción `RejectRcpt string`.
- [ ] **Step 2:** `go test ./core/smtpx/ -v`. Debe fallar.
- [ ] **Step 3:** Implementar.
- [ ] **Step 4:** `go test ./core/smtpx/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): envío SMTP con autenticación y TLS`.

---

### Task 10: Núcleo — cuentas (`core/mail`: Core, errores, eventos, alta de cuenta)

**Files:**
- Create: `core/mail/{core.go,errors.go,events.go,accounts.go}`, `core/mail/accounts_test.go`, `core/mail/helpers_test.go`

**Interfaces:**
- Consumes: `store`, `secrets`, `autoconfig`, `imapx`, `smtpx`, `model`.
- Produces:
```go
type Notifier interface { Emit(event string, data any) }
type Config struct {
    DBPath string; Secrets secrets.Store; Notifier Notifier
    TLS *tls.Config                 // nil = raíces del sistema; los tests inyectan RootCAs
    Autoconfig autoconfig.Options
}
func New(cfg Config) (*Core, error)
func (c *Core) Close() error
func (c *Core) DetectSettings(ctx context.Context, email string) (autoconfig.Settings, error)
func (c *Core) TestAccount(ctx context.Context, in model.AccountInput) error   // imapx.Dial + Close y smtpx.Check
func (c *Core) AddAccount(ctx context.Context, in model.AccountInput) (model.Account, error)
func (c *Core) ListAccounts(ctx context.Context) ([]model.Account, error)
func (c *Core) RemoveAccount(ctx context.Context, id int64) error  // borra caché local + secreto; NO toca el servidor

// errors.go
type UserError struct { Code string; Msg string; Err error } // Error() devuelve Msg; Unwrap devuelve Err
// Codes y mensajes exactos:
// "auth":    "El servidor rechazó el usuario o la contraseña."
// "network": "No pudimos conectarnos al servidor. Revisá el host, el puerto y tu conexión."
// "tls":     "El certificado del servidor no es válido. Por seguridad no nos conectamos."
// "rejected":"El servidor rechazó el envío: %s"
// "not_found":"No encontramos lo que buscabas."

// events.go
const (
    EventAccountsChanged = "mail:accounts-changed" // data: nil
    EventFoldersUpdated  = "mail:folders-updated"  // data: {"accountId": int64}
    EventFolderUpdated   = "mail:folder-updated"   // data: {"folderId": int64}
    EventSyncProgress    = "mail:sync-progress"    // data: {"folderId": int64, "done": int, "total": int}
    EventSyncError       = "mail:sync-error"       // data: {"accountId": int64, "message": string}
)
```

**`AddAccount`:**
1. Valida el email con `net/mail.ParseAddress`. Si `Username` está vacío, usa el email. Si `DisplayName` está vacío, usa la parte local del email.
2. Ejecuta `TestAccount`.
3. Hace `InsertAccount`. El `Color` es el siguiente de la paleta `#2563eb, #16a34a, #ea580c, #9333ea` (cíclica) y `SaveSentCopy` es true.
4. Guarda el secreto con `Secrets.Set(email, password)`. Si falla, hace `DeleteAccount` y devuelve el error.
5. Emite `EventAccountsChanged`.
6. Lanza `SyncAccount` en una goroutine (la define la Task 11). En esta tarea alcanza con un stub que no hace nada; la Task 11 lo reemplaza.

Los errores de `imapx` y `smtpx` (`ErrAuth`, `ErrNetwork`, `ErrTLS`) se traducen a `UserError` con su código.

- [ ] **Step 1:** Escribir en `helpers_test.go` el helper `newTestCore(t) (*Core, *testenv.IMAP, *testenv.SMTP, *recNotifier)`, que levanta TLS + IMAP + SMTP con usuario `f@ej.com` y contraseña `secreta`, usa `secrets.NewMemory()` y una base en `t.TempDir()`. `recNotifier` guarda los eventos emitidos. También `inputFor(imap, smtp, password)`. Después escribir los tests:
  - `TestAddAccount_OK`: la cuenta se persiste con el color `#2563eb`, `Secrets.Get` devuelve `secreta`, se emite `EventAccountsChanged` y `ListAccounts` tiene 1 elemento.
  - `TestAddAccount_WrongPasswordPersistsNothing`: el código de `UserError` es `"auth"`, `ListAccounts` está vacío y `Secrets.Get` devuelve `ErrNotFound`.
  - `TestAddAccount_UntrustedCertIsTLSError`: con un Core con `TLS: nil`, el código es `"tls"` y no se persiste nada.
  - `TestAddAccount_UnreachableIsNetworkError`: con el puerto 1, el código es `"network"`.
  - `TestRemoveAccount_ClearsCacheAndSecret`: después de borrar la cuenta, `ListAccounts` está vacío y el secreto no existe. Además, la INBOX del servidor sigue teniendo sus mensajes (la verificación se hace con `imap.User` del testenv).
- [ ] **Step 2:** `go test ./core/mail/ -v`. Debe fallar.
- [ ] **Step 3:** Implementar.
- [ ] **Step 4:** `go test ./core/mail/ -v`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): alta, prueba y baja de cuentas`.

---

### Task 11: Núcleo — sincronización (`core/mail/sync.go`)

**Files:**
- Create: `core/mail/sync.go`, `core/mail/sync_test.go`
- Modify: `core/mail/core.go` (pool de conexiones y `AddAccount`, que pasa a llamar a `SyncAccount`)

**Interfaces:**
- Consumes: `imapx.Conn`, `store.*`, los eventos de la Task 10.
- Produces:
```go
func (c *Core) ListFolders(ctx context.Context, accountID int64) ([]model.Folder, error) // desde la caché
func (c *Core) SyncAccount(ctx context.Context, accountID int64) error // ListFolders remoto → ReplaceFolders + counts → SyncFolder(INBOX)
func (c *Core) SyncFolder(ctx context.Context, folderID int64) error
func (c *Core) ListMessages(ctx context.Context, folderID int64, q model.ListQuery) ([]model.MessageSummary, error) // Limit 0 → 100
func (c *Core) StartBackgroundSync(ctx context.Context, every time.Duration) // SyncAccount de cada cuenta cada `every`; se detiene con ctx
// interno, usado también por las Tasks 12 y 13:
func (c *Core) withConn(ctx context.Context, accountID int64, fn func(*imapx.Conn) error) error
```

`withConn` mantiene una conexión por cuenta, protegida por un mutex por cuenta. Si `fn` devuelve un error de red, cierra la conexión, vuelve a hacer el dial **una vez** y reintenta `fn`. Si falla de nuevo, emite `EventSyncError` con el mensaje del `UserError` y devuelve ese error. La contraseña la toma de `Secrets.Get(email)`.

**Algoritmo de `SyncFolder`** (no lo determinan las firmas):
```
info := conn.Select(folder.Name)
if sync.UIDValidity != 0 && sync.UIDValidity != info.UIDValidity { store.ResetFolder }
remote := conn.AllUIDs()
store.DeleteUIDsNotIn(folderID, remote)            // borrados desde otro cliente
known := store.KnownUIDs(folderID)
nuevos := remote − known, en orden DESCENDENTE      // lo más nuevo primero
for lote de 200 en nuevos:
    rows := conn.FetchHeaders(lote) → HeaderRow
    store.UpsertHeaders(folderID, rows)             // una transacción por lote
    Emit(EventSyncProgress, {folderID, done, total: len(nuevos)})
flags := conn.FetchFlags(últimos 500 de known ∩ remote) → store.SetFlags
store.SetFolderSync(folderID, {info.UIDValidity, info.UIDNext})
store.SetFolderCounts(folderID, unread y total desde la caché)
Emit(EventFolderUpdated, {folderID})
```
`SetFolderSync` se escribe **al final**. Si la sincronización se corta antes, la siguiente vuelve a calcular `nuevos` desde `KnownUIDs` y no pierde nada.

- [ ] **Step 1:** Escribir los tests. Usan `newTestCore` + `AddAccount` y esperan a que termine la sincronización inicial con un helper `waitEvent(rec, EventFolderUpdated)`:
  - `TestSyncAccount_FoldersAndInbox`: con 3 mensajes en INBOX y la carpeta `Sent`, `ListFolders` devuelve INBOX (`inbox`, `Total=3`) y Sent (`sent`), y `ListMessages(INBOX)` devuelve 3 mensajes ordenados por fecha descendente con From y Subject correctos.
  - `TestSyncFolder_Incremental`: se entrega 1 mensaje más y se sincroniza de nuevo. Quedan 4 mensajes, y en la segunda pasada `FetchHeaders` solo pidió 1 UID. Se verifica contando los `EventSyncProgress` con `total==1`.
  - `TestSyncFolder_RemovesExpunged`: se borra un mensaje en el servidor (expunge vía testenv) y, después de sincronizar, quedan 2.
  - `TestSyncFolder_UIDValidityChangeResets`: se recrea INBOX en el servidor con otro UIDVALIDITY y la caché se reconstruye sin duplicados.
  - `TestSyncFolder_LargeFolderBatchesNewestFirst`: con 450 mensajes, hay 3 `EventSyncProgress` (`done` 200, 400, 450) y, después del primer lote, la caché ya contiene el UID más alto.
  - `TestSyncFolder_RecoversAfterConnectionDrop`: el `Close()` del testenv IMAP se llama a mitad de la sincronización, después del primer lote (`notifier` hace el corte al recibir el primer `EventSyncProgress`). `SyncFolder` devuelve un error con código `"network"`, `FolderSync` sigue en 0 y no hay filas a medio escribir (la caché tiene exactamente 200 mensajes). Con el servidor levantado de nuevo en la misma dirección (`testenv.IMAP.Restart`), la siguiente sincronización termina con 450.
  - `TestStartBackgroundSync_Ticks`: con `every=50ms`, un mensaje nuevo aparece en la caché en menos de 1 s, y después de cancelar el contexto no hay más sincronizaciones.
- [ ] **Step 2:** `go test ./core/mail/ -run Sync -v`. Debe fallar.
- [ ] **Step 3:** Implementar `sync.go` y el pool. Si hace falta, sumar a `core/testenv` `Expunge(mailbox, uid)`, `RecreateMailbox(name, uidValidity)` y `Restart()`.
- [ ] **Step 4:** `go test ./core/mail/ -v -race`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): sincronización incremental de carpetas y encabezados`.

---

### Task 12: Núcleo — lectura (`core/mail/read.go`)

**Files:**
- Create: `core/mail/read.go`, `core/mail/read_test.go`

**Interfaces:**
- Consumes: `withConn`, `imapx.FetchStructure/FetchPart/AddFlags`, `mimex.Plan/DecodeText/DecodeBinary/ResolveCID`, `htmlx.Sanitize/ToText/Snippet`, `store.GetMessage/SetBody/SetFlags`.
- Produces:
```go
func (c *Core) GetMessage(ctx context.Context, messageID int64) (model.MessageDetail, error)
func (c *Core) SaveAttachment(ctx context.Context, messageID int64, index int, destPath string) error
func (c *Core) attachmentBytes(ctx context.Context, messageID int64, index int) (model.Attachment, []byte, error) // la usa la Task 13 al reenviar
```

**`GetMessage`:**
1. Si el cuerpo no está en caché:
   - Pide `FetchStructure` y arma el `Plan`.
   - Baja con `FetchPart` el texto, el HTML y cada `Inline`, y decodifica todo.
   - Resuelve los `cid:` a data URIs y aplica `Sanitize` al HTML.
   - Si no hay texto, usa `ToText(html)`.
   - El snippet es `Snippet(text,160)`.
   - Guarda todo con `SetBody`.
2. Si `!Seen`: hace `SelectRW` + `AddFlags(\Seen)` en el servidor, actualiza la caché con `SetFlags` y emite `EventFolderUpdated`.
3. Devuelve el detalle.

Los adjuntos **no** se bajan en este paso.

**`SaveAttachment`:** escribe en `destPath` con permisos `0644`. Si el archivo ya existe, lo reemplaza, porque el diálogo nativo de guardar ya pidió confirmación.

- [ ] **Step 1:** Escribir los tests:
  - `TestGetMessage_FetchesSanitizesAndMarksSeen`: el HTML del mensaje tiene `<script>` y una imagen remota. El detalle no contiene `script`, tiene `HasRemoteImages==true` y `Snippet` no está vacío. En el servidor, el mensaje queda con `\Seen`, y en la caché `Seen==true`.
  - `TestGetMessage_SecondCallUsesCache`: con el servidor IMAP cerrado después de la primera llamada, la segunda devuelve lo mismo sin error.
  - `TestGetMessage_InlineCIDBecomesDataURI`.
  - `TestGetMessage_TextOnly`: `HTML==""` y `Text` tiene el cuerpo.
  - `TestSaveAttachment_WritesExactBytes`: el PDF de testdata en `t.TempDir()` es idéntico byte a byte.
  - `TestSaveAttachment_BadIndex`: el código del `UserError` es `"not_found"`.
- [ ] **Step 2:** `go test ./core/mail/ -run 'GetMessage|SaveAttachment' -v`. Debe fallar.
- [ ] **Step 3:** Implementar.
- [ ] **Step 4:** `go test ./core/mail/ -v -race`. Debe pasar.
- [ ] **Step 5:** Commit: `feat(core): lectura de mensajes con caché y adjuntos bajo demanda`.

---

### Task 13: Núcleo — redactar, responder, reenviar y enviar (`core/mail/compose.go`)

**Files:**
- Create: `core/mail/compose.go`, `core/mail/compose_test.go`

**Interfaces:**
- Consumes: `GetMessage`, `attachmentBytes`, `mimex.Build/NewMessageID/Recipients`, `htmlx.RestoreRemoteImages/ToText`, `smtpx.Send`, `imapx.Append/AddFlags`, `store.SaveLocalDraft/ListLocalDrafts/DeleteLocalDraft`.
- Produces:
```go
func (c *Core) PrepareCompose(ctx context.Context, messageID int64, mode model.ComposeMode) (model.ComposeDraft, error)
func (c *Core) Send(ctx context.Context, req model.SendRequest) (model.SendResult, error)
func (c *Core) ListLocalDrafts(ctx context.Context) ([]model.LocalDraft, error)
func (c *Core) DeleteLocalDraft(ctx context.Context, id int64) error
```

**`PrepareCompose`:**
- **`reply`:** `To` = ReplyTo del original, o From si no hay ReplyTo.
- **`replyAll`:** `To` igual que en `reply`. `Cc` = To + Cc del original, sin el email de la cuenta, sin direcciones repetidas y sin las que ya están en `To`.
- **`forward`:** `To` vacío.
- **Asunto:** se antepone `Re: ` o `Fwd: `. Si el asunto ya empieza con el mismo prefijo (sin distinguir mayúsculas, aceptando `RE:`, `Re:`, `Fwd:`, `FW:`, `RV:`), no se repite.
- **`QuotedHTML`:** en respuestas, `<p>El {fecha}, {Nombre} &lt;{email}&gt; escribió:</p><blockquote type="cite" style="margin:0 0 0 .8ex;border-left:2px solid #ccc;padding-left:1ex">{HTML original con RestoreRemoteImages, o el texto escapado dentro de <pre>}</blockquote>`. En reenvíos, un bloque `<p>---------- Mensaje reenviado ----------<br>De: …<br>Fecha: …<br>Asunto: …<br>Para: …</p>` seguido del HTML original.
- **Fecha:** formato `2 de enero de 2026, 15:04`, en español y en hora local.
- `AccountID` es la cuenta del mensaje original.

**`Send`:**
1. Arma el `HTML` final como `req.BodyHTML` envuelto en `<div style="font-family:Arial,sans-serif;font-size:14px">…</div>`, seguido del `QuotedHTML` si hay `Reply`.
2. El `Text` es `htmlx.ToText` de ese HTML.
3. En respuestas: `InReplyTo` = MessageID del original y `References` = References del original + MessageID del original. En reenvíos: los adjuntos del original se cargan con `attachmentBytes`.
4. Hace `smtpx.Send`.
   - **Si falla:** `SaveLocalDraft(req, msg)` y devuelve `UserError` (con el código que corresponda, `"rejected"` incluido).
   - **Si funciona:**
     - Si `SaveSentCopy` está activo, hace `Append` en la carpeta con rol `sent` y flag `\Seen`. Si no existe una carpeta con ese rol o el `Append` falla, devuelve `SendResult{SentCopySaved:false, Warning:"El correo se envió, pero no pudimos guardar la copia en Enviados."}`.
     - Si es una respuesta, hace `AddFlags(\Answered)` en el original.
     - Emite `EventFolderUpdated` para Enviados.

- [ ] **Step 1:** Escribir los tests:
  - `TestPrepareCompose_ReplyAndReplyAllRecipients`: el original es From `a@x`, To `f@ej.com, b@x`, Cc `c@x`, y la cuenta es `f@ej.com`. Con `reply`, `To=[a@x]`. Con `replyAll`, `To=[a@x]` y `Cc=[b@x, c@x]`.
  - `TestPrepareCompose_SubjectPrefixNotDoubled`: `"Re: Hola"` con reply da `"Re: Hola"`; `"RV: Hola"` con forward da `"Fwd: RV: Hola"`.
  - `TestPrepareCompose_QuoteRestoresRemoteImages`: el `QuotedHTML` contiene `src="https://…"` y el texto `escribió:`.
  - `TestSend_EndToEnd`: un correo nuevo con From `{"Fabricio Peña","f@ej.com"}`, asunto `Información` y To `d@x`. El SMTP del testenv recibe 1 envelope y, al parsearlo, el asunto es `Información`. La carpeta `Sent` del servidor tiene 1 mensaje con `\Seen`, y el resultado es `SentCopySaved==true`.
  - `TestSend_ReplySetsThreadingAndAnswered`: los encabezados `In-Reply-To` y `References` son los esperados, y el original tiene `\Answered` en el servidor.
  - `TestSend_ForwardIncludesOriginalAttachments`: el mensaje recibido contiene el PDF original con los mismos bytes.
  - `TestSend_SMTPFailureSavesLocalDraft`: con `rechazo@ej.com`, el código es `"rejected"`, `ListLocalDrafts` tiene 1 borrador con el mismo `Subject` y la carpeta `Sent` sigue vacía.
  - `TestSend_NoSentFolderWarns`: si se borra `Sent` del servidor y se resincroniza, el envío funciona igual y devuelve `Warning` no vacío.
- [ ] **Step 2:** `go test ./core/mail/ -run 'Compose|Send' -v`. Debe fallar.
- [ ] **Step 3:** Implementar.
- [ ] **Step 4:** `go test ./... -race`. Debe pasar todo el backend.
- [ ] **Step 5:** Commit: `feat(core): redactar, responder, reenviar y enviar con copia en Enviados`.

---

### Task 14: Cáscara Wails (`app/`, `main.go`) y bindings

**Files:**
- Create: `app/{mailservice.go,notifier.go,paths.go}`, `app/paths_test.go`
- Modify: `main.go`, `frontend/bindings/**` (generados)

**Interfaces:**
- Consumes: todo `core/mail.Core`.
- Produces (los métodos que verá el frontend, con los mismos nombres y tipos de `model`):
```go
type MailService struct { core *mail.Core; app *application.App }
func (s *MailService) DetectSettings(email string) (autoconfig.Settings, error)
func (s *MailService) TestAccount(in model.AccountInput) error
func (s *MailService) AddAccount(in model.AccountInput) (model.Account, error)
func (s *MailService) ListAccounts() ([]model.Account, error)
func (s *MailService) RemoveAccount(id int64) error
func (s *MailService) ListFolders(accountID int64) ([]model.Folder, error)
func (s *MailService) SyncAccount(accountID int64) error
func (s *MailService) SyncFolder(folderID int64) error
func (s *MailService) ListMessages(folderID int64, q model.ListQuery) ([]model.MessageSummary, error)
func (s *MailService) GetMessage(id int64) (model.MessageDetail, error)
func (s *MailService) SaveAttachment(messageID int64, index int) (saved bool, err error) // abre el diálogo nativo "Guardar como" con el filename sugerido; false si se cancela
func (s *MailService) OpenExternal(url string) error                                  // solo http, https y mailto; cualquier otro esquema da error
func (s *MailService) PrepareCompose(messageID int64, mode model.ComposeMode) (model.ComposeDraft, error)
func (s *MailService) Send(req model.SendRequest) (model.SendResult, error)
func (s *MailService) ListLocalDrafts() ([]model.LocalDraft, error)
func (s *MailService) DeleteLocalDraft(id int64) error
func DataDir() (string, error) // $XDG_DATA_HOME/postacontrol o ~/.local/share/postacontrol
```

Si Wails v3 inyecta `context.Context` como primer parámetro, se usa ese contexto (verificarlo en v3.wails.io/features/bindings). Si no, se usa un `context.Background()` con timeout de 60 s por llamada. El servicio implementa `ServiceStartup(ctx, options) error`, que crea el `Core` con `secrets.NewKeyring()` y un `wailsNotifier` (`app.Event.Emit`) y llama a `StartBackgroundSync(ctx, 5*time.Minute)`. También implementa `ServiceShutdown() error`, que llama a `core.Close`. Para el diálogo de guardar y para abrir URLs se usan los managers de Wails v3 (`app.Dialog` y `app.Browser.OpenURL`); verificar la API exacta del diálogo en v3.wails.io/features/dialogs.

`main.go`: aplicación `Name: "PostaControl"`, una ventana con título `PostaControl`, de 1280×800 y mínimo 960×600, y el servicio `MailService` registrado. En la Fase 1, cerrar la ventana cierra la app.

- [ ] **Step 1:** Escribir `TestDataDir_UsesXDG` (con `t.Setenv("XDG_DATA_HOME", tmp)`, devuelve `tmp/postacontrol`) y `TestDataDir_FallbackHome`.
- [ ] **Step 2:** `go test ./app/ -v`. Debe fallar.
- [ ] **Step 3:** Implementar `app/` y `main.go`. Correr `wails3 generate bindings` y verificar que exista `frontend/bindings/postacontrol/app/mailservice.*` con los tipos de `model`.
- [ ] **Step 4:** `go test ./app/ -v` y `task check`. Deben pasar, y `TestCoreHasNoWailsImports` tiene que seguir en verde.
- [ ] **Step 5:** Commit: `feat(app): servicio Wails sobre el núcleo de correo`.

---

### Task 15: Frontend — base visual, tema, alta de cuenta y layout de 3 paneles

**Files:**
- Create: `frontend/src/lib/{queries.ts,events.ts}`, `frontend/src/features/theme/ThemeProvider.tsx`, `frontend/src/features/layout/MailShell.tsx`, `frontend/src/features/accounts/{Welcome.tsx,AccountSetupDialog.tsx}`, `frontend/src/features/accounts/AccountSetupDialog.test.tsx`
- Modify: `frontend/src/{App.tsx,main.tsx,index.css}`

**Interfaces:**
- Consumes: los bindings de `MailService`; `Events.On` de `@wailsio/runtime`.
- Produces:
```ts
// queries.ts — hooks TanStack Query; claves exactas:
// ["accounts"], ["folders", accountId], ["messages", folderId, unreadOnly], ["message", id], ["drafts"]
export function useAccounts(): UseQueryResult<Account[]>
export function useFolders(accountId: number | null): UseQueryResult<Folder[]>
export function useMessages(folderId: number | null, unreadOnly: boolean): UseInfiniteQueryResult<MessageSummary[]> // páginas de 100
export function useMessage(id: number | null): UseQueryResult<MessageDetail>
// events.ts
export function useMailEvents(queryClient: QueryClient): void
//   mail:accounts-changed → invalidate ["accounts"]
//   mail:folders-updated  → invalidate ["folders", accountId]
//   mail:folder-updated   → invalidate ["messages", folderId] y ["folders"]
//   mail:sync-error       → toast.error(message)
//   mail:sync-progress    → estado global {folderId, done, total} expuesto con useSyncProgress()
export function useSyncProgress(): { folderId: number; done: number; total: number } | null
```

**`ThemeProvider`:** sigue a `prefers-color-scheme`, pone la clase `dark` en `<html>` y no tiene interruptor manual en la Fase 1.

**`MailShell`:** usa `ResizablePanelGroup` horizontal con paneles de 18 %, 30 % y 52 % (mínimos de 14, 22 y 30). Guarda los tamaños en `localStorage["pc:layout"]` dentro de un try/catch. Recibe la cuenta, la carpeta y el mensaje seleccionados como estado propio, y pasa los slots a la Task 16.

**`App`:** si `useAccounts()` devuelve una lista vacía, muestra `Welcome`, una pantalla centrada con el título "Bienvenido a PostaControl", el subtítulo "Configurá tu primera cuenta de correo para empezar." y el botón "Agregar cuenta", que abre `AccountSetupDialog`. Si hay cuentas, muestra `MailShell`.

**`AccountSetupDialog` (dos pasos):**
1. **Email y contraseña.** Botón "Continuar". Llama a `DetectSettings` mientras muestra "Buscando la configuración…".
2. **Servidores, editables.**
   - Campos: IMAP host, puerto y seguridad (TLS o STARTTLS); SMTP host, puerto y seguridad; usuario; nombre visible.
   - Si la detección encontró algo, muestra el aviso "Encontramos la configuración" y los campos llegan completos.
   - Si falló, muestra "No pudimos detectar los servidores; cargalos a mano" y los campos llegan vacíos.
   - Botón "Probar y guardar": llama a `AddAccount` mostrando "Probando conexión…". Si hay error, muestra el mensaje del `UserError` en un `Alert` destructivo sin cerrar el diálogo. Si funciona, cierra el diálogo y lanza el toast "Cuenta agregada. Sincronizando…".

- [ ] **Step 1:** Escribir `AccountSetupDialog.test.tsx` con Vitest + Testing Library, mockeando el módulo de bindings:
  - `muestra los servidores detectados`: `DetectSettings` devuelve Hostinger y los inputs muestran `imap.hostinger.com` y `993`.
  - `permite carga manual si no detecta`: `DetectSettings` lanza un error, aparece el texto "cargalos a mano" y los inputs están vacíos y habilitados.
  - `muestra el error de AddAccount sin cerrar`: `AddAccount` lanza "El servidor rechazó el usuario o la contraseña.", el texto se ve y el diálogo sigue abierto.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/accounts`. Debe fallar.
- [ ] **Step 3:** Implementar los archivos listados.
- [ ] **Step 4:** Correr `npx vitest run` y `task check`. Deben pasar. Después `task dev`: con la base vacía, se ve la bienvenida y el diálogo abre. Verificarlo en tema claro y en tema oscuro.
- [ ] **Step 5:** Commit: `feat(ui): bienvenida, alta de cuenta y layout de tres paneles`.

---

### Task 16: Frontend — carpetas, lista de mensajes y lector seguro

**Files:**
- Create: `frontend/src/lib/{folders.ts,srcdoc.ts}`, `frontend/src/lib/{folders.test.ts,srcdoc.test.ts}`, `frontend/src/features/folders/FolderSidebar.tsx`, `frontend/src/features/messages/MessageList.tsx`, `frontend/src/features/reader/{MessageView.tsx,SafeHtmlFrame.tsx,AttachmentList.tsx}`

**Interfaces:**
- Consumes: los hooks de la Task 15 y los bindings `SyncFolder`, `SyncAccount`, `SaveAttachment`, `OpenExternal` y `RemoveAccount`.
- Produces:
```ts
export type FolderNode = Folder & { label: string; children: FolderNode[] }
export function buildFolderTree(folders: Folder[]): FolderNode[]
export function buildSrcDoc(cleanHtml: string, opts: { allowRemote: boolean }): string
```

**`buildFolderTree`:**
- Arma la jerarquía a partir de `Delimiter`.
- En la raíz, primero van las carpetas con rol en el orden inbox, drafts, sent, archive, junk, trash, y después el resto en orden alfabético.
- `label` en español según el rol: Entrada, Borradores, Enviados, Archivo, Spam y Papelera. Las carpetas sin rol usan el último segmento del nombre.
- Las subcarpetas de INBOX que tienen rol (por ejemplo `INBOX.Sent`) se suben a la raíz.

**`buildSrcDoc`:**
- Devuelve un documento HTML completo. En el `<head>` lleva `<meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; img-src data:{allowRemote ? ' https: http:' : ''}; font-src data:">`, `<base target="_blank">` y un estilo base (`body{margin:16px;font:14px/1.5 system-ui;color:#111;background:#fff;word-wrap:break-word} img{max-width:100%;height:auto}`).
- Si `allowRemote` es true, reemplaza cada `data-remote-src` por `src` usando `DOMParser`, nunca expresiones regulares.

**`SafeHtmlFrame`:**
- `<iframe sandbox="allow-same-origin" srcDoc=…>`, sin `allow-scripts` ni `allow-popups`.
- En `onLoad`: ajusta la altura al `scrollHeight` del body y engancha `click` en el `contentDocument`. Para cualquier `<a href>` hace `preventDefault` y llama a `OpenExternal(href)`.
- El iframe va dentro de una tarjeta de fondo blanco también en tema oscuro (decisión mía: los correos asumen fondo blanco).
- Si el mensaje no tiene HTML, muestra `Text` en un `<div class="whitespace-pre-wrap">`.

**`FolderSidebar`:**
- Arriba, la cuenta con su punto de color y un `DropdownMenu` con "Actualizar" (`SyncAccount`) y "Quitar cuenta". Esta última pide confirmación con el texto "Se borra la copia local y la contraseña guardada. Tus correos en el servidor no se tocan.".
- Debajo, el árbol con el contador de no leídos.
- Al hacer clic en una carpeta, la selecciona y llama a `SyncFolder` sin esperar el resultado.

**`MessageList`:**
- Encabezado con el nombre de la carpeta y un toggle "Todos / No leídos".
- Cada fila muestra el punto de color de la cuenta, el remitente (en negrita si no está leído), la fecha relativa (hora si es de hoy, "Ayer", o `d MMM`), el asunto, el snippet y el ícono 📎 si `HasAttachments`.
- Al final, el botón "Cargar más". Mientras sincroniza, una barra de progreso fina usa `useSyncProgress()`.
- Si la carpeta está vacía, muestra "No hay correos en esta carpeta.".

**`MessageView`:**
- Barra con "Responder", "Responder a todos" y "Reenviar", que abren el redactor de la Task 17 con `PrepareCompose`.
- Encabezado con avatar de iniciales, nombre y email del remitente, "para" con los destinatarios, la cuenta y la fecha completa.
- Si `HasRemoteImages`, muestra el aviso "Este correo tiene imágenes remotas bloqueadas para proteger tu privacidad." con el botón "Mostrar imágenes", que vuelve a renderizar con `allowRemote: true` solo para ese mensaje.
- `AttachmentList` muestra chips con nombre y tamaño; al hacer clic, llaman a `SaveAttachment`.
- Si no hay un mensaje seleccionado, se ve "Elegí un correo para leerlo.".

- [ ] **Step 1:** Escribir los tests:
  - `folders.test.ts`: `ordena por rol y traduce`: con `[Trash, INBOX, Zeta, INBOX.Sent, Alfa]` (delimitador `.`), las etiquetas en la raíz son `["Entrada","Enviados","Papelera","Alfa","Zeta"]`.
  - `folders.test.ts`: `anida subcarpetas`: `INBOX.Clientes.ACME` queda como hija de `Clientes`, que a su vez es hija de Entrada.
  - `srcdoc.test.ts`: `bloquea remotas por defecto`: la salida contiene `img-src data:;` y no contiene `src="https://`.
  - `srcdoc.test.ts`: `permite remotas a pedido`: con `allowRemote`, aparecen `img-src data: https: http:` y `src="https://t.example/p.gif"`.
  - `srcdoc.test.ts`: `no introduce scripts`: si la entrada tiene `<script>` (aunque el backend ya lo quitó), la salida tampoco tiene `allow-scripts` ni ejecuta nada; se verifica que la CSP tenga `default-src 'none'`.
- [ ] **Step 2:** `npx vitest run src/lib`. Debe fallar.
- [ ] **Step 3:** Implementar los archivos listados.
- [ ] **Step 4:** Correr `npx vitest run` y `task check`. Deben pasar.
- [ ] **Step 5:** Commit: `feat(ui): carpetas, lista de correos y lector con HTML aislado`.

---

### Task 17: Frontend — redactor (TipTap básico, destinatarios, responder/reenviar, envío)

**Files:**
- Create: `frontend/src/lib/{recipients.ts,recipients.test.ts}`, `frontend/src/features/compose/{ComposerContext.tsx,Composer.tsx,RichEditor.tsx,RecipientInput.tsx}`, `frontend/src/features/compose/Composer.test.tsx`
- Modify: `MailShell.tsx` (botón "Redactar" arriba del panel de carpetas), `MessageView.tsx` (conecta las acciones), `App.tsx` (aviso de borradores locales)

**Interfaces:**
- Consumes: los bindings `PrepareCompose`, `Send`, `ListLocalDrafts`, `DeleteLocalDraft`, `ListAccounts`.
- Produces:
```ts
export function parseRecipients(input: string): { valid: Address[]; invalid: string[] } // acepta "Nombre <a@b.c>", "a@b.c", separados por coma, punto y coma o salto de línea
export function useComposer(): { openNew(accountId: number): void; openFrom(messageId: number, mode: ComposeMode): Promise<void>; openDraft(d: LocalDraft): void }
```

**`Composer`:**
- Es un `Dialog` grande (`max-w-3xl`, alto `80vh`). Arriba tiene el selector "De" (cuentas; en la Fase 1 hay una sola), Para, CC (oculto detrás del botón "CC/CCO", que también muestra CCO) y Asunto.
- Debajo va `RichEditor`, con TipTap `StarterKit` (negrita, cursiva, listas, links). La barra fija tiene B, I, lista con viñetas, lista numerada y link (con un prompt para la URL). En la Fase 1 no hay menú "/".
- Si hay `QuotedHTML`, se muestra debajo del editor en un `SafeHtmlFrame` no editable, con el rótulo "Mensaje original".
- **"Enviar":**
  - Deshabilitado si no hay ningún destinatario válido.
  - Si hay inválidos, muestra "Revisá estas direcciones: …".
  - Al hacer clic llama a `Send({accountId, to, cc, bcc, subject, bodyHtml: editor.getHTML(), reply})`.
- **Si el envío funciona:** cierra el diálogo y muestra el toast "Correo enviado". Si hay `Warning`, muestra un toast de advertencia con ese texto.
- **Si el envío falla:** el diálogo queda abierto con el `Alert` del error y la nota "Guardamos una copia como borrador local.".
- Cerrar con contenido pide confirmación con el texto "¿Descartar este correo?".

**Borradores locales:** al arrancar, si `ListLocalDrafts()` no está vacío, aparece el toast persistente "Tenés N correo(s) sin enviar" con la acción "Abrir", que abre el más reciente con `openDraft`. Al enviarlo con éxito, se llama a `DeleteLocalDraft`.

- [ ] **Step 1:** Escribir los tests:
  - `recipients.test.ts`: `parsea nombre y email`: `'Fabricio Peña <f@ej.com>; b@x.com'` da 2 direcciones válidas, la primera con `name:"Fabricio Peña"`.
  - `recipients.test.ts`: `separa inválidos`: `'a@b.com, sin-arroba'` da `invalid == ["sin-arroba"]`.
  - `Composer.test.tsx`: `envía con reply context`: con `PrepareCompose` mockeado en modo reply, al tocar "Enviar" se llama a `Send` con `reply.mode=="reply"`, el `originalMessageId` correcto y `to` precargado.
  - `Composer.test.tsx`: `mantiene abierto y avisa si Send falla`: `Send` lanza "El servidor rechazó el envío: …", el diálogo sigue abierto y se ve "borrador local".
  - `Composer.test.tsx`: `deshabilita Enviar sin destinatarios`.
- [ ] **Step 2:** `npx vitest run src/lib/recipients.test.ts src/features/compose`. Debe fallar.
- [ ] **Step 3:** Implementar los archivos listados.
- [ ] **Step 4:** Correr `npx vitest run` y `task check`. Deben pasar.
- [ ] **Step 5:** Commit: `feat(ui): redactor con TipTap, respuestas, reenvíos y borradores locales`.

---

### Task 18: Aceptación manual con una cuenta real (criterio de éxito de la Fase 1)

**Files:**
- Create: `docs/fase-1-aceptacion.md` (checklist con resultados)

Esta tarea la hace **Fabricio**, con el implementador al lado. No se automatiza.

- [ ] **Step 1:** `task build` y ejecutar el binario de `bin/`. Alternativa: `task dev`.
- [ ] **Step 2:** Dar de alta una cuenta de Hostinger real. Se espera que la detección por MX complete los servidores de Hostinger, que "Probar y guardar" funcione, que aparezcan las carpetas con nombres en español y que la Entrada se llene de lo más nuevo a lo más viejo.
- [ ] **Step 3:** Abrir un correo HTML con imágenes. Se espera que las imágenes estén bloqueadas, que "Mostrar imágenes" las muestre, que los links se abran en el navegador del sistema y que el correo quede leído también en el webmail de Hostinger.
- [ ] **Step 4:** Guardar un adjunto y abrirlo.
- [ ] **Step 5:** Enviar un correo nuevo a otra cuenta propia, con asunto `Prueba PostaControl — Información ñ`, negrita, una lista y un link. Se espera que llegue con formato y acentos bien, tanto en el webmail como en Thunderbird, y que aparezca en Enviados de PostaControl, del webmail y de Thunderbird **una sola vez** (si aparece duplicado, Hostinger guarda la copia por su cuenta y hay que desactivar `SaveSentCopy` para esa cuenta).
- [ ] **Step 6:** Responder ese correo desde la otra cuenta y reenviar uno con adjunto. Se espera que la cita y los encabezados de hilo sean correctos (en Thunderbird el hilo se agrupa) y que el adjunto viaje en el reenvío.
- [ ] **Step 7:** Volver a mandar un correo con la red desconectada. Se espera ver el error, que el redactor siga abierto y que quede un borrador local. Después de reconectar, el aviso permite abrirlo y enviarlo.
- [ ] **Step 8:** Anotar los resultados en `docs/fase-1-aceptacion.md` y hacer commit con `docs: aceptación de la Fase 1`.

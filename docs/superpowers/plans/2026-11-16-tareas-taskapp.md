# Tareas y vinculación con Taskapp — plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Crear tareas y tickets de Taskapp desde un correo (con el `.eml` adjunto), sumar respuestas del hilo como comentarios, ver mis pendientes de Taskapp y manejar tareas locales con avisos.

**Architecture:** Un paquete nuevo `core/taskapp` (cliente REST, login por navegador y reglas puras: proyecto sugerido, asunto limpio) sin dependencias de `core/mail`. La fachada `core/mail` suma tareas locales, avisos (en el mismo ciclo que los envíos programados), conexión, creación y vínculos, guardando en SQLite (migración v17). `app/` expone todo a la UI con envoltorios finos y traduce el evento de aviso a una notificación del escritorio. El frontend suma la vista "Tareas", el diálogo de creación en el lector y la página de Ajustes → Taskapp.

**Tech Stack:** Go (`net/http`, `httptest`, `emersion/go-message`), SQLite, Wails v3, React + TS + TanStack Query + shadcn/ui, Vitest.

**Spec:** `docs/superpowers/specs/2026-11-16-tareas-taskapp-design.md`

## Global Constraints

- `core/...` no importa Wails (`arch_test.go` lo verifica).
- El token de Taskapp vive solo en el llavero (`secrets.Store`, clave `taskapp-token`); nunca en SQLite, logs ni eventos.
- La base pasa a la versión 17 (`core/store/schema_v17.sql`); las notas de la versión deben avisar que una versión anterior ya no la abre.
- Contra Taskapp de producción **solo lecturas** durante el desarrollo; las escrituras se prueban contra `httptest.Server`.
- Sin conexión a Taskapp todo lo local sigue andando; los errores de Taskapp llegan a la UI como `*mail.UserError` en español.
- Límite de subida de Taskapp: 20 MB (`taskapp.MaxUpload = 20 << 20`).
- Anticipaciones de aviso permitidas (minutos): `0, 5, 15, 30, 60, 1440`; por defecto `15`.
- Dominios públicos que nunca sugieren proyecto: `gmail.com, googlemail.com, hotmail.com, hotmail.com.ar, outlook.com, outlook.com.ar, live.com, live.com.ar, yahoo.com, yahoo.com.ar, icloud.com, me.com, msn.com, aol.com, proton.me, protonmail.com, gmx.com, fibertel.com.ar, speedy.com.ar, arnet.com.ar`.
- Comentarios de "Sumar a la tarea" siempre con `is_internal: true`.
- Verificación completa: `task check`.

## Review Focus

1. **Correo sin `Message-ID` o con `thread_root` vacío** → el vínculo se guarda igual y el chip aparece en ese correo (se usa el id local como raíz). Test en Task 5.
2. **La tarea se creó pero el adjunto falló (401, red, 413)** → la tarea queda vinculada con `AttachPending`, reintentar sube solo el adjunto y nunca crea otra tarea. Test en Task 8.
3. **Taskapp caído al abrir la vista** → se muestra la última copia con `Stale` y su hora; sin copia previa, error visible y las locales igual se listan. Tests en Task 7 y Task 11.
4. **Tarea local con fecha y sin hora, u hora cambiada después de avisar** → sin hora no avisa; cambiar fecha, hora o anticipación vuelve a armar el aviso. Test en Task 6.
5. **Remitente en mayúsculas, con nombre o desde un subdominio** (`Juan <JUAN@Cliente.com>`, `x@mail.cliente.com`) → la comparación ignora mayúsculas y el dominio tiene que coincidir exacto (un subdominio no cuenta). Test en Task 2.

---

### Task 0: Verificar la API de Taskapp en producción (solo lectura)

**Files:**
- Create: `docs/superpowers/plans/2026-11-16-tareas-taskapp-registro.md`

**Interfaces:**
- Produces: el registro con, para cada operación, método, ruta, cuerpo y forma de respuesta reales. Las tareas 1 y 3 los copian.

- [ ] **Step 1: Averiguar las URL reales.** Pedile a Fabricio o deducilo del frontend de producción (`https://taskapp.codecontrol.com.ar`): la URL base de la **API** y la del **frontend** (página `/cli-auth`, tareas en `/tasks/:id`, tickets). Anotá las dos.
- [ ] **Step 2: Leer con el token de la CLI** (`Authorization: Bearer …`; el de `~/.claude.json` → `mcpServers.cc-tickets` sirve si la API acepta el mismo JWT; si no, pedile uno a Fabricio). Hacé solo `GET`: `/api/auth/me`, `/api/projects`, `/api/statuses`, `/api/tasks?assignee_id=<me>`, `/api/tasks/{id}` de una tarea y de un ticket. Las herramientas de lectura del MCP `cc-tickets` (`list_tasks verbose`, `get_task`) sirven para comparar. **No escribas nada.**
- [ ] **Step 3: Leer en el código las escrituras.** Para crear tarea, crear ticket, comentar, subir (`/api/upload`) y adjuntar, mirá en `~/desarrollo_local/taskapp_web/taskApp-backend` (y en el repo actual de Taskapp si Fabricio te da acceso) los handlers y modelos: cuerpo exacto, código de respuesta y forma devuelta. Confirmá si `POST /api/tasks/{id}/comments` acepta `attachments` e `is_internal`, si adjuntar es `PUT /api/tasks/{id}` con la lista completa de `attachments`, y cómo se distingue un ticket de una tarea en el JSON (`source` u otro campo).
- [ ] **Step 4: Escribir el registro.** Una tabla por operación con ruta, cuerpo, respuesta y el JSON real de una tarea y de un proyecto (con `approver_emails` y `workspace_id`). Sin tokens ni API keys de proyectos: redactalos. También van las dos URL por defecto.
- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/2026-11-16-tareas-taskapp-registro.md
git commit -m "docs(taskapp): registro de la API verificada"
```

---

### Task 1: Cliente REST `core/taskapp`

**Files:**
- Create: `core/taskapp/client.go`, `core/taskapp/types.go`
- Test: `core/taskapp/client_test.go`

**Interfaces:**
- Consumes: el registro de Task 0 (rutas y JSON).
- Produces:

```go
type Client struct {
	BaseURL string       // API, sin barra final
	Token   string
	HTTP    *http.Client // nil = http.DefaultClient
}
var ErrUnauthorized = errors.New("taskapp: el token fue rechazado")
type APIError struct{ Status int; Msg string } // Error() = Msg; Msg sale del campo "error" del JSON
const MaxUpload = 20 << 20

type User    struct{ ID int64; Name, Email string }
type Status  struct{ ID int64; Name string; Completed bool }
type Project struct{ ID, WorkspaceID int64; Name, Color, Status string; ApproverEmails []string }
type Task    struct{ ID int64; ProjectID int64; Title, Description, Priority string; DueDate string /* AAAA-MM-DD o "" */; Status Status; Ticket bool; Attachments []string }
type NewTask   struct{ WorkspaceID, ProjectID int64; Title, Description, Priority, DueDate string }
type NewTicket struct{ ProjectID int64 /* 0 = sin proyecto */; Title, Description, Priority string }

func (c *Client) Me(ctx context.Context) (User, error)
func (c *Client) Projects(ctx context.Context) ([]Project, error)
func (c *Client) ListMyPending(ctx context.Context, assigneeID int64) ([]Task, error) // excluye Status.Completed
func (c *Client) GetTask(ctx context.Context, id int64) (Task, error)
func (c *Client) CreateTask(ctx context.Context, t NewTask) (Task, error)
func (c *Client) CreateTicket(ctx context.Context, t NewTicket) (Task, error)
func (c *Client) Upload(ctx context.Context, filename string, data []byte) (key string, err error)
func (c *Client) SetAttachments(ctx context.Context, taskID int64, keys []string) error
func (c *Client) AddComment(ctx context.Context, taskID int64, bodyHTML string, attachments []string, internal bool) error
```

El JSON va con `snake_case` según el registro. `Task.Status` se arma del objeto `status` embebido (`is_completed`). `Ticket` sale del campo que identificó Task 0.

- [ ] **Step 1: Escribir los tests que fallan** con un `httptest.Server` que valida método, ruta, `Authorization: Bearer tok` y el cuerpo:
  - `TestCreateTask_SendsWorkspaceAndProject`: el cuerpo tiene `workspace_id:2`, `project_id:5`, `title`, `description`, `priority:"none"`, y `due_date` solo si no está vacío.
  - `TestCreateTicket_NoDueDate`: el cuerpo no tiene `due_date`, y con `ProjectID 0` tampoco `project_id`.
  - `TestListMyPending_DropsCompleted`: el servidor devuelve 3 tareas, una con `status.is_completed=true`; quedan 2.
  - `TestUnauthorized`: un 401 devuelve `errors.Is(err, ErrUnauthorized)`.
  - `TestAPIError_UsesErrorField`: un 422 con `{"error":"cannot comment on a completed task"}` devuelve `*APIError{Status:422}` con ese texto.
  - `TestUpload_Multipart`: el campo del archivo y el nombre coinciden con el registro, y devuelve `key`.
  - `TestUpload_TooBig`: con `len(data) > MaxUpload` devuelve un error sin hacer el pedido.
  - `TestAddComment_Internal`: el cuerpo tiene `is_internal:true` y `attachments:["k1"]`.
  - `TestSetAttachments_FullList`: manda un `PUT` con `attachments` completo.
- [ ] **Step 2:** `go test ./core/taskapp/ -run 'Test' -v` → FAIL (no compila).
- [ ] **Step 3: Implementar** según las firmas. Un helper `do(ctx, method, path string, in, out any) error` mapea el 401 y los `APIError`. Timeout: el que traiga `HTTP`.
- [ ] **Step 4:** `go test ./core/taskapp/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(taskapp): cliente REST de Taskapp`.

---

### Task 2: Proyecto sugerido y asunto limpio

**Files:**
- Create: `core/taskapp/match.go`
- Test: `core/taskapp/match_test.go`

**Interfaces:**
- Produces:

```go
func SuggestProject(sender string, projects []Project) int64 // 0 = sin sugerencia
func CleanSubject(subject string) string
func IsPublicDomain(domain string) bool
```

`SuggestProject` sigue este orden:
1. Coincidencia exacta de la dirección, sin distinguir mayúsculas, contra `ApproverEmails` de proyectos con `Status == "active"`.
2. Si no hay, el dominio exacto (sin subdominios), siempre que no sea público.
3. Si en un paso coincide más de un proyecto distinto, devuelve 0 y no sigue al paso 2.

`sender` es la dirección pelada (`model.Address.Email`).

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestSuggest_ExactEmail`: `"pmolina@sinergiambiental.com.ar"` → id de Sinergia.
  - `TestSuggest_CaseInsensitive`: `"PMolina@SinergiAmbiental.com.ar"` → Sinergia.
  - `TestSuggest_Domain`: `"otro@sinergiambiental.com.ar"` → Sinergia.
  - `TestSuggest_SubdomainDoesNotMatch`: `"x@mail.sinergiambiental.com.ar"` → 0.
  - `TestSuggest_PublicDomainNever`: un proyecto con aprobador `yordanpz@hotmail.com` y remitente `otro@hotmail.com` → 0. El mismo `yordanpz@hotmail.com` → ese proyecto, porque es coincidencia exacta.
  - `TestSuggest_AmbiguousIsEmpty`: dos proyectos con el mismo dominio aprobador → 0.
  - `TestSuggest_IgnoresInactive`: un proyecto con `Status "completed"` no se sugiere.
  - `TestCleanSubject`: `"Re: RV: Fwd:  Error al exportar"` → `"Error al exportar"`. También `"RE:FW: x"` → `"x"`, `"Respuesta pendiente"` → sin cambios, y `""` → `""`.
- [ ] **Step 2:** `go test ./core/taskapp/ -run 'TestSuggest|TestCleanSubject' -v` → FAIL.
- [ ] **Step 3: Implementar.** La lista de dominios públicos sale de Global Constraints. `CleanSubject` saca en bucle los prefijos `re|rv|fw|fwd|reenviar|tr`, sin distinguir mayúsculas, seguidos de `:` y espacios.
- [ ] **Step 4:** `go test ./core/taskapp/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(taskapp): proyecto sugerido por remitente y asunto limpio`.

---

### Task 3: Login por navegador (mismo flujo que la CLI)

**Files:**
- Create: `core/taskapp/login.go`
- Test: `core/taskapp/login_test.go`

**Interfaces:**
- Produces:

```go
type Grant struct{ Token string; ExpiresAt time.Time }
var ErrDenied = errors.New("taskapp: autorización rechazada")
// Login escucha en 127.0.0.1:<libre>, abre <webURL>/cli-auth?cb=…&state=… con
// open y espera el callback hasta que ctx venza.
func Login(ctx context.Context, webURL string, open func(url string) error) (Grant, error)
```

El callback recibe `token`, `expires_at` y `state`. Si se deniega, recibe el parámetro que haya registrado Task 0 (`error`). El `state` son 16 bytes de `crypto/rand` en hex. Un pedido con otro `state` responde 400 y la espera sigue. La página de respuesta es un HTML corto: "Listo. Ya podés volver a PostaControl." o "No se autorizó la conexión.". El servidor se cierra siempre al volver.

- [ ] **Step 1: Escribir los tests que fallan.** Cada uno usa un `open` falso que parsea la URL, toma `cb` y `state` y hace el GET:
  - `TestLogin_ReturnsToken`.
  - `TestLogin_WrongStateIgnored`: primero llega un state falso (400) y después el bueno (OK).
  - `TestLogin_Denied` → `ErrDenied`.
  - `TestLogin_Timeout`: con un `ctx` de 100 ms y un `open` que no hace nada → `context.DeadlineExceeded`, y el puerto queda libre (se puede escuchar de nuevo en él).
  - `TestLogin_CallbackIsLoopback`: `cb` empieza con `http://127.0.0.1:`.
- [ ] **Step 2:** `go test ./core/taskapp/ -run TestLogin -v` → FAIL.
- [ ] **Step 3: Implementar** con `net.Listen("tcp","127.0.0.1:0")` y un `http.Server` con un solo handler en `/callback`.
- [ ] **Step 4:** `go test ./core/taskapp/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(taskapp): conectar por navegador como la CLI`.

---

### Task 4: `.eml` sin adjuntos

**Files:**
- Create: `core/mimex/strip.go`
- Test: `core/mimex/strip_test.go`

**Interfaces:**
- Produces:

```go
type DroppedPart struct{ Filename string; Size int64 }
// StripAttachments devuelve el mensaje con los encabezados, el texto y el HTML,
// sin las partes adjuntas (Content-Disposition attachment o con filename,
// salvo text/plain y text/html sin nombre). Las imágenes inline (cid) también
// se van.
func StripAttachments(raw []byte) ([]byte, []DroppedPart, error)
```

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestStrip_KeepsBodies`: un `multipart/mixed` con `multipart/alternative` (text + html) y un PDF da un resultado que se puede parsear, con el texto y el HTML intactos, y `DroppedPart{"informe.pdf", <bytes decodificados>}`.
  - `TestStrip_NoAttachmentsUnchanged`: con un `text/plain` simple, los bytes vuelven iguales y la lista vacía.
  - `TestStrip_KeepsHeaders`: se conservan `From`, `Subject`, `Message-ID` y `Date`.
- [ ] **Step 2:** `go test ./core/mimex/ -run TestStrip -v` → FAIL.
- [ ] **Step 3: Implementar** con `github.com/emersion/go-message`, recorriendo y reescribiendo con `message.CreateWriter`.
- [ ] **Step 4:** `go test ./core/mimex/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(mimex): copia del correo sin adjuntos`.

---

### Task 5: Esquema v17 y store de tareas

**Files:**
- Create: `core/store/schema_v17.sql`, `core/store/tasks.go`
- Modify: `core/store/migrate.go` (embed y agregar a `migrations`), `core/model/model.go`
- Test: `core/store/tasks_test.go`

**Interfaces:**
- Produces (modelo, en `core/model/model.go`):

```go
type LocalTask struct {
	ID              int64      `json:"id"`
	Title           string     `json:"title"`
	Notes           string     `json:"notes"`
	DueDate         string     `json:"dueDate"`      // "AAAA-MM-DD" o ""
	DueTime         string     `json:"dueTime"`      // "HH:MM" o ""; solo con DueDate
	RemindBefore    *int       `json:"remindBefore"` // minutos; nil = el de Configuración
	SourceMessageID string     `json:"sourceMessageId"` // Message-ID del correo de origen o ""
	SourceSubject   string     `json:"sourceSubject"`
	DoneAt          *time.Time `json:"doneAt"`
	CreatedAt       time.Time  `json:"createdAt"`
}
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
	StateKnown   bool   `json:"stateKnown"`   // false = no se pudo preguntar a Taskapp
}
```

- Tablas (`schema_v17.sql`):
  - `local_tasks`: columnas de `LocalTask`, más `reminded_at INTEGER NULL`.
  - `task_links`: `thread_root`, `task_id`, `ticket`, `project_name`, `title`, `source_message_id`, `attach_pending`, `created_at`, con índice por `thread_root`.
  - `task_link_messages(link_id REFERENCES task_links ON DELETE CASCADE, message_id TEXT, PRIMARY KEY(link_id, message_id))`, que guarda los correos ya sumados por Message-ID.
  - `taskapp_cache(id INTEGER PRIMARY KEY CHECK(id=1), json TEXT, fetched_at INTEGER)`.
- Store:

```go
func (s *Store) ListLocalTasks(ctx context.Context, includeDone bool) ([]model.LocalTask, error)
func (s *Store) GetLocalTask(ctx context.Context, id int64) (model.LocalTask, error) // sql.ErrNoRows si no está
func (s *Store) SaveLocalTask(ctx context.Context, t model.LocalTask, rearm bool) (model.LocalTask, error) // id 0 = alta; rearm pone reminded_at = NULL
func (s *Store) SetLocalTaskDone(ctx context.Context, id int64, at *time.Time) error
func (s *Store) DeleteLocalTask(ctx context.Context, id int64) error
func (s *Store) PendingReminders(ctx context.Context) ([]model.LocalTask, error) // DueTime != "", DoneAt NULL, reminded_at NULL
func (s *Store) MarkReminded(ctx context.Context, id int64, at time.Time) error
func (s *Store) AddTaskLink(ctx context.Context, l model.TaskLink, sourceMessageID string) (model.TaskLink, error)
func (s *Store) TaskLinksForThread(ctx context.Context, root string) ([]model.TaskLink, error)
func (s *Store) GetTaskLink(ctx context.Context, id int64) (model.TaskLink, error)
func (s *Store) SetAttachPending(ctx context.Context, linkID int64, pending bool) error
func (s *Store) MarkAddedToLink(ctx context.Context, linkID int64, messageID string) error
func (s *Store) AddedToLink(ctx context.Context, linkID int64, messageID string) (bool, error)
func (s *Store) MessageTaskMeta(ctx context.Context, id int64) (headerID, threadRoot string, size int64, err error) // threadRoot vacío → "local:<id>"
func (s *Store) MessageIDByHeader(ctx context.Context, headerID string) (int64, error) // el más nuevo; sql.ErrNoRows si no está
func (s *Store) SaveTaskappCache(ctx context.Context, json string, at time.Time) error
func (s *Store) TaskappCache(ctx context.Context) (json string, at time.Time, ok bool, err error)
func (s *Store) ClearTaskappData(ctx context.Context) error // borra la caché; los vínculos se conservan
```

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestMigrate_V17` (en `store_test.go`, siguiendo el patrón de las migraciones anteriores): una base v16 migra y quedan las cuatro tablas.
  - `TestLocalTask_RoundTrip`: alta, lectura, edición con `rearm=false` (conserva `reminded_at`) y con `rearm=true` (lo limpia).
  - `TestListLocalTasks_HidesDone`.
  - `TestPendingReminders_OnlyTimedUndone`: con 4 tareas (sin hora, hecha, ya avisada y pendiente) vuelve solo la pendiente.
  - `TestTaskLinks_ByThread`: dos vínculos en un hilo y uno en otro.
  - `TestAddedToLink`: `MarkAddedToLink` es idempotente.
  - `TestMessageTaskMeta_EmptyRoot`: un mensaje con `thread_root=''` devuelve `"local:<id>"`.
- [ ] **Step 2:** `go test ./core/store/ -run 'TestMigrate_V17|TestLocalTask|TestPendingReminders|TestTaskLinks|TestAddedToLink|TestMessageTaskMeta' -v` → FAIL.
- [ ] **Step 3: Implementar** el SQL y los métodos siguiendo el estilo de `contacts.go`.
- [ ] **Step 4:** `go test ./core/store/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(store): tablas de tareas locales y vínculos con Taskapp (v17)`.

---

### Task 6: Tareas locales y avisos en el núcleo

**Files:**
- Create: `core/mail/tasks.go`, `core/mail/reminders.go`
- Modify: `core/mail/scheduler.go` (llamar a `fireDueReminders` en cada vuelta), `core/mail/events.go`, `core/mail/settings.go` (validar), `core/model/model.go` (`Settings.TaskReminderMinutes *int` con `json:"taskReminderMinutes"`), `core/mail/backup.go` (exportar e importar locales; `BackupImport.TasksAdded/TasksSkipped`)
- Test: `core/mail/tasks_test.go`, `core/mail/reminders_test.go`, `core/mail/backup_test.go`

**Interfaces:**
- Consumes: el store de Task 5.
- Produces:

```go
const EventTasksChanged = "mail:tasks-changed"  // data: nil
const EventTaskReminder = "mail:task-reminder"  // data: model.TaskReminder
type TaskReminder struct{ TaskID int64 `json:"taskId"`; Title string `json:"title"`; At time.Time `json:"at"` } // en model
func (c *Core) ListLocalTasks(ctx context.Context, includeDone bool) ([]model.LocalTask, error)
func (c *Core) SaveLocalTask(ctx context.Context, t model.LocalTask) (model.LocalTask, error)
func (c *Core) SetLocalTaskDone(ctx context.Context, id int64, done bool) error
func (c *Core) DeleteLocalTask(ctx context.Context, id int64) error
func (c *Core) fireDueReminders(ctx context.Context, now time.Time) error
func remindAt(t model.LocalTask, defaultMin int, loc *time.Location) (time.Time, bool)
```

- `SaveLocalTask` valida y devuelve `UserError{Code:"invalid"}` con estos mensajes:
  - Título vacío: "La tarea necesita un título."
  - `DueTime` sin `DueDate`: "Para poner hora, elegí una fecha."
  - Formatos `AAAA-MM-DD` y `HH:MM` inválidos: "Fecha u hora inválida."
  - `RemindBefore` fuera de `0,5,15,30,60,1440`: "Anticipación del aviso inválida."
- Al editar, pasa `rearm=true` si cambió `DueDate`, `DueTime` o `RemindBefore`. Emite `EventTasksChanged`.
- `fireDueReminders`:
  - Por cada pendiente con `remindAt <= now`: si `now - remindAt <= 24h`, emite `EventTaskReminder`. En los dos casos la marca como avisada.
  - Usa `time.Local`. `defaultMin` sale de `Settings.TaskReminderMinutes` (nil = 15).
  - `setSettingsLocked` rechaza valores fuera de la lista.

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestSaveLocalTask_Validation`: un caso de tabla por cada mensaje de arriba.
  - `TestRemindAt`: `2026-11-19 10:00`, `RemindBefore nil` y default 15 → `09:45`. Con `RemindBefore 0` → `10:00`. Sin hora → `false`.
  - `TestFireDueReminders_FiresOnce`: con un notifier falso, dos vueltas seguidas emiten un solo evento.
  - `TestFireDueReminders_SkipsOlderThan24h`: un aviso vencido hace 25 h no se emite y queda marcado.
  - `TestFireDueReminders_RearmOnEdit`: después de avisar, cambiar la hora a una futura y avanzar el reloj → vuelve a avisar.
  - `TestFireDueReminders_DoneOrDeletedNever`.
  - `TestBackup_IncludesLocalTasks`: exportar e importar en una base nueva conserva título, notas, fecha, hora y anticipación. Reimportar saltea las repetidas (mismo título + fecha + hora) y suma `TasksSkipped`.
  - `TestSetSettings_RejectsBadReminder`.
- [ ] **Step 2:** `go test ./core/mail/ -run 'TestSaveLocalTask|TestRemindAt|TestFireDueReminders|TestBackup_IncludesLocalTasks|TestSetSettings_RejectsBadReminder' -v` → FAIL.
- [ ] **Step 3: Implementar.** En `StartScheduler`, agregar `c.fireDueReminders(ctx, now)` después de `returnDueSnoozes`, con el mismo `slog.Warn` si falla.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(tareas): tareas locales con avisos y respaldo`.

---

### Task 7: Conexión con Taskapp, proyectos y lista de pendientes

**Files:**
- Create: `core/mail/taskapp.go`
- Modify: `core/mail/core.go` (`Config.TaskappHTTP *http.Client`, nil = cliente con timeout de 20 s; caché de proyectos y `me` en memoria con mutex), `core/mail/events.go`, `core/model/model.go`
- Test: `core/mail/taskapp_test.go`

**Interfaces:**
- Consumes: `taskapp.Client`, `taskapp.Login` y `SuggestProject` (Tasks 1–3), y la caché del store (Task 5).
- Produces:

```go
const EventTaskappChanged = "mail:taskapp-changed" // data: nil (conectó, desconectó o hay lista nueva)
const EventTaskappAuth    = "mail:taskapp-auth"    // data: nil (Taskapp rechazó el token)
const taskappSecret = "taskapp-token"
// Settings: TaskappAPIURL string `json:"taskappApiUrl"`, TaskappWebURL string `json:"taskappWebUrl"` (vacío = defaults del registro de Task 0)
type TaskappStatus  struct{ Connected bool; UserName, UserEmail string }                 // json camelCase
type TaskappProject struct{ ID, WorkspaceID int64; Name, Color string }
type TaskappTask    struct{ ID int64; Ticket bool; Title, Priority, DueDate, StatusName string; ProjectID int64; ProjectName, ProjectColor, URL, Description string }
type TaskappList    struct{ Tasks []TaskappTask; FetchedAt time.Time; Stale bool; Error string } // Tasks nunca nil
func (c *Core) ConnectTaskapp(ctx context.Context, open func(string) error) (model.TaskappStatus, error)
func (c *Core) DisconnectTaskapp(ctx context.Context) error
func (c *Core) TaskappStatus(ctx context.Context) (model.TaskappStatus, error)
func (c *Core) TaskappProjects(ctx context.Context) ([]model.TaskappProject, error) // solo activos, orden por nombre
func (c *Core) ListTaskappTasks(ctx context.Context) (model.TaskappList, error)
func (c *Core) taskappClient(ctx context.Context) (*taskapp.Client, error)
func (c *Core) taskappErr(err error) error
func (c *Core) taskURL(id int64) string // <TaskappWebURL>/tasks/<id> (o la ruta que diga el registro)
```

- `ConnectTaskapp`: `taskapp.Login` con timeout de 5 min, después `Me`. Recién ahí guarda el token en el llavero y emite `EventTaskappChanged`. Si `Me` falla, no guarda nada.
- `DisconnectTaskapp`: borra el secreto, ejecuta `ClearTaskappData` y emite.
- `taskappErr` mapea así:
  - `ErrUnauthorized` → emite `EventTaskappAuth` y devuelve `UserError{Code:"taskapp-auth", Msg:"Taskapp rechazó la conexión. Volvé a conectar desde Configuración."}`.
  - Errores de red → `UserError{Code:"taskapp-offline", Msg:"No pudimos conectar con Taskapp."}`.
  - `*APIError` → `UserError{Code:"taskapp", Msg:"Taskapp: "+msg}`.
  - Sin token → `UserError{Code:"taskapp-disconnected", Msg:"Conectá Taskapp en Configuración."}`.
- `ListTaskappTasks`:
  - Si anda: pide las pendientes de `me.ID` y les pone proyecto, color y URL. Guarda el JSON en la caché con `FetchedAt = now`.
  - Si falla por red o error de Taskapp: devuelve la caché con `Stale = true` y `Error = msg`. Sin caché, devuelve el error.
  - Si falla con 401: devuelve el error de auth aunque haya caché, porque hay que reconectar.

- [ ] **Step 1: Escribir los tests que fallan.** Usan un `httptest.Server` y `secrets.NewMemory()`:
  - `TestConnectTaskapp_StoresTokenAfterMe`: `open` falso que completa el callback.
  - `TestConnectTaskapp_MeFailsNoToken`.
  - `TestDisconnectTaskapp_ClearsSecretAndCache`.
  - `TestListTaskappTasks_CachesAndServesStale`: la primera llamada anda; con el servidor apagado, la segunda devuelve las mismas tareas con `Stale=true`.
  - `TestListTaskappTasks_NoCacheOffline` → `taskapp-offline`.
  - `TestTaskapp401_EmitsAuthEvent`.
  - `TestTaskappProjects_OnlyActiveSorted`.
- [ ] **Step 2:** `go test ./core/mail/ -run 'TestConnectTaskapp|TestDisconnectTaskapp|TestListTaskappTasks|TestTaskapp401|TestTaskappProjects' -v` → FAIL.
- [ ] **Step 3: Implementar** según las firmas.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(taskapp): conexión, proyectos y pendientes con caché`.

---

### Task 8: Crear desde un correo y sumar al hilo

**Files:**
- Create: `core/mail/taskapp_create.go`
- Modify: `core/model/model.go`
- Test: `core/mail/taskapp_create_test.go`

**Interfaces:**
- Consumes: Tasks 1, 2, 4, 5 y 7, y además `GetMessage`, `rawMessage` y `htmlx.Sanitize`/`RestoreRemoteImages`.
- Produces:

```go
type TaskappDraft struct {
	MessageID       int64    `json:"messageId"`
	Title           string   `json:"title"`
	DescriptionHTML string   `json:"descriptionHtml"`
	ProjectID       int64    `json:"projectId"` // 0 = sin sugerencia
	Oversize        bool     `json:"oversize"`  // el .eml no entra en MaxUpload: se adjunta sin adjuntos
}
type TaskappCreate struct {
	MessageID       int64  `json:"messageId"`
	Ticket          bool   `json:"ticket"`
	ProjectID       int64  `json:"projectId"`
	Title           string `json:"title"`
	DescriptionHTML string `json:"descriptionHtml"`
	Priority        string `json:"priority"` // none|low|medium|high|urgent
	DueDate         string `json:"dueDate"`  // solo tarea
}
func (c *Core) TaskappDraftFor(ctx context.Context, messageID int64) (model.TaskappDraft, error)
func (c *Core) CreateTaskappItem(ctx context.Context, req model.TaskappCreate) (model.TaskLink, error)
func (c *Core) RetryTaskLinkAttachment(ctx context.Context, linkID int64) (model.TaskLink, error)
func (c *Core) TaskLinksForMessage(ctx context.Context, messageID int64) ([]model.TaskLink, error)
func (c *Core) AddMessageToTask(ctx context.Context, linkID, messageID int64) error
func (c *Core) CreateLocalTaskFrom(ctx context.Context, messageID int64) (model.LocalTask, error) // título limpio y vínculo al correo; no guarda
func (c *Core) FindMessageByHeader(ctx context.Context, headerID string) (model.MessageSummary, error) // UserError "El correo ya no está en esta PC." si no está
func taskDescriptionHTML(d model.MessageDetail, loc *time.Location) string
```

- **`taskDescriptionHTML`.** Arranca con el encabezado `<p><strong>De:</strong> Nombre &lt;mail&gt; · <strong>Fecha:</strong> 16/11/2026 10:32</p>`, con todo escapado. Sigue el cuerpo:
  - Si hay HTML: el HTML ya sanitizado con las imágenes remotas restauradas.
  - Si no: el texto en `<p>` por párrafo y `<br>` por línea.
- **`TaskappDraftFor`.** Usa el asunto con `CleanSubject`. Calcula `SuggestProject(d.From.Email, projects)` y `Oversize = size > MaxUpload - 64<<10`, con el `size` de `MessageTaskMeta`.
- **`CreateTaskappItem`.**
  1. Valida antes de llamar a la red:
     - Una tarea sin proyecto → `UserError{Code:"invalid", Msg:"Elegí un proyecto para la tarea."}`.
     - Título vacío → "La tarea necesita un título.".
     - Prioridad fuera de la lista → inválida.
  2. Crea la tarea (el `WorkspaceID` sale del proyecto) o el ticket.
  3. **Guarda el vínculo con `attach_pending=1` antes de subir.**
  4. Baja el raw. Si pasa el límite, usa `StripAttachments` y agrega a la descripción un `<p>` con "Adjuntos que no se subieron por tamaño:" y la lista de nombres y tamaños, con un `PUT` de la descripción si hace falta (decidilo con el registro).
  5. Ejecuta `Upload` + `SetAttachments` (con la lista existente + la nueva) y deja `attach_pending=0`.
  6. Si falla un paso después de crear, devuelve el vínculo (con `AttachPending=true`) y **no** un error, para que la UI ofrezca reintentar.
  7. Emite `EventTaskappChanged`.
- **`RetryTaskLinkAttachment`.** Repite solo los pasos de subir y adjuntar.
- **`TaskLinksForMessage`.** Busca los vínculos del `threadRoot` del mensaje y completa `URL` y `AlreadyAdded` (por Message-ID). Además, `GetTask` de cada uno, para saber `Completed` y `StateKnown`. Si eso falla, `StateKnown=false` y no es error.
- **`AddMessageToTask`.**
  - Hace `GetTask`. Si está completada → `UserError{Code:"taskapp-completed", Msg:fmt.Sprintf("La tarea #%d está completada.", id)}`.
  - Si no, sube el `.eml` (con el mismo criterio de tamaño) y comenta con `taskDescriptionHTML` y `internal=true`.
  - Termina con `MarkAddedToLink`.
  - Si ya estaba sumado → `UserError{Code:"invalid", Msg:"Ese correo ya está en la tarea."}`.

- [ ] **Step 1: Escribir los tests que fallan.** Usan un servidor falso que registra los pedidos y un mensaje de prueba sembrado como en `helpers_test.go`, con un raw servido por `testenv` IMAP:
  - `TestDraft_SuggestsAndCleans`.
  - `TestDraft_Oversize`: `size` en la base > límite → `Oversize=true`.
  - `TestCreate_TaskWithoutProjectRejectedBeforeNetwork`: el servidor no recibe pedidos.
  - `TestCreate_TicketWithoutProjectOK`.
  - `TestCreate_AttachesEml`: un `upload` y un `PUT` con `attachments:[key]`.
  - `TestCreate_AttachFailsKeepsLink`: el upload da 500 → vuelve un vínculo con `AttachPending=true` y hubo un solo `POST /api/tasks`. Después `RetryTaskLinkAttachment` → `AttachPending=false`, sin otro `POST /api/tasks`.
  - `TestCreate_OversizeStripsAndLists`: la descripción incluye el nombre del adjunto que se dejó afuera.
  - `TestDescriptionHTML_EscapesHeader`: un nombre con `<script>` sale escapado.
  - `TestLinks_ThreadWideAndAlreadyAdded`: un vínculo creado desde el mensaje A aparece en el mensaje B del mismo hilo con `AlreadyAdded=false`. Después de sumar B, `AlreadyAdded=true`.
  - `TestLinks_StateUnknownOffline`.
  - `TestAddMessage_CompletedRejected`.
  - `TestAddMessage_InternalComment`.
  - `TestCreateLocalTaskFrom_LinksSource`.
  - `TestFindMessageByHeader_MissingIsUserError`.
- [ ] **Step 2:** `go test ./core/mail/ -run 'TestDraft_|TestCreate_|TestDescriptionHTML|TestLinks_|TestAddMessage_|TestCreateLocalTaskFrom' -v` → FAIL.
- [ ] **Step 3: Implementar** según las firmas y el orden de arriba.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(taskapp): crear tarea o ticket desde un correo y sumar respuestas`.

---

### Task 9: Servicio para la UI y notificación del aviso

**Files:**
- Modify: `app/mailservice.go`, `app/notify.go`, `app/notifier.go`
- Test: `app/notify_test.go`, `app/mailservice_test.go`

**Interfaces:**
- Consumes: todo lo público de Tasks 6–8.
- Produces:
  - **Métodos de `MailService`:** un envoltorio por método público de core con el mismo nombre: `ListLocalTasks`, `SaveLocalTask`, `SetLocalTaskDone`, `DeleteLocalTask`, `TaskappStatus`, `DisconnectTaskapp`, `TaskappProjects`, `ListTaskappTasks`, `TaskappDraftFor`, `CreateTaskappItem`, `RetryTaskLinkAttachment`, `TaskLinksForMessage`, `AddMessageToTask`, `CreateLocalTaskFrom` y `FindMessageByHeader`.
  - **`ConnectTaskapp(ctx)`:** le pasa a core un `open` que valida con `validateExternalURL` y abre con `s.app.Browser.OpenURL`.
  - **`OpenTaskInTaskapp(ctx, url string) error`:** hace lo mismo que `OpenExternal`, pero solo acepta URL que empiecen con la `TaskappWebURL` configurada.
  - **Evento `app:open-task`** con `{"taskId": int64}`, que se emite al tocar la notificación de un aviso.
  - **`func TaskReminderNotification(r model.TaskReminder, loc *time.Location) notifications.NotificationOptions`:**
    - `ID`: `"task-reminder-<id>"`.
    - `Title`: `"Recordatorio: " + título`.
    - `Body`: escapado, con `"Hoy a las 10:00"` o `"Jue 19/11 a las 10:00"`.
    - `Data`: `{"kind":"task-reminder","taskId":id}`.

  El aviso **se muestra siempre**, aunque la ventana esté a la vista y aunque `DisableNotifications` esté activo, porque ese ajuste es del correo nuevo. `notificationOpensMessage` devuelve false para este `kind`. En la respuesta: `ShowWindow` y emitir `app:open-task`.

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestTaskReminderNotification_Fields`.
  - `TestTaskReminderNotification_EscapesBody`.
  - `TestNotificationOpensMessage_TaskReminderFalse`.
  - `TestOpenTaskInTaskapp_RejectsOtherHosts`.
- [ ] **Step 2:** `go test ./app/ -run 'TestTaskReminder|TestNotificationOpensMessage_TaskReminder|TestOpenTaskInTaskapp' -v` → FAIL.
- [ ] **Step 3: Implementar.** Agregar `taskReminder` al `notifyHub`, como `sendFinished`, y despacharlo en `wailsNotifier.Emit`.
- [ ] **Step 4:** `go test ./app/ -v` → PASS. Después `task common:build:frontend` regenera los bindings: tienen que aparecer `ListLocalTasks`, `CreateTaskappItem` y el resto en `frontend/bindings/postacontrol/app/mailservice.ts`.
- [ ] **Step 5: Commit** `feat(app): servicio de tareas y Taskapp, aviso en el escritorio`.

---

### Task 10: Lógica de la lista de tareas en el frontend

**Files:**
- Create: `frontend/src/lib/tasks.ts`
- Test: `frontend/src/lib/tasks.test.ts`

**Interfaces:**
- Consumes: los tipos generados `LocalTask` y `TaskappTask`.
- Produces:

```ts
export type TaskSource = "all" | "taskapp" | "local";
export type TaskItem =
  | { source: "local"; key: string; due: Date | null; hasTime: boolean; task: LocalTask }
  | { source: "taskapp"; key: string; due: Date | null; hasTime: false; task: TaskappTask };
export type TaskGroup = { id: "overdue" | "today" | "tomorrow" | "week" | "later" | "none"; label: string; items: TaskItem[] };
export function toItems(local: LocalTask[], remote: TaskappTask[], source: TaskSource): TaskItem[];
export function groupByDue(items: TaskItem[], now: Date): TaskGroup[]; // grupos vacíos se omiten
export const REMINDER_OPTIONS: { minutes: number; label: string }[]; // 0 "A la hora", 5, 15, 30, 60 "1 hora antes", 1440 "1 día antes"
```

- Etiquetas: "Vencidas", "Hoy", "Mañana", "Esta semana", "Más adelante", "Sin fecha".
- "Vencida": antes de hoy, o de hoy con hora ya pasada.
- "Esta semana": hasta el domingo de la semana actual.
- Orden dentro del grupo: fecha y hora ascendente, y después título.
- `key`: `"l:<id>"` o `"t:<id>"`.

- [ ] **Step 1: Escribir los tests que fallan**
  - `groups by due`: con `now = 2026-11-18 (miércoles) 12:00` y fechas 17, 18 a las 09:00 (vencida), 18 sin hora (hoy), 19, 22 (esta semana), 23 y sin fecha, cada una cae en su grupo.
  - `filters by source`.
  - `omits empty groups`.
  - `sorts within group`.
- [ ] **Step 2:** `cd frontend && npx vitest run src/lib/tasks.test.ts` → FAIL.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `npx vitest run src/lib/tasks.test.ts` → PASS.
- [ ] **Step 5: Commit** `feat(ui): agrupar tareas por vencimiento`.

---

### Task 11: Vista "Tareas" (lista, detalle y editor de locales)

**Files:**
- Create: `frontend/src/features/tasks/TaskList.tsx`, `TaskPanel.tsx`, `LocalTaskDialog.tsx`
- Modify:
  - `frontend/src/lib/view.ts`: agregar `{ kind: "tasks" }` en `View`, `isView` y `viewIsValid`.
  - `frontend/src/features/layout/MailShell.tsx`: `taskKey: string | null` y `selectTask`, con el mismo patrón que `contactId`; agregar `openTask(id)`, que escucha `app:open-task`.
  - `frontend/src/App.tsx`: ramas `tasks` en `list` y `reader`.
  - `frontend/src/features/folders/FolderSidebar.tsx` (`TasksRow`, debajo de Contactos, ícono `CheckSquareIcon`).
  - `frontend/src/features/folders/SidebarRail.tsx`.
  - `frontend/src/lib/queries.ts`: `useLocalTasks(includeDone)` y `useTaskappTasks()` con `refetchInterval: 5 * 60_000` solo mientras la vista está montada, y `useTaskappStatus()`.
  - `frontend/src/lib/events.ts`: `mail:tasks-changed` → `["local-tasks"]`, y `mail:taskapp-changed` → `["taskapp-tasks"]`, `["taskapp-status"]`, `["task-links"]`.
- Test: `frontend/src/features/tasks/TaskList.test.tsx`, `TaskPanel.test.tsx`, `LocalTaskDialog.test.tsx`, `frontend/src/lib/view.test.ts`

**Interfaces:**
- Consumes: Task 10 y los bindings de Task 9.
- Produces: `ShellState.taskKey`, `ShellState.selectTask(key: string | null)`.

Qué tiene la vista:

- **Encabezado de la lista:** botón "Nueva" y un `Select` Todas/Taskapp/Locales, recordado en `localStorage` con try/catch.
- **Grupos de `groupByDue`:**
  - Cada fila tiene un círculo para marcar hecha (solo en locales), el título, `#id` en las de Taskapp y una etiqueta de origen (el proyecto con su color, o "Local").
- **Interruptor "Mostrar hechas"** al pie.
- **Taskapp sin conectar:** no se piden tareas remotas, y arriba aparece una línea "Conectá Taskapp para ver tus tareas" que abre Ajustes en la página Taskapp.
- **Lista de Taskapp desactualizada (`Stale`):** una línea "Taskapp sin conexión · actualizado hace X".
- **Error sin caché:** una línea con el error. Las locales se ven igual.
- **Detalle de una local:** título, fecha y hora, aviso, notas, un enlace "Correo de origen" (llama a `FindMessageByHeader` y abre ese correo en su carpeta, igual que `app:open-message`; si no está, muestra el error del núcleo) y los botones Hecha, Editar y Borrar (con confirmación).
- **Detalle de Taskapp:** título, proyecto, estado, prioridad, vencimiento, descripción (HTML sanitizado en `SafeHtmlFrame`) y "Abrir en Taskapp".
- **`LocalTaskDialog`:**
  - Campos: Título, Notas, Fecha (opcional), Hora (opcional, deshabilitada sin fecha) y Aviso (deshabilitado sin hora). Aviso usa `REMINDER_OPTIONS` más "Predeterminado (N min)", que guarda `remindBefore = null`.
  - Los errores del núcleo se muestran con `toast.error(errorMessage(e))`.

- [ ] **Step 1: Escribir los tests que fallan**
  - `view.test.ts`: `{kind:"tasks"}` es válido y se recuerda.
  - `TaskList`: muestra los grupos y el origen. "Taskapp sin conexión" aparece cuando `stale`. Sin conexión a Taskapp muestra el aviso de conectar y no llama a `ListTaskappTasks`. Marcar el círculo llama a `SetLocalTaskDone(id, true)`.
  - `TaskPanel`: una de Taskapp muestra "Abrir en Taskapp" y no muestra Editar.
  - `LocalTaskDialog`: la hora está deshabilitada sin fecha, y al guardar manda `remindBefore: null` con "Predeterminado".
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/tasks src/lib/view.test.ts` → FAIL.
- [ ] **Step 3: Implementar.** Seguí `ContactList` y `ContactPanel` para el layout y los estados vacíos ("No tenés tareas pendientes.").
- [ ] **Step 4:** `npx vitest run src/features/tasks src/lib/view.test.ts && npx tsc --noEmit` → PASS.
- [ ] **Step 5: Commit** `feat(ui): vista de Tareas con locales y pendientes de Taskapp`.

---

### Task 12: Crear tarea desde el lector y chips del hilo

**Files:**
- Create: `frontend/src/features/tasks/CreateTaskDialog.tsx`, `frontend/src/features/tasks/TaskChips.tsx`
- Modify:
  - `frontend/src/features/reader/MessageView.tsx`: botón "Crear tarea" (ícono `ListPlusIcon`) después de Reenviar, y `<TaskChips messageId>` debajo del encabezado del correo.
  - `frontend/src/features/layout/useMailKeys.ts`: tecla `t` con un correo abierto.
  - `frontend/src/features/layout/ShortcutsDialog.tsx`: `t` → "Crear tarea".
  - `frontend/src/lib/queries.ts`: `useTaskLinks(messageId)`, con clave `["task-links", messageId]`.
- Test: `frontend/src/features/tasks/CreateTaskDialog.test.tsx`, `TaskChips.test.tsx`, `frontend/src/features/layout/useMailKeys.test.tsx`

**Interfaces:**
- Consumes: `TaskappDraftFor`, `CreateTaskappItem`, `RetryTaskLinkAttachment`, `TaskLinksForMessage`, `AddMessageToTask`, `CreateLocalTaskFrom`, `TaskappProjects`, `TaskappStatus` y `LocalTaskDialog` (Task 11).

**El diálogo, como en el sketch del spec:**

- **Destino** en tres opciones: "Tarea en Taskapp" (marcada por defecto si hay conexión), "Ticket en Taskapp" y "Tarea local" (la única si no hay conexión, con un enlace "Conectar Taskapp").
- **Opción "Tarea local":** el diálogo pasa a mostrar `LocalTaskDialog` precargado con `CreateLocalTaskFrom`.
- **Formulario de Taskapp:**
  - Proyecto: `Select` precargado con `draft.projectId` (0 = vacío, placeholder "Elegí un proyecto"), con el color de cada proyecto.
  - Título.
  - Descripción: un `textarea` con el HTML del borrador. Editar HTML a mano es aceptable para esta versión; si `RichEditor` se reutiliza fácil, mejor.
  - Prioridad.
  - Vence: solo para Tarea.
  - Una nota fija "📎 Se adjunta el correo original (.eml)". Con `oversize`, en su lugar: "El correo pesa más de 20 MB: se adjunta sin sus archivos y la descripción los lista."
- **Botón Crear:** deshabilitado con Tarea y sin proyecto, con el texto de ayuda "Elegí un proyecto para la tarea.".
- **Resultado:**
  - Si sale bien: `toast.success("Tarea #N creada en <proyecto>")` con la acción "Abrir".
  - Si vuelve con `attachPending`: `toast.warning("La tarea #N se creó, pero no se pudo adjuntar el correo.")` con la acción "Reintentar" (`RetryTaskLinkAttachment`).

**`TaskChips`, un chip por vínculo:**

- El chip muestra "Tarea #123 · Sinergia" (o "Ticket #…"). Al tocarlo abre `OpenTaskInTaskapp`.
- Con `attachPending`: un ícono de alerta y el menú "Reintentar adjunto".
- Si `!alreadyAdded`, un botón "Sumar a la tarea #123":
  - Con `completed`, deshabilitado y con tooltip "La tarea #123 está completada", más un botón "Crear tarea nueva" que abre el diálogo.
  - Con `!stateKnown`, habilitado (el núcleo valida).
  - Si hay más de un vínculo en el hilo, "Sumar a…" abre un menú para elegir.

- [ ] **Step 1: Escribir los tests que fallan**
  - `CreateTaskDialog`:
    - Precarga el título y el proyecto del borrador.
    - Crear está deshabilitado con Tarea y sin proyecto, y habilitado con Ticket y sin proyecto.
    - "Vence" no está con Ticket.
    - `attachPending` muestra el toast de reintento.
    - Sin conexión solo ofrece "Tarea local".
  - `TaskChips`:
    - Muestra el chip y "Sumar".
    - "Sumar" está deshabilitado con `completed` y ofrece "Crear tarea nueva".
    - Después de sumar, se invalida `["task-links"]`.
  - `useMailKeys`: `t` con un mensaje abierto llama a `onCreateTask`, y sin mensaje no hace nada.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/tasks src/features/layout/useMailKeys.test.tsx` → FAIL.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `npx vitest run src/features/tasks src/features/layout && npx tsc --noEmit` → PASS.
- [ ] **Step 5: Commit** `feat(ui): crear tarea desde el correo y sumar respuestas del hilo`.

---

### Task 13: Ajustes → Taskapp y aviso de reconexión

**Files:**
- Create: `frontend/src/features/settings/TaskappSection.tsx`
- Modify:
  - `frontend/src/features/settings/SettingsDialog.tsx`: página `{ id: "taskapp", label: "Taskapp", icon: ListChecksIcon }` antes de Respaldo.
  - `frontend/src/features/settings/SettingsPages.tsx`: en General, un `Select` "Aviso de tareas por defecto" con `REMINDER_OPTIONS`.
  - `frontend/src/features/settings/BackupSection.tsx`: suma "tareas" al resumen de importación.
  - `frontend/src/lib/events.ts`: `mail:taskapp-auth` → un `toast` fijo "Taskapp rechazó la conexión." con la acción "Reconectar", que llama a `ConnectTaskapp`.
- Test: `frontend/src/features/settings/TaskappSection.test.tsx`

**Interfaces:**
- Consumes: `TaskappStatus`, `ConnectTaskapp`, `DisconnectTaskapp` y los campos `taskappApiUrl`, `taskappWebUrl` y `taskReminderMinutes` de `Settings`.

**Qué tiene la página:**

- **Desconectado:** el texto "Conectá tu cuenta de Taskapp para crear tareas desde el correo y ver tus pendientes.", un botón "Conectar con Taskapp" y una nota "Se abre el navegador para que autorices la conexión. La página dice «Autorizar TaskApp CLI»: es el mismo permiso.".
- **Mientras espera:** "Esperando la autorización en el navegador…" con un botón Cancelar, que solo cierra el estado local; core vence a los 5 min.
- **Conectado:** "Conectado como <nombre> (<email>)" y un botón Desconectar, con confirmación.
- **Avanzado** (plegado): URL de la API y URL de Taskapp, vacías = por defecto.

- [ ] **Step 1: Escribir los tests que fallan**
  - Desconectado: muestra Conectar y, al tocarlo, llama a `ConnectTaskapp` y muestra "Esperando…".
  - Conectado: muestra el nombre.
  - Desconectar pide confirmación y llama a `DisconnectTaskapp`.
  - El select de aviso guarda `taskReminderMinutes`.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/settings/TaskappSection.test.tsx` → FAIL.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `npx vitest run src/features/settings && npx tsc --noEmit` → PASS.
- [ ] **Step 5: Commit** `feat(ui): conectar Taskapp desde Configuración`.

---

### Task 14: Verificación completa, documentación y prueba de aceptación

**Files:**
- Modify:
  - `README.md`: en Funciones, una sección **Tareas** con tareas locales con aviso, crear en Taskapp desde un correo, sumar respuestas y ver los pendientes. En la tabla de atajos, `t`.
  - `docs/superpowers/plans/2026-11-16-tareas-taskapp-registro.md`: el resultado de la prueba de aceptación.
- [ ] **Step 1:** `task check` → todo verde: vet, tests de Go (incluido `arch_test.go`), tsc, vitest y eslint.
- [ ] **Step 2:** Agregar la nota de versión, donde el proyecto lleva las notas de release: "Esta versión actualiza la base de datos local (v17); una versión anterior ya no puede abrirla."
- [ ] **Step 3: Prueba manual sin escribir en Taskapp.** Lanzar la app (skill `run`) y comprobar:
  - Crear, editar y marcar una tarea local.
  - Recibir un aviso con una tarea a 1 minuto y anticipación "A la hora", con la ventana cerrada en la bandeja.
  - Que el clic en el aviso abra la tarea.
  - Conectar Taskapp y ver los pendientes.
  - Que el diálogo de crear precargue bien desde un correo de un aprobador conocido.
  - Cancelar sin crear.
- [ ] **Step 4: Aceptación con Fabricio.** Él crea la primera tarea real desde la app y suma una respuesta. Anotar el resultado en el registro.
- [ ] **Step 5: Commit** `docs: tareas y Taskapp en el README y notas de versión`.

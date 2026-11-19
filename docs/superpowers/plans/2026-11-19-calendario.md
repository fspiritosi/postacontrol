# Calendario — plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** PostaControl pasa a ser el calendario de Fabricio. Tiene eventos locales con repetición, avisos e invitaciones por correo (iTIP/iMIP de ida y vuelta), muestra las tareas con fecha y las reuniones de Taskapp, permite crear reuniones con invitados externos e importa una vez el calendario de Google.

**Architecture:**
- **`core/icalx`:** paquete nuevo y puro. Lee y arma iCalendar con `emersion/go-ical` y expande repeticiones con `teambition/rrule-go`.
- **`core/store`:** suma las tablas del calendario (migración v18).
- **`core/mail`:**
  - Eventos, repeticiones y avisos.
  - Invitaciones salientes por la bandeja de salida. El `SendRequest` lleva la parte `text/calendar`.
  - Invitaciones entrantes, detectadas al bajar el cuerpo de un correo (abrirlo o el indexador).
  - Reuniones de Taskapp, con caché y espejo para los externos.
  - Importación de `.ics` y `.zip`.
- **`app/`:** expone todo con envoltorios finos y la notificación del aviso.
- **Frontend:** la vista Calendario usa FullCalendar en el panel principal entero, con el detalle a la derecha.

**Tech Stack:** Go, `github.com/emersion/go-ical`, `github.com/teambition/rrule-go`, SQLite, Wails v3, React + TS + TanStack Query + shadcn/ui, FullCalendar 6 (MIT), Vitest.

**Spec:** `docs/superpowers/specs/2026-11-18-calendario-design.md`

## Global Constraints

- `core/...` no importa Wails (`arch_test.go`).
- La base pasa a la versión **18** (`core/store/schema_v18.sql`). Las notas de la versión avisan que una versión anterior ya no la abre.
- Contra Taskapp de producción **solo lecturas** durante el desarrollo. Las escrituras (`POST /api/meetings`) se prueban contra `httptest.Server`. El token no se imprime ni se guarda fuera del llavero.
- Dependencias Go nuevas:
  - `github.com/emersion/go-ical v0.0.0-20261112170022-46c5b53a1723`.
  - `github.com/teambition/rrule-go v1.8.2`, que ya es dependencia de go-ical.
- Dependencias npm nuevas, **versión exacta `6.1.22`** (no la 7):
  - Paquetes: `@fullcalendar/react`, `@fullcalendar/core`, `@fullcalendar/daygrid`, `@fullcalendar/timegrid`, `@fullcalendar/list` y `@fullcalendar/interaction`.
  - Instalar con `npx -y npm@11 install --save-exact …`.
  - `git diff frontend/package-lock.json` no puede borrar ninguna línea `"libc"`. Si las borra, frenar y avisar.
  - Fuera de esos paquetes, el lock no se commitea.
- `PRODID:-//PostaControl//PostaControl//ES`, `VERSION:2.0`, `CALSCALE:GREGORIAN`.
- Horas en los `.ics` que arma PostaControl:
  - Con hora: en UTC (`…Z`), sin `VTIMEZONE`.
  - Día completo: `VALUE=DATE`.
  - Una respuesta (REPLY) copia las horas y los `VTIMEZONE` del original.
- Correo iMIP:
  ```
  multipart/mixed
    multipart/alternative
      text/plain; charset=UTF-8
      text/html; charset=UTF-8
      text/calendar; charset=UTF-8; method=<M>   (base64)
    application/ics; name="invite.ics"          (base64, attachment)
  ```
- Asuntos de los correos de invitación:
  - `Invitación: <título> — <cuándo>`
  - `Invitación actualizada: <título> — <cuándo>`
  - `Cancelado: <título> — <cuándo>`
  - `Aceptado: <título>`, `Tal vez: <título>` y `Rechazado: <título>`
- PARTSTAT válidos para responder: `ACCEPTED`, `TENTATIVE` y `DECLINED`. El estado inicial es `NEEDS-ACTION`.
- Avisos:
  - Anticipaciones permitidas: `0, 5, 15, 30, 60, 1440` minutos. Es la misma lista que las tareas (`remindMinutes`).
  - Valor por defecto para eventos: **15**.
  - Un aviso con más de 24 h de atraso no se muestra (`reminderStale`).
- Eventos de día completo:
  - Se guardan como medianoche **UTC** de la fecha, con fin exclusivo, y se muestran como fechas sin convertir.
  - Avisan a las 09:00 locales del día. Con anticipación de 1440, a las 09:00 del día anterior.
- Primera vez: la migración vuelve a bajar los cuerpos de la Entrada de los últimos **60 días** (`body_cached = 0`), así el indexador detecta las invitaciones viejas.
- Importación: `.ics` y `.zip`, hasta **50 MB** por archivo y por entrada del zip, y hasta **50 entradas** por zip.
- Un `.ics` recibido se lee hasta **1 MB**. Uno más grande no se procesa.
- Verificación completa: `task check`.

## Review Focus

1. **Invitación de Outlook con TZID de Windows o con hora flotante.** Por ejemplo `TZID=Argentina Standard Time` con su `VTIMEZONE`, o un `DTSTART` sin zona. Tiene que quedar a la hora local correcta y no desplazada 3 h. Test en Task 1.
2. **La misma invitación llega dos veces, o llega una versión vieja después de una nueva.** Pasa con la misma invitación en dos carpetas, con un correo reabierto o con una SEQUENCE menor que llega tarde. Nunca se crean dos eventos y una versión vieja no pisa a la nueva. Test en Task 7.
3. **Invitación a un alias o a una lista**, o con el ATTENDEE en otro caso (`MAILTO:Juan@X.com`):
   - El ATTENDEE se compara sin distinguir mayúsculas.
   - Si no coincide con ninguna cuenta, responde la cuenta que recibió el correo y la tarjeta avisa.
   - Test en Task 7.
4. **Serie con una ocurrencia movida y otra borrada, editada después con "este y los siguientes" desde una ocurrencia posterior.**
   - Las excepciones anteriores al corte se conservan.
   - La serie se corta en la ocurrencia elegida.
   - Los invitados reciben una sola actualización de la serie vieja y una invitación a la nueva.
   - Test en Task 5 y Task 6.
5. **Taskapp no responde en un refresco y hay un espejo con externos.** No se marca "se borró en Taskapp". Solo se marca cuando una lectura exitosa que cubre la hora del espejo no trae la reunión. Test en Task 10.

---

### Task 0: Verificar la API de reuniones y juntar muestras

**Files:**
- Create: `docs/superpowers/plans/2026-11-19-calendario-registro.md`
- Create: `core/icalx/testdata/`:
  - `google-request.ics`, `google-update.ics`, `google-cancel.ics`
  - `outlook-request.ics`, con TZID de Windows y `VTIMEZONE`
  - `reply-accepted.ics`
  - `google-export.ics`, con `X-WR-CALNAME`, un evento repetido con excepción, uno de día completo, uno organizado por otro y uno propio
  - `google-request.eml` y `outlook-request.eml`

**Interfaces:**
- Produces:
  - El registro con, para cada operación de reuniones, método, ruta, cuerpo y respuesta reales.
  - La ruta web de una reunión.
  - Las muestras que usan Task 1, 4 y 7.

- [ ] **Step 1: Leer el backend.** En `~/desarrollo_local/taskapp_web/taskApp-backend`, mirá `internal/handler/meeting.go`, `internal/service/meeting.go`, `internal/repository/meeting.go` y `internal/model/meeting.go`, y anotá:
  - Si `GET /api/meetings?workspace_id&from&to` filtra por superposición o por `start_time`.
  - Si el creador queda como asistente.
  - Los valores de `rsvp_status` y de `status` (`scheduled`, `completed`, `cancelled`).
  - La forma de `GET /api/workspaces` y de `GET /api/workspaces/{id}/members`.
  - Si `POST` devuelve la reunión con `attendees`.
- [ ] **Step 2: Leer producción (solo GET).** Con el token de la conexión ya hecha, o con el del MCP `cc-tickets` en `~/.claude.json` sin imprimirlo, pedí:
  - `GET /api/workspaces`.
  - `GET /api/workspaces/{id}/members`.
  - `GET /api/meetings?workspace_id=<id>&from=<RFC3339>&to=<RFC3339>`, con un rango de un mes.
  - Compará contra el código. **No escribas nada.**
- [ ] **Step 3: Ruta web de una reunión.** Buscala en el frontend de Taskapp (`~/desarrollo_local/taskapp_web`) o abriendo `https://taskapp.codecontrol.com.ar` y anotala. Por ejemplo `/meetings/:id` o `/agenda?meeting=:id`.
- [ ] **Step 4: Muestras de invitaciones.**
  - Pedile al controlador que le pida a Fabricio invitaciones reales guardadas como `.eml` (Ver código fuente → Guardar), una de Google y una de Outlook, y el ZIP de exportación de Google Calendar. Con su permiso, el MCP de Gmail sirve para bajar una invitación de Google.
  - Redactá las direcciones a `organizador@example.com`, `fabricio@example.com` y similares, sin cambiar la estructura.
  - Si no hay muestra real de algún tipo, escribila a mano siguiendo RFC 5546/6047, con `X-SAMPLE: synthetic` como comentario en la primera línea.
- [ ] **Step 5: Escribir el registro.**
  - Una tabla por operación, con el JSON real redactado de una reunión, de un workspace y de un miembro.
  - La ruta web.
  - Qué muestras son reales y cuáles sintéticas.
- [ ] **Step 6: Commit** `docs(calendario): registro de la API de reuniones y muestras iCalendar`.

---

### Task 1: `core/icalx` — leer iCalendar

**Files:**
- Create: `core/icalx/parse.go`, `core/icalx/tz.go`
- Test: `core/icalx/parse_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: las muestras de Task 0.
- Produces:

```go
type Attendee struct{ Email, Name, Partstat string } // Email en minúscula, sin "mailto:"
type VEvent struct {
	UID          string
	Sequence     int
	Status       string    // "CONFIRMED", "TENTATIVE" o "CANCELLED"; "" = CONFIRMED
	Summary, Description, Location string
	Start, End   time.Time // con hora: en UTC; día completo: medianoche UTC, fin exclusivo
	AllDay       bool
	TZID         string    // IANA de la zona del original; "" si era UTC o flotante
	RRule        string    // sin "RRULE:"; "" = no se repite
	ExDates      []time.Time
	RecurrenceID time.Time // cero en el maestro
	Organizer    model.Address
	Attendees    []Attendee
}
type Series struct{ Master VEvent; Overrides []VEvent } // Overrides por RECURRENCE-ID
type Calendar struct {
	Method  string   // "REQUEST", "REPLY", "CANCEL", "PUBLISH" o ""
	Name    string   // X-WR-CALNAME
	Series  []Series // agrupado por UID, en el orden del archivo
	Raw     []byte   // el archivo tal cual
}
func Parse(data []byte) (Calendar, error)
func ParseInLocation(data []byte, floating *time.Location) (Calendar, error)
```

- `Parse` usa `time.Local` para las horas flotantes.
- Sin `DTEND`, el fin sale de `DURATION`. Si tampoco hay, es el inicio más 1 día (día completo) o el inicio (con hora).
- Una serie con solo excepciones y sin maestro usa la primera excepción como `Master`.

- [ ] **Step 1: Agregar las dependencias** con `go get github.com/emersion/go-ical@v0.0.0-20261112170022-46c5b53a1723`. Verificá que `go.mod` quede con `rrule-go v1.8.2`.
- [ ] **Step 2: Escribir los tests que fallan**
  - `TestParse_GoogleRequest`: `Method=="REQUEST"`, una serie, UID y SEQUENCE de la muestra, organizador con nombre y correo, ATTENDEE de `fabricio@example.com` con `NEEDS-ACTION`, y `Start` igual a la hora UTC de la muestra.
  - `TestParse_OutlookWindowsTZID`: `TZID=Argentina Standard Time` a las 10:00 → `Start == 13:00Z`, y `TZID=="America/Argentina/Buenos_Aires"`.
  - `TestParse_UnknownTZIDUsesVTimezoneOffset`: un TZID inventado con `VTIMEZONE` `TZOFFSETTO:-0300` → `Start` correcto a −03:00.
  - `TestParse_Floating`: un `DTSTART:20261120T100000` sin zona, con `floating=America/Argentina/Buenos_Aires` → `13:00Z`.
  - `TestParse_AllDay`: `DTSTART;VALUE=DATE:20261120` → `Start == 2026-11-20T00:00Z`, `End == 2026-11-21T00:00Z` y `AllDay`.
  - `TestParse_RecurringWithOverride`: la exportación tiene la serie semanal con `EXDATE` y un override con `RECURRENCE-ID` → `RRule` sin el prefijo, un `ExDates` y un override cuyo `RecurrenceID` es la ocurrencia original.
  - `TestParse_Cancel`: `Method=="CANCEL"` y `Status=="CANCELLED"`.
  - `TestParse_AttendeeCase`: `ATTENDEE:MAILTO:Juan@X.com` → `Email=="juan@x.com"`.
  - `TestParse_ExportName`: `Name` es el `X-WR-CALNAME`, y hay varias series.
  - `TestParse_Garbage`: con `[]byte("hola")` devuelve un error.
- [ ] **Step 3:** `go test ./core/icalx/ -v` → FAIL.
- [ ] **Step 4: Implementar.**
  - `tz.go` tiene `func resolveTZ(tzid string, vtz map[string]*ical.Component) *time.Location`, que prueba en orden:
    1. `time.LoadLocation(tzid)`.
    2. La tabla Windows→IANA, con al menos estas zonas:
       - `Argentina Standard Time`
       - `SA Eastern Standard Time`
       - `E. South America Standard Time`
       - `Pacific SA Standard Time`
       - `SA Pacific Standard Time`
       - `Central Standard Time (Mexico)`
       - `Eastern Standard Time`
       - `Central Standard Time`
       - `Pacific Standard Time`
       - `Mountain Standard Time`
       - `GMT Standard Time`
       - `W. Europe Standard Time`
       - `Romance Standard Time`
       - `UTC`
    3. Un `time.FixedZone` con el `TZOFFSETTO` del `STANDARD` del `VTIMEZONE`.
    4. UTC.
  - Las horas se leen con `Props.Get(...)` y el `TZID` del parámetro, para no depender de que `DateTime` resuelva zonas de Windows.
- [ ] **Step 5:** `go test ./core/icalx/ -v` → PASS.
- [ ] **Step 6: Commit** `feat(icalx): leer invitaciones y calendarios iCalendar`.

---

### Task 2: `core/icalx` — armar iCalendar y expandir repeticiones

**Files:**
- Create: `core/icalx/build.go`, `core/icalx/expand.go`
- Test: `core/icalx/build_test.go`, `core/icalx/expand_test.go`

**Interfaces:**
- Consumes: `VEvent`, `Series` y `Calendar` de Task 1.
- Produces:

```go
const ProdID = "-//PostaControl//PostaControl//ES"
func Build(method string, s Series, now time.Time) ([]byte, error)            // REQUEST, CANCEL o PUBLISH
func BuildReply(orig Calendar, uid, attendee, partstat string, now time.Time) ([]byte, error)
func Expand(start, end time.Time, allDay bool, tzid, rrule string, exdates []time.Time, from, to time.Time) []time.Time
```

- **`Build`:**
  - Escribe `METHOD`, `DTSTAMP=now` y `SEQUENCE`. Escribe `STATUS:CANCELLED` en CANCEL.
  - Escribe `ORGANIZER;CN=…:mailto:` y cada `ATTENDEE;CN=…;ROLE=REQ-PARTICIPANT;PARTSTAT=…;RSVP=TRUE:mailto:`.
  - Escribe `RRULE`, `EXDATE` y un VEVENT por override, con `RECURRENCE-ID`.
  - Pone las horas en UTC, o `VALUE=DATE` en día completo.
- **`BuildReply`:**
  - Parte del VEVENT maestro de `uid` en `orig.Raw`.
  - Saca todos los `ATTENDEE` y agrega solo el de `attendee` con `partstat`.
  - Saca los `VALARM` y la `DESCRIPTION`, y pone `DTSTAMP=now`.
  - Conserva los `VTIMEZONE` del calendario y pone `METHOD:REPLY`.
- **`Expand`:**
  - Devuelve los inicios de las ocurrencias que se superponen con `[from, to)`.
  - Expande en la zona `tzid`: `time.Local` si está vacía o no carga, y UTC si es día completo.
  - Sin `rrule`, devuelve solo `start` si se superpone.
  - Corta en **500** ocurrencias.

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestBuild_RequestRoundTrip`: armar un REQUEST con 2 invitados y volver a leerlo con `Parse` da el mismo UID, SEQUENCE, organizador, invitados, horas y `Method`.
  - `TestBuild_Cancel`: un CANCEL tiene `STATUS:CANCELLED` y `SEQUENCE` igual a la de la serie.
  - `TestBuild_AllDayValueDate`: el texto tiene `DTSTART;VALUE=DATE:20261120`.
  - `TestBuild_OverrideAndExdate`: una serie con un override y un EXDATE da 2 VEVENT, y el segundo tiene `RECURRENCE-ID`.
  - `TestBuild_LinesFolded`: ninguna línea pasa de 75 octetos y todas terminan en CRLF.
  - `TestBuildReply_OnlyMyAttendee`: desde `google-request.ics`, el REPLY tiene `METHOD:REPLY`, un solo `ATTENDEE` con `PARTSTAT=ACCEPTED`, el mismo UID, SEQUENCE y ORGANIZER, y no tiene VALARM.
  - `TestBuildReply_KeepsVTimezone`: desde `outlook-request.ics`, el REPLY conserva el `VTIMEZONE` y el `TZID` del `DTSTART`.
  - `TestExpand_WeeklyExdate`: con `FREQ=WEEKLY;BYDAY=MO,WE`, 4 semanas y un EXDATE quedan 7 ocurrencias.
  - `TestExpand_UntilAndCount`: `UNTIL` y `COUNT` cortan donde corresponde.
  - `TestExpand_OverlapNotJustStart`: un evento de 23:00 a 01:00 aparece en un rango que empieza a las 00:00.
  - `TestExpand_DSTKeepsWallTime`: en `America/New_York`, un evento semanal a las 09:00 sigue a las 09:00 locales después del cambio de hora.
  - `TestExpand_Cap500`: con `FREQ=DAILY` sin fin, en un rango de 10 años, quedan 500 ocurrencias.
- [ ] **Step 2:** `go test ./core/icalx/ -v` → FAIL.
- [ ] **Step 3: Implementar.**
  - `Build` y `BuildReply` usan `ical.NewEncoder`.
  - `Expand` usa `rrule.StrToROptionInLocation`, le pone `Dtstart=start.In(loc)`, arma un `rrule.Set` con los EXDATE y pide `Between(from-dur, to, true)`, donde `dur = end-start`.
- [ ] **Step 4:** `go test ./core/icalx/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(icalx): armar invitaciones y expandir repeticiones`.

---

### Task 3: Esquema v18 y almacenamiento de eventos

**Files:**
- Create: `core/store/schema_v18.sql`, `core/store/events.go`
- Modify: `core/store/migrate.go` (embed y lista), `core/store/tasks.go` (`ClearTaskappData` también borra `taskapp_meetings`), `core/model/model.go`
- Test: `core/store/events_test.go`, y `core/store/store_test.go` (versión 18)

**Interfaces:**
- Produces, en `core/model`:

```go
type Attendee struct {
	Email    string `json:"email"` // minúscula
	Name     string `json:"name"`
	Partstat string `json:"partstat"` // NEEDS-ACTION, ACCEPTED, TENTATIVE o DECLINED
}
type EventException struct {
	RecurrenceAt time.Time `json:"recurrenceAt"` // inicio original de la ocurrencia
	Cancelled    bool      `json:"cancelled"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	Title        string    `json:"title"`
	Location     string    `json:"location"`
	Description  string    `json:"description"`
}
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
	Status           string           `json:"status"` // "confirmed" o "cancelled"
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
```

- Produces, en `core/store`:

```go
func (s *Store) SaveEvent(ctx context.Context, e model.CalendarEvent, seriesEnd *time.Time) (model.CalendarEvent, error) // ID 0 = alta; reemplaza invitados y excepciones
func (s *Store) GetEvent(ctx context.Context, id int64) (model.CalendarEvent, error)      // ErrNotFound
func (s *Store) EventByUID(ctx context.Context, uid string) (model.CalendarEvent, error)  // ErrNotFound
func (s *Store) EventByMeeting(ctx context.Context, meetingID int64) (model.CalendarEvent, error)
func (s *Store) EventsInRange(ctx context.Context, from, to time.Time) ([]model.CalendarEvent, error)
func (s *Store) AllEvents(ctx context.Context) ([]model.CalendarEvent, error)
func (s *Store) DeleteEvent(ctx context.Context, id int64) error
func (s *Store) SetAttendeePartstat(ctx context.Context, eventID int64, email, partstat string) (bool, error)
func (s *Store) TaskappSnapshot(ctx context.Context, eventID int64) (string, error)
func (s *Store) SetTaskappState(ctx context.Context, eventID int64, snapshot, change string) error
func (s *Store) ClaimEventReminder(ctx context.Context, key string, occurrence, at time.Time) (bool, error)

type InviteRecord struct{ MessageID int64; Method, UID string; Sequence int; Sender, ICS, Outcome string }
func (s *Store) InviteRecord(ctx context.Context, messageID int64) (InviteRecord, bool, error)
func (s *Store) InviteSeen(ctx context.Context, uid string, sequence int, sender, method string) (bool, error)
func (s *Store) SaveInviteRecord(ctx context.Context, r InviteRecord) error // INSERT OR REPLACE

type StoredMeeting struct{ ID, WorkspaceID int64; Start, End time.Time; JSON string }
func (s *Store) ReplaceMeetings(ctx context.Context, workspaceID int64, from, to time.Time, ms []StoredMeeting, at time.Time) error
func (s *Store) MeetingsInRange(ctx context.Context, from, to time.Time) (ms []StoredMeeting, fetchedAt time.Time, err error)
```

Esquema `schema_v18.sql`:
- **`events`:**
  - Una columna por campo de `CalendarEvent`, en snake_case.
  - `uid` es `UNIQUE` y `taskapp_meeting_id` es `UNIQUE` (NULL si no hay).
  - `account_id` referencia `accounts(id)` con `ON DELETE SET NULL`.
  - `series_end_at INTEGER`: el fin de la última ocurrencia, o NULL si la serie no tiene fin.
  - `kind` tiene un `CHECK (kind IN ('local','invite','taskapp'))`.
  - Lleva `created_at` y `updated_at`.
  - Índice sobre `start_at`.
- **`event_attendees`:** `(event_id FK CASCADE, email, name, partstat)`, con `PK(event_id, email)`.
- **`event_exceptions`:** `(event_id FK CASCADE, recurrence_at, cancelled, start_at, end_at, title, location, description)`, con `PK(event_id, recurrence_at)`.
- **`event_reminders`:** `(event_key TEXT, occurrence_at INTEGER, reminded_at INTEGER)`, con `PK(event_key, occurrence_at)`.
- **`invite_messages`:** `(message_id INTEGER PRIMARY KEY REFERENCES messages(id) ON DELETE CASCADE, method, uid, sequence, sender, ics, outcome, processed_at)`, con índice sobre `(uid, sequence, sender, method)`.
- **`taskapp_meetings`:** `(id INTEGER PRIMARY KEY, workspace_id, start_at, end_at, json, fetched_at)`, con índice sobre `start_at`.
- Al final:
  ```sql
  UPDATE messages SET body_cached = 0
   WHERE body_cached = 1
     AND date >= CAST(strftime('%s','now','-60 days') AS INTEGER)
     AND folder_id IN (SELECT id FROM folders WHERE role = 'inbox');
  ```

Más:
- **`EventsInRange`** devuelve:
  - Los que no se repiten (`rrule=''`) y se superponen con `[from, to)`.
  - Los que se repiten con `start_at < to` y `(series_end_at IS NULL OR series_end_at > from)`.
  - Van con sus invitados y excepciones.
- **`ReplaceMeetings`** borra las reuniones del workspace cuyo `start_at` cae en `[from, to)` e inserta `ms`, en una transacción.

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestMigrate_V18`: la base nueva queda en `user_version 18`. Al migrar una v17 con un correo de la Entrada de hace 10 días y otro de hace 90, los dos con cuerpo cacheado, solo el de hace 10 días queda con `body_cached = 0`. Un correo de Enviados de hace 10 días no cambia.
  - `TestSaveEvent_RoundTrip`: con invitados y excepciones, `GetEvent` devuelve los mismos campos y los invitados en minúscula.
  - `TestSaveEvent_UIDUnique`: dos altas con el mismo UID → la segunda falla.
  - `TestEventsInRange_SingleAndSeries`: entran uno que cruza el borde del rango y una serie que empezó antes sin fin. No entran una serie con `series_end_at` antes del rango ni uno después del rango.
  - `TestClaimEventReminder_Once`: el primer reclamo devuelve true y el segundo false.
  - `TestInviteSeen`: un registro con `(uid, 3, a@x, REQUEST)` → `InviteSeen(uid, 3, a@x, REQUEST)` da true y con seq 4 da false.
  - `TestInviteRecord_CascadeOnMessageDelete`: al borrar el mensaje se borra el registro.
  - `TestReplaceMeetings_RangeOnly`: reemplazar el rango de noviembre no toca las de diciembre del mismo workspace ni las de otro workspace.
  - `TestClearTaskappData_Meetings`: borra `taskapp_meetings` y conserva los eventos.
  - `TestDeleteAccount_KeepsEvents`: el `account_id` queda en 0.
- [ ] **Step 2:** `go test ./core/store/ -run 'Event|Invite|Meeting|Migrate_V18|ClearTaskapp' -v` → FAIL.
- [ ] **Step 3: Implementar.** Seguí `core/store/tasks.go`: columnas en una constante, `scanX(rowScanner)` y segundos unix.
- [ ] **Step 4:** `go test ./core/store/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(store): tablas del calendario (v18)`.

---

### Task 4: Parte `text/calendar` al enviar y al bajar un correo

**Files:**
- Modify:
  - `core/mimex/build.go`: `Outgoing.Calendar` y `buildWithCalendar`.
  - `core/mimex/plan.go`: `PartPlan.Calendar`.
  - `core/model/model.go`: `SendRequest.Calendar`.
  - `core/mail/outgoing.go`: `buildOutgoing` pasa la parte.
  - `core/mail/read.go`: `fetchBody` llena `BodyRow.Calendar`.
  - `core/store/messages.go`: `BodyRow.Calendar`, que no se persiste.
- Test: `core/mimex/build_test.go`, `core/mimex/read_test.go`, `core/mail/compose_test.go`, `core/mail/read_test.go`

**Interfaces:**
- Produces:

```go
// mimex
type OutCalendar struct{ Method string; Data []byte }
// Outgoing gana: Calendar *OutCalendar
// PartPlan gana: Calendar []int // primera parte text/calendar que no es adjunto; nil si no hay

// model
type CalendarPart struct {
	Method string `json:"method"`
	ICS    string `json:"ics"`
}
// SendRequest gana: Calendar *CalendarPart `json:"calendar,omitempty"`

// store
// BodyRow gana: Calendar []byte // el .ics del correo (en línea o el primer adjunto de calendario de hasta 1 MB); no se guarda
```

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestBuild_CalendarStructure`: con `Calendar{Method:"REQUEST"}`, el mensaje es `multipart/mixed` y contiene un `multipart/alternative` con `text/plain`, `text/html` y `text/calendar; method=REQUEST; charset=UTF-8`, todos en base64, más el adjunto `application/ics` `invite.ics`. Se verifica con `go-message`.
  - `TestPlan_InlineCalendar`: la BODYSTRUCTURE de `google-request.eml` da `Calendar` igual a la ruta de la parte `text/calendar` del alternative. El adjunto `invite.ics` sigue en `Attachments`. `HTMLPart` sigue siendo el HTML.
  - `TestPlan_OutlookOnlyInline`: con `outlook-request.eml`, `Calendar` no es nil y `Attachments` está vacío.
  - `TestQueueSend_CalendarPartTravels`: con `testenv.SMTP`, un `SendRequest` con `Calendar` sale con la parte `text/calendar` y el mismo ICS. El reintento desde la bandeja de salida (`DraftID`) también la lleva.
  - `TestFetchBody_Calendar`: al abrir un correo con invitación en el `testenv.IMAP`, `BodyRow.Calendar` tiene el ICS decodificado. Un correo sin invitación lo deja vacío. Un `.ics` adjunto de más de 1 MB no se baja.
- [ ] **Step 2:** `go test ./core/mimex/ ./core/mail/ -run 'Calendar' -v` → FAIL.
- [ ] **Step 3: Implementar.**
  - `Build` deriva a `buildWithCalendar` cuando `Calendar != nil`. Las imágenes en línea no se combinan con la invitación porque su cuerpo no las tiene.
  - En `Plan`, la parte `text/calendar` sin `Content-Disposition: attachment` va a `Calendar` y no a `Attachments`.
  - `fetchBody` baja esa parte y, si no hay, el primer adjunto con `text/calendar`, `application/ics` o extensión `.ics`, siempre que `Size ≤ 1<<20`. La decodifica con `mimex.DecodeText`.
- [ ] **Step 4:** `go test ./core/mimex/ ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(mimex): parte text/calendar al enviar y al leer`.

---

### Task 5: Eventos locales y la lista del calendario

**Files:**
- Create: `core/mail/calendar.go`
- Modify: `core/model/model.go`, `core/mail/events.go` (`EventCalendarChanged`), `core/mail/settings.go` (`EventReminderMinutes`)
- Test: `core/mail/calendar_test.go`

**Interfaces:**
- Consumes:
  - De Task 2: `icalx.Expand`.
  - De Task 3: `SaveEvent`, `GetEvent`, `EventsInRange` y `DeleteEvent`.
  - `store.ListLocalTasks`.
- Produces:

```go
// model
type CalendarItem struct {
	Key          string    `json:"key"`    // "e:<id>:<inicio original unix>", "m:<reunión>" o "t:<tarea>"
	Source       string    `json:"source"` // "local", "invite", "taskapp" o "task"
	EventID      int64     `json:"eventId"`
	MeetingID    int64     `json:"meetingId"`
	TaskID       int64     `json:"taskId"`
	Title        string    `json:"title"`
	Location     string    `json:"location"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	AllDay       bool      `json:"allDay"`
	Recurring    bool      `json:"recurring"`
	OccurrenceAt time.Time `json:"occurrenceAt"` // inicio original (para "solo este")
	Color        string    `json:"color"`        // color del proyecto (Taskapp)
	ProjectName  string    `json:"projectName"`
	Partstat     string    `json:"partstat"`
	NeedsReview  bool      `json:"needsReview"`
	Cancelled    bool      `json:"cancelled"`
	Attendees    int       `json:"attendees"`
	Confirmed    int       `json:"confirmed"`
	TaskappChange string   `json:"taskappChange"`
	URL          string    `json:"url"` // Taskapp
}
type CalendarRange struct {
	Items            []CalendarItem `json:"items"` // nunca nil, ordenado por Start
	TaskappStale     bool           `json:"taskappStale"`
	TaskappFetchedAt time.Time      `json:"taskappFetchedAt"`
	TaskappError     string         `json:"taskappError"`
}
type EventSave struct {
	Event        CalendarEvent `json:"event"`        // ID 0 = nuevo
	Scope        string        `json:"scope"`        // "" = todo el evento o la serie, "this" o "following"
	OccurrenceAt time.Time     `json:"occurrenceAt"` // la ocurrencia, si Scope != ""
	Notify       bool          `json:"notify"`
}
type EventDelete struct {
	ID           int64     `json:"id"`
	Scope        string    `json:"scope"`
	OccurrenceAt time.Time `json:"occurrenceAt"`
	Notify       bool      `json:"notify"`
}
// Settings gana: EventReminderMinutes *int `json:"eventReminderMinutes"`

// mail
func (c *Core) ListCalendar(ctx context.Context, from, to time.Time) (model.CalendarRange, error)
func (c *Core) GetEvent(ctx context.Context, id int64) (model.CalendarEvent, error)
func (c *Core) SaveEvent(ctx context.Context, req model.EventSave) (model.CalendarEvent, error)
func (c *Core) DeleteEvent(ctx context.Context, req model.EventDelete) error
func (c *Core) occurrences(e model.CalendarEvent, from, to time.Time) []occurrence // aplica excepciones
```

Reglas de esta tarea, todavía sin correo (el correo es de Task 6):
- **Validación**, con `UserError` y Code `invalid`:
  - `"Poné un título."`
  - `"El fin tiene que ser después del inicio."`
  - `"Esa anticipación de aviso no es válida."`
  - `"Ese invitado no tiene una dirección válida: <x>"`
  - Solo se editan eventos `Kind=="local"`. Si no, `"Este evento no se edita desde acá."`
- **Un alta:**
  - UID `uuid@postacontrol`, `Kind local`, `Status confirmed` y `TZID` = `time.Local.String()` si no viene.
  - `series_end_at` se calcula con `Expand` hasta 50 años o sale de `UNTIL`/`COUNT`.
- **Scope `this`:** guarda o reemplaza la excepción de `OccurrenceAt` con los campos nuevos. No cambia la serie.
- **Scope `following`:**
  - Corta la serie vieja con `UNTIL = OccurrenceAt − 1s`, en UTC.
  - Crea un evento nuevo, con otro UID, que empieza en la ocurrencia, con los campos nuevos y la misma regla sin `COUNT`.
  - Las excepciones con `RecurrenceAt ≥ OccurrenceAt` pasan al nuevo, corridas con el mismo desfase.
  - Si `OccurrenceAt` es la primera ocurrencia, equivale a `""`.
- **`DeleteEvent`:** con scope `this` agrega una excepción `Cancelled` y con `following` corta la serie. Sin scope borra.
- **`ListCalendar`:**
  - Eventos `local` e `invite`, salvo `invite` con `MyPartstat == DECLINED`.
  - Los `invite` cancelados salen con `Cancelled`.
  - Tareas locales pendientes con fecha:
    - Con hora, un bloque de 30 min.
    - Sin hora, de día completo.
    - Con `Key "t:<id>"`.
  - Emite `EventCalendarChanged` en cada alta, cambio o baja.

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestSaveEvent_Validations`: título vacío, fin ≤ inicio, `RemindBefore=7` e invitado `"x"` dan los mensajes de arriba.
  - `TestListCalendar_LocalTaskAndAllDay`: con `time.Local = America/Argentina/Buenos_Aires`, un evento de día completo del 20/11 aparece con `Start 2026-11-20T00:00Z` y `AllDay`. Una tarea con fecha 20/11 a las 10:00 aparece de 10:00 a 10:30, en hora local.
  - `TestListCalendar_DeclinedHidden`: un `invite` con `DECLINED` no aparece.
  - `TestSaveEvent_ScopeThis`: en una serie semanal, mover una ocurrencia deja una excepción, y `ListCalendar` muestra esa ocurrencia movida y las demás iguales.
  - `TestSaveEvent_ScopeFollowingKeepsEarlierExceptions` (Review Focus 4): una serie con una ocurrencia movida en la semana 2 y otra borrada en la semana 3, editada con `following` desde la semana 4. Quedan dos eventos: el viejo con `UNTIL` antes de la semana 4 y sus dos excepciones, y el nuevo desde la semana 4 con la hora nueva. No se repite ni se pierde ninguna ocurrencia en 8 semanas.
  - `TestSaveEvent_FollowingFromFirstEqualsAll`: no crea un segundo evento.
  - `TestDeleteEvent_ScopeThis`: la ocurrencia ya no aparece y las otras sí.
  - `TestSaveEvent_EmitsCalendarChanged`: emite `mail:calendar-changed`.
  - `TestSettings_EventReminderValidated`: guardar `EventReminderMinutes=7` da un error y `30` se guarda.
- [ ] **Step 2:** `go test ./core/mail/ -run 'Calendar|SaveEvent|DeleteEvent|EventReminder' -v` → FAIL.
- [ ] **Step 3: Implementar** según las firmas. `occurrences` usa `icalx.Expand` y después aplica las excepciones: `Cancelled` saca la ocurrencia y una movida reemplaza su hora y sus textos, aunque haya quedado fuera del rango original.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(calendario): eventos locales, repeticiones y lista del calendario`.

---

### Task 6: Invitaciones salientes

**Files:**
- Create: `core/mail/calendar_send.go`
- Modify: `core/mail/calendar.go` (`SaveEvent` y `DeleteEvent` llaman al envío), `core/model/model.go`
- Test: `core/mail/calendar_send_test.go`

**Interfaces:**
- Consumes:
  - De Task 2: `icalx.Build`.
  - De Task 4: `SendRequest.Calendar`.
  - De Task 5: `SaveEvent` y `DeleteEvent`.
  - `QueueSend`.
- Produces:

```go
type SavePreview struct {
	NotifyNeeded bool `json:"notifyNeeded"` // hay invitados y cambió algo que les importa
	Added        int  `json:"added"`
	Removed      int  `json:"removed"`
}
func (c *Core) PreviewEventSave(ctx context.Context, req model.EventSave) (model.SavePreview, error)
func (c *Core) PreviewEventDelete(ctx context.Context, req model.EventDelete) (model.SavePreview, error)
func (c *Core) queueInvitation(ctx context.Context, acc model.Account, e model.CalendarEvent, method string, to []model.Address, update bool) error
func eventToSeries(e model.CalendarEvent) icalx.Series
func invitationHTML(e model.CalendarEvent, loc *time.Location) string
```

Reglas:
- **Qué les importa a los invitados:** `Title`, `Start`, `End`, `AllDay`, `Location`, `Description`, `RRule`, `Status`, excepciones y el conjunto de invitados. **No** les importan `Notes`, `RemindBefore` ni `AccountID`.
- **Alta con invitados:** sale siempre un REQUEST a todos, desde `Event.AccountID`. Si es 0, `"Elegí desde qué cuenta mandar las invitaciones."`.
- **Edición con `Notify`:**
  - La serie (o el evento) pasa a `SEQUENCE+1`.
  - A los que siguen invitados les sale un REQUEST con la serie entera, con overrides y EXDATE.
  - A los agregados les sale un REQUEST.
  - A los quitados les sale un CANCEL.
- **Edición sin `Notify`:** no sale nada y la SEQUENCE no cambia.
- **Scope `following` con `Notify`:** sale un REQUEST de la serie vieja cortada y un REQUEST de la nueva.
- **Borrar con `Notify`:** sale un CANCEL con `SEQUENCE+1`.
- El cuerpo de la invitación es una tabla con Qué, Cuándo (`formatDateES`), Dónde y Descripción, todo escapado. `Notes` nunca aparece.
- Las invitaciones van **a la bandeja de salida** (`QueueSend`), con reintento y deshacer. El destinatario va en `To`, uno por correo. `SignatureID` es 0.

- [ ] **Step 1: Escribir los tests que fallan** con `newTestCore` y `testenv.SMTP`.
  - `TestSaveEvent_NewWithAttendeesSendsRequest`: llega un correo por invitado, con `METHOD:REQUEST`, `SEQUENCE:0`, el asunto `Invitación: Reunión — …`, el `From` de la cuenta elegida, y sin las notas en ninguna parte.
  - `TestSaveEvent_NoAccount`: con invitados y `AccountID 0` devuelve el error de arriba y no guarda nada.
  - `TestPreview_NotesOnlyNoNotice`: cambiar solo `Notes` o `RemindBefore` da `NotifyNeeded=false`.
  - `TestPreview_TimeChangeNeedsNotice`: cambiar la hora da `NotifyNeeded=true`.
  - `TestSaveEvent_NotifyFalseSendsNothing`: no sale ningún correo y la SEQUENCE no cambia.
  - `TestSaveEvent_UpdateAddRemove`: con A y B invitados, quitar B y agregar C con `Notify`. A recibe un REQUEST con `SEQUENCE:1`, C un REQUEST y B un CANCEL.
  - `TestSaveEvent_FollowingNotifies` (Review Focus 4): salen dos REQUEST por invitado, uno con el `UID` viejo y `UNTIL`, y otro con un `UID` nuevo.
  - `TestDeleteEvent_NotifyCancel`: sale un CANCEL con `SEQUENCE+1` y `STATUS:CANCELLED`.
  - `TestInvitation_GoesThroughOutbox`: con `UndoSendDelay > 0`, `ListLocalDrafts` muestra el correo esperando y `CancelSend` lo frena.
- [ ] **Step 2:** `go test ./core/mail/ -run 'Invitation|Preview|NewWithAttendees|Notify|UpdateAddRemove' -v` → FAIL.
- [ ] **Step 3: Implementar.** Primero se guarda el evento y después se encolan los correos. Si encolar falla, el evento queda guardado y el error vuelve como `UserError` `"El evento se guardó, pero no pudimos mandar las invitaciones: <motivo>"`.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(calendario): mandar invitaciones, actualizaciones y cancelaciones`.

---

### Task 7: Invitaciones recibidas

**Files:**
- Create: `core/mail/calendar_invites.go`
- Modify:
  - `core/mail/read.go`: después de `SetBody` en `GetMessage`, llama a `noteInvite`.
  - `core/mail/index.go`: lo mismo en `indexOne`.
  - `core/model/model.go`.
- Test: `core/mail/calendar_invites_test.go`

**Interfaces:**
- Consumes:
  - De Task 1: `icalx.Parse`.
  - De Task 2: `icalx.BuildReply`.
  - De Task 3: `InviteRecord`, `InviteSeen`, `SaveInviteRecord`, `EventByUID` y `SetAttendeePartstat`.
  - De Task 4: `BodyRow.Calendar`.
  - `QueueSend`.
- Produces:

```go
type MessageInvite struct {
	Found        bool      `json:"found"`
	Method       string    `json:"method"`  // REQUEST, CANCEL o REPLY
	EventID      int64     `json:"eventId"` // 0 si no quedó en el calendario
	Title        string    `json:"title"`
	Location     string    `json:"location"`
	Organizer    Address   `json:"organizer"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
	AllDay       bool      `json:"allDay"`
	Recurring    bool      `json:"recurring"`
	Partstat     string    `json:"partstat"`
	NeedsReview  bool      `json:"needsReview"`
	Cancelled    bool      `json:"cancelled"`
	Past         bool      `json:"past"`
	Outdated     bool      `json:"outdated"`  // el calendario tiene una versión más nueva
	AliasReply   bool      `json:"aliasReply"`
	ReplyFrom    Address   `json:"replyFrom"` // en un REPLY: quién respondió
	ReplyStatus  string    `json:"replyStatus"`
}
type RespondResult struct{ Warning string `json:"warning"` }

func (c *Core) noteInvite(ctx context.Context, d model.MessageDetail, ics []byte) // nunca falla el abrir/indexar; los errores van a slog
func (c *Core) InviteForMessage(ctx context.Context, messageID int64) (model.MessageInvite, error)
func (c *Core) RespondInvite(ctx context.Context, eventID int64, partstat string) (model.RespondResult, error)
func (c *Core) RemoveCancelledEvent(ctx context.Context, eventID int64) error
```

Reglas de `noteInvite`:
- **Se ignora el correo** si:
  - Su carpeta tiene rol `sent`, `drafts`, `trash` o `junk`.
  - Su `From` es una de las cuentas.
  - Ya tiene registro en `invite_messages`.
- **Idempotencia:** si `InviteSeen(uid, seq, sender, method)` da true, se guarda el registro con `Outcome "duplicate"` y no se cambia nada.
- **REQUEST, primera vez:**
  - Si el organizador es una de las cuentas, se ignora: es un evento propio.
  - Si el evento (o la serie) ya terminó, se registra con `Outcome "past"` y no se crea.
  - Si no, se crea con `Kind invite`, `RawICS` y `MyPartstat`:
    - `MyPartstat` sale del ATTENDEE que coincide, sin distinguir mayúsculas, con alguna cuenta. Esa cuenta pasa a `AccountID` y su dirección a `MyEmail`.
    - Si ninguna cuenta coincide, quedan `AccountID` = la cuenta que recibió el correo, `MyEmail` = su dirección y `AliasReply = true`.
- **REQUEST con la misma UID y SEQUENCE mayor:**
  - Se actualizan los datos y `RawICS`.
  - Si cambió la hora, quedan `NeedsReview = true` y `MyPartstat = NEEDS-ACTION`.
  - Un `DECLINED` sigue declinado, pero con `NeedsReview`.
- **REQUEST con SEQUENCE menor o igual:** `Outcome "outdated"`. No cambia nada.
- **CANCEL:**
  - Con `RECURRENCE-ID`: excepción `Cancelled` de esa ocurrencia.
  - Si no: `Status cancelled`.
- **REPLY a un evento `local` o `taskapp`:** `SetAttendeePartstat` de ese ATTENDEE, solo si ya estaba invitado.
- Emite `EventCalendarChanged` si cambió algo.

Reglas de `RespondInvite`:
- Arma el REPLY con `BuildReply(RawICS, UID, MyEmail, partstat)`.
- Lo manda **por la bandeja de salida**, desde la cuenta de `MyEmail` (o `AccountID`) al `Organizer`, con el asunto según el partstat.
- Actualiza `MyPartstat` y deja `NeedsReview=false`.
- Si `AliasReply`, devuelve el `Warning` `"Ninguna de tus cuentas figura como invitada; respondimos desde <cuenta>."`.
- Un partstat que no es `ACCEPTED`, `TENTATIVE` ni `DECLINED` da `"Esa respuesta no es válida."`.
- Un evento que no es `invite` da `"Este evento no es una invitación."`.

Reglas de `InviteForMessage`:
- Sin registro, si el correo tiene un adjunto de calendario de hasta 1 MB, lo baja con `attachmentBytes` y llama a `noteInvite`. Esto cubre los correos cacheados antes de v18.
- Después arma la vista con el registro y el evento.

- [ ] **Step 1: Escribir los tests que fallan**, con las muestras de Task 0 cargadas en `testenv.IMAP`.
  - `TestInvite_RequestCreatesEvent`: abrir `google-request.eml` crea un `invite` con `NEEDS-ACTION`, y `ListCalendar` lo muestra con `Partstat NEEDS-ACTION`.
  - `TestInvite_IndexerProcesses`: el indexador procesa el correo sin abrirlo.
  - `TestInvite_SameMailTwiceNoDuplicate` (Review Focus 2): el mismo correo en Entrada y en otra carpeta, abierto dos veces, deja un solo evento.
  - `TestInvite_OlderSequenceIgnored` (Review Focus 2): llega `google-update.ics` (seq 2) y después `google-request.ics` (seq 1). El evento queda con los datos de la seq 2 y la tarjeta del viejo dice `Outdated`.
  - `TestInvite_UpdateTimeNeedsReview`: con el evento ya `ACCEPTED`, un update con otra hora deja `NeedsReview` y `NEEDS-ACTION`.
  - `TestInvite_DeclinedStaysDeclined`: con el evento `DECLINED`, un update lo deja `DECLINED` con `NeedsReview`, y no aparece en `ListCalendar`.
  - `TestInvite_Cancel`: queda `Cancelled` en la lista.
  - `TestInvite_ReplyUpdatesAttendee`: a un evento local con A invitado le llega un REPLY de A con `ACCEPTED`, y `Attendees[A].Partstat` pasa a `ACCEPTED`.
  - `TestInvite_IgnoresSentAndOwn`: la misma invitación en Enviados, o con `From` de una cuenta, no crea nada.
  - `TestInvite_PastNotCreated`: una invitación de un evento que ya terminó no crea un evento, y la tarjeta muestra `Past`.
  - `TestRespond_SendsReplyFromMatchingAccount` (Review Focus 3): con dos cuentas y el ATTENDEE `MAILTO:Fabricio@Example.com`, que coincide con la cuenta 2, el REPLY sale desde la cuenta 2 al organizador. Lleva `METHOD:REPLY`, `PARTSTAT=ACCEPTED`, el mismo UID y la misma SEQUENCE.
  - `TestRespond_AliasWarns` (Review Focus 3): si ningún ATTENDEE coincide, responde la cuenta que lo recibió, con `Warning`.
  - `TestRespond_InvalidPartstat`: devuelve el error de arriba.
  - `TestInviteForMessage_LegacyCachedAttachment`: un correo con `body_cached=1` y un adjunto `.ics`, sin registro, se procesa al pedir la tarjeta.
- [ ] **Step 2:** `go test ./core/mail/ -run 'Invite|Respond' -v` → FAIL.
- [ ] **Step 3: Implementar** según las reglas.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(calendario): procesar y responder invitaciones recibidas`.

---

### Task 8: Avisos de eventos

**Files:**
- Create: `core/mail/calendar_reminders.go`
- Modify: `core/mail/scheduler.go` (llama a `fireEventReminders`), `core/mail/events.go`, `core/model/model.go`, `app/notify.go`, `app/notifier.go`
- Test: `core/mail/calendar_reminders_test.go`, `app/notify_test.go`

**Interfaces:**
- Consumes:
  - De Task 3: `ClaimEventReminder` y `MeetingsInRange`.
  - De Task 5: `occurrences`.
- Produces:

```go
// model
type EventReminder struct {
	Key    string    `json:"key"` // la Key del CalendarItem
	Title  string    `json:"title"`
	At     time.Time `json:"at"` // inicio de la ocurrencia
	AllDay bool      `json:"allDay"`
}
// mail
const EventCalendarReminder = "mail:calendar-reminder" // data: model.EventReminder
func (c *Core) fireEventReminders(ctx context.Context, now time.Time) error
func eventRemindAt(start time.Time, allDay bool, lead int, loc *time.Location) time.Time
// app
const EventOpenEvent = "app:open-event" // data: {"key": string, "at": RFC3339}
func EventReminderNotification(r model.EventReminder, now time.Time, loc *time.Location) notifications.NotificationOptions
```

Reglas:
- **Entran:**
  - Eventos `local` e `invite` que no están cancelados ni `DECLINED`. Las invitaciones sin responder también avisan.
  - Reuniones de la caché `taskapp_meetings` con `status == scheduled`.
  - Las ocurrencias de `[now − 24h, now + 1441 min]`.
- **Anticipación:** la del evento, o `Settings.EventReminderMinutes`, o 15. Las reuniones usan la de Ajustes.
- **Día completo:** el aviso es a las 09:00 locales del día, o a las 09:00 del día anterior si la anticipación es 1440.
- **Disparo:** si `remindAt ≤ now` y `ClaimEventReminder(key, occ, now)` reclama, se emite, salvo que `now − remindAt > reminderStale`. Las claves son `"e:<id>"` o `"m:<reunión>"`, y la ocurrencia es su inicio original.
- **Notificación:**
  - Título `Recordatorio: <título>`.
  - Cuerpo `Hoy a las HH:MM`, `<Día> dd/mm a las HH:MM` o, en día completo, `Hoy` o `<Día> dd/mm`.
  - Clic: muestra la ventana y emite `EventOpenEvent`.

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestEventReminder_FiresOnceAtLead`: con un evento a las 10:00 y anticipación 15, a las 09:44 no avisa, a las 09:45 avisa y a las 09:46 no vuelve a avisar.
  - `TestEventReminder_EachOccurrence`: un evento diario avisa en el día 1 y en el día 2.
  - `TestEventReminder_SkipsDeclinedAndCancelled`: no avisa.
  - `TestEventReminder_UnansweredInviteFires`: avisa.
  - `TestEventReminder_StaleSkipped`: un aviso de hace 30 h se reclama sin emitir.
  - `TestEventReminder_AllDayAt9`: un evento de día completo del 20/11 avisa a las 09:00 locales del 20. Con anticipación 1440, el 19 a las 09:00.
  - `TestEventReminder_TaskappMeeting`: una reunión en caché con `scheduled` avisa con la anticipación de Ajustes, y una `cancelled` no avisa.
  - `TestEventReminderNotification_Body` (en `app`): da el formato de "Hoy" y de otro día, y `Data.kind == "event-reminder"`.
- [ ] **Step 2:** `go test ./core/mail/ ./app/ -run 'EventReminder' -v` → FAIL.
- [ ] **Step 3: Implementar.** En `notify.go`, sumá `notifKindEventReminder = "event-reminder"`, la rama del clic que emite `EventOpenEvent`, y la exclusión en `notificationOpensMessage`. En `notifier.go`, enrutá `model.EventReminder`.
- [ ] **Step 4:** `go test ./core/mail/ ./app/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(calendario): avisos de eventos y reuniones`.

---

### Task 9: Cliente de reuniones de Taskapp

**Files:**
- Create: `core/taskapp/meetings.go`
- Test: `core/taskapp/meetings_test.go`

**Interfaces:**
- Consumes: el registro de Task 0 y `Client.do` (`core/taskapp/client.go`).
- Produces:

```go
type Workspace struct{ ID int64; Name string }
type Member struct{ ID int64; Name, Email string }
type MeetingAttendee struct{ UserID int64; Name, Email, Rsvp string }
type Meeting struct {
	ID, WorkspaceID, ProjectID int64 // ProjectID 0 = sin proyecto
	ProjectName, Title, Description, Location, Status string
	Start, End time.Time
	AllDay     bool
	CreatedBy  int64
	Attendees  []MeetingAttendee
	UpdatedAt  time.Time
}
type NewMeeting struct {
	WorkspaceID, ProjectID int64 // 0 = sin proyecto (no se manda project_id)
	Title, Description, Location string
	Start, End  time.Time
	AllDay      bool
	AttendeeIDs []int64
}
func (c *Client) Workspaces(ctx context.Context) ([]Workspace, error)
func (c *Client) Members(ctx context.Context, workspaceID int64) ([]Member, error)
func (c *Client) Meetings(ctx context.Context, workspaceID int64, from, to time.Time) ([]Meeting, error)
func (c *Client) CreateMeeting(ctx context.Context, m NewMeeting) (Meeting, error)
```

- [ ] **Step 1: Escribir los tests que fallan** con `httptest.Server`.
  - `TestMeetings_Query`: `GET /api/meetings` con `workspace_id=3` y `from`/`to` en RFC3339 UTC. Parsea `attendees`, `rsvp_status`, `project_id` nulo como 0 y las horas.
  - `TestCreateMeeting_Body`: el cuerpo tiene `workspace_id`, `title`, `start_time`/`end_time` en RFC3339, `all_day`, `attendee_ids` y `task_ids:[]`. Sin proyecto no manda `project_id`. Con proyecto, `project_id:5`.
  - `TestWorkspacesAndMembers`: rutas y JSON según el registro.
  - `TestMeetings_Unauthorized`: un 401 devuelve `ErrUnauthorized`.
- [ ] **Step 2:** `go test ./core/taskapp/ -run 'Meeting|Workspaces' -v` → FAIL.
- [ ] **Step 3: Implementar** con tipos `wireMeeting` en snake_case, como `types.go`.
- [ ] **Step 4:** `go test ./core/taskapp/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(taskapp): reuniones, workspaces y miembros`.

---

### Task 10: Reuniones de Taskapp en el calendario

**Files:**
- Create: `core/mail/calendar_taskapp.go`
- Modify: `core/mail/calendar.go` (`ListCalendar` suma reuniones) y `core/model/model.go`
- Test: `core/mail/calendar_taskapp_test.go`

**Interfaces:**
- Consumes:
  - De Task 9: los métodos de reuniones.
  - De Task 3: `ReplaceMeetings`, `MeetingsInRange`, `EventByMeeting`, `TaskappSnapshot` y `SetTaskappState`.
  - De Task 6: `queueInvitation`.
  - `taskappClient`, `taskappErr`, `taskappMeCached` y `activeProjects` (`core/mail/taskapp.go`).
- Produces:

```go
// model
type TaskappWorkspace struct{ ID int64 `json:"id"`; Name string `json:"name"` }
type TaskappMember struct{ ID int64 `json:"id"`; Name string `json:"name"`; Email string `json:"email"` }
type TaskappMeetingCreate struct {
	WorkspaceID int64     `json:"workspaceId"`
	ProjectID   int64     `json:"projectId"`
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
// mail
func (c *Core) TaskappWorkspaces(ctx context.Context) ([]model.TaskappWorkspace, error)
func (c *Core) TaskappMembers(ctx context.Context, workspaceID int64) ([]model.TaskappMember, error)
func (c *Core) CreateTaskappMeeting(ctx context.Context, req model.TaskappMeetingCreate) (model.CalendarItem, error)
func (c *Core) NotifyTaskappChange(ctx context.Context, eventID int64) error
func (c *Core) DismissTaskappChange(ctx context.Context, eventID int64) error
func (c *Core) RemoveTaskappExternals(ctx context.Context, eventID int64) (model.RespondResult, error)
func (c *Core) StartMeetingRefresh(ctx context.Context, every time.Duration) // ventana [now−1d, now+14d], cada 10 min
func (c *Core) meetingsFor(ctx context.Context, from, to time.Time) (items []model.CalendarItem, stale bool, at time.Time, errMsg string)
```

Reglas:
- **Qué reuniones entran** (`meetingsFor`):
  - Para cada workspace del usuario, `Meetings(ws, from, to)`, filtradas a las que tienen al usuario (`me.ID`) entre los asistentes o como `CreatedBy`.
  - Se guardan con `ReplaceMeetings`, y la URL es la ruta web del registro.
  - Si falla:
    - Un 401 se trata como en las tareas: traba, `taskapp-auth` y `TaskappError`.
    - Cualquier otro error devuelve las de la caché con `stale` y el motivo.
  - Sin Taskapp conectado no hay reuniones ni error.
- **El espejo de una reunión:**
  - Su `Key` es `"m:<id>"`, con el color y el nombre del proyecto.
  - Si tiene espejo, el ítem sale **una sola vez**, el de la reunión, con los conteos de invitados externos y su `TaskappChange`.
- **`CreateTaskappMeeting`:**
  - Valida título y horas como en Task 5. Con `External` y `AccountID 0` da error.
  - Hace `POST` y, después, con `context.WithoutCancel` y `c.hold()`, como `CreateTaskappItem`:
    - Si hay externos, guarda el espejo con `Kind taskapp`, `TaskappMeetingID`, UID `taskapp-<id>@postacontrol`, los externos como invitados y la foto `{title,start,end,allDay,location,description,status}` en JSON.
    - Encola un REQUEST a cada externo.
  - Si el POST da un error de red ambiguo, devuelve `"No sabemos si la reunión se creó en Taskapp: revisala ahí antes de volver a intentar; no la vuelvas a crear sin revisar."`.
  - Si se creó pero no se pudo guardar el espejo, devuelve `"La reunión se creó en Taskapp, pero no se pudo guardar la lista de invitados externos en esta PC; no la vuelvas a crear."`. Contiene el fragmento `"no la vuelvas a crear"`, el contrato de `isOrphanError`.
- **Detección de cambios en cada lectura exitosa** que cubre la hora de un espejo:
  - Si la reunión no vino, `change = "deleted"`.
  - Si vino con `status cancelled`, `change = "cancelled"`.
  - Si cambió algo de la foto, `change = "changed"`.
  - Solo se emite `EventCalendarChanged` si cambió el estado.
- **`NotifyTaskappChange`:**
  - Con `changed`, sale un REQUEST con `SEQUENCE+1` y los datos nuevos de la reunión.
  - Con `cancelled` o `deleted`, sale un CANCEL y el espejo queda `Status cancelled`.
  - En los dos casos se actualiza la foto y `change = ""`.
- **`DismissTaskappChange`:** actualiza la foto a lo actual y deja `change = ""`, sin correo. Con `cancelled` o `deleted`, el espejo queda cancelado.
- **Quitar los invitados externos** (`RemoveTaskappExternals`): sale un CANCEL a los externos, sin preguntar, y se borra el espejo. Devuelve `RespondResult{Warning: "La reunión sigue en Taskapp; solo se avisó a los invitados externos."}`. Un `DeleteEvent` o un `SaveEvent` de un `Kind taskapp` da `"Este evento no se edita desde acá."`.

- [ ] **Step 1: Escribir los tests que fallan** con un Taskapp falso (`httptest`) y `testenv.SMTP`.
  - `TestMeetings_FilteredToMe`: de 3 reuniones (una donde soy asistente, una que creé y una ajena) aparecen 2 en `ListCalendar`.
  - `TestMeetings_OfflineServesCache`: después de una lectura buena, con el servidor caído, aparecen las mismas con `TaskappStale` y `TaskappFetchedAt`.
  - `TestMeetings_AuthLatches`: un 401 deja `TaskappError` y los eventos locales igual aparecen.
  - `TestCreateMeeting_WithExternals`: el POST lleva los asistentes, se guarda el espejo y les llega un REQUEST a los externos con `UID taskapp-<id>@postacontrol`. `ListCalendar` muestra la reunión una sola vez.
  - `TestCreateMeeting_NoExternalsNoMirror`: no guarda espejo ni manda correo.
  - `TestCreateMeeting_OrphanMessage`: si falla guardar el espejo, el error contiene `"no la vuelvas a crear"`.
  - `TestTaskappChange_Detected`: si la reunión cambia de hora en el servidor, el siguiente `ListCalendar` muestra `TaskappChange "changed"` y no se mandó ningún correo.
  - `TestTaskappChange_OfflineNoFalseDelete` (Review Focus 5): con el servidor caído, el espejo no se marca `deleted`. Con un servidor que responde sin la reunión, en un rango que la cubre, se marca `deleted`.
  - `TestNotifyTaskappChange_Update`: sale un REQUEST con `SEQUENCE:1` y el flag se limpia.
  - `TestDismissTaskappChange_UntilNextChange`: después de descartar no vuelve a marcarse con los mismos datos, y sí con otro cambio.
- [ ] **Step 2:** `go test ./core/mail/ -run 'Meeting|TaskappChange' -v` → FAIL.
- [ ] **Step 3: Implementar.** `StartMeetingRefresh` usa `c.spawn` como `StartScheduler`. Sin Taskapp conectado no hace nada en esa vuelta.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(calendario): reuniones de Taskapp con invitados externos`.

---

### Task 11: Importar de Google y respaldo

**Files:**
- Create: `core/mail/calendar_import.go`
- Modify: `core/mail/backup.go` (eventos en el respaldo) y `core/model/model.go` (`BackupImport` suma `EventsAdded` y `EventsSkipped`)
- Test: `core/mail/calendar_import_test.go`, `core/mail/backup_test.go`

**Interfaces:**
- Consumes:
  - De Task 1: `icalx.Parse`.
  - De Task 3: `SaveEvent`, `EventByUID` y `AllEvents`.
- Produces:

```go
type ICSCalendar struct {
	ID     string `json:"id"`     // ruta, o "ruta#entrada" para un zip
	Name   string `json:"name"`   // X-WR-CALNAME, o el nombre del archivo
	Events int    `json:"events"` // cantidad de series
}
type ICSImportResult struct{ Added, Updated, Skipped int }
func (c *Core) PreviewICSImport(ctx context.Context, paths []string) ([]model.ICSCalendar, error)
func (c *Core) ImportICS(ctx context.Context, ids []string) (model.ICSImportResult, error)
```

Reglas:
- **Límites:** los de Global Constraints. Si se pasan, `"Ese archivo es demasiado grande para importar."`.
- **UID existente:**
  - Si es `invite` y la SEQUENCE importada es mayor, se actualiza (`Updated`).
  - Si no, se saltea (`Skipped`).
- **Organizador que no es una cuenta** (y no vacío): entra como `invite`, con `RawICS` armado con `icalx.Build("PUBLISH", serie)` de esa serie. Lleva `MyPartstat` y `MyEmail` del ATTENDEE que coincide, o `NEEDS-ACTION` y la primera cuenta si ninguno coincide. Conserva la SEQUENCE.
- **Organizador que es una cuenta, o vacío:**
  - Entra como `local`, con `AccountID` de esa cuenta (o 0).
  - Sus invitados quedan sin las direcciones propias.
  - Conserva la SEQUENCE: la próxima actualización sale con la siguiente.
- Las excepciones (`Overrides` y `ExDates`) pasan a `Exceptions`.
- No se manda **ningún** correo. Al final se emite `EventCalendarChanged` una vez.
- **Respaldo:**
  - Suma `Events []backupEvent`, con todos los campos, invitados, excepciones y `RawICS`.
  - Importar suma solo los UID que no están.

- [ ] **Step 1: Escribir los tests que fallan**
  - `TestPreviewICS_ZipEntries`: un zip con `principal.ics` y `feriados.ics` da 2 `ICSCalendar`, con su `Name` y cantidad.
  - `TestImportICS_KindsAndNoMail`: desde `google-export.ics`, el evento de otro entra como `invite` con su PARTSTAT, el propio como `local` con sus invitados sin la dirección propia, la serie con su excepción y el de día completo. No sale ningún correo.
  - `TestImportICS_Twice`: la segunda vez todo va a `Skipped` y no hay duplicados.
  - `TestImportICS_TooBig`: un zip con una entrada de 51 MB declarada da el error.
  - `TestImportedOwnEvent_NextSequence`: un evento propio importado con `SEQUENCE:4`, editado con `Notify`, sale con `SEQUENCE:5`.
  - `TestImportedInvite_LaterUpdateApplies`: después de importar, un REQUEST por correo con la misma UID y SEQUENCE mayor actualiza ese evento.
  - `TestBackup_EventsRoundTrip`: exportar e importar en una base vacía da los mismos eventos. Importar de nuevo da `EventsSkipped`.
- [ ] **Step 2:** `go test ./core/mail/ -run 'ICS|Imported|Backup_Events' -v` → FAIL.
- [ ] **Step 3: Implementar** con `archive/zip` y `io.LimitReader`.
- [ ] **Step 4:** `go test ./core/mail/ -v` → PASS.
- [ ] **Step 5: Commit** `feat(calendario): importar .ics de Google y eventos en el respaldo`.

---

### Task 12: Servicio de la app, bindings y estado del frontend

**Files:**
- Modify:
  - `app/mailservice.go`: envoltorios.
  - `app/mailservice_test.go`.
  - `app/mailservice.go`, también: arranca `core.StartMeetingRefresh(ctx, 10*time.Minute)` al lado de `core.StartScheduler` (línea ~92).
  - `frontend/src/lib/view.ts`: `{ kind: "calendar" }`.
  - `frontend/src/lib/queries.ts`, `frontend/src/lib/events.ts`.
  - `frontend/src/features/folders/FolderSidebar.tsx`, `frontend/src/features/folders/SidebarRail.tsx`: entrada "Calendario" después de "Tareas", con `CalendarIcon`.
  - `frontend/src/features/layout/MailShell.tsx`: prop `full`.
  - `frontend/src/App.tsx`.
- Test: `frontend/src/lib/view.test.ts`, `frontend/src/features/layout/MailShell.test.tsx`, `frontend/src/features/folders/*.test.tsx`

**Interfaces:**
- Consumes: todo lo de `core/mail` de las tareas 5 a 11.
- Produces:
  - **`MailService`**, un método por cada función pública de `core/mail` de las tareas 5 a 11, con el mismo nombre y la misma firma (sin `ctx` en TS). Son:
    - `ListCalendar`, `GetEvent`, `SaveEvent`, `DeleteEvent`, `PreviewEventSave`, `PreviewEventDelete`.
    - `InviteForMessage`, `RespondInvite`, `RemoveCancelledEvent`.
    - `TaskappWorkspaces`, `TaskappMembers`, `CreateTaskappMeeting`, `NotifyTaskappChange`, `DismissTaskappChange`, `RemoveTaskappExternals`.
    - `PreviewICSImport`, `ImportICS`.
  - **`MailService.PickICSFiles() ([]string, error)`:** diálogo de abrir con varios archivos, filtro `Calendario (*.ics, *.zip)` y título `Importar calendario`. Si se cancela, devuelve una lista vacía.
  - **`OpenTaskInTaskapp`** también sirve para las URL de reuniones, con el mismo chequeo de host.
  - **`queries.ts`:**
    - `useCalendar(from: Date, to: Date)`, con key `["calendar", from.toISOString(), to.toISOString()]` y `placeholderData` del rango anterior.
    - `useEvent(id: number | null)`, con key `["event", id]`.
    - `useMessageInvite(messageId: number | null)`, con key `["invite", messageId]`.
    - `useTaskappWorkspaces()` y `useTaskappMembers(workspaceId: number | null)`.
  - **`events.ts`:**
    - `mail:calendar-changed` invalida `["calendar"]`, `["event"]` e `["invite"]`.
    - `onOpenEvent(cb: (r: { key: string; at: string }) => void)` y `requestOpenEvent(r)` para `app:open-event`.
    - `SettingsSection` suma `"calendar"`.
  - **`MailShell`:** `full?: (s: ShellState) => ReactNode | null`. Cuando devuelve algo, el panel `main` lo muestra en vez del grupo lista+lector.
  - **`ShellState`** suma `eventKey: string | null`, `selectEvent(key: string | null)` y `calendarDate: string | null` (ISO). Ese último lo fija `onOpenEvent`, que también cambia a la vista calendario.

- [ ] **Step 1: Escribir los tests que fallan**
  - `view.test.ts`: `isView({kind:"calendar"})`. `loadView` recuerda el calendario y `viewIsValid` da true.
  - `MailShell.test.tsx`: con `full` que devuelve un nodo, no se renderizan `list` ni `reader`.
  - Sidebar y rail: el botón "Calendario" llama a `selectView({kind:"calendar"})` y queda activo.
  - `app/mailservice_test.go`: `PickICSFiles` sin app devuelve un error claro, como los otros pickers.
- [ ] **Step 2:** `task common:build:frontend`, que regenera los bindings, y después `cd frontend && npx vitest run src/lib/view.test.ts src/features/layout src/features/folders` → FAIL.
- [ ] **Step 3: Implementar.** Arreglá los fixtures de frontend que rompan los bindings nuevos, por ejemplo `features/tasks/fixtures.ts`, `Settings` y `BackupImport`.
- [ ] **Step 4:** `task check` → PASS.
- [ ] **Step 5: Commit** `feat(app): servicio y navegación del calendario`.

---

### Task 13: Vista Calendario (FullCalendar) y detalle

**Files:**
- Create:
  - `frontend/src/features/calendar/CalendarView.tsx`
  - `frontend/src/features/calendar/CalendarToolbar.tsx`
  - `frontend/src/features/calendar/EventDetail.tsx`
  - `frontend/src/features/calendar/ScopeDialog.tsx`
  - `frontend/src/features/calendar/NotifyDialog.tsx`
  - `frontend/src/features/calendar/calendar.css`
  - `frontend/src/lib/calendar.ts`
- Modify: `frontend/package.json` y `frontend/package-lock.json` (solo FullCalendar), `frontend/src/App.tsx` (`full` → `CalendarView`)
- Test: `frontend/src/lib/calendar.test.ts`, `frontend/src/features/calendar/CalendarView.test.tsx`, `frontend/src/features/calendar/EventDetail.test.tsx`

**Interfaces:**
- Consumes:
  - De Task 12: `useCalendar`, `useEvent` y `ShellState.eventKey`/`calendarDate`.
  - Los bindings `SaveEvent`, `PreviewEventSave`, `RespondInvite`, `NotifyTaskappChange`, `DismissTaskappChange`, `RemoveTaskappExternals`, `RemoveCancelledEvent` y `OpenTaskInTaskapp`.
- Produces:
  - **`lib/calendar.ts`:**
    - `type CalendarViewId = "timeGridDay" | "workWeek" | "timeGridWeek" | "dayGridMonth" | "listWeek"`.
    - `loadCalendarView(): CalendarViewId`, con clave `pc:calendar:view` y valor por defecto `"workWeek"`.
    - `saveCalendarView(v)`.
    - `toFcEvent(it: CalendarItem): EventInput`, con `id=key`, `editable` solo si `source==="local"` y no está cancelado, `classNames` por fuente y estado, y `backgroundColor` = `it.color` en Taskapp.
    - `allDayDate(iso: string): string`: la fecha `AAAA-MM-DD` en UTC, para que un día completo no se corra.
  - **`CalendarView({ state })`:**
    - Usa `@fullcalendar/react` con `[dayGridPlugin, timeGridPlugin, listPlugin, interactionPlugin]` y `locale: esLocale` (`@fullcalendar/core/locales/es`).
    - Opciones: `firstDay: 1`, `nowIndicator`, `scrollTime: "08:00"`, `headerToolbar: false` y `views: { workWeek: { type: "timeGrid", duration: { weeks: 1 }, weekends: false } }`.
    - `datesSet` actualiza el rango de `useCalendar`.
    - Muestra el detalle (`EventDetail`) en una columna fija `w-80` a la derecha.
  - **`CalendarToolbar`:** Hoy, ‹, › y el título del rango; un selector segmentado `Día | Semana laboral | Semana | Mes | Agenda`, que se recuerda; y `+ Nuevo`. Si `TaskappStale`, muestra un aviso chico: "Taskapp sin conexión · actualizado HH:MM".
  - **Interacción:**
    - Clic en un hueco (`select`), "+ Nuevo" y "Editar" llaman a los callbacks `onNewEvent(start, end, allDay)` y `onEditEvent(item)`. En esta tarea quedan sin destino; Task 14 los conecta a `EventDialog`.
    - Con `state.calendarDate`, la vista va a esa fecha (`gotoDate`) y selecciona `state.eventKey`. Así abre el aviso de un evento.
    - Arrastrar o estirar un evento local (`eventDrop`/`eventResize`):
      1. Si es repetido, pide el alcance con `ScopeDialog`.
      2. `PreviewEventSave`. Si `notifyNeeded`, pregunta "¿Avisar a los invitados?" con `NotifyDialog`.
      3. `SaveEvent`. Si se cancela o falla, `info.revert()`.
  - **`EventDetail({ item })`, según `source`:**
    - `local`: datos, invitados con su estado (`Confirmed/Attendees`), y Editar y Borrar.
    - `invite`: organizador; Aceptar, Tal vez y Rechazar; "Cambió: revisá" si `needsReview`; y "Quitar" si está cancelada (`RemoveCancelledEvent`).
    - `taskapp`: asistentes, "Abrir en Taskapp", y "Quitar invitados externos" si los hay (`RemoveTaskappExternals`, con su `warning` en un toast). Si hay `taskappChange`, el aviso "Cambió en Taskapp: ¿avisar a los invitados externos?" con `Avisar` y `Ahora no`.
    - `task`: "Abrir tarea", que llama a `state.openTask(taskId)`.
  - **`ScopeDialog({ open, onPick: (s: "this" | "following" | null) => void })`:** "Solo este" o "Este y los siguientes". `null` es cancelar.
  - **`NotifyDialog({ open, added, removed, onPick: (notify: boolean | null) => void })`:** el título es "¿Avisar a los invitados?" y los botones son "Avisar" y "No avisar". `null` es cancelar.
  - **Estilos:** `calendar.css` mapea las variables de FullCalendar a los tokens de la app (`--fc-border-color: var(--border)` y similares, en claro y oscuro). Las clases son:
    - `.ev-local`: color primario.
    - `.ev-task`: gris con borde punteado.
    - `.ev-invite-pending`: fondo de la tarjeta con borde naranja.
    - `.ev-cancelled`: tachado.

- [ ] **Step 1: Instalar** con `cd frontend && npx -y npm@11 install --save-exact @fullcalendar/react@6.1.22 @fullcalendar/core@6.1.22 @fullcalendar/daygrid@6.1.22 @fullcalendar/timegrid@6.1.22 @fullcalendar/list@6.1.22 @fullcalendar/interaction@6.1.22`. Verificá con `git diff frontend/package-lock.json | grep '^-.*"libc"'`: no tiene que imprimir nada.
- [ ] **Step 2: Escribir los tests que fallan.** En los tests de componentes, mockeá `@fullcalendar/react` con un stub que expone sus props.
  - `calendar.test.ts`:
    - `toFcEvent` de un `local` da `editable true`. De `invite`, `taskapp` y `task` da false.
    - Un `invite` con `NEEDS-ACTION` lleva `ev-invite-pending`, y uno cancelado `ev-cancelled`.
    - `allDayDate("2026-11-20T00:00:00Z") === "2026-11-20"` con `TZ=America/Argentina/Buenos_Aires`.
    - `loadCalendarView()` sin nada guardado da `"workWeek"`, y con basura también.
  - `CalendarView.test.tsx`:
    - La vista inicial es `workWeek`. Cambiar a Mes guarda `dayGridMonth`.
    - Con `calendarDate` y `eventKey` en el estado, llama a `gotoDate` con esa fecha y muestra el detalle de ese ítem.
    - Con `TaskappStale` se ve el aviso.
    - Un `eventDrop` de un evento con invitados y `notifyNeeded` abre la pregunta. Elegir "No avisar" llama a `SaveEvent` con `notify:false`. Cancelar llama a `revert`.
  - `EventDetail.test.tsx`:
    - En una invitación, "Aceptar" llama a `RespondInvite(id, "ACCEPTED")` y muestra el `warning` en un toast si viene.
    - En una reunión con `taskappChange:"changed"`, "Avisar" llama a `NotifyTaskappChange` y "Ahora no" llama a `DismissTaskappChange`.
    - En una tarea, "Abrir tarea" llama a `openTask`.
- [ ] **Step 3:** `cd frontend && npx vitest run src/lib/calendar.test.ts src/features/calendar` → FAIL.
- [ ] **Step 4: Implementar.**
- [ ] **Step 5:** `task check` → PASS. Con `wails3 dev` o `task dev`, abrí el calendario y mirá en claro y oscuro: la semana laboral, el mes, la agenda y arrastrar un evento.
- [ ] **Step 6: Commit** `feat(ui): vista Calendario` con `package.json` y solo las entradas de FullCalendar del lock.

---

### Task 14: Crear y editar eventos y reuniones

**Files:**
- Create:
  - `frontend/src/features/calendar/EventDialog.tsx`
  - `frontend/src/lib/rrule.ts`
- Modify: `frontend/src/features/calendar/CalendarView.tsx` (conecta `onNewEvent` y `onEditEvent`)
- Test: `frontend/src/lib/rrule.test.ts`, `frontend/src/features/calendar/EventDialog.test.tsx`

**Interfaces:**
- Consumes:
  - Los bindings `GetEvent`, `SaveEvent`, `PreviewEventSave`, `DeleteEvent`, `PreviewEventDelete`, `CreateTaskappMeeting` y `TaskappProjects`.
  - `useTaskappWorkspaces`, `useTaskappMembers` y `useTaskappStatus`.
  - `RecipientInput` (`features/compose/RecipientInput.tsx`), para los invitados con autocompletado de contactos.
  - De Task 13: `ScopeDialog`, `NotifyDialog` y los callbacks `onNewEvent` y `onEditEvent`.
- Produces:
  - **`lib/rrule.ts`:**
    - `type Repeat = { freq: "none" | "daily" | "weekly" | "monthly" | "yearly"; days: number[] /* 0=lunes */; until: string /* AAAA-MM-DD o "" */ }`.
    - `buildRRule(r: Repeat, start: Date): string`: `""` si es `none`. Semanal sin días usa el día de `start`. `UNTIL` va a las 23:59:59 locales, pasado a UTC.
    - `parseRRule(s: string): Repeat | null`, que da null si la regla no la armó `buildRRule`.
    - `describeRRule(s: string): string`, por ejemplo "Cada semana: lun, mié · hasta 20/12/2026" o "Repetición personalizada".
  - **`EventDialog({ open, onOpenChange, initial, eventId })`:**
    - Usa el patrón de `DialogContent` de v0.8.1: `max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-lg`.
    - Arriba, el destino: `Calendario local` o `Reunión en Taskapp`. Este último aparece solo si Taskapp está conectado y solo al crear.
    - Campos comunes: título, día completo o inicio y fin, lugar, descripción (con la ayuda "Va en la invitación") y notas privadas (con la ayuda "Solo en esta PC").
    - Campos de `local`: repetición (con `describeRRule` si es personalizada), aviso (`Por defecto` + `REMINDER_OPTIONS`), invitados y "Enviar desde", que solo aparece con invitados. Por defecto es la cuenta de la vista abierta o la primera.
    - Campos de `taskapp`:
      - Proyecto, opcional, sacado de `TaskappProjects`.
      - Workspace: sale del proyecto. Sin proyecto, el selector aparece solo si hay más de un workspace.
      - Asistentes: casillas con los miembros del workspace.
      - Invitados externos y "Enviar desde".
    - **Guardar un `local`:**
      1. Si el evento ya existe y es repetido, `ScopeDialog` pide "Solo este" o "Este y los siguientes".
      2. `PreviewEventSave`. Si `notifyNeeded`, `NotifyDialog`.
      3. `SaveEvent`.
      - Un alta con invitados no pregunta y manda.
    - **Borrar:** pasa por `ScopeDialog` si es repetido, después `PreviewEventDelete` y, si `notifyNeeded`, `NotifyDialog` con "Avisar y borrar" o "Borrar sin avisar".
    - **Guardar un `taskapp`:** llama a `CreateTaskappMeeting`. Si da un error huérfano (`isOrphanError`), el diálogo se cierra igual y se muestra el mensaje.

- [ ] **Step 1: Escribir los tests que fallan**
  - `rrule.test.ts`:
    - `buildRRule({freq:"weekly", days:[0,2], until:""})` → `"FREQ=WEEKLY;BYDAY=MO,WE"`.
    - Con `until:"2026-12-20"` lleva `UNTIL=20261221T025959Z` en `America/Argentina/Buenos_Aires`.
    - `parseRRule(buildRRule(x))` vuelve a dar `x`.
    - `parseRRule("FREQ=WEEKLY;INTERVAL=2")` da null.
    - `describeRRule` de una regla ajena da "Repetición personalizada".
  - `EventDialog.test.tsx`:
    - Sin Taskapp no aparece el destino.
    - Con título vacío, el botón Guardar queda deshabilitado.
    - Con invitados aparece "Enviar desde".
    - Editar solo las notas: `PreviewEventSave` da `notifyNeeded:false`, así que no pregunta y llama a `SaveEvent` con `notify:false`.
    - Editar la hora de uno con invitados: pregunta, "Avisar" → `notify:true`.
    - Un repetido pide el alcance y manda `scope:"following"` con `occurrenceAt`.
    - En una reunión de Taskapp con un solo workspace y sin proyecto, no aparece el selector de workspace. Con dos, sí.
    - Crear la reunión manda `CreateTaskappMeeting` con `attendeeIds`, `external` y `accountId`.
- [ ] **Step 2:** `cd frontend && npx vitest run src/lib/rrule.test.ts src/features/calendar/EventDialog.test.tsx` → FAIL.
- [ ] **Step 3: Implementar.** Conectá `onNewEvent` y `onEditEvent` de `CalendarView` a `EventDialog`.
- [ ] **Step 4:** `task check` → PASS.
- [ ] **Step 5: Commit** `feat(ui): crear y editar eventos y reuniones de Taskapp`.

---

### Task 15: Tarjeta de invitación en el lector

**Files:**
- Modify: `frontend/src/features/reader/InviteCard.tsx` (reescrita) y `frontend/src/features/reader/MessageView.tsx`
- Delete: `frontend/src/lib/ics.ts`, y su test si existe, si nada más lo importa
- Test: `frontend/src/features/reader/InviteCard.test.tsx`

**Interfaces:**
- Consumes:
  - De Task 12: `useMessageInvite(messageId)`.
  - Los bindings `RespondInvite`, `RemoveCancelledEvent` y `SaveAttachment`.
  - `requestOpenEvent`.
- Produces: `InviteCard({ messageId, attachment?: Attachment })`, que se muestra si `invite.found`. `MessageView` la monta para todo correo, sin depender de `isCalendar`, y le pasa el adjunto de calendario si existe para "Guardar .ics".

Qué muestra la tarjeta:
- **REQUEST vigente:**
  - Título, cuándo (con "se repite" si corresponde), lugar y "Organiza …".
  - Botones Aceptar, Tal vez y Rechazar. El actual queda marcado según `partstat`.
  - Si `needsReview`, "Cambió la hora: volvé a responder".
  - Si `aliasReply`, "Ninguna de tus cuentas figura como invitada; vas a responder desde <cuenta>".
  - "Ver en el calendario" llama a `requestOpenEvent({key:"e:<id>:…", at:start})`.
- **`outdated`:** "Hay una versión más nueva de esta invitación", sin botones.
- **`past`:** "Este evento ya pasó", sin botones.
- **CANCEL:** "Cancelada" y "Quitar del calendario".
- **REPLY:** "<nombre> aceptó", "rechazó" o "respondió tal vez" la invitación a "<título>".

- [ ] **Step 1: Escribir los tests que fallan**
  - Un REQUEST con `NEEDS-ACTION`: "Aceptar" llama a `RespondInvite(eventId,"ACCEPTED")` y, si vuelve un `warning`, lo muestra en un toast.
  - `outdated` y `past` no muestran botones de respuesta.
  - `aliasReply` muestra el aviso.
  - Un CANCEL: "Quitar del calendario" llama a `RemoveCancelledEvent`.
  - Un REPLY muestra el texto con el nombre.
  - Con `found:false` no renderiza nada.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/reader` → FAIL.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check` → PASS.
- [ ] **Step 5: Commit** `feat(ui): responder invitaciones desde el correo`.

---

### Task 16: Ajustes → Calendario, documentación y versión

**Files:**
- Create: `frontend/src/features/settings/CalendarSection.tsx`, `docs/releases/v0.9.0.md`
- Modify: `frontend/src/features/settings/SettingsDialog.tsx` (sección `calendar`, con ícono `CalendarIcon`, después de Taskapp), `README.md` y `docs/superpowers/plans/2026-11-19-calendario-registro.md` (checklist de aceptación)
- Test: `frontend/src/features/settings/CalendarSection.test.tsx`

**Interfaces:**
- Consumes:
  - Los bindings `PickICSFiles`, `PreviewICSImport` e `ImportICS`.
  - `useSettings` con `eventReminderMinutes`.
- Produces:
  - **`CalendarSection`:**
    - "Aviso de eventos por defecto", con un select de `REMINDER_OPTIONS` y 15 por defecto.
    - "Importar .ics". Elige archivos y muestra la lista de calendarios con una casilla cada uno, todos marcados salvo los que se llamen "Feriados", "Holidays" o "Cumpleaños".
    - El botón "Importar N calendarios" muestra el resultado: "Se importaron X eventos (Y actualizados, Z ya estaban)".
    - Texto de ayuda: "En Google Calendar: Configuración → Importar y exportar → Exportar. Elegí el .zip que baja."
  - **`v0.9.0.md`:**
    - Novedades.
    - El aviso de que la base pasa a la v18 y una versión anterior no la abre.
    - El aviso de que en la primera apertura se vuelven a bajar los correos de la Entrada de los últimos 60 días para encontrar invitaciones.

- [ ] **Step 1: Escribir los tests que fallan**
  - Cambiar el aviso guarda `eventReminderMinutes`.
  - Después de elegir archivos, "Feriados" queda sin marcar. "Importar" llama a `ImportICS` con los ids marcados y muestra el resultado.
  - Si el picker devuelve una lista vacía, no pasa nada.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/settings/CalendarSection.test.tsx` → FAIL.
- [ ] **Step 3: Implementar.** Agregá la sección del README "Calendario" al lado de la de Tareas: qué hace, invitaciones, Taskapp, importar y la limitación de que no hay sincronización ni móvil.
- [ ] **Step 4: Checklist de aceptación** en el registro, para que Fabricio la haga con la app real:
  1. Crear un evento con su Gmail como invitado. Llega la invitación, Google la muestra con los botones y aceptarla allá marca "aceptó" acá.
  2. Recibir una invitación de Google y aceptarla desde PostaControl. En Google figura como aceptada.
  3. Mover el evento y avisar. En Google se actualiza.
  4. Borrarlo y avisar. En Google se cancela.
  5. Crear una reunión de Taskapp con un externo. Aparece en Taskapp y el externo recibe el `.ics`.
  6. Cambiar la hora en Taskapp. Aparece el aviso "Cambió en Taskapp" y "Avisar" le actualiza al externo.
  7. Importar el ZIP de Google sin "Feriados" y verificar las repeticiones y los días completos.
  8. Verificar que llega el aviso de un evento.
- [ ] **Step 5:** `task check` → PASS.
- [ ] **Step 6: Commit** `docs(calendario): ajustes, README y notas de la v0.9.0`.

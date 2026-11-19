# Registro de la API de reuniones de Taskapp y muestras iCalendar (verificado)

Fecha: 2026-11-19. Rama `calendario`. Solo `GET` sobre producción; no se escribió nada.

## Cómo leer este registro

- **[PROD]**: observado con `GET` autenticado directo contra `https://apitaskapp.codecontrol.com.ar` (token de la conexión de PostaControl en el llavero, servicio `postacontrol`, cuenta `taskapp-token`; no se imprimió ni se guardó).
- **[CÓDIGO]**: leído en la copia local del backend (`~/desarrollo_local/taskapp_web/taskApp-backend`). Es de agosto y puede estar atrasada. Todo lo que es escritura (`POST`/`PUT`/`DELETE`) es **[CÓDIGO]**: hay que confirmarlo en la primera prueba real.
- Los datos de personas (emails, nombres, CUIT, domicilio, firmas de URLs) están redactados con placeholders.

Autenticación y URL base: igual que en `2026-11-16-tareas-taskapp-registro.md` (`Authorization: Bearer <JWT>`, errores `{"error":"..."}`).

## Respuestas a las preguntas del Step 1

| Pregunta | Respuesta | Fuente |
|---|---|---|
| ¿`GET /api/meetings` filtra por superposición o por `start_time`? | **Solo por `start_time`**: `start_time >= from AND start_time <= to`. Una reunión que empezó antes de `from` y sigue en curso **no** aparece. Prueba: reunión 13:00–14:30Z; con `from=13:30Z&to=14:00Z` devuelve 0; con `from=12:00Z&to=13:30Z` la devuelve. Consecuencia para PostaControl: pedir un rango ensanchado hacia atrás (p. ej. 1 día antes del `from` visible) y recortar por superposición en el cliente. | [CÓDIGO] `repository/meeting.go` + [PROD] |
| ¿El creador queda como asistente? | **No automáticamente.** Solo son asistentes los `attendee_ids` que se mandan (y deben ser miembros del workspace). En prod, 8 de 11 reuniones tienen al creador en `attendees` porque el frontend lo incluye; 3 no. Para que figure, hay que mandarlo en `attendee_ids`. | [CÓDIGO] + [PROD] |
| Valores de `status` | `scheduled`, `completed`, `cancelled`. En prod se vieron los tres (8 cancelled, 1 completed, 2 scheduled en el rango consultado). Solo `scheduled` admite completar, reprogramar y cancelar (`400 meeting is not scheduled`). | [CÓDIGO] + [PROD] |
| Valores de `rsvp_status` | `pending`, `accepted`, `declined`, `maybe` (otro valor: `400 invalid rsvp status: <x>`). En prod se vieron `accepted` y `pending`. Reprogramar vuelve todos a `pending`. | [CÓDIGO] + [PROD] |
| ¿`POST /api/meetings` devuelve la reunión con `attendees`? | Sí: `201` con `GetByID` (la misma forma que el `GET`, con `attendees` si hay y `tasks` si hay). | [CÓDIGO] |
| `all_day` | Es booleano y siempre viene (`false` en las 11 de prod; ninguna de día completo). El código no impone fechas especiales para día completo: `start_time`/`end_time` siguen siendo instantes RFC 3339. | [CÓDIGO] + [PROD] |

Diferencias código/producción: **`GET /api/workspaces/{id}/members` devuelve `"status": ""`** en prod (el código sugería `"active"`); no usar ese campo. Las listas de reuniones **sí** traen `attendees` (con `omitempty`: si no hay, la clave falta).

## Operaciones de reuniones

Todas bajo `/api/meetings`, protegidas. Un usuario sin acceso al workspace recibe `workspace not found` / `meeting not found`.

| Operación | Método y ruta | Cuerpo | Respuesta | Errores |
|---|---|---|---|---|
| Listar | `GET /api/meetings?workspace_id=<id>&from=<RFC3339>&to=<RFC3339>[&project_id=<id>]` | – | `200` array de reuniones (con `attendees`, sin `tasks`), orden `start_time ASC`; `[]` si no hay | `400 {"error":"workspace_id is required"}` [PROD]; `from`/`to` inválidos se **ignoran** en silencio [CÓDIGO]; `from`/`to` son opcionales [PROD: sin ellos responde 200] |
| Detalle | `GET /api/meetings/{id}` | – | `200` reunión (con `attendees` y `tasks`) [PROD, `tasks` ausente si no hay] | `404 {"error":"meeting not found"}` [PROD]; `400 invalid meeting id` |
| Crear | `POST /api/meetings` | `CreateMeetingRequest` (abajo) | `201` reunión con `attendees` | `400`: `title is required`, `workspace_id is required`, `end_time must be after start_time`, `workspace not found`, `project not found`, `project does not belong to this workspace`, `attendee <id> is not a member of this workspace`, `task <id> not found` |
| Editar | `PUT /api/meetings/{id}` | `UpdateMeetingRequest` (punteros: solo cambia lo enviado; `clear_project` desasocia) | `200` reunión | `400 title cannot be empty`, `end_time must be after start_time`, etc. |
| Borrar | `DELETE /api/meetings/{id}` | – | `204` sin cuerpo | `404` |
| Invitados | `PUT /api/meetings/{id}/attendees` | `{"attendee_ids":[1,2]}` (reemplaza; conserva el RSVP de los que quedan; avisa a los nuevos) | `200` reunión | `400` |
| RSVP propio | `POST /api/meetings/{id}/rsvp` | `{"status":"accepted"}` | `200` reunión | `400 invalid rsvp status` |
| Tareas vinculadas | `PUT /api/meetings/{id}/tasks` | `{"task_ids":[...]}` | `200` reunión | `400` |
| Completar | `POST /api/meetings/{id}/complete` | `{"minutes","attended_user_ids","link_task_ids","new_tasks"}` | `200` | `400 only the creator or an admin can manage this meeting`, `meeting is not scheduled` |
| Reprogramar | `POST /api/meetings/{id}/reschedule` | `{"start_time","end_time","all_day","reason"}` | `200` (reinicia RSVP y avisa) | idem |
| Cancelar | `POST /api/meetings/{id}/cancel` | `{"reason"}` | `200` (`status: cancelled`, `cancelled_at`, `cancel_reason`; avisa) | idem |

`CreateMeetingRequest` [CÓDIGO]:

```json
{
  "workspace_id": 2,
  "project_id": null,
  "title": "obligatorio",
  "description": "<p>HTML</p>",
  "agenda": "",
  "location": "",
  "start_time": "2026-12-03T15:00:00Z",
  "end_time": "2026-12-03T16:00:00Z",
  "all_day": false,
  "attendee_ids": [1, 4],
  "task_ids": []
}
```

Efectos reales al crear con invitados: notificación in-app y **email de invitación** a cada invitado (incluido el creador si se mandó), con link a la reunión. Cancelar/reprogramar avisa a los invitados salvo al actor. Un recordatorio automático (`MEETING_REMINDER_LEAD_MINUTES`, 30 por defecto). No probado (solo lectura).

### Reunión [PROD, redactada]

Tomada de `GET /api/meetings?workspace_id=2&...` (reunión completada; las `scheduled`/`cancelled` tienen la misma forma):

```json
{
  "id": 4,
  "workspace_id": 2,
  "project_id": 12,
  "project_name": "<nombre del proyecto>",
  "title": "<titulo>",
  "description": "<p>HTML</p>",
  "agenda": "",
  "location": "",
  "start_time": "2026-06-30T13:00:00Z",
  "end_time": "2026-06-30T14:30:00Z",
  "all_day": false,
  "created_by": 1,
  "created_by_name": "<nombre>",
  "reminder_sent": false,
  "status": "completed",
  "minutes": "<p>HTML del acta</p>",
  "completed_at": "2026-06-30T22:31:43.692049Z",
  "cancelled_at": null,
  "cancel_reason": "",
  "attendees": [
    {
      "user_id": 2,
      "name": "<nombre>",
      "email": "<email>",
      "rsvp_status": "accepted",
      "responded_at": "2026-06-29T22:36:16.581169Z",
      "attended": true
    }
  ],
  "created_at": "2026-06-29T22:35:36.52921Z",
  "updated_at": "2026-06-30T22:31:43.692049Z"
}
```

Notas de tipos:

- Fechas en RFC 3339 UTC (`Z`). `completed_at`/`cancelled_at`/`responded_at` son `null` si no aplican.
- `project_id` y `project_name` son `null`/ausentes sin proyecto. `description` y `minutes` son HTML.
- `tasks` (solo en el detalle, y solo si hay): `[{"id","title","status_name","is_completed"}]` [CÓDIGO; en prod el detalle de la reunión 4 no la trae porque no tiene].
- Una reunión cancelada: `status: "cancelled"`, `cancelled_at` con fecha y `cancel_reason` con el motivo (puede ser `""`).

### Workspace [PROD, redactado]

`GET /api/workspaces` → `200` array (sin paginar):

```json
{
  "id": 2,
  "user_id": 1,
  "name": "<nombre>",
  "description": "",
  "is_default": false,
  "company_name": "<empresa>",
  "company_cuit": "<cuit>",
  "company_legal_address": "<domicilio>",
  "company_logo_url": "<URL firmada, vence>",
  "created_at": "2026-05-08T16:09:59.419995Z",
  "updated_at": "2026-10-13T17:33:44.494895Z"
}
```

El usuario tiene 2 workspaces: id 1 (`is_default: true`, "Personal") e id 2. Los datos de empresa y la URL del logo no hacen falta: el cliente Go debe ignorarlos.

### Miembro [PROD, redactado]

`GET /api/workspaces/{id}/members` → `200` array de usuarios (404 `{"error": ...}` si no es miembro) [CÓDIGO]:

```json
{
  "id": 4,
  "email": "<email>",
  "name": "<nombre>",
  "role": "agent",
  "status": "",
  "hourly_rate": null,
  "created_at": "2026-06-17T13:33:53.170435Z"
}
```

- `role`: se vieron `admin`, `agent`, `auditor`. `status` viene vacío en prod (ver arriba). `hourly_rate` es sensible: ignorarlo.
- El `id` es el `user_id` que usan `attendee_ids` y `attendees[].user_id`. Para mapear un email de invitación a un `user_id` se usa `email` de esta lista.

## Ruta web de una reunión

`{frontend}/agenda?meeting={id}` con `{frontend}` = `https://taskapp.codecontrol.com.ar`.

- [CÓDIGO] El frontend (`src/App.tsx`) solo tiene `/agenda` (no existe `/meetings/:id`); `src/pages/agenda.tsx` lee el parámetro `meeting` y abre el detalle. El backend arma el mismo link para los avisos (`meetingPath`: `/agenda?meeting=%d`).
- [NO VERIFICADO] En el navegador (requiere sesión).

## Muestras iCalendar (`core/icalx/testdata/`)

**Todas son sintéticas.** Fabricio no pudo aportar `.eml` reales en este momento. Se escribieron a mano siguiendo RFC 5545/5546/6047 y el formato que mandan Google Calendar y Outlook. Cada archivo lo declara: en los `.ics`, la propiedad `X-SAMPLE:synthetic` justo después de `BEGIN:VCALENDAR`; en los `.eml`, la cabecera `X-Sample: synthetic`. Direcciones solo `example.com` (`organizador@`, `fabricio@`, `otra@`). Saltos de línea CRLF, líneas plegadas a 75 octetos.

**Pendiente cuando haya muestras reales** (conviene sumarlas aparte, con otro nombre, sin pisar estas): una invitación real de Google, una de Outlook (en especial con `TZID` de Windows y su `VTIMEZONE` real) y el ZIP de exportación de Google Calendar.

Referencia de fechas: hora de Buenos Aires = UTC−3, sin horario de verano (2026).

| Archivo | Qué es | UID | SEQUENCE | Inicio / fin esperados (UTC) |
|---|---|---|---|---|
| `google-request.ics` | `METHOD:REQUEST`, Google; organiza `organizador@`, asisten `organizador@` (ACCEPTED), `fabricio@` y `otra@` (NEEDS-ACTION). `LOCATION` con sala y link de Meet | `7q3k9d2m5x8reunionproyecto@google.com` | 1 | 2026-12-03 15:00Z – 16:00Z (12:00–13:00 ART) |
| `google-update.ics` | `METHOD:REQUEST`, mismo UID, otro horario y `LAST-MODIFIED` posterior | `7q3k9d2m5x8reunionproyecto@google.com` | 2 | 2026-12-04 17:00Z – 18:00Z (14:00–15:00 ART) |
| `google-cancel.ics` | `METHOD:CANCEL`, mismo UID, `STATUS:CANCELLED` | `7q3k9d2m5x8reunionproyecto@google.com` | 3 | 2026-12-04 17:00Z – 18:00Z |
| `outlook-request.ics` | `METHOD:REQUEST`, Outlook/Exchange; `TZID=Argentina Standard Time` con bloque `VTIMEZONE` (`STANDARD` −0300, sin DST); UID largo hexadecimal estilo Exchange; `X-MICROSOFT-CDO-*`; `VALARM` −15 min; asiste `fabricio@` | `040000008200E00074C5B7101A82E0080000000070A1B2C3D4E5F60100000000000000001000000012345678ABCDEF0123456789ABCDEF01` | 0 | local 2026-12-10 10:00–11:00 → **2026-12-10 13:00Z – 14:00Z** |
| `reply-accepted.ics` | `METHOD:REPLY` de `otra@` con `PARTSTAT=ACCEPTED`, para el evento de `google-request.ics` | `7q3k9d2m5x8reunionproyecto@google.com` | 1 | 2026-12-03 15:00Z – 16:00Z |
| `google-export.ics` | `METHOD:PUBLISH`, `X-WR-CALNAME:fabricio@example.com`, `X-WR-TIMEZONE`, `VTIMEZONE` de `America/Argentina/Buenos_Aires`; 6 `VEVENT` (abajo) | ver abajo | ver abajo | ver abajo |
| `google-request.eml` | Correo de Google: `multipart/mixed` > `multipart/alternative{text/plain (base64), text/html (quoted-printable), text/calendar; method=REQUEST (7bit)}` + adjunto `invite.ics` (`application/ics`, base64). Embebe el contenido de `google-request.ics`. Asunto con encabezado RFC 2047 | igual que `google-request.ics` | 1 | igual |
| `outlook-request.eml` | Correo de Outlook: `multipart/alternative{text/plain, text/html, text/calendar; method=REQUEST}`, todo en base64; **sin** adjunto `.ics`. Embebe el contenido de `outlook-request.ics` | igual que `outlook-request.ics` | 0 | igual |

Detalle de `google-export.ics` (todos con fechas futuras salvo el último):

| UID | Qué es | SEQUENCE | Horario esperado |
|---|---|---|---|
| `serie-semanal-daily@google.com` (evento maestro) | Serie semanal `RRULE:FREQ=WEEKLY;COUNT=10;BYDAY=MO,WE`, `DTSTART;TZID=America/Argentina/Buenos_Aires:20261123T100000` (30 min), organiza `fabricio@`, `EXDATE` 2026-11-30 10:00 ART | 0 | Ocurrencias base a las 13:00Z–13:30Z: 11-23, 11-25, ~~11-30~~ (excluida), 12-02, 12-07, 12-09, 12-14, 12-16, 12-21, 12-23. Total efectivo: **9** ocurrencias (10 menos el `EXDATE`), una de ellas modificada |
| `serie-semanal-daily@google.com` (override) | Misma UID, `RECURRENCE-ID;TZID=...:20261202T100000`, movida a las 15:00–15:30 ART, título "Daily del equipo (movida a la tarde)" | 1 | 2026-12-02 **18:00Z – 18:30Z** (reemplaza a la ocurrencia de 13:00Z) |
| `todo-el-dia-feriado@google.com` | Día completo: `DTSTART;VALUE=DATE:20261208`, `DTEND;VALUE=DATE:20261209`, `TRANSP:TRANSPARENT` | 0 | 2026-12-08 (fecha, sin hora; fin exclusivo 12-09) |
| `organiza-otro-planificacion@google.com` | Organiza `organizador@`; `fabricio@` es `ATTENDEE` con `PARTSTAT=ACCEPTED` | 0 | 2026-12-11 14:00Z – 15:00Z |
| `propio-demo-cliente@google.com` | Organiza `fabricio@`; `otra@` es `ATTENDEE` con `NEEDS-ACTION` (y `fabricio@` ACCEPTED) | 0 | 2026-12-15 19:00Z – 20:00Z |
| `pasado-evento-viejo@google.com` | **Evento pasado**, sin invitados, organiza `fabricio@` | 0 | 2025-03-10 14:00Z – 15:00Z |

Para tests que necesiten "pasado" vs "futuro" respecto del 2026-11-19: solo `pasado-evento-viejo@google.com` es pasado; el resto de las muestras caen entre el 2026-11-23 y el 2026-12-23.

## Pendiente de confirmar en la primera prueba real

1. Todas las escrituras de reuniones (cuerpos y códigos), leídas solo del código de agosto.
2. Cómo se serializa una reunión `all_day: true` (ninguna en prod).
3. Si el detalle (`GET /api/meetings/{id}`) trae `tasks` con vínculos (en prod la reunión consultada no tenía).
4. Que el email de invitación de Taskapp no se duplique si PostaControl crea la reunión además de mandar su propio correo.
5. Muestras reales de Google/Outlook y el ZIP de exportación de Google Calendar (ver arriba).

## Checklist de aceptación (con la app real)

1. [ ] Crear un evento con su Gmail como invitado. Llega la invitación, Google la muestra con los botones y aceptarla allá marca "aceptó" acá.
2. [ ] Recibir una invitación de Google y aceptarla desde PostaControl. En Google figura como aceptada.
3. [ ] Mover el evento y avisar. En Google se actualiza.
4. [ ] Borrarlo y avisar. En Google se cancela.
5. [ ] Crear una reunión de Taskapp con un externo. Aparece en Taskapp y el externo recibe el `.ics`.
6. [ ] Cambiar la hora en Taskapp. Aparece el aviso "Cambió en Taskapp" y "Avisar" le actualiza al externo.
7. [ ] Importar el ZIP de Google sin "Feriados" y verificar las repeticiones y los días completos.
8. [ ] Verificar que llega el aviso de un evento.

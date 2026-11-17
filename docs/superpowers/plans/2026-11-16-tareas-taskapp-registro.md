# Registro de la API de Taskapp (verificada)

Fecha: 2026-11-16. Rama `tareas-taskapp`. Solo lectura sobre producción.

## Cómo leer este registro

Cada dato lleva su fuente:

- **[PROD]**: observado en producción (bundle JS del frontend, `curl` sin token, o las herramientas de lectura del MCP `cc-tickets`, que devuelven el JSON crudo de la API).
- **[CÓDIGO]**: leído en la copia local del backend/frontend (`~/desarrollo_local/taskapp_web`, último commit 2026-08-26). **Está desactualizada respecto de producción**: producción devuelve campos que la copia no tiene (`close_proposal_*`, `client_summary`, `client_confirmed_*`, `verification_steps`, `status_changed_at`). Las escrituras no se pudieron probar en producción (regla: solo `GET`), así que los cuerpos y códigos de escritura son **[CÓDIGO]** y hay que tratarlos como "a confirmar en la primera prueba real".
- **[NO VERIFICADO]**: no se pudo comprobar.

Limitación importante: el sistema bloqueó leer el token de `~/.claude.json`, por lo que **no se hizo ningún `GET` autenticado directo con `curl`** a la API REST. Las respuestas autenticadas de lectura (proyectos, tareas) se obtuvieron con las herramientas de lectura del MCP `cc-tickets`. `GET /api/auth/me` y `GET /api/statuses` no se pudieron observar en producción; ver más abajo.

## URL base

| Qué | Valor | Fuente |
|---|---|---|
| URL base de la API (por defecto) | `https://apitaskapp.codecontrol.com.ar` | [PROD] constante embebida en el bundle `assets/index-*.js` del frontend; `GET /health` responde `{"status":"ok"}` 200 |
| URL base del frontend (por defecto) | `https://taskapp.codecontrol.com.ar` | [PROD] |
| Página de la tarea (source `internal`) | `{frontend}/tasks/{id}` | [PROD] rutas del bundle (`/tasks/:id`) |
| Página del ticket (source `widget`) | `{frontend}/tickets/{id}` | [PROD] rutas del bundle (`/tickets/:id`); [CÓDIGO] los listados de tickets navegan a `/tickets/{id}` |
| Login de la CLI | `{frontend}/cli-auth?cb=<url loopback>&state=<estado>` | [PROD] ruta `/cli-auth` existe; [CÓDIGO] parámetros |

Las rutas de la API son `/api/...`. Los archivos se sirven en `{api}/files/...`.

## Autenticación

- Cabecera `Authorization: Bearer <JWT>`. Sin cabecera: `401 {"error":"missing authorization header"}` [PROD].
- Respuestas de error: JSON `{"error":"<mensaje>"}` (helper `writeError`) [CÓDIGO].
- El JWT de la CLI (`mcpServers.cc-tickets` de `~/.claude.json`) lo acepta el MCP contra esta API [PROD, indirecto]; no se probó contra REST directo.

### Flujo `cli-auth` (login por navegador)

[CÓDIGO] `src/pages/cli-auth.tsx`:

- Entrada: `?cb=<URL loopback en localhost>&state=<opaco>`. Sin `cb` o `state` muestra "Solicitud inválida". Si no hay sesión redirige a `/login?returnUrl=...`.
- **Autorizar**: el frontend hace `POST /api/cli/grant-token` y redirige a `cb` con los parámetros `token`, `expires_at`, `email` y `state`.
- **Cancelar / denegar**: redirige a `cb` con `error=user_cancelled` y `state`. **El parámetro es `error` con valor `user_cancelled`** (no hay otro).
- Error al generar el token: no redirige; muestra un toast y se queda en la página.
- `cb` solo se acepta si es loopback (`localhost`/`127.0.0.1`).

### `GET /api/auth/me`

- Ruta protegida [CÓDIGO]; sin token da 401 [PROD].
- Respuesta 200 [CÓDIGO] (el objeto usuario):

```json
{
  "id": 1,
  "email": "<email>",
  "name": "<nombre>",
  "role": "<admin|member|auditor...>",
  "status": "active",
  "hourly_rate": null,
  "created_at": "2026-05-08T12:00:00Z"
}
```

- `password` nunca se serializa. El `id` es el que se usa como `assignee_id`. [NO VERIFICADO en prod con token; el `id` del usuario de Fabricio es 1 según `user_id`/`assignee_id` de sus tareas en prod.]
- Un token expirado o inválido da 401 con `{"error": ...}`.

## Proyectos

### `GET /api/projects`

- Devuelve un **array** de proyectos accesibles (propios + de workspaces donde es miembro) [CÓDIGO]. Sin paginación. Soporta `?workspace_id=` (según la herramienta MCP) [PROD, indirecto].
- Forma real de un proyecto [PROD] (valores sensibles redactados):

```json
{
  "id": 3,
  "user_id": 1,
  "workspace_id": 2,
  "name": "Transporte SP",
  "description": "",
  "status": "active",
  "deadline": null,
  "slug": null,
  "environment": "development",
  "api_key": "<REDACTADO>",
  "is_active": true,
  "icon": "zap",
  "color": "#3b82f6",
  "requires_valorization": true,
  "approver_email": "<email>",
  "approver_emails": ["<email>"],
  "kind": "client",
  "client_name": "Transporte SP",
  "priority": 8,
  "repo_url": "https://github.com/<org>/<repo>",
  "prod_url": "https://<dominio>",
  "dev_url": "https://<dominio-dev>",
  "default_assignee_id": 1,
  "testing_data": "",
  "created_at": "2026-05-08T16:42:25.387959Z",
  "updated_at": "2026-06-19T00:14:19.665171Z",
  "has_environment": true
}
```

- `status`: se vieron `"active"` y `"completed"` [PROD]. `kind`: `"client"` | `"internal"`. `color`: hex `#rrggbb`. `api_key` puede ser `null` (proyectos sin clave). `approver_emails` es siempre array (puede ser `[]`); `approver_email` es el primero o `null`. `workspace_id` es número (se vieron 1 y 2).
- **No guardar `api_key` en PostaControl**: la API la devuelve a todo miembro; el cliente Go debe ignorarla.
- Alternativa liviana [CÓDIGO]: `GET /api/cli/me/projects` devuelve `[{id,name,slug,environment,api_key,is_active,workspace_id}]` y **genera claves como efecto secundario**: no usarla.

### `GET /api/projects/{id}`, `GET /api/projects/{id}/tasks`, `GET /api/projects/{id}/assignable-users`, `GET /api/projects/{id}/members` [CÓDIGO]

Existen. `assignable-users` sirve para elegir `assignee_id` válido (la API valida que el asignado sea miembro del proyecto o del workspace).

## Estados

`GET /api/statuses` [CÓDIGO]: lista de estados; admite `?project_id=` y `?applies_to=`. Forma de un estado [PROD, embebida en cada tarea]:

```json
{
  "id": 1, "name": "Pendiente", "slug": "pending", "color": "#f59e0b",
  "icon": "circle-dashed", "position": 0, "applies_to": "both",
  "is_global": true, "is_default": true, "is_completed": false,
  "triggers_resolved_email": false, "is_client_visible": false,
  "created_at": "2026-05-08T12:08:36.077093Z"
}
```

- `applies_to`: `"task"` | `"ticket"` | `"both"`.
- **Completada = `status.is_completed == true`** [PROD]. Ejemplo: id 6 `resolved` ("Resuelto", `applies_to: "ticket"`, `is_completed: true`). Los estados son editables desde Admin: no cablear ids ni slugs, usar `is_completed`.
- Estado inicial de una tarea nueva: `pending` (id 1); de un ticket interno: `open` degradando a `pending` [CÓDIGO].

## Tareas

### Listar: `GET /api/tasks`

- Devuelve un **array JSON** de tareas (no un objeto envoltorio) [CÓDIGO]. Orden `created_at DESC`.
- Parámetros de query [CÓDIGO]: `assignee_id`, `project_id`, `workspace_id`, `status_id`, `priority` (`none|low|medium|high|urgent`), `due_before`, `overdue=true`, `unassigned=true`, `label_id`, `feature_id`, `favorite=true`, `has_links=true`, `link_type`, `source` (`widget` para tickets, `internal` para tareas), `include_subtasks=true`, paginación opt-in `limit` y `offset` (con `limit` responde cabecera `X-Total-Count`).
- **Asignado a mí: `?assignee_id=<id de /api/auth/me>`**. Existe `mine=true` solo en la herramienta MCP, no en la REST [CÓDIGO: la REST no lo define].
- **Los completados NO se excluyen**: la REST no filtra por estado completado; hay que descartar en el cliente los de `status.is_completed == true` [CÓDIGO; coherente con que la herramienta MCP aplica su propio filtro `state=pending` por encima]. [PROD] el MCP confirma que hay tareas completadas asignadas a Fabricio (`state=completed` devuelve 180).
- **Los archivados SÍ se excluyen por defecto** (`t.archived_at IS NULL`); los archivados tienen su propia ruta `GET /api/tasks/archived` [CÓDIGO].
- Cada tarea trae `status` embebido (objeto completo, ver arriba) [PROD], por lo que no hace falta llamar a `/api/statuses`.
- Las subtareas no aparecen salvo `include_subtasks=true` [CÓDIGO].

### Detalle: `GET /api/tasks/{id}`

Objeto de tarea; 404 `{"error":"task not found"}` si no existe o no hay acceso [CÓDIGO]. Forma real [PROD] (campos de la lista y del detalle coinciden; el MCP `get_task` agrega `project_environment`, que no es de la REST):

```json
{
  "id": 842,
  "user_id": 1,
  "workspace_id": 2,
  "project_id": 21,
  "title": "Funcionalidad Necesaria",
  "description": "<p>HTML enriquecido</p>",
  "status_id": 1,
  "status": { "id": 1, "name": "Pendiente", "slug": "pending", "is_completed": false, "...": "..." },
  "priority": "urgent",
  "due_date": null,
  "source": "internal",
  "origin": "external",
  "reporter_email": null,
  "reporter_name": null,
  "assignee_id": 2,
  "assignee_name": "<nombre>",
  "creator_name": "<nombre>",
  "feature_id": null,
  "attachments": [],
  "resolved_at": null,
  "valorized_at": null,
  "estimated_hours": null,
  "dev_hours": null,
  "claimed_at": null,
  "settled_at": null,
  "proposed_to": null, "proposed_by": null, "proposed_hours": null, "proposed_at": null,
  "acceptance_criteria": null,
  "analysis_notes": null,
  "completion_summary": null,
  "verification_steps": null,
  "client_summary": null,
  "client_confirmed_at": null, "client_confirmed_by": null,
  "reopen_status": null, "reopen_reason": null, "reopen_attachments": [],
  "reopen_requested_at": null, "reopen_requested_by": null,
  "close_proposal_status": null, "close_proposal_target": null, "close_proposal_reason": null,
  "close_proposal_requested_at": null, "close_proposal_requested_by": null,
  "close_proposal_declined_at": null, "close_proposal_declined_reason": null,
  "is_blocked": false, "block_reason": null, "blocked_at": null, "blocked_by": null,
  "labels": [],
  "is_favorite": false,
  "archived_at": null,
  "created_at": "2026-11-15T13:00:39.941051Z",
  "updated_at": "2026-11-15T13:00:47.121473Z",
  "status_changed_at": "2026-11-15T13:00:39.941051Z"
}
```

Notas de tipos:

- Fechas: RFC 3339 UTC con microsegundos (`2026-11-15T13:00:39.941051Z`). `due_date` se vio `null`; al escribir se acepta `time.Time` (RFC 3339) [CÓDIGO] y el formato en el que viene al leerse con fecha **no se observó** (todas las tareas vistas tenían `null`) [NO VERIFICADO].
- `description`: **HTML** (`<p>...</p>`), no texto plano ni markdown [PROD]. Los tickets que vienen del widget público pueden traer texto plano con saltos de línea [CÓDIGO, comentario en `rich-content.tsx`].
- `attachments`: array de **URLs firmadas** (`{api}/files/<key>?exp=<unix>&sig=<hex>`) con vencimiento (la firma que devuelve la lista dura ~15 minutos según el código; en `completion_summary` aparecen firmas largas). No son las keys crudas [PROD].
- `priority`: `none|low|medium|high|urgent`.
- Un campo puede faltar o ser `null`; el cliente debe tolerar ambos.

### Cómo se distingue un ticket de una tarea

- **`source`**: `"widget"` = ticket; `"internal"` = tarea [PROD y CÓDIGO]. El frontend lista tickets con `GET /api/tasks?source=widget` [CÓDIGO `src/api/tickets.ts`].
- `origin` (`"external"` | `"internal"`) indica de dónde vino. Una tarea `source=internal` puede tener `origin=external` (se vio en prod, tarea 842), así que **no usar `origin` para distinguir**.
- Un ticket cargado por el widget público: `source=widget`, `origin=external`, con `reporter_email`/`reporter_name` [PROD, tarea 840]. Un ticket interno (`POST /api/tickets`): `source=widget`, `origin=internal`, con el email/nombre del usuario como reportante [CÓDIGO].
- La URL del frontend sigue a `source`: `widget` -> `/tickets/{id}`, `internal` -> `/tasks/{id}`.

### Crear tarea: `POST /api/tasks`

Cuerpo [CÓDIGO]:

```json
{
  "title": "obligatorio",
  "description": "<p>HTML</p>",
  "workspace_id": 2,
  "project_id": 21,
  "priority": "medium",
  "due_date": "2026-12-01T00:00:00Z",
  "assignee_id": 1,
  "acceptance_criteria": "texto",
  "label_ids": [],
  "estimated_hours": 3
}
```

- Obligatorios: `title` y **`workspace_id`** (distinto de 0; el usuario debe ser dueño o miembro). Si se manda `project_id`, el servidor **reemplaza** el workspace por el del proyecto, y si no se manda `assignee_id` se asigna **al responsable por defecto del proyecto** (`default_assignee_id`).
- `priority` por defecto `none`; valor inválido -> 400 con `invalid priority: ... (valid: none, low, medium, high, urgent)`.
- `estimated_hours > 0` + sin asignar = la tarea entra al "banco de tareas". `0` se guarda como nulo.
- **Sin `due_date` es válido** (campo opcional).
- Éxito: **`201 Created`**, cuerpo = el objeto tarea (misma forma que `GET /api/tasks/{id}`, con `source: "internal"`, `attachments: []`, `labels: []`).
- Errores: `400 {"error": "title is required" | "workspace_id is required" | "workspace not found" | "project not found" | "assignee is not a member of the project" | ...}`.
- **Efecto real**: dispara eventos en tiempo real y notificaciones (email/Telegram/campanita) al asignado. No probado.

### Crear ticket interno: `POST /api/tickets`

Cuerpo [CÓDIGO] (`CreateInternalTicketRequest`):

```json
{
  "title": "obligatorio",
  "description": "texto",
  "priority": "medium",
  "project_id": 3,
  "acceptance_criteria": "texto"
}
```

- **No acepta `due_date`**, ni `assignee_id`, ni `workspace_id`, ni adjuntos (campos desconocidos se ignoran). Confirmado: el modelo no tiene `due_date`.
- Obligatorio: `title` (400 `title is required`). `priority` por defecto `medium`. El workspace se deduce del proyecto; sin proyecto usa el workspace por defecto del usuario.
- Estado inicial `open` (degrada a `pending`). Se crea con `source: "widget"`, `origin: "internal"`, `reporter_email`/`reporter_name` = usuario autenticado, `attachments: []`.
- Éxito: **`201 Created`**, cuerpo = objeto tarea. Rol `auditor` es solo lectura (403).
- **Efecto real**: notifica a aprobadores/equipo del proyecto. No probado.

### Actualizar: `PUT /api/tasks/{id}`

Cuerpo [CÓDIGO]: todos los campos son punteros/opcionales; solo se cambia lo que se manda (**actualización parcial**):

```json
{
  "title": "...", "description": "<p>HTML</p>", "status_id": 3, "project_id": 3,
  "clear_project": false, "priority": "high", "due_date": "...", "clear_due_date": false,
  "workspace_id": 2, "assignee_id": 1, "clear_assignee": false, "feature_id": 7,
  "clear_feature": false, "acceptance_criteria": "...", "analysis_notes": "...",
  "completion_summary": "<p>HTML</p>", "estimated_hours": 2,
  "attachments": ["internal/<uuid>.png"]
}
```

- **Adjuntar un archivo subido**: no hay ruta dedicada para el usuario autenticado; se hace con `PUT /api/tasks/{id}` con `attachments` = **lista completa** (reemplaza). `attachments` ausente = no se toca; `[]` = se borran todos. El servidor normaliza cada elemento con `stripSignedURL`, así que se puede reenviar la lista leída (URLs firmadas) mezclada con la `key` nueva (`internal/<uuid>.<ext>`). Para añadir sin pisar: hacer `GET`, tomar `attachments`, anexar la key nueva y hacer `PUT`.
- **La descripción se actualiza igual**: `PUT` con `{"description": "<p>...</p>"}`.
- Éxito: `200` con la tarea actualizada [CÓDIGO]. Errores: `400` (validación o `invalid task id`), `409` (bloqueada por tareas abiertas; o error al reclamar).
- Cuidado: mandar `assignee_id` de otra persona en una tarea con `estimated_hours` no asigna: crea una **propuesta** (`proposed_to`). Asignarse a uno mismo la "reclama" (`claimed_at`).
- Existen además `POST /api/tasks/{id}/archive`, `/unarchive`, `/claim` y otras; fuera de alcance.

### Comentarios

`GET /api/tasks/{id}/comments` [CÓDIGO] devuelve un árbol (`replies`) de comentarios.

`POST /api/tasks/{id}/comments` [CÓDIGO]:

```json
{
  "body": "texto o HTML",
  "is_internal": false,
  "parent_id": null,
  "attachments": ["internal/<uuid>.png"]
}
```

- **Sí acepta `attachments`** (keys crudas o URLs firmadas; se normalizan) **e `is_internal`** (por defecto `false`) y `parent_id` (respuesta).
- `body` puede estar vacío si hay adjuntos; si no hay ninguno de los dos: `400 {"error":"body is required"}`.
- **Tarea completada**: `400 {"error":"cannot comment on a completed task"}` (la condición es `status.is_completed`). Tarea inexistente o sin acceso: `404 {"error":"task not found"}`. Cuerpo inválido: `400 {"error":"invalid request body"}`.
- Éxito: **`201 Created`** con el comentario (adjuntos ya como URLs firmadas):

```json
{
  "id": 1, "task_id": 842, "author_id": 1, "author_email": "<email>", "author_name": "<nombre>",
  "body": "...", "is_internal": false, "parent_id": null,
  "attachments": [], "created_at": "2026-11-16T12:00:00Z"
}
```

- **Efecto real**: notifica por email/Telegram/SSE (incluido el reportante externo si el comentario no es interno). `is_internal: true` evita avisar al cliente.

### Subida de archivos: `POST /api/upload`

[CÓDIGO]:

- `multipart/form-data`, **campo `file`** (único). Sin el campo: `400 {"error":"missing file"}`.
- **Límite 20 MB** (`20 << 20`, `NewUploadHandler(storage, 0)` usa el valor por defecto). Excedido: `413 {"error":"file too large"}`.
- Cualquier tipo de archivo. Key = `internal/<uuid>.<ext>` (extensión del nombre original, `bin` si no tiene).
- Éxito: **`201 Created`** con `{"key": "internal/<uuid>.<ext>", "url": "<URL firmada, 365 días>"}`.
- Para adjuntar a una tarea o comentario se usa la `key` en `attachments` (ver arriba).
- Servir: `GET /files/*` con URL firmada (`exp`, `sig`) [CÓDIGO].

## Resumen de códigos de estado

| Operación | Éxito | Errores vistos en el código |
|---|---|---|
| `GET /api/auth/me` | 200 | 401 |
| `GET /api/projects` | 200 | 401 |
| `GET /api/tasks` | 200 | 400 (parámetro inválido), 401 |
| `GET /api/tasks/{id}` | 200 | 404 |
| `POST /api/tasks` | 201 | 400 |
| `POST /api/tickets` | 201 | 400, 403 (auditor), 500 |
| `PUT /api/tasks/{id}` | 200 | 400, 404, 409 |
| `POST /api/tasks/{id}/comments` | 201 | 400, 404 |
| `POST /api/upload` | 201 | 400, 413 |

## Pendiente de confirmar en la primera prueba real

1. Forma exacta de `GET /api/auth/me` y `GET /api/statuses` con token (se infirió del código).
2. Códigos y cuerpos de todas las escrituras (solo leídos del código de agosto; producción es más nueva).
3. Formato de `due_date` cuando tiene valor.
4. Que `GET /api/tasks?assignee_id=<id>` devuelva también las completadas (el código no las filtra).
5. Si producción agregó una ruta dedicada para adjuntar (el MCP tiene `add_task_attachment`/`attach_uploaded_file`, que podrían usar `PUT /api/tasks/{id}` por debajo).

## Prueba de aceptación

Pendiente: se completa con Fabricio. La prueba manual no escribe en Taskapp.

### Prueba manual (sin escribir en Taskapp)

- [ ] Crear, editar y marcar una tarea local.
- [ ] Recibir un aviso con una tarea a 1 minuto y anticipación «A la hora», con la ventana cerrada en la bandeja.
- [ ] El clic en el aviso abre la tarea.
- [ ] Conectar Taskapp y ver los pendientes.
- [ ] El diálogo de crear precarga bien desde un correo de un aprobador conocido.
- [ ] Cancelar sin crear.

### Aceptación con Fabricio

- [ ] Crea la primera tarea real desde la app.
- [ ] Suma una respuesta del hilo a esa tarea.
- [ ] Resultado anotado (códigos y cuerpos de las escrituras, para cerrar los puntos de «Pendiente de confirmar»).

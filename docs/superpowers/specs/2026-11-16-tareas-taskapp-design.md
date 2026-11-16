# Tareas y vinculación con Taskapp — diseño

Fecha: 2026-11-16

## Intención

Los requerimientos de trabajo llegan por correo, y el objetivo es que
**Taskapp sea la única fuente de trabajo** del equipo. PostaControl pasa a ser
la puerta de entrada: desde un correo se crea en un paso la tarea (o el
ticket) en el proyecto que corresponde de Taskapp, con el correo original
adjunto.

Además, PostaControl tiene **tareas propias (locales)** para lo que no es
trabajo de un proyecto, por ejemplo una reunión comercial. No compiten con
Taskapp: cubren lo que Taskapp no tiene por qué saber.

Este es el primero de tres proyectos (tareas → calendario → mejoras de
contactos). El calendario sale del no-objetivo del diseño original y se
diseña aparte; este diseño solo deja preparadas las tareas locales con fecha
y hora para que aparezcan en él.

## Objetivos

1. **Crear en Taskapp desde un correo:** tarea o ticket, con proyecto
   preseleccionado cuando hay una coincidencia clara, y el `.eml` adjunto.
2. **Sumar un correo a una tarea existente:** cuando el cliente responde en el
   mismo hilo, un botón agrega ese correo como comentario (con el `.eml`) a la
   tarea ya vinculada.
3. **Ver las tareas de Taskapp** asignadas a mí y pendientes, de solo lectura,
   con acceso directo a Taskapp.
4. **Tareas locales** con título, notas, fecha y hora opcionales, correo de
   origen opcional y marca de hecha.
5. **Avisos** de tareas locales con hora, por notificación del escritorio, con
   anticipación configurable.

## No-objetivos

- Gestionar tareas de Taskapp desde PostaControl: cambiar estado, resolver,
  reasignar, comentar libremente o editar. Eso se hace en Taskapp. La única
  escritura es crear (objetivo 1) y sumar un correo (objetivo 2).
- Calendario (proyecto siguiente).
- Sincronizar tareas locales con Taskapp ni con otros dispositivos.
- Usar el MCP `cc-tickets` o el CLI de Taskapp como canal.
- Varios usuarios o varias cuentas de Taskapp a la vez.

## Anti-objetivos

- **Inferir un proyecto que no tiene sentido.** Si no hay coincidencia clara,
  el proyecto queda vacío. Una preselección equivocada es peor que ninguna.
- **Convertirse en un segundo Taskapp.** Si la vista de tareas empieza a
  necesitar estados, tableros o filtros de Taskapp, es señal de que se fue de
  alcance.
- **Perder el requerimiento.** Una tarea no se crea sin proyecto, y el correo
  original siempre queda en Taskapp.

## Restricciones

- Núcleo en Go (`core/`) sin imports de Wails; frontend React + TS + shadcn.
  Se respeta la arquitectura existente (`arch_test.go`).
- Contraseñas y tokens solo en el llavero del sistema (`core/secrets`).
- La base local es SQLite con migraciones versionadas (`core/store/schema_vN.sql`,
  hoy la última es la v16). Las tareas locales necesitan una nueva versión,
  y las notas de la versión deben avisarlo (una versión anterior no la abre).
- Las tareas locales viven en SQLite como fuente de verdad, igual que los
  contactos, y **entran en el respaldo** (`core/mail/backup.go`).
- Taskapp es un servicio en la red: todo lo que dependa de él tiene que
  degradar bien sin conexión.

## Approaches (cómo hablarle a Taskapp)

Se consideraron tres:

1. **API REST de Taskapp — elegida.** El núcleo llama a los mismos endpoints
   que usan el frontend y el CLI de Taskapp (`/api/tasks`, `/api/tickets`,
   `/api/tasks/{id}/comments`, subida de archivos, `/api/auth/me`,
   `/api/cli/me/projects` o `/api/projects`). Autenticación con
   `Authorization: Bearer <JWT>`; el token "purpose: cli" pasa por el mismo
   `middleware.Auth`. JSON con tipos claros, fácil de probar con un servidor
   falso. Riesgo: no es una API pública documentada; si cambia, hay que
   ajustar PostaControl. Fabricio mantiene ambos, así que el riesgo es
   controlado.
2. **MCP `cc-tickets` por HTTP.** Ya funciona y tiene atajos (subir y
   adjuntar en un paso), pero sus respuestas están pensadas para un modelo
   (resúmenes compactos, texto dentro de JSON) y pueden cambiar de formato.
   Suma una capa de protocolo sin valor para una app. Descartada.
3. **Invocar el CLI de Taskapp.** Exige Node instalado y es lento. Descartada.

Elegida la 1 por Fabricio.

**Verificación previa obligatoria:** la copia local del backend
(`~/desarrollo_local/taskapp_web/taskApp-backend`) es de agosto de 2026. Antes
de construir el cliente hay que confirmar contra producción
(`https://taskapp.codecontrol.com.ar`, o el host real de la API) los
endpoints y formas de: listar tareas (filtros por asignado y estado),
crear tarea, crear ticket interno, comentar, adjuntar un archivo a una
tarea/ticket, y la URL de una tarea en el frontend (`/tasks/:id` en la copia
local). En la copia local, adjuntar es `POST /api/upload` (devuelve `key`,
límite 20 MB) y después `PUT /api/tasks/{id}` con la lista completa de
`attachments`; `POST /api/tasks/{id}/comments` acepta `attachments` e
`is_internal`. Los tickets internos no aceptan `due_date` al crearse.

## Diseño

### Conexión (Ajustes → Taskapp)

- **Conectar (decisión de Fabricio: mismo flujo que la CLI).** Botón
  "Conectar con Taskapp": PostaControl levanta un servidor HTTP efímero en
  `127.0.0.1` (puerto libre), abre en el navegador
  `<frontend>/cli-auth?cb=http://127.0.0.1:<puerto>/…&state=<aleatorio>`, y
  recibe el token que devuelve `POST /api/cli/grant-token` (vigencia 1 año).
  Valida `state`, cierra el servidor al recibirlo o a los pocos minutos sin
  respuesta, y guarda el token en el llavero. Reproducir exactamente lo que
  hace `taskApp-frontend/src/pages/cli-auth.tsx` y el CLI.
- La página de Taskapp dice "Autorizar TaskApp CLI". Cambiar ese texto es un
  ajuste opcional en Taskapp, **fuera de este proyecto**: todo el trabajo es
  en PostaControl.
- Ajustes muestra con qué usuario quedó conectado (`/api/auth/me`), la URL
  base de la API (con valor por defecto, editable) y "Desconectar", que borra
  el token del llavero.
- Sin token configurado, todo lo de Taskapp queda oculto o deshabilitado con
  un enlace a Ajustes; las tareas locales funcionan igual.
- Token vencido o rechazado (401): aviso no invasivo "Reconectar Taskapp" que
  repite el flujo de conexión. No se reintenta en bucle.

### Pruebas contra Taskapp *(decisión de Claude)*

- Durante el desarrollo, contra producción **solo lecturas** (verificar
  endpoints y formas). Las escrituras (crear tarea/ticket, comentar, subir y
  adjuntar) se prueban contra un servidor falso en los tests, armado con las
  formas verificadas: escribir en producción dispara mails y Telegram reales.
- La primera escritura real la hace Fabricio desde la app, como prueba de
  aceptación.

### Crear en Taskapp desde un correo

Acción en el lector (botón y menú del correo; atajo de teclado libre a
elección del builder, verificando que no choque con `lib/keys.ts`).

```
┌ Crear en Taskapp ─────────────────────────────┐
│ ( • Tarea   ○ Ticket )                        │
│ Proyecto   [ Sinergia                    ▾ ]  │
│ Título     [ Error al exportar informes     ] │
│ Descripción  (cuerpo del correo, editable)    │
│ Prioridad  [ Ninguna ▾ ]  Vence [ ——/——/—— ]  │
│ 📎 Se adjunta el correo original (.eml)       │
│                      [Cancelar]  [Crear]      │
└───────────────────────────────────────────────┘
```

- **Tarea o ticket:** se elige cada vez; arranca en Tarea. El ticket usa el
  endpoint de tickets internos (reporter = usuario autenticado).
- **Proyecto:** lista de proyectos activos a los que tengo acceso, con su
  color. Preselección:
  1. Dirección exacta del remitente entre los `approver_emails` de los
     proyectos.
  2. Si no hay, dominio del remitente contra los dominios de esos
     `approver_emails`, **excluyendo dominios públicos** (gmail.com,
     hotmail.com, outlook.com, yahoo.*, live.com, icloud.com y similares;
     lista mantenida en el código).
  3. Si coinciden dos o más proyectos, o ninguno: vacío.
- **Tarea sin proyecto no se puede crear** (botón deshabilitado con el motivo).
  Un ticket sí puede ir sin proyecto. El `workspace_id` de la tarea sale del
  proyecto elegido.
- **Título:** asunto sin prefijos de respuesta/reenvío (`Re:`, `RE:`, `RV:`,
  `Fwd:`, `FW:`, repetidos). **Descripción:** cuerpo del correo, editable,
  convertido a HTML limpio (Taskapp colapsa texto plano en un párrafo), con
  una línea de cabecera "De: … · Fecha: …".
- **Prioridad** (none/low/medium/high/urgent, por defecto none) y
  **vencimiento** opcional. *(Decisión de Claude)* "Vence" solo se muestra
  para Tarea: los tickets internos no lo aceptan al crearse y no vale un
  segundo paso que pueda fallar.
- **Asignado:** no se elige; lo resuelve Taskapp con el responsable por
  defecto del proyecto.
- **Adjunto:** se sube el `.eml` del correo (la app ya sabe guardar `.eml`)
  y se adjunta a la tarea/ticket creado. *(Decisión de Claude)* Si el `.eml`
  supera el límite de subida de Taskapp (20 MB), se sube un `.eml` sin los
  adjuntos (encabezados y cuerpo) y la descripción lista los nombres y tamaños
  de los adjuntos que quedaron afuera; el diálogo lo avisa antes de crear. Así
  el requerimiento siempre queda en Taskapp.
- **Orden y fallos:** primero se crea la tarea; después se adjunta. Si el
  adjunto falla, la tarea queda creada y vinculada, y se avisa con opción de
  reintentar solo el adjunto. Nunca se duplica la tarea por reintentar.
- **Vínculo local:** se guarda en SQLite (id de tarea, tipo, proyecto, título,
  ids del mensaje y raíz del hilo). En el lector aparece un chip
  "Tarea #123 · Sinergia" que abre la tarea en el navegador. El chip se
  muestra en **toda la conversación** del hilo vinculado.

### Sumar un correo a la tarea

- Si el hilo tiene una tarea vinculada y el correo abierto todavía no se
  sumó, aparece "Sumar a la tarea #123". Agrega un comentario con el cuerpo
  del correo (mismo formato que la descripción) y adjunta su `.eml`.
- Se recuerda qué correos ya se sumaron para no ofrecerlo dos veces.
- Si el hilo tiene más de una tarea vinculada, se elige a cuál sumar.
- *(Decisión de Claude)* Los comentarios van siempre como **internos**
  (`is_internal: true`): el `.eml` puede traer reenvíos internos del hilo y no
  debe quedar visible para el cliente en un ticket.
- *(Decisión de Claude)* Taskapp no acepta comentarios en tareas completadas.
  Antes de ofrecer el botón se consulta el estado de la tarea vinculada; si
  está completada, el botón queda deshabilitado con el motivo ("La tarea #123
  está completada") y se ofrece "Crear tarea nueva", que abre el diálogo de
  creación y vincula la tarea nueva al mismo hilo. Reabrir sigue fuera de
  alcance.

### Vista de Tareas

Nueva vista en el sidebar junto a Contactos (también en la columna de íconos
del sidebar plegado), con el mismo patrón: lista al centro, detalle a la
derecha.

```
┌ Sidebar ─┬ Tareas ─────────────────────────┬ Detalle ──────────────┐
│ ✉ Cuentas│ [+ Nueva]   [Todas ▾]           │ Reunión comercial     │
│ 👤 Contac│ ── Vencidas ──                   │ con Rucso             │
│ ☑ Tareas │ ● #412 Exportar informes  Sinerg│ Jue 19/11 · 10:00     │
│          │ ── Hoy ──                        │ Notas…                │
│          │ ○ Reunión comercial Rucso  Local │ ✉ Correo de origen    │
│          │ ● #398 Alta de usuarios   Ecokit │                       │
│          │ ── Sin fecha ──                  │ [Hecha] [Editar]      │
│          │ ○ Llamar al contador       Local │                       │
└──────────┴─────────────────────────────────┴───────────────────────┘
```

- **Una sola lista**, las dos fuentes mezcladas, agrupadas por vencimiento:
  Vencidas, Hoy, Mañana, Esta semana, Más adelante, Sin fecha. Cada fila
  indica su origen (proyecto con su color, o "Local").
- **Filtro:** Todas / Taskapp / Locales.
- **Taskapp:** pendientes asignadas a mí (tareas y tickets). *(Decisión de
  Claude)* "Pendiente" = todo lo que no está en un estado con
  `is_completed` ni archivado (incluye en curso, en revisión y bloqueadas),
  igual que el criterio `state=pending` del MCP. Cada fila muestra el nombre
  de su estado. Detalle de solo
  lectura: título, proyecto, estado, prioridad, vencimiento, descripción y
  "Abrir en Taskapp". Se refresca al abrir la vista y periódicamente mientras
  está abierta (intervalo a elección del builder). Se guarda la última copia
  en SQLite: sin conexión se muestra esa copia con "Actualizado hace X".
- **Locales:** crear (botón "Nueva" o desde un correo, ver abajo), editar,
  marcar hecha, borrar. Las hechas se ocultan por defecto, con opción de
  verlas.

### Tareas locales

- Campos: título (obligatorio), notas (texto), fecha opcional, hora opcional
  (solo si hay fecha), anticipación del aviso (solo si hay hora), correo de
  origen opcional (enlace que abre el correo), hecha (con fecha en que se
  marcó).
- **Desde un correo:** la misma acción "Crear tarea" del lector ofrece como
  destino "PostaControl (local)" además de Taskapp; precarga título y vincula
  el correo de origen. Para locales no se adjunta nada ni se copia el cuerpo:
  el enlace al correo alcanza.
- Entran en el respaldo manual y automático, y en su restauración.
- Pensadas para el calendario: una tarea local con fecha y hora es lo que el
  calendario mostrará más adelante. No se diseña nada más del calendario acá.

### Avisos

- Cada tarea local con hora avisa con una notificación del escritorio.
  Anticipación elegible por tarea (por ejemplo: a la hora, 5, 15, 30 min,
  1 h, 1 día); por defecto toma la de Ajustes (15 min por defecto).
- Funciona con la ventana cerrada (la app sigue en la bandeja). Clic en la
  notificación abre la tarea.
- Un aviso se dispara una sola vez. Si la app estaba cerrada a la hora del
  aviso, al arrancar se muestran los avisos vencidos de las últimas 24 h que
  no se mostraron; los más viejos no.
- Cambiar fecha, hora o anticipación rearma el aviso. Marcar hecha o borrar lo
  cancela.
- Mecanismo: un ciclo en segundo plano como el de envíos programados y
  correos pospuestos (`core/mail/scheduler.go`, `snooze.go`) y las
  notificaciones existentes (`app/notify.go`).

## Lo que queda al builder

- Forma exacta del cliente REST y sus tipos, tras verificar contra producción.
- Esquema SQLite (tareas locales, vínculos correo↔tarea, caché de tareas de
  Taskapp, avisos mostrados) y número de versión.
- Intervalo de refresco de la lista de Taskapp y del ciclo de avisos.
- Atajo de teclado de "Crear tarea" y ubicación exacta de botones en el lector.
- Lista exacta de dominios públicos excluidos.
- Conversión de cuerpo de correo a HTML para Taskapp (se puede reutilizar
  `core/htmlx`).

# Calendario — diseño

Fecha: 2026-11-18

## Intención

Fabricio hoy maneja la agenda entre Google Calendar y la memoria, y se le
pasan cosas. Quiere **gestionar todo en PostaControl**: el correo, las tareas
y ahora el calendario, en una sola app. PostaControl pasa a ser **su
calendario**. Google Calendar deja de usarse como agenda: lo que tiene ahí se
importa una vez y no se sincroniza.

Asume la consecuencia: hasta que exista una app móvil, el calendario no está
en el celular.

Como con las tareas, las reuniones de trabajo también pueden vivir en
**Taskapp**, que tiene reuniones propias con asistentes, confirmación, minuta
y tareas. PostaControl las muestra junto a los eventos locales y permite
crearlas.

Es el segundo de tres proyectos (tareas → **calendario** → mejoras de
contactos). Las tareas ya están hechas (v0.8.x): las locales con fecha y hora
se pensaron para aparecer acá.

## Objetivos

1. **Calendario local** con eventos de día completo u horarios, lugar, notas,
   invitados y repetición. Vista por defecto: **semana laboral con horario**,
   con el detalle a la derecha. También vistas Día, Semana completa, Mes y
   Agenda.
2. **Avisos** de los eventos (locales y de Taskapp) por notificación del
   escritorio, con la anticipación configurable. Mismo mecanismo que las
   tareas.
3. **Invitaciones que llegan por correo** (`.ics` de Google, Outlook o
   cualquiera): Aceptar / Tal vez / Rechazar desde el correo. Al responder,
   el evento entra (o se marca) en el calendario y la respuesta le llega al
   organizador por correo, desde la cuenta que recibió la invitación. Las
   actualizaciones y cancelaciones posteriores del organizador se aplican al
   evento.
4. **Invitaciones que se mandan**: un evento local con invitados les manda
   por correo la invitación `.ics`, desde la cuenta que se elija. Sus
   respuestas marcan quién confirmó. Editar o cancelar el evento les manda la
   actualización o la cancelación.
5. **Tareas locales con fecha** aparecen en el calendario, sin duplicarse.
6. **Reuniones de Taskapp**:
   - Se ven en el calendario: las del usuario, como asistente o creador.
   - Se pueden crear desde PostaControl, con proyecto opcional y asistentes
     entre los miembros del workspace.
   - Sin conexión se muestra la última copia.
7. **Clientes en reuniones de Taskapp**: a una reunión de Taskapp se le pueden
   agregar invitados externos por correo. PostaControl les manda la
   invitación `.ics` y registra sus respuestas, igual que en el objetivo 4.
8. **Importar una vez** el calendario de Google desde el `.ics` que exporta
   Google, para no arrancar de cero.

## No-objetivos

- Sincronización con Google Calendar u otro servidor (CalDAV, API de Google,
  OAuth).
- App móvil o servidor de calendario propio.
- Gestionar reuniones de Taskapp desde PostaControl más allá de crearlas: la
  minuta, las tareas vinculadas, reprogramar, cancelar y completar se hacen
  en Taskapp. El detalle se ve y se abre en Taskapp.
- Varios calendarios por usuario o calendarios compartidos.
- Exportar o publicar el calendario.

## Anti-objetivos

- **Una tercera agenda que mantener.** El calendario tiene que ser donde
  Fabricio *crea* cosas, no un espejo de solo lectura.
- **Responder mal a un organizador.** Una respuesta que Google u Outlook no
  reconoce, o que sale desde la cuenta equivocada, deja la reunión sin
  confirmar del otro lado. Ese es el fallo que más se nota.
- **Mandar invitaciones de más.** Editar un evento sin cambiar nada que les
  importe a los invitados (por ejemplo, las notas privadas o el aviso) no les
  manda correo.
- **Duplicar.** La misma invitación procesada dos veces, o la misma tarea o
  reunión mostrada dos veces, nunca genera dos eventos.

## Restricciones

- Mismo stack y arquitectura: núcleo Go en `core/` sin Wails
  (`arch_test.go`), cáscara en `app/`, UI React + shadcn.
- **SQLite es la fuente de verdad** de los eventos locales, como de los
  contactos y las tareas. Migración nueva (la base está en v17). Las notas de
  la versión avisan que una versión anterior no la abre. Los eventos entran
  en el respaldo.
- **Estándares de invitación**: iCalendar (RFC 5545) e iTIP/iMIP (RFC 5546,
  6047) con los métodos REQUEST, REPLY y CANCEL. El correo de invitación es
  `multipart/alternative` con un `text/calendar; method=…`, más el `.ics`
  adjunto, como hacen Google y Outlook.
- Zona horaria: se guarda en UTC más el TZID del evento. Se muestra en la
  zona local. Los eventos de día completo son fechas sin hora.
- Taskapp por su API REST, con la conexión ya existente (token en el
  llavero), el `taskappErr`, la traba de 401 y la caché. Copia local del
  backend (puede estar vieja):
  - `/api/meetings`: `GET` con `workspace_id`, `from`, `to` y `project_id`; `POST` con `CreateMeetingRequest` (`workspace_id`, `project_id`, `title`, `description`, `agenda`, `location`, `start_time`, `end_time`, `all_day`, `attendee_ids`, `task_ids`).
  - Asistentes con estado de confirmación.
  - `/api/workspaces/{id}/members` para elegir asistentes.

## Approaches

Se consideraron:

1. **Modelo propio en SQLite, más el `.ics` original de cada invitación
   recibida — elegida.** Los eventos van en tablas normales, así las
   consultas por rango son simples y rápidas. Para las invitaciones recibidas
   se guarda el VEVENT original (UID, SEQUENCE, ORGANIZER, ATTENDEE), que
   hace falta para responder bien y aplicar los cambios del organizador. Se
   leen y arman con `github.com/emersion/go-ical`, del mismo autor que
   go-message e IMAP. Las repeticiones se expanden con una librería de RRULE
   (por ejemplo `github.com/teambition/rrule-go`).
2. **Guardar solo el `.ics` de cada evento y parsearlo al leer.** Es el más
   fiel al original, pero cada vista tiene que parsear y expandir todo, y
   filtrar se complica. Descartada.
3. **Servidor CalDAV embebido.** Prepararía la sincronización móvil, pero
   suma mucho y el móvil quedó para después. Descartada. El modelo de la 1 se
   puede exponer más adelante.

Grilla de UI: **FullCalendar** (React, núcleo y plugins daygrid, timegrid,
list e interaction, todos con licencia MIT), con los estilos de la app.
Resuelve arrastrar, estirar la duración y los solapamientos.

Fabricio eligió la 1.

## Diseño

### Vista Calendario

Entrada nueva en el sidebar y en la columna plegada, al lado de Tareas.

```
┌ Sidebar ─┬ [Hoy] ‹ ›  16 – 20 nov 2026     [Día|Semana|Mes|Agenda] [+ Nuevo] ┬ Detalle ─────────┐
│ ✉ …      │      Lun 16   Mar 17   Mié 18   Jue 19        Vie 20             │ Reunión comercial│
│ ☑ Tareas │ 09  Daily    Daily    Daily    Daily         Daily  (Taskapp)    │ Jue 19 10–11     │
│ 📅 Cal.  │ 10           Contador Demo ?   Reunión com.                      │ 📍 Oficina Rucso │
│          │ 11  Llamar…                                  Retro               │ 👥 1/2 confirmó  │
│          │ 15           Kickoff  Médico                                     │ [Editar]         │
└──────────┴──────────────────────────────────────────────────────────────────┴──────────────────┘
```

- **Vistas.** La de arranque es la semana laboral (lunes a viernes) con
  horario. Se cambia a Día, Semana completa, Mes y Agenda, y la elección se
  recuerda.
- **Colores.** Evento local en un color propio. Reunión de Taskapp con el
  color del proyecto, o neutro si no tiene. Tarea con hora en gris punteado.
  Invitación sin responder con borde naranja.
- **Interacción.**
  - Clic en un hueco: crear un evento con esa hora.
  - Arrastrar o estirar un evento local: moverlo o cambiar su duración. Si
    tiene invitados, se pregunta "¿Avisar a los invitados?".
  - Las reuniones de Taskapp, las invitaciones recibidas y las tareas no se
    mueven desde la grilla.
- **Detalle a la derecha** según el tipo:
  - Evento local: datos, invitados con su estado, Editar y Borrar.
  - Invitación recibida: datos, organizador y Aceptar / Tal vez / Rechazar.
  - Reunión de Taskapp: datos, asistentes, "Abrir en Taskapp".
  - Tarea: abre la tarea en la vista Tareas.

### Crear y editar un evento

Diálogo con el destino arriba, como el de crear tarea: **Calendario local** o
**Reunión en Taskapp**. Este último solo aparece con Taskapp conectado.

- **Comunes:** título, día completo o inicio y fin, lugar y notas.
- **Descripción y notas privadas** *(decisión de Claude)*: son dos campos
  separados. La **descripción** va en la invitación (por ejemplo, el link de
  la videollamada) y, en Taskapp, en la `description` de la reunión.
  Cambiarla cuenta como cambio que les importa a los invitados. Las **notas
  privadas** quedan solo en esta PC y nunca salen.
- **Solo local:**
  - Repetición: ninguna, diaria, semanal (días), mensual o anual, con fin
    opcional.
  - Aviso: la anticipación del evento, o el valor por defecto de Ajustes.
  - Invitados: correos con autocompletado de contactos.
  - "Enviar desde": cuenta. Por defecto, la de la vista abierta.
- **Solo Taskapp:**
  - Proyecto, opcional.
  - Asistentes: miembros del workspace del proyecto. Sin proyecto, se elige
    el workspace; el selector solo aparece si hay más de uno *(decisión de
    Claude)*.
  - Invitados externos por correo y cuenta de envío, como en el local. Solo
    se agregan **al crear la reunión desde PostaControl** *(decisión de
    Claude)*. A las reuniones creadas en la web de Taskapp no se les agregan
    externos desde acá.
- **Al guardar con invitados** se mandan las invitaciones. Al editar algo que
  les importa (hora, lugar, título, repetición, invitados) se manda una
  actualización, con la SEQUENCE incrementada. Al borrar se manda una
  cancelación. Las notas y el aviso no disparan correo.
- **Repeticiones:** al editar o borrar una ocurrencia se pregunta "Solo este"
  o "Este y los siguientes". "Todos" queda como caso de "este y los
  siguientes" desde la primera.

### Invitaciones recibidas

- **Detección:** un correo con `text/calendar` o un adjunto `.ics` con
  METHOD=REQUEST, CANCEL o REPLY. Reemplaza a la `InviteCard` actual, que
  solo deja guardar el `.ics`.
- **REQUEST nuevo:** tarjeta con Aceptar / Tal vez / Rechazar. Antes de
  responder, el evento aparece en el calendario como "sin responder".
  - Al responder se manda un REPLY al ORGANIZER, desde la cuenta que recibió
    el correo, con el PARTSTAT de esa dirección.
  - Rechazar lo saca del calendario y deja el registro, para que una
    actualización no lo resucite sin aviso.
- **REQUEST con la misma UID y SEQUENCE mayor:** se actualiza el evento. Si
  cambió la hora, se marca "cambió, revisá" y se vuelve a pedir la respuesta.
- **CANCEL:** el evento se marca cancelado. Se ve tachado y después se puede
  quitar.
- **REPLY a un evento propio:** se actualiza el estado de ese invitado.
- **Cuenta que responde** *(decisión de Claude)*: la cuenta cuya dirección
  coincide con el ATTENDEE. Si ninguna coincide (llegó a un alias o a una
  lista), responde la cuenta que recibió el correo y se avisa.
- **Envío** *(decisión de Claude)*: las invitaciones, respuestas y
  cancelaciones salen por la bandeja de salida existente, con reintento y con
  los segundos para deshacer el envío.
- **Primera vez:** se procesan las invitaciones de correos que ya están en la
  caché local y cuyo evento todavía no terminó.
- **Idempotencia:** procesar dos veces el mismo correo (por UID, SEQUENCE y
  remitente) no cambia nada. Las invitaciones se procesan al abrir el correo
  y también cuando llega correo nuevo, para que las respuestas se registren
  solas.

### Reuniones de Taskapp

- **Qué se muestra:** las reuniones del rango visible de los workspaces del
  usuario en las que es asistente o creador.
- **Cómo se trae:** `GET /api/meetings?workspace_id&from&to`, filtrando del
  lado del cliente. Se guarda una caché local por rango. Sin conexión se
  muestra la copia, con la hora de actualización.
- **Crear:** `POST /api/meetings`. Taskapp avisa a sus asistentes por su
  lado. Los invitados externos reciben el `.ics` desde PostaControl, con la
  misma UID que el evento-espejo local que guarda sus respuestas.
- **Desde PostaControl** no se edita ni se cancela. Si se borra el
  evento-espejo de los externos, se les manda una cancelación y se avisa que
  la reunión sigue en Taskapp.
- **Cambios hechos en Taskapp** *(decisión de Fabricio: avisar y confirmar)*.
  El espejo guarda una foto de lo que se les mandó a los externos (título,
  inicio y fin, lugar, descripción y estado). En cada refresco se compara con
  la reunión de Taskapp:
  - Si cambió la hora, el lugar, el título o la descripción, o si la reunión
    se canceló o se borró, el evento muestra "Cambió en Taskapp: ¿avisar a
    los invitados externos?".
  - Con un clic se manda la actualización (SEQUENCE+1) o la cancelación.
    "Ahora no" descarta el aviso hasta el próximo cambio.
  - Nunca se manda sin que Fabricio lo confirme. Si el cambio pasó con la app
    cerrada, el aviso aparece en el siguiente refresco.

### Avisos

Con el mismo ciclo y la misma marca de "avisado" que las tareas:

- Eventos locales, con su anticipación o la de Ajustes (nuevo valor por
  defecto para eventos, 15 min).
- Reuniones de Taskapp, con el valor de Ajustes.
- Las invitaciones sin responder también avisan.
- Las ocurrencias de un evento repetido avisan cada una.
- La ventana de 24 h para avisos vencidos al arrancar es la misma que para
  las tareas.

### Importar de Google

En Ajustes → Calendario, "Importar .ics": se eligen uno o más archivos. Google
exporta un ZIP con un `.ics` por calendario, y Fabricio elige cuáles importar
(por ejemplo, el principal y no los feriados) *(decisión de Claude)*.

- Se conserva la UID de cada evento, así importar dos veces no duplica.
- Las repeticiones y los días completos se conservan.
- No se manda ningún correo al importar.
- **Eventos organizados por otro** *(decisión de Fabricio)*: si el ORGANIZER
  no es una de las direcciones de las cuentas, el evento entra como
  invitación recibida.
  - Guarda el VEVENT original, el organizador, la SEQUENCE y la respuesta que
    ya se había dado (PARTSTAT).
  - Si el organizador manda después un cambio con la misma UID, se aplica
    sobre ese evento.
- **Eventos organizados por Fabricio** *(decisión de Fabricio)*: entran como
  eventos locales con sus invitados.
  - Editarlos o moverlos pregunta siempre "¿Avisar a los invitados?" antes de
    mandar nada, porque esas personas ya los tienen en su calendario con esa
    UID.
  - Si avisa, la actualización sale con la SEQUENCE siguiente a la importada.

## Lo que queda al builder

- Esquema SQLite (eventos, ocurrencias modificadas o excepciones, invitados,
  VEVENT original, caché de Taskapp, marca de avisos) y versión.
- La librería de RRULE concreta.
- La integración de FullCalendar con el tema de la app.
- Las rutas exactas de la API de Taskapp, verificadas como en las tareas
  (leer producción sí, escribir solo contra un servidor falso).
- El formato exacto del correo iMIP. Probarlo contra lo que Google y Outlook
  esperan, con muestras reales en testdata.

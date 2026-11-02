# PostaControl — Fase 5A "Selección múltiple y posición del panel de lectura" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Fabricio pueda poner el panel de lectura a la derecha, abajo (por defecto) o arriba, y marcar varios correos para leerlos, destacarlos, moverlos o borrarlos de una vez, incluso arrastrándolos a una carpeta.

**Architecture:**
- **Posición del panel:** es una preferencia nueva en `model.Settings` (`readingPane`), que se guarda en la fila JSON de `settings` y no necesita migración.
  - `MailShell` arma dos grupos redimensionables anidados: el panel izquierdo y el resto, y adentro lista y lector en horizontal (derecha) o vertical (abajo, arriba).
  - Las filas de la lista se compactan a un renglón cuando el lector está abajo o arriba.
- **Selección:** es estado de `MailShell` (`ShellState.marked`), porque la usan la lista (marca y barra), el lector (aviso de "N seleccionados") y el panel izquierdo (soltar al arrastrar).
  - La lógica de marcar (Ctrl, Shift, todos) es pura, en `lib/selection.ts`.
- **Acciones sobre la selección:** reusan `SetSeen`, `SetFlagged`, `MoveMessages` y `DeleteMessages`, que ya reciben listas de ids pero exigen **una sola cuenta**.
  - El frontend agrupa por cuenta y llama una vez por cuenta (`lib/bulk.ts`). Así resuelve la falla parcial que pide el spec sin tocar el núcleo.

**Tech Stack:** el de siempre. No se suman dependencias; arrastrar y soltar usa HTML5 nativo.

**Spec:** `docs/superpowers/specs/2026-11-01-carpetas-lectura-seleccion-design.md`, secciones "Panel de lectura" y "Selección múltiple", y las "Decisiones mías" 2, 3 y 4. Spec base: `docs/superpowers/specs/2026-10-30-postacontrol-design.md`.

## Global Constraints

- **Siguen vigentes las reglas del proyecto:**
  - `core/` sin Wails; tests solo con `core/testenv`.
  - Español rioplatense con voseo.
  - `task check` verde antes de cada commit.
  - Commits convencionales en español que terminan con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Bindings solo con `task common:generate:bindings`.
  - Nunca leer ni imprimir `~/.npmrc`.
- **Rama:** `fase-5`, desde `main`. La 5B sigue en la misma rama.
- **Escrituras en el servidor:** solo las que dispara Fabricio con una acción; nunca un `EXPUNGE` sin UID (el núcleo ya lo cumple).
- **Valores de `readingPane`:** `"right"`, `"bottom"` y `"top"`.
  - `""` (cero, configuración vieja) se lee como `"bottom"`. Es la "Decisión mía" 4: al actualizar, el panel arranca abajo.
  - Cualquier otro valor en `SetSettings` es un `UserError{Code: "invalid"}` con el mensaje "Posición del panel de lectura desconocida."
- **Textos exactos:**
  - Configuración:
    - etiqueta "Panel de lectura" y descripción "Dónde se muestra el correo abierto.";
    - opciones "Derecha", "Abajo", "Arriba".
  - Barra de selección:
    - "<N> seleccionados", o "<N> conversaciones seleccionadas" en la vista de conversaciones;
    - botones "Marcar leído", "Marcar no leído", "Destacar" ("Quitar destacado" si ya están todos destacados), "Mover a…", "Eliminar" ("Eliminar definitivamente" si están todos en la Papelera);
    - botón con `aria-label="Quitar selección"` (ícono ✕).
  - Lector con 2 o más marcados: "<N> correos seleccionados" o "<N> conversaciones seleccionadas", y abajo "Arrastralos a una carpeta o usá la barra de arriba."
  - Casilla de cada fila: `aria-label="Seleccionar"`.
  - Confirmación de borrado definitivo:
    - título "Eliminar definitivamente";
    - todos en la Papelera: "¿Eliminar definitivamente <N> correos? No se puede deshacer.";
    - mezcla: "<K> de los <N> correos están en la Papelera y se eliminan definitivamente. Los demás van a la Papelera. No se puede deshacer.";
    - botones "Cancelar" y "Eliminar".
  - Toasts:
    - movidos: "<N> movidos a <carpeta>";
    - borrados (fuera de la Papelera): "<N> movidos a la Papelera";
    - falla por cuenta: "No se pudo en <nombre o correo de la cuenta>: <error>". Se muestra uno por cuenta que falló.

## Review Focus

1. **Un clic común, sin teclas, tiene que seguir abriendo el correo como hoy y soltar la selección.** Cualquier regresión acá molesta todo el día. Lo cubre el test de la Task 3 "un clic común abre y suelta la selección".
2. **Eliminar desde la búsqueda con correos de la Papelera mezclados.** Los de la Papelera se borrarían para siempre sin aviso, porque `DeleteMessages` expurga lo que ya está en la Papelera. Siempre se confirma. Lo cubre el test de la Task 4 "mezcla con la Papelera pide confirmación".
3. **Conversación marcada que incluye respuestas de Enviados.** Solo se tocan los correos de la carpeta que se está viendo. Lo cubre el test de la Task 4 "una conversación marcada no toca lo de Enviados".
4. **Falla en una cuenta de la bandeja unificada:**
   - lo de las otras cuentas se hace;
   - sale un toast por la cuenta que falló;
   - sus correos quedan marcados.
   Lo cubre el test de la Task 4 "falla parcial deja marcados los de esa cuenta".
5. **Cambiar de carpeta, buscar o prender Conversaciones con correos marcados.** La selección se suelta: si no, una acción tocaría correos que ya no se ven. Lo cubre el test de la Task 3 "cambiar de vista suelta la selección".

---

### Task 1: Preferencia `readingPane` y selector en Configuración

**Files:**
- Modify: `core/model/model.go` (`Settings`), `core/mail/settings.go`, `core/mail/settings_test.go`
- Create: `frontend/src/lib/settings.ts`, `frontend/src/lib/settings.test.ts`
- Modify: `frontend/src/features/settings/SettingsDialog.tsx`, `frontend/src/features/settings/SettingsDialog.test.tsx`, `frontend/src/features/messages/MessageList.tsx` (usa el helper)
- Regenerar bindings.

**Interfaces:**
- Produces:
  - `model.Settings.ReadingPane string \`json:"readingPane"\``;
  - en TS, `type ReadingPane = "right" | "bottom" | "top"`;
  - `readingPane(s: Settings | undefined): ReadingPane`, con `""`/`undefined` → `"bottom"`;
  - `useReadingPane(): ReadingPane` en `lib/settings.ts`, que lee `useSettings()`;
  - `saveSettings(current: Settings | undefined, patch: Partial<Settings>): Promise<void>`. Arma la configuración completa con valores cero para lo que falte, llama `svc.SetSettings` e invalida `["settings"]` con el `queryClient` que recibe. Firma exacta: `saveSettings(queryClient, current, patch)`.

- [ ] **Step 1: Tests del núcleo** en `settings_test.go`:
  - `TestSetSettings_ReadingPane` guarda `"top"` y lo lee igual;
  - `TestSetSettings_ReadingPaneInvalid`: `"left"` devuelve `UserError` con `Code == "invalid"` y no cambia lo guardado.
- [ ] **Step 2:** Correr `go test ./core/mail/ -run ReadingPane`. Esperado: FAIL.
- [ ] **Step 3: Implementar** el campo y la validación en `Core.SetSettings`. Se aceptan `""`, `"right"`, `"bottom"` y `"top"`.
- [ ] **Step 4:** Correr el test. Esperado: PASS. Después correr `task common:generate:bindings`.
- [ ] **Step 5: Tests del frontend:**
  - `settings.test.ts`: `readingPane(undefined)` y `readingPane({...readingPane: ""})` dan `"bottom"`; `"right"` da `"right"`;
  - `SettingsDialog.test.tsx`: elegir "Derecha" en el selector "Panel de lectura" llama `SetSettings` con `readingPane: "right"` y conserva `unifiedInbox`, `disableNotifications` y `threadedList`.
- [ ] **Step 6: Implementar** `lib/settings.ts` y el selector (shadcn `Select`) en `SettingsDialog`, después de "Iniciar al abrir sesión".
  - Las tres escrituras actuales de `SettingsDialog` y la de `MessageList.setThreaded` pasan a usar `saveSettings`. Así un campo nuevo no se pierde por un objeto armado a mano.
- [ ] **Step 7:** Correr `task check`. Esperado: verde. Commit `feat(config): elegir dónde va el panel de lectura`.

### Task 2: Paneles según la posición y filas de un renglón

**Files:**
- Modify: `frontend/src/features/layout/MailShell.tsx`, `frontend/src/features/layout/MailShell.test.tsx`
- Modify: `frontend/src/features/messages/MessageRow.tsx`, `frontend/src/features/messages/ThreadList.tsx` (`ThreadRow`), `frontend/src/features/messages/MessageList.test.tsx`

**Interfaces:**
- Consumes: `useReadingPane()` (Task 1).
- Produces:
  - `MessageRow` y `ThreadRow` leen la posición con `useReadingPane()`. Con `"bottom"` o `"top"` se dibujan en un renglón.
  - El marcado `data-density="compact"` o `data-density="comfortable"` en el `<li>` sirve para los tests.

- [ ] **Step 1: Tests:**
  - `MailShell.test.tsx` "con el lector abajo, la lista va antes que el lector en un grupo vertical": con settings `readingPane: "bottom"`, el panel `[data-panel-id="list"]` precede a `[data-panel-id="reader"]` (o el atributo equivalente de `ResizablePanel`), y su grupo tiene orientación vertical.
  - Lo mismo con `"top"`, donde el lector va primero.
  - Con `"right"`, el grupo es horizontal.
  - `MessageList.test.tsx` "con el lector abajo, las filas son de un renglón": `data-density="compact"`, y siguen estando remitente, asunto, fecha y el botón "Destacar".
- [ ] **Step 2:** Correr los tests. Esperado: FAIL.
- [ ] **Step 3: Implementar en `MailShell`.**
  - **Grupo exterior:** horizontal, `{ sidebar, main }`.
  - **Grupo interior:** `{ list, reader }`.
    - Orientación: horizontal con `"right"` y vertical con `"bottom"` o `"top"`.
    - Con `"top"` se dibuja primero el lector.
  - **Tamaños:** se guardan en `localStorage` con la clave `pc:layout:<posición>`, con la forma `{ outer: {sidebar, main}, inner: {list, reader} }`.
    - Por defecto, el exterior es `{sidebar: 18, main: 82}`.
    - El interior es `{list: 36, reader: 64}` con `"right"` y `{list: 40, reader: 60}` abajo o arriba.
    - La clave vieja `pc:layout` se ignora.
  - **Mínimos:** se conservan `minSize` (`sidebar` 14%). En vertical, la lista y el lector tienen 20% como mínimo.
  - **Cambio de posición:** al cambiar, el grupo interior se vuelve a montar (`key` = posición) para tomar sus tamaños.
- [ ] **Step 4: Implementar la variante compacta** de `MessageRow` y `ThreadRow`, en una línea:
  - punto de color;
  - remitente o participantes, con ancho fijo `w-[28%]` y truncado (en negrita si no está leído);
  - contador de la conversación;
  - asunto y " — " con la vista previa en gris, en `flex-1` truncado;
  - etiqueta de carpeta si viene (`folderLabel`), en `text-xs`;
  - adjunto;
  - fecha;
  - estrella.
  La variante de tres renglones queda igual.
- [ ] **Step 5:** Correr `task check`. Esperado: verde. Commit `feat(layout): panel de lectura a la derecha, abajo o arriba`.

### Task 3: Marcar varios correos

**Files:**
- Create: `frontend/src/lib/selection.ts`, `frontend/src/lib/selection.test.ts`
- Modify:
  - `frontend/src/features/layout/MailShell.tsx` (estado)
  - `frontend/src/features/messages/MessageRow.tsx`, `frontend/src/features/messages/ThreadList.tsx`, `frontend/src/features/messages/MessageList.tsx`, `frontend/src/features/search/SearchResults.tsx`
  - `frontend/src/App.tsx` (lector con varios marcados)
  - tests: `MessageList.test.tsx`, `ThreadList.test.tsx`, `SearchResults.test.tsx`

**Interfaces:**
- Produces, en `lib/selection.ts`:
  - `type Marks<K> = { keys: K[]; anchor: K | null }`;
  - `const NO_MARKS: Marks<never>`;
  - `toggleMark<K>(m: Marks<K>, k: K, open: K | null): Marks<K>`. Ctrl + clic: si no hay nada marcado y hay uno abierto distinto de `k`, el abierto también queda marcado. El ancla pasa a ser `k`.
  - `rangeMark<K>(m: Marks<K>, k: K, order: K[], open: K | null): Marks<K>`. Shift + clic marca de `anchor` (o del abierto, o de `k` si no hay ninguno) hasta `k`, según `order`, y reemplaza lo marcado. El ancla no cambia.
  - `markAll<K>(order: K[]): Marks<K>`.
- Produces, en `ShellState`:
  - `marked: Marks<number>` (ids de mensaje en las listas planas y en la búsqueda);
  - `markedThreads: Marks<string>` (raíces, en la vista de conversaciones);
  - `setMarked(m: Marks<number>)`, `setMarkedThreads(m: Marks<string>)` y `clearMarks()`.
  - `selectView`, `setSearch`, `selectMessage(id != null)`, `selectThread(root != null)` y `openContact` sueltan las dos selecciones.
  - `MessageList.setThreaded` llama a `clearMarks()`.
- Produces, en las filas:
  - `onSelect(id, e: { ctrl: boolean; shift: boolean })`, donde `ctrl` es `e.ctrlKey || e.metaKey`;
  - la prop `marked: boolean`;
  - la casilla, que llama a `onToggle(id)`.
  Lo mismo para `ThreadRow`, con `root`.

- [ ] **Step 1: Tests puros** en `selection.test.ts`:
  - `toggleMark` con un abierto marca los dos;
  - sacar uno de dos deja uno;
  - `rangeMark` de 2 a 4 sobre `[1,2,3,4,5]` da `[2,3,4]`, y al revés también;
  - `rangeMark` sin ancla parte del abierto;
  - `markAll`.
- [ ] **Step 2: Tests de componentes:**
  - **"Ctrl + clic marca y la barra muestra la cantidad":** con el correo 1 abierto, Ctrl + clic en el 3 da "2 seleccionados".
  - **"Shift + clic marca el rango".**
  - **"Ctrl + A marca todos los cargados"**, con el foco en la lista.
  - **"la casilla marca sin abrir":** no llama a `GetMessage`.
  - **"un clic común abre y suelta la selección"** (Review Focus 1): con 2 marcados, un clic en otro llama `selectMessage(id)` y no hay barra.
  - **"Esc suelta la selección"**, con el foco dentro de la lista.
  - **"cambiar de vista suelta la selección"** (Review Focus 5).
  - **En conversaciones:** Ctrl + clic en dos hilos da "2 conversaciones seleccionadas".
  - **Lector:** con 2 marcados, `App` muestra "2 correos seleccionados" en lugar del correo.
  - **Un solo marcado** (por ejemplo, después de sacar uno de dos): se abre ese correo en el lector y no hay barra.
- [ ] **Step 3:** Correr los tests. Esperado: FAIL.
- [ ] **Step 4: Implementar.**
  - **Casilla:** está siempre en el DOM, pero en la variante de tres renglones es invisible sin hover. Usa `opacity-0 group-hover:opacity-100`, y queda visible si la fila está marcada o hay algo marcado.
  - **Fila marcada:** `bg-primary/10`.
  - **Ctrl + A** se maneja en `onKeyDown` del contenedor con scroll de la lista (`tabIndex={-1}`, con foco al hacer clic). Hace `preventDefault`.
  - **La barra** de esta tarea es solo la cantidad y la ✕ (componente `SelectionBar` en `features/messages/SelectionBar.tsx`). Los botones de acción llegan en la Task 4.
  - **Con 1 marcado** se abre ese con `selectMessage` o `selectThread`, que suelta las marcas.
- [ ] **Step 5:** Correr `task check`. Esperado: verde. Commit `feat(lista): marcar varios correos con Ctrl, Shift y casillas`.

### Task 4: Acciones sobre la selección

**Files:**
- Create: `frontend/src/lib/bulk.ts`, `frontend/src/lib/bulk.test.ts`, `frontend/src/features/messages/SelectionBar.test.tsx`
- Modify: `frontend/src/features/messages/SelectionBar.tsx`, `frontend/src/features/messages/MessageList.tsx`, `frontend/src/features/reader/MessageActions.tsx` (usa `invalidateMailQueries`)

**Interfaces:**
- Consumes: `ShellState.marked`, `markedThreads` y `setMarked` (Task 3); `svc.GetThread(folderId, root)`.
- Produces, en `lib/bulk.ts`:
  - `invalidateMailQueries(queryClient)`: las invalidaciones de `MessageActions.refresh`, más `["message"]`;
  - `type Target = { id: number; accountId: number; folderId: number; seen: boolean; flagged: boolean }`;
  - `runPerAccount(targets: Target[], fn: (accountId: number, ids: number[]) => Promise<void>): Promise<{ failed: { accountId: number; ids: number[]; error: unknown }[] }>`. Agrupa por cuenta, llama una vez por cuenta en secuencia y nunca rechaza;
  - `threadTargets(folderId: number, roots: string[]): Promise<Target[]>`. Llama `GetThread` por cada raíz y deja solo los mensajes con `folderId` igual a la carpeta vista (Decisión mía 2).

- [ ] **Step 1: Tests de `bulk.test.ts`:**
  - `runPerAccount` con dos cuentas llama dos veces con los ids de cada una;
  - si la segunda falla, devuelve sus ids en `failed` y la primera igual se hizo;
  - `threadTargets` descarta los mensajes de otra carpeta.
- [ ] **Step 2: Tests de `SelectionBar.test.tsx`:**
  - **"Marcar leído llama SetSeen(ids, true) y suelta"**; lo mismo con "Marcar no leído".
  - **"Destacar con todos destacados dice Quitar destacado".**
  - **"Mover a… lista las carpetas seleccionables de la cuenta salvo la actual y mueve":** toast "2 movidos a Clientes".
  - **"Mover a… no aparece si son de dos cuentas".**
  - **"Eliminar fuera de la Papelera no pregunta":** `DeleteMessages` y toast "2 movidos a la Papelera".
  - **"todos en la Papelera: Eliminar definitivamente pide confirmación".**
  - **"mezcla con la Papelera pide confirmación"** (Review Focus 2): texto exacto de mezcla, con K y N.
  - **"una conversación marcada no toca lo de Enviados"** (Review Focus 3): `GetThread` devuelve uno de la Entrada y uno de Enviados, y `DeleteMessages` recibe solo el de la Entrada.
  - **"falla parcial deja marcados los de esa cuenta"** (Review Focus 4): toast "No se pudo en <cuenta>: …", y `setMarked` queda con los ids que fallaron.
- [ ] **Step 3:** Correr los tests. Esperado: FAIL.
- [ ] **Step 4: Implementar.**
  - **Lista plana y búsqueda:** los `Target` salen de los items ya cargados en la lista. `SearchResults` le pasa los suyos a la barra; conviene que `MessageList` y `SearchResults` le den a `SelectionBar` un `targets: () => Promise<Target[]>`.
  - **Conversaciones:** los `Target` salen de `threadTargets`.
  - **Papelera:** un correo está "en la Papelera" si su carpeta tiene `role === "trash"`, según `useAllFolders`.
  - **Mover a…:** solo si todos son de una cuenta. Ofrece las carpetas seleccionables de esa cuenta salvo la carpeta vista, con `folderLabel`.
  - **Después de cada acción:**
    - se llama a `invalidateMailQueries`;
    - quedan marcados solo los ids que fallaron;
    - si se movió o se borró lo abierto, se cierra el lector.
  - **Cuenta en el toast de falla:** se nombra con `displayName || email`.
- [ ] **Step 5:** Correr `task check`. Esperado: verde. Commit `feat(lista): leer, destacar, mover y borrar varios correos a la vez`.

### Task 5: Arrastrar correos a una carpeta

**Files:**
- Create: `frontend/src/lib/dnd.ts`, `frontend/src/lib/dnd.test.ts`
- Modify:
  - `frontend/src/features/messages/MessageRow.tsx`, `frontend/src/features/messages/ThreadList.tsx`
  - `frontend/src/features/folders/AccountSection.tsx` (`FolderRow` como zona para soltar)
  - `frontend/src/features/folders/FolderSidebar.test.tsx`

**Interfaces:**
- Consumes: `ShellState.marked` y `markedThreads`; `runPerAccount` e `invalidateMailQueries` (Task 4).
- Produces, en `lib/dnd.ts` (la 5B lo extiende para carpetas):
  - `MSG_MIME = "application/x-postacontrol-messages"`;
  - `accountMime(accountId: number): string`, que devuelve `"application/x-postacontrol-account-" + id`. En `dragover` no se pueden leer los datos, solo los tipos: la cuenta viaja en el tipo;
  - `setMessageDrag(dt: DataTransfer, accountId: number, payload: { ids?: number[]; folderId?: number; roots?: string[] })`;
  - `readMessageDrag(dt: DataTransfer): { ids?: number[]; folderId?: number; roots?: string[] } | null`;
  - `dragAccount(dt: DataTransfer): number | null`, que lee el tipo.

- [ ] **Step 1: Tests:**
  - `dnd.test.ts`: ida y vuelta de `setMessageDrag`/`readMessageDrag`, y `dragAccount`.
  - `FolderSidebar.test.tsx`, con `fireEvent.dragOver` y `drop` y un `DataTransfer` falso:
    - **"soltar correos en una carpeta los mueve":** `MoveMessages(ids, folderId)` y toast "2 movidos a <carpeta>";
    - **"soltar en una carpeta de otra cuenta no hace nada":** sin `preventDefault` en el `dragover`, sin llamada;
    - **"soltar en la misma carpeta no hace nada".**
- [ ] **Step 2:** Correr los tests. Esperado: FAIL.
- [ ] **Step 3: Implementar.**
  - **Qué se arrastra:**
    - si la fila está marcada, van todos los marcados;
    - si no, solo esa fila, sin cambiar la selección;
    - una conversación arrastrada lleva `roots` y `folderId`, y al soltar se resuelve con `threadTargets`.
    - Si los marcados son de varias cuentas no se arrastra nada: `dragstart` hace `preventDefault`.
  - **Zona para soltar:** es la fila de una carpeta seleccionable de la misma cuenta, distinta de la de origen.
    - Mientras está encima, la fila lleva `outline outline-2 outline-primary bg-primary/10`.
  - **Al soltar:** `MoveMessages`, el toast y la actualización de la selección son iguales a los de "Mover a…" en la Task 4. Conviene reusar una función `moveTargets(targets, folder)` de `bulk.ts`.
- [ ] **Step 4:** Correr `task check`. Esperado: verde. Commit `feat(carpetas): arrastrar correos a una carpeta`.

### Task 6: Prueba manual de Fabricio

No se despacha. Fabricio la hace con `task install:desktop`:
- **Posición del panel:** arranca abajo; se cambia a la derecha y arriba en Configuración; los tamaños se recuerdan por posición.
- **Marcar:** con Ctrl, Shift, Ctrl + A y las casillas, en una carpeta, en la unificada, en la búsqueda y en conversaciones.
- **Acciones:** cada botón de la barra; borrar en la Papelera y una mezcla desde la búsqueda.
- **Arrastrar:** uno y varios correos a una carpeta.

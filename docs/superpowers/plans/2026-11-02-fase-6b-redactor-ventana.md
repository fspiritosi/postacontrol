# Fase 6B: redactor en ventana propia. Plan de implementación

> **Para agentes:** SUB-SKILL REQUERIDA: usar superpowers:subagent-driven-development (recomendada) o superpowers:executing-plans para implementar este plan tarea por tarea. Los pasos usan casillas (`- [ ]`) para el seguimiento.

**Goal:** "Redactar", "Responder", "Responder a todos", "Reenviar" y abrir un borrador o un correo de la Bandeja de salida abren una ventana del sistema aparte, que se puede mover, estirar y maximizar. La ventana principal sigue usable, y nunca se pierde lo escrito.

**Architecture:**
- **En `app/`:** un registro de ventanas de redactor (`ComposeWindows`).
  - Crea cada ventana con `app.Window.NewWithOptions` en la URL `/#/compose/<id>`.
  - Guarda el `ComposerInit` como JSON opaco, para que la ventana lo pida al cargar.
  - Deduplica por clave: `draft:<id>` o `outbox:<id>`.
  - Intercepta el cierre de cada ventana con el hook `WindowClosing`.
  - Coordina la salida de la app: les pide a todas las ventanas que guarden, y espera hasta 10 s.
  - La lógica del registro está detrás de una interfaz chica de ventana, para poder probarla sin Wails.
- **En el frontend:** `main.tsx` decide por `location.hash` si monta la app o la ventana del redactor.
  - `ComposerProvider` ya no dibuja el redactor: le pide a Go que abra una ventana.
  - `Composer` pasa de `Dialog` a un layout de ventana completa.

**Tech Stack:** Go 1.27 + Wails v3.0.0 (`github.com/wailsapp/wails/v3/pkg/application`, `.../pkg/events`), runtime JS `@wailsio/runtime` 3.0.0 (`Events`, `Window`), React 19 + Tailwind v4 + shadcn, Vitest/jsdom.

**Spec:** `docs/superpowers/specs/2026-11-02-fondo-noleidos-redactor-visor-design.md`, sección 3 "Redactor en ventana propia".

## Global Constraints

- **Núcleo:** `core/` nunca importa Wails. La ventana es asunto de `app/` y del frontend.
- **Antes de cada commit:** `task check` en verde (desde la raíz). Bindings solo con `task common:generate:bindings`.
- **Commits:** convencionales, en español, terminados en `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`. Sin prettier: no reformatear archivos enteros. No leer ni imprimir `~/.npmrc`.
- **Textos del spec y del plan, exactos:**
  - **Título de la ventana:** el asunto, o si no hay asunto, el título actual del redactor según el modo (`TITLES[mode]` en `Composer.tsx`: "Nuevo correo", "Responder", "Responder a todos", "Reenviar").
  - **Una ventana de la Bandeja de salida con cambios que bloquea la salida:** "Tenés cambios sin enviar en este correo de la Bandeja de salida. Envialo o descartá los cambios antes de salir.".
  - **Una ventana que no pudo guardar al salir:** "No pudimos guardar este borrador; la app sigue abierta." y después el error.
- **Valores:**
  - tamaño inicial de la ventana: 900×720; tamaño mínimo: 640×480;
  - espera máxima de la salida: 10 s.
- **Eventos (nombres exactos):**
  - de Go a una ventana: `compose:close-request`, `compose:flush` y `compose:quit-blocked` (data: el mensaje, un string);
  - de Go a todas las ventanas: `compose:send-started` (data: el id del correo de la bandeja de salida);
  - `files-staged` ahora va solo a la ventana donde se soltaron los archivos.
- **Cerrar un redactor** (✕ de la ventana o Esc) hace lo mismo que `requestClose()` hoy: guarda si hay autoguardado, o pregunta "¿Descartar este correo?". Si se elige "Cancelar", la ventana sigue abierta.
- **Al salir de la app** no se descarta nada:
  - un redactor vacío o sin tocar se cierra;
  - uno con autoguardado guarda sin preguntar;
  - un reintento de la Bandeja de salida con cambios bloquea la salida.

## Review Focus

1. **Cerrar con la ✕ mientras está abierto el diálogo "¿Descartar este correo?":** un segundo `compose:close-request` no abre otro diálogo ni cierra la ventana. `requestClose` ya ignora mientras `sending || closing`; con el diálogo abierto tampoco hace nada. Test en Task 3.
2. **Salir de la app mientras un redactor está enviando:** la ventana responde `ok` recién cuando terminó `QueueSend`, o falla con el error. La app no se cierra con un envío a medias. Test en Task 3.
3. **Abrir dos veces el mismo borrador**, por ejemplo desde el aviso de borradores y desde la carpeta: la segunda vez se trae adelante la ventana existente. Pasa también cuando la clave se registró después del primer autoguardado (`SetComposeKey`). Tests en Task 1 y Task 3.
4. **Una ventana que no responde a `compose:flush`** (colgada o cerrándose): a los 10 s la app no se cierra y esa ventana se trae adelante. Test con un timeout corto inyectado, en Task 1.
5. **Soltar archivos sobre un redactor con dos abiertos:** los adjuntos llegan solo a esa ventana. El evento va por `win.EmitEvent`, no por `app.Event.Emit`. Test de la lógica en Task 1.

---

### Task 1: Registro de ventanas del redactor en Go

**Files:**
- Create: `app/composewin.go`, `app/composewin_test.go`.
- Modify:
  - `app/filedrop.go`: `SetupFileDrop` recibe la ventana y emite solo a ella;
  - `app/desktop.go`: "Salir" y Ctrl+Q pasan por el registro;
  - `app/mailservice.go`: métodos ligados;
  - `main.go`.
- Test: `core/mail/drafts_test.go` (concurrencia de borradores).

**Interfaces:**
- Produces (Go, `app`):
  - `type composeWindow interface { Focus(); Close(); EmitEvent(name string, data ...any) bool; SetAllowClose(bool) }`. La implementación real envuelve `*application.WebviewWindow` y un `allowClose atomic.Bool`.
  - `type ComposeWindows struct { ... }`, creado con `newComposeWindows(create func(id int) composeWindow, quit func(), timeout time.Duration) *ComposeWindows`.
  - `func (r *ComposeWindows) Open(initJSON, key string) (int, error)`: si `key != ""` y ya hay una ventana con esa clave, la trae adelante y devuelve su id. Si no, crea una.
  - `func (r *ComposeWindows) Init(id int) (string, error)`: un id desconocido da `UserError` Code `"not_found"`.
  - `func (r *ComposeWindows) SetKey(id int, key string)`.
  - `func (r *ComposeWindows) Close(id int)`: permite el cierre, cierra y saca la ventana del registro.
  - `func (r *ComposeWindows) Closed(id int)`: la ventana ya no existe.
  - `func (r *ComposeWindows) Flushed(id int, ok bool, message string)`.
  - `func (r *ComposeWindows) Quit()`: sin ventanas, llama a `quit()`. Con ventanas, emite `compose:flush` a cada una y espera respuestas hasta `timeout`.
    - Si todas responden `ok`, llama a `quit()`.
    - Si no, a la primera que falló o no respondió le hace `Focus()` y le emite `compose:quit-blocked` con el mensaje. Si no respondió, el mensaje es "No pudimos guardar este borrador; la app sigue abierta.".
    - Corre en una goroutine: no bloquea el hilo de la UI.
  - Métodos ligados de `MailService`, que delegan en el registro que se le inyecta con `SetComposeWindows(r)`:
    - `OpenComposeWindow(initJSON string, key string) error`;
    - `ComposeInit(id int) (string, error)`;
    - `SetComposeKey(id int, key string)`;
    - `CloseComposeWindow(id int)`;
    - `ComposeFlushed(id int, ok bool, message string)`;
    - `ComposeSendStarted(draftID int64)`, que emite `compose:send-started` a todas las ventanas.
  - `SetupComposeWindows(app *application.App, svc *MailService) *ComposeWindows`. Crea ventanas con:
    - `Title "Nuevo correo"`, `Width 900`, `Height 720`, `MinWidth 640`, `MinHeight 480`;
    - `URL "/#/compose/<id>"`, `EnableFileDrop: true`;
    - el mismo `BackgroundColour` que la principal.

    En cada ventana registra:
    - el hook `WindowClosing`: si no está permitido cerrar, `e.Cancel()` y `EmitEvent("compose:close-request")`;
    - `SetupFileDrop(app, win, svc)`;
    - un listener de `WindowClosed` que llama a `Closed(id)`.

- [ ] **Step 1: Tests que fallan** en `app/composewin_test.go`, con una ventana falsa que registra `Focus`, `Close` y los eventos emitidos:
  - `TestComposeWindows_OpenStoresInit`: `Open(json, "")` devuelve el id, e `Init(id)` devuelve el mismo JSON. Un id desconocido da error `not_found`.
  - `TestComposeWindows_DedupByKey`:
    - `Open(a, "draft:5")` y luego `Open(b, "draft:5")` devuelven el mismo id, crean una sola ventana y la segunda llamada hace `Focus()`;
    - después de `SetKey(id2, "draft:9")` en otra ventana, `Open(c, "draft:9")` enfoca la `id2`;
    - `Open(x, "")` dos veces crea dos ventanas.
  - `TestComposeWindows_CloseAndClosedForget`: después de `Close(id)` o `Closed(id)`, `Open(_, sameKey)` crea una ventana nueva.
  - `TestComposeWindows_QuitWithoutWindows`: `quit` se llama una vez.
  - `TestComposeWindows_QuitAllOK`: con dos ventanas, `Quit()` emite `compose:flush` a cada una; cuando las dos responden `Flushed(id, true, "")`, se llama a `quit`.
  - `TestComposeWindows_QuitBlocked`: con una que responde `Flushed(id, false, "msg")`, `quit` no se llama, esa ventana recibe `Focus()` y `compose:quit-blocked` con "msg".
  - `TestComposeWindows_QuitTimeout`: con un timeout de 50 ms y una ventana que no responde, `quit` no se llama, y la ventana recibe `Focus()` y `compose:quit-blocked` con "No pudimos guardar este borrador; la app sigue abierta.".
  - `TestHandleDrop` (los existentes) siguen pasando. `SetupFileDrop` emite con `win.EmitEvent`; esa parte se verifica en la revisión, porque no tiene test unitario.
  - En `core/mail/drafts_test.go`, `TestSaveDraft_ConcurrentNewDraftsSameAccount`: dos `SaveDraft(ctx, 0, req)` en paralelo para la misma cuenta dan dos borradores distintos, los dos guardados. Si falla, agregar el bloqueo que falte en el núcleo.
- [ ] **Step 2:** Correr `go test ./app/... ./core/mail -run 'ComposeWindows|ConcurrentNewDrafts'`. Se espera que fallen.
- [ ] **Step 3: Implementar** `app/composewin.go` y los métodos ligados.
  - **Cableado en `main.go`:**
    - `composeWins := app.SetupComposeWindows(wailsApp, mailService)`;
    - `mailService.SetComposeWindows(composeWins)`;
    - `SetupDesktop` recibe una función de salida (`composeWins.Quit`) para el menú "Salir" de la bandeja;
    - `QuitKeyBindings(composeWins.Quit)`;
    - se saca `SetupFileDrop` de la ventana principal y se pone `EnableFileDrop: false` en ella.
  - **Firmas:** cambiar `SetupDesktop(app, win)` a `SetupDesktop(app, win, quit func())` y actualizar sus tests.
  - Después, `task common:generate:bindings`.
- [ ] **Step 4:** Correr `go test ./app/... ./core/mail` (se espera PASS) y `task check` (verde).
- [ ] **Step 5: Commit.** `feat(redactor): registro de ventanas del redactor y salida que espera los borradores`.

### Task 2: El frontend abre ventanas en vez del diálogo

**Files:**
- Create:
  - `frontend/src/lib/composeWindow.ts`;
  - `frontend/src/features/compose/ComposeWindowApp.tsx`, como esqueleto que esta tarea solo monta; lo completa la Task 3.
- Modify:
  - `frontend/src/main.tsx`;
  - `frontend/src/features/compose/ComposerContext.tsx`.
- Test:
  - `frontend/src/lib/composeWindow.test.ts`;
  - `frontend/src/features/compose/ComposerContext.test.tsx` (crear);
  - los tests de los que usan `useComposer` siguen pasando (MessageActions, OutboxList, UnsyncedDraftsBanner, ContactPanel, MailShell, MessageView).

**Interfaces:**
- Consumes: los bindings de la Task 1 (`svc.OpenComposeWindow`, `svc.ComposeInit`, etc.).
- Produces:
  - `composeWindowId(hash?: string): number | null`, que lee `#/compose/<id>` (default `location.hash`). Algo que no matchea, o un id que no es entero positivo, da `null`.
  - `serializeInit(init: ComposerInit): string` y `parseInit(json: string): ComposerInit`, que validan los campos obligatorios: `accountId` número y los arrays presentes.
  - En `ComposerContext`:
    - la API `ComposerApi` queda igual;
    - `open(init, key)` llama a `svc.OpenComposeWindow(serializeInit(init), key)` y muestra `toast.error(errorMessage(e))` si falla;
    - las claves son `openDraft(d)` → `outbox:${d.id}`, `openComposeDraft(d)` → `draft:${d.id}`, `openNew`/`openFrom` → `""`;
    - `ComposerProvider` ya no monta `<Composer>`;
    - escucha `compose:send-started` y llama a `sendStarted(id)`.

- [ ] **Step 1: Tests que fallan.**
  - `composeWindow.test.ts`:
    - `composeWindowId("#/compose/7")` da `7`;
    - `""`, `"#/compose/"`, `"#/compose/x"`, `"#/compose/0"` y `"#/compose/-3"` dan `null`;
    - `parseInit(serializeInit(init))` da `init`;
    - `parseInit("{}")` lanza.
  - `ComposerContext.test.tsx`, con `svc` y `@wailsio/runtime` mockeados:
    - `openNew(7)` llama a `OpenComposeWindow` con un JSON cuyo `accountId` es 7 y con la clave `""`;
    - `openComposeDraft({ id: 12, … })` usa la clave `"draft:12"`;
    - `openDraftById(5)` usa la clave `"outbox:5"`;
    - si `OpenComposeWindow` rechaza, se muestra el toast de error;
    - no se renderiza ningún `role="dialog"`;
    - un evento `compose:send-started` con 42 muestra el toast `"Enviando correo…"` (vía `sendStarted`).
- [ ] **Step 2:** Correr `cd frontend && npx vitest run src/lib/composeWindow.test.ts src/features/compose/ComposerContext.test.tsx`. Se espera que fallen.
- [ ] **Step 3: Implementar.**
  - `main.tsx` monta los mismos providers (`ThemeProvider`, `QueryClientProvider`, `Toaster`) y adentro `<ComposeWindowApp id={id} />` si `composeWindowId()` no es null, o `<App />` si lo es.
  - `ComposeWindowApp` queda como esqueleto ("Cargando…"); la Task 3 lo completa.
- [ ] **Step 4:** Correr `cd frontend && npx vitest run` (se espera PASS) y `task check` (verde).
- [ ] **Step 5: Commit.** `feat(redactor): abrir el redactor en una ventana propia`.

### Task 3: La ventana del redactor

**Files:**
- Modify:
  - `frontend/src/features/compose/ComposeWindowApp.tsx`;
  - `frontend/src/features/compose/Composer.tsx`: de `Dialog` a layout de ventana.
- Test:
  - `frontend/src/features/compose/ComposeWindowApp.test.tsx`;
  - `frontend/src/features/compose/Composer.test.tsx`, que se adapta: lo que hoy busca dentro del diálogo pasa a buscar en el documento.

**Interfaces:**
- Consumes:
  - `svc.ComposeInit`, `svc.CloseComposeWindow`, `svc.SetComposeKey`, `svc.ComposeFlushed` y `svc.ComposeSendStarted` (Task 1);
  - `parseInit` (Task 2).
- Produces:
  - `ComposeWindowApp({ id }: { id: number })`;
  - `Composer({ init, onClose, windowId }: { init: ComposerInit; onClose: () => void; windowId?: number })`.

- [ ] **Step 1: Tests que fallan.** En `ComposeWindowApp.test.tsx`, con `svc` y `@wailsio/runtime` mockeados (`Events.On` guarda los handlers y `Window.SetTitle` es un spy):
  - `"carga el init y muestra el redactor"`: `ComposeInit(3)` resuelve un JSON, y aparece el campo "Asunto" con el valor del init;
  - `"init inválido muestra el error"`: un JSON roto muestra un texto de error y ningún redactor;
  - `"cerrar llama a CloseComposeWindow"`: en un redactor sin tocar, Esc llama a `CloseComposeWindow(3)`.

  En `Composer.test.tsx`, los casos nuevos:
  - `"el título de la ventana sigue al asunto"`: arranca con `SetTitle("Nuevo correo")`, y al escribir "Hola" pasa a `SetTitle("Hola")`;
  - `"compose:close-request hace lo mismo que Esc"`:
    - con autoguardado y cambios, guarda y cierra;
    - sin autoguardado y con cambios, muestra "¿Descartar este correo?";
    - un segundo `close-request` con el diálogo abierto no abre otro ni cierra;
  - `"compose:flush"`:
    - sin tocar → `ComposeFlushed(id, true, "")` sin guardar;
    - con autoguardado y cambios → `flush()` y `ComposeFlushed(id, true, "")`;
    - si `flush` rechaza → `ComposeFlushed(id, false, <mensaje del error>)`;
    - reintento de la Bandeja de salida con cambios → `ComposeFlushed(id, false, "Tenés cambios sin enviar en este correo de la Bandeja de salida. Envialo o descartá los cambios antes de salir.")`;
    - con un envío en curso → la respuesta llega después de que resuelve `QueueSend`;
  - `"compose:quit-blocked muestra el mensaje"`: el texto aparece en el área de error del redactor;
  - `"cuando el autoguardado crea el borrador registra la clave"`: `SetComposeKey(id, "draft:<n>")` se llama una vez cuando `draftId` pasa de 0 a n;
  - `"enviar avisa a la principal"`: después de `QueueSend` resuelto con 99, se llama a `ComposeSendStarted(99)` y no a `sendStarted` local, y después `onClose`.
- [ ] **Step 2:** Correr `cd frontend && npx vitest run src/features/compose`. Se espera que fallen.
- [ ] **Step 3: Implementar.**
  - **Layout:** `Composer` deja de usar `Dialog` y `DialogContent` para el redactor; las confirmaciones siguen siendo `Dialog`. El contenedor raíz es `id="compose-drop"`, `data-file-drop-target`, `h-screen flex flex-col` con el mismo overlay de soltar archivos. Arriba un encabezado con el título del modo, en el medio el cuerpo con scroll y abajo el pie de hoy.
  - **Esc:** un listener `keydown` en `window`. Si el menú "/" o las sugerencias están abiertos, los cierra; si no, `requestClose()`. Hay que ignorarlo si hay un `Dialog` de confirmación abierto, que maneja su propio Esc.
  - **Eventos:** con `windowId`, escucha `compose:close-request`, `compose:flush` y `compose:quit-blocked` con `Events.On`, y los limpia al desmontar.
  - **Título:** `Window.SetTitle(subject.trim() || TITLES[mode] || "Nuevo correo")` cuando cambia el asunto.
  - **Enviar:** reemplaza `sendStarted(id)` por `svc.ComposeSendStarted(id)` cuando hay `windowId`.
  - **`ComposeWindowApp`:**
    - pide `ComposeInit(id)` y hace `parseInit`;
    - monta `<Composer init windowId={id} onClose={() => void svc.CloseComposeWindow(id)} />`;
    - no monta `MailShell` ni `useMailEvents`;
    - los datos que usa (cuentas, firmas, sugerencias) los pide por las mismas queries que ya usa `Composer`.
- [ ] **Step 4:** Correr `cd frontend && npx vitest run` (se espera PASS) y `task check` (verde). Confirmar que `frontend/dist/index.html` sigue siendo la única entrada.
- [ ] **Step 5: Commit.** `feat(redactor): la ventana del redactor cierra, guarda y avisa como antes`.

### Task 4: Prueba manual de Fabricio (no la hace un subagente)

Con `pkill -x postacontrol; task install:desktop`:
- **Abrir:** "Redactar" abre una ventana aparte que se mueve, se estira y se maximiza. "Responder" y "Reenviar" también.
- **Varias a la vez:** se pueden tener dos redactores abiertos. Abrir un borrador que ya está abierto trae su ventana adelante.
- **Cerrar:**
  - la ✕ de la ventana con texto escrito guarda en Borradores;
  - un reintento de la Bandeja de salida con cambios pregunta "¿Descartar este correo?", y "Cancelar" deja la ventana abierta;
  - Esc hace lo mismo que la ✕.
- **Adjuntar:** arrastrar archivos desde el administrador de archivos a una de dos ventanas abiertas los adjunta solo ahí. "Adjuntar" abre el diálogo sobre esa ventana.
- **Enviar:** la ventana se cierra, y en la principal aparece "Enviando correo…" y después "Correo enviado".
- **Salir con redactores abiertos** (con Ctrl+Q o desde la bandeja): se guardan los borradores y la app se cierra. Al volver a abrirla, los borradores están en Borradores.
- **Tema:** con el tema del sistema cambiado, la ventana del redactor lo sigue.

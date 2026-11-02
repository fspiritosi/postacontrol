# Fase 6A: no leídos, fondo del panel de lectura y visor de adjuntos. Plan de implementación

> **Para agentes:** SUB-SKILL REQUERIDA: usar superpowers:subagent-driven-development (recomendada) o superpowers:executing-plans para implementar este plan tarea por tarea. Los pasos usan casillas (`- [ ]`) para el seguimiento.

**Goal:** Que un correo no leído se vea de un vistazo. Que el panel de lectura vacío muestre una imagen elegida por Fabricio. Que los PDF y las imágenes adjuntas se vean dentro de la app sin descargarlos.

**Architecture:**
- **No leídos:** dos tokens de color nuevos en el tema (`--unread`, `--unread-foreground`) que usan las filas de mensaje y de conversación.
- **Fondo:**
  - El núcleo copia la imagen elegida a su carpeta de datos (la del `DBPath`) y guarda el nombre del archivo en `Settings.Background`.
  - La UI pide la imagen como data URL y la pinta detrás de los estados vacíos del lector.
- **Visor:**
  - El núcleo devuelve los bytes de un adjunto, con un tope de 50 MB.
  - La UI lo muestra en un diálogo a pantalla casi completa. Las imágenes se ven con `<img>` sobre un blob URL; los PDF, con pdf.js dibujado en canvas.

**Tech Stack:** Go 1.27 + Wails v3.0.0 (`app/`), núcleo `core/mail` (sin Wails), React 19 + Tailwind v4 + shadcn/radix + TanStack Query, Vitest/jsdom, `pdfjs-dist` 6.x (nuevo).

**Spec:** `docs/superpowers/specs/2026-11-02-fondo-noleidos-redactor-visor-design.md`, secciones 1, 2 y 4. La sección 3 (el redactor en ventana propia) es la Fase 6B, en otro plan.

## Global Constraints

- **Núcleo:**
  - `core/` nunca importa Wails.
  - Los tests usan solo `core/testenv`, nunca cuentas reales.
  - TLS siempre verificado.
- **Escrituras en el servidor:** ninguna en esta fase. Leer un adjunto es un FETCH: no cambia flags.
- **Antes de cada commit:** `task check` en verde (desde la raíz del repo).
- **Commits:** convencionales, en español, terminados en `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- **Bindings:** solo con `task common:generate:bindings`, después de cambiar firmas de `app/`.
- **Texto de la UI:** en español rioplatense con voseo, como el resto de la app.
- **Valores y textos del spec, exactos:**
  - el azul del logo es `#3598CC` (rgb 53 152 204);
  - tope del fondo: 20 MB (`20 << 20`); tope del visor: 50 MB (`50 << 20`);
  - formatos del fondo: JPG, PNG, WebP y GIF;
  - el visor abre `application/pdf`, `image/png`, `image/jpeg`, `image/gif` e `image/webp`;
  - texto del tope del visor: "Es muy grande para verlo acá.";
  - la tira de abajo del visor tiene la etiqueta "Adjuntos:";
  - botones del visor: "Descargar" y ✕ (aria-label "Cerrar").
- **No leer ni imprimir `~/.npmrc`.** `frontend/.npmrc` tiene `minimum-release-age`: `npm install` elige solo versiones con más de 7 días, y eso está bien.
- **El proyecto no usa prettier.** No formatear archivos enteros.

## Review Focus

1. **Adjunto PDF que llega como `application/octet-stream` con nombre `factura.pdf`:** se tiene que abrir en el visor igual. Se decide por la extensión cuando el MIME es genérico. Test en Task 5 (`viewKind`).
2. **`saveSettings` del frontend arma la configuración completa y la manda entera:** cambiar "Panel de lectura" no puede borrar el fondo. `Core.SetSettings` ignora el `Background` que llega y conserva el guardado. Test en Task 2.
3. **Alguien borra a mano el archivo de fondo de la carpeta de datos:** `BackgroundImage` devuelve "" sin error y la app sigue andando. Test en Task 2.
4. **Adjunto de 80 MB:** el visor no lo baja. El núcleo corta por `Attachment.Size` antes del FETCH y devuelve el error `too_big`. Test en Task 4.
5. **PDF corrupto o que pdf.js no puede abrir:** el visor muestra el error y sigue ofreciendo "Descargar"; nada se cuelga. Test en Task 6.

---

### Task 1: Correos no leídos (variante C)

**Files:**
- Modify: `frontend/src/index.css`: dentro de `@theme inline`, en `:root` y en `.dark`.
- Modify: `frontend/src/features/messages/MessageRow.tsx`.
- Modify: `frontend/src/features/messages/ThreadList.tsx` (la fila de conversación, alrededor de las líneas 50-180).
- Test: `frontend/src/features/messages/MessageList.test.tsx`, `frontend/src/features/messages/ThreadList.test.tsx`.

**Interfaces:**
- Produces:
  - las clases Tailwind `bg-unread` y `text-unread-foreground`;
  - el atributo `data-unread="true"` en el `<li>` de una fila no leída (en las conversaciones, `t.unread > 0`).

- [ ] **Step 1: Tests que fallan.** En `MessageList.test.tsx`, test `"una fila no leída tiene fondo y remitente resaltados"`. Con un mensaje `seen: false` y otro `seen: true`, en densidad compacta y cómoda:
  - el `<li>` del no leído tiene `data-unread="true"`;
  - su botón principal tiene la clase `bg-unread`;
  - el elemento con el remitente tiene `text-unread-foreground`;
  - el leído no tiene ninguna de las tres cosas.

  Test `"abierto o marcado gana el fondo, pero el remitente sigue azul"`, con un no leído:
  - abierto (`aria-current="true"`): el botón tiene `bg-accent` y no `bg-unread`;
  - marcado: el `<li>` tiene `bg-primary/10` y el botón no tiene `bg-unread`;
  - en los dos casos, el remitente sigue con `text-unread-foreground`.

  En `ThreadList.test.tsx`, el mismo primer test para una conversación con `unread: 2`.
- [ ] **Step 2:** Correr `cd frontend && npx vitest run src/features/messages`. Se espera que fallen los tests nuevos.
- [ ] **Step 3: Tokens.**
  - En `:root`: `--unread: rgb(53 152 204 / 0.12);` y `--unread-foreground: rgb(30 112 160);`.
  - En `.dark`: `--unread: rgb(53 152 204 / 0.18);` y `--unread-foreground: rgb(120 190 230);`.
  - En `@theme inline`: `--color-unread: var(--unread);` y `--color-unread-foreground: var(--unread-foreground);`.
- [ ] **Step 4: Filas.**
  - **Botón principal**, en las dos densidades: `selected && !marked && "bg-accent"`, y además `!seen && !selected && !marked && "bg-unread"`.
  - **Remitente no leído:** `font-semibold text-unread-foreground`, en lugar de solo `font-semibold`.
  - **`<li>`:** `data-unread={!seen || undefined}`.
  - **Fila de conversación:** lo mismo, con `unread` (= `t.unread > 0`).
- [ ] **Step 5:** Correr `cd frontend && npx vitest run src/features/messages` (se espera PASS) y después `task check` desde la raíz (verde).
- [ ] **Step 6: Commit.** `feat(lista): resaltar los correos no leídos con fondo y remitente azul`.

### Task 2: Fondo, núcleo y servicio

**Files:**
- Modify: `core/model/models.go`: en `Settings`, sumar `Background string \`json:"background"\``, con el comentario "nombre del archivo de fondo dentro de la carpeta de datos; vacío = sin fondo".
- Create: `core/mail/background.go`.
- Modify: `core/mail/settings.go` (`SetSettings`).
- Modify: `core/mail/core.go`: guardar `dataDir = filepath.Dir(cfg.DBPath)` en `Core`.
- Modify: `app/mailservice.go`.
- Test: `core/mail/background_test.go`, `core/mail/settings_test.go`.

**Interfaces:**
- Produces, en el núcleo:
  - `const MaxBackgroundBytes = 20 << 20`;
  - `func (c *Core) SetBackground(ctx context.Context, srcPath string) error`;
  - `func (c *Core) ClearBackground(ctx context.Context) error`;
  - `func (c *Core) BackgroundImage(ctx context.Context) (string, error)`, que devuelve un data URL (`data:<mime>;base64,...`), o "" si no hay fondo.
- Produces, en el servicio:
  - `func (s *MailService) PickBackground(ctx context.Context) (bool, error)`: abre el diálogo nativo y devuelve false sin error si se cancela;
  - `func (s *MailService) ClearBackground(ctx context.Context) error`;
  - `func (s *MailService) BackgroundImage(ctx context.Context) (string, error)`.

- [ ] **Step 1: Tests que fallan** en `core/mail/background_test.go`, con `newTestCore`. Los PNG de prueba son bytes válidos para `http.DetectContentType`: alcanza con la firma PNG seguida de relleno.
  - `TestSetBackground_CopiesAndServesDataURL`:
    - después de `SetBackground(png)`, `GetSettings().Background != ""` y el archivo existe en `filepath.Dir(DBPath)`;
    - `BackgroundImage` empieza con `data:image/png;base64,`;
    - borrar el original no cambia nada;
    - se emite `EventSettingsChanged`.
  - `TestSetBackground_ReplacesAndRemovesOld`: después de un segundo `SetBackground`, el archivo anterior ya no existe.
  - `TestSetBackground_RejectsNonImage`: un `.txt` da un `UserError` con Code `"background"` y Msg `"Solo se pueden usar imágenes PNG, JPG, GIF o WebP."`. La configuración no cambia.
  - `TestSetBackground_RejectsTooBig`: un archivo de `MaxBackgroundBytes+1` bytes da un `UserError` con Code `"background"`, y el Msg contiene `"20 MB"`.
  - `TestClearBackground_RemovesFile`: la configuración queda en `Background == ""`, y el archivo ya no existe.
  - `TestBackgroundImage_MissingFileIsEmpty`: si se borra a mano el archivo copiado, devuelve `"", nil`.
  - En `settings_test.go`, `TestSetSettings_KeepsBackground`: con un fondo puesto, `SetSettings(model.Settings{ReadingPane: "top"})` (con `Background` vacío) deja el fondo como estaba.
- [ ] **Step 2:** Correr `go test ./core/mail -run 'Background|KeepsBackground'`. Se espera que fallen.
- [ ] **Step 3: Implementar `core/mail/background.go`.**
  - **Validar:** `os.Stat` (que exista y no sea carpeta), el tamaño `<= MaxBackgroundBytes`, y `http.DetectContentType` dentro de png, jpeg, gif o webp. Para el texto del tamaño, reusar `formatSize` / `sameSize` como hace `imageTooBig`.
  - **Copiar:** a `filepath.Join(c.dataDir, fmt.Sprintf("background-%d%s", c.now().UnixNano(), ext))`, con `ext` según el MIME. Usar `writeFileAtomic` con permisos 0o600.
  - **Guardar:** el nombre base en la configuración, conservando el resto de los campos.
  - **Borrar el anterior** solo después de guardar la configuración, sin fallar si no existe.
  - **Avisar:** `c.emit(EventSettingsChanged, nil)`.
  - **`ClearBackground`:** el camino inverso.
  - **`BackgroundImage`:** si el archivo no existe, devuelve `"", nil`.
  - **`SetSettings`:** lee la configuración guardada y copia su `Background` en `st` antes de guardar.
- [ ] **Step 4: Servicio.** `PickBackground` copia el patrón de `PickImage`:
  - título "Elegir fondo";
  - filtro `"Imágenes", "*.png;*.jpg;*.jpeg;*.gif;*.webp"`;
  - si se cancela (`path == ""`), devuelve `false, nil`;
  - si no, `s.core.SetBackground`.

  `ClearBackground` y `BackgroundImage` pasan directo al núcleo. Después, `task common:generate:bindings`.
- [ ] **Step 5:** Correr `go test ./core/mail` (se espera PASS) y `task check` (verde).
- [ ] **Step 6: Commit.** `feat(fondo): guardar una imagen de fondo para el panel de lectura`.

### Task 3: Fondo, UI

**Files:**
- Modify: `frontend/src/lib/queries.ts`: sumar `useBackground()`.
- Modify: `frontend/src/lib/events.ts`: en `mail:settings-changed`, invalidar también `["background"]`.
- Modify: `frontend/src/lib/settings.ts`: sumar `background: ""` a los valores base de `saveSettings`, para que compile con el binding nuevo.
- Create: `frontend/src/features/reader/ReaderBackdrop.tsx`.
- Modify:
  - `frontend/src/features/reader/MessageView.tsx` (el bloque `messageId == null`);
  - `frontend/src/features/reader/MarkedPane.tsx`;
  - `frontend/src/features/contacts/ContactPanel.tsx` (el bloque `id == null`);
  - `frontend/src/features/settings/SettingsDialog.tsx`.
- Test:
  - `frontend/src/features/reader/ReaderBackdrop.test.tsx`;
  - `frontend/src/features/settings/SettingsDialog.test.tsx`;
  - los tests existentes de MarkedPane, MessageView y ContactPanel siguen pasando (mockear `BackgroundImage` donde haga falta).

**Interfaces:**
- Consumes: `svc.PickBackground`, `svc.ClearBackground` y `svc.BackgroundImage` (Task 2).
- Produces:
  - `useBackground(): UseQueryResult<string>`, con queryKey `["background"]` y `staleTime: Infinity`;
  - `ReaderBackdrop({ children }: { children: ReactNode })`.

- [ ] **Step 1: Tests que fallan.**
  - `ReaderBackdrop.test.tsx`:
    - `"sin fondo muestra el contenido como siempre"`: no hay elemento con `data-testid="reader-backdrop"`, y el texto hijo está;
    - `"con fondo pinta la imagen detrás y el texto sobre un recuadro"`: hay `data-testid="reader-backdrop"` con `style.backgroundImage` que contiene el data URL, `background-size: cover` y `background-position: center`, y el hijo está dentro de un contenedor con la clase `backdrop-blur-sm`.
  - `SettingsDialog.test.tsx`:
    - `"Elegir imagen… llama a PickBackground"`;
    - `"Quitar llama a ClearBackground y solo aparece con fondo puesto"`;
    - `"con fondo muestra la miniatura"`: hay un `img` con `alt="Fondo actual"`.
- [ ] **Step 2:** Correr `cd frontend && npx vitest run src/features/reader src/features/settings`. Se espera que fallen.
- [ ] **Step 3: `ReaderBackdrop`.**
  - Sin imagen: devuelve `children` envuelto en el mismo `div` centrado de hoy (`flex h-full items-center justify-center p-6 text-center`).
  - Con imagen: el `div` lleva `data-testid="reader-backdrop"` y el estilo `backgroundImage: url("…")`, `backgroundSize: "cover"` y `backgroundPosition: "center"`. Los hijos van en `rounded-lg bg-background/80 px-4 py-3 backdrop-blur-sm`.
  - Usarlo en tres lugares, cada uno con su texto de hoy y su icono:
    - el vacío de `MessageView`;
    - `MarkedPane`, que ya no arma su propio `div` exterior;
    - el vacío de `ContactPanel`.
- [ ] **Step 4: Configuración.** Una fila "Fondo del panel de lectura" con la descripción "Se ve cuando no hay un correo abierto.", debajo de "Panel de lectura":
  - una miniatura de 64×40 `object-cover` si hay fondo;
  - un botón "Elegir imagen…" (`PickBackground`; con `true`, se invalida `["background"]`);
  - un botón "Quitar", solo con fondo puesto.

  Los errores van con `toast.error(errorMessage(e))`.
- [ ] **Step 5:** Correr `cd frontend && npx vitest run` (se espera PASS) y `task check` (verde).
- [ ] **Step 6: Commit.** `feat(fondo): mostrar el fondo en el panel de lectura vacío y elegirlo en Configuración`.

### Task 4: Visor, bytes del adjunto (núcleo y servicio)

**Files:**
- Modify:
  - `core/model/models.go`: `type AttachmentData struct { Filename string \`json:"filename"\`; MIMEType string \`json:"mimeType"\`; Data []byte \`json:"data"\` }`;
  - `core/mail/read.go`;
  - `app/mailservice.go`.
- Test: `core/mail/read_test.go`.

**Interfaces:**
- Produces:
  - `const MaxPreviewBytes = 50 << 20`;
  - `func (c *Core) AttachmentData(ctx context.Context, messageID int64, index int) (model.AttachmentData, error)`;
  - `func (s *MailService) AttachmentData(ctx context.Context, messageID int64, index int) (model.AttachmentData, error)`.
- En TS, `Data` (`[]byte`) llega como string base64. Verificarlo en `frontend/bindings/postacontrol/core/model/models.ts` después de generar; si fuera otra cosa, anotarlo en el reporte.

- [ ] **Step 1: Tests que fallan** en `read_test.go`. Reusar `attachmentMail` y `pdfBytes`.
  - `TestAttachmentData_ReturnsBytes`: devuelve `Data == pdfBytes`, `Filename == "factura.pdf"` y `MIMEType == "application/pdf"`.
  - `TestAttachmentData_TooBigSkipsFetch`: con el detalle del mensaje en la caché y `Size` del adjunto forzado a `MaxPreviewBytes+1` en el store, da un `UserError` con Code `"too_big"` y Msg `"Es muy grande para verlo acá."`. Para no depender del servidor, cerrar el IMAP de prueba antes de llamar: si hubiera FETCH, el error sería otro. Si forzar el tamaño necesita un helper del store solo para tests, agregarlo en un `_test.go` del paquete `mail`.
  - `TestAttachmentData_BadIndex`: Code `"not_found"`.
- [ ] **Step 2:** Correr `go test ./core/mail -run AttachmentData`. Se espera que falle.
- [ ] **Step 3: Implementar.**
  - **Antes de `attachmentBytes`:** leer el detalle (`GetMessage`), validar el índice, y si `att.Size > MaxPreviewBytes`, devolver `too_big`.
  - **Después de bajar:** volver a chequear `len(data)`, porque `Size` es el tamaño codificado aproximado.
  - **`MIMEType`:** el del adjunto.
  - **Servicio:** pasa directo al núcleo.
  - Después, `task common:generate:bindings`.
- [ ] **Step 4:** Correr `go test ./core/mail` (se espera PASS) y `task check` (verde).
- [ ] **Step 5: Commit.** `feat(visor): leer un adjunto en memoria con tope de 50 MB`.

### Task 5: Visor, diálogo e imágenes

**Files:**
- Create:
  - `frontend/src/lib/attachments.ts`;
  - `frontend/src/features/reader/AttachmentViewer.tsx`.
- Modify: `frontend/src/features/reader/AttachmentList.tsx`.
- Test:
  - `frontend/src/lib/attachments.test.ts`;
  - `frontend/src/features/reader/AttachmentViewer.test.tsx`;
  - `frontend/src/features/reader/AttachmentList.test.tsx` (crear si no existe).

**Interfaces:**
- Consumes: `svc.AttachmentData` (Task 4) y `svc.SaveAttachment` (ya existe).
- Produces:
  - `type ViewKind = "pdf" | "image"`;
  - `viewKind(a: Pick<Attachment, "mimeType" | "filename">): ViewKind | null`;
  - `base64ToBytes(s: string): Uint8Array`;
  - `AttachmentViewer({ messageId, attachments, index, onClose }: { messageId: number; attachments: Attachment[]; index: number; onClose: () => void })`;
  - un punto de enchufe para la Task 6: el componente `PdfView({ data, zoom }: { data: Uint8Array; zoom: number | null })`, que en esta tarea es un stub en `frontend/src/features/reader/PdfView.tsx` que dice "Vista previa de PDF próximamente". La Task 6 lo reemplaza.

- [ ] **Step 1: Tests que fallan.**
  - **`attachments.test.ts`:**
    - `viewKind` da `"pdf"` para `application/pdf`;
    - da `"image"` para png, jpeg, gif y webp;
    - da `null` para `application/vnd.ms-excel` y `image/svg+xml` (SVG puede traer scripts, así que no se previsualiza);
    - con `application/octet-stream` decide por la extensión: `factura.PDF` da `"pdf"`, `foto.jpeg` da `"image"` y `x.zip` da `null`.
    - `base64ToBytes("AAEC")` da `[0,1,2]`.
  - **`AttachmentViewer.test.tsx`**, con `svc` mockeado y `URL.createObjectURL`/`revokeObjectURL` stubbeados:
    - `"muestra Cargando… y después la imagen"`: aparece `"Cargando…"` y después un `img` con `alt` igual al nombre del archivo;
    - `"el error del núcleo se muestra y Descargar sigue"`: con un `AttachmentData` rechazado con `"Es muy grande para verlo acá."`, se ve ese texto y el botón "Descargar" llama a `SaveAttachment(messageId, index)`;
    - `"la tira lista todos los adjuntos y los no visibles van apagados"`: un `.xlsx` tiene `data-viewable="false"` y, al hacer clic, llama a `SaveAttachment` sin cambiar lo que se ve;
    - `"flechas pasan entre los visibles"`: con [pdf, xlsx, png] y el índice en el pdf, `ArrowRight` salta al png (`AttachmentData` llamado con el índice 2), y `ArrowLeft` vuelve al pdf;
    - `"zoom: Ajustar, + y −"`: arranca en el texto `"Ajustar"`; `+` lleva a `"100%"`, `+` a `"125%"`, `−` a `"100%"`, y `−` desde `50%` vuelve a `"Ajustar"`;
    - `"Esc y ✕ cierran"`: llaman a `onClose`;
    - `"revoca el blob URL al cambiar o cerrar"`.
  - **`AttachmentList.test.tsx`:**
    - un clic en un PDF o una imagen abre el visor (aparece el diálogo con el nombre del archivo) y no llama a `SaveAttachment`;
    - un clic en un `.zip` llama a `SaveAttachment` como hoy.
- [ ] **Step 2:** Correr `cd frontend && npx vitest run src/lib/attachments.test.ts src/features/reader`. Se espera que fallen.
- [ ] **Step 3: Implementar.**
  - **Contenedor:** `Dialog` de shadcn con `DialogContent` grande (`w-[calc(100vw-3rem)] max-w-none h-[calc(100vh-3rem)] p-0`, flex en columna), con `DialogTitle` igual al nombre del archivo y `DialogDescription` sr-only.
  - **Barra de arriba:** nombre, "pág. N de M" (solo PDF; la Task 6 lo llena), `−`, la etiqueta de zoom, `+`, "Descargar" y ✕ (aria-label "Cerrar").
  - **Zoom:** el estado es `number | null`, donde `null` es "Ajustar". Los niveles son `[0.5, 0.75, 1, 1.25, 1.5, 2, 3]`.
    - `+`: de `null` pasa a 1; si no, al nivel siguiente, con tope 3.
    - `−`: de 0.5 pasa a `null`; si no, al nivel anterior; desde `null` no hace nada.
  - **Contenido de una imagen:**
    - `<img>` sobre un blob URL de `base64ToBytes(data)` con el MIME;
    - "Ajustar": `max-h-full max-w-full object-contain`;
    - con zoom, `width = naturalWidth * zoom`, dentro de un contenedor `overflow-auto` que se arrastra con el puntero para moverla.
  - **Contenido de un PDF:** `<PdfView data zoom />`.
  - **Tira de abajo:** "Adjuntos:" seguido de un botón por adjunto, con `data-viewable` según `viewKind`. El actual va resaltado.
  - **Teclado:** `ArrowLeft` y `ArrowRight` pasan entre los visibles. Esc lo maneja el `Dialog`.
  - **Mientras carga:** "Cargando…".
  - **Si hay error:** `errorMessage(e)`, con "Descargar" disponible.
  - **`AttachmentList`:** un adjunto con `viewKind != null` abre el visor en su índice; el resto guarda como hoy.
- [ ] **Step 4:** Correr `cd frontend && npx vitest run` (se espera PASS) y `task check` (verde).
- [ ] **Step 5: Commit.** `feat(visor): ver imágenes adjuntas sin descargarlas`.

### Task 6: Visor, PDF con pdf.js

**Files:**
- Modify: `frontend/package.json`: `npm install pdfjs-dist@6` (con la edad mínima de `frontend/.npmrc`).
- Create:
  - `frontend/src/lib/pdf.ts`: el único archivo que importa `pdfjs-dist`;
  - `frontend/src/features/reader/PdfView.tsx`, que reemplaza el stub.
- Modify: `frontend/src/features/reader/AttachmentViewer.tsx`, para mostrar "pág. N de M".
- Test: `frontend/src/features/reader/PdfView.test.tsx`, con `@/lib/pdf` mockeado.

**Interfaces:**
- Produces:
  - `type PdfDoc = { numPages: number; renderPage(n: number, canvas: HTMLCanvasElement, scale: number): Promise<{ width: number; height: number }>; destroy(): void }`;
  - `loadPdf(data: Uint8Array): Promise<PdfDoc>`;
  - `PdfView({ data, zoom, onPageChange }: { data: Uint8Array; zoom: number | null; onPageChange?: (page: number, total: number) => void })`.

- [ ] **Step 1: Tests que fallan** en `PdfView.test.tsx`:
  - `"dibuja una canvas por página"`: con `numPages: 3`, hay 3 `canvas` y `renderPage` se llamó para 1, 2 y 3;
  - `"zoom null ajusta al ancho; con número usa esa escala"`: con `zoom=1.5`, `renderPage` recibe `scale` 1.5. Con `null`, la escala sale del ancho del contenedor dividido por el ancho de la página a escala 1. En jsdom, stubbear `clientWidth`;
  - `"PDF que no abre muestra el error"`: con `loadPdf` rechazado, el texto contiene `"No pudimos abrir el PDF"`;
  - `"avisa la página visible"`: `onPageChange(1, 3)` al cargar;
  - `"destruye el documento al desmontar"`.
- [ ] **Step 2:** Correr `cd frontend && npx vitest run src/features/reader/PdfView.test.tsx`. Se espera que falle.
- [ ] **Step 3: `lib/pdf.ts`.**
  - **Worker:** `import PdfWorker from "pdfjs-dist/build/pdf.worker.min.mjs?worker"` y `GlobalWorkerOptions.workerPort = new PdfWorker()`, una sola vez y en forma perezosa. Así Vite emite el worker como `.js` y no depende de que el servidor de assets de Wails conozca el MIME de `.mjs`.
  - **Abrir:** `getDocument({ data, isEvalSupported: false, enableXfa: false })`.
  - **Dibujar:** solo en canvas (`page.render({ canvasContext, viewport })`), sin capa de texto ni de anotaciones, así los enlaces del PDF no se pueden cliquear.
  - Con eso se cumple lo de "no ejecutar scripts del PDF" del spec.
- [ ] **Step 4: `PdfView`.**
  - **Páginas:** una debajo de otra, dentro de un contenedor `overflow-auto`. Cada una se dibuja al montarse y se vuelve a dibujar cuando cambia el zoom o, con "Ajustar", el ancho del contenedor (`ResizeObserver`, con guarda si no existe en jsdom).
  - **Página visible:** con `IntersectionObserver` (también con guarda), avisa la página por `onPageChange`.
  - **Mensaje de error:** `"No pudimos abrir el PDF. Podés descargarlo y abrirlo con otro programa."`.
  - **En el visor:** la barra muestra "pág. N de M" con lo que llega por `onPageChange`.
- [ ] **Step 5: Build real.** Correr `task check` (verde) y confirmar que en `frontend/dist/assets/` hay un `pdf.worker…js`. Correr `go test . ./app/...`.
- [ ] **Step 6: Commit.** `feat(visor): ver PDF adjuntos con pdf.js`.

### Task 7: Prueba manual de Fabricio (no la hace un subagente)

Con `pkill -x postacontrol; task install:desktop`:
- **No leídos:** se ven con fondo azul y remitente azul, en tema claro y oscuro, con el panel abajo y a la derecha, y en conversaciones.
- **Fondo:** elegir una imagen en Configuración:
  - se ve con el panel vacío, con varios marcados y en Contactos sin ficha;
  - se va al abrir un correo;
  - "Quitar" la saca.
- **Visor:**
  - un PDF de varias páginas: scroll, zoom y "pág. N de M";
  - una imagen grande: zoom, y moverla arrastrando;
  - un Excel en la tira se descarga;
  - "Descargar" desde el visor;
  - Esc cierra.
- **El worker de pdf.js en la app instalada:** si un PDF se queda en "Cargando…" para siempre, el worker no arrancó.

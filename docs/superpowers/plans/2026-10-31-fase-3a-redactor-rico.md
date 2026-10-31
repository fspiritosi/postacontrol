# PostaControl — Fase 3A "Redactor rico y adjuntos" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Fabricio redacte correos con la comodidad de Notion y Outlook (menú "/", barra de formato al seleccionar, títulos, citas, tablas, imágenes, separadores) y adjunte archivos propios desde el disco, y que lo que sale se vea bien en Gmail y Outlook.

**Architecture:**
- **Editor:** el TipTap de la Fase 1 suma extensiones (tabla, imagen), un menú "/" hecho con `@tiptap/suggestion` y una barra flotante (`BubbleMenu`).
- **Imágenes en el editor:** viven como `data:` URI. Al enviar, el núcleo las saca del HTML y las manda como partes `cid:` dentro de `multipart/related`, porque Gmail no muestra `data:`.
- **HTML de correo:** se convierte con estilos en línea, porque los clientes de correo ignoran las hojas de estilo. La versión en texto plano sale del mismo HTML.
- **Adjuntos:**
  - Los archivos se eligen con el diálogo nativo o se arrastran desde el administrador de archivos (file drop de Wails).
  - El núcleo los copia a SQLite ("adjuntos preparados") y el pedido de envío lleva solo sus ids. Así la Bandeja de salida puede reintentar después de reiniciar, aunque el archivo original se haya movido.

**Tech Stack:** el de las fases anteriores. Se suman:
- **En el frontend:** `@tiptap/extension-table`, `@tiptap/extension-image`, `@tiptap/suggestion` y `@tiptap/extension-file-handler`, todos en `3.32.2` (la misma versión que el resto de TipTap), y `@floating-ui/dom`, que `@tiptap/react/menus` necesita como peer.
- **En Wails 3.0.0:** `WebviewWindowOptions.EnableFileDrop`, el atributo `data-file-drop-target`, el evento `events.Common.WindowFilesDropped` y `WindowEventContext.DroppedFiles()` / `DropTargetDetails()`. Verificado en el código de v3.0.0, también la implementación de Linux.

**Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`, secciones "Redactor", "Responder y reenviar" y "Fase 3". La Fase 3 se parte en dos planes:
- **3A:** este plan.
- **3B:** firmas globales y borradores que se guardan solos.

## Global Constraints

- **Siguen vigentes** todas las de las fases 1 y 2:
  - `core/` sin Wails; contraseñas solo en el llavero; TLS verificado; tests solo con `core/testenv`.
  - Español rioplatense con voseo.
  - `task check` verde antes de cada commit.
  - Commits convencionales en español con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Bindings con `task common:generate:bindings`, nunca con `wails3 generate bindings` directo.
- **Rama:** `fase-3a`, desde `main`.
- **Escrituras en el servidor:** esta fase no agrega ninguna. Solo cambia qué se envía: el envío y la copia en Enviados siguen como están.
- **Menú "/":**
  - Ítems, en este orden y con estos títulos exactos: "Título 1", "Título 2", "Título 3", "Lista con viñetas", "Lista numerada", "Cita", "Tabla", "Imagen", "Separador", "Enlace".
  - No se ofrecen columnas, toggles ni bloques de código: el spec pide no ofrecer lo que no se representa bien en email.
- **Barra al seleccionar texto,** con estas etiquetas accesibles exactas: "Negrita", "Cursiva", "Subrayado", "Tachado", "Enlace" y "Quitar formato".
- **Límites:**
  - `MaxMessageBytes = 25 << 20`: el total de adjuntos más imágenes en línea por correo. Es el límite de Gmail para recibir; Hostinger acepta un poco más.
  - `MaxInlineImageBytes = 5 << 20` por imagen.
  - Los mensajes de error usan `formatSize` (frontend) y su equivalente en Go `formatSize(n int64) string` en `core/mail`, con el mismo formato: "B", "KB" redondeado, "MB" con un decimal y punto. Así los dos lados dicen lo mismo.
- **Imágenes en el editor:**
  - Se aceptan PNG, JPEG, GIF y WebP.
  - En el editor solo existen imágenes `data:image/...`. Las imágenes remotas de un HTML pegado se descartan, para que la ventana principal nunca cargue nada de internet.
- **El HTML del original citado sigue sin entrar a TipTap** (spec, "Responder y reenviar").

## Review Focus

1. **Archivos con nombres no ASCII** ("presupuesto año 2026 — versión final.pdf"): llegan con el nombre intacto. Lo cubren `TestStageFiles_KeepsNonASCIIName` (Task 3) y el test de `mimex` ya existente para RFC 2231.
2. **Un envío falla con adjuntos y la app se cierra:** al volver, el reintento desde la Bandeja de salida todavía los tiene. Lo cubre `TestPruneStaged_KeepsReferencedByOutbox` (Task 3).
3. **La misma imagen pegada dos veces** (y en la 3B, el logo de una firma): viaja una sola vez. Lo cubre `TestExtractInlineImages_DedupesIdenticalImages` (Task 1).
4. **HTML pegado desde una web o desde Word,** con estilos propios: los estilos del usuario ganan sobre los nuestros y nada se rompe. Lo cubre `TestEmailHTML_ExistingStylesWin` (Task 1).
5. **Escribir "/" en medio de una palabra** ("y/o", una URL): el menú no se abre. Lo cubre "y/o no abre el menú" (Task 5).

---

## File Structure

```
core/
  htmlx/email.go            # EmailHTML, ExtractInlineImages (Task 1)
  htmlx/totext.go           # separadores y listas numeradas (Task 1)
  mimex/build.go            # Outgoing.Inline → multipart/related (Task 2)
  model/model.go            # StagedAttachment, PickedImage; SendRequest/ComposeDraft nuevos campos (Tasks 2, 3)
  store/schema_v5.sql       # staged_attachments (Task 3)
  store/staged.go           # adjuntos preparados (Task 3)
  mail/compose.go           # deliver usa EmailHTML + inline + adjuntos (Tasks 2, 3)
  mail/attach.go            # StageFiles, LoadImage, StagedInfo, formatSize (Task 3)
app/
  mailservice.go            # PickAttachments, PickImage, StagedInfo (Task 4)
  filedrop.go               # FilesDropped → files-staged (Task 4)
main.go                     # EnableFileDrop + SetupFileDrop (Task 4)
frontend/src/features/compose/
  RichEditor.tsx            # extensiones, barra flotante, herramientas de tabla (Tasks 5, 6)
  slashCommands.ts          # ítems y filtro del menú "/" (Task 5)
  SlashMenu.tsx             # extensión + popup del menú (Task 5)
  editorImages.ts           # pegar e insertar imágenes (Task 6)
  AttachmentList.tsx        # chips de adjuntos (Task 7)
  Composer.tsx, ComposerContext.tsx   # adjuntos, reenvío, arrastrar (Task 7)
```

---

### Task 1: HTML de correo e imágenes en línea

**Files:**
- Create: `core/htmlx/email.go`, `core/htmlx/email_test.go`
- Modify: `core/htmlx/totext.go`, `core/htmlx/totext_test.go`

**Interfaces:**
- Produces:
  - **`func EmailHTML(fragment string) string`:** prepara el HTML del redactor para enviarlo.
    - Incluye lo que hoy hace `TightParagraphs`: `<p>` con `margin:0` y párrafos vacíos como `<p><br></p>`. `TightParagraphs` queda como helper interno o se borra, si nadie más lo usa.
    - Antepone estos estilos en línea al `style` existente (`nuestro;existente`, así gana el existente):

      | Elemento | Estilo |
      |---|---|
      | `h1` | `margin:16px 0 8px;font-size:24px;line-height:1.25;font-weight:bold` |
      | `h2` | `margin:14px 0 6px;font-size:20px;line-height:1.3;font-weight:bold` |
      | `h3` | `margin:12px 0 4px;font-size:16px;line-height:1.4;font-weight:bold` |
      | `blockquote` | `margin:8px 0;padding:0 0 0 12px;border-left:3px solid #d0d0d0;color:#555555` |
      | `ul`, `ol` | `margin:4px 0;padding-left:24px` |
      | `table` | `border-collapse:collapse;border:1px solid #d0d0d0;margin:8px 0` |
      | `td` | `border:1px solid #d0d0d0;padding:6px 10px;vertical-align:top;text-align:left` |
      | `th` | lo mismo que `td`, más `font-weight:bold;background-color:#f3f3f3` |
      | `hr` | `border:0;border-top:1px solid #d0d0d0;margin:12px 0` |
      | `img` | `max-width:100%;height:auto` |
      | `a` | `color:#2563eb;text-decoration:underline` |

    - Quita los `<colgroup>` que agrega la tabla de TipTap, porque algunos clientes los dibujan mal.
  - **`type InlineImage struct { CID, MIMEType string; Data []byte }`**
  - **`func ExtractInlineImages(fragment string, newCID func() string) (string, []InlineImage)`:**
    - Cada `<img src="data:image/(png|jpeg|gif|webp);base64,…">` válido pasa a `src="cid:<CID>"`.
    - Imágenes con los mismos bytes comparten un solo CID.
    - Un `<img>` con otro `data:`, o con base64 inválido, se quita del HTML.
    - Las imágenes `http(s)` y `cid:` quedan como están.
- **`ToText`:**
  - `<hr>` sale como un renglón `----------` entre líneas en blanco.
  - Los ítems de `<ol>` salen numerados (`1. `, `2. `…); los de `<ul>` siguen con `- `.

- [ ] **Step 1: Tests que fallan**

```go
func TestEmailHTML_InlineStylesPerElement(t *testing.T)           // h1..h3, blockquote, ul, table, td, th, hr, img, a llevan su estilo de la tabla
func TestEmailHTML_ExistingStylesWin(t *testing.T)                 // <p style="color:red"> → style="margin:0;color:red"; <h1 style="font-size:30px"> conserva 30px al final
func TestEmailHTML_KeepsTightParagraphs(t *testing.T)              // los casos de los tests actuales de TightParagraphs siguen pasando
func TestEmailHTML_DropsColgroup(t *testing.T)
func TestExtractInlineImages_ReplacesDataURIs(t *testing.T)        // 2 imágenes distintas → 2 InlineImage con MIMEType y bytes; src="cid:…"
func TestExtractInlineImages_DedupesIdenticalImages(t *testing.T)  // misma data URI 2 veces → 1 InlineImage; los dos src apuntan al mismo cid
func TestExtractInlineImages_DropsBadDataImages(t *testing.T)      // data:text/html, data:image/svg+xml y base64 roto → <img> quitado
func TestExtractInlineImages_LeavesRemoteAndCID(t *testing.T)
func TestToText_RulesAndNumberedLists(t *testing.T)                // <hr> → "----------"; <ol><li>a<li>b → "1. a\n2. b"
```

- [ ] **Step 2:** `go test ./core/htmlx/`. Debe fallar.
- [ ] **Step 3: Implementar** con `golang.org/x/net/html`, reusando `parseFragment` y `render`.
- [ ] **Step 4:** `go test -race ./core/htmlx/`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(htmlx): HTML compatible con email e imágenes en línea`

---

### Task 2: Mensajes con imágenes en línea

**Files:**
- Modify: `core/mimex/build.go`, `core/mimex/build_test.go`, `core/mail/compose.go`, `core/mail/compose_test.go`

**Interfaces:**
- Consumes: `htmlx.EmailHTML` y `htmlx.ExtractInlineImages` (Task 1).
- Produces:
  - **`type OutInline struct { CID, MIMEType string; Data []byte }`** y el campo **`Outgoing.Inline []OutInline`**.
  - **Estructura MIME** cuando hay imágenes en línea:
    ```
    multipart/alternative
      text/plain
      multipart/related
        text/html
        image/* (una por imagen)
    ```
    - Cada parte de imagen lleva `Content-ID: <CID>`, `Content-Disposition: inline; filename="imagen-N.<ext>"` y base64.
    - Con adjuntos, todo eso va dentro de `multipart/mixed`, como hoy.
    - Sin imágenes en línea, la salida es idéntica a la actual.
  - **`deliver`:**
    1. `body = bodyWrapperOpen + EmailHTML(req.BodyHTML) + "</div>" + cita`.
    2. `Text = ToText(body)`, calculado antes de extraer las imágenes.
    3. `html, imgs = ExtractInlineImages(body, func() string { return uuid.NewString() + "@postacontrol" })` sobre todo, cita incluida. Así las imágenes `cid` del original, que `GetMessage` entregó como `data:`, viajan bien al responder.

- [ ] **Step 1: Tests que fallan**

```go
func TestBuild_InlineImagesUseRelated(t *testing.T)        // parsear con go-message: alternative > [text/plain, related > [text/html, image/png con Content-ID]]
func TestBuild_InlineImagesWithAttachments(t *testing.T)   // mixed > [alternative > …, adjunto]
func TestBuild_NoInlineUnchanged(t *testing.T)             // sin Inline, misma estructura que antes
func TestSend_PastedImageTravelsAsCID(t *testing.T)        // testenv SMTP: BodyHTML con <img src="data:image/png;base64,…"> → el HTML recibido tiene src="cid:X" y existe la parte con Content-ID <X>
func TestSend_ReplyQuotesOriginalInlineImageAsCID(t *testing.T) // original con imagen cid → la cita del correo que sale vuelve a usar cid, no data:
func TestSend_HeadingsAndTablesGetInlineStyles(t *testing.T)
```

- [ ] **Step 2:** `go test ./core/mimex/ ./core/mail/ -run 'Inline|PastedImage|QuotesOriginal|Headings'`. Debe fallar.
- [ ] **Step 3: Implementar.** go-message no arma `related` dentro de `alternative` por sí solo. Usar `textproto`/`message.CreateWriter` de go-message para la parte `multipart/related` dentro de la `InlineWriter`.
- [ ] **Step 4:** `go test -race ./core/mimex/ ./core/mail/`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(mail): imágenes en línea como partes cid`

---

### Task 3: Adjuntos preparados

**Files:**
- Create: `core/store/schema_v5.sql`, `core/store/staged.go`, `core/store/staged_test.go`, `core/mail/attach.go`, `core/mail/attach_test.go`
- Modify: `core/store/migrate.go`, `core/model/model.go`, `core/mail/compose.go`, `core/mail/core.go` (limpieza al abrir), `core/mail/compose_test.go`

**Interfaces:**
- **`schema_v5.sql`:** `CREATE TABLE staged_attachments (id INTEGER PRIMARY KEY, filename TEXT NOT NULL, mime_type TEXT NOT NULL, size INTEGER NOT NULL, data BLOB NOT NULL, created_at INTEGER NOT NULL)`.
- Produces:
  - **En `model`:**
    - `type StagedAttachment struct { ID int64 \`json:"id"\`; Filename string \`json:"filename"\`; MIMEType string \`json:"mimeType"\`; Size int64 \`json:"size"\` }`
    - `type PickedImage struct { DataURL string \`json:"dataUrl"\`; Filename string \`json:"filename"\` }`, con `DataURL` vacío si se canceló.
    - `SendRequest.Attachments []int64 \`json:"attachments"\``: ids de adjuntos preparados.
    - `SendRequest.SkipOriginalAttachments []int \`json:"skipOriginalAttachments"\``: índices de adjuntos del original que no se reenvían.
    - `ComposeDraft.OriginalAttachments []Attachment \`json:"originalAttachments"\``: solo al reenviar; en los otros modos es vacío y no nil.
  - **En `store`:**
    - `StageAttachment(ctx, filename, mimeType string, data []byte) (model.StagedAttachment, error)`
    - `StagedAttachments(ctx, ids []int64) ([]StagedFile, error)`, con `type StagedFile struct { model.StagedAttachment; Data []byte }`. Respeta el orden de `ids` y devuelve `ErrNotFound` si falta alguno.
    - `StagedInfo(ctx, ids []int64) ([]model.StagedAttachment, error)`: sin datos; los que faltan se omiten.
    - `DeleteStagedAttachments(ctx, ids []int64) error`
    - `PruneStagedAttachments(ctx, olderThan time.Time) error`: borra los creados antes de `olderThan` que ninguna fila de `local_drafts` referencia (`json_each(request_json, '$.attachments')`). **La 3B va a sumar a este filtro los borradores.**
  - **En `mail`:**
    - `const MaxMessageBytes = 25 << 20`, `const MaxInlineImageBytes = 5 << 20`
    - **`(*Core).StageFiles(ctx, paths []string) ([]model.StagedAttachment, error)`:**
      - Valida todos antes de guardar ninguno: o se adjuntan todos, o ninguno.
      - Primero hace `os.Stat`, sin leer el archivo. Errores (`UserError`, código `attach`):
        - `«<nombre>» es una carpeta; solo se pueden adjuntar archivos.`
        - `«<nombre>» pesa <formatSize>; el máximo por correo es 25.0 MB.`
        - `No pudimos leer «<nombre>».`
      - El MIME sale de `mime.TypeByExtension`, con `http.DetectContentType` de respaldo. El nombre es `filepath.Base`.
    - **`(*Core).LoadImage(ctx, path string) (model.PickedImage, error)`:**
      - Detecta el tipo por contenido (`http.DetectContentType`). Errores (`UserError`, código `image`):
        - `Solo se pueden insertar imágenes PNG, JPG, GIF o WebP.`
        - `La imagen pesa <formatSize>; el máximo es 5.0 MB.`
    - `(*Core).StagedInfo(ctx, ids []int64) ([]model.StagedAttachment, error)`
    - **`enqueue`:**
      - Suma tres cosas: los tamaños de `req.Attachments`, los de los adjuntos del original no salteados (si reenvía) y los bytes decodificados de las `data:` del cuerpo.
      - Si pasa `MaxMessageBytes`: `UserError{Code: "too_large", Msg: "El correo pesa <formatSize> y el máximo es 25.0 MB. Sacá algún adjunto."}`.
      - Si un id no existe: `UserError{Code: "attach", Msg: "Uno de los adjuntos ya no está disponible. Volvé a adjuntarlo."}`.
      - En los dos casos no se guarda nada en la bandeja.
    - **`deliver`:**
      - Suma los adjuntos preparados después de los del original.
      - Saltea `SkipOriginalAttachments`.
      - Cuando el envío sale, borra los adjuntos preparados del pedido.
    - **`DeleteLocalDraft`:** borra también los adjuntos preparados de ese correo.
    - **Al abrir el núcleo,** junto a `FailInterruptedDrafts`: `PruneStagedAttachments(now - 24h)`. Así se limpia lo que quedó de redactores cancelados.

- [ ] **Step 1: Tests que fallan**

```go
func TestStageFiles_StoresAndReturnsInfo(t *testing.T)            // 2 archivos en t.TempDir → 2 StagedAttachment con nombre, tipo y tamaño
func TestStageFiles_KeepsNonASCIIName(t *testing.T)               // "presupuesto año 2026 — versión final.pdf"
func TestStageFiles_RejectsDirectoryAndTooLarge(t *testing.T)     // carpeta y archivo > 25 MB (os.Truncate, sin escribir 25 MB) → mensajes exactos; no se guarda ninguno
func TestLoadImage_TypesAndLimit(t *testing.T)                    // PNG → DataURL "data:image/png;base64,"; .txt renombrado a .png → error de tipo; > 5 MB → error de tamaño
func TestSend_WithStagedAttachments(t *testing.T)                 // testenv SMTP recibe los adjuntos; después StagedInfo devuelve vacío
func TestSend_TooLargeIsRejectedBeforeQueue(t *testing.T)         // ListLocalDrafts vacío; código too_large
func TestSend_ForwardSkipsOriginalAttachments(t *testing.T)       // original con 2 adjuntos, Skip [0] → sale solo el segundo
func TestSend_FailureKeepsStagedForRetry(t *testing.T)            // SMTP rechaza → el correo queda en la bandeja; StagedInfo sigue devolviéndolos
func TestPruneStaged_KeepsReferencedByOutbox(t *testing.T)        // viejo y referenciado → queda; viejo suelto → se borra; nuevo suelto → queda
func TestDeleteLocalDraft_DeletesItsStaged(t *testing.T)
func TestPrepareCompose_ForwardListsOriginalAttachments(t *testing.T)
```

- [ ] **Step 2:** `go test ./core/store/ ./core/mail/ -run 'Stage|LoadImage|WithStaged|TooLarge|Skips|FailureKeeps|Prune|DeleteLocalDraft|ForwardLists'`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test -race ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(mail): adjuntar archivos propios`

---

### Task 4: Diálogos y arrastrar archivos (app)

**Files:**
- Create: `app/filedrop.go`, `app/filedrop_test.go`
- Modify: `app/mailservice.go`, `main.go`, bindings generados

**Interfaces:**
- Consumes: `StageFiles`, `LoadImage` y `StagedInfo` (Task 3).
- Produces:
  - **`(*MailService).PickAttachments(ctx) ([]model.StagedAttachment, error)`:**
    - `Dialog.OpenFile()` con `CanChooseFiles(true)`, `SetTitle("Adjuntar archivos")` y `PromptForMultipleSelection()`, con la ventana actual como padre (igual que `SaveAttachment`).
    - Cancelar devuelve un slice vacío, sin error.
  - **`(*MailService).PickImage(ctx) (model.PickedImage, error)`:**
    - Un solo archivo, con `SetTitle("Insertar imagen")` y `AddFilter("Imágenes", "*.png;*.jpg;*.jpeg;*.gif;*.webp")`.
    - Cancelar devuelve un `PickedImage` vacío.
  - `(*MailService).StagedInfo(ctx, ids []int64) ([]model.StagedAttachment, error)`
  - **Arrastrar archivos:**
    - `const ComposeDropTarget = "compose-drop"` y `const EventFilesStaged = "files-staged"`.
    - Payload: `type FilesStagedPayload struct { Attachments []model.StagedAttachment \`json:"attachments"\`; Error string \`json:"error"\` }`.
    - `func SetupFileDrop(win *application.WebviewWindow, core *mail.Core, emit func(event string, data any))` registra `WindowFilesDropped`.
    - La lógica pura vive en `func handleDrop(ctx, stage func(context.Context, []string) ([]model.StagedAttachment, error), targetID string, paths []string) (FilesStagedPayload, bool)`:
      - devuelve `false` si `targetID != ComposeDropTarget` o si no hay rutas;
      - si `StageFiles` falla, `Error` lleva el mensaje del `UserError`.
    - El manejador corre `handleDrop` en una goroutine, para no frenar la UI con archivos grandes, y emite el evento.
  - **`main.go`:** `EnableFileDrop: true` en `WebviewWindowOptions` y llamada a `app.SetupFileDrop`.

- [ ] **Step 1: Tests que fallan**

```go
func TestHandleDrop_IgnoresOtherTargets(t *testing.T)   // targetID "" o "otro" → false, no llama a stage
func TestHandleDrop_StagesAndReportsErrors(t *testing.T) // stage ok → Attachments; stage con UserError → Error = su Msg
```

- [ ] **Step 2:** `go test ./app/ -run HandleDrop`. Debe fallar.
- [ ] **Step 3: Implementar.** Regenerar los bindings con `task common:generate:bindings`.
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(app): elegir y arrastrar archivos para adjuntar`

---

### Task 5: Menú "/", barra de formato y tablas

**Files:**
- Create: `frontend/src/features/compose/slashCommands.ts`, `slashCommands.test.ts`, `SlashMenu.tsx`
- Modify: `frontend/src/features/compose/RichEditor.tsx`, `RichEditor.test.tsx`, `frontend/package.json`

**Interfaces:**
- **Dependencias:** `npm install @tiptap/extension-table@3.32.2 @tiptap/extension-image@3.32.2 @tiptap/suggestion@3.32.2 @tiptap/extension-file-handler@3.32.2 @floating-ui/dom`. Si `minimum-release-age` del `.npmrc` frena una versión, usar la última que acepte, igual para todos los `@tiptap/*`.
- Produces:
  - **`slashCommands.ts`:**
    - `type SlashContext = { pickImage: () => void; openLink: () => void }`
    - `type SlashItem = { id: string; title: string; keywords: string[]; run: (editor: Editor, ctx: SlashContext) => void }`
    - `export const SLASH_ITEMS: SlashItem[]`: los 10 ítems de Global Constraints, en ese orden.
      - "Tabla" inserta una tabla de 3×3 con fila de encabezado.
      - "Imagen" llama a `ctx.pickImage()`; "Enlace" llama a `ctx.openLink()`.
    - `export function filterSlashItems(query: string): SlashItem[]`: compara sin acentos ni mayúsculas contra el título y las palabras clave, que incluyen al menos "titulo"/"encabezado", "vinetas"/"lista", "numerada", "cita", "tabla", "imagen"/"foto", "separador"/"linea", "enlace"/"link".
  - **`SlashMenu.tsx`:**
    - `export function createSlashCommand(getCtx: () => SlashContext): Extension`, sobre `@tiptap/suggestion` con `char: "/"`. Se usa el `allowedPrefixes` por defecto, que solo abre después de un espacio o al principio del renglón.
    - El popup es React (`ReactRenderer`), posicionado con `@floating-ui/dom`, y lista los ítems filtrados.
    - Teclado: ↑/↓ mueven la selección de forma circular, Enter ejecuta, Esc cierra. Sin coincidencias muestra "Sin resultados".
    - Al ejecutar, primero se borra el texto `/consulta`.
  - **`RichEditor`:**
    - Suma `Table` (con `resizable: false`), `TableRow`, `TableHeader`, `TableCell` y el comando "/".
    - El `LinkButton` pasa a ser un popover controlado (`open` / `onOpenChange`) que también abre `ctx.openLink`.
    - `BubbleMenu` (`@tiptap/react/menus`) aparece con texto seleccionado (no con una imagen seleccionada ni con selección de celdas) y tiene los 6 botones de Global Constraints. "Quitar formato" corre `unsetAllMarks().clearNodes()`.
    - Con el cursor dentro de una tabla, la barra fija muestra "Agregar fila", "Agregar columna", "Eliminar fila", "Eliminar columna" y "Eliminar tabla".
    - Estilos en el editor para `h1`–`h3`, `blockquote`, `table`/`td`/`th`, `hr` e `img`, que se parezcan a lo que sale en el correo.
    - **Props nuevas:** `onPickImage?: () => void`, que la Task 6 conecta. Mientras no haya prop, el ítem "Imagen" no aparece.

- [ ] **Step 1: Tests que fallan**
  - **`slashCommands.test.ts`:**
    - "titulo" devuelve los 3 títulos
    - "VIÑETAS" devuelve "Lista con viñetas"
    - "zzz" devuelve vacío
    - el orden de `SLASH_ITEMS` es el de Global Constraints
  - **`RichEditor.test.tsx`:**
    - "escribir / abre el menú con los ítems"
    - "/tab filtra a Tabla y Enter inserta una tabla" (el HTML de `onChange` contiene `<table>` y `<th>`)
    - "Esc cierra el menú y deja el texto"
    - "y/o no abre el menú"
    - "/titulo 1 + Enter convierte el renglón en h1"
    - "con el cursor en una tabla aparecen Agregar fila y Eliminar tabla"
    - "Agregar fila suma un tr"
- [ ] **Step 2:** `npx vitest run src/features/compose`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): menú "/", barra de formato y tablas en el redactor`

---

### Task 6: Imágenes en el editor

**Files:**
- Create: `frontend/src/features/compose/editorImages.ts`, `editorImages.test.ts`
- Modify: `frontend/src/features/compose/RichEditor.tsx`, `RichEditor.test.tsx`, `Composer.tsx`

**Interfaces:**
- Consumes: el binding `PickImage` (Task 4) y `onPickImage` (Task 5).
- Produces:
  - **`editorImages.ts`:**
    - `export const IMAGE_TYPES = ["image/png", "image/jpeg", "image/gif", "image/webp"]`
    - `export const MAX_INLINE_IMAGE_BYTES = 5 * 1024 * 1024`
    - `export function imageError(file: { type: string; size: number }): string | null`, que devuelve los mismos textos que `LoadImage` (Task 3), con `formatSize`.
    - `export function readAsDataURL(file: File): Promise<string>`
  - **Extensión de imagen:** `Image.configure({ inline: false, allowBase64: true })`, con `parseHTML` restringido a `img[src^="data:image/"]`. Una imagen remota pegada no entra al documento.
  - **Pegar:** `FileHandler` con `allowedMimeTypes: IMAGE_TYPES` y `onPaste`. Valida con `imageError` e inserta en la posición del cursor. Los errores van a `onError`.
  - **`RichEditor`:**
    - prop nueva `onError?: (msg: string) => void`;
    - botón "Imagen" (`ImageIcon`) en la barra fija;
    - `onPickImage` del ítem "/" y del botón;
    - una imagen seleccionada se ve con un borde y se borra con Supr o Retroceso, que ya es el comportamiento de TipTap.
  - **`Composer`:**
    - conecta `onPickImage` con `svc.PickImage()` e inserta `setImage({ src: dataUrl, alt: filename })` si no se canceló;
    - conecta `onError` con su `Alert` de error.

- [ ] **Step 1: Tests que fallan**
  - **`editorImages.test.ts`:**
    - PNG de 1 KB → `null`
    - `application/pdf` → mensaje de tipo
    - 6 MB → "La imagen pesa 6.0 MB; el máximo es 5.0 MB."
  - **`RichEditor.test.tsx`:**
    - "pegar HTML con <img src=https://…> no agrega la imagen"
    - "el contenido con <img src=data:image/png…> se conserva"
    - "el botón Imagen llama a onPickImage"
  - **`Composer.test.tsx`:**
    - "insertar imagen agrega el img al cuerpo enviado" (mock `PickImage` → `QueueSend` recibe `bodyHtml` con `data:image/png`)
    - "cancelar el diálogo no cambia nada"
- [ ] **Step 2:** `npx vitest run src/features/compose`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): imágenes en el redactor`

---

### Task 7: Adjuntos en el redactor

**Files:**
- Create: `frontend/src/features/compose/AttachmentList.tsx`
- Modify: `frontend/src/features/compose/Composer.tsx`, `Composer.test.tsx`, `ComposerContext.tsx`, `frontend/src/lib/events.ts`

**Interfaces:**
- Consumes: los bindings `PickAttachments` y `StagedInfo`, el evento `files-staged` con `{attachments, error}` y el id `compose-drop` (Task 4), y `ComposeDraft.originalAttachments` y `SendRequest.attachments` / `skipOriginalAttachments` (Task 3).
- Produces:
  - **`ComposerInit`** suma tres campos:
    - `attachments: StagedAttachment[]`
    - `originalAttachments: Attachment[]`, solo al reenviar
    - `skipOriginalAttachments: number[]`
  - **`openFrom("forward")`** carga `originalAttachments`. **`openDraft`** pasa a ser async: busca los nombres con `StagedInfo(d.request.attachments)` y conserva `skipOriginalAttachments`.
  - **`AttachmentList`:**
    - Recibe `{ items: { key: string; filename: string; size: number }[]; onRemove(key) }`.
    - Cada chip muestra un ícono, el nombre (truncado, con `title` completo), `formatSize` y un botón con la etiqueta accesible `Quitar <nombre>`.
    - Se muestra debajo del asunto, solo si hay ítems.
  - **`Composer`:**
    - Botón "Adjuntar" (`PaperclipIcon`) a la izquierda del pie: llama a `PickAttachments` y suma lo elegido.
    - Quitar un chip lo saca del estado; el núcleo limpia lo que quedó suelto (Task 3).
    - Los adjuntos del original reenviado aparecen como chips. Quitarlos suma su `index` a `skipOriginalAttachments`.
    - Total = adjuntos preparados + originales no salteados. Si pasa 25 MB, muestra en rojo `Los adjuntos suman <formatSize>; el máximo es 25.0 MB.` y deshabilita "Enviar".
    - **Arrastrar:**
      - `DialogContent` lleva `id="compose-drop"` y `data-file-drop-target`.
      - Con la clase `file-drop-target-active` (la pone el runtime de Wails), una capa encima dice "Soltá los archivos para adjuntarlos".
      - El redactor escucha `files-staged` mientras está abierto: suma los adjuntos o muestra el `error` en su `Alert`.
    - Cambiar los adjuntos marca el redactor como modificado (pide confirmar antes de descartar).
    - `QueueSend` lleva `attachments` (ids) y `skipOriginalAttachments`.
  - **`lib/events.ts`:** `onFilesStaged(cb: (p: { attachments: StagedAttachment[]; error: string }) => void): () => void`.

- [ ] **Step 1: Tests que fallan** (en `Composer.test.tsx`)
  - "Adjuntar suma chips y QueueSend lleva sus ids"
  - "quitar un chip lo saca del envío"
  - "cancelar el diálogo no suma nada"
  - "reenviar muestra los adjuntos del original y quitarlos los manda en skipOriginalAttachments"
  - "más de 25 MB muestra el aviso y deshabilita Enviar"
  - "files-staged suma adjuntos; con error muestra el mensaje"
  - "reabrir un correo de la Bandeja de salida muestra sus adjuntos"
  - "adjuntar marca el redactor como modificado"
- [ ] **Step 2:** `npx vitest run src/features/compose`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): adjuntos en el redactor`

---

### Task 8: Prueba de aceptación (Fabricio)

Con la app instalada (`task install:desktop`). Para ver cómo llega, mandate los correos de prueba a una cuenta de Gmail.

- [ ] **Menú "/":** en un correo nuevo, escribí "/" y armá un correo con título, lista numerada, cita, tabla y separador. Escribir "y/o" no abre el menú.
- [ ] **Barra al seleccionar:** seleccioná una palabra y aplicale negrita, subrayado y un enlace. "Quitar formato" la deja limpia.
- [ ] **Imágenes:** pegá una captura de pantalla con Ctrl+V e insertá otra con "/imagen". En Gmail se ven las dos dentro del cuerpo, no como adjuntos.
- [ ] **Cómo se ve:** en Gmail y en el webmail de Hostinger, los títulos, la tabla con bordes y la cita se ven parecidos al redactor. La versión en texto plano (en Gmail, "Mostrar original") se lee bien.
- [ ] **Adjuntos:**
  - Adjuntá dos archivos con "Adjuntar" y arrastrá un tercero desde el administrador de archivos. Uno tiene que tener acentos o ñ en el nombre.
  - Quitá uno antes de enviar.
  - En Gmail llegan los que quedaron, con el nombre bien.
- [ ] **Reenviar:** reenviá un correo con adjuntos y sacale uno: llega sin ese.
- [ ] **Límite:** un archivo de más de 25 MB muestra el aviso y no se adjunta.
- [ ] **Bandeja de salida:**
  - Con el wifi apagado, enviá un correo con un adjunto: queda en la Bandeja de salida.
  - Cerrá la app del todo (Salir) y volvé a abrirla.
  - Reabrí el correo: el adjunto sigue ahí y, con conexión, sale bien.

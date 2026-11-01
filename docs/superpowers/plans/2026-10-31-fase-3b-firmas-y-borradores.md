# PostaControl — Fase 3B "Firmas y borradores" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Fabricio tenga una biblioteca de firmas globales con una firma por defecto por cuenta, y que nada de lo que escribe se pierda: los correos se guardan solos mientras escribe, en la compu y en la carpeta Borradores del servidor, y se retoman desde ahí.

**Architecture:**
- **Firmas:**
  - Viven en SQLite, que para firmas es la fuente de verdad (spec). Cada cuenta apunta a una firma por defecto o a ninguna.
  - En el redactor, la firma **no** está dentro del texto que escribís: se elige con un selector y se muestra debajo del editor, como la cita.
  - Al enviar se arma `cuerpo + firma + cita`. Así cambiar de firma o de cuenta nunca pisa lo escrito.
- **Armado del mensaje:** `deliver` se parte en un `buildOutgoing` compartido. Con él se arma tanto el correo que sale como la copia del borrador que se sube al servidor.
- **Borradores:**
  - Tabla nueva `compose_drafts`, separada de la bandeja de salida (`local_drafts`).
  - El redactor guarda en la compu a los 1,5 s de dejar de escribir, y sube al servidor a los 30 s de dejar de escribir y al cerrarse.
  - **Cada borrador tiene un `Message-ID` fijo.** Para reemplazar la versión anterior en el servidor:
    1. `APPEND` de la nueva;
    2. `UID SEARCH HEADER Message-ID`;
    3. se borran todas menos la de UID más alto, con `UID EXPUNGE` (o solo `\Deleted` si el servidor no tiene UIDPLUS).
  - Los que no pudieron subir se reintentan en la pasada de sincronización de fondo.

**Tech Stack:** el de las fases anteriores. No hay dependencias nuevas.

**Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`, secciones "Firmas", "Redactor", "Sincronización… sin conexión" (lo redactado no se pierde) y "Fase 3". Sigue a la 3A (`2026-10-31-fase-3a-redactor-rico.md`) y toma sus pendientes del registro `2026-10-31-fase-3a-registro.md`: el ancho de las imágenes en Outlook, la limpieza de adjuntos al descartar y la limpieza que considera los borradores.

## Global Constraints

- **Siguen vigentes** todas las de las fases 1, 2 y 3A:
  - `core/` sin Wails; tests solo con `core/testenv`; TLS verificado; contraseñas solo en el llavero.
  - Español rioplatense con voseo.
  - `task check` verde antes de cada commit.
  - Commits convencionales en español con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Bindings con `task common:generate:bindings`.
- **Rama:** `fase-3b`, desde `main`.
- **Escrituras en el servidor que agrega esta fase.** Son solo dos, ambas en la carpeta con rol `drafts` de la cuenta del borrador:
  - `APPEND` con `\Draft \Seen`;
  - el borrado de versiones viejas **del mismo borrador**, identificadas por su `Message-ID`, con `UID EXPUNGE`.
  - **Nunca EXPUNGE a secas.** Sin UIDPLUS se deja `\Deleted` y nada más.
  - Si la cuenta no tiene carpeta Borradores, el borrador queda solo en la compu.
- **Tiempos del redactor:** `DRAFT_LOCAL_DEBOUNCE_MS = 1500` y `DRAFT_UPLOAD_DEBOUNCE_MS = 30000`.
- **Un redactor vacío nunca crea borrador.** Vacío quiere decir: sin destinatarios, sin asunto, sin cuerpo y sin adjuntos.
- **El redactor reabierto desde la Bandeja de salida** (reintento de envío, `draftId`) **no** autoguarda y mantiene su comportamiento actual.
- **Textos exactos:**
  - Estado del borrador en el pie del redactor: "Guardando…", "Guardado", "Guardado solo en esta compu".
  - Al cerrar con contenido: toast "Guardado en Borradores."
  - Botón "Descartar". Su confirmación: título "¿Descartar este borrador?" y texto "Se borra también de la carpeta Borradores del servidor."
  - Acción del lector en borradores: "Seguir editando".
  - Selector: etiqueta "Firma", opción "Sin firma".
  - Configuración: sección "Firmas", botón "Nueva firma", campo "Nombre", selector "Firma por defecto" por cuenta.
  - Aviso en las vistas de borradores: "<N> borrador(es) todavía no se subieron al servidor." (singular: "1 borrador todavía no se subió al servidor.").
- **Firma en el correo:**
  - Va entre el cuerpo y la cita, envuelta en `<div style="margin-top:16px">`.
  - Pasa por `EmailHTML`, y sus imágenes viajan como `cid`; el logo repetido se deduplica.
  - En el texto plano va precedida por un renglón vacío, sin el separador `-- `, como hace Outlook.

## Review Focus

1. **Escribir sin conexión y cerrar la app:** el borrador sigue en la compu y, al volver la conexión, sube solo, una sola vez, sin duplicados en el servidor. Lo cubren `TestDrafts_UploadPendingRetriesInBackground` y `TestDrafts_ReuploadReplacesPreviousVersion` (Task 3).
2. **Enviar un correo que venía de un borrador:** el borrador desaparece de la compu y del servidor, pero sus adjuntos siguen vivos para la bandeja de salida hasta que el envío sale. Lo cubre `TestSend_FromDraftRemovesDraftKeepsStaged` (Task 3).
3. **Borrar una firma que es la por defecto de una cuenta:** la cuenta queda sin firma y los redactores abiertos con esa firma no rompen el envío (firma inexistente = sin firma). Lo cubren `TestSignatures_DeleteClearsAccountDefault` (Task 1) y `TestSend_MissingSignatureSendsWithout` (Task 2).
4. **Un borrador creado en el webmail o en el celular,** con adjuntos y CCO: al seguir editándolo conserva destinatarios, CCO y adjuntos, y al guardarlo reemplaza al original en el servidor. Lo cubre `TestOpenDraftMessage_ForeignDraftKeepsBccAndAttachments` (Task 3).
5. **Cambiar la cuenta "De" con la firma sin tocar:** pasa a la firma por defecto de la nueva cuenta. Si el usuario eligió una firma a mano, se respeta. Lo cubre "cambiar De cambia la firma salvo que se haya elegido a mano" (Task 5).

---

## File Structure

```
core/
  model/model.go            # Signature, Draft; Account.DefaultSignatureID; SendRequest.SignatureID, ComposeDraftID, DraftMessageID (Tasks 1-3)
  store/schema_v6.sql       # signatures, accounts.default_signature_id, compose_drafts (Task 1)
  store/signatures.go       # CRUD de firmas (Task 1)
  store/composedrafts.go    # borradores (Task 1)
  store/staged.go           # Prune considera compose_drafts (Task 1)
  mimex/build.go            # Outgoing.IncludeBcc para borradores (Task 2)
  htmlx/email.go            # ancho de imágenes anchas (Task 2)
  mail/outgoing.go          # buildOutgoing compartido (Task 2)
  mail/signatures.go        # API de firmas (Task 2)
  mail/drafts.go            # SaveDraft, UploadDraft, DeleteDraft, OpenDraftMessage, pendientes (Task 3)
  imapx/search.go           # SearchHeader (Task 3)
app/mailservice.go          # bindings (Tasks 2, 3)
frontend/src/features/
  settings/SignaturesSection.tsx, SignatureDialog.tsx   # (Task 4)
  compose/SignaturePicker.tsx                           # (Task 5)
  compose/useDraftAutosave.ts                           # (Task 6)
  compose/Composer.tsx, ComposerContext.tsx             # (Tasks 5, 6)
  reader/MessageActions.tsx                             # "Seguir editando" (Task 6)
  messages/UnsyncedDraftsBanner.tsx                     # (Task 6)
```

---

### Task 1: Base de datos de firmas y borradores

**Files:**
- Create: `core/store/schema_v6.sql`, `core/store/signatures.go`, `core/store/composedrafts.go`, `core/store/signatures_test.go`, `core/store/composedrafts_test.go`
- Modify: `core/store/migrate.go`, `core/store/accounts.go`, `core/store/staged.go`, `core/store/staged_test.go`, `core/model/model.go`

**Interfaces:**
- **`schema_v6.sql`:**
  - `CREATE TABLE signatures (id INTEGER PRIMARY KEY, name TEXT NOT NULL, html TEXT NOT NULL, updated_at INTEGER NOT NULL)`
  - `ALTER TABLE accounts ADD COLUMN default_signature_id INTEGER REFERENCES signatures(id) ON DELETE SET NULL`. SQLite admite `ALTER … ADD COLUMN … REFERENCES` con default NULL; `foreign_keys(1)` ya está activo.
  - `CREATE TABLE compose_drafts (id INTEGER PRIMARY KEY, account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, request_json TEXT NOT NULL, message_id TEXT NOT NULL UNIQUE, needs_upload INTEGER NOT NULL DEFAULT 1, upload_error TEXT NOT NULL DEFAULT '', updated_at INTEGER NOT NULL)`
- Produces:
  - **En `model`:**
    - `type Signature struct { ID int64 \`json:"id"\`; Name string \`json:"name"\`; HTML string \`json:"html"\` }`
    - `Account.DefaultSignatureID int64 \`json:"defaultSignatureId"\``, donde 0 quiere decir sin firma.
    - `type Draft struct { ID int64 \`json:"id"\`; Request SendRequest \`json:"request"\`; MessageID string \`json:"messageId"\`; NeedsUpload bool \`json:"needsUpload"\`; UploadError string \`json:"uploadError"\`; UpdatedAt time.Time \`json:"updatedAt"\` }`
    - En `SendRequest`, tres campos nuevos:
      - `SignatureID int64 \`json:"signatureId"\``
      - `ComposeDraftID int64 \`json:"composeDraftId"\``
      - `DraftMessageID string \`json:"draftMessageId"\``, el `Message-ID` del borrador del que viene el envío.
  - **En `store`:**
    - `ListSignatures(ctx) ([]model.Signature, error)`, ordenadas por nombre.
    - `GetSignature(ctx, id) (model.Signature, error)`, con `ErrNotFound` si no existe.
    - `SaveSignature(ctx, s model.Signature) (model.Signature, error)`: crea si `ID == 0` y actualiza si no.
    - `DeleteSignature(ctx, id) error`
    - `SetAccountSignature(ctx, accountID, signatureID int64) error`, donde 0 la deja en NULL. `ListAccounts` y `GetAccount` devuelven `DefaultSignatureID`.
    - **`SaveComposeDraft(ctx, id int64, accountID int64, req model.SendRequest, messageID string) (int64, error)`:** crea o actualiza, y deja `needs_upload = 1` y `upload_error = ''`.
    - `GetComposeDraft(ctx, id) (model.Draft, error)`
    - `ComposeDraftByMessageID(ctx, messageID string) (model.Draft, error)`
    - `ListPendingDrafts(ctx) ([]model.Draft, error)`: `needs_upload = 1`, el más nuevo primero.
    - `MarkDraftUploaded(ctx, id, uploadedAt time.Time) error`: pone `needs_upload = 0` **solo si** `updated_at <= uploadedAt`. Así un guardado que llegó durante la subida no se pierde.
    - `MarkDraftUploadFailed(ctx, id, msg string) error`
    - `DeleteComposeDraft(ctx, id) error`: borra solo la fila.
  - **`PruneStagedAttachments`:** además de `local_drafts`, no borra los adjuntos referenciados por `compose_drafts` (`json_each(request_json, '$.attachments')`).

- [ ] **Step 1: Tests que fallan**

```go
func TestSignatures_CRUDAndOrder(t *testing.T)
func TestSignatures_DeleteClearsAccountDefault(t *testing.T)    // cuenta con default = X; DeleteSignature(X) → GetAccount.DefaultSignatureID == 0
func TestSetAccountSignature_ZeroClears(t *testing.T)
func TestComposeDrafts_SaveUpdateAndLookup(t *testing.T)        // segunda Save con el mismo id actualiza; ComposeDraftByMessageID lo encuentra
func TestComposeDrafts_MarkUploadedIgnoresStale(t *testing.T)   // Save(t=10), MarkUploaded(uploadedAt=5) → sigue pendiente; MarkUploaded(20) → ya no
func TestComposeDrafts_PendingList(t *testing.T)
func TestComposeDrafts_DeleteAccountCascades(t *testing.T)
func TestPruneStaged_KeepsReferencedByComposeDraft(t *testing.T)
func TestMigrateV6_FromV5(t *testing.T)                         // base en v5 con una cuenta → la cuenta queda con DefaultSignatureID 0
```

- [ ] **Step 2:** `go test ./core/store/ -run 'Signature|ComposeDrafts|PruneStaged|MigrateV6'`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test -race ./core/store/` y `task check`. Deben dar PASS.
- [ ] **Step 5: Commit** `feat(store): firmas y borradores en la base local`

---

### Task 2: Firmas en el correo que sale

**Files:**
- Create: `core/mail/outgoing.go`, `core/mail/signatures.go`, `core/mail/signatures_test.go`
- Modify: `core/mail/compose.go`, `core/mail/compose_test.go`, `core/mimex/build.go`, `core/mimex/build_test.go`, `core/htmlx/email.go`, `core/htmlx/email_test.go`, `app/mailservice.go`, bindings

**Interfaces:**
- Consumes: el store de la Task 1.
- Produces:
  - **`func (c *Core) buildOutgoing(ctx context.Context, acc model.Account, req model.SendRequest, forDraft bool) (mimex.Outgoing, error)`:**
    - Es todo lo que hoy arma `deliver` antes de `mimex.Build`: original, cuerpo, cita, imágenes, adjuntos, `In-Reply-To` y `References`.
    - Suma la firma de `req.SignatureID` entre el cuerpo y la cita, según Global Constraints. Una firma que ya no existe se trata como sin firma.
    - Con `forDraft`: `MessageID = req.DraftMessageID` e `IncludeBcc = true`.
    - `deliver` pasa a usarla, sin cambiar su comportamiento.
  - **`mimex.Outgoing.IncludeBcc bool`:** con `true` escribe el encabezado `Bcc`, que solo se usa en borradores. Sin él, igual que hoy, `Bcc` nunca se escribe.
  - **`checkSize`** suma también las imágenes `data:` de la firma.
  - **Pendiente de la 3A (ancho en Outlook):** `ExtractInlineImages` agrega `width="600"` a las imágenes PNG, JPEG o GIF cuyo ancho natural (`image.DecodeConfig`) supera 600 px y que no traen `width`. WebP queda como está.
  - **En `Core`:**
    - `ListSignatures(ctx) ([]model.Signature, error)`
    - `SaveSignature(ctx, s model.Signature) (model.Signature, error)`
      - Valida que el nombre no esté vacío: `UserError{Code: "invalid", Msg: "Poné un nombre para la firma."}`.
      - Pasa la firma por `htmlx.Sanitize` antes de guardarla, para que no entre nada peligroso.
    - `DeleteSignature(ctx, id) error`
    - `SetAccountSignature(ctx, accountID, signatureID int64) error`
    - Todos emiten `EventSignaturesChanged = "mail:signatures-changed"`. `SetAccountSignature` emite además el evento de cuentas existente.
  - **`PrepareCompose`:** `ComposeDraft` suma `SignatureID int64 \`json:"signatureId"\``, con la firma por defecto de la cuenta.
  - **Bindings** en `MailService`: `ListSignatures`, `SaveSignature`, `DeleteSignature` y `SetAccountSignature`.

- [ ] **Step 1: Tests que fallan**

```go
func TestSend_SignatureBetweenBodyAndQuote(t *testing.T)    // respuesta con firma → HTML: cuerpo, luego la firma, luego la cita; texto plano también en ese orden
func TestSend_SignatureLogoTravelsAsCID(t *testing.T)       // firma con data:image/png → parte cid; la misma imagen en cuerpo y firma → una sola parte
func TestSend_MissingSignatureSendsWithout(t *testing.T)
func TestBuild_IncludeBccOnlyForDrafts(t *testing.T)
func TestExtractInlineImages_WideImagesGetWidth(t *testing.T) // PNG 1200 px → width="600"; 300 px → sin width; con width propio → se respeta
func TestSaveSignature_ValidatesAndSanitizes(t *testing.T)  // nombre vacío → error; <script> se quita; data:image se conserva
func TestPrepareCompose_UsesAccountDefaultSignature(t *testing.T)
```

- [ ] **Step 2:** `go test ./core/... -run 'Signature|IncludeBcc|WideImages|PrepareCompose_Uses'`. Debe fallar.
- [ ] **Step 3: Implementar.** Regenerar los bindings.
- [ ] **Step 4:** `go test -race ./core/... ./app/` y `task check`. Deben dar PASS.
- [ ] **Step 5: Commit** `feat(mail): firmas globales en el correo`

---

### Task 3: Borradores en la compu y en el servidor

**Files:**
- Create: `core/mail/drafts.go`, `core/mail/drafts_test.go`
- Modify: `core/imapx/search.go`, `core/imapx/search_test.go`, `core/mail/compose.go`, `core/mail/sync.go` (pasada de fondo), `app/mailservice.go`, bindings, `core/testenv` si hace falta soporte de `UID SEARCH HEADER` o de UIDPLUS en el servidor de prueba

**Interfaces:**
- Consumes: `buildOutgoing(…, forDraft=true)` (Task 2) y el store de la Task 1.
- Produces:
  - **`(*imapx.Conn).SearchHeader(name, value string) ([]uint32, error)`:** en la carpeta ya abierta, con los UID en orden ascendente.
  - **`(*Core).SaveDraft(ctx, id int64, req model.SendRequest) (model.Draft, error)`:**
    - Solo local, no toca la red.
    - Si `id == 0`, crea el borrador con un `Message-ID` nuevo (`mimex.NewMessageID(acc.Email)`) y lo devuelve.
    - Valida la cuenta y nada más: un borrador puede no tener destinatarios.
  - **`(*Core).UploadDraft(ctx, id int64) (model.Draft, error)`:**
    1. Arma el MIME con `buildOutgoing(forDraft)`. Antes pone `req.DraftMessageID = d.MessageID`: el `Message-ID` sale siempre de la fila, no del pedido que mandó la UI.
    2. Busca la carpeta con rol `drafts`. Si no hay: `MarkDraftUploadFailed(id, "Esta cuenta no tiene carpeta Borradores en el servidor.")` y devuelve el borrador sin error.
    3. `NOOP` y después `APPEND` sin reintento, con `\Draft` y `\Seen`, igual que `appendSent`.
    4. `SelectRW` de Borradores y `SearchHeader("Message-ID", id)`.
    5. Borra todos los UID menos el mayor, con `ExpungeUIDs`; si es `ErrUnsupported`, `StoreFlags(\Deleted)`.
    6. `MarkDraftUploaded(id, inicioDeLaSubida)`, sincroniza la carpeta Borradores (mejor esfuerzo) y emite `EventFolderUpdated`.
    - Un error de red llama a `MarkDraftUploadFailed` con el mensaje del `UserError` y devuelve el borrador marcado, sin error: es autoguardado y la UI solo muestra el estado.
  - **`(*Core).DeleteDraft(ctx, id int64) error`:**
    - Borra las copias del servidor con su `Message-ID`, igual que en el paso 5.
    - Borra la fila y los adjuntos preparados que solo ese borrador referenciaba (no los de la bandeja de salida).
    - Si el servidor no responde, borra lo local igual y devuelve `UserError{Code: "draft", Msg: "Borramos el borrador de esta compu, pero no pudimos borrarlo del servidor."}`.
  - **`(*Core).OpenDraftMessage(ctx, messageID int64) (model.Draft, error)`:**
    - El mensaje tiene que estar en una carpeta con rol `drafts`; si no: `UserError{Code: "invalid", Msg: "Ese correo no es un borrador."}`.
    - Si su `Message-ID` coincide con un `compose_drafts`, devuelve ese.
    - Si no, crea uno con `De`, `Para`, `CC`, `CCO` (del encabezado), asunto, cuerpo (`GetMessage(...).HTML`, o el texto escapado en `<p>`) y adjuntos (preparados con `attachmentBytes`). Su `Message-ID` es el del mensaje, así el próximo `UploadDraft` lo reemplaza.
    - La cuenta es la de la carpeta. Sin contexto de respuesta.
  - **`(*Core).ListPendingDrafts(ctx) ([]model.Draft, error)`**
  - **Envío desde un borrador:**
    - `enqueue`, con `req.ComposeDraftID != 0`, borra la fila del borrador **después** de encolar, y no borra sus adjuntos (ahora los referencia la bandeja de salida).
    - `deliver`, cuando el envío sale y hay `DraftMessageID`, borra sus copias del servidor (mejor esfuerzo).
  - **`backgroundPass`:** además de sincronizar, llama a `UploadDraft` para cada `ListPendingDrafts` de las cuentas que no están en pausa.
  - **Emite** `EventDraftsChanged = "mail:drafts-changed"` después de `SaveDraft`, `UploadDraft` y `DeleteDraft`.
  - **Bindings:** `SaveDraft`, `UploadDraft`, `DeleteDraft`, `OpenDraftMessage` y `ListPendingDrafts`.
- **Hallazgo de la 3A:** "limpieza de adjuntos al descartar" queda cubierto por `DeleteDraft`.

- [ ] **Step 1: Tests que fallan**

```go
func TestSearchHeader_FindsByMessageID(t *testing.T)
func TestDrafts_SaveIsLocalOnly(t *testing.T)                     // sin servidor arriba → ok, NeedsUpload true
func TestDrafts_UploadAppendsWithDraftFlagAndBcc(t *testing.T)    // testenv: un mensaje en Borradores con \Draft, Bcc presente, Message-ID del borrador
func TestDrafts_ReuploadReplacesPreviousVersion(t *testing.T)     // 3 subidas → queda 1 mensaje en el servidor
func TestDrafts_UploadWithoutUIDPlusOnlyFlagsDeleted(t *testing.T) // StartIMAPWithCaps sin UIDPLUS → las viejas quedan \Deleted, sin EXPUNGE
func TestDrafts_UploadOfflineMarksFailed(t *testing.T)
func TestDrafts_UploadPendingRetriesInBackground(t *testing.T)    // falla, después el servidor vuelve → backgroundPass lo sube; NeedsUpload false
func TestDrafts_SaveDuringUploadStaysPending(t *testing.T)
func TestDrafts_NoDraftsFolderKeepsLocal(t *testing.T)
func TestDeleteDraft_RemovesServerCopiesAndStaged(t *testing.T)
func TestOpenDraftMessage_OwnDraftReturnsLocal(t *testing.T)
func TestOpenDraftMessage_ForeignDraftKeepsBccAndAttachments(t *testing.T)
func TestOpenDraftMessage_RejectsNonDraftFolder(t *testing.T)
func TestSend_FromDraftRemovesDraftKeepsStaged(t *testing.T)     // tras enviar: sin fila de borrador, sin copia en Borradores, adjuntos enviados
```

- [ ] **Step 2:** `go test ./core/imapx/ ./core/mail/ -run 'SearchHeader|Drafts|DeleteDraft|OpenDraftMessage|FromDraft'`. Debe fallar.
- [ ] **Step 3: Implementar.** Regenerar los bindings.
- [ ] **Step 4:** `go test -race ./core/... ./app/` y `task check`. Deben dar PASS.
- [ ] **Step 5: Commit** `feat(mail): borradores que se guardan solos en la compu y en el servidor`

---

### Task 4: Firmas en Configuración

**Files:**
- Create: `frontend/src/features/settings/SignaturesSection.tsx`, `SignatureDialog.tsx`, `SignaturesSection.test.tsx`
- Modify: `frontend/src/features/settings/SettingsDialog.tsx`, `frontend/src/lib/queries.ts`, `frontend/src/lib/events.ts`

**Interfaces:**
- Consumes: los bindings de firmas (Task 2), `PickImage` (3A) y `RichEditor` con `onPickImage` (3A).
- Produces:
  - **`useSignatures()`** en `queries.ts`, con key `["signatures"]`, invalidada con `mail:signatures-changed`.
  - **Sección "Firmas" en `SettingsDialog`:**
    - lista de firmas con "Editar" y "Eliminar" (Eliminar pide confirmar con "¿Eliminar la firma «<nombre>»?");
    - botón "Nueva firma";
    - por cada cuenta, un selector "Firma por defecto" con las firmas y "Sin firma", que llama a `SetAccountSignature`.
  - **`SignatureDialog`:**
    - campo "Nombre" y un `RichEditor` con imágenes (`onPickImage` como en el redactor);
    - botones "Cancelar" y "Guardar", que llama a `SaveSignature`;
    - los errores van en un `Alert`;
    - el contenido del `DialogContent` se puede desplazar, para firmas largas.

- [ ] **Step 1: Tests que fallan**
  - "lista las firmas y Nueva firma guarda nombre y HTML"
  - "editar precarga nombre y contenido"
  - "eliminar pide confirmación"
  - "el selector por cuenta llama a SetAccountSignature con el id o 0"
  - "nombre vacío muestra el error del núcleo"
- [ ] **Step 2:** `npx vitest run src/features/settings`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): biblioteca de firmas en Configuración`

---

### Task 5: Firma en el redactor

**Files:**
- Create: `frontend/src/features/compose/SignaturePicker.tsx`
- Modify: `frontend/src/features/compose/Composer.tsx`, `Composer.test.tsx`, `ComposerContext.tsx`

**Interfaces:**
- Consumes: `useSignatures`, `Account.defaultSignatureId`, `ComposeDraft.signatureId` y `SendRequest.signatureId` (Tasks 1, 2 y 4).
- Produces:
  - **`ComposerInit.signatureId: number`:**
    - nuevo correo → la firma por defecto de la cuenta;
    - responder o reenviar → la de `PrepareCompose`;
    - Bandeja de salida o borrador → la del pedido.
  - **`SignaturePicker`:** debajo de "De", con la etiqueta "Firma" y las opciones "Sin firma" más las firmas.
  - **Vista previa** debajo del editor y antes de la cita, con `SafeHtmlFrame` (`bare`, sin remotas). Se oculta con "Sin firma".
  - **Cambiar "De":** si la firma no se tocó a mano, pasa a la por defecto de la nueva cuenta.
  - **Una firma borrada** mientras el redactor está abierto pasa a "Sin firma".
  - **`QueueSend`** lleva `signatureId`.

- [ ] **Step 1: Tests que fallan**
  - "un correo nuevo arranca con la firma por defecto de la cuenta"
  - "cambiar De cambia la firma salvo que se haya elegido a mano"
  - "Sin firma oculta la vista previa y manda signatureId 0"
  - "responder usa la firma de PrepareCompose"
  - "una firma borrada pasa a Sin firma"
- [ ] **Step 2:** `npx vitest run src/features/compose`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): elegir firma en el redactor`

---

### Task 6: Autoguardado, descartar y seguir editando

**Files:**
- Create: `frontend/src/features/compose/useDraftAutosave.ts`, `useDraftAutosave.test.ts`, `frontend/src/features/messages/UnsyncedDraftsBanner.tsx`
- Modify: `frontend/src/features/compose/Composer.tsx`, `Composer.test.tsx`, `ComposerContext.tsx`, `frontend/src/features/reader/MessageActions.tsx`, `MessageActions.test.tsx`, `frontend/src/features/messages/MessageList.tsx` (o donde se arme la vista de carpeta), `frontend/src/lib/queries.ts`, `frontend/src/lib/events.ts`

**Interfaces:**
- Consumes: `SaveDraft`, `UploadDraft`, `DeleteDraft`, `OpenDraftMessage`, `ListPendingDrafts` y `mail:drafts-changed` (Task 3).
- Produces:
  - **`useDraftAutosave({ enabled, initialDraftId, buildRequest, isEmpty })`** devuelve `{ draftId, status: "idle" | "saving" | "saved" | "local", flush(): Promise<void>, discard(): Promise<void>, notifyChange(): void }`.
    - `notifyChange` programa `SaveDraft` a `DRAFT_LOCAL_DEBOUNCE_MS` y `UploadDraft` a `DRAFT_UPLOAD_DEBOUNCE_MS`.
    - Las llamadas se serializan, nunca dos a la vez.
    - El estado es "local" cuando el último `UploadDraft` dejó `needsUpload` en `true`.
    - `flush()` guarda y sube lo pendiente ya.
    - `discard()` cancela los timers y llama a `DeleteDraft` si hay `draftId`.
    - Con `isEmpty()` no guarda nada.
  - **`Composer`:**
    - **Autoguardado** solo si no es un reintento de la Bandeja de salida. El pedido que arma es el mismo que `QueueSend` y suma `composeDraftId` y `draftMessageId`.
    - **Cerrar** (X, Esc o clic afuera) con autoguardado activo: si no está vacío, `flush()` y el toast "Guardado en Borradores.". No pregunta nada.
    - **"Descartar"** (ícono de tacho, en el pie, a la izquierda de "Adjuntar"): confirmación con los textos de Global Constraints, y después `discard()` y cerrar.
    - **Estado** en el pie, con los textos de Global Constraints.
    - **Enviar** cancela los timers antes de `QueueSend`, que lleva `composeDraftId` y `draftMessageId`.
    - **Reintento de la Bandeja de salida:** todo sigue como antes, con "Cancelar" y "¿Descartar este correo?".
  - **`ComposerContext.openComposeDraft(d: model.Draft)`:** carga los adjuntos con `StagedInfo` y la cita con `PrepareCompose`, si es respuesta, como hace `openDraft`.
  - **`MessageActions`:** en una carpeta con rol `drafts`, el botón principal es "Seguir editando", que llama a `OpenDraftMessage` y después a `openComposeDraft`, y no muestra Responder ni Reenviar. Las demás acciones (borrar, mover) siguen.
  - **`UnsyncedDraftsBanner`:**
    - Aparece arriba de la lista en las vistas de carpeta con rol `drafts` y en la vista unificada de Borradores.
    - Usa `ListPendingDrafts`, filtrado por cuenta en la vista de carpeta, con key `["pending-drafts"]`, invalidada con `mail:drafts-changed`.
    - Muestra el texto de Global Constraints y un botón "Abrir" por borrador, con el asunto o "(Sin asunto)".

- [ ] **Step 1: Tests que fallan**
  - **`useDraftAutosave.test.ts`** (con fake timers):
    - "guarda local a los 1500 ms y sube a los 30000 ms"
    - "cambios seguidos reinician los timers"
    - "vacío no guarda"
    - "flush guarda y sube ya"
    - "discard borra y cancela"
    - "upload con needsUpload true deja estado local"
  - **`Composer.test.tsx`:**
    - "cerrar con contenido guarda y avisa Guardado en Borradores"
    - "cerrar vacío no crea borrador"
    - "Descartar pide confirmación y llama a DeleteDraft"
    - "enviar lleva composeDraftId y draftMessageId"
    - "el reintento de la Bandeja de salida no autoguarda y sigue preguntando"
  - **`MessageActions.test.tsx`:** "en Borradores muestra Seguir editando y abre el redactor con el borrador"
  - **Banner:**
    - "el aviso cuenta los pendientes de la cuenta y Abrir los abre"
    - "singular con un solo pendiente"
- [ ] **Step 2:** `npx vitest run src/features`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(ui): borradores que se guardan solos`

---

### Task 7: Prueba de aceptación (Fabricio)

Con la app instalada (`task install:desktop`).

- [ ] **Firmas:**
  - En Configuración, creá dos firmas, una con tu logo. Asigná una por defecto a dos cuentas y dejá otra sin firma.
  - En un correo nuevo arranca la firma de la cuenta. Al cambiar "De" cambia sola, y "Sin firma" la saca.
  - En Gmail, el logo se ve dentro del correo y la firma queda entre tu texto y la cita.
- [ ] **Autoguardado:**
  - Escribí un correo y esperá unos segundos: el pie dice "Guardado".
  - Cerralo: aparece "Guardado en Borradores.".
  - En el webmail de Hostinger el borrador está en Borradores. Editalo de nuevo en PostaControl y esperá: en el webmail sigue habiendo **uno solo**.
- [ ] **Seguir editando:** abrí el borrador desde la carpeta Borradores, con "Seguir editando", y envialo. Desaparece de Borradores y llega bien.
- [ ] **Desde el webmail:** creá un borrador en el webmail con un adjunto y CCO. En PostaControl, "Seguir editando" muestra los destinatarios, el CCO y el adjunto.
- [ ] **Sin conexión:**
  - Con el wifi apagado, escribí un borrador y cerrá la app del todo.
  - Al volver, el aviso de borradores sin subir lo muestra.
  - Con conexión, en unos minutos sube solo y el aviso desaparece.
- [ ] **Descartar:** "Descartar" pide confirmación y el borrador desaparece también del webmail.

# PostaControl — Fase 2C "Búsqueda y conversaciones" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Fabricio encuentre cualquier correo al instante y sin conexión con un buscador local, y que pueda ver la lista agrupada en conversaciones, con sus propias respuestas de Enviados adentro de cada hilo.

**Architecture:**
- **Índice:** una tabla FTS5 en SQLite (`messages_fts`) se mantiene sola con triggers sobre `messages`: asunto, remitente, destinatarios y texto del cuerpo.
- **Indexador:** baja en segundo plano los cuerpos que faltan, de lo más nuevo a lo más viejo, sin marcar nada como leído. Reusa `fetchBody`, que ya lee con `PEEK`.
- **Búsqueda en el servidor:** si el índice todavía no cubre todo, se puede buscar también en el servidor con `UID SEARCH`, que es de solo lectura.
- **Hilos:** cada mensaje guarda una clave de hilo (`thread_root`) calculada de `References`, `In-Reply-To` y `Message-ID`, que se guardan desde la Fase 1.

**Tech Stack:** el de las fases anteriores. FTS5 viene en `modernc.org/sqlite` (verificado: `tokenize='unicode61 remove_diacritics 2'` encuentra "Información" buscando "informacion").

**Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`, secciones "Sincronización, búsqueda y uso sin conexión", "Lista de correos: sueltos o conversaciones" y "Fase 2". Es la tercera parte de la Fase 2 (ver los planes 2A y 2B).

## Global Constraints

- **Siguen vigentes** todas las de las fases 1, 2A y 2B: `core/` sin Wails, contraseñas solo en el llavero, TLS verificado, tests solo con `core/testenv`, español rioplatense con voseo, `task check` verde antes de cada commit, commits convencionales en español con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`, bindings con `task common:generate:bindings`.
- **Nada de esta fase escribe en el servidor.**
  - El indexador baja partes con `BODY.PEEK`, así que nunca marca `\Seen`.
  - La búsqueda en el servidor abre la carpeta en solo lectura (`EXAMINE`) y usa `UID SEARCH`.
- **Adjuntos:** nunca se bajan en segundo plano (spec). El indexador baja lo mismo que abrir un correo: texto, HTML e imágenes en línea.
- **El indexador no compite con el usuario:**
  - toma la conexión de la cuenta de a lotes chicos (`indexBatch = 10` mensajes) y descansa `indexPause = 2 * time.Second` entre lotes;
  - respeta la pausa por contraseña (`authPaused`);
  - se frena con `Close`.
- **Lista agrupada en conversaciones:**
  - apagada por defecto (`Settings.ThreadedList`, cuyo valor cero es `false`);
  - aplica solo a las vistas de carpeta: en las vistas unificadas y en la Bandeja de salida la lista sigue plana y el interruptor no aparece.
- **Búsqueda:**
  - por defecto busca en **todas las cuentas y carpetas**, como Gmail y Outlook, con la opción "Solo en esta carpeta";
  - ordena por fecha descendente;
  - cada resultado muestra la cuenta (punto de color) y la carpeta.

## Review Focus

1. **Texto de búsqueda con caracteres especiales de FTS5** (`"`, `*`, `-`, `:`, `(`, `NEAR`, `AND`, `OR`): no debe haber errores de SQL y se busca el texto literal. Lo cubre `TestSearch_SpecialCharactersAreLiteral` (Task 1).
2. **Acentos y mayúsculas:** "informacion" encuentra "Información" y "PEÑA" encuentra "peña". Lo cubre `TestSearch_IgnoresAccentsAndCase` (Task 1).
3. **El indexador nunca marca un correo como leído en el servidor.** Lo cubre `TestIndexer_DoesNotMarkSeen` (Task 3).
4. **Un correo cuyo cuerpo no se puede bajar** (borrado en el servidor o MIME roto) no frena al indexador ni se reintenta para siempre: después de 3 fallas se saltea. Lo cubre `TestIndexer_SkipsAfterThreeFailures` (Task 3).
5. **Una respuesta que llega antes que el original, o una respuesta propia que está en Enviados,** queda en el mismo hilo. Lo cubren `TestThreads_ReplyBeforeParent` y `TestThreads_IncludesOwnRepliesFromSent` (Task 2).

---

## File Structure

```
core/
  model/model.go            # + SearchQuery, SearchResult, ThreadSummary; Settings.ThreadedList (Tasks 1, 2, 4)
  store/
    schema_v4.sql           # FTS5 + triggers + backfill; thread_root; index_failures (Tasks 1, 2)
    search.go               # Search, PendingBodies, ftsQuery (Task 1)
    threads.go              # ListThreads, ThreadMessages, threadRoot (Task 2)
    messages.go             # UpsertHeaders calcula thread_root; NextToIndex, NoteIndexFailure (Tasks 2, 3)
  imapx/search.go           # SearchText (Task 4)
  mail/
    index.go                # StartIndexing (Task 3)
    search.go               # Search, SearchServer, ListThreads, GetThread (Task 4)
app/mailservice.go          # bindings + StartIndexing en ServiceStartup (Task 4)
frontend/src/
  features/search/SearchBox.tsx, SearchResults.tsx   # (Task 5)
  features/messages/ThreadList.tsx                   # (Task 6)
  features/reader/ThreadView.tsx                     # (Task 6)
```

---

### Task 1: Índice de búsqueda local

**Files:**
- Create: `core/store/schema_v4.sql`, `core/store/search.go`
- Modify: `core/store/migrate.go`, `core/model/model.go`
- Test: `core/store/search_test.go`

**Interfaces:**
- Produces:
  - **En `model`:**
    - `type SearchQuery struct { Text string \`json:"text"\`; FolderID int64 \`json:"folderId"\`; Limit int \`json:"limit"\`; Offset int \`json:"offset"\` }`, donde `FolderID` 0 quiere decir todas las carpetas de todas las cuentas.
    - `type SearchResult struct { Messages []MessageSummary \`json:"messages"\`; Pending int \`json:"pending"\` }`, donde `Pending` es la cantidad de mensajes del alcance que todavía no tienen el cuerpo indexado.
  - **En `store`:**
    - `(*Store).Search(ctx, q model.SearchQuery) ([]model.MessageSummary, error)`: ordena por fecha descendente y aplica Limit y Offset igual que `ListMessages`.
    - `(*Store).PendingBodies(ctx, folderID int64) (int, error)`: cuenta los mensajes con `body_cached = 0` y `index_failures < 3`, de una carpeta o de todo si `folderID` es 0.
    - `func ftsQuery(text string) string`, sin exportar: parte el texto en palabras y las cita como `"palabra"*` (prefijo) unidas con AND. Duplica las comillas internas y devuelve `""` si no queda ninguna palabra; con `""`, `Search` devuelve vacío sin consultar.
- **`schema_v4.sql`** (migración v4):
  - `CREATE VIRTUAL TABLE messages_fts USING fts5(subject, sender, recipients, body, tokenize='unicode61 remove_diacritics 2')`, con `rowid` = `messages.id`.
  - **Triggers** `AFTER INSERT`, `AFTER UPDATE OF subject, from_name, from_email, to_json, cc_json, body_text` y `AFTER DELETE` sobre `messages`, que mantienen `messages_fts`:
    - `sender` = `from_name || ' ' || from_email`
    - `recipients` = `to_json || ' ' || cc_json`
    - `body` = `body_text`
  - Un `INSERT … SELECT` que carga lo que ya existe.
  - `ALTER TABLE messages ADD COLUMN index_failures INTEGER NOT NULL DEFAULT 0`.
  - La columna `thread_root` la agrega la Task 2 en la misma v4: los dos `ALTER` y sus `UPDATE` de carga van en este archivo; la Task 2 completa el suyo.

- [ ] **Step 1: Tests que fallan**

```go
func TestSearch_FindsSubjectSenderAndBody(t *testing.T)   // mensajes con "presupuesto" en asunto, "Mariana" en remitente y "cotización" en cuerpo (vía SetBody) → cada búsqueda encuentra el suyo
func TestSearch_IgnoresAccentsAndCase(t *testing.T)       // "informacion" encuentra "Información"; "PEÑA" encuentra "peña"
func TestSearch_PrefixAndAllWords(t *testing.T)           // "presu" encuentra "presupuesto"; "presupuesto marzo" exige las dos palabras
func TestSearch_SpecialCharactersAreLiteral(t *testing.T) // `"`, `*`, `-`, `:`, `(`, `NEAR`, `AND`, `OR`, `a"b` → sin error; "" y "   " → vacío sin error
func TestSearch_ScopeAndOrder(t *testing.T)               // FolderID limita; 0 busca en dos cuentas; orden por fecha desc; Limit/Offset
func TestSearch_IndexFollowsDeletes(t *testing.T)         // DeleteMessages, ResetFolder y DeleteAccount (cascada) → ya no aparecen en la búsqueda
func TestPendingBodies(t *testing.T)                      // 3 sin cuerpo, 1 con cuerpo, 1 con index_failures=3 → 2
func TestMigrateV4_BackfillsExistingRows(t *testing.T)    // base en v3 con mensajes (abrir con las migraciones hasta v3, insertar, cerrar; reabrir) → Search los encuentra
```

- [ ] **Step 2:** `go test ./core/store/ -run 'Search|Pending|MigrateV4'`. Debe fallar.
- [ ] **Step 3: Implementar.** Para el test de migración, `migrate` puede aceptar un tope interno (`migrateTo(db, n)`) usado solo por tests.
- [ ] **Step 4:** `go test -race ./core/store/`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(store): índice de búsqueda local con FTS5`

---

### Task 2: Hilos de conversación

**Files:**
- Create: `core/store/threads.go`, `core/store/threads_test.go`
- Modify: `core/store/schema_v4.sql`, `core/store/messages.go` (`UpsertHeaders`), `core/model/model.go`

**Interfaces:**
- **`thread_root`:** `ALTER TABLE messages ADD COLUMN thread_root TEXT NOT NULL DEFAULT ''`, más un índice `(thread_root)`. La carga de lo existente usa la misma regla que `threadRoot`.
- **`func threadRoot(messageID, inReplyTo string, references []string, rowID int64) string`**, sin exportar. Devuelve el primer elemento no vacío de esta lista:
  1. `references[0]`
  2. `inReplyTo`
  3. `messageID`
  4. `"id:<rowID>"`
- **Mensaje que llega antes de su original:** `UpsertHeaders` calcula `thread_root` al insertar. Si el mensaje nuevo tiene `message_id` igual al `thread_root` de mensajes de la misma cuenta, esos ya apuntan a él: no hace falta tocarlos. Si el mensaje nuevo es una respuesta cuyo `references[0]` no coincide con el hilo de su padre (porque el padre a su vez era respuesta), se reusa el `thread_root` del padre cuando el padre existe en la cuenta (búsqueda por `message_id = inReplyTo`).
- Produces:
  - **En `model`:** `type ThreadSummary struct { ThreadRoot string \`json:"threadRoot"\`; Latest MessageSummary \`json:"latest"\`; Count int \`json:"count"\`; Unread int \`json:"unread"\`; Participants []string \`json:"participants"\` }`. `Participants` son nombres o emails de los remitentes, sin repetir, como mucho 3, el más reciente primero.
  - **`(*Store).ListThreads(ctx, folderID int64, q model.ListQuery) ([]model.ThreadSummary, error)`:**
    - agrupa los mensajes de la carpeta por `thread_root`;
    - `Latest` es el más nuevo **de la carpeta**;
    - `Count` y `Unread` cuentan los mensajes del hilo en esa carpeta más los de la carpeta con rol `sent` de la misma cuenta;
    - ordena por la fecha de `Latest`, descendente;
    - `UnreadOnly` deja los hilos con `Unread > 0`.
  - **`(*Store).ThreadMessages(ctx, folderID int64, threadRoot string) ([]model.MessageSummary, error)`:** los mensajes del hilo en esa carpeta más los de Enviados de la misma cuenta, por fecha ascendente. Con la carpeta Enviados abierta, cada mensaje aparece una sola vez.

- [ ] **Step 1: Tests que fallan**

```go
func TestThreadRoot_Rule(t *testing.T)                 // tabla: refs → refs[0]; sin refs con inReplyTo → inReplyTo; solo messageID → messageID; nada → "id:7"
func TestThreads_GroupsReplies(t *testing.T)           // original + 2 respuestas en INBOX → 1 hilo, Count 3, Latest = la última
func TestThreads_ReplyBeforeParent(t *testing.T)       // la respuesta (References: <a>) se inserta antes que <a> → mismo hilo al final
func TestThreads_IncludesOwnRepliesFromSent(t *testing.T) // original en INBOX + respuesta propia en Sent (In-Reply-To <a>) → ListThreads(INBOX): Count 2, Latest sigue siendo el de INBOX; ThreadMessages(INBOX, root) trae los 2 en orden
func TestThreads_UnreadOnlyAndOrder(t *testing.T)      // hilos ordenados por su último mensaje; UnreadOnly filtra
func TestThreads_NoDuplicatesInSent(t *testing.T)      // ThreadMessages(Sent, root) no repite mensajes
```

- [ ] **Step 2:** `go test ./core/store/ -run 'Thread'`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test -race ./core/store/`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(store): hilos de conversación por References`

---

### Task 3: Indexador en segundo plano

**Files:**
- Create: `core/mail/index.go`, `core/mail/index_test.go`
- Modify: `core/store/messages.go` (+ `NextToIndex`, `NoteIndexFailure`), `core/mail/core.go` (campos)

**Interfaces:**
- Consumes: `fetchBody`, `store.SetBody`, `withConn` y `authPaused`.
- Produces:
  - **`(*store.Store).NextToIndex(ctx, accountID int64, n int) ([]model.MessageDetail, error)`:** los `n` mensajes más nuevos de la cuenta con `body_cached = 0` e `index_failures < 3`, con lo que `fetchBody` necesita (`AccountID`, `FolderID`, `UID`).
  - **`(*store.Store).NoteIndexFailure(ctx, id int64) error`:** suma 1 a `index_failures`.
  - **`(*Core).StartIndexing(ctx context.Context)`:** una goroutine (`c.spawn`) que repite esta pasada:
    - recorre las cuentas;
    - en cada una que no esté en pausa toma `NextToIndex(indexBatch)` y le baja el cuerpo a cada mensaje con `fetchBody` + `SetBody`;
    - descansa `c.indexPause` entre lotes;
    - si ninguna cuenta tiene pendientes, espera `c.indexIdle` (= 1 minuto) antes de volver a mirar.
  - **Fallas:**
    - una falla de un mensaje que no sea de red ni de contexto llama a `NoteIndexFailure` y sigue con el siguiente;
    - una falla de red corta el lote de esa cuenta;
    - no emite `EventSyncError`, porque `withConn` ya lo hace con los errores de conexión.
  - **Al terminar cada lote** emite `EventFolderUpdated` por cada carpeta tocada: los snippets de la lista mejoran.
  - **Campos para tests:** `c.indexPause` y `c.indexIdle`.

- [ ] **Step 1: Tests que fallan.** Usan `indexPause` e `indexIdle` de 10 ms.

```go
func TestIndexer_DownloadsNewestFirst(t *testing.T)    // 30 mensajes sincronizados sin cuerpo; StartIndexing → los 30 terminan con body_cached=1 (poll); el primero en completarse es el más nuevo (registrar el orden por EventFolderUpdated o por un hook de test)
func TestIndexer_DoesNotMarkSeen(t *testing.T)         // tras indexar, en el servidor ningún mensaje tiene \Seen (FetchFlags con otra conexión) y en la caché siguen no leídos
func TestIndexer_SkipsAfterThreeFailures(t *testing.T) // un mensaje expulsado en el servidor después de sincronizar → index_failures llega a 3 y el resto se indexa igual; no hay bucle (contar intentos)
func TestIndexer_RespectsAuthPause(t *testing.T)       // cuenta en pausa → no baja nada; al levantar la pausa (clearAuthFailure) indexa
func TestIndexer_StopsOnClose(t *testing.T)            // Close vuelve antes de 1 s con el indexador corriendo
func TestIndexer_SearchFindsBodyText(t *testing.T)     // tras indexar, store.Search encuentra una palabra que solo está en el cuerpo
```

- [ ] **Step 2:** `go test ./core/mail/ -run Indexer`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `go test -race -count=3 ./core/mail/ -run Indexer` y `go test -race ./core/...`. Debe dar PASS.
- [ ] **Step 5: Commit** `feat(core): indexar cuerpos en segundo plano sin marcar leídos`

---

### Task 4: Búsqueda, servidor y hilos en el núcleo y en la app

**Files:**
- Create: `core/imapx/search.go`, `core/mail/search.go`, `core/mail/search_test.go`
- Modify:
  - `core/model/model.go` (`Settings.ThreadedList bool \`json:"threadedList"\``)
  - `app/mailservice.go` (bindings; `StartIndexing` en `ServiceStartup`)
- Test: `core/imapx/imapx_test.go`

**Interfaces:**
- Produces:
  - **`(*imapx.Conn).SearchText(text string) ([]uint32, error)`:**
    - `UID SEARCH` con `imap.SearchCriteria{Text: []string{text}}` sobre la carpeta ya abierta;
    - el que llama hace el `Select` (EXAMINE);
    - devuelve los UIDs ascendentes.
  - **`(*mail.Core).Search(ctx, q model.SearchQuery) (model.SearchResult, error)`:**
    - Limit 0 se toma como 100;
    - `Pending` sale de `PendingBodies(q.FolderID)`.
  - **`(*mail.Core).SearchServer(ctx, folderID int64, text string) ([]model.MessageSummary, error)`:**
    - con `withConn`, hace `selectChecked(conn, f, last, false)` y `SearchText`;
    - para los UIDs que no estén en la caché sincroniza la carpeta una vez (`SyncFolder`);
    - devuelve `MessagesByUIDs` por fecha descendente, como mucho 200;
    - con texto vacío devuelve `invalid`.
  - **`(*mail.Core).ListThreads(ctx, folderID int64, q model.ListQuery) ([]model.ThreadSummary, error)`** (Limit 0 se toma como 100) y **`(*mail.Core).GetThread(ctx, folderID int64, threadRoot string) ([]model.MessageSummary, error)`**.
  - **`MailService`** expone `Search`, `SearchServer`, `ListThreads` y `GetThread`, y en `ServiceStartup` llama a `core.StartIndexing(ctx)` después de `StartWatching`.

- [ ] **Step 1: Tests que fallan**

```go
// imapx
func TestSearchText_FindsInBody(t *testing.T)   // 3 mensajes, uno con "factura" solo en el cuerpo → SearchText("factura") = [su UID]
// mail
func TestSearch_ReturnsPending(t *testing.T)    // sin indexar: Search devuelve lo que matchea por asunto y Pending = cantidad sin cuerpo
func TestSearchServer_FindsUnindexedBody(t *testing.T) // palabra solo en el cuerpo, sin indexar → Search local vacío; SearchServer(inbox, palabra) la encuentra
func TestSearchServer_EmptyTextInvalid(t *testing.T)
func TestListThreads_DefaultLimit(t *testing.T) // 150 hilos → 100 sin Limit
```

- [ ] **Step 2:** `go test ./core/... -run 'SearchText|Search|ListThreads'`. Debe fallar.
- [ ] **Step 3: Implementar** y regenerar los bindings.
- [ ] **Step 4:** `task check` da PASS.
- [ ] **Step 5: Commit** `feat(core): búsqueda local, en el servidor y por hilos`

---

### Task 5: Buscador

**Files:**
- Create:
  - `frontend/src/features/search/SearchBox.tsx`
  - `frontend/src/features/search/SearchResults.tsx`
  - sus tests
- Modify:
  - `frontend/src/features/messages/MessageList.tsx` (encabezado con el buscador)
  - `frontend/src/lib/queries.ts` (`useSearch`)
  - `frontend/src/features/layout/MailShell.tsx` (estado de búsqueda)

**Interfaces:**
- Consumes: `svc.Search` y `svc.SearchServer` (Task 4).
- **Buscador:** un campo arriba de la lista ("Buscar correo", con ícono de lupa).
  - Busca 300 ms después de dejar de escribir y al apretar Enter.
  - Esc o la "x" lo limpian y vuelven a la lista normal.
- **Mientras hay texto, la lista muestra los resultados:**
  - mismo formato de fila, más la etiqueta de la carpeta y el punto de color de la cuenta;
  - "Cargar más" con Offset.
  - Arriba, un selector: "En todo" (por defecto) o "Solo en esta carpeta". Este último solo aparece con una vista de carpeta.
- **Si `pending > 0`:** una línea "Todavía faltan indexar <pending> correos." y, con una vista de carpeta, el botón "Buscar también en el servidor".
  - El botón agrega abajo los resultados de `SearchServer`, sin repetir los que ya están.
- **Elegir un resultado** lo abre en el lector como siempre (`selectMessage`).
- **El estado de búsqueda vive en `MailShell`:** cambiar de vista limpia la búsqueda.

- [ ] **Step 1: Tests que fallan**
  - "escribir busca después de la pausa y muestra resultados con su carpeta"
  - "Esc vuelve a la lista"
  - "Solo en esta carpeta manda folderId"
  - "con pendientes ofrece buscar en el servidor y suma sin duplicar"
  - "elegir un resultado lo abre"
  - "cambiar de carpeta limpia la búsqueda"

  Todos usan fake timers para la pausa de 300 ms.
- [ ] **Step 2:** `npx vitest run src/features/search src/features/messages`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check` da PASS.
- [ ] **Step 5: Commit** `feat(ui): buscador local con búsqueda en el servidor`

---

### Task 6: Lista en conversaciones

**Files:**
- Create:
  - `frontend/src/features/messages/ThreadList.tsx`
  - `frontend/src/features/reader/ThreadView.tsx`
  - sus tests
- Modify:
  - `frontend/src/features/messages/MessageList.tsx` (interruptor y elección de lista)
  - `frontend/src/features/layout/MailShell.tsx` (`ShellState` suma `threadRoot: string | null` y `selectThread`)
  - `frontend/src/App.tsx` (el lector muestra `ThreadView` cuando hay hilo elegido)
  - `frontend/src/lib/queries.ts` (`useThreads`, `useThread`)

**Interfaces:**
- Consumes: `svc.ListThreads`, `svc.GetThread` (Task 4), `Settings.threadedList`.
- **Interruptor "Conversaciones"** en el encabezado de la lista:
  - solo en vistas de carpeta y sin búsqueda activa;
  - guarda `threadedList` con `SetSettings`, conservando los otros campos.
- **`ThreadList`, una fila por hilo:**
  - participantes;
  - un contador si `count > 1`;
  - asunto del último mensaje;
  - fecha;
  - negrita si `unread > 0`;
  - estrella si el último está destacado.
- **Al elegir un hilo:** `selectThread(root)`.
- **`ThreadView`:**
  - lista los mensajes del hilo, del más viejo al más nuevo, cada uno con su encabezado (remitente, fecha);
  - el último está expandido y muestra el cuerpo con el `MessageView` existente, embebido;
  - los demás están plegados con su snippet y se expanden al hacer clic (abrir uno lo marca leído, como hoy);
  - los mensajes de Enviados muestran la etiqueta "Enviado".
  - Las acciones (Responder, etc.) son las de cada mensaje.
- **Con el interruptor apagado** todo funciona igual que antes.
- **Con un hilo abierto,** las acciones de mover o borrar cierran el hilo (`selectThread(null)`).

- [ ] **Step 1: Tests que fallan**
  - "el interruptor guarda threadedList y muestra hilos"
  - "un hilo muestra participantes, contador y negrita si hay no leídos"
  - "abrir un hilo muestra los mensajes en orden con el último expandido"
  - "expandir un mensaje plegado lo carga"
  - "los de Enviados llevan la etiqueta Enviado"
  - "en vista unificada no aparece el interruptor"
- [ ] **Step 2:** `npx vitest run src/features/messages src/features/reader`. Debe fallar.
- [ ] **Step 3: Implementar.**
- [ ] **Step 4:** `task check` da PASS.
- [ ] **Step 5: Commit** `feat(ui): lista agrupada en conversaciones`

---

### Task 7: Prueba de aceptación (Fabricio)

Con la app instalada (`task install:desktop`).

- [ ] **Indexado:** al abrir la app con las 4 cuentas, el uso normal no se pone lento. Al rato, en Configuración o en el buscador, la cantidad de correos sin indexar baja sola.
- [ ] **Leídos:** ningún correo aparece marcado como leído por el indexado. Revisalo en el webmail.
- [ ] **Buscar sin acentos:** buscar "informacion" encuentra correos con "Información".
- [ ] **Buscar por remitente y por cuerpo:** buscar un remitente y buscar una palabra que solo está en el cuerpo de un correo viejo.
- [ ] **Sin conexión:** con el wifi apagado, la búsqueda local sigue funcionando.
- [ ] **En el servidor:** en una carpeta todavía sin indexar, "Buscar también en el servidor" encuentra lo que falta.
- [ ] **Conversaciones:** activá "Conversaciones" en la Entrada. Un intercambio de ida y vuelta aparece como un solo hilo, con tus respuestas de Enviados adentro.

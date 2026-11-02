# Fase 7 "Etiquetas" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Etiquetas propias con color, guardadas como keywords IMAP. Se ven como línea y pastillas en la fila, se ponen y sacan con clic derecho, teclas 1–9 y el lector, y se filtran desde la cabecera de la lista en todas las carpetas.

**Architecture:**
- **Definiciones:** la definición de cada etiqueta (nombre, color, orden y un keyword fijo `$pc_xxxxxxxx`) vive en SQLite.
- **Marca en el correo:** cada correo la lleva en el servidor como keyword IMAP. La caché la refleja en `message_labels`, que se llena por los mismos caminos que hoy llenan seen y flagged: `headerRows`/`UpsertHeaders` y `refreshFlags`.
- **Cuentas cuyo servidor no acepta keywords** (sin `\*` en PERMANENTFLAGS): quedan marcadas `labels_local`, y ahí la caché es la única fuente de verdad.
- **Filtro:** lee la caché (`ListLabeled`) y se completa en segundo plano con `UID SEARCH KEYWORD` por carpeta (`RefreshLabel`).

**Tech Stack:** Go 1.27, go-imap v2 beta.8, modernc sqlite, Wails v3.0.0, React 19, TanStack Query, shadcn/radix (context-menu, dropdown-menu, popover, dialog), Vitest.

**Spec:** `docs/superpowers/specs/2026-11-02-etiquetas-design.md`. Bocetos aprobados: `.superpowers/brainstorm/4010542-1793647001/content/etiquetas.html` (1 B, 2 A, 3 completa).

## Global Constraints

- **Núcleo:** `core/` no importa Wails.
- **Seguridad:** TLS siempre verificado y contraseñas solo en el keyring.
- **Tests:** nunca contra cuentas reales; Go usa `core/testenv` y el frontend usa Vitest con mocks de bindings.
- **Escrituras al servidor:** solo por acción explícita, que acá es poner o sacar una etiqueta o borrarla. STORE toca únicamente el keyword `$pc_…` de la etiqueta en juego, nunca otros flags ni keywords ajenos. Nunca `EXPUNGE` sin UID.
- **Límites de las etiquetas:**
  - como mucho **20**;
  - el nombre se recorta (trim) y se corta a **40** runas;
  - el nombre no puede quedar vacío ni repetir otro, sin distinguir mayúsculas (`strings.EqualFold`);
  - el color cumple `^#[0-9a-f]{6}$`.
- **Keyword:** `$pc_` seguido de 8 hex minúsculos al azar (`crypto/rand`), único y que nunca se reusa. Los keywords `$pc_…` sin definición local se ignoran: no se muestran, no se tocan y no se borran.
- **Paleta**, en el frontend (`lib/labels.ts`, en este orden): `#dc2626 #ea580c #d97706 #65a30d #16a34a #0d9488 #0891b2 #2563eb #4f46e5 #7c3aed #db2777 #475569`. Una etiqueta nueva toma el primer color que no use otra etiqueta, o el primero de la lista si están todos usados.
- **Textos de error del núcleo** (`UserError` con code `invalid`, salvo el que se indica):
  - `"Poné un nombre para la etiqueta."`
  - `"Ya hay una etiqueta con ese nombre."`
  - `"Ese color no es válido."`
  - `"Ya tenés 20 etiquetas, que es el máximo."`
  - `"Esa etiqueta ya no existe."` (code `notfound`)
  - para ids de dos cuentas, el `msgOneAccount` de siempre.
- **Eventos nuevos** (`core/mail/events.go`):
  - `EventLabelsChanged = "mail:labels-changed"`, sin datos;
  - `EventLabelsLocalOnly = "mail:labels-local-only"`, con datos `{"accountId": int64, "email": string}`.
- **Textos de la UI:**
  - **Filtro:**
    - botón `Etiqueta` con `ChevronDownIcon` y `aria-label="Filtrar por etiqueta"`;
    - con una elegida, muestra el nombre y una ✕ con `aria-label="Quitar filtro de etiqueta"`;
    - lista vacía: `No hay correos con esta etiqueta.`;
    - sin etiquetas creadas, el menú dice `Todavía no tenés etiquetas.` y ofrece `Administrar etiquetas…`.
  - **Menús:** `Etiquetas`, `Administrar etiquetas…`, `Marcar como leído` / `Marcar como no leído`, `Destacar` / `Quitar destacado`.
  - **Lector:** botón `Etiquetas`. La ✕ de cada pastilla lleva `aria-label="Quitar etiqueta {nombre}"`.
  - **Configuración**, sección `Etiquetas`:
    - botón `Nueva etiqueta` y vacío `Todavía no tenés etiquetas.`;
    - confirmación con título `¿Borrar la etiqueta «{nombre}»?`;
    - descripción `Se saca de {N} correos. Los correos no se borran.` (con N = 1 dice `1 correo`; con N = 0 dice `Ningún correo la tiene todavía. Los correos no se borran.`);
    - botones `Cancelar` / `Borrar`.
  - **Aviso de cuenta sin keywords** (toast de advertencia, 10 s): `El servidor de {email} no guarda etiquetas. En esa cuenta quedan solo en esta PC y pueden perderse si movés el correo.`
- **Comandos:** `task check` es la verificación completa, y los bindings se regeneran con `wails3 generate bindings -clean=true -ts -i`, desde la raíz. No se usa prettier (no está en el proyecto). Las dependencias npm, si hiciera falta alguna, se instalan con `npm install --before=<hoy menos 7 días>`; este plan no necesita ninguna.

## Review Focus

1. **Teclas 1–9 mientras se escribe:** con el foco en el buscador, un input, un textarea, un `contenteditable` o un diálogo abierto, no hacen nada; tampoco con Ctrl, Alt o Meta. Test en Task 7.
2. **Varios marcados con estado mixto:** si algunos tienen la etiqueta, se pone en todos; si todos la tienen, se saca de todos. El submenú muestra la marca intermedia. Test en Task 6 (`toggleTargets`) y en Task 7.
3. **Renombrar o cambiar de color** no escribe en el servidor y no cambia el keyword. Test en Task 2: la cantidad de ops IMAP queda igual.
4. **Borrar una etiqueta con la cuenta desconectada:** la limpieza queda pendiente y se hace en la próxima pasada en segundo plano. Test en Task 2.
5. **Un sync no borra lo ajeno ni pisa las cuentas locales:**
   - en una cuenta `labels_local`, un sync no borra las etiquetas de la caché;
   - en una cuenta normal, un keyword ajeno (`$label1`, `$pc_desconocido`) no aparece y sobrevive a poner o sacar la nuestra.

   Tests en Task 3.

---

### Task 1: Esquema v9 y store de etiquetas

**Files:**
- Create: `core/store/schema_v9.sql`, `core/store/labels.go`, `core/store/labels_test.go`
- Modify: `core/store/migrate.go` (embed y `migrations`), `core/store/messages.go` (columna de etiquetas en `summaryCols`/`scanSummary`, `HeaderRow.Keywords`, `UpsertHeaders`), `core/store/threads.go` (`ThreadSummary.Labels`), `core/store/search.go` (`LabelID`), `core/store/accounts.go`, `core/model/model.go`

**Interfaces:**
- **Produces, en `core/model`:**
  - `type Label struct { ID int64 \`json:"id"\`; Name string \`json:"name"\`; Color string \`json:"color"\`; Position int \`json:"position"\` }`.
  - `MessageSummary.Labels []int64 \`json:"labels"\``: ids ordenados por `labels.position`, nunca nil.
  - `ThreadSummary.Labels []int64 \`json:"labels"\``: la unión de las etiquetas de los mensajes del hilo dentro del alcance de `threadScope`, sin repetir y en orden de position.
  - `SearchQuery.LabelID int64 \`json:"labelId"\``: 0 es sin filtro.
- **Produces, en `core/store`:**
  - `ListLabels(ctx) ([]model.Label, error)`, por position y después id.
  - `InsertLabel(ctx, name, color, keyword string) (model.Label, error)`: queda última en el orden.
  - `UpdateLabel(ctx, l model.Label) error`, que cambia solo name y color.
  - `SetLabelPositions(ctx, ids []int64) error`.
  - `LabelKeyword(ctx, id int64) (string, error)` y `LabelByKeyword(ctx) (map[string]int64, error)`.
  - `DeleteLabel(ctx, id int64, cleanupAccounts []int64) (keyword string, err error)`: en una transacción borra la fila (las de `message_labels` se van en cascada) e inserta un `label_cleanup(account_id, keyword)` por cada cuenta de la lista.
  - `CountLabeled(ctx, labelID int64) (int, error)`.
  - `SetMessageLabels(ctx, messageID int64, labelIDs []int64) error`, que reemplaza el conjunto.
  - `AddMessageLabel(ctx, ids []int64, labelID int64, on bool) error`.
  - `ListLabeled(ctx, labelID, accountID int64, q model.ListQuery) ([]model.MessageSummary, error)`, donde accountID 0 es todas las cuentas. Por `listWhere`, respeta UnreadOnly y paginación.
  - `PendingLabelCleanups(ctx, accountID int64) ([]string, error)` y `DoneLabelCleanup(ctx, accountID int64, keyword string) error`.
  - `AccountLabelsLocal(ctx, accountID int64) (bool, error)` y `SetAccountLabelsLocal(ctx, accountID int64) (changed bool, err error)`: changed es true solo en el paso de 0 a 1.
  - `SaveLabelCarry(ctx, accountID int64, messageID string, labelIDs []int64) error`.
  - `HeaderRow.Keywords []string`.

- [ ] **Step 1: Write the failing tests** in `core/store/labels_test.go`, using the existing helpers of `store_test.go` to open a temporary store with an account and folders:
  - `TestLabelsCRUDAndOrder`: `InsertLabel` a, b and c, then `SetLabelPositions([c,a,b])`; `ListLabels` returns the names in the order c, a, b. `UpdateLabel` changes name and color but keeps the keyword (`LabelKeyword` is the same).
  - `TestUpsertHeadersMapsKeywords`: the label has keyword `$pc_aaaaaaaa`, and a `HeaderRow` arrives with `Keywords: ["$pc_aaaaaaaa", "$label1", "$pc_ffffffff"]`. `ListMessages` returns `Labels == []int64{id}`. Upserting again with `Keywords: nil` leaves `Labels == []int64{}`.
  - `TestUpsertHeadersKeepsLocalLabels`: after `SetAccountLabelsLocal` (which returns `changed == true`, and `false` the second time) and `AddMessageLabel(on=true)`, an upsert with `Keywords: nil` keeps the label.
  - `TestLabelCarryAppliesOnInsert`: on a local account, `SaveLabelCarry(acc, "<x@y>", [id])` followed by an upsert of a new row with `MessageID "<x@y>"` in another folder of the same account makes that row carry `Labels [id]`, and the carry row is consumed (a second insert with the same Message-ID does not get it).
  - `TestDeleteLabelCascadesAndQueuesCleanup`: `DeleteLabel(id, [acc])` leaves `CountLabeled == 0`, `ListMessages` returns empty `Labels`, and `PendingLabelCleanups(acc) == ["$pc_aaaaaaaa"]`.
  - `TestListLabeledScopesAndFilters`: two accounts, each with two folders and labeled messages. `ListLabeled(id, 0, {})` returns all of them by date descending. `ListLabeled(id, acc1, {})` returns only acc1's. `UnreadOnly` filters.
  - `TestThreadLabelsUnion`: in a thread of two messages, one carries label A and the other label B; `ListThreads` returns `Labels [A, B]` in position order.
  - `TestSearchByLabel`: `Search{Text, LabelID}` returns only the matches that carry the label.
- [ ] **Step 2:** `go test ./core/store/ -run 'Label|Thread|Search'` fails: the types do not exist yet.
- [ ] **Step 3: Write `schema_v9.sql`** with these tables:
  - `labels(id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, color TEXT NOT NULL, keyword TEXT NOT NULL UNIQUE, position INTEGER NOT NULL DEFAULT 0)`;
  - `message_labels(message_id INTEGER NOT NULL REFERENCES messages(id) ON DELETE CASCADE, label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE, PRIMARY KEY(message_id, label_id))` plus `CREATE INDEX idx_message_labels_label ON message_labels(label_id)`;
  - `label_cleanup(account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, keyword TEXT NOT NULL, PRIMARY KEY(account_id, keyword))`;
  - `label_carry(account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE, message_id TEXT NOT NULL, label_id INTEGER NOT NULL REFERENCES labels(id) ON DELETE CASCADE, PRIMARY KEY(account_id, message_id, label_id))`;
  - `ALTER TABLE accounts ADD COLUMN labels_local INTEGER NOT NULL DEFAULT 0`.

  Before relying on the cascade, check that the store enables `PRAGMA foreign_keys` (`store.go`); if it does not, delete by hand inside the transaction.
- [ ] **Step 4: Implement the store.**
  - **Lists:** the label column of `summaryCols` is `(SELECT group_concat(ml.label_id) FROM message_labels ml JOIN labels l ON l.id = ml.label_id WHERE ml.message_id = m.id ORDER BY l.position, l.id)`. Since `group_concat` does not guarantee order with `ORDER BY` in the subquery, order it in Go after reading the positions, or use an ordered subquery. `scanSummary` parses it to `[]int64{}`.
  - **`UpsertHeaders`:**
    - for every row of a non-`labels_local` account, replace the set with the label ids of the known keywords (`LabelByKeyword`, read once per batch);
    - for `labels_local` accounts, do not touch the labels of existing rows;
    - on new rows, apply and delete the matching `label_carry`.
  - **Threads:** `ThreadSummary.Labels` comes from an aggregation over `x.thread_root = m.thread_root AND x.folder_id IN scope`.
- [ ] **Step 5:** `go test ./core/store/` passes.
- [ ] **Step 6: Commit** `feat(etiquetas): esquema v9 y store de etiquetas`.

### Task 2: Etiquetas en el núcleo: alta, cambios, orden y borrado con limpieza en el servidor

**Files:**
- Create: `core/mail/labels.go`, `core/mail/labels_test.go`
- Modify: `core/mail/events.go`, `core/mail/sync.go` (`backgroundPass`), `core/imapx/search.go` (`SearchKeyword`)

**Interfaces:**
- **Consumes:** the store from Task 1.
- **Produces, on `*mail.Core`:**
  - `ListLabels(ctx) ([]model.Label, error)`.
  - `CreateLabel(ctx, name, color string) (model.Label, error)`.
  - `UpdateLabel(ctx, l model.Label) (model.Label, error)`, which changes name and color only.
  - `ReorderLabels(ctx, ids []int64) error`; ids must be exactly the existing ones, or it fails with `invalid`.
  - `LabelUsage(ctx, id int64) (int, error)`.
  - `DeleteLabel(ctx, id int64) error`.
  - `runLabelCleanups(ctx, accountID int64)`, unexported, which `backgroundPass` calls after `SyncAccount` when the account connected (next to `uploadPendingDrafts`).
- **Produces, in `imapx`:** `(*Conn).SearchKeyword(kw string) ([]uint32, error)`, which is `UID SEARCH KEYWORD` on the selected folder (`SearchCriteria{Flag: []imap.Flag{imap.Flag(kw)}}`), with UIDs in ascending order.
- **Events:** every successful create, update, reorder or delete emits `EventLabelsChanged`.

- [ ] **Step 1: Write the failing tests** in `core/mail/labels_test.go`, with the `helpers_test.go` helpers (`testenv` with an IMAP server and a configured account):
  - `TestCreateLabelValidates`. Cases:
    - empty name, or one of spaces only → `"Poné un nombre para la etiqueta."`;
    - `"facturas"` when `"Facturas"` already exists → `"Ya hay una etiqueta con ese nombre."`;
    - color `"red"` or `"#ZZZZZZ"` → `"Ese color no es válido."`;
    - creating label 21 → `"Ya tenés 20 etiquetas, que es el máximo."`;
    - a 50-rune name is stored with 40;
    - the keyword matches `^\$pc_[0-9a-f]{8}$`.
  - `TestUpdateLabelNoServerWrite`: count the server ops (`testenv` already records `ops`) before and after `UpdateLabel`. They are equal, and the keyword does not change. A rename to the name of another label fails with the duplicate error, and so does a rename to the same name with different case.
  - `TestDeleteLabelCleansServer`: messages in INBOX and in `Proveedores` (a folder never synced) carry the keyword on the server, put there directly through `testenv`, plus `$label1`. After `DeleteLabel`, no message on the server has `$pc_…`, `$label1` is still there, `PendingLabelCleanups` is empty, and `EventLabelsChanged` was emitted.
  - `TestDeleteLabelOfflineQueues`: with the server down (`Close`), `DeleteLabel` returns nil and the label disappears from `ListLabels`. The cleanup stays pending. After `Restart` and `backgroundPass`, the keyword is gone from the server and nothing is left pending.
  - `TestDeleteLabelLocalAccountSkipsServer`: with the account `labels_local`, no cleanup is queued and no IMAP op happens.
- [ ] **Step 2:** `go test ./core/mail/ -run Label` fails.
- [ ] **Step 3: Implement.**
  - `DeleteLabel` computes the non-local accounts, calls `store.DeleteLabel(id, thoseAccounts)`, emits the event and does `c.spawn` of `runLabelCleanups` for each one.
  - `runLabelCleanups`, for each pending keyword:
    - goes through the account's selectable folders in the cache: `SelectRW` → `SearchKeyword` → `StoreFlags(uids, false, imap.Flag(kw))`, when there are uids;
    - calls `DoneLabelCleanup` only if every folder worked; a connection error leaves it pending;
    - uses `withConn`, since this is idempotent.
- [ ] **Step 4:** `go test ./core/mail/ ./core/imapx/` passes.
- [ ] **Step 5: Commit** `feat(etiquetas): alta, cambios, orden y borrado de etiquetas en el núcleo`.

### Task 3: Poner y sacar etiquetas, sincronizar keywords, cuentas sin keywords y mover

**Files:**
- Modify:
  - `core/mail/labels.go` (`SetLabel`);
  - `core/mail/sync.go` (`headerRows` copies keywords; `refreshFlags` compares labels);
  - `core/mail/actions.go` (`applyMove` saves the carry on local accounts);
  - `core/imapx/folders.go` (`SelectInfo.KeywordsOK`);
  - `core/imapx/fetch.go`, if `RemoteHeader` does not already expose the flags;
  - `core/testenv` (`NoKeywords` simulation).
- Test: `core/mail/labels_test.go`, `core/mail/sync_test.go`

**Interfaces:**
- **Consumes:** Tasks 1 and 2.
- **Produces:**
  - `(*Core).SetLabel(ctx, ids []int64, labelID int64, on bool) error`. All ids come from a single account, or it fails with `msgOneAccount`.
  - `imapx.SelectInfo.KeywordsOK bool`, true when `PermanentFlags` contains `imap.FlagWildcard`.
  - `testenv.(*IMAP).NoKeywords()`: from then on SELECT drops `\*` from PERMANENTFLAGS and STORE with a keyword answers NO.
- **Helper:** `keywordsOf(flags []imap.Flag) []string` returns the flags that do not start with `\`.

- [ ] **Step 1: Write the failing tests.**
  - `TestSetLabelWritesKeyword`:
    - `SetLabel([id1,id2], L, true)` leaves the keyword on the server for both, `Labels == [L]` in the cache, and an `EventFolderUpdated` per folder;
    - `SetLabel(…, false)` takes it off;
    - a message that already had `$label1` keeps it on the server after both operations.
  - `TestSetLabelUnknown`: an id that does not exist → `"Esa etiqueta ya no existe."`.
  - `TestSetLabelNoKeywordsServer`:
    - with `NoKeywords()`, `SetLabel` returns nil, the cache has the label, `AccountLabelsLocal == true`, and `EventLabelsLocalOnly` is emitted once with the account's `accountId` and `email`;
    - a second `SetLabel` neither emits it again nor connects (the `Sessions()` count does not grow);
    - a `SyncFolder` afterwards does not erase it.
  - `TestSyncPicksUpKeywordChanges`: in `sync_test.go`, put or take off the keyword directly on the server; a `SyncFolder` reflects it in the cache, both for new messages (headers) and for already-known ones (`refreshFlags`). An unknown keyword (`$pc_ffffffff`) produces no label.
  - `TestMoveKeepsLabel`:
    - on a normal account, label a message and `MoveMessages` it to another folder; after the destination sync that runMoves already starts, the message in the destination carries the label;
    - on a `NoKeywords` account, the same holds thanks to the carry (the message has a Message-ID).
  - `TestSetLabelServerRejects`: a STORE that the server rejects for any other reason returns an error, and the cache stays as it was.
- [ ] **Step 2:** `go test ./core/mail/ -run 'Label|Sync|Move'` fails.
- [ ] **Step 3: Implement.**
  - **`SetLabel`:** same skeleton as `setFlag` (`MessageRefs` → `groupByFolder` → per folder).
    - If `AccountLabelsLocal`, it only writes the cache.
    - If not, `selectChecked(…, rw=true)`. If after the SELECT `KeywordsOK` is false, `SetAccountLabelsLocal`; if that returns `changed`, emit `EventLabelsLocalOnly`. Then continue with the cache only.
    - If `KeywordsOK`, `StoreFlags(uids, on, imap.Flag(keyword))` and then `AddMessageLabel`.
    - At the end, `EventFolderUpdated` per folder.
    - `selectChecked` needs to return the `SelectInfo`. Add a variant, or change its signature and update the callers.
  - **`refreshFlags`:** besides seen/flagged/answered, compare the label ids derived from the keywords against `m.Labels` and call `SetMessageLabels` when they differ. Skip this for `labels_local` accounts.
  - **`applyMove`:** for `labels_local` accounts, before `DeleteMessages`, `SaveLabelCarry` for each ref that has a MessageID and labels. `MessageRef` needs `Labels`, or a store read.
- [ ] **Step 4:** `go test ./core/...` passes.
- [ ] **Step 5: Commit** `feat(etiquetas): poner y sacar etiquetas como keywords IMAP, con cuentas sin keywords`.

### Task 4: Filtro en el núcleo, completado en el servidor, y servicio Wails

**Files:**
- Modify: `core/mail/labels.go` (`ListLabeled`, `RefreshLabel`), `core/mail/search.go` (it passes `LabelID` on), `app/mailservice.go`, `frontend/bindings/**` (regenerated)
- Test: `core/mail/labels_test.go`, `app/mailservice_test.go` (only if the existing pattern tests delegation; otherwise none)

**Interfaces:**
- **Produces, on `*mail.Core`:**
  - `ListLabeled(ctx, labelID, accountID int64, q model.ListQuery) ([]model.MessageSummary, error)`. Limit 0 means `defaultPageSize`, and an unknown label fails with `notfound`.
  - `RefreshLabel(ctx, labelID, accountID int64) error`, with accountID 0 meaning every account.
    - It covers every non-`labels_local` account in scope and every selectable folder in each. For each folder:
      1. take the `folderSyncLock`;
      2. `selectChecked` read-only;
      3. `SearchKeyword`;
      4. headers of the missing UIDs (those not in `KnownUIDs`) with `FetchHeaders` → `UpsertHeaders`. If the folder has `UIDValidity` 0 in the cache, `SetFolderSync{UIDValidity}` first, as `prepareSync` does;
      5. cached messages in that folder with the label whose UID is not in the result lose it;
      6. `RecountFolder` + `EventFolderUpdated` if something changed.
    - Connection errors of one account do not cut the others, and it returns the first error.
- **Produces, on `MailService`:** thin methods with the same names and signatures as the Core ones (without `ctx` in TS): `ListLabels`, `CreateLabel`, `UpdateLabel`, `ReorderLabels`, `LabelUsage`, `DeleteLabel`, `SetLabel`, `ListLabeled`, `RefreshLabel`.

- [ ] **Step 1: Write the failing tests.**
  - `TestListLabeledAcrossFolders`: labeled messages in INBOX and in another synced folder of the same account, and in a second account. With accountID set, it returns the account's messages from both folders by date. With 0, it returns everything.
  - `TestRefreshLabelFindsUnsyncedFolder`: a message with the keyword in a folder never opened. `ListLabeled` does not include it at first; after `RefreshLabel` it does, carrying the label.
  - `TestRefreshLabelDropsStale`: a message labeled in the cache, but not on the server, loses the label after `RefreshLabel`.
  - `TestSearchWithLabel`: `Search{Text, LabelID}` filters through the store.
- [ ] **Step 2:** the tests fail.
- [ ] **Step 3: Implement** the Core functions and the service, then regenerate the bindings. `frontend/bindings/postacontrol/core/model/models.ts` must then show `Label`, `labels` on `MessageSummary` and `ThreadSummary`, and `labelId` on `SearchQuery`.
- [ ] **Step 4:** `task check` is green. The frontend still compiles: the new fields are optional to the code that does not use them yet. If some existing test builds a `MessageSummary` by hand and `tsc` asks for `labels`, add `labels: []` to that fixture.
- [ ] **Step 5: Commit** `feat(etiquetas): filtro por etiqueta en el núcleo y métodos del servicio`.

### Task 5: Frontend: datos de etiquetas, eventos y sección en Configuración

**Files:**
- Create: `frontend/src/lib/labels.ts`, `frontend/src/lib/labels.test.ts`, `frontend/src/features/settings/LabelsSection.tsx`, `frontend/src/features/settings/LabelsSection.test.tsx`
- Modify: `frontend/src/lib/queries.ts` (`useLabels`), `frontend/src/lib/events.ts`, `frontend/src/features/settings/SettingsDialog.tsx`, `frontend/src/features/folders/FolderSidebar.tsx`

**Interfaces:**
- **Produces, in `lib/labels.ts`:**
  - `LABEL_COLORS: readonly string[]`, the palette from the Global Constraints.
  - `nextColor(used: string[]): string`.
  - `labelChipStyle(color: string): CSSProperties`. It returns `{ color: \`color-mix(in oklab, ${color} 78%, var(--foreground))\`, backgroundColor: \`color-mix(in oklab, ${color} 12%, transparent)\`, borderColor: \`color-mix(in oklab, ${color} 40%, transparent)\` }`, which is readable in light and dark.
  - `orderedLabels(ids: number[], labels: Label[]): Label[]`: it keeps those that exist, in the order of `labels`.
- **Produces, in `lib/queries.ts`:** `useLabels()`, with key `["labels"]` and `svc.ListLabels`, normalized to `[]`.
- **Produces, in `lib/events.ts`:**
  - `requestSettings(section?: "labels")` and `onSettingsRequest(cb)`, a module emitter like `onPasswordRequest`.
  - `useMailEvents` adds:
    - `mail:labels-changed`, which invalidates `["labels"]`, `["messages"]`, `["threads"]`, `["thread"]` and `["search"]`, plus `["message"]` with `refetchType: "none"`;
    - `mail:labels-local-only`, which shows `toast.warning` with the exact text and `duration: 10000`, and an `id` per account so it does not repeat.
  - In `mail:folder-updated`, also invalidate `["messages", "label"]`.
- **Produces, as a component:** `LabelsSection({ focus?: boolean })`, with `id="settings-labels"`; with `focus` it scrolls into view.

- [ ] **Step 1: Write the failing tests.**
  - `labels.test.ts`:
    - `nextColor([])` is `"#dc2626"`;
    - `nextColor(["#dc2626"])` is `"#ea580c"`;
    - with all of them used, it returns `"#dc2626"`;
    - `orderedLabels([3,1,99], [{id:1},{id:2},{id:3}])` returns ids `[1,3]`.
  - `LabelsSection.test.tsx`, with mocked bindings like `SignaturesSection.test.tsx`:
    - "Nueva etiqueta" creates with the name typed and `nextColor` (it calls `CreateLabel(name, color)`);
    - the core error is shown with `toast.error`;
    - renaming with Enter or on blur calls `UpdateLabel`;
    - picking a color in the popover (12 buttons with `aria-label` equal to the hex) calls `UpdateLabel`;
    - the ↑/↓ buttons call `ReorderLabels` with the new order, and ↑ is disabled on the first one;
    - "Borrar" opens the confirmation with the text for `LabelUsage` = 3 (`Se saca de 3 correos. Los correos no se borran.`), for 1 and for 0; "Borrar" calls `DeleteLabel`;
    - with no labels, `Todavía no tenés etiquetas.` is shown.
  - `events.test.ts`: `mail:labels-local-only` shows the toast with the email.
- [ ] **Step 2:** `npx vitest run src/lib/labels.test.ts src/features/settings` fails.
- [ ] **Step 3: Implement.**
  - The section lives in SettingsDialog after Firmas, with the same visual pattern.
  - Each row shows: a color swatch that opens a `Popover` with the palette, the name in an `Input`, ↑ ↓ and a delete button.
  - The section also shows the key number: "1"…"9" for the first nine, nothing from the tenth on.
  - `FolderSidebar` subscribes to `onSettingsRequest` to open the dialog with `focus`.
- [ ] **Step 4:** `task check` is green.
- [ ] **Step 5: Commit** `feat(etiquetas): sección Etiquetas en Configuración`.

### Task 6: Frontend: línea y pastillas en la fila, etiquetas en el lector y menú compartido

**Files:**
- Create:
  - `frontend/src/features/labels/LabelStripe.tsx`, with the line and the pills;
  - `frontend/src/features/labels/LabelMenu.tsx`, with the menu items, in two versions: one for `DropdownMenu` and one for `ContextMenu`;
  - `frontend/src/features/labels/applyLabel.ts`, plus tests for all three.
- Modify: `frontend/src/features/messages/MessageRow.tsx`, `frontend/src/features/messages/ThreadList.tsx`, `frontend/src/features/reader/MessageView.tsx`, `frontend/src/features/reader/MessageActions.tsx`, `frontend/src/lib/bulk.ts` (`Target.labels: number[]`, filled by `loadedTargets` and `threadTargets`)

**Interfaces:**
- **Produces, in `applyLabel.ts`:**
  - `labelState(targets: Target[], labelId: number): "all" | "some" | "none"`.
  - `toggleTargets(targets: Target[], labelId: number): { on: boolean }`: on is `true` unless the state is `"all"`.
  - `applyLabel(queryClient, targets: Target[], labelId: number): Promise<{ failed: AccountFailure[] }>`, which uses `runPerAccount` + `svc.SetLabel`, then `invalidateMailQueries`, and shows `toast.error` per failure.
- **Produces, in `LabelMenu.tsx`:** `LabelMenuItems({ kind: "dropdown" | "context", targets: Target[] })`, with one item per label. Each item has:
  - a dot of its color;
  - a check (`CheckIcon`) if `"all"`, or a dash (`MinusIcon`) if `"some"`;
  - its key number 1–9 on the right.

  At the end there is a separator and `Administrar etiquetas…`, which calls `requestSettings("labels")`.
- **Produces, in `LabelStripe.tsx`:**
  - `LabelLine({ ids })`: an absolute line of 4px on the left edge of the row, split into equal vertical segments, one per color in order.
  - `LabelChips({ ids, max })`: pills with `labelChipStyle` and, if there are more than `max`, a final `+N`. `max` is 2 on the one-line row and 3 on the three-line row.

- [ ] **Step 1: Write the failing tests.**
  - `applyLabel.test.ts`:
    - `labelState` with `[{labels:[1]},{labels:[]}]` and 1 is `"some"`;
    - `toggleTargets` returns on=true for `"some"` and `"none"`, and false for `"all"`;
    - `applyLabel` with targets from two accounts calls `SetLabel` twice, once per account.
  - `LabelStripe.test.tsx`: two ids render two segments with the background colors of those labels; `LabelChips` with 4 ids and max 2 shows 2 names and `+2`; unknown ids are not drawn.
  - `MessageRow` (existing test file, or new cases):
    - a message with `labels:[A]` shows the line and the pill with A's name;
    - with no labels, there is no line;
    - the selection checkbox remains clickable (`left-0.5` is not covered: the line has `pointer-events-none`).
  - `MessageView`: pills with ✕ (`aria-label="Quitar etiqueta Facturas"`); clicking the ✕ calls `SetLabel([id], A, false)`. The "Etiquetas" button opens the menu with the check on the label the message has.
  - `ThreadList`: a thread with `labels:[A,B]` shows the line split in two.
- [ ] **Step 2:** the tests fail.
- [ ] **Step 3: Implement.**
  - Rows read the labels with `useLabels()`; to avoid one hook per row, pass them down from the list if needed.
  - Pills go between the sender and the subject on the one-line row, and on the subject line on the three-line row.
  - In the reader, the pills go under the sender/date block. The "Etiquetas" button goes in `MessageActions`, next to Destacar, with `TagIcon`.
- [ ] **Step 4:** `task check` is green.
- [ ] **Step 5: Commit** `feat(etiquetas): línea y nombres en la fila, y etiquetas en el correo abierto`.

### Task 7: Frontend: clic derecho en filas, teclas 1–9 y barra de selección

**Files:**
- Create: `frontend/src/features/labels/useLabelKeys.ts` + test, `frontend/src/features/messages/RowContextMenu.tsx` + test
- Modify: `MessageRow.tsx`, `ThreadList.tsx`, `SearchResults.tsx` (if it renders `MessageRow` without passing what is needed), `MessageList.tsx`, `SelectionBar.tsx`, `MailShell.tsx` (it mounts `useLabelKeys`)

**Interfaces:**
- **Consumes:** Task 6 (`LabelMenuItems`, `applyLabel`, `Target.labels`) and the `ShellState`.
- **Produces:**
  - **`RowContextMenu`**, with props `{ targets: () => Promise<Target[]>; children }`. It wraps the row with radix `ContextMenu`. The items are:
    - `Etiquetas ›`, a submenu with `LabelMenuItems kind="context"`;
    - `Marcar como leído` / `Marcar como no leído`, depending on whether all are read, through `runPerAccount` + `svc.SetSeen`;
    - `Destacar` / `Quitar destacado`, through `svc.SetFlagged`.

    Ruling: Mover and Borrar are left out (they need a destination and confirmation; the selection bar and the reader already have them).
  - **Context-menu targets:**
    - if the row is marked, all the marked ones (the same `barTargets`);
    - if not, only that row;
    - on a thread row, `threadTargets(folderId, [root])`.
  - **`useLabelKeys(state: ShellState)`**: a `keydown` on `window`. Keys `1`…`9` with no modifier, outside an `input`/`textarea`/`select`/`[contenteditable]` and with no `[role=dialog]` open, act on the targets in this order:
    1. what is marked (messages or threads);
    2. the open message;
    3. the open thread (all its messages).

    The key applies `applyLabel` with the label at position N−1, if it exists.
  - **`SelectionBar`**: an "Etiquetas" button (`TagIcon`) with `DropdownMenu` + `LabelMenuItems kind="dropdown"` over `current`.

- [ ] **Step 1: Write the failing tests.**
  - `useLabelKeys.test.ts`:
    - with a message open and labels `[A,B]`, pressing `2` calls `SetLabel([id], B, true)`;
    - pressing `2` again on a message that already has B calls it with `false`;
    - with the focus in an `<input>`, or `Ctrl+2`, or a `role=dialog` open, or `9` with only 2 labels → no call;
    - with 2 messages marked, one with A and one without, pressing `1` puts A on both.
  - `RowContextMenu.test.tsx`:
    - right-clicking an unmarked row opens the menu; under `Etiquetas` you see the labels, and picking one calls `SetLabel` with that id;
    - on a marked row, it acts on all the marked ones;
    - `Marcar como no leído` calls `SetSeen(ids, false)`.
  - `SelectionBar.test.tsx`: the "Etiquetas" button shows the dash when some of the marked have the label.
- [ ] **Step 2:** the tests fail.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** `task check` is green.
- [ ] **Step 5: Commit** `feat(etiquetas): poner etiquetas con clic derecho, teclas 1 a 9 y la barra de selección`.

### Task 8: Frontend: filtro por etiqueta

**Files:**
- Modify: `frontend/src/features/layout/MailShell.tsx` (`labelFilter` in `ShellState`), `frontend/src/lib/queries.ts` (`useLabeled`, `useSearch` with labelId), `frontend/src/features/messages/MessageList.tsx`, `frontend/src/features/search/SearchResults.tsx`
- Create: `frontend/src/features/labels/LabelFilter.tsx` + test
- Test: `MessageList.test.tsx`

**Interfaces:**
- **Produces:**
  - **`ShellState.labelFilter: number | null`** and **`setLabelFilter(id: number | null)`**. `selectView` clears it. Changing it calls `clearMarks()`.
  - **`useLabeled(labelId: number | null, accountId: number, unreadOnly: boolean)`**: an infinite query with key `["messages", "label", labelId, accountId, unreadOnly]` over `svc.ListLabeled`, with pages of `PAGE_SIZE`. It is enabled when `labelId != null`.
  - **`LabelFilter({ value, onChange })`**:
    - with no label chosen, it shows "Etiqueta ▾";
    - with one chosen, it shows the color dot and the name, plus the ✕;
    - its menu has one item per label, with a check on the current one, and the empty state if there are no labels.
  - **`MessageList`:**
    - with `labelFilter`, the list comes from `useLabeled`;
    - accountId is 0 in the unified view and the account of the open folder otherwise;
    - threading is turned off (`canThread` is false), the folder name is shown in each row (`folderLabel`, like `SearchResults`), and the empty text is the one from the constraints;
    - when the filter is chosen, it fires `svc.RefreshLabel(id, accountId)` once without waiting. When it resolves, it invalidates `["messages", "label"]`; errors are ignored, since offline the cache is enough.
  - **`useSearch(text, folderId, labelId)`**: it adds labelId to the key and to the `Search` call. With an active search and an active filter, `SearchResults` passes `labelFilter`.
  - **Removing the label being filtered** from a message makes it disappear, because of the `["messages"]` invalidation.
  - **If the filtered label gets deleted** (it is no longer in `useLabels`), the filter is cleared.

- [ ] **Step 1: Write the failing tests.**
  - `LabelFilter.test.tsx`: it opens the menu, picks "Facturas proveedores" → `onChange(id)`; with that value it shows the name, and the ✕ → `onChange(null)`.
  - `MessageList.test.tsx`:
    - **Folder view:** with the filter on label L, it calls `ListLabeled(L, accountOfFolder, …)` and not `ListMessages`; it shows the folder name on each row; it calls `RefreshLabel(L, accountId)` once.
    - **Unified view:** it calls `ListLabeled(L, 0, …)`.
    - **"No leídos" with the filter:** `unreadOnly: true` reaches `ListLabeled`.
    - **Empty:** `No hay correos con esta etiqueta.`.
    - **Changing view:** `selectView` clears the filter.
    - **Threads:** with conversations on and the filter active, it renders a flat list.
  - `SearchResults.test.tsx`: with `labelFilter`, `Search` receives `labelId`.
- [ ] **Step 2:** the tests fail.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** `task check` is green.
- [ ] **Step 5: Commit** `feat(etiquetas): filtro por etiqueta en la lista`.

### Task 9: Prueba manual de Fabricio (no la hace un subagente)

To record in the registro as pending, with `pkill -x postacontrol; task install:desktop`:
- [ ] **Create:** create "Facturas proveedores", "Por hacer" and "Seguimiento" with colors, then reorder them and check the number on each.
- [ ] **Label a mail:** with the right click, with key 1 and from the open mail, you see the line and the name in the list.
- [ ] **Two labels:** a mail with two labels shows the split line and both names, in light and dark themes.
- [ ] **Move:** move a labeled mail to another folder, then filter by that label from the Entrada; the mail shows up, with the name of the new folder.
- [ ] **Unified inbox:** the filter brings mail from every account.
- [ ] **Take one off:** "Por hacer" from the ✕ of the open mail, with the filter on "Por hacer"; the mail leaves the list.
- [ ] **Delete a label** with mails: check that the mails are still there and that the label is no longer on any of them.
- [ ] **Hostinger:** if the "no guarda etiquetas" notice appears on any account, write down which one.

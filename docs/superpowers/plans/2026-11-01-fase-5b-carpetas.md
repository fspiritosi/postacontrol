# PostaControl — Fase 5B "Gestión de carpetas" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Fabricio pueda crear, renombrar, mover, reordenar y eliminar carpetas de una cuenta desde el panel izquierdo, con menú contextual y arrastrando, sin perder nunca un correo.

**Architecture:**
- **IMAP (`core/imapx/folders.go`):** se suman `CREATE`, `RENAME`, `DELETE`, `SUBSCRIBE`/`UNSUBSCRIBE`, el prefijo personal de `NAMESPACE`, `STATUS` de una carpeta y `LIST` de un subárbol.
- **Núcleo (`core/mail/folders.go`):** valida, protege las carpetas especiales, escribe en el servidor y recién después pone al día la caché.
  - Renombrar y mover son un mismo `RENAME`.
  - La caché conserva el id y los correos de la carpeta y de sus subcarpetas: se cambian los nombres por prefijo.
- **Eliminar:** primero se mueven a la Papelera todos los correos del subárbol, según el servidor y no según la caché. Recién se borra cada carpeta después de comprobar en el servidor que quedó vacía. Si un paso falla, no se borra nada de ahí para arriba.
- **Orden:** es una columna nueva `folders.position` (schema v8).
  - `ReplaceFolders` actualiza por `(account_id, name)`, así que la posición sobrevive a la sincronización.
  - Como el renombre local cambia el `name` de la misma fila, la posición acompaña a la carpeta.
- **Frontend:**
  - menú contextual (shadcn `context-menu`) en cada carpeta;
  - diálogos para nombre, destino y confirmación de borrado;
  - arrastrar y soltar carpetas, que extiende `lib/dnd.ts` de la 5A.

**Tech Stack:** el de siempre, más el componente shadcn `context-menu`.

**Spec:** `docs/superpowers/specs/2026-11-01-carpetas-lectura-seleccion-design.md`: sección "Carpetas", "Decisiones mías" 1 y 3, y "Restricciones". Spec base: `docs/superpowers/specs/2026-10-30-postacontrol-design.md`, sobre todo el anti-objetivo de perder correo.

## Global Constraints

- **Las reglas del proyecto y de la 5A siguen vigentes**, en la misma rama `fase-5`.
- **Escrituras en el servidor:** solo por una acción explícita de Fabricio. Nunca un `EXPUNGE` sin UID; mover usa `imapx.Conn.Move`, que ya cumple.
  - Cada escritura sigue el patrón de `applyMove`: primero `withConn` con `Noop`, después `withConnNoRetry` con la escritura, sin reintento.
- **Carpeta especial:** la que tiene `role != ""` (Entrada, Enviados, Borradores, Papelera, Spam, Archivo).
  - No se elimina, no se renombra y no se mueve, pero sí se reordena.
  - Una carpeta que **contiene** una especial en su subárbol (por nombre, con el delimitador) tampoco se elimina, renombra ni mueve (Decisión mía 1).
  - El núcleo lo verifica siempre. La UI además deshabilita esas opciones.
- **Todo pasa dentro de la misma cuenta:** no se mueve una carpeta a otra cuenta.
- **Nombre válido:** se le sacan los espacios de los bordes, no puede quedar vacío y no puede contener el delimitador de la cuenta.
  - Tampoco puede coincidir con una carpeta hermana. La comparación no distingue mayúsculas, y la `INBOX` de cualquier forma cuenta como especial.
- **Orden:**
  - Hermanos con `position > 0` van primero, por `position`.
  - Después van los de `position == 0`, en el orden de hoy: en la raíz, el rango de rol y después alfabético.
  - Una carpeta creada recibe `MAX(position)+1` de la cuenta si la cuenta ya tiene alguna posición; si no, `0`.
  - Ruling: el spec pide "al final de su nivel". Si nunca se reordenó, el nivel es alfabético y la nueva entra en su lugar alfabético. Si me equivoqué, una carpeta nueva no queda última en un nivel que nunca se ordenó.
- **Dónde se crea una carpeta de primer nivel:** con el prefijo personal de `NAMESPACE` si el servidor lo informa (por ejemplo `INBOX.` en Dovecot), o en la raíz si no.
  - `buildFolderTree` ya cuelga esas carpetas donde corresponde.
- **Suscripción:** `SUBSCRIBE` al crear y al renombrar (el nombre nuevo); `UNSUBSCRIBE` del nombre viejo y al borrar.
  - Si la suscripción falla, la operación igual cuenta como hecha; solo se registra en el log.
- **Textos exactos:**
  - Menú contextual: "Nueva subcarpeta…", "Renombrar…", "Mover a…", "Subir", "Bajar", "Eliminar…".
  - Menú de la cuenta: "Nueva carpeta…".
  - Diálogo de nombre:
    - títulos "Nueva carpeta", "Nueva subcarpeta de «<carpeta>»" y "Renombrar «<carpeta>»";
    - campo "Nombre";
    - botones "Crear", "Renombrar" y "Cancelar".
  - Diálogo de mover:
    - título "Mover «<carpeta>»";
    - la primera opción es "Primer nivel";
    - botones "Mover" y "Cancelar".
  - Diálogo de eliminar:
    - título "¿Eliminar «<carpeta>»?";
    - vacía y sin subcarpetas: sin descripción extra;
    - con correos o subcarpetas: "Tiene <N> correos y <S> subcarpetas (<«Acme», con 7 correos>; …). Los <T> correos van a la Papelera y las carpetas se borran del servidor." Con 1 se dice "1 correo" y "1 subcarpeta"; sin subcarpetas se omite esa parte;
    - botones "Cancelar" y "Eliminar".
  - Toasts: "Carpeta «<nombre>» creada", "Carpeta eliminada; <T> correos a la Papelera" (o solo "Carpeta eliminada" si no tenía correos).
  - Errores del núcleo (`UserError`, código entre paréntesis):
    - (`invalid`) "Escribí un nombre para la carpeta."
    - (`invalid`) "El nombre no puede llevar «<delimitador>»."
    - (`duplicate`) "Ya hay una carpeta «<nombre>» ahí."
    - (`invalid`) "Las carpetas especiales no se pueden cambiar." También vale para una carpeta que contiene una especial.
    - (`invalid`) "No se puede mover una carpeta dentro de sí misma."
    - (`no_trash`) "Esta cuenta no tiene carpeta Papelera." Ya existe como `msgNoTrash`, y se usa si hay correos para mover.
    - (`stale`) "Llegaron correos nuevos a «<carpeta>» mientras se vaciaba; probá de nuevo." Se usa si después de tres pasadas de mover la carpeta sigue sin quedar vacía.

## Review Focus

1. **Correos que están en el servidor pero no en la caché** (una carpeta nunca abierta, o abierta solo en parte). El aviso los cuenta y se mueven igual: la cantidad y los UIDs salen de `STATUS` y `UID SEARCH ALL` en el servidor. Lo cubre el test de la Task 3 "eliminar mueve también lo que no está en la caché".
2. **Falla a mitad del borrado:** el `MOVE` de una subcarpeta falla o se corta la conexión. Esa carpeta y sus padres no se borran, y el error llega a la UI. Lo cubre el test de la Task 3 "si un paso falla no se borra nada de ahí para arriba".
3. **Renombrar o mover una carpeta que tiene subcarpetas y correos en caché.** Se conservan los ids y los correos de la caché, y la carpeta abierta sigue abierta. Lo cubre el test de la Task 2 "renombrar conserva la caché del subárbol".
4. **Nombre con acentos o espacios, como "Facturación 2026".** Se crea, se lista y se ve igual (go-imap codifica UTF-7 modificado). Lo cubre el test de la Task 2 "crear con acentos".
5. **Soltar una carpeta sobre una descendiente suya, o sobre una especial que la tenga adentro.** No hace nada, y el núcleo lo rechaza aunque la UI lo deje pasar. Lo cubren los tests de la Task 2 "mover dentro de sí misma falla" y de la Task 5 "no se ofrece soltar sobre una descendiente".

---

### Task 1: IMAP y store para carpetas

**Files:**
- Modify: `core/imapx/folders.go`, `core/imapx/imapx_test.go`
- Create: `core/store/schema_v8.sql`
- Modify: `core/store/migrate.go`, `core/store/folders.go`, `core/store/store_test.go` (o un `folders_test.go` nuevo)
- Modify: `core/model/model.go` (`Folder.Position int \`json:"position"\``)

**Interfaces:**
- Produces, en `imapx.Conn`:
  - `CreateMailbox(name string) error`;
  - `RenameMailbox(oldName, newName string) error`;
  - `DeleteMailbox(name string) error`;
  - `Subscribe(name string) error` y `Unsubscribe(name string) error`;
  - `PersonalPrefix() (prefix string, err error)`: con `NAMESPACE` devuelve el prefijo del primer namespace personal, y sin la capacidad devuelve `""`;
  - `MailboxTotal(name string) (int, error)`: `STATUS (MESSAGES)`;
  - `ListUnder(name, delim string) ([]RemoteFolder, error)`: `LIST "" "<name><delim>*"`, sin `STATUS`, y no incluye a `name`.
- Produces, en `store.Store`:
  - `RenameFolderTree(ctx, accountID int64, oldName, newName, delim string) error`: en una transacción, cambia el `name` de la carpeta `oldName` y de todas las que empiezan con `oldName+delim`, reemplazando ese prefijo. Conserva ids, mensajes, `uid_validity`, `uid_next` y `position`;
  - `SetFolderPositions(ctx, accountID int64, ids []int64) error`: `position = índice+1` en ese orden. Las carpetas de otra cuenta se ignoran;
  - `NextFolderPosition(ctx, accountID int64) (int, error)`: `MAX(position)+1`, o 0 si todas son 0;
  - `SetFolderPosition(ctx, folderID int64, pos int) error`;
  - `DeleteFolderRow(ctx, folderID int64) error`, que borra en cascada los mensajes de la caché.
- `schema_v8.sql`: `ALTER TABLE folders ADD COLUMN position INTEGER NOT NULL DEFAULT 0;`. Se agrega a `folderCols` y `scanFolder`.

- [ ] **Step 1: Tests de imapx**, contra `testenv.StartIMAP`:
  - crear, renombrar y borrar una carpeta y ver el `LIST`;
  - `MailboxTotal` después de `Deliver` dos veces da 2;
  - `ListUnder("Clientes", delim)` devuelve `Clientes<d>Acme`;
  - `PersonalPrefix` contra el servidor de pruebas da `""`, o lo que informe si soporta `NAMESPACE`.
  - Antes de escribir los asserts de `RenameMailbox` con subcarpetas, verificá si `imapmemserver` renombra las inferiores. Si no lo hace, el test cubre solo la carpeta hoja.
- [ ] **Step 2: Tests del store:**
  - `RenameFolderTree` cambia `Clientes` y `Clientes/Acme` a `Proyectos` y `Proyectos/Acme` con los mismos ids y mensajes, y no toca `ClientesViejos`, porque el prefijo exige el delimitador;
  - `SetFolderPositions`;
  - `NextFolderPosition` con todo en 0 da 0, y con `[0, 3]` da 4;
  - la migración v8 sobre una base v7 deja `position = 0`.
- [ ] **Step 3:** Correr `go test ./core/imapx/ ./core/store/`. Esperado: FAIL.
- [ ] **Step 4:** Implementar.
- [ ] **Step 5:** Correr los tests. Esperado: PASS. Después, `task check` verde y commit `feat(store): posición de carpetas y operaciones IMAP de buzones`.

### Task 2: Crear, renombrar, mover y ordenar en el núcleo

**Files:**
- Create: `core/mail/folders.go`, `core/mail/folders_test.go`
- Modify: `app/mailservice.go` (fachada fina) y regenerar bindings

**Interfaces:**
- Consumes: lo producido en la Task 1.
- Produces, en `*Core` y con el mismo nombre en `MailService`:
  - `CreateFolder(ctx, accountID int64, parentFolderID int64, name string) (model.Folder, error)`. Con `parentFolderID == 0` va al primer nivel (prefijo personal). Si el padre existe, el nombre completo es `padre.Name + delim + name`;
  - `RenameFolder(ctx, folderID int64, name string) (model.Folder, error)`: cambia solo el último segmento;
  - `MoveFolder(ctx, folderID int64, newParentFolderID int64) (model.Folder, error)`: `0` es el primer nivel; conserva el último segmento;
  - `SetFolderOrder(ctx, accountID int64, folderIDs []int64) error`.
  - Todas emiten `EventFoldersUpdated` con `{"accountId": …}`.
  - **El delimitador** es el de las carpetas de la cuenta en la caché, y si no hay ninguna, el del `LIST`.
  - **Una carpeta padre `\Noselect`** que existe en la caché sirve como padre. Un padre sintetizado por la UI (`id == 0`) no existe acá: la UI no ofrece crear adentro de uno así.

- [ ] **Step 1: Tests** (`folders_test.go`, con `testenv`):
  - **"crear en primer nivel":** aparece en `ListFolders` con `Position` según `NextFolderPosition` y queda suscripta (verificalo con `LIST` o `LSUB` del servidor de pruebas, si lo expone);
  - **"crear subcarpeta";**
  - **"crear con acentos":** "Facturación 2026" (Review Focus 4);
  - **"nombre vacío", "con delimitador" y "duplicado sin distinguir mayúsculas":** los textos y códigos exactos de Global Constraints;
  - **"renombrar conserva la caché del subárbol"** (Review Focus 3): `Clientes` con 2 correos sincronizados y `Clientes/Acme` con 1. Al renombrar a `Proyectos`, los ids de carpeta y de mensaje no cambian y `SyncFolder` de `Proyectos` no vuelve a bajar cabeceras;
  - **"mover a otro padre" y "mover al primer nivel";**
  - **"mover dentro de sí misma falla":** también a una descendiente (Review Focus 5);
  - **"especial no se renombra ni se mueve":** tampoco una carpeta con una especial adentro, como `Padre` con `Padre/Trash` de rol trash;
  - **"mover a un padre de otra cuenta falla" (`invalid`);**
  - **"SetFolderOrder guarda las posiciones":** `ListFolders` las devuelve.
- [ ] **Step 2:** Correr `go test ./core/mail/ -run Folder`. Esperado: FAIL.
- [ ] **Step 3: Implementar.**
  - Las validaciones van antes de escribir en el servidor.
  - Después de `RENAME`, el orden es:
    1. `store.RenameFolderTree`;
    2. `SUBSCRIBE` del nombre nuevo y `UNSUBSCRIBE` del viejo;
    3. el mismo refresco de carpetas que hace `SyncAccount` hasta `EventFoldersUpdated`, pero sin sincronizar la Entrada. Conviene extraer `refreshFolders(ctx, accountID)` de `SyncAccount` y usarlo en los dos lados.
- [ ] **Step 4:** Correr los tests. Esperado: PASS. Después, `task common:generate:bindings`, `task check` verde y commit `feat(carpetas): crear, renombrar, mover y ordenar carpetas`.

### Task 3: Eliminar una carpeta sin perder correo

**Files:**
- Modify: `core/mail/folders.go`, `core/mail/folders_test.go`, `core/model/model.go`, `app/mailservice.go`, bindings

**Interfaces:**
- Produces:
  - `model.FolderDeleteInfo{ Total int \`json:"total"\`; Subfolders []model.SubfolderCount \`json:"subfolders"\` }`, con `model.SubfolderCount{ Name string \`json:"name"\`; Total int \`json:"total"\` }`. `Total` es solo el de la carpeta; el total general lo suma la UI;
  - `DeleteFolderPreview(ctx, folderID int64) (model.FolderDeleteInfo, error)`: lee del servidor, con `ListUnder` y `MailboxTotal`;
  - `DeleteFolder(ctx, folderID int64) (moved int, err error)`.

- [ ] **Step 1: Tests:**
  - **"preview cuenta según el servidor":** 3 correos entregados con `Deliver` y ninguno sincronizado da `Total 3`;
  - **"eliminar carpeta vacía":** desaparece del servidor y de la caché, y devuelve 0;
  - **"eliminar mueve también lo que no está en la caché"** (Review Focus 1): `Clientes` con 2 sin sincronizar y `Clientes/Acme` con 1. La Papelera del servidor queda con 3, las dos carpetas desaparecen y devuelve 3;
  - **"sin Papelera y con correos":** falla con `no_trash` y no borra nada;
  - **"sin Papelera y vacía":** se borra;
  - **"especial o con una especial adentro no se elimina";**
  - **"si un paso falla no se borra nada de ahí para arriba"** (Review Focus 2): se simula la falla con un `MOVE` imposible, por ejemplo borrando la Papelera en el servidor con `testenv.DeleteMailbox` justo antes de llamar a `DeleteFolder`, con la caché todavía creyendo que existe. El resultado es un error, y `Clientes` y `Clientes/Acme` siguen existiendo en el servidor;
  - **"Noselect con hijos":** se borran los hijos y después el padre, sin `SELECT` del padre.
- [ ] **Step 2:** Correr los tests. Esperado: FAIL.
- [ ] **Step 3: Implementar `DeleteFolder`.**

  Este orden lo decide el plan:
  1. Validar: no es especial ni contiene una especial, y existe.
  2. `ListUnder` y armar la lista con la carpeta, de la más profunda a la menos profunda (por cantidad de segmentos).
  3. Por cada carpeta seleccionable de la lista:
     - `SelectRW`;
     - `AllUIDs`;
     - si hay UIDs y no hay Papelera, devolver `no_trash`;
     - `Move(uids, trash.Name)`;
     - volver a contar con `MailboxTotal`;
     - hasta 3 pasadas; si sigue con correos, devolver `stale`.
  4. Recién con la carpeta vacía (o si es `\Noselect`), en este orden: `DeleteMailbox`, `Unsubscribe` y `store.DeleteFolderRow` si la caché la tiene. Ante cualquier error se corta y se devuelve.
  5. Al final, o al cortar:
     - `refreshFolders`;
     - `EventFolderUpdated` de la Papelera;
     - `spawn` de `SyncFolder` de la Papelera, como `runMoves`.

  Las escrituras siguen el patrón `Noop` + `withConnNoRetry` de `applyMove`.
- [ ] **Step 4:** Correr los tests. Esperado: PASS. Después, bindings, `task check` verde y commit `feat(carpetas): eliminar carpetas mandando sus correos a la Papelera`.

### Task 4: Menú contextual y diálogos

**Files:**
- Create: `frontend/src/components/ui/context-menu.tsx`, con `npx shadcn@latest add context-menu`. Revisá que el import de `cn` sea `@/lib/utils`: en la Fase 4 el generador puso `"cn"`.
- Create: `frontend/src/features/folders/FolderDialogs.tsx`, `frontend/src/features/folders/FolderDialogs.test.tsx`
- Modify: `frontend/src/features/folders/AccountSection.tsx`, `frontend/src/lib/folders.ts`, `frontend/src/lib/folders.test.ts`, `frontend/src/features/folders/FolderSidebar.test.tsx`

**Interfaces:**
- Consumes: los métodos de las Tasks 2 y 3 (bindings).
- Produces, en `lib/folders.ts`:
  - `buildFolderTree` ordena hermanos según Global Constraints (`position`);
  - `isSpecialTree(node: FolderNode): boolean`: el nodo o algún descendiente tiene rol;
  - `canModify(node: FolderNode): boolean`: `node.id !== 0 && !isSpecialTree(node)`;
  - `siblingsOf(tree: FolderNode[], node: FolderNode): FolderNode[]`;
  - `moveTargets(tree: FolderNode[], node: FolderNode): FolderNode[]`: padres posibles con `id !== 0`, de la misma cuenta, sin el nodo, sus descendientes ni su padre actual.

- [ ] **Step 1: Tests:**
  - `folders.test.ts`:
    - orden por `position` con los de 0 al final;
    - en la raíz sin posiciones, el orden de hoy;
    - `isSpecialTree`;
    - `moveTargets` excluye descendientes.
  - `FolderDialogs.test.tsx` y `FolderSidebar.test.tsx`:
    - **"Nueva carpeta… desde el menú de la cuenta crea en primer nivel":** `CreateFolder(accountId, 0, "Clientes")` y toast "Carpeta «Clientes» creada";
    - **"Nueva subcarpeta…":** usa el id del padre;
    - **"Renombrar…":** viene con el nombre actual en el campo;
    - **"un error del núcleo se muestra en el diálogo y no lo cierra";**
    - **"Mover a… ofrece Primer nivel y los destinos válidos";**
    - **"Subir y Bajar":** `SetFolderOrder` con los hermanos en el orden nuevo; "Subir" está deshabilitado en el primero;
    - **"en una especial solo hay Subir y Bajar habilitados";**
    - **"Eliminar… muestra el texto con los conteos del preview":** el texto exacto, con 48 + «Acme», con 7 correos, da "Los 55 correos…";
    - **"Eliminar en vacía no lleva descripción";**
    - **"después de eliminar, si estaba abierta, la vista vuelve a la por defecto":** sale solo por `viewIsValid` de `MailShell`, pero el test lo confirma.
- [ ] **Step 2:** Correr los tests. Esperado: FAIL.
- [ ] **Step 3: Implementar.**
  - **`FolderRow`** se envuelve en `ContextMenu`. Los padres sintetizados (`id === 0`) no tienen menú.
  - **"Nueva carpeta…"** va en el `DropdownMenu` de la cuenta.
  - **Diálogo de eliminar:** carga el preview al abrirse y muestra "Contando correos…" mientras tanto. Si el preview falla, muestra el error y deshabilita "Eliminar".
  - **Mientras corre `DeleteFolder`,** el botón dice "Eliminando…" y queda deshabilitado.
- [ ] **Step 4:** Correr `task check`. Esperado: verde. Commit `feat(carpetas): menú contextual y diálogos de carpetas`.

### Task 5: Arrastrar carpetas

**Files:**
- Modify: `frontend/src/lib/dnd.ts`, `frontend/src/lib/dnd.test.ts`, `frontend/src/features/folders/AccountSection.tsx`, `frontend/src/features/folders/FolderSidebar.test.tsx`

**Interfaces:**
- Consumes: `accountMime` y la zona de soltar de la 5A Task 5; `moveTargets`, `siblingsOf` y `canModify` (Task 4).
- Produces:
  - `FOLDER_MIME = "application/x-postacontrol-folder"`;
  - `setFolderDrag(dt, accountId, folderId)` y `readFolderDrag(dt): number | null`;
  - `dropZone(offsetY: number, height: number): "before" | "into" | "after"`: el cuarto de arriba es `before`, el de abajo es `after` y el resto es `into`.

- [ ] **Step 1: Tests:**
  - `dnd.test.ts`: `dropZone` en los bordes y en el medio;
  - `FolderSidebar.test.tsx`:
    - **"soltar una carpeta en el medio de otra la mete adentro":** `MoveFolder(id, destId)`;
    - **"soltar entre dos cambia el orden":** `SetFolderOrder` con los hermanos y la carpeta en el lugar nuevo. Si viene de otro nivel, primero `MoveFolder` al padre de destino y después el orden;
    - **"no se ofrece soltar sobre una descendiente"** (Review Focus 5): sin `preventDefault` en el `dragover`;
    - **"una especial se puede arrastrar solo para reordenar":** `into` no se acepta, `before`/`after` entre hermanos sí;
    - **"correos y carpetas no se confunden":** soltar correos en el borde de una fila igual los mueve a esa carpeta.
- [ ] **Step 2:** Correr los tests. Esperado: FAIL.
- [ ] **Step 3: Implementar.**
  - **Qué se puede arrastrar:** las filas de carpetas con `id !== 0`.
  - **Indicador:**
    - para `into`, el mismo recuadro que en la 5A;
    - para `before` y `after`, una línea de 2px `bg-primary` arriba o abajo de la fila.
  - **Errores del núcleo:** se muestran con `toast.error`.
- [ ] **Step 4:** Correr `task check`. Esperado: verde. Commit `feat(carpetas): arrastrar carpetas para moverlas u ordenarlas`.

### Task 6: Prueba manual de Fabricio

No se despacha. Fabricio la hace con `task install:desktop`, en una cuenta real:
- **Crear:** una carpeta y una subcarpeta, y verlas en el webmail.
- **Renombrar y mover:** una con correos, y comprobar que siguen ahí sin volver a bajar.
- **Ordenar:** reordenar con "Subir" y "Bajar" y arrastrando.
- **Eliminar:** una carpeta con correos y subcarpeta, y comprobar en el webmail que todo quedó en la Papelera.
- **Arrastrar correos:** a una carpeta recién creada.

# Sidebar colapsable, color de carpetas y tema — Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Sidebar plegable a una columna de íconos (con popover de carpetas por cuenta), color por carpeta y tema claro/oscuro/sistema.

**Architecture:** El tema es un campo nuevo de `Settings` que aplica `ThemeProvider` (con copia en `localStorage` para el primer render). El color de carpeta es una columna local de `folders` que el upsert de sincronización no toca. El sidebar usa el modo `collapsible` de `react-resizable-panels`; un flag propio en `localStorage` es la fuente de verdad del plegado, y el árbol de carpetas se extrae a un hook + componente reusable para el popover.

**Tech Stack:** Go (store SQLite, core `mail`, servicio Wails v3), React 19 + TanStack Query, radix-ui (shadcn), react-resizable-panels 4.15, vitest + Testing Library, testify.

**Spec:** `docs/superpowers/specs/2026-11-14-sidebar-colapsable-colores-tema-design.md`

## Global Constraints

- Textos de UI en español rioplatense, como el resto de la app ("Elegí", "Expandí").
- Paleta de carpetas = `LABEL_COLORS` de `frontend/src/lib/labels.ts`, más "Sin color" (`""`).
- Valores de tema: `"light"`, `"dark"`, `"system"`; `""` (configuración vieja) se lee como `"system"`.
- Ancho colapsado del sidebar: `collapsedSize="52px"`.
- El color de carpeta es local: nunca se manda a IMAP.
- Después de cambiar firmas públicas de `app/mailservice.go` o `core/model`, regenerar bindings: `cd /home/fabricio/desarrollo_local/postacontrol && wails3 generate bindings -clean=true -ts -i` y commitear `frontend/bindings`.
- Verificación completa al final: `task check` desde la raíz.
- Trabajo en una rama nueva (`sidebar-colores-tema`), no en `main`.

## Review Focus

1. **Diálogos abiertos desde el popover** (Renombrar, Mover, Eliminar, Nueva subcarpeta): se espera que el diálogo quede abierto aunque el popover se cierre. Por eso los diálogos los dibuja el dueño del popover, fuera de `PopoverContent` (Task 5/7). Test en Task 7.
2. **Arrancar con el sidebar plegado**: se espera que vuelva plegado y que, al desplegarlo, tenga el ancho de antes, no el mínimo. Por eso no se guarda el layout exterior mientras está plegado (Task 6). Test en Task 6.
3. **Sincronizar después de colorear o renombrar**: se espera que el color siga ahí. Tests de store en Task 3.
4. **Arrastrar correos a una cuenta con el sidebar plegado**: se espera que, quedándose sobre la burbuja, se abra el popover para soltar en una carpeta. Test en Task 7.
5. **Tema guardado inválido o `localStorage` inaccesible**: se espera que se use "system" sin romper. Tests en Task 2.

---

### Task 1: Tema en la configuración (Go)

**Files:**
- Modify: `core/model/model.go` (struct `Settings`)
- Modify: `core/mail/settings.go` (`setSettingsLocked`)
- Test: `core/mail/settings_test.go`

**Interfaces:**
- Produces: `model.Settings.Theme string` con tag `json:"theme"`; en TS, `Settings.theme: string`.

- [ ] **Step 1: Tests que fallan** en `core/mail/settings_test.go`, con el mismo armado que `TestSetSettings_ReadingPane`:
  - `TestSetSettings_Theme`: guardar `Theme: "dark"` → `GetSettings` devuelve `"dark"`. Lo mismo para `"light"`, `"system"` y `""`.
  - `TestSetSettings_ThemeInvalid`: guardar `"dark"` y después `"sepia"` → error `*UserError` con `Code == "invalid"`, y lo guardado sigue en `"dark"`.
- [ ] **Step 2:** `go test ./core/mail -run TestSetSettings_Theme -v` → FAIL (no existe el campo).
- [ ] **Step 3:** Agregar `Theme` a `Settings` con este comentario: `// Theme es el tema de la UI: "light", "dark" o "system". Vacío (configuración vieja) se lee como "system".`. En `setSettingsLocked`, validarlo como `ReadingPane`, con el mensaje `"Tema desconocido."`.
- [ ] **Step 4:** `go test ./core/mail -run TestSetSettings -v` → PASS.
- [ ] **Step 5:** Regenerar bindings (ver Global Constraints). En `frontend/src/lib/settings.ts`, `saveSettings` agrega `theme: ""` a los valores por defecto.
- [ ] **Step 6:** Commit `feat(ajustes): campo de tema en la configuración`.

### Task 2: Aplicar y elegir el tema (frontend)

**Files:**
- Create: `frontend/src/lib/theme.ts`, `frontend/src/lib/theme.test.ts`
- Modify: `frontend/src/features/theme/ThemeProvider.tsx`
- Create: `frontend/src/features/theme/ThemeProvider.test.tsx`
- Modify: `frontend/src/main.tsx` (`ThemeProvider` pasa adentro de `QueryClientProvider`)
- Modify: `frontend/src/components/ui/sonner.tsx` (usa el tema efectivo)
- Modify: `frontend/src/lib/events.ts` (mover la invalidación de `["settings"]` a `onComposeDataEvents`)
- Modify: `frontend/src/features/settings/SettingsPages.tsx` (página General)
- Test: `frontend/src/features/settings/SettingsDialog.test.tsx`

**Interfaces:**
- Consumes: `Settings.theme` (Task 1).
- Produces (`lib/theme.ts`):
  - `type ThemePref = "light" | "dark" | "system"`
  - `type ResolvedTheme = "light" | "dark"`
  - `themePref(s: Settings | undefined): ThemePref`: lo desconocido o vacío da `"system"`.
  - `resolveTheme(pref: ThemePref, systemDark: boolean): ResolvedTheme`
  - `loadCachedTheme(): ThemePref` / `saveCachedTheme(p: ThemePref): void`, con la clave `"pc:theme"`. Van en try/catch; si falla o el valor es inválido, se usa `"system"`.
- Produces (`ThemeProvider.tsx`): `useResolvedTheme(): ResolvedTheme`, por contexto.

- [ ] **Step 1: Tests que fallan** en `lib/theme.test.ts`:
  - `themePref(undefined)`, `themePref({theme:""})` y `themePref({theme:"sepia"})` dan `"system"`; `themePref({theme:"dark"})` da `"dark"`.
  - `resolveTheme("system", true)` da `"dark"`, `resolveTheme("system", false)` da `"light"` y `resolveTheme("light", true)` da `"light"`.
  - Con `localStorage.getItem` que lanza error, `loadCachedTheme()` da `"system"`. Con `"pc:theme"="garbage"`, también `"system"`.
- [ ] **Step 2:** `cd frontend && npx vitest run src/lib/theme.test.ts` → FAIL.
- [ ] **Step 3:** Implementar `lib/theme.ts`.
- [ ] **Step 4: Tests que fallan** en `ThemeProvider.test.tsx`, con `GetSettings` mockeado y `matchMedia` falso:
  - "con theme dark pone la clase dark aunque el sistema sea claro".
  - "con theme system sigue al sistema y cambia al disparar change".
  - "antes de cargar los ajustes usa el tema guardado en pc:theme" (`GetSettings` queda pendiente y `pc:theme=dark` → `<html>` tiene `dark`).
  - "guarda en pc:theme la preferencia que llega en los ajustes".
- [ ] **Step 5:** Reescribir `ThemeProvider`. La preferencia es `themePref(settings.data)` si ya cargó y `loadCachedTheme()` si no. Escucha `prefers-color-scheme` solo con `"system"`. Aplica la clase `dark` y `colorScheme` en un `useLayoutEffect`. Llama a `saveCachedTheme` cuando llegan los ajustes. El tema efectivo se expone por contexto. Actualizar el comentario de cabecera. En `main.tsx`, dejar `ThemeProvider` dentro de `QueryClientProvider`, envolviendo app y `Toaster`. En `sonner.tsx`, `theme={useResolvedTheme()}`.
- [ ] **Step 6:** En `lib/events.ts`, mover `Events.On("mail:settings-changed")` → invalidar `["settings"]` a `onComposeDataEvents`, para que la ventana del redactor siga el cambio. La invalidación de `["autoBackup"]` sigue en `useMailEvents`, con su propio listener. Ajustar `lib/events.test.ts` si chequea el lugar.
- [ ] **Step 7: Test que falla** en `SettingsDialog.test.tsx`: "en General, elegir Oscuro guarda theme dark". Abrir General, elegir "Oscuro" en el select "Tema" y esperar `SetSettings` con `expect.objectContaining({ theme: "dark" })`.
- [ ] **Step 8:** En la página General, agregar una `SettingRow` primera con `htmlFor="theme"`, `label="Tema"` y `description="Claro, oscuro o el mismo que el sistema."`. Lleva un `Select` con `value={themePref(settings.data)}` y las opciones `system`="Sistema", `light`="Claro" y `dark`="Oscuro". Guarda con `save({ theme: v })`, igual que el panel de lectura.
- [ ] **Step 9:** `cd frontend && npx vitest run src/lib src/features/theme src/features/settings && npx tsc --noEmit` → PASS.
- [ ] **Step 10:** Commit `feat(tema): elegir tema claro, oscuro o del sistema`.

### Task 3: Color de carpeta (Go)

**Files:**
- Create: `core/store/schema_v16.sql`
- Modify: `core/store/migrate.go` (embed + `migrations`)
- Modify: `core/model/model.go` (`Folder.Color`)
- Modify: `core/store/folders.go` (`folderCols`, `scanFolder`, `SetFolderColor`)
- Modify: `core/mail/folders.go` (`Core.SetFolderColor`)
- Modify: `app/mailservice.go` (`MailService.SetFolderColor`)
- Test: `core/store/store_test.go`, `core/mail/folders_test.go`

**Interfaces:**
- Produces:
  - SQL: `ALTER TABLE folders ADD COLUMN color TEXT NOT NULL DEFAULT '';`
  - `model.Folder.Color string` con tag `json:"color"` y comentario `// Color es el color elegido para la carpeta (#rrggbb); vacío = sin color. Es solo local.`
  - `func (s *Store) SetFolderColor(ctx context.Context, folderID int64, color string) error` (usa `execFolder`).
  - `func (c *Core) SetFolderColor(ctx context.Context, folderID int64, color string) error`: valida `""` o `^#[0-9a-f]{6}$` (si no, `UserError{Code:"invalid", Msg:"Color inválido."}`), guarda y emite `EventFoldersUpdated` con `{"accountId": f.AccountID}`. Toma el lock con `lockFolderAccount`, como las otras operaciones.
  - `func (s *MailService) SetFolderColor(ctx context.Context, folderID int64, color string) error`
  - TS: `svc.SetFolderColor(folderID: number, color: string)`.

- [ ] **Step 1: Tests que fallan** en `core/store/store_test.go`:
  - `TestSetFolderColor_SurvivesReplaceFolders`: `ReplaceFolders` con INBOX + Work, `SetFolderColor(work, "#2563eb")`, otra vez `ReplaceFolders` con la misma lista → `ListFolders` da Work con `Color == "#2563eb"` y el mismo ID.
  - `TestSetFolderColor_SurvivesRename`: colorear Work, `RenameFolderTree(acc, "Work", "Trabajo", "/")` → la carpeta renombrada mantiene el color.
  - `TestOpen_MigratesAndIsIdempotent` sigue pasando con la v16.
- [ ] **Step 2: Tests que fallan** en `core/mail/folders_test.go`:
  - `TestSetFolderColor_SavesAndClears`: `"#16a34a"` se guarda; `""` lo borra.
  - `TestSetFolderColor_Invalid`: `"red"` y `"#FFF"` dan `UserError` con código `"invalid"` y no cambian nada.
  - `TestSetFolderColor_UnknownFolder`: un ID inexistente da error.
- [ ] **Step 3:** `go test ./core/store ./core/mail -run 'SetFolderColor|TestOpen' -v` → FAIL.
- [ ] **Step 4:** Implementar la migración, el modelo, el store, el core y el servicio. El upsert de `ReplaceFolders` no cambia: no lista `color` en `DO UPDATE`.
- [ ] **Step 5:** `go test ./core/... ./app/...` → PASS.
- [ ] **Step 6:** Regenerar bindings. Agregar `color: ""` a los `Folder` armados a mano en los tests del frontend si `tsc` lo pide.
- [ ] **Step 7:** Commit `feat(carpetas): guardar un color local por carpeta`.

### Task 4: Elegir y mostrar el color de carpeta (frontend)

**Files:**
- Modify: `frontend/src/features/folders/AccountSection.tsx` (`FolderActions`, `FolderRow`, menú contextual)
- Modify: `frontend/src/lib/folders.ts` (`FolderNode` ya extiende `Folder`; los padres sintetizados de `buildFolderTree` llevan `color: ""`)
- Test: `frontend/src/features/folders/FolderSidebar.test.tsx`, `frontend/src/lib/folders.test.ts`

**Interfaces:**
- Consumes: `svc.SetFolderColor`, `Folder.color` (Task 3), `LABEL_COLORS`.
- Produces: `FolderNode.color` (heredado de `Folder`; `""` en los sintetizados) y `FolderActions.onColor: (node: FolderNode, color: string) => void`.

- [ ] **Step 1: Tests que fallan:**
  - `folders.test.ts`: `buildFolderTree` copia `color` y un padre sintetizado tiene `color: ""`.
  - `FolderSidebar.test.tsx`: "el ícono de una carpeta con color usa ese color". La fila de Work con `color: "#2563eb"` tiene el ícono con `style.color` igual a ese valor.
  - `FolderSidebar.test.tsx`: "Color en el menú contextual guarda el color elegido". En el menú contextual de Work, entrar a "Color" y hacer clic en el botón con `aria-label` "#16a34a" → `SetFolderColor(id, "#16a34a")`. "Sin color" → `SetFolderColor(id, "")`.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/folders src/lib/folders.test.ts` → FAIL.
- [ ] **Step 3:** En el menú contextual, antes de la separación de Subir/Bajar, agregar `ContextMenuSub` "Color". Adentro va una grilla de 6 columnas con los `LABEL_COLORS`, como en `LabelsSection`: cada botón con `aria-label={c}` y `aria-pressed`. Debajo, el ítem "Sin color". `onColor` llama a `svc.SetFolderColor`, invalida `["folders", account.id]` y con error muestra `toast.error(errorMessage(e))`. El ícono, el de rol o `FolderIcon`, va con `style={{ color: node.color || undefined }}` y conserva `text-muted-foreground` cuando no hay color.
- [ ] **Step 4:** Mismo comando → PASS.
- [ ] **Step 5:** Commit `feat(carpetas): colorear carpetas desde el menú contextual`.

### Task 5: Extraer el árbol de carpetas reusable

Es un refactor sin cambio visible. Lo deja listo para el popover.

**Files:**
- Create: `frontend/src/features/folders/FolderTree.tsx` (se mueven `FolderRow`, `FolderActions`, `FolderZone`, `folderZoneAllowed`, `EXPAND_DELAY_MS`, `ROLE_ICON` y la lógica de `shift`, `dropFolder` y `onColor`)
- Modify: `frontend/src/features/folders/AccountSection.tsx`
- Test: los existentes en `FolderSidebar.test.tsx`, sin cambios

**Interfaces:**
- Produces (`FolderTree.tsx`):
  - `export const ROLE_ICON` y `export const EXPAND_DELAY_MS` (se reexporta `ROLE_ICON` desde `AccountSection` o se actualizan los imports).
  - `export function useFolderTree(account: Account, state: ShellState): { tree: FolderNode[]; list: Folder[]; query: UseQueryResult<Folder[]>; actions: FolderActions; dialogs: ReactNode; openCreate: () => void }`. `dialogs` son los `NameDialog`, `MoveDialog`, `DeleteDialog` y `EmptyTrashDialog` que el que llama dibuja donde quiera. `openCreate()` abre "Nueva carpeta…" en la raíz.
  - `export function FolderTreeRows({ tree, state, accountName, actions }: {...}): ReactNode`: las filas de primer nivel, `depth={1}`.

- [ ] **Step 1:** Mover el código sin cambiar comportamiento. `AccountSection` usa `useFolderTree`, dibuja `<FolderTreeRows>` en `CollapsibleContent` y `{dialogs}` donde hoy están los diálogos. "Nueva carpeta…" del menú de la cuenta llama a `openCreate()`.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/folders && npx tsc --noEmit && npx eslint src/features/folders` → PASS, sin tocar tests.
- [ ] **Step 3:** Commit `refactor(carpetas): árbol de carpetas reusable fuera de AccountSection`.

### Task 6: Sidebar plegable en MailShell

**Files:**
- Modify: `frontend/src/features/layout/MailShell.tsx`
- Modify: `frontend/src/main.tsx` (`TooltipProvider delayDuration={300}` envolviendo la app)
- Test: `frontend/src/features/layout/MailShell.test.tsx`

**Interfaces:**
- Produces:
  - `export type SidebarMode = { collapsed: boolean; toggle: () => void }`
  - La prop `sidebar` cambia a `(s: ShellState, mode: SidebarMode) => ReactNode`. Actualizar el llamador en `App.tsx`.
  - Clave de `localStorage`: `"pc:sidebar-collapsed"` (`"1"` = plegado), con lectura y escritura en try/catch.

- [ ] **Step 1: Tests que fallan** en `MailShell.test.tsx`:
  - "el botón Plegar sidebar pasa collapsed=true al slot y lo recuerda". El slot `sidebar` dibuja `String(mode.collapsed)`. Clic en el botón con `aria-label="Plegar sidebar"` → muestra `"true"` y `localStorage["pc:sidebar-collapsed"] === "1"`.
  - "arranca plegado si estaba guardado y Expandir sidebar lo despliega".
  - "plegado, Redactar es un botón de ícono con aria-label Redactar".
  - "mientras está plegado no guarda el layout exterior". Simular `onLayoutChanged` con el panel plegado y verificar que `pc:layout:<pane>` no cambia en `outer`. Si en jsdom no se puede disparar, probar la función de guardado extraída: `saveOuterLayout(pane, sizes, collapsed)` no escribe con `collapsed=true`.
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/layout/MailShell.test.tsx` → FAIL.
- [ ] **Step 3:** Implementar. El estado `collapsed` arranca de `"pc:sidebar-collapsed"` y es la fuente de verdad. El panel lleva `collapsible`, `collapsedSize="52px"` y `panelRef={usePanelRef()}` (de `react-resizable-panels`). `toggle` cambia el estado, lo guarda y llama a `collapse()` o `expand()`. Un `useLayoutEffect` al montar llama a `collapse()` si arranca plegado. `onResize` del panel sincroniza el estado cuando el usuario pliega o despliega arrastrando: lee `panelRef.current.isCollapsed()` y guarda el flag. `onLayoutChanged` del grupo exterior no guarda mientras está plegado.
  - Expandido: arriba, `Redactar` (como hoy, `flex-1`) y al lado un botón `ghost` `size="icon"` con `PanelLeftCloseIcon` y `aria-label="Plegar sidebar"`.
  - Plegado: una columna con un botón de ícono `PencilIcon` (`aria-label="Redactar"`, tooltip "Redactar") y abajo un botón `PanelLeftOpenIcon` (`aria-label="Expandir sidebar"`, tooltip "Expandir sidebar").
- [ ] **Step 4:** Mismo comando → PASS, y `npx tsc --noEmit`.
- [ ] **Step 5:** Commit `feat(sidebar): plegar el sidebar a una columna`.

### Task 7: Columna de íconos y popover de carpetas

**Files:**
- Create: `frontend/src/features/folders/SidebarRail.tsx`
- Create: `frontend/src/features/folders/SidebarRail.test.tsx`
- Modify: `frontend/src/features/folders/FolderSidebar.tsx` (prop `collapsed`: dibuja `SidebarRail` en lugar de `nav` y del pie, y mantiene los diálogos)
- Modify: `frontend/src/App.tsx` (pasa `mode.collapsed` a `FolderSidebar`)

**Interfaces:**
- Consumes: `useFolderTree`, `FolderTreeRows`, `EXPAND_DELAY_MS`, `ROLE_ICON` (Task 5), `SidebarMode` (Task 6), `hasMessageDrag`/`dragAccount` (`lib/dnd`).
- Produces: `export function SidebarRail({ accounts, state, unifiedRows, onOpenSettings }: { accounts: Account[]; state: ShellState; unifiedRows: UnifiedRole[]; onOpenSettings: () => void })`. `FolderSidebar` gana la prop `collapsed?: boolean`.

La columna se ordena así: vistas unificadas (si `unified`), separador, una burbuja por cuenta, separador, Bandeja de salida (solo si hay pendientes, en rojo si falló), Contactos y, al pie, Configuración. Cada ítem es un botón de ícono con `aria-label` igual a su nombre, un `Tooltip side="right"` y `aria-current` cuando está activo.

**Burbuja de cuenta:** un círculo `size-8` con `backgroundColor: account.color`, la inicial en blanco y un contador de no leídos de la Entrada (`99+` como tope) en la esquina. Tooltip: `"<nombre> · <email>"`. Al hacer clic se abre un `Popover` (`side="right" align="start"`, `w-64`, `max-h-[70vh]` con scroll). Adentro hay un encabezado con el nombre y `<FolderTreeRows>`, que recibe un `state` envuelto: su `selectView` llama al original y después cierra el popover. Los `dialogs` de `useFolderTree` se dibujan fuera de `PopoverContent`.

- [ ] **Step 1: Tests que fallan** en `SidebarRail.test.tsx`, con mocks como en `FolderSidebar.test.tsx` y dos cuentas:
  - "muestra una burbuja por cuenta con su nombre como etiqueta y el contador de no leídos".
  - "clic en la burbuja abre sus carpetas; elegir una selecciona la vista y cierra el popover" → `selectView({kind:"folder", folderId})` y el popover deja de estar en el documento.
  - "Renombrar desde el popover deja el diálogo abierto aunque el popover se cierre" (Review Focus 1).
  - "arrastrar correos de la cuenta sobre la burbuja abre el popover después de la demora" (Review Focus 4). Con timers falsos, `dragover` con `setMessageDrag` de esa cuenta y avanzar `EXPAND_DELAY_MS` → aparece la fila de Work. Soltar en Work llama a `MoveMessages`.
  - "las vistas unificadas solo aparecen con la bandeja unificada activa".
  - "Configuración llama a onOpenSettings".
- [ ] **Step 2:** `cd frontend && npx vitest run src/features/folders/SidebarRail.test.tsx` → FAIL.
- [ ] **Step 3:** Implementar `SidebarRail`. Arrastrar sobre la burbuja usa el mismo patrón de `armExpand` de `FolderRow`: un timer de `EXPAND_DELAY_MS` que se cancela en `dragleave` y `drop`. Abre el popover solo si `hasMessageDrag(dt) && dragAccount(dt) === account.id`. El `Popover` va con `modal={false}`.
- [ ] **Step 4:** En `FolderSidebar`, con `collapsed`, dibujar `<SidebarRail>` en lugar del `nav` y del pie (Agregar cuenta queda solo expandido). Reusar `unifiedRows`. `onOpenSettings` hace lo mismo que el botón Configuración. En `App.tsx`, pasar `collapsed={mode.collapsed}`.
- [ ] **Step 5:** Test en `FolderSidebar.test.tsx`: "con collapsed no muestra Agregar cuenta y sí la columna de íconos".
- [ ] **Step 6:** `cd frontend && npx vitest run && npx tsc --noEmit && npx eslint .` → PASS.
- [ ] **Step 7:** Commit `feat(sidebar): columna de íconos con carpetas por cuenta en un popover`.

### Task 8: Verificación final

- [ ] **Step 1:** `cd /home/fabricio/desarrollo_local/postacontrol && task check` → todo verde.
- [ ] **Step 2:** Correr la app (`wails3 dev`) y probar a mano:
  - Plegar, reiniciar (vuelve plegado), desplegar (vuelve el ancho anterior).
  - Popover y diálogo de renombrar.
  - Colorear Entrada y una subcarpeta y después Actualizar la cuenta.
  - Tema Oscuro, Claro y Sistema, con el redactor abierto en otra ventana.
- [ ] **Step 3:** Commit del spec y del plan si todavía no están: `docs: diseño y plan de sidebar colapsable, colores y tema`.

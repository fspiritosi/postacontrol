# Fase 8: cuentas, enlaces de correo y corrector. Plan de implementación

> **Para agentes:** sub-skill obligatoria: superpowers:subagent-driven-development (recomendada) o superpowers:executing-plans, para implementar el plan tarea por tarea. Los pasos usan casillas (`- [ ]`) para el seguimiento.

**Objetivo:** que las cuentas se puedan editar y ordenar, que los `mailto:` de los correos y del sistema abran el redactor de PostaControl (que pasa a ser el cliente predeterminado), y que el redactor corrija la ortografía en es_AR y en_US.

**Arquitectura:**
- **Corrector:** se prende en el `WebKitWebContext` por defecto, con cgo, en `app/`.
- **Cuentas:**
  - la edición vive en el núcleo como `UpdateAccount`: prueba la conexión, mueve el secreto en el llavero y, si cambia el buzón, borra la caché de la cuenta;
  - el orden es una columna nueva (`accounts.position`, esquema v10).
- **Enlaces `mailto:`:** todos pasan por Go (`MailService.OpenMailto`). Arma el JSON de arranque del redactor y abre la ventana con `ComposeWindows.Open`, sin pasar por la ventana principal.

**Stack:** Wails v3.0.0 (GTK4, WebKitGTK 6.0 2.52.6), Go 1.27, modernc sqlite, React 19, TanStack Query, shadcn/radix, Vitest.

**Diseño:** `docs/superpowers/specs/2026-11-02-cuentas-enlaces-corrector-design.md`. Leelo antes de cada tarea: manda sobre el plan.

## Restricciones globales

- **Núcleo:** `core/` nunca importa Wails; WebKit/GTK e instancia única viven en `app/` o en `main.go`.
- **Credenciales:**
  - la contraseña nunca va a un log;
  - solo se guarda en el llavero (`c.cfg.Secrets`), con la dirección de correo como clave;
  - el TLS siempre se verifica (`c.cfg.TLS`).
- **Servidor:**
  - nada escribe en el servidor: editar, reordenar y `mailto:` no hacen escrituras IMAP/SMTP;
  - las pruebas de conexión solo hacen LOGIN;
  - nunca `EXPUNGE` sin UID.
- **Tests:** nunca contra cuentas reales; se usa `core/testenv` (los tests del núcleo arrancan con `newTestCore(t)` de `core/mail/helpers_test.go`).
- **Copy y formato:** en español rioplatense con voseo; sin prettier.
- **Bindings:** después de cambiar métodos de `MailService`, `wails3 generate bindings -clean=true -ts -i`.
- **`mailto:`:** solo se toman `to`, `cc`, `bcc`, `subject` y `body`; `attach`/`attachment` y todo lo demás se ignora. Un `mailto:` nunca envía solo.
- **`OpenExternal`:** acepta solo `http` y `https`.
- **Verificación:** `task check` en verde al cerrar cada tarea (vet/tests de Go y tsc/vitest/eslint). `go build ./...` falla por `build/ios` desde antes: usá `go build . ./app/... ./core/...`.
- **No tocar:** no leas ni imprimas `~/.npmrc`.

## Foco de la revisión

1. **`mailto:` con codificación y varios destinatarios:** `mailto:a@x.com,b@y.com?cc=c@z.com&subject=Hola%20qu%C3%A9%20tal&body=l1%0D%0Al2%3Cb%3E` → Para = a y b, Cc = c, asunto "Hola qué tal", cuerpo `l1<br>l2&lt;b&gt;`. Test en la Task 6.
2. **Cambiar solo la dirección de una cuenta:** la contraseña queda en el llavero bajo la dirección nueva y la vieja se borra. Si falla el guardado en la base, el llavero queda como estaba. Test en la Task 2.
3. **Sincronización en curso al cambiar de buzón:** una sincronización que ya había empezado contra el buzón viejo no puede volver a escribir carpetas ni mensajes después del borrado de la caché. Test en la Task 2 (época de la cuenta).
4. **Lista de orden vieja:** si se agregó o borró una cuenta mientras la lista estaba en pantalla, `ReorderAccounts` falla sin cambiar nada y la interfaz vuelve a pedir las cuentas. Tests en las Tasks 4 y 5.
5. **Texto plano:** la puntuación final queda afuera del enlace (`https://x.com/a.` → `https://x.com/a`) y una dirección de correo dentro de una URL no se enlaza dos veces. Test en la Task 7.

---

### Task 1: Corrector ortográfico

**Archivos:**
- Crear: `app/spellcheck_linux.go` (cgo, `//go:build linux`), `app/spellcheck_other.go` (`//go:build !linux`, sin efecto).
- Modificar: `core/model/model.go` (`Settings`), `app/mailservice.go` (`SetSettings`), `main.go`, `frontend/src/features/settings/SettingsDialog.tsx`.
- Modificar: los campos que no se revisan:
  - `frontend/src/features/compose/RecipientInput.tsx` (el `<input>` de la línea 185 aprox.);
  - los inputs de servidores, usuario y contraseña de `frontend/src/features/accounts/AccountSetupDialog.tsx`.
- Tests: `frontend/src/features/settings/SettingsDialog.test.tsx`, `app/spellcheck_test.go` (solo la lógica pura).

**Interfaces:**
- **Produce:**
  - `model.Settings.DisableSpellcheck bool` (json `disableSpellcheck`; cero = prendido);
  - `func applySpellcheck(enabled bool) []string` en `app/`: devuelve los idiomas que WebKit cargó, y nil cuando está apagado o fuera de Linux;
  - `var spellLanguages = []string{"es_AR", "en_US"}`.

- [ ] **Paso 1: tests que fallan.**
  - **`SettingsDialog.test.tsx`:**
    - un `Switch` con label "Corregir ortografía al escribir" aparece prendido con `disableSpellcheck: false`;
    - al apagarlo se guarda `{ disableSpellcheck: true }` por el mismo `saveSettings` que usa `setNotifications`.
  - **`RecipientInput.test.tsx`:** el input tiene `spellcheck="false"`.
- [ ] **Paso 2:** `cd frontend && npx vitest run src/features/settings src/features/compose/RecipientInput` → FALLA.
- [ ] **Paso 3: `app/spellcheck_linux.go`.**
  - Usa `#cgo pkg-config: webkitgtk-6.0` y `webkit_web_context_get_default()`.
  - Arma un `const gchar*[]` con NULL al final, a partir de `spellLanguages`.
  - Llama a `webkit_web_context_set_spell_checking_languages` y después a `webkit_web_context_set_spell_checking_enabled`.
  - Devuelve lo que diga `webkit_web_context_get_spell_checking_languages`.
  - Se llama siempre dentro de `application.InvokeSync` (hilo de GTK).
  - Si prendido y devuelve menos idiomas que `spellLanguages`: `slog.Warn("corrector: faltan diccionarios", "cargados", got)`; si no, `slog.Info("corrector", "idiomas", got)`.
- [ ] **Paso 4: aplicarlo.**
  - **`main.go`:** `wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, …)` lee la configuración con un método nuevo `mailService.SpellcheckEnabled() bool` (true si falla la lectura) y llama a `applySpellcheck` dentro de `application.InvokeAsync`. Exportá una función de `app/` (`app.ApplySpellcheck(enabled bool)`) para no exponer cgo en `main`.
  - **`MailService.SetSettings`:** si `s.core.SetSettings` salió bien, llama a `ApplySpellcheck(!st.DisableSpellcheck)` vía `application.InvokeAsync`, a través de un campo `spellcheck func(bool)`. En `NewMailService` queda `ApplySpellcheck`; en tests se reemplaza.
- [ ] **Paso 5: el interruptor en `SettingsDialog`**, junto al de notificaciones.
  - Label "Corregir ortografía al escribir".
  - Descripción "Marca las palabras mal escritas en español y en inglés."
  - Agregá `spellcheck="false"` donde dice "Archivos".
- [ ] **Paso 6: test de Go en `app/spellcheck_test.go`.** `SetSettings` llama al `spellcheck` falso con `false` cuando `DisableSpellcheck: true`, y no lo llama si el núcleo devuelve error. Si `MailService` no se puede armar con un núcleo de prueba sin Wails, sacá a una función `afterSettingsSaved(st model.Settings, apply func(bool))` lo que decide y testeá esa.
- [ ] **Paso 7: verificar en la app real.**
  - Corré `task build`.
  - Lanzá `./bin/postacontrol` durante unos 8 segundos (`timeout 8 ./bin/postacontrol 2>&1 | grep corrector`). Si hay otra instancia abierta, la instancia única hace que esta salga enseguida: en ese caso probá con `XDG_RUNTIME_DIR` apuntando a un directorio temporal, o anotá que no se pudo.
  - Lo esperado es `corrector idiomas="[es_AR en_US]"`. Anotá la salida en el informe.
- [ ] **Paso 8:** `task check` en verde y commit `feat(corrector): ortografía en español e inglés en el redactor`.

### Task 2: `UpdateAccount` en el núcleo

**Archivos:**
- Modificar: `core/store/accounts.go`, `core/mail/accounts.go`, `core/mail/sync.go` (época), `core/mail/core.go` (campo de épocas).
- Tests: `core/mail/accounts_test.go`, `core/store/accounts_test.go` (si existe; si no, crearlo).

**Interfaces:**
- **Consume:**
  - `TestAccount`, `parseEmail`, `c.cfg.Secrets` (`Get`/`Set`/`Delete`);
  - `stopWatcher`, `startWatcher`, `dropConn`, `clearAuthFailure`, `spawn`, `SyncAccount`.
- **Produce:**
  - `func (c *Core) UpdateAccount(ctx context.Context, id int64, in model.AccountInput) (model.Account, error)`;
  - `func (s *Store) UpdateAccount(ctx context.Context, a model.Account) error`: actualiza todas las columnas de `accountCols` menos `id`; `DefaultSignatureID` 0 se guarda como NULL, como en `SetAccountSignature`;
  - `func (s *Store) ResetAccountCache(ctx context.Context, accountID int64) error`: en una transacción borra las carpetas de la cuenta (los mensajes, el índice y `message_labels` caen por cascada y triggers), las filas de `label_cleanup` y `label_carry` de la cuenta, y deja `labels_local = 0`.

**Reglas** (diseño §1 y "Decisiones tomadas en la revisión"):

- **Valores por defecto:**
  - la dirección se valida con `parseEmail` y no puede repetir la de otra cuenta: `UserError{Code:"duplicate", Msg:"Esa cuenta ya está configurada."}`;
  - un nombre vacío se completa como en `AddAccount`;
  - un usuario vacío pasa a ser la dirección nueva.
- **Color:** tiene que cumplir `^#[0-9a-fA-F]{6}$`. Si no: `UserError{Code:"invalid", Msg:"El color no es válido."}`.
- **Cambio de conexión:** el usuario efectivo, `IMAP` o `SMTP` son distintos de lo guardado (host sin distinguir mayúsculas), o `in.Password != ""`.
  - En ese caso se prueba `TestAccount` con `in.Password` o, si vino vacía, con `Secrets.Get(email viejo)`.
  - Si `Get` falla: `UserError{Code:"invalid", Msg:"Ingresá la contraseña."}`.
- **Cambio de buzón:** cambió el host IMAP (sin distinguir mayúsculas) o el usuario efectivo.
- **Orden de los pasos:**
  1. validar;
  2. probar (si hace falta);
  3. si cambia la dirección o hay contraseña nueva: `Secrets.Set(email nuevo, pw)`;
  4. `store.UpdateAccount`. Si falla, se revierte el paso 3: con dirección nueva, `Delete(nuevo)`; con la misma dirección, `Set(viejo, pw viejo)` si se había leído;
  5. si cambió la dirección: `Delete(viejo)`, ignorando `secrets.ErrNotFound`;
  6. si cambia el buzón: `stopWatcher`; subir la época de la cuenta; `dropConn`; `store.ResetAccountCache`; emitir `EventFoldersUpdated {"accountId": id}`;
  7. si cambió la conexión, sea o no otro buzón: `dropConn`, `clearAuthFailure`, `startWatcher` (después de un `stopWatcher` si no se hizo antes) y, si `!c.cfg.SkipInitialSync`, `spawn(SyncAccount)`;
  8. emitir `EventAccountsChanged` y devolver la cuenta guardada.
- **Época por cuenta:**
  - es un `map[int64]uint64` con mutex en `Core`;
  - `SyncAccount` la lee al empezar;
  - antes de cada escritura de carpetas (`ReplaceFolders`) y de cada tanda de mensajes de una carpeta, si la época cambió, corta con un error interno `errAccountReset`, que no se informa al usuario (no emite `EventSyncError`);
  - ubicá esos puntos en `sync.go`.

- [ ] **Paso 1: tests que fallan** en `core/mail/accounts_test.go`, todos con `newTestCore` y una cuenta agregada con `AddAccount` contra los servidores de testenv:
  - `TestUpdateAccount_NameAndColor`: con `DisplayName:"Fabri"`, `Color:"#dc2626"` y el resto igual, `ListAccounts` devuelve los valores nuevos, la caché (carpetas) sigue y se emite `EventAccountsChanged`.
  - `TestUpdateAccount_BadColor`: `Color:"rojo"` da `UserError` "El color no es válido." y no cambia nada.
  - `TestUpdateAccount_DuplicateEmail`: con dos cuentas, poner en la segunda la dirección de la primera da "Esa cuenta ya está configurada.".
  - `TestUpdateAccount_WrongPasswordChangesNothing`: con `Password:"mala"`, la prueba falla, la cuenta y el secreto quedan como estaban y no hay `EventAccountsChanged`.
  - `TestUpdateAccount_EmailMovesSecret`: al cambiar solo la dirección (el usuario se manda explícito, igual al de antes), el secreto existe bajo la nueva y no bajo la vieja, y la caché no se borró.
  - `TestUpdateAccount_StoreFailureRestoresSecret`: el guardado en la base falla porque la dirección nueva viola UNIQUE, insertada a mano en la base entre la validación y el guardado; si no se puede, con un hook de test en `Core`. El llavero queda como antes.
  - `TestUpdateAccount_MailboxChangeResetsCache`: al cambiar el usuario (testenv acepta otro usuario con la misma contraseña; si no, agregá en testenv una segunda credencial), se emite `EventFoldersUpdated`, las carpetas viejas desaparecen, `labels_local` queda en 0 y, después de sincronizar, vuelven las carpetas del servidor.
  - `TestUpdateAccount_SignatureAndSaveSent`: `DefaultSignatureID` y `SaveSentCopy` se guardan.
  - `TestSyncAccount_StaleEpochWritesNothing`: se sube la época a mitad de la sincronización (hook en testenv o en `Core` antes de `ReplaceFolders`) y no queda ninguna carpeta escrita.
- [ ] **Paso 2:** `go test ./core/... -run 'UpdateAccount|StaleEpoch'` → FALLA.
- [ ] **Paso 3:** implementar `Store.UpdateAccount`, `Store.ResetAccountCache`, la época y `Core.UpdateAccount` según las reglas.
- [ ] **Paso 4:** `go test ./core/...` → PASA.
- [ ] **Paso 5:** `task check` y commit `feat(cuentas): editar una cuenta en el núcleo`.

### Task 3: Diálogo "Editar cuenta"

**Archivos:**
- Crear: `frontend/src/features/accounts/AccountEditDialog.tsx` y su test.
- Modificar:
  - `app/mailservice.go`: `UpdateAccount(ctx, id int64, in model.AccountInput) (model.Account, error)`, que delega;
  - `frontend/src/features/accounts/AccountSetupDialog.tsx`: exportá su fieldset de servidor (la función que termina en la línea ~99) para reutilizarlo;
  - `frontend/src/features/folders/AccountSection.tsx`: ítem "Editar cuenta…" en el menú de la cuenta, antes de "Cambiar contraseña";
  - los bindings (regenerar).

**Interfaces:**
- **Consume:** `UpdateAccount` (Task 2); `LABEL_COLORS` de `@/lib/labels`; `useSignatures()`.
- **Produce:** `export function AccountEditDialog({ account, open, onOpenChange }: { account: Account | null; open: boolean; onOpenChange(open: boolean): void })`.

**Comportamiento:**
- **Campos:**
  - nombre para mostrar, dirección, usuario y contraseña (vacía = conservar), con el texto de ayuda "Dejala vacía para conservar la actual.";
  - IMAP y SMTP con el fieldset reutilizado;
  - color: botones redondos con `LABEL_COLORS`. Si `account.color` no está en la paleta, se agrega primero a la fila y aparece elegido. `aria-label` = el código del color y `aria-pressed`;
  - firma predeterminada (Select con "Sin firma" y las firmas);
  - "Guardar copia en Enviados" (Switch).
- **Aviso de cambio de buzón:** si el host IMAP (sin distinguir mayúsculas) o el usuario difieren de los originales, debajo de los servidores se muestra: "Cambiaste el servidor o el usuario: se van a volver a bajar los correos de esta cuenta. En el servidor no se borra nada."
- **Guardar:** botón "Guardar". Mientras guarda, "Probando…" si cambió algo de conexión y "Guardando…" si no.
  - Si sale bien: invalida `["accounts"]` y `["folders", id]`, muestra el toast "Cuenta guardada." y cierra.
  - Si falla: el error queda en el diálogo (`errorMessage`) y no cierra.
- **Campos sin corrector:** dirección, usuario, contraseña y hosts llevan `spellcheck="false"`.

- [ ] **Paso 1: tests que fallan** (`AccountEditDialog.test.tsx`, con el mock de `svc` que usan los tests de `AccountSetupDialog`):
  - abre con los datos de la cuenta cargados y la contraseña vacía;
  - cambiar el nombre y guardar llama a `UpdateAccount(id, {...displayName:"Nuevo", password:""})`;
  - elegir `#dc2626` lo manda en `color`;
  - con una cuenta `#9333ea`, ese botón aparece con `aria-pressed="true"`;
  - cambiar el host IMAP muestra el aviso de cambio de buzón;
  - un rechazo muestra el mensaje y el diálogo sigue abierto.
  - En el test de `AccountSection`: el menú tiene "Editar cuenta…" y abre el diálogo.
- [ ] **Paso 2:** vitest de esos archivos → FALLA.
- [ ] **Paso 3:** método del servicio, bindings, componente y entrada en el menú.
- [ ] **Paso 4:** vitest → PASA; `task check` en verde.
- [ ] **Paso 5:** commit `feat(cuentas): diálogo para editar una cuenta`.

### Task 4: Orden de las cuentas en el núcleo

**Archivos:**
- Crear: `core/store/schema_v10.sql`.
- Modificar: `core/store/migrate.go` (embed y lista), `core/store/accounts.go`, `core/mail/accounts.go`, `app/mailservice.go`, los bindings.
- Tests: `core/store/migrate_test.go` (o donde estén los tests de migración), `core/mail/accounts_test.go`.

**Interfaces:**
- **Produce:**
  - `func (s *Store) SetAccountPositions(ctx context.Context, ids []int64) error`;
  - `func (c *Core) ReorderAccounts(ctx context.Context, ids []int64) error`;
  - `MailService.ReorderAccounts(ctx, ids []int64) error`.

**Esquema v10:**

```sql
ALTER TABLE accounts ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
UPDATE accounts SET position = id;
```

- **Alta:** `InsertAccount` pone `position = COALESCE(MAX(position),0)+1`.
- **Lectura:** `ListAccounts` usa `ORDER BY position, id`.
- **Reordenar:** `ReorderAccounts` exige que `ids` sea una permutación exacta de las cuentas existentes, sin repetidos ni faltantes. Si no, devuelve `UserError{Code:"invalid", Msg:"La lista de cuentas cambió. Probá de nuevo."}` sin escribir. Si está bien, escribe las posiciones 1..n en una transacción y emite `EventAccountsChanged`.

- [ ] **Paso 1: tests que fallan.**
  - **Migración:** una base en v9 con cuentas de id 1 y 2 migra a v10 con posiciones 1 y 2.
  - **`TestReorderAccounts`:** tres cuentas; `ReorderAccounts([3,1,2])` hace que `ListAccounts` devuelva 3,1,2; una cuenta nueva va al final.
  - **`TestReorderAccounts_Stale`:** `[1,2]` con tres cuentas, `[1,1,2]` y `[1,2,99]` dan el `UserError` y el orden no cambia.
- [ ] **Paso 2:** `go test ./core/...` → FALLA.
- [ ] **Paso 3:** implementar.
- [ ] **Paso 4:** `go test ./core/...` → PASA.
- [ ] **Paso 5:** `task check` y commit `feat(cuentas): orden de las cuentas en el núcleo`.

### Task 5: Sección "Cuentas" en Configuración

**Archivos:**
- Crear: `frontend/src/features/settings/AccountsSection.tsx` y su test.
- Modificar: `SettingsDialog.tsx`, para ponerla antes de `<SignaturesSection />`.

**Interfaces:**
- **Consume:** `ReorderAccounts` (Task 4), `AccountEditDialog` (Task 3), `useAccounts()`.

**Comportamiento:**
- **Fila por cuenta:** punto de color, nombre y dirección, y los botones:
  - "Subir {nombre}": `aria-label`, deshabilitado en la primera;
  - "Bajar {nombre}": deshabilitado en la última;
  - "Editar".

  Copiá el patrón de `LabelsSection.tsx` (líneas 113-122 y 184).
- **Al mover:** llama a `ReorderAccounts` con la lista completa en el orden nuevo y después invalida `["accounts"]`. Si falla, `toast.error(errorMessage(e))` y también invalida `["accounts"]`.
- **Editar:** abre `AccountEditDialog`.
- **Título:** "Cuentas", con la descripción "El orden se usa en la barra lateral y al elegir el remitente."

- [ ] **Paso 1: tests que fallan.**
  - Con tres cuentas, "Bajar A" llama a `ReorderAccounts([B, A, C])` (ids).
  - "Subir" en la primera está deshabilitado.
  - Un rechazo muestra el toast y vuelve a pedir las cuentas.
  - "Editar" abre el diálogo con esa cuenta.
- [ ] **Paso 2:** vitest → FALLA.
- [ ] **Paso 3:** implementar.
- [ ] **Paso 4:** vitest → PASA, `task check`.
- [ ] **Paso 5:** commit `feat(cuentas): sección Cuentas para ordenar y editar`.

### Task 6: `OpenMailto` y `mailto:` en el lector

**Archivos:**
- Crear: `app/mailto.go` y `app/mailto_test.go`.
- Modificar:
  - `app/mailservice.go`: `OpenMailto`, y `validateExternalURL` sin `mailto`;
  - `app/mailservice_test.go`: el test de `validateExternalURL`, si existe;
  - `frontend/src/features/reader/SafeHtmlFrame.tsx` y su test: `handleFrameClick` recibe la cuenta;
  - `frontend/src/features/reader/MessageView.tsx`: pasa `message.accountId`;
  - bindings.

**Interfaces:**
- **Consume:** `ListAccounts` (ordenadas, Task 4) y `ComposeWindows.Open(initJSON, key string) (int, error)`.
- **Produce:**
  - `type mailtoFields struct{ To, Cc, Bcc []model.Address; Subject, Body string }`;
  - `func parseMailto(link string) (mailtoFields, error)`: error si el esquema no es `mailto` (sin distinguir mayúsculas) o si `len(link) > 8192`;
  - `func mailtoInitJSON(f mailtoFields, acc model.Account) (string, error)`;
  - `func pickMailtoAccount(accs []model.Account, id int64) (model.Account, bool)`: la de ese id, o la primera; false si no hay cuentas;
  - `func (s *MailService) OpenMailto(ctx context.Context, link string, accountID int64) error`;
  - `SafeHtmlFrame` gana la prop `accountId: number`;
  - `export function handleFrameClick(ev: Event, accountId: number): void`.

**Reglas:**
- **Interpretación:**
  - `url.Parse`; los destinatarios del path (`u.Opaque`, decodificado con `url.PathUnescape`) se separan por coma;
  - `to` de la consulta se suma a Para; `cc`/`bcc` pueden repetirse o venir con comas;
  - cada dirección pasa por `net/mail.ParseAddress` y las inválidas se descartan;
  - `subject` y `body` se toman tal cual, decodificados.
- **Cuerpo:** `html.EscapeString(body)`, y después `\r\n` / `\n` pasan a `<br>`.
- **JSON:** un struct de Go con las claves exactas de `ComposerInit` (`frontend/src/features/compose/ComposerContext.tsx:20-35`):
  - `accountId`, `to`, `cc`, `bcc`, `subject`, `bodyHtml`, `quotedHtml:""`, `reply:null`, `attachments:[]`, `originalAttachments:[]`, `skipOriginalAttachments:[]`, `signatureId: acc.DefaultSignatureID`;
  - las listas nunca son `null`;
  - las direcciones usan el JSON de `model.Address`.
- **`OpenMailto`:** parsea, elige la cuenta (si no hay: `mail.UserError{Code:"invalid", Msg:"Primero agregá una cuenta."}`) y abre con `s.composes.Open(json, "")`.
- **Frontend:** en `handleFrameClick`, si `href` empieza con `mailto:` (sin distinguir mayúsculas), llama a `svc.OpenMailto(href, accountId)`; si no, a `OpenExternal`, como hoy. Los errores van a `toast.error`.

- [ ] **Paso 1: tests que fallan** en `app/mailto_test.go`:
  - `TestParseMailto_Full`: el caso 1 del foco de la revisión.
  - `TestParseMailto_IgnoresAttach`: `mailto:a@x.com?attach=/etc/passwd&attachment=x&in-reply-to=y` → solo Para; nada más en los campos.
  - `TestParseMailto_Rejects`: `https://x.com`, `javascript:alert(1)` y un `mailto:` de 9000 bytes dan error.
  - `TestParseMailto_EmptyIsOK`: `mailto:` da campos vacíos y ningún error.
  - `TestMailtoInitJSON_Keys`: decodificado a `map[string]any`, tiene exactamente las 12 claves de arriba; las listas son arreglos (no null), `reply` es null y `signatureId` es la firma de la cuenta.
  - `TestPickMailtoAccount`: id existente → esa cuenta; 0 o inexistente → la primera; lista vacía → false.
  - `validateExternalURL("mailto:a@x.com")` ahora da error.
  - **Frontend:** `handleFrameClick` sobre `<a href="mailto:a@x.com">` llama a `OpenMailto("mailto:a@x.com", 7)` y no a `OpenExternal`.
- [ ] **Paso 2:** `go test ./app/ -run 'Mailto|ExternalURL'` y vitest de `SafeHtmlFrame` → FALLA.
- [ ] **Paso 3:** implementar y regenerar bindings.
- [ ] **Paso 4:** los tests → PASAN, `task check`.
- [ ] **Paso 5:** commit `feat(enlaces): mailto abre el redactor de PostaControl`.

### Task 7: Enlaces en correos de texto plano

**Archivos:**
- Crear: `frontend/src/lib/linkify.ts` y `linkify.test.ts`.
- Modificar: `frontend/src/features/reader/SafeHtmlFrame.tsx` (rama `!html`) y su test.

**Interfaces:**
- **Produce:** `export type TextPart = { kind: "text"; text: string } | { kind: "link"; text: string; href: string }` y `export function linkify(text: string): TextPart[]`.

**Reglas:**
- Se detectan, en orden de aparición, `https?://…`, `www.…` (href `https://www.…`) y direcciones de correo (href `mailto:…`).
- **Puntuación final:** `.`, `,`, `;`, `:`, `!`, `?` y `)` del final no son parte del enlace. Un `)` se conserva si dentro del enlace hay un `(` sin cerrar.
- Una dirección de correo dentro de una URL ya detectada no se detecta aparte.
- **Render:** la rama de texto renderiza cada parte con React (`<a href>` para los enlaces, texto para el resto) y nunca usa `innerHTML`.
- **Clic:** `preventDefault`; si `href` es `mailto:`, `svc.OpenMailto(href, accountId)`; si no, `svc.OpenExternal(href)`.

- [ ] **Paso 1: tests que fallan** en `linkify.test.ts`:
  - `"ver https://x.com/a."` da `[text "ver ", link "https://x.com/a", text "."]`;
  - `"(www.x.com)"` da `[text "(", link "www.x.com" href "https://www.x.com", text ")"]`;
  - `"https://es.wikipedia.org/wiki/A_(b)"` es un solo enlace con el `)`;
  - `"escribí a ana@x.com, gracias"` da un enlace `mailto:ana@x.com`;
  - `"https://x.com/?u=ana@x.com"` es un solo enlace;
  - un texto sin enlaces da una sola parte de texto.
  - **SafeHtmlFrame:** con `html=""` y `text="ir a https://x.com"`, el clic en el enlace llama a `OpenExternal("https://x.com")`.
- [ ] **Paso 2:** vitest → FALLA.
- [ ] **Paso 3:** implementar.
- [ ] **Paso 4:** vitest → PASA, `task check`.
- [ ] **Paso 5:** commit `feat(enlaces): enlaces cliqueables en correos de texto plano`.

### Task 8: PostaControl como cliente de correo del sistema

**Archivos:**
- Modificar: `main.go`, `app/desktop.go` (`StartHidden`), `app/desktop_test.go`, `Taskfile.yml` (`install:desktop`) y `build/linux/postacontrol.desktop`.
- Crear: en `app/mailto.go`, `func MailtoArgs(args []string) []string`, y en `MailService`, `func (s *MailService) OpenMailtoArgs(args []string)`.

**Interfaces:**
- **Consume:** `OpenMailto(ctx, link, 0)` (Task 6).
- **Produce:**
  - `MailtoArgs` devuelve, en orden, los argumentos que empiezan con `mailto:` (sin distinguir mayúsculas) y miden hasta 8192 bytes;
  - `OpenMailtoArgs` llama a `OpenMailto(context.Background(), a, 0)` por cada uno y registra los errores con `slog.Warn("mailto: no se pudo abrir", "err", err)`, sin el enlace.

**Comportamiento:**
- **`StartHidden(args)`:** `true` solo si está `--hidden` y `MailtoArgs(args)` viene vacío.
- **`OnSecondInstanceLaunch`:** si `MailtoArgs(data.Args)` no está vacío, `go mailService.OpenMailtoArgs(...)` y no muestra la ventana principal. Si no, `desktop.ShowWindow()`, como hoy.
- **Arranque en frío:** en el mismo `ApplicationStarted` de la Task 1 (o en uno propio), `mailService.OpenMailtoArgs(os.Args[1:])`.
- **Instalación (`Taskfile.yml`, `install:desktop`):**
  - el `.desktop` gana `MimeType=x-scheme-handler/mailto;`;
  - `Exec="$HOME/.local/bin/{{.APP_NAME}}" %u`;
  - después de `update-desktop-database` corre `xdg-mime default ar.com.codecontrol.postacontrol.desktop x-scheme-handler/mailto`.
- **`build/linux/postacontrol.desktop`:** el mismo `MimeType` y `%u` en `Exec`.

- [ ] **Paso 1: tests que fallan.**
  - `TestMailtoArgs`: `["--hidden","MAILTO:a@x.com","x","mailto:"+9000 bytes]` da `["MAILTO:a@x.com"]`.
  - `TestStartHidden`: `["--hidden"]` da true; `["--hidden","mailto:a@x.com"]` da false; `[]` da false.
- [ ] **Paso 2:** `go test ./app/ -run 'MailtoArgs|StartHidden'` → FALLA.
- [ ] **Paso 3:** implementar.
- [ ] **Paso 4:** `go test ./app/` → PASA, `task check`.
- [ ] **Paso 5: verificar que llegan los argumentos.**
  - Leé en `$(go env GOMODCACHE)/github.com/wailsapp/wails/v3@v3.0.0/pkg/application/single_instance*.go` cómo se arma `SecondInstanceData.Args` en Linux y confirmá que trae los argumentos de la segunda instancia sin el ejecutable, o con él (`MailtoArgs` lo filtra igual).
  - Anotá el resultado en el informe.
  - No corras `xdg-mime default` en la máquina de Fabricio: lo hace `task install:desktop` cuando él instale.
- [ ] **Paso 6:** commit `feat(enlaces): PostaControl atiende los mailto del sistema`.

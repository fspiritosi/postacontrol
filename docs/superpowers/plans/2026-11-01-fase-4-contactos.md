# PostaControl — Fase 4 "Contactos" Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que Fabricio tenga una libreta de contactos con fichas amplias dentro de la pantalla principal, que Para/CC/CCO se completen solos con sus contactos y con la gente a la que ya les escribió, y que pueda agregar al remitente de un correo con un clic.

**Architecture:**
- **Datos:** en SQLite, que para contactos es la fuente de verdad (spec).
  - `contacts` guarda la ficha.
  - `contact_emails` guarda los correos de cada ficha; un correo pertenece a una sola ficha.
  - `recipients` lleva la cuenta de "a quiénes les escribí". Se alimenta de dos lados:
    - al enviar desde PostaControl (Para, CC y CCO);
    - con un trigger, cada vez que entra a la caché un correo de una carpeta con rol `sent`.
  - La migración rellena `recipients` con lo que ya está en la caché de Enviados.
  - Por qué hacen falta los dos lados: la sincronización de fondo solo trae INBOX, así que Enviados puede no estar nunca en la caché.
- **Núcleo:** `core/mail/contacts.go` tiene el ABM de fichas, la búsqueda, "agregar remitente" y las sugerencias. Compara sin mayúsculas ni acentos, así "pena" encuentra a "Peña".
- **Redactor:** Para/CC/CCO pasan a ser chips con una lista de sugerencias.
  - El estado del redactor **sigue siendo el mismo texto** ("Nombre <correo>, …"), así que envío, autoguardado y validación no cambian.
  - Lo que se está escribiendo vive en el campo y también forma parte de ese texto.
- **Libreta:** una vista nueva `{ kind: "contacts" }`, con una fila "Contactos" en el panel izquierdo. La lista con buscador va en el centro y la ficha a la derecha.

**Tech Stack:** el de las fases anteriores; `golang.org/x/text` (ya está en `go.mod`) para quitar acentos. En el frontend se suma solo el componente shadcn `textarea`.

**Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`, secciones "Contactos", "Pantalla principal" y "Fase 4". Fabricio decidió tres cosas el 2026-11-01:
- se sugiere solo a quienes les escribió (no a todos los remitentes);
- los destinatarios se muestran como chips;
- la libreta vive en los paneles.

## Global Constraints

- **Siguen vigentes** todas las de las fases anteriores:
  - `core/` sin Wails; tests solo con `core/testenv`.
  - Español rioplatense con voseo.
  - `task check` verde antes de cada commit.
  - Commits convencionales en español con `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Bindings con `task common:generate:bindings`.
- **Rama:** `fase-4`, desde `main`.
- **Esta fase no escribe nada en el servidor.** Los contactos y `recipients` son solo locales, y no se importa nada de Thunderbird.
- **Comparación de texto:** `fold(s)` pasa a minúsculas y quita marcas diacríticas (NFD, sin `Mn`). Correos: siempre sin distinguir mayúsculas.
- **Sugerencias:** como máximo `SUGGEST_LIMIT = 8`, desde 1 carácter escrito.
  - Primero los contactos, después el resto de `recipients`.
  - Dentro de cada grupo, primero las que empiezan con lo escrito: el correo, o cualquier palabra del nombre. Después las que solo lo contienen.
  - Desempate: contactos por nombre; `recipients` por `times` descendente y después por `last_used` descendente.
  - Un correo que ya es de un contacto no aparece dos veces.
  - No se sugieren las direcciones que ya están en ese mismo campo.
- **Textos exactos:**
  - Panel izquierdo: fila "Contactos".
  - Lista:
    - buscador con placeholder "Buscar contactos" y botón "Nuevo contacto";
    - sin contactos: "Todavía no tenés contactos.";
    - sin resultados: "Ningún contacto coincide con «<q>»."
  - Ficha sin elegir: "Elegí un contacto o creá uno nuevo."
  - Ficha:
    - campos "Nombre", "Correos", "Teléfonos", "Empresa", "Notas";
    - botones "Agregar correo", "Agregar teléfono", "Guardar", "Cancelar", "Editar", "Eliminar" y "Escribirle".
  - Al borrar, la confirmación dice "¿Eliminar a «<nombre>»?" y "Se borra de la libreta. Los correos no se tocan."
  - Lector:
    - botón "Agregar a contactos", o "Ver contacto" si el remitente ya está en la libreta;
    - al agregar, toast "«<nombre>» se agregó a tus contactos."
  - Errores del núcleo:
    - "Poné un nombre o un correo." (código `invalid`);
    - "«<x>» no es un correo válido." (código `invalid`);
    - "«<x>» ya está en la ficha de <nombre>." (código `duplicate`).
- **Nombre que se muestra** de un contacto: su nombre, o si no tiene, su primer correo.
- **Evento:** `mail:contacts-changed` (sin datos), emitido en cada alta, cambio o baja.

## Review Focus

1. **Un nombre con coma o comillas elegido de la lista** (`Peña, Fabricio`) queda como un solo chip y se envía bien. Test "elegir una sugerencia con coma en el nombre deja un chip válido" (Task 3).
2. **Pegar una lista** `a@x.com; b@y.com, c@z.com` en el campo deja tres chips. Test "pegar varias direcciones las convierte en chips" (Task 3).
3. **Acentos y mayúsculas:**
   - "pena" encuentra a "Peña" y "JOSE" encuentra a "José";
   - `Juan@X.com` es la misma persona que `juan@x.com`, así que no se puede repetir en otra ficha y el lector muestra "Ver contacto".
   - Tests: `TestSuggest_FoldsAccentsAndCase` y `TestSaveContact_EmailUniqueIgnoringCase` (Task 2); "con el remitente ya en la libreta muestra Ver contacto" (Task 5).
4. **Un borrador retomado** con destinatarios muestra todos como chips y no deja texto suelto en el campo. Test "un valor inicial se muestra entero como chips" (Task 3).
5. **El mismo correo de Enviados sincronizado otra vez** (resincronizar la carpeta) no suma dos veces en `recipients`. Test `TestRecipients_SentTriggerCountsOncePerMessage` (Task 1).

---

## File Structure

```
core/
  model/model.go             # Contact, Suggestion (Task 1)
  store/schema_v7.sql        # contacts, contact_emails, recipients + trigger + relleno (Task 1)
  store/migrate.go           # registra v7 (Task 1)
  store/contacts.go          # ABM de fichas y búsqueda por correo (Task 1)
  store/recipients.go        # RecordRecipients, ListRecipients (Task 1)
  mail/fold.go               # fold() (Task 2)
  mail/contacts.go           # API de contactos y sugerencias (Task 2)
  mail/compose.go            # deliver registra destinatarios (Task 2)
  mail/events.go             # EventContactsChanged (Task 2)
app/mailservice.go           # fachada Wails (Task 2)
frontend/src/
  lib/recipients.ts          # splitRecipientTokens exportado (Task 3)
  features/compose/RecipientInput.tsx   # chips + sugerencias (Task 3)
  features/compose/Composer.tsx         # Esc de la lista de sugerencias (Task 3)
  lib/queries.ts, lib/events.ts         # useContacts, useContact, useContactForEmail, useSuggestions; evento (Tasks 3-4)
  lib/view.ts                # View { kind: "contacts" } (Task 4)
  features/layout/MailShell.tsx         # contactId, selectContact, openContact (Task 4)
  features/folders/FolderSidebar.tsx    # fila "Contactos" (Task 4)
  features/contacts/ContactList.tsx     # lista con buscador (Task 4)
  features/contacts/ContactPanel.tsx    # ficha: ver, editar, borrar (Task 4)
  features/compose/ComposerContext.tsx  # openNew(accountId, to?) (Task 4)
  components/ui/textarea.tsx            # shadcn (Task 4)
  App.tsx                    # slots de la vista contacts (Task 4)
  features/reader/MessageView.tsx       # Agregar a contactos / Ver contacto (Task 5)
```

---

### Task 1: Esquema y store de contactos y destinatarios

**Files:**
- Create: `core/store/schema_v7.sql`, `core/store/contacts.go`, `core/store/recipients.go`, `core/store/contacts_test.go`
- Modify: `core/store/migrate.go`, `core/model/model.go`

**Interfaces:**
- Produces (model):
  ```go
  type Contact struct {
      ID      int64    `json:"id"`
      Name    string   `json:"name"`
      Emails  []string `json:"emails"`  // en orden; nunca nil
      Phones  []string `json:"phones"`  // nunca nil
      Company string   `json:"company"`
      Notes   string   `json:"notes"`
  }
  type Suggestion struct {
      Name      string `json:"name"`
      Email     string `json:"email"`
      ContactID int64  `json:"contactId"` // 0 = no es contacto
  }
  ```
- Produces (store):
  - `ListContacts(ctx) ([]model.Contact, error)`: todas las fichas, con sus correos en orden. El orden lo decide el núcleo.
  - `GetContact(ctx, id int64) (model.Contact, error)`: `ErrNotFound` si no está.
  - `SaveContact(ctx, c model.Contact) (model.Contact, error)`:
    - con ID 0 crea la ficha; si no, la actualiza (`ErrNotFound` si no existe);
    - reemplaza los correos en una transacción;
    - devuelve `ErrEmailTaken{Email, ContactID}` (tipo nuevo) si un correo ya es de otra ficha.
  - `DeleteContact(ctx, id int64) error`: sus correos se van con `ON DELETE CASCADE`.
  - `ContactIDForEmail(ctx, email string) (int64, error)`: `ErrNotFound` si ningún contacto lo tiene.
  - `RecordRecipients(ctx, addrs []model.Address, at time.Time) error`: suma 1 a `times` por dirección (dedup dentro de la llamada), pone `last_used = max(last_used, at)` y guarda el nombre si viene no vacío.
  - `ListRecipients(ctx) ([]Recipient, error)`, con `type Recipient struct{ Email, Name string; Times int; LastUsed int64 }`.

- [ ] **Step 1: Escribir `schema_v7.sql`**

```sql
CREATE TABLE contacts (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT NOT NULL DEFAULT '',
  phones_json TEXT NOT NULL DEFAULT '[]',
  company     TEXT NOT NULL DEFAULT '',
  notes       TEXT NOT NULL DEFAULT '',
  updated_at  INTEGER NOT NULL
);
-- Un correo pertenece a una sola ficha (sin distinguir mayúsculas).
CREATE TABLE contact_emails (
  contact_id INTEGER NOT NULL REFERENCES contacts(id) ON DELETE CASCADE,
  position   INTEGER NOT NULL,
  email      TEXT NOT NULL,
  PRIMARY KEY (contact_id, position)
);
CREATE UNIQUE INDEX idx_contact_emails_email ON contact_emails (email COLLATE NOCASE);

-- A quiénes les escribí: para sugerir destinatarios. email va en minúsculas.
CREATE TABLE recipients (
  email     TEXT PRIMARY KEY,
  name      TEXT NOT NULL DEFAULT '',
  times     INTEGER NOT NULL DEFAULT 0,
  last_used INTEGER NOT NULL DEFAULT 0
);
```

Agregar al mismo archivo:
- **El trigger** `recipients_from_sent`: `AFTER INSERT ON messages`, solo `WHEN (SELECT role FROM folders WHERE id = NEW.folder_id) = 'sent'`. Hace upsert de cada dirección de `json_each(NEW.to_json)` y `json_each(NEW.cc_json)`:
  - `email` en minúsculas;
  - `times + 1`;
  - `last_used = max(last_used, NEW.date)`;
  - `name` se pisa solo si el nuevo no está vacío.
  - Las direcciones sin `@` se ignoran.
- **El relleno** con un `INSERT … SELECT` equivalente sobre los mensajes que ya están en carpetas `sent`.

Antes de confiar en el trigger, confirmar en `core/store/messages.go` que `UpsertHeaders` no reinserta las filas existentes. Por ejemplo, un `INSERT OR REPLACE` dispararía el trigger de nuevo. Si las reinserta, el contador se filtra en el trigger con `NOT EXISTS` sobre `(folder_id, uid)`, y se anota la decisión en el registro.

- [ ] **Step 2: Tests que fallan** en `core/store/contacts_test.go`:
  - `TestContacts_CRUDKeepsEmailOrder`:
    - crea `{Name:"Ana", Emails:["b@x.com","a@x.com"], Phones:["+54 11 1"]}`;
    - `GetContact` devuelve los correos en ese orden;
    - después de actualizar con `Emails:["a@x.com"]`, queda uno solo;
    - `DeleteContact`, y luego `GetContact`, da `ErrNotFound`.
  - `TestContacts_EmailTakenIgnoringCase`: Ana tiene `juan@x.com`. Guardar otra ficha con `Juan@X.com` da `ErrEmailTaken{Email:"Juan@X.com", ContactID: ana.ID}` y no la crea.
  - `TestContacts_ContactIDForEmailIgnoresCase`.
  - `TestRecipients_RecordAccumulates`:
    - dos llamadas con `Ana <ANA@x.com>` dejan `times=2` y el email en minúsculas;
    - un nombre vacío no pisa "Ana";
    - `last_used` es el mayor.
  - `TestRecipients_SentTriggerCountsOncePerMessage`:
    - `seedFolder` más una carpeta `{Name:"Sent", Role:"sent"}` vía `ReplaceFolders`;
    - `UpsertHeaders` de un mensaje con `To:[{Name:"Bea", Email:"bea@y.com"}]` y `Cc:[{Email:"cc@y.com"}]`, dos veces con el mismo UID → `times=1` para cada uno;
    - un mensaje en INBOX no suma nada.
  - `TestMigrateV7_BackfillsSentRecipients`: con `migrateTo(db, 6)`, insertar un mensaje en una carpeta `sent`, migrar a 7 y ver la fila en `recipients`. Copiar el estilo de `plan_test.go`, o el test de migración que ya exista.

- [ ] **Step 3: Correr** `go test ./core/store/ -run 'Contacts|Recipients|MigrateV7'`. Esperado: FAIL, no compila.
- [ ] **Step 4: Implementar** `schema_v7.sql` en `migrations`, más `contacts.go` y `recipients.go`, siguiendo el estilo de `signatures.go`.
- [ ] **Step 5: Correr** `go test ./core/store/` → PASS, y `task check` → verde.
- [ ] **Step 6: Commit** `feat(store): contactos y destinatarios usados`.

---

### Task 2: API de contactos y sugerencias en el núcleo

**Files:**
- Create: `core/mail/fold.go`, `core/mail/contacts.go`, `core/mail/contacts_test.go`
- Modify: `core/mail/events.go`, `core/mail/compose.go` (en `deliver`), `app/mailservice.go`, y los bindings regenerados.

**Interfaces:**
- Consumes: todo lo que produce el store en la Task 1.
- Produces (core y fachada `MailService`, mismos nombres):
  - `ListContacts(ctx, query string) ([]model.Contact, error)`:
    - con `query` vacío devuelve todas las fichas;
    - si no, filtra las que tienen `fold(query)` contenido en `fold` de nombre, empresa o algún correo, o en algún teléfono sin espacios ni guiones;
    - ordena por `fold(nombre que se muestra)`.
  - `GetContact(ctx, id int64) (model.Contact, error)`: si no está, `UserError{Code:"not_found"}` con el mensaje genérico de no encontrado que ya existe.
  - `SaveContact(ctx, c model.Contact) (model.Contact, error)`. Normaliza, valida, guarda y emite `EventContactsChanged`. Normaliza así:
    - recorta los espacios de todo;
    - quita correos y teléfonos vacíos;
    - deduplica los correos sin distinguir mayúsculas, conservando el primero.
  - `DeleteContact(ctx, id int64) error`: emite el evento.
  - `ContactForEmail(ctx, email string) (*model.Contact, error)`: devuelve `nil, nil` si nadie tiene ese correo.
  - `AddSenderToContacts(ctx, addr model.Address) (model.Contact, error)`:
    - si el correo ya es de una ficha, la devuelve sin cambios y sin evento;
    - si no, crea `{Name: addr.Name recortado, Emails: [addr.Email]}` y emite el evento.
  - `SuggestRecipients(ctx, query string, exclude []string) ([]model.Suggestion, error)`: sigue las reglas de "Sugerencias" de las Global Constraints. `exclude` son los correos que ya están en el campo.
  - `const EventContactsChanged = "mail:contacts-changed"`.

- [ ] **Step 1: Tests que fallan** en `core/mail/contacts_test.go`. Usan `newTestCore`; los de sugerencias cargan `recipients` con `c.store.RecordRecipients`.
  - `TestSaveContact_ValidatesAndNormalizes`:
    - `{}` da `invalid` "Poné un nombre o un correo.";
    - `{Emails:["no-es-correo"]}` da `invalid` "«no-es-correo» no es un correo válido.";
    - `{Name:" Ana ", Emails:[" a@x.com ","A@X.com",""], Phones:[" ",""]}` se guarda como `{Name:"Ana", Emails:["a@x.com"], Phones:[]}`;
    - emite `mail:contacts-changed`.
  - `TestSaveContact_EmailUniqueIgnoringCase`: Ana tiene `juan@x.com`. Guardar `{Name:"Otro", Emails:["Juan@X.com"]}` da `duplicate` "«Juan@X.com» ya está en la ficha de Ana."
  - `TestListContacts_FiltersFoldedAndSorts`:
    - fichas "Ñandú", "ana", "José" (empresa "Acme") y una sin nombre con `zz@x.com`;
    - con `""` el orden es ana, José, Ñandú, zz@x.com, porque "Ñandú" se pliega a "nandu";
    - "acme" encuentra a José y "NAND" a Ñandú.
  - `TestAddSenderToContacts_CreatesOrReturnsExisting`: la segunda llamada con `Ana@X.com` devuelve la misma ficha y no emite el evento.
  - `TestContactForEmail_NilWhenMissing`.
  - `TestSuggest_FoldsAccentsAndCase`: el contacto "Peña" con `pp@x.com` aparece con "pena" y con "PEÑ".
  - `TestSuggest_OrderAndLimit`:
    - el contacto "Juan Pérez" va antes que el destinatario `juana@y.com`, aunque este tenga `times=50`;
    - entre destinatarios, el de más `times` va primero;
    - "juan" pone la coincidencia de prefijo antes que `x-juan@z.com`;
    - con 12 coincidencias devuelve 8.
  - `TestSuggest_SkipsContactDuplicatesAndExcluded`:
    - un correo que está en una ficha y también en `recipients` aparece una vez, con `ContactID`;
    - los de `exclude` no aparecen;
    - con `query` vacío devuelve una lista vacía.
  - `TestSend_RecordsRecipients`: un `Send` exitoso con To, CC y CCO deja a las tres direcciones con `times=1`. Tomar como modelo un test de envío existente de `compose_test.go`. Un envío que falla no registra nada.
- [ ] **Step 2: Correr** `go test ./core/mail/ -run 'Contact|Suggest|RecordsRecipients'` → FAIL.
- [ ] **Step 3: Implementar.**
  - `fold(s string) string` con `golang.org/x/text/unicode/norm` y `runes.Remove(runes.In(unicode.Mn))`.
  - La validación de correo usa `net/mail.ParseAddress`, y exige que la dirección resultante sea igual al texto recortado (rechaza `Ana <a@x.com>` como correo suelto).
  - `ErrEmailTaken` se traduce al `UserError` `duplicate` con el nombre que se muestra de la otra ficha.
  - En `deliver`, justo después de que `smtpSend` sale bien, llamar a `RecordRecipients` con To, CC y CCO de `req`. Usar `context.WithoutCancel`, como las otras limpiezas, e ignorar el error.
- [ ] **Step 4: Fachada y bindings.**
  - Agregar los siete métodos a `app/mailservice.go`, delegando como `ListSignatures`.
  - Correr `task common:generate:bindings`.
  - El reenvío del evento a la UI tiene que cubrir `mail:contacts-changed`. Mirar cómo pasa `mail:signatures-changed`.
- [ ] **Step 5: Correr** `go test -race ./core/... ./app/` y `task check` → verde.
- [ ] **Step 6: Commit** `feat(mail): contactos y sugerencias de destinatarios`.

---

### Task 3: Chips con sugerencias en Para/CC/CCO

**Files:**
- Modify: `frontend/src/lib/recipients.ts`, `frontend/src/features/compose/RecipientInput.tsx`, `frontend/src/features/compose/Composer.tsx`, `frontend/src/lib/queries.ts`, `frontend/src/lib/events.ts`
- Test: `frontend/src/features/compose/RecipientInput.test.tsx` (nuevo); `Composer.test.tsx` sigue pasando.

**Interfaces:**
- Consumes: `svc.SuggestRecipients(query, exclude)` y el evento `mail:contacts-changed`.
- Produces:
  - `export function splitRecipientTokens(input: string): string[]`, que es el `splitTokens` actual exportado.
  - `useSuggestions(query: string, exclude: string[])` en `queries.ts`:
    - clave `["suggest", query, exclude]`;
    - `enabled: query.trim() !== ""`;
    - `placeholderData: keepPreviousData`.
  - En `events.ts`, `mail:contacts-changed` invalida `["contacts"]`, `["contact"]`, `["contact-for"]` y `["suggest"]`.
  - `RecipientInput` conserva `{ id, label, value, onChange }` y suma `onSuggestionsChange?: (close: (() => void) | null) => void`, el mismo patrón que `onSlashMenuChange`.

**Modelo del componente.** El `value` es el texto completo, igual que hoy.
- **Estado local:** `pending`, que es el texto del campo.
- **Chips:** son `splitRecipientTokens(value)` menos el último token, si ese token es igual a `pending.trim()` y `pending` no está vacío.
- **Cada cambio** emite `[...chips, pending].filter(Boolean).join(", ")`.
- **Al escribir un separador** (`,`, `;` o un salto de línea, fuera de comillas y de `<…>`), o al pegar texto con separadores:
  - los tokens completos pasan a ser chips;
  - lo que sigue al último separador queda en `pending`.
- **Enter o Tab** con la lista abierta eligen la sugerencia activa. Con la lista cerrada y `pending` no vacío, convierten `pending` en chip. Tab sin `pending` deja pasar el foco.
- **Al perder el foco,** `pending` pasa a ser chip.
- **Backspace** con `pending` vacío quita el último chip.
- **Elegir una sugerencia** agrega `formatRecipients([{name, email}])` como chip y limpia `pending`.
- **Lista:**
  - `role="listbox"`, opciones `role="option"` con nombre y correo, y `aria-activedescendant` en el input;
  - las flechas recorren la lista de forma circular;
  - Esc la cierra sin cerrar el redactor (en `Composer`, `onEscapeKeyDown` mira el ref de la lista igual que con el menú "/").
- **Chip:**
  - muestra el nombre, o si no hay, el correo, con `title` igual al token completo;
  - tiene un botón × con `aria-label="Quitar <token>"`;
  - si el token no pasa `parseRecipients`, se marca con `aria-invalid` y en rojo.
- **`exclude`** son los correos de los chips válidos.

- [ ] **Step 1: Tests que fallan** en `RecipientInput.test.tsx`. Mockean `SuggestRecipients` y envuelven en `QueryClientProvider`. Usar los polyfills de `SignaturesSection.test.tsx`.
  - "un valor inicial se muestra entero como chips": con `value='Ana <a@x.com>, b@y.com'` hay dos chips ("Ana" y "b@y.com") y el input queda vacío.
  - "escribir y poner coma crea un chip": escribir `c@z.com,` llama `onChange` con `"…, c@z.com"`, crea un chip y deja el input vacío.
  - "pegar varias direcciones las convierte en chips": pegar `a@x.com; b@y.com, c@z.com` deja tres chips.
  - "Backspace en vacío quita el último chip".
  - "la × quita ese chip".
  - "un chip inválido queda marcado": `value="hola"` y blur dan un chip con `aria-invalid="true"`.
  - "las sugerencias aparecen y Enter elige la activa":
    - el mock devuelve `[{name:"Juan Pérez", email:"juan@x.com", contactId:1}, {name:"", email:"juana@y.com", contactId:0}]`;
    - al escribir "jua" la primera opción queda activa;
    - ↓ activa la segunda y Enter la elige: el último `onChange` es `"juana@y.com"` y queda un chip "juana@y.com".
  - "elegir una sugerencia con coma en el nombre deja un chip válido": click en `{name:"Peña, Fabricio", email:"f@x.com"}` deja el valor `"\"Peña, Fabricio\" <f@x.com>"`, y `parseRecipients` devuelve un válido.
  - "Esc cierra la lista": escribir "jua", la lista se abre, Esc cierra la lista y `pending` sigue siendo "jua".
  - "no sugiere lo que ya está en el campo": con un chip `juan@x.com`, `SuggestRecipients` se llama con `exclude` que contiene `juan@x.com`.
- [ ] **Step 2: Correr** `npx vitest run src/features/compose/RecipientInput.test.tsx` → FAIL.
- [ ] **Step 3: Implementar.**
  - El input queda dentro de un contenedor con borde que también contiene los chips (estética de `Input`, `flex-wrap`).
  - La lista va absoluta debajo del contenedor, sin portal: dentro del diálogo, igual que el menú "/".
- [ ] **Step 4: Correr** `npx vitest run src/features/compose` → PASS, incluido `Composer.test.tsx`. Si algún test de `Composer` escribía destinatarios con `user.type` y espera el texto del input, ajustarlo a chips sin cambiar lo que comprueba. Después `task check` → verde.
- [ ] **Step 5: Commit** `feat(redactor): destinatarios como chips con sugerencias`.

---

### Task 4: Libreta de contactos en los paneles

**Files:**
- Create: `frontend/src/features/contacts/ContactList.tsx`, `ContactPanel.tsx`, `ContactList.test.tsx`, `ContactPanel.test.tsx`, y `frontend/src/components/ui/textarea.tsx` (`npx shadcn@latest add textarea`; si no hay red, escribirlo a mano con el estilo de `input.tsx`).
- Modify: `frontend/src/lib/view.ts`, `features/layout/MailShell.tsx`, `features/folders/FolderSidebar.tsx`, `App.tsx`, `lib/queries.ts`, `features/compose/ComposerContext.tsx`, y los tests que ya tocan `View` o `ShellState`.

**Interfaces:**
- Consumes: `svc.ListContacts`, `GetContact`, `SaveContact` y `DeleteContact`.
- Produces:
  - En `View`, la variante `{ kind: "contacts" }`, que se recuerda como las demás. `viewIsValid` siempre es true para ella y `syncView` no hace nada.
  - En `ShellState`:
    - `contactId: number | null`, donde `0` es una ficha nueva en edición;
    - `selectContact(id: number | null)`;
    - `openContact(id: number)`, que pasa a la vista `contacts` y abre esa ficha. `selectView` limpia `contactId`.
  - Queries:
    - `useContacts(query)` con la clave `["contacts", query]`;
    - `useContact(id)` con `["contact", id]`, deshabilitada para `null` y `0`;
    - `useContactForEmail(email)` con `["contact-for", email.toLowerCase()]`.
  - `ComposerContext.openNew(accountId: number, to?: Address[])`, que precarga Para.

- [ ] **Step 1: Tests que fallan.**
  - **`ContactList.test.tsx`:**
    - lista los nombres que se muestran;
    - escribir en "Buscar contactos" llama `ListContacts` con el texto;
    - sin contactos muestra "Todavía no tenés contactos.";
    - con búsqueda y sin resultados muestra "Ningún contacto coincide con «zzz».";
    - "Nuevo contacto" llama `selectContact(0)`;
    - click en una fila llama `selectContact(id)`.
  - **`ContactPanel.test.tsx`:**
    - con `contactId=null` muestra "Elegí un contacto o creá uno nuevo.";
    - la ficha muestra correos, teléfonos, empresa y notas;
    - "Editar", cambiar "Empresa" y "Guardar" llaman `SaveContact` con la ficha completa;
    - "Agregar correo" suma un campo;
    - en una ficha nueva (`0`), "Guardar" llama `SaveContact` con `id: 0` y después `selectContact(<id devuelto>)`;
    - si `SaveContact` rechaza con "«x» ya está en la ficha de Ana.", el texto se muestra en un `Alert` y el formulario sigue abierto;
    - "Cancelar" en una ficha nueva llama `selectContact(null)`;
    - "Eliminar" pide la confirmación con los textos exactos, y confirmar llama `DeleteContact(id)` y `selectContact(null)`;
    - "Escribirle" llama `openNew(<id de la primera cuenta>, [{name, email: primer correo}])`. Se muestra solo si hay correos.
  - **`FolderSidebar.test.tsx`:** la fila "Contactos" llama `selectView({ kind: "contacts" })` y queda con `aria-current` cuando la vista es esa.
  - **Test de `view`** (agregar donde se testee `loadView`): `{ kind: "contacts" }` se guarda y se recupera.
- [ ] **Step 2: Correr** los tests nuevos → FAIL.
- [ ] **Step 3: Implementar.**
  - `App.tsx`: en la vista `contacts`, `list` renderiza `ContactList` y `reader` renderiza `ContactPanel`.
  - Fila "Contactos" en `FolderSidebar`:
    - con el mismo estilo que `OutboxRow` e ícono `UsersIcon`;
    - debajo de las cuentas;
    - siempre visible.
  - `ContactPanel`: la vista de ficha y el formulario son el mismo componente, con un estado `editing`.
  - Los teléfonos se muestran como texto. Los correos se muestran como texto seleccionable, sin `mailto:`, porque para escribir está "Escribirle".
- [ ] **Step 4: Correr** `npx vitest run` → PASS, y `task check` → verde.
- [ ] **Step 5: Commit** `feat(ui): libreta de contactos`.

---

### Task 5: "Agregar a contactos" desde el lector

**Files:**
- Modify: `frontend/src/features/reader/MessageView.tsx`, `ThreadView.tsx` (pasa `openContact`)
- Test: `frontend/src/features/reader/MessageView.test.tsx` (nuevo, o el archivo que ya cubra el encabezado del lector)

**Interfaces:**
- Consumes: `svc.ContactForEmail`, `svc.AddSenderToContacts`, `useContactForEmail` y `ShellState.openContact`.
- Produces: el componente interno del mensaje recibe `openContact: (id: number) => void`. `MessageView` lo saca de `state` y `ThreadView` lo pasa a cada mensaje.

- [ ] **Step 1: Tests que fallan.**
  - Junto al remitente hay un botón de ícono `UserPlusIcon` con `aria-label="Agregar a contactos"`. Al tocarlo:
    - llama `AddSenderToContacts(message.from)`;
    - muestra el toast "«Ana» se agregó a tus contactos.", con el nombre que se muestra de la ficha devuelta.
  - "con el remitente ya en la libreta muestra Ver contacto": `ContactForEmail` devuelve `{id: 7, …}`, así que el botón es `UserIcon` con `aria-label="Ver contacto"`, y al tocarlo llama `openContact(7)`.
  - En un borrador (rol `drafts`), el remitente es uno mismo, así que no aparece ningún botón.
- [ ] **Step 2: Correr** → FAIL.
- [ ] **Step 3: Implementar.** Después de agregar, el evento invalida `["contact-for"]` y el botón pasa solo a "Ver contacto".
- [ ] **Step 4: Correr** `npx vitest run` y `task check` → verde.
- [ ] **Step 5: Commit** `feat(lector): agregar el remitente a contactos`.

---

### Task 6: Prueba manual de Fabricio

No es una tarea de código: la hace Fabricio con `task install:desktop`.

- [ ] Crear dos o tres contactos, uno con varios correos y teléfonos, y buscarlos por nombre sin acento, por empresa y por correo.
- [ ] Abrir un correo recibido, "Agregar a contactos", y comprobar que el botón pasa a "Ver contacto" y abre la ficha.
- [ ] Redactar:
  - escribir dos letras de un contacto y elegirlo con Enter;
  - escribir dos letras de alguien a quien le escribiste y que no es contacto, y verlo sugerido;
  - un newsletter que solo te escribe a vos no aparece.
- [ ] Pegar una lista de direcciones en Para, quitar una con la ×, y enviar.
- [ ] Retomar un borrador con destinatarios: se ven como chips.
- [ ] "Escribirle" desde una ficha abre el redactor con Para cargado.

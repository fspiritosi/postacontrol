# SDD ledger — plan: docs/superpowers/plans/2026-10-30-fase-1-primer-correo.md

Spec: docs/superpowers/specs/2026-10-30-postacontrol-design.md (leído). Rama: fase-1 (en el lugar, elegido por Fabricio). Merge base: 80c1769.

## Pre-flight scan

| Tareas | Qué se cruza | Hallazgo |
|---|---|---|
| T1 → todas | T1 produce `core/model` + tags JSON camelCase; las demás consumen | OK |
| T2 ↔ T11 | Store: UpsertHeaders/KnownUIDs/DeleteUIDsNotIn/SetFlags/SetFolderSync/SetFolderCounts/ResetFolder/GetFolder | OK, todas definidas en T2 |
| T2 ↔ T12/T13 | GetMessage/SetBody/SaveLocalDraft/ListLocalDrafts/DeleteLocalDraft | OK |
| T5 ↔ T7/T9/T10–T13 | testenv StartIMAP/StartSMTP/Deliver | T9 pide “opción RejectRcpt” que cambiaría la firma fija de StartSMTP → Ruling R1 |
| T5 testenv | IMAP sobre tls.Listen: imapserver puede no ver TLS y negar LOGIN | Ruling R2 |
| T7 ↔ T12 | PartPlan devuelve `[]int{1}` para no-multipart; BODY[1.MIME] no está bien definido en mensajes de una sola parte | Ruling R3 |
| T10 ↔ T11 | T10 llama a SyncAccount (stub) que T11 implementa; T11 lista sync.go como Create | Ruling R4 |
| T10 ↔ T11–T13 | AddAccount lanza sync en goroutine → tests no deterministas | Ruling R5 |
| T13 ↔ T9 | T13 usa `rechazo@ej.com` rechazado por testenv SMTP | cubierto por R1 |
| T13 testenv | TestSend_NoSentFolderWarns necesita borrar Sent en el server | el implementador de T13 agrega `DeleteMailbox` a testenv (dentro de testenv, sin conflicto) |
| T14 ↔ T10–T13 | MailService envuelve métodos de Core con las mismas firmas | OK |
| T15 ↔ T16 ↔ T17 | hooks/events (T15), SafeHtmlFrame/OpenExternal (T16) usados por T17; MailShell modificado por T17 | OK |
| T1 | `task check` corre eslint; la plantilla puede no traer ESLint | ya resuelto en el plan (Step 3) |
| Cada tarea | tests vs código vs archivos | Autoconsistentes salvo T9 (toca testenv sin listarlo, R1) y T11 (R4) |

- Ruling R1: `core/testenv` SMTP rechaza siempre el destinatario constante `testenv.RejectedRecipient = "rechazo@ej.com"` (lo agrega T9 dentro de testenv, sin cambiar la firma de StartSMTP) — mantiene estable la interfaz de T5 — si está mal, solo cambia un helper de test.
- Ruling R2: si imapserver rechaza LOGIN sobre el listener TLS, testenv usa `InsecureAuth: true` del lado servidor (solo en testenv; el cliente sigue verificando cert con RootCAs) — no viola la restricción TLS porque el cliente sigue en TLS — costo: ninguno en producción.
- Ruling R3: en T12, si la BodyStructure no es multipart, se baja con `FetchPart(uid, nil)` (HEADER + TEXT) en lugar de `[]int{1}`; PartPlan sigue devolviendo `[1]` como pide T7 — evita depender de BODY[1.MIME] en mensajes de una parte — costo: un branch más en read.go.
- Ruling R4: T10 crea `core/mail/sync.go` con un stub `func (c *Core) SyncAccount(ctx context.Context, accountID int64) error { return nil }`; T11 lo modifica (no Create) — firmas iguales entre tareas — costo: ninguno.
- Ruling R5: `mail.Config` gana `SkipInitialSync bool` (default false = producción sincroniza al agregar); los tests de T10–T13 lo ponen en true y sincronizan explícito cuando lo necesitan — tests deterministas — costo: un campo de config solo usado en tests.
Task 1: dispatched (implementer sonnet, base 80c1769)
Task 1: implementer DONE_WITH_CONCERNS (commit 8c65719); concerns: shadcn v4 sin preset new-york → radix-vega + zinc manual; `shadcn add` mete `import {cn} from "cn"` (reparar tras cada add); check construye frontend antes de vet/test (embed dist); RED del arch test falló en setup. Review dispatched (sonnet) con diff filtrado review-task1-filtered.diff.
- Ruling R6: las tareas futuras que corran `shadcn add` deben verificar y corregir `import { cn } from "cn"` → `@/lib/utils` y no dejar el paquete `cn` en package.json — evita regresión conocida del CLI — costo: un paso manual por tarea de UI.
Task 1: review → spec ❌ (estilo radix-vega en vez de new-york; dep geist sin uso), Important: guardia ESLint para import "cn"; RED del arch test sin probar (+ -test).
- Ruling R7: se acepta estilo shadcn `radix-vega` con colores zinc a mano en lugar de "new-york" — el CLI v4 ya no ofrece new-york; visualmente es la misma estética shadcn zinc — si está mal, se reconfigura components.json.
- Ruling R8: el arch test usa `go list -deps -test ./core/...` (endurece el snippet del plan) — cubre imports de tests — costo: ninguno.
Task 1: minor (deferred): slices nil serializan como null en JSON (model.go) — tareas de core deben inicializar slices o el frontend proteger.
Task 1: minor (deferred): tsconfig noImplicitAny:false heredado de plantilla.
Task 1: minor (deferred): frontend/.npmrc minimum-release-age ignorado por npm; README de plantilla Wails.
Task 1: fix round 1 dispatched (resume implementer; base fix 8c65719)
Task 1: fix round 1 implementer DONE (commit 5e67b6f); re-review dispatched (haiku)
Task 1: fix round 1/5 (5 addressed, 0 open; commits 8c65719..5e67b6f). Re-review nota: import.meta.dirname en vite.config sin build verificado → resuelto por controller: `task check` ejecuta common:build:frontend (vite build) antes de vet/test, y el report muestra exit 0.
Task 1: minor (deferred): sonner.tsx usa next-themes; Task 15 ThemeProvider debe integrarse o reemplazarlo.
Task 1: complete (commits 80c1769..5e67b6f, review clean)
Task 2: dispatched (implementer sonnet, base 5e67b6f)
Task 2: implementer DONE (commit 8f95488); review dispatched (sonnet)
- Ruling R9: se aceptan trailers Co-Authored-By con el modelo real del subagente (p.ej. "Claude Sonnet 5.5") en vez de "Opus 5.5" — la atribución exacta del autor es más honesta; el harness del subagente lo indica — costo: inconsistencia cosmética en el log.
Task 2: review → spec ✅; Important: test de atomicidad de UpsertHeaders no prueba atomicidad. Fix round 1 dispatched (+ toJSON error tragado, tests de preservar cuerpo y paginado).
Task 2: minor (deferred): DeleteUIDsNotIn borra fila por fila fuera de tx (lento en carpetas grandes).
Task 2: minor (deferred): SetFlags/DeleteLocalDraft no devuelven ErrNotFound (inconsistente con SetBody).
Task 2: fix round 1/5 (3 addressed, 0 open; commits 8f95488..070f386)
- Ruling R10: se acepta `CHECK (uid > 0)` agregado in-place al schema v1 — no hay bases publicadas todavía; los UID IMAP son ≥1 — costo: ninguno antes del primer uso real.
Task 2: complete (commits 5e67b6f..070f386, review clean)
Task 3: dispatched (implementer haiku, base 070f386)
Task 3: implementer DONE (commit a61aa6b); review dispatched (sonnet)
Task 3: review → spec ✅, quality Approved (trailer: cubierto por R9).
Task 3: minor (deferred): email vacío en secrets no validado; validar en AddAccount (Task 10).
Task 3: complete (commits 070f386..a61aa6b, review clean)
Task 4: dispatched (implementer sonnet, base a61aa6b)
Task 4: implementer DONE (commit 9cd4956); review dispatched (sonnet)
Task 4: review → spec ✅; Important: downgrade https→http por redirect en http.Client default. Fix round 1 dispatched (+ %EMAILDOMAIN%, ctx.Err(), test de tags).
Task 4: minor (deferred): cobertura (STARTTLS en IMAP, ssl minúscula, XML sin SMTP válido, body enorme).
Task 4: fix round 1/5 (3 addressed, 1 open — test de redirect no ejercita cliente default ni cap 5; commits 9cd4956..bc9b280). Fix round 2 dispatched.
Task 4: fix round 2/5 (1 addressed, 0 open; commits bc9b280..e7c43fc)
Task 4: minor (deferred): test del cliente default solo prueba downgrade, cap 5 probado vía checkRedirect directo.
Task 4: complete (commits a61aa6b..e7c43fc, review clean)
- Ruling R11 (refina R1): el SMTP de testenv rechaza `testenv.RejectedRecipient = "rechazo@ej.com"` desde la Task 5 (no en T9) — una sola tarea toca la firma/comportamiento de testenv SMTP — costo: ninguno.
Task 5: dispatched (implementer sonnet, base e7c43fc)
Task 5: implementer DONE (commit cc73a42; impl antes que tests — RED vía build roto); review dispatched (sonnet)
Task 5: review → spec ❌: HasAttachments ignora disposition (BODY no extendido); Important: sin test de Select read-only. Fix round 1 dispatched (+ ctx/timeout en Dial/Login).
Task 5: minor (deferred): todo *imap.Error de Login → ErrAuth (incluye NO [UNAVAILABLE]); isTLSError con strings.Contains("tls:"); RejectedRecipient case-sensitive; sin test de Append a mailbox inexistente.
Task 5: minor (deferred): TDD invertido (tests después de impl), sin impacto en código.
Task 5: fix round 1/5 (3 addressed, 0 open; commits cc73a42..564c4de)
Task 5: minor (deferred): Dial puede devolver Conn sobre cliente cerrado si ctx vence justo tras LOGIN (falta ctx.Err() tras stop()); comandos post-Dial sin ctx/deadline (solo timeout 30s de go-imap).
Task 5: complete (commits e7c43fc..564c4de, review clean)
Task 6: dispatched (implementer sonnet, base 564c4de)
Task 6: implementer DONE_WITH_CONCERNS (commit 05262ef; url() remotas en CSS no bloqueadas → CSP T16); review dispatched (sonnet)
Task 6: review → spec ✅; Important: ToText pega celdas de tabla; reescritura de img src es denylist (relativas pasan). Fix round 1 dispatched (+ hasRemoteImages por url()/@import CSS, <pre> en ToText).
Task 6: minor (deferred): cobertura (blockquote anidado, li con contenido anidado, Snippet con palabra > n).
- Ruling R12: <img src="ftp://…"> se elimina entero (bluemonday) en vez de pasar a data-remote-src — más seguro (no carga nada); solo pierde la opción de mostrarla — costo: imágenes ftp nunca visibles (rarísimas).
Task 6: fix round 1/5 (4 addressed, 0 open; commits 05262ef..8d9afec)
Task 6: minor (deferred): cssHasRemote no detecta url( ofuscado (escapes/comentarios) — CSP T16 lo cubre.
Task 6: complete (commits 564c4de..8d9afec, review clean)
Task 7: dispatched (implementer sonnet, base 8d9afec)
Task 7: implementer DONE_WITH_CONCERNS (commit c82b588; memserver descarta filename* latin1 → caso probado con BodyStructure sintética; Plan con recursión propia en vez de Walk); review dispatched (sonnet)
- Ruling R13: se acepta probar RFC 2231 iso-8859-1 con BodyStructure sintética (params crudos tipo Dovecot) en vez de vía memserver — limitación del servidor de pruebas, no del código — costo: el camino end-to-end se valida en la aceptación manual (T18).
Task 7: review → spec ❌ (text/* con nombre → adjunto, no señalado); Important: DecodeText traga errores / rama muerta. Fix round 1 dispatched (+ readHeader parcial, tests extra).
- Ruling R14: DecodeText mantiene el contrato "texto best-effort + nil" ante entrada malformada (el lector no debe romper); error solo para fallas reales; ante falla de transfer-decoding devuelve el cuerpo crudo — costo: el usuario puede ver texto crudo sin aviso.
- Ruling R15: text/* con nombre va a Attachments (es un archivo adjunto) — costo: un cuerpo text/plain con name= (raro) se mostraría como adjunto.
- Cross-task gap G1: el Subject/From del ENVELOPE con charset legacy (=?windows-1252?…) depende de imapclient.Options.WordDecoder con CharsetReader de go-message/charset en imapx.Dial (Task 5). Llevar a Task 11: verificar/agregar y testear asunto latin1 decodificado.
Task 7: minor (deferred): filename 8-bit latin1 crudo sin decodificar; adjunto sin nombre → Filename "" (UI necesita default); message/rfc822 sin nombre ni CID se descarta; imagen con CID fuera de related → adjunto (cid sin resolver); decode2231 con segmentos mixtos y % literal.
Task 7: fix round 1/5 (4 addressed, 0 open; commits c82b588..c07b2cb)
Task 7: minor (deferred): DecodeText devuelve ("", err) si la conversión de charset no produce bytes — casi inalcanzable; más coherente con R14 caer a toUTF8(data).
Task 7: complete (commits 8d9afec..c07b2cb, review clean)
Task 8: dispatched (implementer sonnet, base c07b2cb)
Task 8: implementer DONE_WITH_CONCERNS (commit f164259; filename no ASCII sale RFC 2047 entrecomillado, no 2231 — go-message lo fuerza); review dispatched (sonnet)
Task 8: review → spec ❌: filename no ASCII en RFC 2047 (el brief pide 2231; la librería sí puede). Fix round 1 dispatched (+ Date cero, inyección CR/LF en In-Reply-To/References).
Task 8: minor (deferred): NewMessageID usa localhost sin dominio; Build omite Message-ID vacío sin avisar (T13 debe llamar siempre NewMessageID); sin test de cuerpo vacío ni varios adjuntos.
Task 8: fix round 1/5 (3 addressed, 0 open; commits f164259..9727dde)
Task 8: minor (deferred): Subject y MessageID sin chequeo de CR/LF (posible inyección si el asunto trae CR/LF) — revisar en final review; test de inyección solo afirma Error.
Task 8: complete (commits c07b2cb..9727dde, review clean)
Task 9: dispatched (implementer sonnet, base 9727dde)
Task 9: implementer DONE_WITH_CONCERNS (commit 3970de9; STARTTLS sin test de entrega completa); review dispatched (sonnet)
Task 9: review → spec ✅ (dial manual con ctx justificado); Important: isTLSError duplicado con imapx; STARTTLS sin test e2e. Fix round 1 dispatched (+ AUTH 4xx → ErrNetwork, STARTTLS no anunciado → ErrTLS).
Task 9: minor (deferred): Security desconocido → ErrNetwork (debería ser error de config); rcpts vacío falla en DATA; t.Cleanup desde goroutine en test.
Task 9: fix round 1/5 (3 addressed, 0 open; commits 3970de9..9b137c0)
Task 9: minor (deferred): detección "STARTTLS no anunciado" por string de go-smtp (frágil ante upgrade; test lo detectaría).
Task 9: complete (commits 9727dde..9b137c0, review clean)
Task 10: dispatched (implementer sonnet, base 9b137c0)
Task 10: implementer DONE (commit 9fa09e2); review dispatched (sonnet)
Task 10: review → spec ✅; Important: emails duplicados comparten clave de llavero → RemoveAccount borra la contraseña del otro. Fix round 1 dispatched (+ color libre, validación en español, orden de borrado en RemoveAccount).
- Ruling R16: nuevos códigos UserError "duplicate" ("Esa cuenta ya está configurada.") e "invalid" ("La dirección de correo no es válida." / "Ingresá la contraseña.") además de los del plan — mensajes claros en español para errores de validación — costo: el frontend debe mostrar Msg tal cual (ya lo hace por diseño).
Task 10: minor (deferred → T11): goroutine de sync en AddAccount sin trackear; Close() no la espera y usa context.Background(). T11 debe darle ctx cancelable y esperar en Close.
Task 10: minor (deferred): prefijo "smtpx: envío rechazado: " acoplado por string entre errors.go y smtpx.
Task 10: fix round 1/5 (4 addressed, 0 open; commits 9fa09e2..854c78e)
- Ruling R17: el email se normaliza a minúsculas también como usuario de login por defecto — en la práctica (Hostinger y casi todos) el login es case-insensitive; simplifica duplicados y llavero — costo: si algún servidor fuera case-sensitive, el usuario puede cargar Username a mano en el diálogo.
Task 10: complete (commits 9b137c0..854c78e, review clean)
Task 11: dispatched (implementer opus — concurrencia/recuperación, base 854c78e)
Task 11: implementer DONE_WITH_CONCERNS (commit c0980d6; toca imapx: opErr→ErrNetwork, Conn.Abort, WordDecoder (G1); testenv: DeliverAt, Expunge(t,..), RecreateMailbox(t,name) uint32); review dispatched (opus)
- Ruling R18 (→ T12/T13): las operaciones no idempotentes (APPEND a Enviados, STORE \Answered tras enviar) NO deben pasar por el reintento automático de withConn; usar una variante sin reintento (p.ej. withConnNoRetry) — evita duplicar la copia en Enviados — costo: un error de red ahí se reporta como Warning sin reintentar.
Task 11: review (opus) → spec ✅; Important [plan-mandated]: cambio de UIDVALIDITY tras primera sync interrumpida no se detecta. Fix round 1 dispatched (+ ctx derivado a fn, withConnNoRetry, test incremental, doc de Notifier).
- Ruling R19: con last.UIDValidity==0 se persiste {UIDValidity, UIDNext:0} apenas tras Select; UIDNext==0 = sync incompleta; reemplaza la aserción del plan "FolderSync sigue en 0" — cierra hueco de integridad (operaciones por UID sobre el mensaje equivocado) — costo: ninguno visible.
- Ruling R20: withConn pasa a `fn func(ctx, *imapx.Conn) error` (ctx derivado) — cancelación por Close visible dentro de la operación; T12/T13 usan esta firma — costo: desvío menor de la firma del plan.
Task 11: minor (deferred): refreshFlags carga la carpeta entera cada pasada (O(n)); RemoveAccount→dropConn no aborta comando en curso; errores NO/FK sin traducir llegan en inglés; credenciales rechazadas se reintentan cada tick sin backoff (riesgo de bloqueo de cuenta); sin test de concurrencia SyncFolder ∥ background.
Task 11: fix round 1/5 (5 addressed, 0 open; commits c0980d6..1f3b5e2)
Task 11: minor (deferred): test de recuperación indexa progress[len-2] sin require de longitud; lifeCtx anidado redundante.
Task 11: complete (commits 854c78e..1f3b5e2, review clean)
Task 12: dispatched (implementer sonnet, base 1f3b5e2)
Task 12: implementer DONE (commit 8060e1f; TDD invertido de nuevo; attachmentBytes vía GetMessage marca leído); review dispatched (sonnet)
Task 12: review → spec ❌: GetMessage traga errores de caché de markSeen. Fix round 1 dispatched (+ UID inexistente → not_found con sentinela imapx.ErrNoMessage, código "stale" para UIDVALIDITY cambiado, SaveAttachment atómico).
- Ruling R21: nuevo código UserError "stale" ("El mensaje cambió en el servidor. Actualizá la carpeta.") — distingue de not_found — costo: un código más para la UI (muestra Msg).
Task 12: minor (deferred): carrera leve en decremento de Unread; attachmentBytes vía GetMessage marca leído al bajar adjunto (efecto lateral aceptado); sin test de que el fetch no marque \Seen.
Task 12: fix round 1/5 (4 addressed, 0 open; commits 8060e1f..ee2ab28)
Task 12: minor (deferred): IMAP NO/BAD en STORE \Seen llega sin traducir (asUserError sin caso para imap.Error genérico).
Task 12: complete (commits 1f3b5e2..ee2ab28, review clean)
Task 13: dispatched (implementer sonnet, base ee2ab28)
Task 13: implementer DONE_WITH_CONCERNS (commit 0dcf498; TDD invertido; código extra no_recipients; RV: no deduplica en forward por el test del plan); review dispatched (sonnet)
- Ruling R22: prefijos — reply deduplica Re:/RE:, forward deduplica Fwd:/FW:; RV: no se trata como Fwd (lo exige el test del plan "Fwd: RV: Hola") — costo: "Fwd: RV:" en reenvíos de correos ya reenviados desde clientes en español.
- Ruling R23: código UserError "no_recipients" agregado — validación temprana — costo: la UI ya deshabilita Enviar sin destinatarios.
Task 13: review → spec ❌: fallos previos al SMTP (GetMessage, attachmentBytes, Build) no guardan borrador; sin test de text/plain citado con ">". Fix round 1 dispatched (+ validar destinatarios antes, Mode desconocido → invalid, fecha cero omitida).
Task 13: minor (deferred): no valida que el original pertenezca a req.AccountID; markAnswered usa flags del inicio; inline cid reenviados como adjuntos; attachmentBytes llama GetMessage N veces.
Task 13: fix round 1/5 (3 addressed, 0 open; commits 0dcf498..920dfd3)
Task 13: minor (deferred): si SaveLocalDraft falla tras un fallo de envío, se descarta en silencio (el usuario no sabe que el borrador no se guardó; el redactor sigue abierto en la UI).
Task 13: complete (commits ee2ab28..920dfd3, review clean)
Task 14: dispatched (implementer sonnet, base 920dfd3)
Task 14: implementer DONE_WITH_CONCERNS (commit 98e40b1; bindings tipan listas T[]|null; params Go-case); review dispatched (sonnet, diff sin bindings generados)
- Ruling R24: el frontend (T15–T17) siempre normaliza listas de bindings con `?? []`, porque el tipo generado es `T[] | null` aunque el core intente devolver no-nil — defensa en profundidad — costo: ninguno.
Task 14: review → spec ❌: SetApp exportado queda enlazado a JS (puede dejar app nil; bindings espurios). Fix round 1 dispatched (application.Get() en ServiceStartup, UserError en SaveAttachment/diálogo/OpenURL, diálogo modal si es barato).
- Ruling R25: código UserError "ui" para fallas del diálogo/abrir enlace — mensajes en español — costo: ninguno.
Task 14: minor (deferred): sin test de ciclo de vida ServiceStartup/Shutdown ni de SaveAttachment en app/.
Task 14: fix round 1/5 (3 addressed, 0 open; commits 98e40b1..bfd411b)
Task 14: minor (deferred): validateExternalURL devuelve errors.New planos (sin UserError); sin tests de SaveAttachment/OpenExternal en app/.
Task 14: complete (commits 920dfd3..bfd411b, review clean)
Task 15: dispatched (implementer sonnet, base bfd411b)
Task 15: implementer DONE_WITH_CONCERNS (commit 9cd079a; sin verificación visual; quitó next-themes); review dispatched (sonnet)
Task 15: review → spec ❌: falta verificación visual (Step 4); Important: carrera cerrar-diálogo vs DetectSettings. Fix round 1 dispatched (+ errorMessage() para rechazos de Wails, validación de host/puerto, useLayoutEffect en tema, tests de copy de carga).
- Ruling R26: la verificación visual claro/oscuro (Step 4 de T15, y equivalentes de T16/T17) la hace el controlador tras T17 con navegador contra el dev server de Vite (WebKitGTK no es capturable por los subagentes) — costo: se detecta tarde algún problema visual.
Task 15: fix round 1/5 (5 addressed, 0 open; commits 9cd079a..9df43f8)
Task 15: minor (deferred): rama de descarte por requestId sin test propio; events.ts muestra ev.data.message directo.
Task 15: complete (commits bfd411b..9df43f8, review clean; visual pendiente por R26)
Task 16: dispatched (implementer sonnet, base 9df43f8)
Task 16: implementer DONE_WITH_CONCERNS (commit bbb16c7; stale detectado por texto; test del plan 'bloquea remotas' ajustado a regex); review dispatched (sonnet)
Task 16: review → spec ❌ (botones de respuesta sin cablear → R27 a T17); Important: altura del iframe 32px corta; etiquetas de rol duplicadas. Fix round 1 dispatched (+ constante/contrato stale Go↔UI, tests de srcdoc más fuertes, test de intercepción de enlaces, reset de selección al quitar cuenta).
- Ruling R27: Responder/Responder a todos/Reenviar se cablean a PrepareCompose en T17 (el brief de T16 lo pedía, pero el redactor es de T17) — costo: ninguno, T17 lo cubre.
Task 16: minor (deferred): test "no introduce scripts" es débil (el <script> sobrevive y lo neutralizan CSP+sandbox); sin tests de format.ts.
Task 16: fix round 1/5 (5 addressed, 1 open — instanceof HTMLAnchorElement entre realms rompe enlaces; commits bbb16c7..2f402ed). Fix round 2 dispatched (+ observer en rama texto, altura que encoge).
Task 16: fix round 2/5 (1 addressed, 0 open; commits 2f402ed..ec6a242)
Task 16: complete (commits 9df43f8..ec6a242, review clean; visual pendiente por R26)
Task 17: dispatched (implementer sonnet, base ec6a242)
Task 17: implementer DONE_WITH_CONCERNS (commit a96c854; sin RED; borrador reabierto no muestra la cita original; 4 desvíos menores de UX); review dispatched (sonnet)
Task 17: review → spec ❌; Important: trackNewestDraft asume id más alto = borrador nuevo (puede borrar un borrador ajeno o dejar huérfanos). Fix round 1 dispatched (diff de ids, nota condicional, prop en SafeHtmlFrame, test de dirección inválida).
- Ruling R28: se aceptan los desvíos de T17 — (a) Enviar deshabilitado con cualquier dirección inválida; (b) "1 correo / N correos"; (c) confirmar descarte solo tras edición; (d) borrador reabierto no muestra la cita (el backend la reconstruye al enviar) — costo: (d) el usuario no ve el original al revisar el borrador.
Task 17: minor (deferred → R26): window.prompt para links en WebKitGTK podría no funcionar; verificar en la prueba visual/manual.
Task 17: fix round 1/5 (4 addressed, 0 open; commits a96c854..c0afea6)
Task 17: complete (commits ec6a242..c0afea6, review clean)
R26 visual check (controller): server mode (task build:server DEV=true) + seed en scratchpad + chrome-devtools. Hallazgo V1: en tema claro, etiquetas de carpetas, asuntos y remitentes no leídos casi invisibles (contraste muy bajo).
R26 visual check: OK. V1 retractado (era la animación de carga). Bienvenida, alta, 3 paneles, lector (CSP/sandbox verificados en DOM, altura correcta), tema oscuro con tarjeta blanca y redactor con cita: correctos. Notas menores: imagen remota bloqueada muestra ícono roto con alt; resaltado de carpeta seleccionada sutil en claro; bundle JS 2.2 MB.
Final review: dispatched (opus)
Final review (opus): With fixes. Important: (1) primera sync bloquea lectura/envío y no llena la lista progresivamente; (2) APPEND a Enviados sobre conexión posiblemente muerta; (3) no hay forma de desactivar SaveSentCopy. Minors seleccionados para el mismo fix wave.
- Ruling R29: el fix wave final incluye, además de los 3 Important, estos minors baratos y relevantes para la prueba real: toasts duplicados, barra de progreso congelada, cita con imágenes remotas en vista previa, espaciado de párrafos del editor, \Seen NO best-effort, timeout del handshake SMTP, EHLO con hostname, LogLevel Info explícito, lang="es", window.prompt → popover inline, ToText no cuadrático. Quedan diferidos el resto (ver triage) — costo: un fix wave más grande.
Final fix wave: dispatched (opus, base c0afea6)
Final fix wave: DONE_WITH_CONCERNS (10 commits c0afea6..a26914f; lock por carpeta agregado; timeout SMTP solo fase de conexión; EHLO pre-STARTTLS sigue localhost). Re-review dispatched (sonnet).
Final fix wave: re-review clean (c0afea6..a26914f). Minors: foco tras Aplicar en popover de link; syncLocks sin poda; test de perf con presupuesto de tiempo; toasts dobles en PrepareCompose/SaveAttachment; hostname en EHLO expone nombre del equipo; aria-pressed de la toolbar puede quedar desactualizado.

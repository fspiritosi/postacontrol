# SDD ledger — plan: docs/superpowers/plans/2026-10-31-fase-3b-firmas-y-borradores.md

Spec: docs/superpowers/specs/2026-10-30-postacontrol-design.md (alcanzable).
Rama: fase-3b desde main 201bbc8. Plan commiteado en cc8027b. MERGE_BASE = 201bbc8.

## Pre-flight scan

| Par / tarea | Produce → consume | Hallazgo |
|---|---|---|
| T1→T2 | model.Signature, Account.DefaultSignatureID, store.GetSignature/ListSignatures → buildOutgoing, Core CRUD | consistente |
| T1→T3 | SaveComposeDraft(id, accountID, req, messageID), MarkDraftUploaded(id, uploadedAt), ListPendingDrafts → SaveDraft/UploadDraft | **conflicto**: el store guarda tiempos con `.Unix()` (segundos); con segundos, un guardado durante la subida en el mismo segundo queda marcado como subido. Ver Ruling 1. |
| T1→T3 | SaveComposeDraft "crea o actualiza" → enqueue/DeleteDraft borran la fila | **riesgo**: un guardado en vuelo con id≠0 después de enviar/descartar resucitaría la fila. Ver Ruling 2. |
| T2→T3 | buildOutgoing(forDraft) con DraftMessageID, IncludeBcc → UploadDraft | consistente (T3 paso 1 fija DraftMessageID desde la fila) |
| T2→T4/T5 | bindings de firmas, ComposeDraft.SignatureID → useSignatures, SignaturePicker | consistente |
| T3→T6 | SaveDraft/UploadDraft/DeleteDraft/OpenDraftMessage/ListPendingDrafts, mail:drafts-changed → useDraftAutosave, banner, MessageActions | consistente; Draft.NeedsUpload define estado "local" |
| T4→T5 | useSignatures (queries.ts, key ["signatures"]) → SignaturePicker | consistente |
| T5→T6 | Composer.tsx, ComposerContext.tsx (secuencial) | consistente; T6 construye sobre el pedido de T5 (signatureId) |
| T6 interna | enviar cancela timers vs. guardado serializado en vuelo | **riesgo**: un SaveDraft en vuelo al enviar. Ver Ruling 2 (back) y Ruling 3 (UI). |
| T1 | tests ↔ código; archivos | consistente; migración v6 sigue el patrón TestMigrateV4 |
| T2 | tests ↔ código | consistente |
| T3 | "cuentas que no están en pausa" | existe `authPaused(accountID)` (core/mail/authpause.go) — usar ese |
| T3 | adjuntos ajenos "preparados con attachmentBytes" | existe store.StageAttachment(ctx, filename, mime, data) |
| T4, T5, T6 | tests ↔ código | consistentes |

Ruling 1: `compose_drafts.updated_at` se guarda en UnixNano (no segundos) y SaveComposeDraft lo hace estrictamente creciente por fila (max(now, prev+1)); UploadDraft pasa a MarkDraftUploaded el `UpdatedAt` de la fila leída al inicio de la subida — con segundos, un guardado en el mismo segundo de la subida se marcaría como subido y se perdería en el servidor — si es incorrecto cuesta solo cambiar la unidad de una columna nueva.
Ruling 2: SaveComposeDraft con id≠0 y fila inexistente devuelve ErrNotFound (no recrea); Core.SaveDraft lo traduce a UserError not_found — evita resucitar un borrador ya enviado o descartado por un guardado en vuelo — si es incorrecto, la UI recibe un error que debe ignorar.
Ruling 3: en el redactor, enviar y descartar esperan a que termine la llamada de borrador en vuelo (la cadena serializada del hook) antes de QueueSend/DeleteDraft; un not_found de SaveDraft/UploadDraft se ignora en silencio — complementa Ruling 2 desde la UI — si es incorrecto, el envío tarda lo que tarde un guardado local.

## Tareas
Task 1: dispatched (BASE cc8027b, implementer sonnet a56aa812257ff6b90)
Task 1: complete (commits cc8027b..022da09, review clean)
Task 1: minor (deferred): MarkDraftUploaded/MarkDraftUploadFailed devuelven nil si la fila no existe o se saltea; sin comentario que lo diga (composedrafts.go:268-277)
Task 1: minor (deferred): message_id duplicado al crear sale como error UNIQUE crudo de SQLite (composedrafts.go:212)
Task 1: minor (deferred): sin test que fuerce la rama +1 de updated_at (mismo nanosegundo)
Task 1: minor (deferred): SetAccountSignature con firma inexistente devuelve error FK crudo, no ErrNotFound
Task 1: note: Composer.tsx manda signatureId:0, composeDraftId:0, draftMessageId:"" como placeholders — T5/T6 los cablean
Task 2: dispatched (BASE 022da09, implementer sonnet a3a9ff690b07ee768)
Task 2: complete (commits 022da09..b7e44b9, review clean)
Task 2: minor (deferred): deliver vuelve a pedir GetMessage del original después del envío solo para markAnswered; si falla, no queda \Answered (compose.go:371-375) — antes reusaba orig
Task 2: minor (deferred): sin test de que las imágenes data: de la firma cuentan en checkSize
Task 2: minor (deferred): SaveSignature descarta el segundo valor de htmlx.Sanitize sin comentario (signatures.go)
Task 2: minor (deferred): TestSend_SignatureLogoTravelsAsCID usa PNG falso; el tope de ancho no se ejercita junto con firmas
Task 3: dispatched (BASE b7e44b9, implementer opus a9978764267153483)
Task 3: implementer DONE_WITH_CONCERNS c291d14; review dispatched (opus a3376dd0a75bdbdcc)
Ruling 4 (concern T3 #1, cambiar "De" en un borrador ya subido deja la copia vieja en Borradores de la otra cuenta): se arregla en la ronda de fix de T3 — columna `uploaded_account_id` en compose_drafts (schema_v6 todavía no está en main, se puede tocar), la fija MarkDraftUploaded; UploadDraft y DeleteDraft borran por Message-ID en la carpeta drafts de esa cuenta si difiere de la actual (mejor esfuerzo) — sin esto Fabricio ve duplicados en el webmail — si es incorrecto cuesta una columna.
Ruling 5 (concern T3 #2, imágenes remotas de un borrador del webmail se pierden): se acepta como limitación — el editor de la 3A solo admite img data:, así que se perderían igual al editar; restaurarlas cargaría rastreadores — costo: un borrador ajeno con imágenes remotas las pierde al reeditar.
Ruling 6 (concern T3 #3, borrador de reenvío vuelve a bajar los adjuntos del original en cada subida): se difiere como minor para la revisión final — es rendimiento, no correctitud; las subidas son cada ≥30 s — costo: tráfico extra en reenvíos con adjuntos grandes.
Ruling 7 (concern T3 #4, borrador del webmail sin Message-ID queda duplicado): se acepta — los webmails ponen Message-ID; borrar por UID saldría de la regla "solo por Message-ID" — costo: un duplicado en un caso raro.
Ruling 8 (extra de T3: UploadDraft no toca la red con la cuenta en pausa por contraseña): se acepta — evita bloqueos del proveedor por reintentos de login.
Task 3: review → Needs fixes (1 Important: ComposeDraftID viejo en la bandeja de salida + rowid reutilizado borra un borrador nuevo ajeno) + Ruling 4 entra en la ronda 1
Task 3: minor (deferred): authPaused y "sin carpeta Borradores" se chequean después de buildOutgoing (baja adjuntos / hace LOGIN en cuenta en pausa)
Task 3: minor (deferred): UploadDraft no saltea si NeedsUpload ya es false (la pasada de fondo puede re-subir uno ya al día)
Task 3: minor (deferred): enqueue confía en DraftMessageID de la UI si ComposeDraftID==0 o falla el lookup; errores no-NotFound de GetComposeDraft se ignoran
Task 3: minor (deferred): texto de error con err.Error() en inglés puede llegar al usuario ("No pudimos armar el borrador: " + err.Error())
Task 3: minor (deferred): OpenDraftMessage falla entero sin conexión (fetch de ENVELOPE para Bcc obligatorio)
Task 3: minor (deferred): sin test del draftLock (subida vs DeleteDraft / limpieza post-envío)
Task 3: minor (deferred): cuenta sin carpeta Borradores queda needs_upload para siempre; cada pasada rearma MIME y emite evento
Task 3: fix round 1 dispatched (FIX_BASE c291d14; Important rowid reuse + Ruling 4 uploaded_account_id; AUTOINCREMENT in schema_v6)
Task 3: minor (deferred): si falla la limpieza en la cuenta vieja, la copia queda huérfana y ya no se rastrea (uploaded_account_id pasa a la nueva)
Task 3: minor (deferred): un reintento de envío desde la bandeja de salida no limpia la copia de la cuenta vieja (el id de cuenta vieja no viaja en la bandeja)
Task 3: fix round 1/5 (2 addressed, 0 open; commits c291d14..1e54c24)
Task 3: minor (deferred): en el reintento, si ListLocalDrafts falla, DraftMessageID queda vacío y no se limpian las copias del servidor
Task 3: complete (commits b7e44b9..1e54c24, review clean)
Task 4: dispatched (BASE 1e54c24, implementer sonnet a96812897c191363b)
Task 4: implementer DONE_WITH_CONCERNS fdc2acb (sin RED limpio: implementó antes de correr tests)
Task 4: ⚠️ verificado por el controller: DeleteSignature y SetAccountSignature emiten mail:signatures-changed + mail:accounts-changed (core/mail/signatures.go:41-52); events.ts invalida ["signatures"] y ["accounts"]
Task 4: complete (commits 1e54c24..fdc2acb, review clean)
Task 4: minor (deferred): el título de confirmar borrado parpadea "¿Eliminar la firma «»?" mientras se cierra (SignaturesSection.tsx:442)
Task 4: minor (deferred): Select de firma por defecto queda en blanco si defaultSignatureId apunta a una firma que ya no existe (SignaturesSection.tsx:423)
Task 4: minor (deferred): sin tests de Cancelar, de borrar la firma por defecto, ni del getter en edición
Task 4: note: confirmación de borrado usa Dialog (no hay AlertDialog en ui/) y agrega "Las cuentas que la usaban quedan sin firma por defecto."
Task 5: dispatched (BASE fdc2acb, implementer sonnet ad931e65239643a66)
Task 5: complete (commits fdc2acb..5f1db45, review clean)
Task 5: minor (deferred): sin test de reabrir un pedido de la bandeja/borrador con signatureId distinto del default o 0
Task 5: minor (deferred): ComposerContext.openNew siembra signatureId desde la caché, duplicando la lógica followsAccount del Composer
Task 5: minor (deferred): si falla la query de firmas, un id viejo se manda tal cual (el core lo trata como sin firma)
Task 6: dispatched (BASE 5f1db45, implementer opus a09d31cdf5fa01131)
Task 6: implementer DONE_WITH_CONCERNS 540957b
Ruling 9 (concern T6 #1, una respuesta/reenvío sin tocar se cierra sin crear borrador ni toast): se acepta — lo precargado no es algo que escribiste; Gmail hace lo mismo; la regla de "vacío" de la spec es un piso — costo: cerrar una respuesta sin tocar no deja borrador.
Ruling 10 (concern T6 #2, flush resuelve al guardar local y la subida sigue detrás): se acepta — cerrar no debe esperar la red; la pasada de fondo reintenta — costo: al cerrar sin red, el estado "local" no se ve (aparece en el aviso).
Task 6: minor (deferred): direcciones inválidas o a medio escribir no se guardan en el borrador (SendRequest solo lleva válidas)
Task 6: minor (deferred): el frontend detecta not_found comparando el texto "No encontramos lo que buscabas."; ningún test de Go fija ese texto
Task 6: note: "Cancelar" desaparece del pie con autoguardado; sigue en reintentos de la bandeja
Task 6: complete (commits 5f1db45..540957b, review clean)
Task 6: minor (deferred): cancel() no cubre un flush() en curso: flushNow encola UploadDraft después de que cancel resolvió; save/upload no miran st.stopped (hoy inalcanzable desde el Composer)
Task 6: minor (deferred): estado del pie arranca "idle" en un borrador reabierto y st.local=false aunque venga con needsUpload true
Task 6: minor (deferred): vaciar el redactor y cerrar con el primer SaveDraft(0) en vuelo puede dejar un borrador vacío en Borradores
Task 6: minor (deferred): send usa buildRequest del render del clic después del await; destinatarios/asunto del clic y cuerpo vivo
Task 6: minor (deferred): test "muestra el estado del borrador en el pie" tiene mock muerto (needsUpload true sin llegar a subir); esperas reales de 1500 ms en tests del Composer
Task 6: minor (deferred): chequeo inDrafts duplicado en MessageActions.tsx y MessageView.tsx
Task 6: ⚠️ (para revisión final): carrera entre redactores — subida en vuelo de un redactor cerrado mientras otro abre el mismo borrador y lo envía (depende del draftLock del core)
Final review: dispatched (opus a1d0336d6011d30a2, range 201bbc8..540957b)
Final review: With fixes — C1 (reintento borra borrador retomado), C2 (respuesta/reenvío con original movido no sube ni sale), I1 (borrar desde la lista no reconcilia compose_drafts → re-APPEND), I2 (fila local vieja gana a versión nueva del servidor con mismo Message-ID), I3 (chequeos de pausa/carpeta después de buildOutgoing), I4 (tests de draftLock y msgNotFound)
Ruling 11: la ola de fix final cubre C1, C2, I1, I3, I4 y tres minors baratos (err.Error en inglés, capWidth con height/style, comentario MarkDraftUploaded); I2 se difiere — depende de si el webmail de Hostinger conserva el Message-ID al re-guardar, que Fabricio verifica en la Task 7 — costo si es incorrecto: editar en el webmail un borrador que ya abriste en PostaControl y retomarlo en PostaControl pisa las ediciones del webmail.
Ruling 12: "direcciones a medio escribir no se guardan en el borrador" queda para la próxima iteración (revisor: real pero no bloquea) — costo: perder una dirección mal tipeada al cerrar.
Ruling 13: el resto de minors del triage quedan como están (revisor: can stay).
Final fix wave: dispatched (FIX_BASE 540957b, opus adae3ef31cb516b83)
Final fix wave: DONE_WITH_CONCERNS bdce9d1, 33feaaa
Final fix: minor (deferred): carrera de milisegundos — "Seguir editando" durante la limpieza post-envío deja el borrador sin copia en el servidor hasta la próxima edición
Final fix: minor (deferred): C2 solo degrada si el original ya no está en la caché; borrado en el servidor pero aún en caché falla hasta la próxima sincronización
Final fix: minor (deferred): tests de carrera de I4 usan sleeps de 100 ms (sin falsos negativos, pero pueden no ejercitar la carrera)
Final fix: minor (deferred): GetMessage del original al armar un borrador de respuesta puede marcar \Seen en el servidor (comportamiento previo de GetMessage)
Final fix: note: task check falló una vez con exit 201 y no se repitió (igual que en la 3A) — posible test inestable
Final fix wave: re-review clean (all addressed, 540957b..33feaaa)

## Pendiente
Task 7: prueba de aceptación manual de Fabricio con `task install:desktop` (no se despacha). Verificar además si el webmail de Hostinger conserva el Message-ID al re-guardar un borrador (decide la prioridad de I2, Ruling 11).

## Ajustes pedidos por Fabricio (2026-11-01)

- Configuración: el diálogo desbordaba porque su grilla tomaba el ancho mínimo de "Firma por defecto · <correo largo>". Ahora la columna es `minmax(0,1fr)` y el diálogo es `sm:max-w-2xl` (el de firma, `sm:max-w-3xl`).
- Redactor (cuerpo y firmas):
  - las imágenes se redimensionan desde las esquinas con el node view de TipTap;
  - una barra sobre la imagen la deja sola en su renglón, a la izquierda o a la derecha del texto, y la sube o baja de a un bloque;
  - hay un selector de tamaño de texto.
- Ruling 14: la alineación se guarda como `align="left|right"` en el `<img>` y no como `data-*` ni como `style`. Por qué: el saneo de firmas (bluemonday UGC) lo conserva y Outlook lo respeta. `EmailHTML` le suma `float` y margen, y cierra el fragmento con `clear:both`. Si me equivoqué, algún cliente muestra la imagen sin el texto alrededor.
- Ruling 15: "arriba" y "abajo" se resuelven con "Subir imagen" y "Bajar imagen", que mueven la imagen de a un bloque; además se puede arrastrar. No pasa del renglón vacío final. Si me equivoqué, hace falta otra forma de ubicarla.
- Ruling 16: los tamaños son Chico 12px, Normal (sin tamaño, 14px del correo), Grande 18px y Muy grande 24px. Si me equivoqué, se cambian en `FONT_SIZES`.

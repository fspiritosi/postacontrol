# SDD ledger — plan: docs/superpowers/plans/2026-10-31-fase-2a-multicuenta.md

Spec: docs/superpowers/specs/2026-10-30-postacontrol-design.md (leído). Rama: fase-2a (repo normal, no worktree). Base: a482470.

## Pre-flight scan

| Par / tarea | Produce ↔ consume | Hallazgo |
|---|---|---|
| T0 ↔ T9 | T0 toca Welcome.tsx; T9 abre AccountSetupDialog | sin conflicto |
| T0 ↔ T7 | main.go vs app/mailservice.go | archivos distintos, ok |
| T1 ↔ T5 | ambos suman tipos a model.go (Settings / UnifiedView) | aditivo, ok |
| T1 | migración schema_v3 tras [v1,v2] actuales | coherente |
| T2 → T3/T4 | StoreFlags/Move/ExpungeUIDs/ErrUnsupported | nombres coinciden |
| T2 | Move "solo si MOVE, IMAP4rev2 o UIDPLUS" | CapSet.Has ya resuelve rev2 → ver Ruling P1 |
| T3 → T4 | groupByFolder, MessageRefs, actions.go | coinciden |
| T4 | errors.go traduce ErrUnsupported | ok |
| T5 ↔ T6 | SyncUnified llama SyncFolder; T6 toca useConn/sync.go | sin choque de firmas |
| T1..T6 → T7 | GetSettings, SetSettings, SetSeen, SetFlagged, MoveMessages, DeleteMessages, ListUnified, SyncUnified, UpdatePassword | coinciden |
| T8 → T9/T10/T11 | ShellState{view,messageId,selectView,selectMessage}, useAllFolders, useSettings, defaultView | coinciden |
| T8 ↔ T10 | ambos tocan MessageList.tsx | secuencial, ok |
| T9 ↔ T11 | menú "Cambiar contraseña" en AccountSection lo conecta T11 | coherente |
| T8 ↔ T11 | ambos tocan events.ts | secuencial, ok |
| T9 interno | "plegadas se recuerdan" vs "con unificada arrancan plegadas" | ambigüedad → Ruling P2 |
| T0 interno | install:desktop escribe fuera del repo; el implementador no lo corre | ok |
| T6 interno | backgroundPass extraída para testear sin ticker | ok |
| T12 | manual (Fabricio) | no se despacha |

- Ruling P1: imapx.Move usa imapclient.Move cuando `Caps().Has(CapMove) || Caps().Has(CapUIDPlus)` (Has ya contempla IMAP4rev2); así el fallback COPY siempre usa UID EXPUNGE — porque imapclient.Move cae a EXPUNGE general sin UIDPLUS — costo si está mal: un servidor raro sin MOVE/UIDPLUS no podría mover (error claro, sin daño).
- Ruling P2: el estado plegado guardado en localStorage gana; "arrancan plegadas con la unificada" aplica solo a cuentas sin estado guardado — porque respeta lo que el usuario eligió — costo si está mal: retoque de UX.
- Ruling P3: Task 12 (aceptación manual) no se despacha; queda para Fabricio — costo: ninguno.

## Progreso
Task 0: dispatched (implementer sonnet, base a482470)
Task 0: review → spec ❌ (ico/icns regenerados, no los aprobados); quality Approved
Task 0: minor (deferred): build/linux/postacontrol.desktop (plantilla nfpm/AppImage) sigue con Name=postacontrol y Keywords=wails
Task 0: minor (deferred): build/brand/LEEME.txt, web/head.html y site.webmanifest dicen "CodeControl Mail" (no se embeben)
Task 0: fix round 1 dispatched (ico/icns aprobados + Exec citado), fix base 894bb9b
Task 0: fix round 1/5 (2 addressed, 0 open; commits 894bb9b..a30edec)
Task 0: minor (deferred): summary de generate:icons quedó desactualizado; Assets.car (macOS) ya no se regenera con el guard status
Task 0: complete (commits a482470..a30edec, review clean)
Task 1: dispatched (implementer sonnet), base a30edec
Task 1: minor (deferred): sin test de JSON corrupto en settings; literal id=1 repetido
Task 1: complete (commits a30edec..fb98e02, review clean)
Task 2: dispatched (implementer sonnet), base fb98e02
Task 2: review → Needs fixes. Important: imapclient.Move fallback (UIDPLUS sin MOVE) encadena COPY+STORE+UID EXPUNGE sin esperar COPY → si COPY falla se pierden los originales.
- Ruling R1 (corrige P1): con CapMove se usa imapclient.Move nativo; sin CapMove pero con UIDPLUS, el fallback se hace a mano y en orden (UID COPY .Wait() → si OK, STORE +\Deleted → UID EXPUNGE); sin ninguno, ErrUnsupported — porque P1 delegaba en un fallback de la librería que puede borrar sin copiar — costo si está mal: un camino de código más que mantener.
Task 2: minor (deferred): ExpungeUIDs deja \Deleted si falla el UID EXPUNGE (documentar); test rev1 no verifica Trash vacía
Task 2: fix round 1 dispatched (fallback secuencial + test UIDPLUS-only + uids vacíos + assert \Deleted foráneo), fix base 37ebc5a
Task 2: fix round 1/5 (4 addressed, 0 open; commits 37ebc5a..72111e3)
Task 2: minor (deferred): en el fallback UIDPLUS, si falla el UID EXPUNGE tras STORE quedan originales \Deleted + copia (duplicado, no pérdida); el comentario de Move no lo dice
Task 2: complete (commits fb98e02..72111e3, review clean)
Task 3: dispatched (implementer sonnet), base 72111e3
Task 3: minor (deferred): setFlag escribe flagged/answered del snapshot previo (pisa cambios concurrentes hasta la próxima sync); orden de carpetas aleatorio (map); MessageRefs una consulta por id; test mixed-accounts no verifica flags en servidor
Task 3: complete (commits 72111e3..0d10636, review clean)
Task 4: dispatched (implementer sonnet), base 0d10636
Task 4: minor (deferred): si falla la caché tras escribir en servidor, el destino no recibe evento/sync; borrado mixto en servidor con MOVE sin UIDPLUS deja acción a medias; selectChecked con UIDVALIDITY 0 no chequea (blindar en escrituras); sortedSubjects sin uso en actions_test.go; test mixto no verifica caché; NO del servidor en MOVE llega como goimap.Error sin traducir
Task 4: complete (commits 0d10636..978e333, review clean)
Task 5: dispatched (implementer sonnet), base 978e333
Task 5: minor (deferred): test ListByRole sin carpeta de otro rol; SyncUnified traga errores de store fuera de withConn; SyncUnified devuelve invalid con vista desconocida (no pedido, razonable)
Task 5: complete (commits 978e333..299eed9, review clean; task check corrido por el controlador: exit 0)
Task 6: dispatched (implementer sonnet), base 299eed9
Task 6: minor (deferred): una sync manual exitosa no levanta la pausa; authFailedAt no se limpia al quitar cuenta (ids AUTOINCREMENT, impacto nulo); tests de sync de B/A débiles (ListFolders desde caché)
Task 6: complete (commits 299eed9..7321619, review clean)
Task 7: dispatched (implementer haiku), base 7321619
- Ruling R2: el commit c19fcad lleva el trailer "Co-Authored-By: Claude Haiku 5.5" (modelo real del subagente) en lugar de "Opus 5.5" del plan; no se reescribe — porque la atribución veraz al modelo que hizo el cambio es más correcta que la del plan — costo si está mal: un trailer distinto en un commit.
Task 7: minor (deferred): métodos nuevos de MailService sin comentario de doc
Task 7: complete (commits 7321619..c19fcad, review clean)
Task 8: dispatched (implementer opus), base c19fcad
Task 8: minor (deferred): posible destello de vista guardada inválida antes de validar (MailShell pasa `chosen` mientras !ready); vista unificada no muestra "Sincronizando…" y puede decir vacía mientras SyncUnified corre; botón stale dice "Actualizar carpeta" en unificada; vista outbox guardada reabre vacía; sin tests de invalidación a mitad de sesión
Task 8: complete (commits c19fcad..bf78f2a, review clean)
Task 9: dispatched (implementer sonnet), base bf78f2a
Task 9: review → Approved con Important: ruling P2 sin tests (arranque plegado con unificada, pc:expanded gana)
Task 9: minor (deferred): ids viejos quedan en pc:collapsed/pc:expanded al quitar cuenta; líneas en blanco dobles; Task 11 debe pasar onChangePassword desde App.tsx
Task 9: fix round 1 dispatched (tests P2 + ausencia de "Todas las cuentas" + mocks en beforeEach), fix base a35f1fb
Task 9: fix round 1/5 (3 addressed, 0 open; commits a35f1fb..1553727)
Task 9: minor (deferred): tests de "plegado" usan espera fija de 50 ms (pragmática, sin falsos positivos según re-review)
Task 9: complete (commits bf78f2a..1553727, review clean)
Task 10: dispatched (implementer sonnet), base 1553727
Task 10: minor (deferred): sin guarda de doble clic en Borrar/Mover/estrella (segunda llamada da toast de error, sin daño); estrella de lista invalida ["message",id] (refetch extra); test de diálogo elige botón por orden DOM
Task 10: complete (commits 1553727..6b8bb67, review clean)
Task 11: dispatched (implementer sonnet), base 6b8bb67
- Ruling R3: el diálogo de contraseña vive en FolderSidebar (un solo diálogo, suscripto a onPasswordRequest) y no en cada AccountSection como decía el plan — porque evita montar un diálogo por cuenta y el despacho lo habilitó — costo si está mal: mover unas líneas.
Task 11: minor (deferred): sin test de limpieza del campo al reabrir; sin test del cableado emitter→diálogo en FolderSidebar; contrato de texto REJECTED_CREDENTIALS sin test del lado Go; setSaving/handleOpenChange frágil
Task 11: complete (commits 6b8bb67..9929ad4, review clean)
Final review: dispatched (opus), range a482470..9929ad4
Final review (opus): With fixes. Important: varias carpetas con rol trash (DetectRole por nombre a cualquier profundidad) → Borrar elige la primera alfabética, no la \Trash del servidor; confirmación "Eliminar definitivamente" puede mentir. Minors seleccionados: borrar sin confirmación si las carpetas no cargaron; toast de credenciales no se cierra tras cambiar contraseña; NO del servidor en MOVE sin traducir.
- Ruling R4: el fix wave final incluye el Important (un rol por cuenta: gana el marcado por el servidor, si no el de nombre menos profundo) + minors #2, #3, #4 + dos diferidos baratos (vista outbox guardada no se restaura vacía; sync exitosa levanta la pausa de contraseña). Queda fuera #5 (destacados en Papelera/Spam en la vista unificada: decisión de UX discutible) — costo si está mal: un fix wave algo más grande.
Final fix wave: dispatched (opus), fix base 9929ad4
Final fix wave: re-review clean (9929ad4..d141285; 6/6 addressed, sin roturas). task check exit 0 (Go ok, vitest 108/108), go test -race mail+imapx ok.
Final: minor (deferred): con dos Papeleras de primer nivel sin marca del servidor, el desempate alfabético elige "Deleted Items" antes que "Trash"; folderDepth solo cuenta el delimitador reportado; destacados de Papelera/Spam aparecen en Destacados unificado (#5 de la revisión final)

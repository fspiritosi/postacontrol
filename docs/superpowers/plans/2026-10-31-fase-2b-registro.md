# SDD ledger — plan: docs/superpowers/plans/2026-10-31-fase-2b-bandeja-del-sistema.md

Spec: docs/superpowers/specs/2026-10-30-postacontrol-design.md (leído). Rama: fase-2b (repo normal). Base: 946a5da.

## Pre-flight scan

| Par / tarea | Produce ↔ consume | Hallazgo |
|---|---|---|
| T1 → T3 | DialWatch(ctx,s,user,pass,tls,onChange), IdleWatch(ctx,mailbox), ErrUnsupported | coinciden |
| T2 → T3 | SyncFolder emite EventNewMail; T3 solo llama SyncFolder | coherente |
| T2 → T5 | model.NewMail / NewMailItem, EventNewMail data = struct | recNotifier.matching castea a map → Ruling P1 |
| T2 → T4 | InboxUnreadTotal (store + Core) | coinciden |
| 2A authpause → T3 | noteAuthFailure/authPaused/clearAuthFailure | existen (verificar nombres reales en authpause.go) |
| T4 ↔ T5 | T5 necesita la ventana para IsVisible/IsFocused y mostrarla al clic | T4 no define cómo se comparte → Ruling P2 |
| T4 ↔ T5 | ambos tocan main.go | secuencial, ok |
| T5 → T6 | Settings.disableNotifications en bindings; evento app:open-message {folderId,messageId} | coinciden |
| T4 → T6 | GetStartAtLogin/SetStartAtLogin en MailService | coinciden |
| T1 interno | rev1 sin IDLE → ErrUnsupported | StartIMAPRev1 existe |
| T3 interno | campos inyectables dialWatch, watchDebounce, watchBackoff, watcherCount | coherente |
| T4 interno | tray/hook/instancia única sin test automático | verificación manual T7 |
| T7 | manual (Fabricio) | no se despacha |

- Ruling P1: EventNewMail emite un model.NewMail (struct); los tests de core lo leen de recNotifier.events (Data.(model.NewMail)), no con matching() que castea a map — porque el plan fija el struct y el notificador de app lo consume tipado — costo si está mal: un helper de test.
- Ruling P2: app/desktop.go guarda la ventana principal en un tipo `desktop` (o variable de paquete) que SetupDesktop crea y devuelve; T5 lo reutiliza para IsVisible/IsFocused/Show/Focus — porque el plan no fija el canal y T5 lo necesita — costo si está mal: reacomodar dos funciones.
- Ruling P3: Task 7 (aceptación manual) no se despacha; queda para Fabricio.

## Progreso
Task 1: dispatched (implementer sonnet), base 946a5da
Task 1: review → Needs fixes. Important: hueco SELECT→IDLE sin documentar (y live se marca tarde); si falla el reinicio automático de IDLE, IdleWatch queda bloqueado sin IDLE.
Task 1: fix round 1 dispatched (live antes de Idle + doc "sincronizar al arrancar", rama cmd.Wait, Wait acotado, causa en ErrNetwork, tests sin sleep), fix base 198aa33
Task 1: fix round 1/5 (6 addressed, 0 open; commits 198aa33..9911efc)
Task 1: minor (deferred): la rama cmd.Wait no tiene test (go-imap solo nota el fin del IDLE en el reinicio de 28 min); un NO del servidor al reiniciar IDLE vuelve sin ErrNetwork (T3 debe reconectar ante cualquier error que no sea ctx/ErrUnsupported)
Task 1: complete (commits 946a5da..9911efc, review clean)
Task 2: dispatched (implementer sonnet), base 9911efc
- Ruling R1: firstSync también es true cuando cambió UIDVALIDITY y se reseteó la caché (desvío del brief, aceptado) — porque si no, una re-descarga completa dispararía una lluvia de notificaciones — costo si está mal: tras un reset no se avisa el correo que llegó justo en esa pasada.
Task 2: minor (deferred): sin tests de R1 ni de primera sync interrumpida; pasada no-primera interrumpida tras guardar lotes no anuncia esos correos
Task 2: complete (commits 9911efc..ebf9878, review clean)
Task 3: dispatched (implementer opus), base ebf9878
- Ruling R2: el escalón de espera del vigía se resetea solo tras una sesión que duró ≥1 min (watchStable), no en cada conexión exitosa como decía el plan — porque si no, un servidor que acepta LOGIN y corta enseguida recibiría un login cada 5 s para siempre — costo si está mal: reconexiones algo más lentas tras cortes cortos.
Task 3: review → Needs fixes. Important: la pausa por contraseña se puede saltear (SyncFolder del vigía no la mira; useConn la levanta con éxito sobre sesión vieja → vigía reintenta cada 5 s); falla de auth del vigía queda silenciosa.
- Ruling R3: useConn levanta la pausa solo si ese intento hizo un LOGIN nuevo (pool sin conexión al empezar); el vigía no sincroniza mientras la cuenta está en pausa; el vigía emite EventSyncError solo para fallas de auth, una vez por falla — porque el objetivo de la pausa es no arriesgar el bloqueo de Hostinger y que el usuario se entere — costo si está mal: la pausa puede durar hasta 30 min aunque la sesión vieja ande.
Task 3: minor (deferred): dos logins fallidos al arrancar con contraseña mala; sync post-conexión a los 2 s y no al activar IDLE; inbox renombrada → login exitoso cada 10 min; ctx de StartWatching cancelado no se recupera (documentar)
Task 3: fix round 1 dispatched, fix base 9c30f1e
Task 3: fix round 1/5 (4 addressed, 0 open; commits 9c30f1e..e4f36d0)
Task 3: minor (deferred): al vencer la pausa pueden salir 2-3 logins fallidos (pasada periódica + vigía + su sync); rechazo transitorio deja la cuenta pausada hasta 30 min aunque la sesión ande; doc de authPaused desactualizada
Task 3: complete (commits ebf9878..e4f36d0, review clean)
Task 4: dispatched (implementer sonnet), base e4f36d0
Task 4: review → Needs fixes. Critical: Ctrl+Q registrado como "ctrl+q" con KeyBinding.Add no matchea el "Ctrl+Q" que arma el handler de Linux.
Task 4: minor (deferred): carrera teórica sobre `desktop` en el callback de instancia única (hay guarda nil); autostart con build de desarrollo registra otra ruta
Task 4: fix round 1 dispatched, fix base ed3f2d7
Task 4: fix round 1/5 (1 addressed, 0 open; commits ed3f2d7..3806a21)
Task 4: complete (commits e4f36d0..3806a21, review clean)
Task 5: dispatched (implementer sonnet), base 3806a21
Task 5: minor (deferred): el servicio de notificaciones queda expuesto al webview (bindings sin uso); sin bus D-Bus de sesión la app no arranca (comportamiento de Wails); Count 1 con Items vacío daría "1 correos nuevos"; remitentes repetidos en el resumen
Task 5: complete (commits 3806a21..17ecea8, review clean)
Task 6: dispatched (implementer sonnet), base 17ecea8
Task 6: minor (deferred): interruptor de inicio al abrir sesión habilitado durante la llamada (doble clic); app:open-message se pierde si llega antes de montar MailShell (app escondida al arrancar); bloques de interruptores duplicados
Task 6: complete (commits 17ecea8..b685cd4, review clean)
Final review: dispatched (opus), range 946a5da..b685cd4
Final review (opus): With fixes. Important: el tooltip de la bandeja no existe en Linux (Wails 3.0.0 setTooltip es un stub) → el contador de no leídos nunca se ve.
- Ruling R4: se descarta el tooltip con contador en Linux: tooltip fijo "PostaControl", se quita el refresco por eventos (consulta + salto al hilo principal inútiles) y se documenta la limitación en el plan — porque no hay forma soportada de mostrarlo y SetLabel pondría texto en la barra superior — costo si está mal: falta el contador hasta que Wails lo implemente.
- Ruling R5: el fix wave final incluye además: Caps nil tras LOGIN → ErrNetwork (no ErrUnsupported); RemoveAccount frena el vigía antes de borrar el secreto; solo [AUTHENTICATIONFAILED]/[AUTHORIZATIONFAILED] o NO sin código cuentan como contraseña rechazada ([UNAVAILABLE] → red); registrar el notificador antes que MailService; escapar &,<,> en el cuerpo de la notificación; doc de authPaused; aserción de R1 en el test de UIDVALIDITY; remitentes sin repetir en el resumen — costo si está mal: un fix wave algo más grande.
Final fix wave: dispatched (opus), fix base b685cd4
Final fix wave: re-review (b685cd4..d6cc2a8): 8/8 addressed. Residual load-bearing (introducido por R5): códigos LOGIN no transitorios ([EXPIRED], [CONTACTADMIN], [ALERT], [NOPERM], desconocidos) quedan como ErrNetwork → ~30 logins fallidos/hora/cuenta (antes ~2). Recomendación: allowlist de transitorios (UNAVAILABLE, SERVERBUG, INUSE, LIMIT → red; todo lo demás → auth/pausa). Minors residuales: InboxUnreadTotal sin uso (código muerto); subtítulo (email) sin escapar en la notificación de Linux.
- Ruling R6: no hay segunda tanda automática (regla del proceso); el residual de clasificación de LOGIN se le presenta a Fabricio antes del merge con la recomendación de aplicarlo — costo si se mergea sin él: una cuenta vencida/bloqueada por el admin recibiría ~30 logins fallidos por hora.
task check exit 0 (vitest 113/113, Go ok); go test -race ./core/... ./app/ ok.

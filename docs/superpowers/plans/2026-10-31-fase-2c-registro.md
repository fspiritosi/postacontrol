# SDD ledger — plan: docs/superpowers/plans/2026-10-31-fase-2c-busqueda-y-conversaciones.md

Spec: docs/superpowers/specs/2026-10-30-postacontrol-design.md (leído). Rama: fase-2c (repo normal). Base: c448d4f.

## Pre-flight scan

| Par / tarea | Produce ↔ consume | Hallazgo |
|---|---|---|
| T1 ↔ T2 | ambos escriben schema_v4.sql (v4 todavía no publicada) | orden de columnas/ALTER → Ruling P1 |
| T1 → T3/T4 | PendingBodies, Search, index_failures | coinciden |
| T2 → T4 | ListThreads, ThreadMessages, ThreadSummary | coinciden |
| T3 → T4 | StartIndexing; app lo llama en ServiceStartup | coinciden |
| T4 → T5 | svc.Search(SearchQuery)→SearchResult{messages,pending}, svc.SearchServer(folderId,text) | coinciden |
| T4 → T6 | svc.ListThreads, svc.GetThread, Settings.threadedList | coinciden |
| T5 ↔ T6 | ambos tocan MessageList.tsx, MailShell.tsx, queries.ts | secuencial, ok |
| T1 interno | ftsQuery con `"palabra"*` | sintaxis de prefijo sobre frase citada a verificar con test (ya pedido) |
| T3 interno | fetchBody usa PEEK (verificado en imapx.FetchPart) | ok |
| T7 | manual (Fabricio) | no se despacha |

- Ruling P1: T1 crea schema_v4.sql con FTS5 + triggers + carga + index_failures; T2 agrega a ese mismo archivo la columna thread_root, su índice y su carga (v4 no salió de la rama, editarla es seguro) — porque el plan dejaba la frontera difusa — costo si está mal: reordenar dos sentencias.
- Ruling P2: Task 7 (aceptación manual) no se despacha; queda para Fabricio.

## Progreso
Task 1: dispatched (implementer sonnet), base c448d4f
Task 1: review → Needs fixes. Important: palabras solo de puntuación anulan la búsqueda (AND con frase vacía).
- Ruling R1: en la misma ronda se indexan los destinatarios por nombre/email (json_each) en lugar del JSON crudo, y el trigger de UPDATE solo reindexa si cambió algún valor indexado — porque "email"/"name" matcheaban todo y cada re-sync reescribía el cuerpo en FTS — costo si está mal: un poco más de SQL en los triggers.
Task 1: minor (deferred): migrateTo no valida el tope
Task 1: fix round 1 dispatched, fix base 1100f8c
Task 1: fix round 1/5 (4 addressed, 0 open; commits 1100f8c..42ba053)
Task 1: minor (deferred): expresión de recipients repetida 3 veces en schema_v4.sql; un JSON escrito a mano con claves faltantes dejaría NULL en el concat (no ocurre por código)
Task 1: complete (commits c448d4f..42ba053, review clean)
Task 2: dispatched (implementer sonnet), base 42ba053
Task 2: minor (deferred): cadena de respuestas sin References llegando al revés parte el hilo; búsqueda del padre sin ORDER BY si el Message-ID está en varias carpetas; ListThreads con subconsultas correlacionadas (medir; índice compuesto (folder_id, thread_root, date) si es lento); N+1 de participantes; sin tests de Message-ID entre cuentas ni de raíz estable en re-sync
Task 2: complete (commits 42ba053..b0bf3d7, review clean)
Task 3: dispatched (implementer opus), base b0bf3d7
- Ruling R2: se aceptan dos desvíos del indexador: (a) un error "stale" sincroniza esa carpeta para destrabar el indexado (si no, una Enviados recreada lo frenaría hasta abrirla); (b) un NO/BAD del servidor suma la falla y corta el lote de la cuenta con espera de 1 min (si no, una negativa general del servidor quemaría los 3 intentos de lotes enteros cada 2 s) — porque protegen contra perder indexado en masa — costo si está mal: indexado algo más lento ante errores del servidor.
Task 3: review → Needs fixes. Important: mensaje "venenoso" (corta la conexión siempre) se reintenta para siempre y bloquea la cuenta; espera fija de 1 min sin conexión/limitado.
- Ruling R3: tras un corte de red en un mensaje se prueba la conexión con NOOP: si responde, la culpa es del mensaje (suma falla y sigue); si no, es falta de red (corta con espera). Espera por cuenta 1→2→5→10 min, que se reinicia con un mensaje indexado — porque cumple "nunca para siempre" sin contar fallas cuando de verdad no hay red — costo si está mal: hasta 3 re-logins por mensaje venenoso.
Task 3: minor (deferred): hueco chequear-y-actuar de la pausa antes del lock del pool (compartido con sync periódica; arreglarlo en runOnConn chocaría con "lo que pide el usuario siempre intenta"); imágenes en línea grandes retienen el lock en segundo plano; index_failures nunca se resetea
Task 3: fix round 1 dispatched, fix base ac659a8
Task 3: fix round 1/5 (5 addressed, 0 open; commits ac659a8..2b92ad4)
Task 3: minor (deferred): sin red, cada corte da 2 avisos (fetch + sondeo) aunque con espera creciente; mensaje venenoso cuesta ~6-9 logins y 3 avisos antes de saltearse; en enlace muy inestable un mensaje sano podría acumular 3 fallas (index_failures nunca se resetea); muchos mensajes venenosos sin espera entre ellos
Task 3: complete (commits b0bf3d7..2b92ad4, review clean)
Task 4: dispatched (implementer sonnet), base 2b92ad4
Task 4: minor (deferred): err sombreado en SearchServer; sin tests del tope 200/orden ni de stale en el resync de SearchServer
Task 4: complete (commits 2b92ad4..734dbc4, review clean)
Task 5: dispatched (implementer sonnet), base 734dbc4
Task 5: minor (deferred): resultados del servidor se ocultan pero no se borran (reaparecen al volver al mismo alcance); filas del servidor no refrescan la estrella; filtro "No leídos" visible en búsqueda sin efecto; "1 correos" sin singular; la búsqueda no se refresca con correo nuevo; sin evidencia RED
Task 5: complete (commits 734dbc4..2ef6866, review clean)
Task 6: dispatched (implementer opus), base 2ef6866
Task 6: minor (deferred): remitente repetido en el mensaje expandido; marcar no leído/mover un mensaje cierra todo el hilo; si llega una respuesta nueva se expande y se marca leída sola; la negrita de hilos depende de eventos del backend; parpadeo al alternar "No leídos"
Task 6: complete (commits 2ef6866..f9e1f7c, review clean)
Final review: dispatched (opus), range c448d4f..f9e1f7c
Final review (opus): With fixes. Critical: ListThreads cuadrático (1,6–10 s/página en 10k mensajes, re-pedido cada 500 ms). Important: ["search"] nunca se invalida (acciones/indexado/pendientes); búsqueda en servidor trata varias palabras como frase.
- Ruling R4: fix wave final con: índice (folder_id, thread_root, date DESC) + test EXPLAIN + no invalidar threads en cada tick de progreso; invalidar ["search"] en acciones y folder-updated (con freno); SEARCH del servidor con una clave TEXT por palabra; NextToIndex que use su índice parcial; dedupe de páginas locales; etiqueta "Buscar en el servidor en <carpeta>"; ThreadView expande el no leído más nuevo; re-enraizar hilos cuando llega el padre tarde (caso normal con sync de lo nuevo a lo viejo); FTS contentless solo si la versión de SQLite lo soporta (si no, se deja) — costo si está mal: un fix wave más grande.
Final fix wave: dispatched (opus), fix base f9e1f7c
Final fix wave: re-review clean (f9e1f7c..e33fa31; 9/9 addressed, sin roturas). task check exit 0 (vitest 135/135), go test -race ./core/... ./app/ ok.
Final: minor (deferred): la carga de v4 no re-enraiza cadenas existentes sin References (quedan partidas hasta re-sync del intermedio); un hilo abierto que se re-enraiza muestra "ya no tiene correos"; ["threads"] se recarga en cada lote del indexador (barato con el índice); índice de hilos creado antes de la carga masiva (solo velocidad de migración)

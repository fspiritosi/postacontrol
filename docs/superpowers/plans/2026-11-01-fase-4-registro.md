# Fase 4 "Contactos": registro de decisiones y pendientes

- **Plan:** `2026-11-01-fase-4-contactos.md`.
- **Spec:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`.
- **Rama:** `fase-4`, desde `main` (0bf6731).
- **Ejecución:** con subagentes. Cada tarea tuvo una revisión propia, después una revisión de toda la rama, una ola de arreglos y una re-revisión.
- **Decisiones de Fabricio (2026-11-01):**
  - se sugiere solo a quienes les escribió;
  - los destinatarios son chips;
  - la libreta vive en los paneles.

## Commits

| Commit | Qué |
|---|---|
| 926b83a | plan |
| 595054a | store: esquema v7 (`contacts`, `contact_emails`, `recipients` con trigger y relleno) |
| 381c67f | núcleo: API de contactos, sugerencias y registro de destinatarios al enviar; fachada y bindings |
| 93870fe, 104a0c9 | redactor: chips con sugerencias; arreglo de chips con `"` o `<` sin cerrar |
| 639f3f6 | libreta de contactos (vista `contacts`) |
| fdb5a7f | lector: "Agregar a contactos" / "Ver contacto" |
| 4404660 | relleno de la migración v7 lineal (con 30k enviados: de 14,7 s a 0,17 s) |
| 563c219 | Enter/Tab no cambian una dirección completa por otra sugerencia |
| fe51f18 | sin botón de contactos en Enviados |
| 5c84ef1 | la lista de contactos no se vacía con cada tecla |
| (este) | pasar el mouse no cambia una dirección completa por otra sugerencia |

## Pre-flight

La revisión de los pares de tareas (lo que una produce y la otra consume) y de cada tarea contra sí misma no encontró conflictos.

## Decisiones (rulings)

1. **Chip con delimitadores sin cerrar (Task 3).**
   - Se mantiene el modelo del plan: el valor es el texto completo y los chips se derivan de él.
   - Al confirmar un chip se equilibran o se quitan las `"` y `<` sin par, y una coma dentro de `<…>` cierra la dirección.
   - Si me equivoqué: una dirección rara con comillas legítimas sin par se reescribe al confirmarse.
2. **Enter/Tab con una dirección completa escrita (revisión final).** Me aparto del plan, que decía "Enter o Tab con la lista abierta eligen la sugerencia activa":
   - el núcleo pone primero la coincidencia exacta del correo;
   - Enter/Tab confirman lo escrito si ya es una dirección válida distinta de la opción activa, o si la lista todavía es la de la consulta anterior;
   - una opción elegida con las flechas, o con un clic, se respeta.
   - Si me equivoqué: para elegir una sugerencia teniendo escrita una dirección completa, hay que usar las flechas o el clic.
3. **Relleno de v7.** Se armó con un agregado y `row_number()`. Si me equivoqué: en un empate de fechas, el nombre elegido puede ser otro de los que tienen la misma fecha.
4. **Arreglos chicos en la ola final.**
   - Qué se sumó:
     - un test de prefijo que discrimina;
     - "JOSE" encuentra a "José";
     - sin botón en Enviados;
     - la lista de contactos conserva los resultados mientras busca.
   - Si me equivoqué: la ola quedó apenas más grande.
5. **Pasar el mouse por la lista (resuelto después de la revisión, a pedido de Fabricio).**
   - Antes: pasar el mouse sobre una sugerencia contaba como elegirla, y Enter la tomaba aunque lo escrito fuera una dirección completa.
   - Ahora:
     - con una dirección completa escrita, el mouse solo resalta la fila y Enter deja lo escrito;
     - con texto a medio escribir, el mouse sigue eligiendo;
     - un clic o las flechas eligen siempre.

## Desvíos aceptados en la ola final

- Cuando Enter va a confirmar lo escrito, ninguna opción queda resaltada. La primera ↓ va a la opción 0.
- No hay un test de tiempo de la migración: un límite holgado no lo distinguiría de la versión lenta, y uno ajustado fallaría de vez en cuando.
- La lista de contactos no muestra "Ningún contacto coincide…" mientras se ven los resultados de la búsqueda anterior.
- En Enviados y Borradores, el botón de contactos puede aparecer un instante si las carpetas de esa cuenta todavía no están cargadas. Ya pasaba en Borradores.

## Menores diferidos

- **Conteo de `recipients`:**
  - un envío desde PostaControl cuenta dos veces: al enviar y cuando su copia entra a Enviados;
  - un reset de UIDVALIDITY en Enviados cuenta todo de nuevo.
  - Las dos cosas solo afectan el orden de las sugerencias.
- **`SuggestRecipients`:** tarda ~67 ms con 10k destinatarios, sin debounce. Carga todo y arma `fold` en cada llamada.
- **Ficha:**
  - editar una ficha y tocar otra pierde los cambios sin aviso;
  - "Correos" y "Teléfonos" son etiquetas sin asociar;
  - después de borrar puede aparecer un destello del error.
- **"Agregar a contactos":** un doble clic lo llama dos veces. No se duplica la ficha, pero puede salir un segundo toast.
- **Fila "Contactos":** va debajo de todos los árboles de cuentas, y con las 4 cuentas abiertas puede quedar fuera de la vista.
- **Chips:**
  - la × no se alcanza con Tab;
  - Esc deja la lista cerrada si se vuelve a escribir lo mismo;
  - la lista puede recortarse abajo del área con scroll;
  - Enter no tiene en cuenta la composición de un IME.
- **Detalles de código:**
  - `emailAsTyped` es redundante;
  - el orden por nombre está duplicado;
  - se ignora un error de búsqueda en el camino de `duplicate`;
  - los nombres con guion no cuentan como dos palabras.
- **Tests:**
  - falta uno del desempate por `last_used`;
  - falta uno de `SuggestRecipients` rechazado;
  - `TestSend_FailureRecordsNoRecipients` es solo una guarda.

## Pendiente

- **Task 6:** la prueba manual de Fabricio con `task install:desktop`. Sumar estos chequeos:
  - escribir completa una dirección que es parte de la de un contacto y apretar Enter: tiene que quedar la escrita;
  - en el primer arranque con la base real, que la migración no demore.

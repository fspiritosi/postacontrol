# Fase 5 "Carpetas, panel de lectura y selección múltiple": registro de decisiones y pendientes

- **Spec:** `docs/superpowers/specs/2026-11-01-carpetas-lectura-seleccion-design.md`.
- **Planes:** `2026-11-01-fase-5a-seleccion-y-panel.md` y `2026-11-01-fase-5b-carpetas.md`.
- **Rama:** `fase-5`, desde `main` (f3e1772).
- **Ejecución:** con subagentes.
  - Cada tarea tuvo su propia revisión.
  - Después hubo una revisión de toda la rama, una ola de arreglos y una re-revisión.
- **Decisiones de Fabricio (2026-11-01):**
  - **Carpetas:** se crean, se renombran, se mueven, se reordenan y se eliminan.
    - Todo pasa dentro de la misma cuenta.
    - Las especiales no se eliminan, no se renombran ni se mueven.
    - El orden vive solo en PostaControl.
    - Al eliminar una carpeta, sus correos van a la Papelera con aviso.
  - **Panel de lectura:** puede ir a la derecha, abajo o arriba. Por defecto va abajo.
  - **Selección múltiple:** según el boceto `seleccion.html`.
  - **Decisiones mías que Fabricio aprobó:**
    - una carpeta que contiene una especial queda bloqueada;
    - una conversación marcada solo toca la carpeta vista;
    - si una acción falla en una parte, se hace lo posible y se avisa por cuenta;
    - el panel arranca abajo después de actualizar.

## Commits

| Commit | Qué |
|---|---|
| f182653 | preferencia `readingPane` y selector en Configuración |
| 3a1f9ef, 770fc47 | paneles según la posición y filas de un renglón |
| 1434bb4, ef00aa7 | marcar varios correos (Ctrl, Shift, Ctrl + A, casillas) |
| adb1c32, 8e54100 | acciones sobre la selección, agrupadas por cuenta |
| f3fc1d1 | arrastrar correos a una carpeta |
| f78d80c | esquema v8 (`folders.position`) y operaciones IMAP de buzones |
| b853576, 06f60ca | crear, renombrar, mover y ordenar en el núcleo |
| e8eb070, eb1c0ff | eliminar una carpeta mandando sus correos a la Papelera |
| fee5512, a00af2f, f38e81a | menú contextual y diálogos |
| 69af41d, 16b538d | arrastrar carpetas |
| ed3d15f, 1471245, 8c18724 | ola de arreglos de la revisión final |

## Decisiones (rulings)

1. **Casilla en las filas de un renglón.** Sigue la misma regla que en las de tres renglones: se ve al pasar el mouse y queda fija si hay algo marcado. Si me equivoqué, en esas filas la casilla se comporta distinto de lo que esperabas.
2. **Nombre de una función.** La función que mueve los marcados se llama `moveMarked`, y la reusan "Mover a…" y el arrastre. Si me equivoqué, solo cambia un nombre.
3. **La casilla nunca abre un correo.** Tildar la primera deja un solo correo marcado, sin barra. Solo se abre el que queda cuando se pasa de dos o más a uno. Así no se marca un correo como leído en el servidor sin querer. Si me equivoqué, tildar una sola casilla muestra únicamente la casilla tildada.
4. **El filtro "No leídos" también suelta la selección.** Si no, se actuaría sobre correos que no se ven. Si me equivoqué, cambiar el filtro pierde la selección.
5. **Una sola revisión final para toda la rama**, después de la 5B. Si me equivoqué, los problemas de integración de la 5A recién aparecieron al final.
6. **Subcarpetas al renombrar o mover.** El núcleo no confía en que el servidor las renombre (el servidor de pruebas no lo hace) y renombra a mano las que queden con el nombre viejo. Si me equivoqué, en un servidor que sí las renombra se hace un LIST de más.
7. **Nombres con `*` o `%`.** Se rechazan, porque son comodines de LIST. Si me equivoqué, no se pueden crear carpetas con esos caracteres desde PostaControl.
8. **Ventana de IMAP al borrar.** IMAP no tiene "borrar si está vacía": un correo que llegue justo entre el último STATUS vacío y el DELETE se pierde. Lo acepté como límite del protocolo. Si me equivoqué, ese correo se pierde.
9. **"Nueva subcarpeta…" dentro de las especiales.** Está habilitado; el plan decía que en las especiales solo valían "Subir" y "Bajar". El spec solo prohíbe eliminarlas, renombrarlas o moverlas, y en Dovecot las carpetas propias viven dentro de Entrada. Si me equivoqué, algún servidor rechaza una subcarpeta y se ve su error.
10. **Tests del arrastre real.** Con el ref de `dragstart`, que es lo que usa el webview real, entraron como arreglo y no quedaron como detalle. Si me equivoqué, la ronda fue apenas más grande.
11. **El rol por nombre solo vale para el primer nivel o las hijas directas de INBOX.** Más abajo, el rol sale solo de SPECIAL-USE. Así "Clientes/Archivo" no se vuelve especial ni bloquea a su padre. Si me equivoqué, un servidor con su Papelera anidada y sin SPECIAL-USE deja de tener Papelera: borrar falla con aviso y no se pierde correo.
12. **Posición de una carpeta creada o movida.** Mira su nivel de destino: si ese nivel ya se ordenó, va al final; si nunca se ordenó, queda en su lugar alfabético. Si me equivoqué, en un nivel nunca ordenado la nueva no queda última.
13. **Arreglos chicos que entraron en la ola final:**
    - un candado por cuenta entre el refresco de carpetas y crear, renombrar, mover o eliminar (el refresco de fondo podía pisar un renombre);
    - ocultar "Primer nivel" cuando ya está en el primer nivel.
    Si me equivoqué, la ola fue algo más grande.

## Lo que quedó sin arreglar (detalles)

**Avisos y textos**
- **Borrado detenido a mitad de camino:** si el servidor deja de informar la cantidad de correos, el error dice "no se borró nada", aunque algunos correos ya estén en la Papelera. El diálogo vuelve a contar y muestra el estado real. Convendría otro texto, como "se detuvo; lo movido está en la Papelera".
- **Textos:**
  - con la carpeta padre vacía y correos en las subcarpetas, el aviso dice "Tiene 0 correos y N subcarpetas…";
  - sin subcarpetas dice "las carpetas", en plural.

**Arrastre**
- Una carpeta de origen que no es la que se ve se resalta al arrastrar encima, y soltar no hace nada.
- En WebKitGTK el recuadro puede parpadear al pasar por el ícono.
- No hay imagen con la cantidad al arrastrar varios correos.

**Carpetas**
- **Renombrar a medias:** no refresca la búsqueda, y el nombre nuevo no queda suscripto.
- **Prefijo `INBOX.`:**
  - En Dovecot, crear "Archivo" o "Spam" en el primer nivel en una cuenta sin esa carpeta especial la vuelve especial.
  - Soltar antes o después de un ítem de la raíz algo muy anidado lo deja dentro de Entrada.
- **Bandeja unificada y búsqueda:** "Mover a…" ofrece la carpeta donde ya están los correos. Con "En todo", excluye la carpeta vista.
- **Sincronización de fondo:** un borrado largo en una cuenta demora la sincronización de fondo de las demás en esa pasada.

**Panel y lista**
- Si guardaste el panel "a la derecha", al arrancar se ve un instante el modo compacto hasta que carga la configuración.

**Código y tests**
- **Duplicación:**
  - las variantes de fila repiten piezas;
  - el marcado de la casilla está en dos archivos;
  - los tres diálogos repiten el manejo de busy y error.
- **Faltan tests de:**
  - ThreadRow compacta;
  - el texto de la barra en conversaciones;
  - la rama MarkedPane de App;
  - el foco real al hacer clic;
  - `dragstart` con varias cuentas;
  - soltar conversaciones;
  - mover con hijos;
  - el prefijo NAMESPACE contra un servidor que lo tenga.
- **Tests:** un WARN de UNSUBSCRIBE aparece en la salida de `go test -v`.

## Pendiente

- **Task 6, prueba manual de Fabricio con `task install:desktop`:**
  - **Panel de lectura:** arranca abajo; se cambia a la derecha y arriba, y recuerda los tamaños en cada posición.
  - **Marcar:** con Ctrl, Shift, Ctrl + A y las casillas, en una carpeta, en la unificada, en la búsqueda y en conversaciones.
  - **Barra de acciones:** probar cada botón, borrar dentro de la Papelera, y una mezcla desde la búsqueda (pide confirmación).
  - **Arrastrar correos:** uno y varios, a una carpeta.
  - **Carpetas, en una cuenta real:**
    - crear una carpeta y una subcarpeta, y verlas en el webmail;
    - renombrar y mover una con correos;
    - reordenar con "Subir" y "Bajar" y arrastrando;
    - eliminar una con correos y subcarpetas, y comprobar en el webmail que todo quedó en la Papelera.
  - **Arrastre en WebKitGTK real:** que las carpetas se puedan arrastrar.
  - **Prefijo de Hostinger:** ver si las carpetas propias aparecen dentro de Entrada. Si es así, "Primer nivel" no se ofrece para ellas.

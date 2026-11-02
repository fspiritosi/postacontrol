# PostaControl: carpetas, posición del panel de lectura y selección múltiple

- **Fecha:** 2026-11-01.
- **Origen:** primer pedido de Fabricio después de usar la app con las 4 fases del spec original terminadas.
- **Spec base:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`. Sus restricciones y anti-objetivos siguen valiendo; este documento solo suma.
- **Bocetos aprobados:** en `.superpowers/brainstorm/` (sin commitear): `carpetas.html`, `panel-lectura.html` y `seleccion.html`.

## Intención

Fabricio ordenaba su correo en Thunderbird creando carpetas y arrastrando correos a ellas. En PostaControl todavía no puede crear carpetas, y solo puede mover un correo por vez, desde el lector. Quiere hacer lo mismo que en Thunderbird, con correos de a muchos. También quiere elegir dónde va el panel de lectura, y por defecto lo quiere abajo, como en el Thunderbird clásico.

Son tres partes que se pueden construir por separado:
1. gestión de carpetas;
2. posición del panel de lectura;
3. selección múltiple de correos.

## Objetivos

- **Carpetas:** crear, renombrar, mover y eliminar carpetas en una cuenta, y cambiar su orden en el panel.
- **Panel de lectura:** a la derecha, abajo o arriba. El valor por defecto es abajo.
- **Selección:** varios correos a la vez, con acciones sobre todos y la posibilidad de arrastrarlos a una carpeta.

## No-objetivos

- **Reglas o filtros automáticos.** Fabricio los usaba en Thunderbird, pero el spec base los deja afuera. Si se quieren, son un proyecto aparte.
- **Mover una carpeta a otra cuenta.**
- **Mover correos entre cuentas.**
- **Un orden de carpetas guardado en el servidor:** IMAP no lo tiene.

## Anti-objetivos

- **Perder correo al eliminar una carpeta.** Esto es lo más grave. Los correos se mueven a la Papelera y la carpeta se borra solo después de confirmar que esa carpeta quedó vacía en el servidor. Si cualquier paso falla, la carpeta no se borra y se avisa.
- **Que el aviso de cantidad mienta.** La caché local puede no tener todos los correos de una carpeta. La cantidad del aviso y lo que se mueve salen del servidor, no de la caché.
- **Que un clic común deje de abrir el correo.** La selección múltiple no puede volver incómodo el uso de siempre.

## Decisiones

### Carpetas (boceto `carpetas.html`)

**Menú contextual.** El clic derecho sobre una carpeta abre un menú con:
- "Nueva subcarpeta…";
- "Renombrar…";
- "Mover a…";
- "Subir" y "Bajar";
- "Eliminar…".

El menú "…" de cada cuenta, que ya existe, suma "Nueva carpeta" para crearla en el primer nivel. "Renombrar" lo propuse yo y Fabricio lo aceptó.

**Arrastrar y soltar en el panel izquierdo.**
- Uno o varios correos soltados sobre una carpeta se mueven ahí.
- Una carpeta soltada sobre otra pasa a ser su subcarpeta.
- Una carpeta soltada entre dos cambia de lugar en el orden.

**Carpetas especiales** (decisión confirmada por Fabricio). Entrada, Enviados, Borradores, Papelera, Spam y Archivo no se pueden eliminar, renombrar ni mover. Sí se pueden reordenar.

**Orden.** Lo elige Fabricio y vive solo en PostaControl. En el webmail el orden sigue siendo el del servidor.
- Una carpeta nueva va al final de su nivel.
- La forma de guardar el orden queda a criterio del builder, siempre que:
  - sobreviva a una re-sincronización;
  - acompañe a una carpeta renombrada o movida.

**Todo pasa dentro de la misma cuenta** (decisión confirmada por Fabricio).

**Eliminar.**
- **Carpeta vacía:** se confirma con "¿Eliminar 'X'?".
- **Carpeta con correos:** el aviso dice cuántos tiene, contando sus subcarpetas. Por ejemplo: "Tiene 48 correos y 1 subcarpeta ("Acme", con 7 correos). Los 55 correos van a la Papelera y las carpetas se borran del servidor."
  - Al confirmar, todos los correos de la carpeta y de sus subcarpetas se mueven a la Papelera de esa cuenta, según el servidor.
  - Después se borran las carpetas, de la más profunda hacia arriba.
- **Si la cuenta no tiene Papelera:** no se elimina una carpeta con correos y se avisa.
- **Carpeta que se puede seleccionar pero tiene subcarpetas:** se borra con ellas, como describe el aviso.
- **Carpeta con una especial adentro** (mi decisión, ver "Decisiones mías"). Pasa, por ejemplo, con el `[Gmail]` de Gmail. Esa carpeta no se elimina, no se renombra y no se mueve. El menú muestra esas opciones deshabilitadas.

### Panel de lectura (boceto `panel-lectura.html`)

- **Posiciones:** a la derecha (como hoy), abajo (por defecto, decisión de Fabricio) o arriba.
- **Dónde se cambia:** en Configuración, con un selector "Panel de lectura: Derecha / Abajo / Arriba". La elección queda guardada.
- **Filas de la lista:**
  - a la derecha, cada correo sigue ocupando tres renglones;
  - abajo y arriba, ocupa un solo renglón: remitente, asunto con la vista previa y fecha. Así entran más correos en la lista, que es más baja.
  - En un renglón siguen estando la marca de no leído, el color de la cuenta, el adjunto, la estrella y la carpeta en los resultados de búsqueda.
- **Tamaño de los paneles:** se recuerda por separado para cada posición.
- **Contactos:** la libreta y la ficha siguen la misma posición.

### Selección múltiple (boceto `seleccion.html`)

**Cómo se marca.**
- Ctrl + clic suma o saca un correo.
- Shift + clic marca todos los que hay entre el último marcado y este.
- Ctrl + A marca todos los de la lista, con el foco en la lista.
- Una casilla al pasar el mouse por la fila permite marcar sin teclado.
- Un clic común, sin teclas ni casilla, abre el correo como hoy y suelta la selección.

**Qué pasa con dos o más marcados.**
- Arriba de la lista aparece una barra con:
  - la cantidad;
  - "Marcar leído", "Marcar no leído", "Destacar", "Mover a…" y "Eliminar";
  - una ✕ para soltar la selección.
- El panel de lectura muestra "N correos seleccionados".
- Esc suelta la selección.

**Eliminar.**
- Fuera de la Papelera, manda los correos a la Papelera sin preguntar, como pasa hoy con uno solo.
- Dentro de la Papelera, el botón es "Eliminar definitivamente" y pide confirmación, igual que hoy.

**Conversaciones.** En esa vista, marcar una conversación marca todos sus correos de la carpeta que se está viendo (mi decisión). Las respuestas propias que la conversación muestra desde Enviados no se tocan: borrar una conversación de la Entrada no borra lo que mandaste.

**Bandeja unificada y búsqueda.** También se puede seleccionar.
- Cada correo se mueve o se borra en su propia cuenta.
- "Mover a…" y arrastrar a una carpeta solo se ofrecen si todos los marcados son de la misma cuenta.

**Si una parte falla** (mi decisión). Por ejemplo, en la bandeja unificada una cuenta no tiene Papelera o pierde la conexión:
- se hace lo que se puede;
- se avisa qué quedó sin hacer, por cuenta;
- los correos que quedaron sin hacer siguen marcados.

**Destacar y marcar leído o no leído** sobre una selección mezclada aplican lo mismo a todos; no alternan correo por correo.

**Arrastrar.** Los marcados se pueden arrastrar a una carpeta del panel izquierdo. Si se arrastra un correo sin marcar, se mueve solo ese.

## Decisiones mías (para que Fabricio las revise)

1. **Especiales adentro.** Una carpeta que tiene una carpeta especial en su interior no se elimina, no se renombra y no se mueve. Por qué: borrarla arrastraría a la Papelera misma o a Enviados. Si me equivoqué, esas carpetas contenedoras no se pueden manejar desde PostaControl.
2. **Conversación seleccionada.** Las acciones van solo sobre sus correos en la carpeta que se está viendo. Por qué: borrar una conversación no debería borrar lo enviado. Si me equivoqué, hay que ir a Enviados para limpiar las respuestas propias.
3. **Fallas parciales.** Se hace lo posible, se avisa por cuenta qué quedó sin hacer, y eso queda marcado. Si me equivoqué, una acción puede quedar a medias, aunque siempre con aviso.
4. **Panel abajo al actualizar.** Después de actualizar, el panel arranca abajo aunque hoy esté a la derecha, porque Fabricio pidió abajo por defecto.

## Restricciones

- Las del spec base: el núcleo sin Wails, TLS verificado, tests nunca contra cuentas reales.
- **Escrituras en el servidor:** nunca sin una acción explícita del usuario, y nunca un `EXPUNGE` sin UID.
- **Lo que ya existe:**
  - Las acciones del núcleo `SetSeen`, `SetFlagged`, `MoveMessages` y `DeleteMessages` ya reciben listas de ids. La selección múltiple debería apoyarse en ellas.
  - `imapx.Conn.Move` ya resuelve MOVE, o COPY con UID EXPUNGE.
- **Crear y renombrar:** respetan el delimitador de la cuenta (columna `folders.delimiter`).
  - Un nombre vacío no se acepta.
  - Un nombre con el delimitador adentro tampoco.
  - Un nombre que ya existe en ese nivel tampoco.
  - En todos esos casos se avisa con un mensaje claro.

## Lo que queda a criterio del builder

- **Suscripción IMAP:** si una carpeta creada se suscribe con `SUBSCRIBE`, y si se desuscribe al borrarla. Mi recomendación es que sí, para que el webmail y otros clientes la vean.
- **Caché local de una carpeta renombrada o movida:** si se conserva (mismo id, nombre nuevo) o se vuelve a bajar. Conservarla es preferible si la UIDVALIDITY no cambia.
- **Librería de arrastrar y soltar:** HTML5 nativo o una librería.
- **Dónde se guardan la posición del panel y los tamaños:** `Settings` del núcleo o `localStorage`, como el layout actual. La posición es una preferencia, y conviene que esté en `Settings`.
- **Cómo se ve el progreso al eliminar una carpeta grande.**
- **Detalles visuales**, dentro de los bocetos.

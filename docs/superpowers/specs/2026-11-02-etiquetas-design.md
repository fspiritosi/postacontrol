# PostaControl: etiquetas de correo

- **Fecha:** 2026-11-02.
- **Origen:** pedido de Fabricio después de aceptar la Fase 6.
- **Spec base:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`. Sus restricciones y anti-objetivos siguen valiendo; este documento solo suma.
- **Bocetos aprobados:** `.superpowers/brainstorm/4010542-1793647001/content/etiquetas.html` (sin commitear). Fabricio eligió la variante 1 B para la fila, la 2 A para el filtro, y aprobó la 3 entera (clic derecho, teclas y correo abierto).

## Intención

Fabricio quiere clasificar correos con etiquetas propias para encontrarlos rápido después. Sus ejemplos:

- **Para juntar a fin de mes:** "Facturas proveedores" y "Facturas enviadas". A fin de mes quiere juntar todas las facturas de proveedores sin buscarlas carpeta por carpeta.
- **Para llevar pendientes:** "Por hacer", "Reunión" y "Seguimiento". Una etiqueta de pendiente se saca cuando el asunto está resuelto.

Lee correo solo desde esta PC. No hace falta que las etiquetas se vean en el celular ni en el webmail, y no se importa nada de Thunderbird (no usaba etiquetas ahí).

## Objetivos

- **Etiquetas propias:** Fabricio crea las que quiera, cada una con nombre y color.
- **Correo etiquetado en la lista:** se reconoce de un vistazo por una línea del color de la etiqueta en el costado de la fila, y por el nombre de la etiqueta.
- **Filtro:** muestra todos los correos con una etiqueta, en cualquier carpeta.
- **Poner y sacar etiquetas:** es rápido, con uno o con varios correos a la vez.

## No-objetivos

- Las etiquetas en el panel de carpetas. *(Decisión de Fabricio: se manejan desde el filtro, no como carpetas.)*
- Importar las etiquetas de Thunderbird, o mostrar con nombre y color las marcas que puso otro programa.
- Reglas que etiqueten solas: los filtros automáticos siguen fuera de alcance, como en el spec base.
- Filtrar por varias etiquetas a la vez. *(Mi decisión: una a la vez alcanza para los usos descritos.)*

## Anti-objetivos

- **Que el filtro "pierda" correos.** Un correo etiquetado que se movió de carpeta, aunque sea a una que todavía no se abrió, tiene que aparecer en el filtro de esa etiqueta.
- **Que un correo pierda su etiqueta** al moverlo de carpeta, o cuando la caché de mensajes se vuelve a armar (por ejemplo, al quitar y volver a agregar una cuenta, o al resincronizar una carpeta). Borrar el archivo entero de la base no entra: ahí se pierden también las firmas y los contactos.
- **Que la línea o los nombres tapen** lo que hoy muestra la fila: el no leído (variante C de la Fase 6), el abierto, el marcado con Ctrl, la casilla de selección y el punto de color de la cuenta.
- **Que borrar una etiqueta toque algo más que esa etiqueta.** Ni los correos ni sus otras marcas.

## Decisiones

### 1. Las etiquetas

- **Qué es una etiqueta:** un nombre, un color y un orden.
- **Una sola lista para todas las cuentas.** "Seguimiento" es la misma etiqueta en cualquier cuenta. *(Mi decisión.)*
- **Varias por correo:** un correo puede tener varias a la vez. *(Mi decisión.)*
- **Dónde se administran:** en Configuración, en una sección "Etiquetas". Ahí se crean, se renombran, se les cambia el color, se reordenan y se borran.
  - El menú de etiquetas también tiene "Administrar etiquetas…", que abre esa sección.
- **Colores:** se eligen de una paleta fija de unos 12 colores, pensados para leerse en tema claro y oscuro. *(Mi decisión.)*
- **Nombres:**
  - el nombre no puede quedar vacío;
  - no puede repetir otro, sin distinguir mayúsculas;
  - se recorta a una longitud razonable. *(Mi decisión.)*
- **El orden importa:**
  - define el orden en los menús y en la fila;
  - define el número de tecla: la primera etiqueta es la tecla 1 y la novena la 9. Desde la décima ya no tienen tecla.
- **Renombrar o cambiar el color** se ve al instante en todos los correos, sin tocar nada en el servidor. *(Mi decisión: la marca guardada en cada correo no depende del nombre.)*
- **Borrar una etiqueta:**
  - pide confirmación y dice en cuántos correos de la caché está;
  - no borra ningún correo.

  Después la saca de todos los correos, en el servidor y en la caché *(mi decisión)*:
  - en cada cuenta conectada se buscan en todas sus carpetas los correos con esa marca (`UID SEARCH KEYWORD`) y se les saca;
  - si una cuenta no está conectada, la limpieza queda pendiente y se hace en la próxima sincronización;
  - la etiqueta desaparece de la app en el momento, sin esperar la limpieza.
- **Tope:** hasta 20 etiquetas *(mi decisión)*. Es mucho más que las cinco que nombró Fabricio, y queda lejos del límite de 26 marcas por carpeta de Dovecot.
- **Sin etiquetas de fábrica:** la app empieza sin ninguna. *(Mi decisión: Fabricio crea las suyas.)*

### 2. En la lista (variante 1 B)

- **La línea:** un correo con etiquetas tiene una línea vertical en el borde izquierdo de la fila, del color de la etiqueta.
  - Con varias etiquetas, la línea se parte en tramos, uno por color, en el orden de Configuración.
- **Los nombres:** al lado del asunto, el nombre de cada etiqueta aparece como una pastilla chica de su color, en el mismo orden.
  - Si no entran todas, se ven las primeras y un "+N". *(Mi decisión.)*
- **En los dos tamaños de fila** de hoy: tres renglones y un renglón.
- **Conversaciones:** el hilo muestra las etiquetas de todos sus correos juntas, sin repetir. *(Mi decisión.)*
  - Poner o sacar una etiqueta sobre la fila de un hilo actúa sobre todos sus correos, como hoy mover o borrar un hilo.
  - En el correo abierto, la ✕ y el botón "Etiquetas" actúan solo sobre ese correo.
- **Bandeja unificada y resultados de búsqueda:** se ven igual.
- **Convive con lo que ya hay:**
  - el fondo de no leído, abierto y marcado no cambia;
  - la casilla de selección, que aparece al pasar el mouse, no queda tapada por la línea;
  - el punto de la cuenta sigue en su lugar.

### 3. El filtro (variante 2 A)

- **Dónde está:** un botón "Etiqueta ▾" en la cabecera de la lista, al lado de "Todos / No leídos". Abre un menú con las etiquetas, cada una con su color.
- **Al elegir una:**
  - el botón muestra su color y su nombre, con una ✕ para quitar el filtro;
  - la lista pasa a mostrar los correos con esa etiqueta de **todas las carpetas**, no solo de la que estaba abierta;
  - cada fila dice en qué carpeta está, como en los resultados de búsqueda de hoy.
- **Alcance según la vista:**
  - en la bandeja unificada, junta todas las cuentas;
  - con una cuenta, solo esa. *(Decisión de Fabricio.)*
- **Se combina** con "No leídos" y con el buscador. *(Mi decisión.)*
- **Orden y acciones:**
  - la lista va por fecha, más nuevo arriba;
  - las acciones de siempre (abrir, marcar, mover, borrar, selección múltiple) funcionan igual.
- **Al cambiar de carpeta o de cuenta,** el filtro de etiqueta se quita. *(Mi decisión, igual que hoy con lo marcado.)*
- **Sin resultados:** "No hay correos con esta etiqueta."
- **De dónde salen los resultados** *(mi decisión)*:
  - el filtro se arma al instante con la caché, y funciona sin conexión;
  - cuando la app mueve un correo etiquetado, la caché lo sabe enseguida en la carpeta de destino, sin esperar a abrirla;
  - con conexión, además, se completa en segundo plano con una búsqueda en el servidor (`UID SEARCH KEYWORD` en las carpetas de la cuenta), y se bajan los encabezados de lo que falte.

  Fabricio lee correo solo desde esta PC, así que los movimientos que importan son los de la app. La búsqueda en el servidor cubre el resto.

### 4. Poner y sacar etiquetas (variante 3)

- **Clic derecho sobre una fila:** abre un menú nuevo para las filas de la lista, que hoy no tienen.
  - Trae "Etiquetas ›", con un submenú: cada etiqueta con su color, una tilde si el correo la tiene y su número de tecla; al final, "Administrar etiquetas…".
  - Sobre una fila marcada con Ctrl, actúa sobre todos los marcados.
  - El menú también trae las acciones que hoy ya existen para un correo (marcar leído/no leído, destacar, mover, borrar), si es sencillo. *(Mi decisión: el boceto las muestra; si no, solo "Etiquetas".)*
- **Teclas 1 a 9:** ponen o sacan la etiqueta de ese número.
  - Actúan sobre el correo abierto, o sobre los marcados si hay.
  - No se activan mientras se escribe en un campo, como el buscador o el redactor.
- **En el correo abierto:**
  - las etiquetas del correo aparecen arriba, con nombre y una ✕ para sacarlas;
  - un botón "Etiquetas" en la barra de acciones abre el mismo submenú.
- **Tilde, tecla o ✕:** si el correo ya tiene la etiqueta, se saca; si no, se pone.
- **Con varios correos marcados:**
  - si no todos la tienen, se pone en todos;
  - si todos la tienen, se saca de todos;
  - en el submenú, una etiqueta que tienen algunos y otros no se ve con una marca intermedia. *(Mi decisión, como en Thunderbird.)*
- **Al instante:** el cambio se ve en el momento, como hoy con leído o destacado. Si el servidor falla, se vuelve atrás y se avisa.
- **Un correo que deja de cumplir el filtro** (le sacaste la etiqueta que estás filtrando) desaparece de la lista, igual que hoy pasa con "No leídos".

### 5. Dónde se guardan

- **En cada correo, como una marca IMAP del servidor** (keyword), el mismo mecanismo que usa Thunderbird para sus etiquetas. *(Mi decisión.)* Así la etiqueta viaja con el correo cuando se mueve de carpeta y sobrevive a borrar la caché.
- **La definición de cada etiqueta** (nombre, color y orden) vive en SQLite, como las firmas y los contactos. La marca de cada correo es un identificador fijo, no el nombre, para que renombrar no toque el servidor.
- **La caché local también guarda** qué etiquetas tiene cada correo, para dibujar la lista y filtrar sin esperar al servidor.
- **Las marcas que no son nuestras se ignoran:** no se muestran ni se tocan, y tampoco se borran. Lo mismo vale para una marca `$pc_…` que no tiene etiqueta definida en esta PC.
- **Si el servidor de una cuenta no acepta marcas propias:**
  - las etiquetas se pueden poner igual, pero en esa cuenta quedan solo en esta PC;
  - la app lo avisa una sola vez por cuenta, y lo recuerda entre reinicios;
  - esas cuentas entran en el filtro de la bandeja unificada como las demás;
  - al mover un correo desde la app, la etiqueta se lleva en la caché, si se puede;
  - en ese caso, mover el correo de carpeta o borrar la caché puede hacer perder la etiqueta, y el aviso lo dice.

  Esto pasa cuando el servidor no anuncia `\*` en `PERMANENTFLAGS`. *(Mi decisión.)*
- **Escribir en el servidor:** poner o sacar una etiqueta es una acción explícita del usuario, así que entra en la regla del spec base. Nunca se tocan otros flags.

## Restricciones

- **Las del spec base:**
  - el núcleo (`core/`) sin Wails;
  - TLS verificado;
  - tests nunca contra cuentas reales;
  - contraseñas solo en el keyring;
  - escrituras en el servidor solo por acción explícita del usuario;
  - nunca un `EXPUNGE` sin UID.
- **Lo que ya existe y hay que reusar:**
  - `imapx.Conn.StoreFlags` (`core/imapx/write.go`) ya pone y saca flags en lote;
  - `Core.setFlag` (`core/mail/actions.go`) es el camino de leído y destacado: servidor primero, después la caché;
  - `refreshFlags` (`core/mail/sync.go`) relee los flags de la carpeta en cada sincronización, y el armado de filas lee los flags de cada correo nuevo. Las etiquetas tienen que entrar por esos mismos lugares, para que un cambio hecho en otro lado se refleje.
- **Hecho del código actual que importa para el filtro:** la sincronización en segundo plano solo trae la Entrada de cada cuenta; las otras carpetas se sincronizan cuando se abren. Un correo etiquetado y movido a una carpeta que no se abrió todavía no está en la caché con su UID nuevo. El filtro igual tiene que encontrarlo (ver anti-objetivos).
- **Mover desde la app** (`MOVE` o `COPY` por IMAP) conserva las marcas del correo en el servidor.
- **Dovecot y otros servidores** pueden limitar cuántas marcas distintas hay por carpeta (Dovecot con Maildir guarda hasta 26). Con las etiquetas que describió Fabricio no se llega, pero el builder tiene que manejar el rechazo del servidor como un error común: se vuelve atrás y se avisa.
- **La ruta de búsqueda en el servidor** (`imapx/search.go`) ya existe, por si el builder la usa para el filtro.

## Lo que queda a criterio del builder

- **El identificador de la marca IMAP:** tiene que ser un atom IMAP válido, fijo y propio de PostaControl, por ejemplo `$pc_<id>`. No se reusa el id de una etiqueta borrada.
- **Esquema de SQLite** para las etiquetas y su relación con los mensajes (migración v9).
- **Cómo lleva la caché la etiqueta** de un correo movido desde la app hasta su UID nuevo: con el Message-ID, con COPYUID/MOVE, o resincronizando la carpeta de destino.
- **Componentes shadcn** (context-menu, dropdown, la paleta de colores) y detalles visuales, dentro de los bocetos aprobados.
- **Orden de construcción.** Se recomienda:
  1. etiquetas en el núcleo y la caché, y la sección de Configuración;
  2. poner y sacar, con la línea y los nombres en la fila;
  3. el filtro.

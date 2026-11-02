# PostaControl: fondo del panel de lectura, no leídos, redactor en ventana propia y visor de adjuntos

- **Fecha:** 2026-11-02.
- **Origen:** pedido de Fabricio después de usar la Fase 5.
- **Spec base:** `docs/superpowers/specs/2026-10-30-postacontrol-design.md`. Sus restricciones y anti-objetivos siguen valiendo; este documento solo suma.
- **Bocetos aprobados:** `.superpowers/brainstorm/2706333-1793627275/content/no-leidos-y-visor.html` (sin commitear). Fabricio eligió la variante C para los no leídos y aprobó el visor tal como está dibujado.

## Intención

Cuatro mejoras que Fabricio notó al usar la app todos los días:

1. **Fondo propio:** poner una imagen suya, como un fondo de escritorio, en el panel de lectura cuando no hay un correo abierto.
2. **No leídos:** hoy se notan poco (negrita y un punto) y quiere verlos de un vistazo.
3. **Redactor:** hoy es un cuadro chico dentro de la app. Lo quiere en una ventana propia, como en Thunderbird, que se pueda mover, agrandar y maximizar.
4. **Visor:** ver los PDF y las imágenes adjuntas sin descargarlos.

Las cuatro son independientes y se pueden construir y entregar por separado.

## Objetivos

- **Fondo:** una imagen elegida por Fabricio se ve en el panel de lectura cuando no hay un correo abierto.
- **No leídos:** una fila no leída se distingue de una leída de un vistazo, en tema claro y oscuro.
- **Redactor:** "Redactar", "Responder", "Responder a todos", "Reenviar" y abrir un borrador o un correo de la bandeja de salida abren una ventana del sistema aparte, que se puede mover, estirar y maximizar.
- **Visor:** un clic en un PDF o una imagen adjunta lo muestra dentro de la app, sin guardarlo en el disco.

## No-objetivos

- Fondos en otros paneles (lista, carpetas) o detrás de un correo abierto.
- Previsualizar adjuntos de Office, zip u otros tipos: siguen como hoy (se guardan con el diálogo).
- Editar o anotar PDF o imágenes en el visor.
- Que el redactor siga existiendo como cuadro dentro de la app: lo reemplaza la ventana propia.

## Anti-objetivos

- **Perder lo que se está escribiendo.** Cerrar la ventana del redactor, ya sea con la ✕ de la ventana, con Esc o cerrando la app, se comporta como cerrar el redactor hoy: guarda el borrador si corresponde, o pide confirmación para descartar. Nunca se pierde texto en silencio.
- **Que la ventana principal se trabe** mientras hay un redactor abierto. Las dos se usan a la vez.
- **Que el visor ejecute algo del adjunto.** Un PDF o una imagen que llegó por correo es contenido no confiable: se muestra, pero no corre scripts, no abre enlaces solo y no carga nada de internet.
- **Que el fondo tape o confunda la información.** "N correos seleccionados" y los textos del panel vacío se tienen que poder leer sobre cualquier imagen.

## Decisiones

### 1. Fondo del panel de lectura

- **Dónde se elige:** en Configuración, con "Elegir imagen…" (diálogo nativo de archivos) y "Quitar". Muestra una miniatura de la imagen actual.
- **Copia propia:** la app guarda una copia de la imagen en su carpeta de datos. Sigue apareciendo aunque se borre o mueva el original. Al cambiarla o quitarla, la copia anterior se borra. *(Mi decisión, aprobada por Fabricio en la conversación.)*
- **Dónde se ve:** solo en el panel de lectura, cuando no hay un correo abierto. *(Decisión de Fabricio.)* Eso incluye:
  - el panel vacío, sin nada seleccionado;
  - el panel con varios correos marcados, detrás de "N correos seleccionados";
  - Contactos sin una ficha abierta.

  *(Estos dos últimos son mi decisión.)* Con un correo o una ficha abiertos, no se ve.
- **Cómo se ajusta:** llena el panel sin deformarse (`cover`), centrada. Si la proporción no coincide, se recortan los bordes. *(Mi decisión.)*
- **Legibilidad:** el texto que va encima ("N correos seleccionados", el aviso del panel vacío) va sobre un recuadro semitransparente para que se lea con cualquier imagen.
- **Formatos:** los que muestra el webview: JPG, PNG, WebP y GIF. Otro tipo se rechaza con un aviso claro.
- **Tamaño:** una imagen de más de 20 MB se rechaza con un aviso. Por debajo de eso se guarda tal cual, sin redimensionar. *(Mi decisión.)*

### 2. Correos no leídos (variante C del boceto)

- **Fila no leída:** fondo azul intermedio (el azul del logo, `#3598CC`, con transparencia), el remitente en azul y en negrita, el asunto en semi-negrita, y el punto de hoy.
  - En tema oscuro, el fondo es un poco más intenso y el azul del remitente más claro, para que contraste.
- **Vale en todas las listas:** filas de tres renglones y de un renglón, vista por conversaciones (una conversación con algo sin leer), bandeja unificada y resultados de búsqueda.
- **Se distingue** del correo abierto (gris) y del marcado con Ctrl (gris más oscuro). Si un no leído está marcado o abierto, gana el color de marcado o abierto en el fondo, y el remitente sigue en azul.
- **Carpetas en el panel izquierdo:** no cambian.

### 3. Redactor en ventana propia

- **Ventana del sistema aparte.** Se puede mover, estirar, maximizar y minimizar. Tiene un tamaño inicial razonable y un tamaño mínimo para que la barra de formato y los botones entren.
- **Título:** el asunto, o "Nuevo correo", "Responder" o "Reenviar" mientras no haya asunto.
- **Varios a la vez:** se pueden tener varias ventanas de redactor abiertas. *(Mi decisión, aceptada por Fabricio.)*
  - Si se abre un borrador o un correo de la bandeja de salida que ya está abierto en otra ventana, esa ventana se trae adelante en vez de abrir otra.
  - La identidad es el id del borrador local o del correo de salida.
  - Un redactor nuevo, que todavía no guardó nada, no tiene identidad y no se deduplica. *(Mi decisión.)*
- **Varios redactores de la misma cuenta** pueden autoguardar a la vez. El núcleo tiene que tolerarlo sin que un borrador pise a otro. El builder verifica cómo está hoy y agrega el bloqueo que falte.
- **Lo que carga la ventana:** cada ventana trae por su cuenta lo que necesita: cuentas, firmas y contactos para las sugerencias. Usa el mismo tema claro u oscuro que la principal y lo sigue si cambia. *(Mi decisión.)*
- **Todo lo que hace hoy el redactor sigue funcionando en la ventana:**
  - destinatarios con sugerencias;
  - formato, menú "/", tablas e imágenes;
  - firmas;
  - adjuntos desde el disco, incluido arrastrar archivos a la ventana;
  - autoguardado del borrador;
  - enviar, y los avisos de envío.
- **Cerrar:** la ✕ de la ventana y Esc hacen lo mismo que cerrar el redactor hoy. Guardan el borrador si corresponde, o preguntan "¿Descartar este correo?". Si se elige "Cancelar", la ventana queda abierta.
- **Cerrar la app** (desde la bandeja del sistema) con redactores abiertos: cada uno guarda su borrador **sin preguntar** antes de que la app termine; al salir de la app nunca se descarta nada. Un redactor vacío, en el que no se escribió nada, se cierra sin guardar. Si alguno no puede guardar, o no responde en unos 10 segundos, la app no se cierra: esa ventana se trae adelante con el aviso, y lo demás queda como estaba. *(Mi decisión.)*
- **Cierre asíncrono:** guardar puede tardar porque va por IMAP. Por eso el hook `WindowClosing` cancela siempre el cierre, y es la ventana la que se cierra sola cuando terminó de guardar o se confirmó descartar.
- **Después de enviar,** la ventana se cierra sola. La ventana principal se entera, como hoy, para actualizar la bandeja de salida y Enviados.
- **La ventana principal** queda usable todo el tiempo y ya no muestra el cuadro del redactor.

### 4. Visor de adjuntos (boceto aprobado)

- **Qué se abre en el visor:** PDF e imágenes (JPG, PNG, GIF, WebP). Un clic en uno de esos adjuntos lo abre. El resto sigue con un clic que guarda, como hoy.
- **Cómo se ve:** encima de la app, ocupando casi toda la ventana, con el fondo oscurecido. Esc o ✕ lo cierran.
- **Barra de arriba:** nombre del archivo, zoom (− / porcentaje / +), "Descargar" (el diálogo de guardar de hoy) y ✕. En los PDF también "pág. N de M".
- **PDF:** las páginas una debajo de otra, con scroll y zoom.
- **Imagen:** ajustada a la ventana. El zoom la agranda, y agrandada se puede mover.
- **Tira de abajo:** todos los adjuntos del correo, para saltar de uno a otro. Las flechas ← → pasan entre los que se pueden ver. Los que no se previsualizan aparecen apagados, y un clic los guarda.
- **Sin escribir en el disco:** el contenido va del núcleo al visor en memoria.
- **Seguridad:** el PDF se dibuja con pdf.js con los scripts del documento apagados. Los enlaces del PDF no se abren solos, y nada carga recursos de internet.
- **Archivos grandes:** mientras se baja el adjunto se muestra "Cargando…". Si falla, el error y "Descargar" siguen disponibles.
- **Tope:** un adjunto de más de 50 MB no se previsualiza. El visor muestra "Es muy grande para verlo acá" y "Descargar". *(Mi decisión.)*

## Restricciones

- **Las del spec base:**
  - el núcleo (`core/`) sin Wails;
  - TLS verificado;
  - tests nunca contra cuentas reales;
  - contraseñas solo en el keyring;
  - escrituras en el servidor solo por acción explícita del usuario o el guardado de borradores ya existente;
  - nunca un `EXPUNGE` sin UID.
- **Lo que ya existe y hay que reusar:**
  - `Core.attachmentBytes` (`core/mail/read.go`) ya baja un adjunto por IMAP. El visor debería apoyarse ahí, con un método que devuelva los bytes en vez de guardarlos.
  - `SaveAttachment` sigue siendo el "Descargar".
  - El redactor (`features/compose/Composer.tsx`, `ComposerContext.tsx`) y su autoguardado (`useDraftAutosave`) se reusan dentro de la ventana nueva.
  - `ComposerInit` es serializable y puede viajar a la ventana nueva.
- **Wails v3.0.0 en Linux (GTK4 / WebKitGTK 6):**
  - Se pueden abrir más ventanas con `app.Window.NewWithOptions`, en otra URL del mismo frontend.
  - Los servicios ligados funcionan en todas las ventanas.
  - El cierre se intercepta con el hook `WindowClosing` y `Cancel()`, como ya hace `app/desktop.go` con la principal.
  - Cada ventana es un contexto JS aparte, con sus propios providers y su propio QueryClient.
  - **Ojo:** `app.Window.Current()`, que usa `SaveAttachment`, devuelve la ventana con foco, no la que llamó.
- **La app es de instancia única.** Las ventanas del redactor son de la misma instancia.
- **WebKitGTK no trae visor de PDF.** Hace falta pdf.js (`pdfjs-dist`), con su worker servido desde los assets embebidos. El tipo MIME de `.mjs` tiene que ser JavaScript, o el worker no arranca.

## Lo que queda a criterio del builder

- **Imagen de fondo:** dónde se guarda la copia y cómo llega al frontend (data URI por binding, o un handler de assets).
- **Ventana del redactor:**
  - cómo recibe su `ComposerInit`: una entrada HTML aparte o una ruta en el mismo `index.html`;
  - cómo la principal se entera de los envíos y borradores;
  - los tamaños inicial y mínimo.
- **pdf.js:** directo o vía `react-pdf`.
- **Orden de construcción:** se recomienda primero no leídos (chico), después fondo y visor, y al final el redactor en ventana propia (el más grande y riesgoso).
- **Detalles visuales**, dentro de los bocetos aprobados.

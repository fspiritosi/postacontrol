# Fase 6A "No leídos, fondo y visor": registro de decisiones y pendientes

- **Spec:** `docs/superpowers/specs/2026-11-02-fondo-noleidos-redactor-visor-design.md`, secciones 1, 2 y 4.
- **Plan:** `2026-11-02-fase-6a-noleidos-fondo-visor.md`.
- **Rama:** `fase-6`, desde `main` (3854d12).
- **Ejecución:** con subagentes, con revisión por tarea. Después hubo una revisión de toda la rama, una ola de arreglos y una re-revisión.
- **Estado:** `task check` en verde en 14ab260 (480 tests del frontend, todos los paquetes Go).

## Commits

| Commit | Qué |
|---|---|
| ea187f5 | no leídos con fondo azul y remitente azul (variante C) |
| 3459764, 797b3b3 | fondo: copia en la carpeta de datos, `Settings.Background`, candado de configuración |
| cbf8d1c | fondo en el panel de lectura vacío y en Configuración |
| b27a40f | bytes de un adjunto en memoria, tope de 50 MB |
| dce8fe4 | visor: diálogo, zoom, tira de adjuntos, imágenes |
| 5fe4c04, 5fec23d | PDF con pdf.js 6.4.299, dibujando solo las páginas cerca de la vista |
| b109a26, 14ab260 | ola de arreglos de la revisión final |

## Decisiones (rulings)

1. **Fondo sin pasar por `SetSettings`.** `SetBackground` y `ClearBackground` guardan directo en el store, porque `SetSettings` ignora a propósito el `Background` que llega: así cambiar otra opción no puede borrar el fondo. Si me equivoqué, nada visible; es solo un camino interno.
2. **Decodificadores wasm locales para PDF escaneados.** Son jbig2 y openjpeg, unos 350 kB servidos desde la app. Sin ellos, los PDF escaneados salían en blanco. Si me equivoqué, el binario pesa eso de más.
3. **Las páginas del PDF se dibujan solo cerca de la vista.** El plan decía dibujarlas todas al abrir, pero un PDF escaneado largo agotaba la memoria de WebKit y mostraba páginas en blanco. Si me equivoqué, PdfView tiene algo más de código.
4. **pdfjs-dist 6.4.299, instalado con `npm --before=2026-10-26`.** npm 11 ignora `minimum-release-age` de `frontend/.npmrc`, y sin esa opción se instalaba una versión de un día. `isEvalSupported` no existe en pdf.js 6; se verificó que no hay `eval` en el paquete. Si me equivoqué, la versión es apenas más vieja.
5. **Tope del visor según el tamaño decodificado.** El servidor informa el tamaño en base64; se estima ×3/4 y después de bajar se vuelve a chequear el tamaño exacto. Si me equivoqué, un adjunto que no es base64 de casi 50 MB se baja y recién ahí se rechaza.
6. **El fondo también aparece en la Bandeja de salida vacía**, porque es "el panel de lectura sin un correo abierto". Si me equivoqué, se ve en un lugar más de lo esperado.
7. **Los adjuntos se reconocen por el MIME y, si es genérico, por la extensión.** SVG nunca se previsualiza, porque puede traer scripts. Si me equivoqué, un SVG adjunto se descarga en vez de verse.

## Lo que quedó sin arreglar (detalles)

- **Visor:**
  - arrastrar la barra de scroll con zoom puede pelear con el arrastre para mover la imagen;
  - ←/→ cambian de adjunto aunque haya zoom (el spec lo pide así);
  - un PDF de cientos de páginas mide todas antes de mostrar la primera;
  - pdf.js 6 necesita un WebKitGTK muy nuevo (el 2.52.6 instalado sirve). Si alguna vez corre en otro equipo con uno viejo, usar `pdfjs-dist/legacy`.
- **Fondo:** al cambiar de imagen se ve un instante el panel sin fondo. El barrido de huérfanos usa `filepath.Glob` sobre la carpeta de datos, así que una carpeta con `*`, `?` o `[` en el nombre no se barre.
- **Texto:** el filtro del selector de imágenes no muestra extensiones mezcladas como `.Jpg`; sí muestra `.jpg` y `.JPG`.
- **Tests y código:**
  - mocks que se filtran entre tests en `MessageList.test.tsx`;
  - indentación del ternario de la fila cómoda;
  - nombre de un test de Configuración que promete más de lo que chequea;
  - `GetMessage` se llama dos veces en `AttachmentData`.

## Pendiente

- **Task 7, prueba manual de Fabricio con `pkill -x postacontrol; task install:desktop`:**
  - **No leídos:** fondo y remitente azules en tema claro y oscuro, con el panel abajo y a la derecha, y en conversaciones.
  - **Fondo:**
    - elegir una foto `.JPG` en Configuración;
    - que se vea con el panel vacío, con varios marcados, en Contactos sin ficha y en la Bandeja de salida;
    - que se vaya al abrir un correo, y que "Quitar" lo saque.
  - **Visor:**
    - un PDF largo: scroll, "pág. N de M", y zoom en la página 10 o más, sin perder la página y sin que quede scroll vacío al volver a "Ajustar";
    - una imagen grande: zoom y moverla arrastrando;
    - un Excel en la tira se descarga;
    - "Descargar" desde el visor;
    - Esc cierra.
  - **El worker de pdf.js en la app instalada:** si un PDF se queda en "Cargando…", el worker no arrancó.
- **La Fase 6B (redactor en ventana propia)** sigue en esta misma rama.

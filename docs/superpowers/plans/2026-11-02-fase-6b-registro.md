# Fase 6B "Redactor en ventana propia": registro de decisiones y pendientes

- **Spec:** `docs/superpowers/specs/2026-11-02-fondo-noleidos-redactor-visor-design.md`, sección 3.
- **Plan:** `2026-11-02-fase-6b-redactor-ventana.md`.
- **Rama:** `fase-6`, después de la 6A (registro 19068b0).
- **Ejecución:** con subagentes, con revisión por tarea. Después hubo una revisión de la 6B entera, una ola de arreglos y una re-revisión.
- **Estado:** `task check` en verde en 84547a2 (526 tests del frontend, todos los paquetes Go).

## Commits

| Commit | Qué |
|---|---|
| 6503a95 | registro de ventanas en Go, salida que espera los borradores, Ctrl+Q y "Salir" por ahí |
| bc6f588 | "Redactar" y compañía abren una ventana en vez del diálogo |
| 463d644, 0dbadaa | la ventana del redactor: cierre, guardado al salir, título, avisos en la principal, foco inicial |
| 412893c, 84547a2 | ola de arreglos de la revisión final |

## Decisiones (rulings)

1. **Eventos por ventana con `DispatchWailsEvent`.** El plan decía `win.EmitEvent`, pero en Wails 3.0.0 eso le llega a todas las ventanas: los archivos soltados en un redactor se adjuntaban en todos. Si me equivoqué, nada; es el camino que garantiza una sola ventana.
2. **"Ventana cerrada" = el listener de `WindowClosing`.** Wails 3.0.0 no tiene un evento `WindowClosed`. Ese listener corre solo cuando ningún hook canceló el cierre, que es la misma señal que usa Wails para destruir la ventana. Si me equivoqué, el registro guarda una ventana de más hasta que se cierre la app.
3. **Los avisos de la ventana del redactor se ven en la principal.** Son "Guardado en Borradores.", el error al descartar, "Enviando correo…" y el resultado del envío. Antes se mostraban en la ventana que se estaba cerrando y nadie los veía. Si me equivoqué, hay un método ligado más (`ComposeNotice`).
4. **Un borrador borrado en otro lado mientras está abierto se vuelve a crear** si seguís escribiendo. Vuelve a aparecer en Borradores con otro Message-ID, para no perder el texto. Si me equivoqué, un borrador que borraste a propósito reaparece.
5. **Un borrador borrado en otro lado sin cambios después** se cierra sin aviso. Lo que ya estaba escrito y guardado se va con el borrador que borraste. La revisión lo marcó como decisión de producto, no como defecto. Si me equivoqué, al cerrar esa ventana se pierde texto que todavía se veía en pantalla. El arreglo es corto: marcarlo como "sin guardar" y volver a crearlo.
6. **Envío fallido con la principal escondida:** aparece una notificación del escritorio, "No se pudo enviar el correo", aunque estén apagadas las notificaciones de correo nuevo. Al hacer clic se abre la ventana principal. Si me equivoqué, te llega un aviso que habías silenciado (pero ese ajuste es de correo nuevo).
7. **Los diálogos ("Adjuntar", "Insertar imagen", "Guardar adjunto", "Elegir fondo") se abren sobre la ventana que los pidió**, no sobre la que tiene el foco. Si me equivoqué, nada visible.
8. **Ctrl+Q también sale desde una ventana de redactor**, y pasa por la misma espera de borradores. Si me equivoqué, Ctrl+Q en el redactor cierra la app.

## Lo que quedó sin arreglar (detalles)

- **Salida:**
  - una ventana abierta en el mismo instante en que salís, o que todavía no terminó de cargar, puede quedar afuera de la ronda; si su página no escuchó el pedido a tiempo, la salida espera 10 s y queda bloqueada con el aviso;
  - no hay tests de "cerrar una ventana durante la salida cuenta como ok" ni de "un segundo Ctrl+Q se ignora".
- **Envíos:** si se borró el borrador en otro lado y mandás antes de que se guarde algo, el envío lleva el id viejo. Pasaba igual antes.
- **Notificaciones:** si la app se cierra en medio de un envío, puede aparecer la notificación de envío fallido mientras se cierra.
- **Código:**
  - `emitToWindow` siempre devuelve false;
  - `SetupFileDrop` tiene un parámetro sin uso;
  - `SetKey` no evita una clave repetida;
  - `ComposeSendStarted` ignora su error.

## Pendiente

- **Task 4, prueba manual de Fabricio con `pkill -x postacontrol; task install:desktop`:**
  - **Abrir:** "Redactar", "Responder" y "Reenviar" abren una ventana aparte que se mueve, se estira y se maximiza. El foco arranca en "Para", o en el cuerpo si ya hay destinatarios.
  - **Varios a la vez:** dos redactores abiertos, y abrir un borrador que ya está abierto trae su ventana adelante.
  - **Cerrar:**
    - la ✕ con texto guarda, y en la principal aparece "Guardado en Borradores.";
    - un reintento de la Bandeja de salida con cambios pregunta "¿Descartar este correo?", y "Cancelar" deja la ventana abierta;
    - Esc hace lo mismo que la ✕.
  - **Adjuntar:** arrastrar archivos a una de dos ventanas los adjunta solo ahí. "Adjuntar" abre el diálogo sobre esa ventana.
  - **Enviar:** la ventana se cierra, y en la principal aparece "Enviando correo…" y después "Correo enviado".
  - **Salir con redactores abiertos**, con Ctrl+Q o desde la bandeja:
    - los borradores se guardan y la app se cierra;
    - al volver a abrirla, están en Borradores.
  - **Borrado en otro lado:** con un borrador abierto, borrarlo en Borradores desde la principal, seguir escribiendo y cerrar. Tiene que volver a aparecer con el texto.
  - **Tema:** cambiar el tema del sistema; la ventana del redactor lo sigue.
- **Fase 6A, Task 7:** ver `2026-11-02-fase-6a-registro.md`.

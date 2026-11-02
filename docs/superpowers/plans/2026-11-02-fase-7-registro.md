# Fase 7 "Etiquetas": registro de decisiones y pendientes

- **Spec:** `docs/superpowers/specs/2026-11-02-etiquetas-design.md`.
- **Plan:** `2026-11-02-fase-7-etiquetas.md`.
- **Rama:** `fase-7`, desde `main` (6c6ef5f).
- **Ejecución:** con subagentes y revisión por tarea. Al final hubo una revisión de toda la rama, una ola de arreglos y una re-revisión.
- **Estado:** `task check` en verde en d9a4aac (600 tests del frontend, todos los paquetes Go).

## Commits

| Commit | Qué |
|---|---|
| 74d3198 | esquema v9 y store de etiquetas |
| 9f637dc | alta, cambios, orden y borrado de etiquetas en el núcleo, con limpieza en el servidor |
| 8a9a1f9, 533a627 | poner y sacar etiquetas como keywords IMAP, cuentas sin keywords y mover |
| 7651eb5, e3cea8c | filtro por etiqueta en el núcleo, completado en el servidor, y métodos del servicio |
| b1e2793 | sección Etiquetas en Configuración |
| c74a985 | línea y nombres en la fila, y etiquetas en el correo abierto |
| 86b0257, 831fd59 | clic derecho, teclas 1 a 9 y barra de selección |
| adc7713 | filtro por etiqueta en la lista |
| 8536938, ca41b41, d9a4aac | ola de arreglos de la revisión final |

## Decisiones (rulings)

1. **Búsqueda con el filtro de etiqueta puesto:** la búsqueda conserva su propio alcance ("Esta carpeta" / "Todas") y solo le suma la etiqueta. Si me equivoqué, con "Todas" puede traer correos de otras cuentas aunque estés mirando una sola.
2. **Clic derecho:** el menú trae Etiquetas, leído / no leído y destacar. Mover y Borrar quedan afuera, porque necesitan elegir destino y confirmar, y ya están en la barra de selección y en el lector. Si me equivoqué, para mover o borrar hay que ir a la barra o al lector.
3. **Conversaciones con el filtro puesto:** no se agrupan, porque el filtro cruza carpetas y las conversaciones son por carpeta, igual que pasa hoy con la búsqueda. Si me equivoqué, los correos de un mismo hilo se ven sueltos.
4. **Un solo plan de 9 tareas:** las partes dependían una de otra. Si me equivoqué, no hubo una entrega parcial para mergear antes del final.
5. **Marcas del servidor en tres estados:**
   - el servidor anuncia que acepta marcas propias: se guardan ahí;
   - anuncia la lista de marcas y no incluye las propias: la cuenta pasa a "solo en esta PC";
   - no anuncia nada: se intenta guardar igual, y un rechazo es un error común, sin pasar la cuenta a "solo en esta PC".

   Así lo pide el estándar IMAP, y Hostinger no está verificado. Si me equivoqué, un servidor que no anuncia nada y acepta la marca sin guardarla de verdad la pierde sin avisar.
6. **Dos arreglos chicos en la Task 3:**
   - las marcas se comparan sin distinguir mayúsculas;
   - si falla guardar el traslado de etiquetas al mover, igual se limpia la caché.

   Si me equivoqué, nada.
7. **Revisión final:** una sola ola de arreglos con todo lo encontrado. Si me equivoqué, nada.

## Lo que quedó sin arreglar (detalles)

- **Barra de selección con conversaciones marcadas:** después de poner o sacar una etiqueta, el menú de la barra no se actualiza. Al reabrirlo, las tildes quedan viejas y un segundo clic repite la acción en vez de revertirla. Se arregla soltando y volviendo a marcar.
- **Carreras con la sincronización:**
  - una sincronización que leyó las marcas justo antes de que pusieras una etiqueta puede deshacerla en la caché hasta la próxima, igual que pasa con leído y destacado;
  - el filtro, al completarse en el servidor, la vuelve a poner.
- **Cuentas "solo en esta PC":**
  - entre el MOVE y el guardado del traslado hay una ventana chica en la que un sync de la carpeta destino puede traer el correo sin la etiqueta;
  - una sola carpeta que no acepta marcas pasa la cuenta entera a "solo en esta PC", para siempre.
- **Borrar una etiqueta:** si una carpeta falla siempre al abrirse, la limpieza de esa etiqueta queda pendiente y se reintenta en cada pasada.
- **Filtro:**
  - cada vez que elegís una etiqueta, la app revisa todas las carpetas de la cuenta (o de todas las cuentas, en la unificada), sin evitar repeticiones;
  - con búsqueda activa también se dispara, aunque no haga falta;
  - con el filtro puesto, la barra de selección no ofrece como destino de "Mover" la carpeta abierta.
- **Visual:** en listas angostas, dos pastillas en la fila de un renglón pueden achicar el asunto.
- **Tests y código:**
  - faltan algunos tests de UI (la marca intermedia del menú, teclas sobre filas filtradas);
  - hay una carrera de bajo riesgo entre el tope de 20 etiquetas y el alta;
  - los `<li>` de las filas quedaron sin reindentar;
  - el clic derecho sobre un hilo pide el hilo dos veces.

## Pendiente

**Task 9, prueba manual de Fabricio** con `pkill -x postacontrol; task install:desktop`:

- **Crear:** crear "Facturas proveedores", "Por hacer" y "Seguimiento" con colores, reordenarlas y ver el número de cada una.
- **Etiquetar:** etiquetar un correo con el clic derecho, con la tecla 1 y desde el correo abierto. Se ven la línea y el nombre en la lista.
- **Correo abierto:**
  - sacar una etiqueta con la ✕ y poner otra desde el botón "Etiquetas";
  - las pastillas del correo abierto cambian en el momento;
  - un segundo clic revierte el primero.
- **Dos etiquetas:** un correo con dos muestra la línea partida y los dos nombres, en tema claro y oscuro.
- **Mover:** mover un correo etiquetado a otra carpeta y filtrar por esa etiqueta desde la Entrada. Aparece, con el nombre de la carpeta nueva, y el contador de esa carpeta no cambia.
- **Bandeja unificada:** el filtro trae correos de todas las cuentas.
- **Sacar con el filtro puesto:** sacar "Por hacer" con la ✕ del correo abierto mientras filtrás por "Por hacer". El correo sale de la lista.
- **Conversaciones:** etiquetar un correo de Enviados de una conversación. La fila del hilo muestra la etiqueta y se puede sacar desde ahí.
- **Borrar una etiqueta con correos:** los correos siguen y ya no la tienen.
- **Hostinger:** si aparece el aviso "no guarda etiquetas" en alguna cuenta, anotar cuál.

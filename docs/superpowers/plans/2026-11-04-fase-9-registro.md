# Fase 9 "Configuración por secciones y Reglas": registro de decisiones y pendientes

- **Pedido:** ordenar Configuración con tabs o barra lateral (estaba todo mezclado) y agregar Reglas, por ejemplo para mover sola a una carpeta todo lo que llega de un remitente. Referencia: Thunderbird.
- **Sin diseño ni plan escritos:** Fabricio pidió arrancar directo. Las decisiones de abajo son mías; revisarlas.
- **Rama:** `main`.
- **Estado:** `task check` en verde en 929fe4c, con GTK4 y WebKitGTK 6 (641 tests del frontend, todos los paquetes Go).

## Commits

| Commit | Qué |
|---|---|
| e14842d | reglas en el núcleo (esquema v11), servicio y bindings |
| e5e6f64 | Configuración con barra lateral, sección Reglas, editor y «Crear regla…» desde un correo |
| 929fe4c | al abrir Configuración, el foco va a la sección activa |

## Decisiones

1. **Barra lateral, no tabs.** Secciones: General, Lectura, Redacción, Cuentas, Etiquetas, Reglas (Thunderbird también usa una lista a la izquierda). Sin sección pedida, vuelve a la última que se vio.
2. **Reglas por cuenta**, en orden, como la lista de filtros de cada cuenta de Thunderbird. Mover solo puede ir a una carpeta de la misma cuenta.
3. **Condiciones:** De, Para, Cc, Para o Cc, Asunto; contiene, no contiene, es, no es, empieza con, termina con; «todas» o «alguna». Sin cuerpo del correo: la caché solo tiene cabeceras y traer cada cuerpo haría lenta la sincronización.
   - En direcciones, «contiene» mira «Nombre <correo>»; «es», «empieza con» y «termina con» miran el nombre o el correo.
   - Las negativas se cumplen si ninguna dirección cumple la positiva.
4. **Acciones:** mover a una carpeta, marcar como leído, destacar, poner etiqueta, mandar a la Papelera y «no aplicar las reglas que siguen». Mover o borrar también cortan las reglas siguientes para ese correo. Primero se aplican marcas y etiquetas y después se mueve, así viajan con el correo.
5. **Cuándo corren:**
   - Solas, al sincronizar la Entrada, sobre lo nuevo **no leído**. Así un correo que se devuelve a mano a la Entrada no se vuelve a mover (Thunderbird hace lo mismo con IMAP).
   - Nunca en la primera sincronización de una cuenta, igual que el aviso de correo nuevo.
   - Antes del aviso de correo nuevo: lo que una regla movió o marcó como leído no se avisa.
   - A mano, con «Aplicar ahora» sobre una carpeta, también sobre lo leído. En la Papelera se ignora «mandar a la Papelera», porque ahí borrar es definitivo.
6. **Carpeta o etiqueta borrada:** la regla guarda el id. Si ya no existe, esa acción se saltea (queda en el registro) y la lista muestra un aviso. Renombrar o mover la carpeta desde la app conserva el id; renombrarla desde el webmail no lo conserva.
7. **Errores al aplicar las reglas solas:** solo se registran. El correo queda en la Entrada y no se reintenta.
8. **«Crear regla…»:** en el lector y con clic derecho en una fila. Abre Configuración → Reglas con el editor completado («De es <correo>»), como «Crear filtro desde el mensaje» de Thunderbird.

## Pendientes

- **`task` y `wails3`** no están instalados en el sistema de la PC nueva: en esta sesión se usaron copias temporales. Con otra versión de npm, `task check` reescribe `frontend/package-lock.json` (solo los campos `libc`): no hay que commitear ese cambio.
- **Probar en la app real:** crear una regla desde un correo, mandarse un correo que la cumpla y ver que llegue a la carpeta. La UI solo se miró en una página de prueba con datos falsos.
- No implementado (Thunderbird sí lo tiene): copiar a carpeta, reenviar, responder, condiciones por fecha, tamaño o cuerpo, y reglas al enviar.

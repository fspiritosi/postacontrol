# Fase 8 "Cuentas, enlaces de correo y corrector": registro de decisiones y pendientes

- **Diseño:** `docs/superpowers/specs/2026-11-02-cuentas-enlaces-corrector-design.md`.
- **Plan:** `2026-11-02-fase-8-cuentas-enlaces-corrector.md`.
- **Rama:** `fase-8`, desde `main` (4d4ca77).
- **Ejecución:** con subagentes y revisión por tarea. Al final hubo una revisión de toda la rama, una ola de arreglos y una re-revisión.
- **Estado:** `task check` en verde en def8092 (628 tests del frontend, todos los paquetes Go).

## Commits

| Commit | Qué |
|---|---|
| 6d27715 | corrector ortográfico en español e inglés (WebKitGTK), con interruptor en Configuración |
| 9600499 | editar una cuenta en el núcleo: prueba de conexión, llavero, borrado de caché al cambiar de buzón, época por cuenta |
| 9e8eea4 | diálogo "Editar cuenta…" |
| 0ea16d2 | orden de las cuentas en el núcleo (esquema v10) |
| d436a23 | sección "Cuentas" en Configuración (subir, bajar, editar) |
| 978d8b3, 2cc43d8 | los `mailto:` abren el redactor de PostaControl; `OpenExternal` solo http/https |
| 6e29330 | enlaces cliqueables en correos de texto plano |
| ac51ae5 | PostaControl como cliente de correo del sistema |
| 9afb192, 8311521, def8092 | ola de arreglos de la revisión final |

## Decisiones (rulings)

1. **Paso 5 de la Task 1:** una línea suelta del plan ("Agregá spellcheck=false donde dice 'Archivos'") se ignoró. Los campos sin corrector son los que lista la tarea. Si me equivoqué, falta un `spellcheck=false` en algún campo chico.
2. **Test del caso completo de `mailto:` (Task 6):** el implementador no tenía el caso del plan y escribió uno propio. Lo traté como un hueco real y lo mandé a una ronda de arreglos, que agregó el caso exacto. Si me equivoqué, nada.
3. **Ventana entre la época y el borrado de la caché:** queda como está.
   - El arreglo que se había sugerido (hacer el borrado bajo el lock de la época) se traba.
   - La ventana es inocua: lo escrito antes del borrado se borra, lo de después falla por clave foránea, y la sincronización que sigue reemplaza la lista de carpetas.

   Si me equivoqué, una lista de carpetas del buzón viejo puede quedar hasta la próxima sincronización.
4. **Decisiones del diseño:** en el diseño, sección "Decisiones tomadas en la revisión del diseño":
   - los `mailto:` se abren desde Go, sin pasar por la ventana principal;
   - la cuenta remitente es la del correo abierto o, si no hay, la primera;
   - el cambio de buzón es cambio de host IMAP o de usuario.

## Revisión final: lo que se arregló

- **Arranque:** un `mailto:` de una segunda instancia que llegaba mientras la app arrancaba la hacía caer. Ahora espera a que termine de arrancar.
- **El `+` en los `mailto:`:** se leía como espacio en la consulta (`cc=a+tag@x.com`), y la dirección se perdía sin aviso. Ahora es literal.
- **Llavero:**
  - si no se puede leer la contraseña vieja, la edición no sigue, para no dejar la contraseña nueva a medias;
  - si falla el borrado del secreto viejo, queda en el log (sin dirección ni contraseña).
- **Diálogo:** un cambio solo de mayúsculas en el usuario ahora muestra el aviso de cambio de buzón, como hace el núcleo.
- **Texto plano:** el clic del medio en un enlace abre el navegador y no navega la ventana.
- **Compilación con `-tags gtk3`:** el corrector no intenta enlazar WebKitGTK 6.

## Lo que quedó sin arreglar (detalles)

- **Editar cuenta:**
  - si falla el borrado de la caché después de guardar, el vigía queda frenado y la caché vieja convive con el buzón nuevo hasta que la verificación de UIDVALIDITY la corrija;
  - la etiqueta "Probando…" del diálogo cuenta el cambio de dirección como cambio de conexión (solo cosmético);
  - faltan tests de "cambio solo de conexión", de la etiqueta "Probando…" y del cambio de contraseña desde el diálogo;
  - el diálogo no tiene animación de cierre.
- **Ordenar:**
  - la validación y la escritura de `ReorderAccounts` no van en la misma transacción;
  - dos clics muy seguidos en Subir/Bajar pueden pisarse, igual que en Etiquetas.
- **Enlaces:**
  - `https://` o `www.` solos se enlazan;
  - `foo.www.bar.com` enlaza `www.bar.com`;
  - en `ftp://user@host.com` se enlaza la parte del correo;
  - el clic del medio dentro de un correo HTML no se maneja (el iframe no deja abrir ventanas);
  - `OpenMailtoArgs` y `SpellcheckEnabled` también se pueden llamar desde la interfaz (inofensivo).
- **Instancia única:** una segunda instancia sin `mailto:` mientras la app arranca no muestra la ventana (carrera previa a esta fase).
- **Corrector:** cada cambio de Configuración lo vuelve a aplicar y deja una línea en el log.
- **Tests:** algunos bordes sin test:
  - linkify: recorte de `!?;:`, `))`;
  - `MailtoArgs`: orden con varios, 8192 bytes justos;
  - la rama de segunda instancia de `main.go`.

## Pendiente

**Prueba manual de Fabricio** con `pkill -x postacontrol; task install:desktop`:

- **Corrector:**
  - escribir "ola komo andas" y "helo wrld" en un correo nuevo: aparecen subrayadas;
  - el clic derecho ofrece correcciones;
  - "Aprender ortografía" deja de marcar la palabra, también después de reiniciar;
  - apagarlo en Configuración saca los subrayados.
- **Editar cuenta:**
  - desde el menú de la cuenta o desde Configuración → Cuentas → Editar, cambiar nombre y color: se ven en la barra lateral y en "De", y el próximo correo sale con el nombre nuevo;
  - poner una contraseña mala: avisa y no cambia nada.
- **Ordenar:** mover una cuenta en Configuración → Cuentas: cambia el orden en la barra lateral y en "De", y sigue igual después de reiniciar.
- **Enlaces:**
  - un `mailto:` en un correo abre el redactor de PostaControl con la cuenta de ese correo;
  - en un correo de texto plano los enlaces se pueden cliquear (web al navegador, correo al redactor).
- **Programa predeterminado:**
  - después de `task install:desktop`, `xdg-mime query default x-scheme-handler/mailto` dice `ar.com.codecontrol.postacontrol.desktop`;
  - un `mailto:` en Chrome abre el redactor, con la app abierta (no aparece la ventana principal, solo el redactor) y con la app cerrada.

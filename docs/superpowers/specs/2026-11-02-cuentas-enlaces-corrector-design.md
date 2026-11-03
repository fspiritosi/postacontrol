# Fase 8: editar y ordenar cuentas, enlaces de correo y corrector ortográfico

Origen: las notas de mejoras de Fabricio (`~/desarrollo_local/notas_desarrollo/postaControl.md`), los cuatro puntos sin "OK" salvo el de "dejarla lista para usar", que se saltea por ahora.

## Por qué

PostaControl reemplaza a Thunderbird/Betterbird. Hoy:

- una cuenta no se puede corregir después de agregarla;
- las cuentas aparecen en el orden en que se agregaron;
- un clic en un enlace `mailto:` de un correo abre Betterbird, que es el cliente predeterminado del sistema;
- en los correos de texto plano los enlaces no se pueden cliquear;
- el redactor no marca errores de ortografía.

Fabricio quiere tener estas funciones, no arreglar una molestia puntual. Lo que ya existe tiene que seguir andando igual.

## Objetivos

1. **Editar una cuenta:** todo lo que se puede editar, sin borrar y volver a agregar.
2. **Ordenar las cuentas:** el orden se elige y se respeta en toda la app.
3. **Enlaces de correo:**
   - un `mailto:`, venga de un correo abierto o de otra aplicación del sistema, abre el redactor de PostaControl;
   - PostaControl queda como cliente de correo predeterminado del sistema;
   - en los correos de texto plano las direcciones web y de correo son cliqueables.
4. **Corrector ortográfico:**
   - corrige mientras escribís, en español (Argentina) e inglés a la vez;
   - el clic derecho muestra sugerencias, "Aprender" e "Ignorar";
   - viene prendido y se apaga desde Configuración.

## No objetivos

- La nota "que la aplicación quede lista para que se mantenga siempre disponible": Fabricio la saltea en esta fase.
- Elegir otros idiomas del corrector o administrar el diccionario personal desde la app.
- Revisar la ortografía al enviar (no frena ni avisa).
- Cambiar el comportamiento de los enlaces web: siguen abriendo el navegador del sistema.
- Registrar PostaControl para otros tipos (archivos `.eml`, etc.).

## Anti-objetivos (sería un fracaso aunque funcione)

- **Credenciales:**
  - la contraseña nunca se escribe en un log ni sale del llavero del sistema;
  - el TLS siempre se verifica.
- **Escrituras en el servidor:**
  - editar una cuenta, reordenarla o abrir un `mailto:` no escribe nada en el servidor;
  - descartar la caché local nunca borra correos del servidor.
- **Enviar:** un `mailto:` nunca envía solo; el correo sale únicamente cuando Fabricio toca Enviar.
- **Adjuntos en `mailto:`:** un `mailto:` no puede adjuntar archivos. Los parámetros `attach`/`attachment` y cualquier cabecera que no sea `to`, `cc`, `bcc`, `subject` o `body` se ignoran.
- **Ediciones fallidas:** una edición que falla la prueba de conexión no deja la cuenta a medio cambiar. No cambia nada y lo dice.
- **Duplicados:** abrir un `mailto:` con la app ya abierta no levanta una segunda copia de la app.

## Restricciones del proyecto (vigentes)

- **Núcleo:** `core/` nunca importa Wails. Lo específico de la ventana (WebKitGTK, instancia única) vive en `app/` o en `main.go`.
- **Tests:** nunca contra cuentas reales; se usan los servidores de `core/testenv`.
- **Servidor:** nunca `EXPUNGE` sin UID.
- **Formato:** sin prettier. Copy en español rioplatense (voseo).
- **Bindings:** se regeneran con `wails3 generate bindings -clean=true -ts -i`.
- **Esquema:** la migración nueva es la v10 (`core/store/migrate.go`, que hoy llega a v9).

## 1. Editar una cuenta

### Qué se edita

Todo lo de `model.Account`, salvo el id:

- nombre para mostrar;
- dirección de correo;
- color;
- usuario;
- contraseña;
- servidor IMAP: host, puerto y seguridad;
- servidor SMTP: host, puerto y seguridad;
- guardar copia de lo enviado;
- firma predeterminada.

### Dónde

- **Menú de la cuenta en la barra lateral:** un ítem "Editar cuenta…" junto a "Cambiar contraseña", etc.
- **Configuración:** una sección nueva "Cuentas" (ver §2), con un botón "Editar" por cuenta.

Las dos entradas abren el mismo diálogo, con los mismos campos que el alta (`AccountSetupDialog`) ya cargados.

### Campos del diálogo

- **Contraseña:** vacía significa "conservar la actual"; el campo lo aclara.
- **Color:** se elige de `LABEL_COLORS` (`frontend/src/lib/labels.ts`). Si el color actual de la cuenta no está en esa paleta (las cuentas viejas pueden tener `#9333ea`), se muestra igual como seleccionado.

### Reglas en el núcleo

Un método nuevo, `UpdateAccount(ctx, id, in model.AccountInput) (model.Account, error)`, reemplaza `SetSaveSentCopy` y `SetAccountSignature` en el diálogo. Esos dos métodos pueden quedar para los atajos que ya los usan.

- **Validación:**
  - la dirección se valida como en el alta y no puede repetir la de otra cuenta;
  - un nombre vacío se completa como en el alta (la parte antes de `@`);
  - un usuario vacío pasa a ser la dirección;
  - el color tiene que ser `#rrggbb`.
- **Cambio de conexión:** cambiaron el usuario, cualquier dato de IMAP o de SMTP, o vino una contraseña nueva.
  - Antes de guardar se prueba la conexión con `TestAccount`. Usa la contraseña nueva o, si vino vacía, la guardada en el llavero.
  - Si la prueba falla, no cambia nada y devuelve el error de la prueba.
- **Llavero:** guarda la contraseña bajo la dirección de correo.
  - Si cambia la dirección, la contraseña (nueva o la de siempre) se guarda bajo la dirección nueva y se borra la vieja, pero recién cuando el guardado en la base salió bien.
  - Si falla el guardado en la base, el llavero vuelve a como estaba.
- **Cambio de buzón:** cambió el host IMAP (sin distinguir mayúsculas) o el usuario. Puede ser otro buzón, así que:
  - se frena el vigía y se descarta la conexión;
  - se borra la caché local de esa cuenta: carpetas y todo lo que cuelga de ellas, el espejo de etiquetas y las limpiezas o traslados de etiquetas pendientes de esa cuenta;
  - se vuelve a "admite etiquetas en el servidor" (`labels_local = 0`) y se sincroniza de cero.
  - Las definiciones de etiquetas son globales y no se tocan.
  - Los borradores locales y la bandeja de salida de la cuenta se conservan.
  - El diálogo avisa antes de guardar: "Cambiaste el servidor o el usuario: se van a volver a bajar los correos de esta cuenta. En el servidor no se borra nada."
- **Cambio de conexión sin cambio de buzón** (puerto, seguridad, SMTP o contraseña): se descarta la conexión del pool, se levanta la pausa por contraseña rechazada (como `UpdatePassword`), se reinicia el vigía y se sincroniza en segundo plano.
- **Siempre:** se emite `EventAccountsChanged`, más el evento de carpetas si se borró la caché.

## 2. Ordenar las cuentas

- **Base de datos:** la migración v10 agrega `accounts.position`. Las existentes quedan en el orden de hoy (por id) y una cuenta nueva va al final.
- **Orden de lectura:** `ListAccounts` ordena por `position` y después por `id`. Todo lo que lista cuentas respeta ese orden: la barra lateral, el selector "De" del redactor, la sección "Cuentas" y la elección de la cuenta para un `mailto:`.
- **Reordenar:** un método nuevo, `ReorderAccounts(ctx, ids []int64) error`.
  - Recibe todas las cuentas en el orden nuevo. Si no es exactamente el conjunto de cuentas existentes, devuelve un error de validación y no cambia nada.
  - Emite `EventAccountsChanged`.
- **Interfaz:** la sección "Cuentas" de Configuración lista cada cuenta con su color, nombre y dirección, los botones "Subir" y "Bajar" (como `LabelsSection`) y "Editar".
- El orden se guarda solo en esta PC; no hay nada que guardar en el servidor.

## 3. Enlaces de correo

### Un solo camino para los `mailto:` (en Go)

Todo `mailto:` se interpreta y se abre en Go, en `app/`, sin depender de la ventana principal. La ventana principal puede estar oculta en la bandeja, o puede no haber cargado todavía en un arranque desde Chrome.

- **Método nuevo:** `MailService.OpenMailto(link string, accountID int64) error`.
  - Interpreta el enlace según el RFC 6068, con `net/url`:
    - los destinatarios del path van separados por coma;
    - de la consulta se toman `to`, `cc`, `bcc`, `subject` y `body`, decodificados por porcentaje;
    - el resto se ignora (ver anti-objetivos).
  - Arma el JSON de arranque del redactor, el mismo formato que `ComposerInit` / `parseInit` del frontend:
    - `reply: null`, todas las listas vacías salvo los destinatarios;
    - `signatureId` = la firma predeterminada de la cuenta;
    - el cuerpo es texto plano: se escapa con `html.EscapeString` y los saltos de línea pasan a `<br>`.
  - Lo abre con el `ComposeWindows.Open` que ya existe, sin clave, así cada enlace abre su propio redactor.
  - Un `mailto:` sin nada válido abre un redactor vacío.
  - Un enlace que no es `mailto:` devuelve error de validación.
- **Test que ata los dos lados:** un test de Go verifica que el JSON trae todas las claves que `parseInit` exige, para que los dos formatos no se separen.
- **Cuenta remitente:**
  - `accountID` es la cuenta del correo abierto desde el que se hizo clic;
  - 0, o una cuenta que ya no existe, significa la primera cuenta según el orden de §2;
  - sin cuentas, devuelve un error de usuario: "Primero agregá una cuenta."

### `mailto:` dentro de la app

- **Lector:** un clic en un `mailto:` del correo abierto, con HTML o en texto plano, llama a `OpenMailto(href, cuenta del correo)` y no a `OpenExternal`.
- **`OpenExternal`:** pasa a aceptar solo `http` y `https`, para que nada termine en Betterbird por error.

### PostaControl como cliente predeterminado del sistema

- **Instalación** (`task install:desktop`, `Taskfile.yml`):
  - el `.desktop` instalado declara `MimeType=x-scheme-handler/mailto;` y pasa a `Exec="…/postacontrol" %u`;
  - después corre `xdg-mime default ar.com.codecontrol.postacontrol.desktop x-scheme-handler/mailto`;
  - se agrega lo mismo a `build/linux/postacontrol.desktop`.
- **Recepción** (`main.go` / `app/`):
  - se aceptan solo los argumentos que empiezan con `mailto:` (sin distinguir mayúsculas) y miden hasta 8 KB;
  - cada uno pasa por `OpenMailto(arg, 0)`;
  - un error se registra con `slog.Warn`, sin el enlace completo;
  - **app ya abierta:** el `OnSecondInstanceLaunch` existente toma los `mailto:` de `SecondInstanceData.Args`. Si hay alguno, no muestra la ventana principal, solo abre el redactor; si no hay, sigue mostrando la ventana como hoy;
  - **app cerrada:** los `mailto:` de `os.Args` del arranque se abren cuando la app terminó de arrancar (evento de aplicación iniciada de Wails), porque las ventanas no se pueden crear antes.
- **Arranque oculto:** un arranque con `mailto:` no es un arranque oculto. `StartHidden` devuelve `false` si hay un `mailto:` entre los argumentos.
- **Verificación al construir:**
  - en Linux, `SecondInstanceData.Args` trae el argumento `%u`;
  - la clave de instancia única (`ar.com.codecontrol.postacontrol`) es la misma con que arranca el `.desktop`.

### Enlaces en correos de texto plano

- **Qué se vuelve cliqueable:** en la vista de texto plano (`SafeHtmlFrame`, rama sin HTML) se convierten en enlaces `http://…`, `https://…`, `www.…` (como `https://www.…`) y las direcciones de correo (como `mailto:`).
- **Cómo se arma:** con elementos de React, nunca con `innerHTML`.
- **Clic:** web va por `OpenExternal` (navegador); correo abre el redactor.
- **Bordes:** la puntuación final pegada (`.`, `,`, `)`, `;`, `:`) no forma parte del enlace.

## 4. Corrector ortográfico

### Cómo se activa

- **Dónde:** se prende en el motor de la ventana (WebKitGTK 6.0), no en JavaScript.
- **Código:** archivo cgo en `app/`, solo Linux (con un archivo vacío para el resto), usando `webkit_web_context_get_default()`:
  - `webkit_web_context_set_spell_checking_languages` con `es_AR` y `en_US`;
  - `webkit_web_context_set_spell_checking_enabled`.
- **Diccionarios:** en el equipo de Fabricio están instalados `hunspell-es` (con `es_AR`), `en_US` y `enchant-2`.
- **Hilo:** las llamadas corren en el hilo de GTK.
  - Se hacen al arrancar la app, antes o justo después de crear las ventanas.
  - Se repiten cuando cambia la opción en Configuración, sin reiniciar.
- **Alcance:** vale para todas las ventanas (principal y redactores), si comparten el contexto por defecto.

### Qué ve Fabricio

- **Subrayado:** las palabras mal escritas se subrayan en el cuerpo (TipTap) y en el asunto.
- **Sin revisión:** los campos de destinatarios y los de configuración (servidores, usuarios) llevan `spellcheck="false"`.
- **Clic derecho sobre una palabra marcada:** muestra el menú nativo de WebKit con:
  - las sugerencias;
  - "Aprender ortografía": diccionario personal de enchant, en esta PC;
  - "Ignorar ortografía".
- **Menú de Wails:** si el runtime de Wails tapa el menú nativo en esos campos, hay que habilitar el menú por defecto ahí (`--default-contextmenu`) sin cambiar los menús propios de la app.

### Configuración

- **Campo nuevo:** `model.Settings.DisableSpellcheck bool`. El cero significa prendido, como `DisableNotifications`.
- **En la pantalla:** un interruptor "Corregir ortografía al escribir" en Configuración.

### Riesgo y verificación

Es lo más incierto de la fase. Por eso va primero.

- **Prueba técnica:** después de activarlo, `webkit_web_context_get_spell_checking_languages` tiene que devolver los dos idiomas. WebKit solo devuelve los que pudo cargar.
- **Si no devuelve los dos idiomas:** se registra con `slog.Warn` y la app sigue sin corrector.
- **Prueba previa (2026-11-02):** un programa mínimo en C con WebKitGTK 2.52.6 activó el corrector y devolvió `es_AR` y `en_US`. Wails crea sus webviews con `g_object_new(WEBKIT_TYPE_WEB_VIEW, "user-content-manager", …)`, o sea con el contexto por defecto, y su runtime muestra el menú nativo sobre los campos editables (`--default-contextmenu: auto`). La puerta técnica está pasada; queda la confirmación visual.
- **Prueba final:** la confirmación visual (subrayado y menú) queda para la prueba manual de Fabricio.

## Orden de trabajo

1. Corrector ortográfico, empezando por la prueba técnica.
2. Editar cuenta (núcleo, servicio, diálogo).
3. Ordenar cuentas (v10, núcleo, sección "Cuentas").
4. Enlaces: `mailto:` en el lector y en el redactor, enlaces en texto plano, cliente predeterminado del sistema.

## Decisiones tomadas en la revisión del diseño (de Claude)

- **Ventana oculta:** los `mailto:` se abren desde Go y no desde la ventana principal, porque esa ventana puede estar oculta o sin cargar.
- **Cuenta remitente:** la del correo abierto. Si no hay correo abierto (enlaces de otras apps), la primera cuenta.
- **Cambio de buzón:** se compara el usuario guardado con el usuario efectivo nuevo (si viene vacío, pasa a ser la dirección) y el host IMAP. Cambiar solo la dirección, dejando el usuario como estaba, no borra la caché.
- **SMTP:** `TestAccount` ya prueba IMAP y SMTP; no hace falta extenderlo.
- **Corrector apagado:** el menú nativo de WebKit ya no muestra opciones de ortografía; no hay que hacer nada más.

## Lo que decide quien construye

- La forma exacta del diálogo en modo edición (componente nuevo o `AccountSetupDialog` con un modo).
- La expresión para detectar enlaces en texto plano, dentro de las reglas de §3.
- La forma exacta del struct de Go que replica `ComposerInit`.
- Dónde y cómo se engancha la llamada de GTK al arranque.
- Cómo se renombran o se reutilizan `SetSaveSentCopy` y `SetAccountSignature`.

## Prueba manual (Fabricio, `pkill -x postacontrol; task install:desktop`)

- **Corrector:**
  - escribir "ola komo andas" y "helo wrld" en un correo nuevo: aparecen subrayadas;
  - el clic derecho ofrece correcciones;
  - "Aprender" deja de marcar la palabra, aun después de reiniciar;
  - apagarlo en Configuración saca los subrayados.
- **Editar:**
  - cambiar nombre y color: se ven en la barra lateral y en "De", y el próximo correo enviado sale con el nombre nuevo;
  - cambiar a una contraseña mala: avisa y no cambia nada.
- **Ordenar:** mover una cuenta: el orden cambia en la barra lateral y en "De", y sigue igual después de reiniciar.
- **Enlaces:**
  - un `mailto:` en un correo abre el redactor de PostaControl;
  - en Chrome, un enlace `mailto:` abre el redactor de PostaControl con la app abierta, y también con la app cerrada;
  - en un correo de texto plano los enlaces se pueden cliquear.

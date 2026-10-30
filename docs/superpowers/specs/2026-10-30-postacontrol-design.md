# PostaControl — cliente de correo de escritorio

**Fecha:** 2026-10-30
**Estado:** revisado por un builder independiente, pendiente de aprobación de Fabricio
**Nombre:** PostaControl (carpeta y módulo Go: `postacontrol`).

## Intención y por qué

Fabricio usa Thunderbird con 4 cuentas IMAP (3 de Hostinger y 1 de otro proveedor con configuración similar) y en la práctica solo recibe y envía correo. Quiere un cliente propio por dos razones:

1. **Aprender construyéndolo.** La experiencia de haber hecho un cliente de correo completo es un objetivo en sí mismo.
2. **Tener una base propia que crezca con el uso,** como pasó con Taskapp: una vez que la app existe, se le agregan las funciones que el uso diario muestra que hacen falta. Una integración futura con Taskapp es una idea posible, no un requisito.

Lo que más le molesta de Thunderbird es **la interfaz**, que siente anticuada. Quiere algo moderno con la estética de shadcn/ui. En redacción toma como referencia a Outlook (más amigable que Thunderbird) y a Notion (menú con "/" para insertar elementos, como el que ya usa en Taskapp).

Es de **uso personal**: un solo usuario, en su PC con Ubuntu.

## Criterio de éxito

**La Fase 1 está terminada cuando puede configurar una cuenta real, recibir un correo y enviar uno correctamente desde la app.** "Correctamente" significa que el correo enviado llega bien formado al destinatario (incluido HTML y texto plano), queda guardado en Enviados del servidor y se ve igual en otros clientes (Thunderbird o webmail de Hostinger).

## Objetivos, no-objetivos y anti-objetivos

**Objetivos (en fases, ver abajo):** leer, buscar, responder, redactar y enviar correo en las 4 cuentas; libreta de contactos con fichas y autocompletado; firmas globales; redactor moderno.

**No-objetivos (por ahora):** calendario, chat, RSS, cifrado PGP/S/MIME, filtros/reglas automáticas, importar datos de Thunderbird (ni correos ni contactos), multiusuario, distribución a terceros, Windows/macOS (Wails lo permite, pero no se prueba ni se empaqueta).

**Anti-objetivos (cosas que lo harían un fracaso aunque funcione):**
- Que se vea o se sienta como Thunderbird con otra pintura: la interfaz moderna es el motivo principal del proyecto.
- Perder o corromper correo en el servidor. La app nunca debe borrar ni mover mensajes en el servidor salvo por una acción explícita del usuario.
- Guardar contraseñas en texto plano.
- Acoplar la lógica de correo a Wails de manera que después no se pueda exponer como API.

## Decisiones tomadas

### Stack

| Pieza | Decisión | Origen |
|---|---|---|
| Shell de escritorio | **Wails v3.0.0** (estable desde 2026-10-26) | Decisión de Fabricio, después de verificar el estado de v3 |
| Backend | **Go**, con toolchain fijada en `go.mod` como `toolchain go1.27.3` (incluye el fix de `crypto/x509` recomendado por la release de Wails) | Decisión de Fabricio (Go) / mía (versión) |
| Frontend | **React + TypeScript + Vite + Tailwind + shadcn/ui** | Decisión de Fabricio (shadcn); el resto es su stack habitual (Taskapp) |
| Editor de correo | **TipTap**, el mismo que usa Taskapp (`@tiptap/react`, `starter-kit`, `suggestion`) | Decisión de Fabricio |
| Almacenamiento local | **SQLite** | Mía, en línea con Mostrador |
| Contraseñas | **Llavero del sistema** (Secret Service / GNOME Keyring) | Mía |
| Plataforma objetivo | Ubuntu 26.04, GTK4 + WebKitGTK 6.0 (ya instalados, `wails3 doctor` sin problemas) | Entorno existente |

El entorno ya está preparado: `wails3` v3.0.0 instalado en `~/go/bin`, y Go 1.27.3 disponible vía `GOTOOLCHAIN` (el Go del sistema es 1.26.8 del paquete de Ubuntu y no se toca).

### Arquitectura: núcleo separado de la cáscara

Decisión de Fabricio: el núcleo Go tiene que poder usarse por separado si hace falta.

- **Núcleo de correo** (paquete Go independiente, sin imports de Wails): cuentas, sincronización IMAP, envío SMTP, parseo MIME, almacenamiento SQLite, contactos y firmas. Expone una API Go con servicios bien definidos.
- **Cáscara Wails:** registra servicios Wails finos que delegan en el núcleo, y traduce los eventos del núcleo (correo nuevo, progreso de sincronización, errores) a eventos Wails para el frontend.
- **Motivo:** el día que haga falta una app móvil o un acceso remoto, el mismo núcleo se expone como API HTTP en el VPS de Fabricio (Wails v3 incluye "server builds", que pueden servir para eso) y solo se escribe otra interfaz. Fabricio prefiere infraestructura self-hosted.
- **Fuente de verdad:** para el correo es el servidor IMAP, y SQLite es una caché local para velocidad, búsqueda y uso offline. Para contactos y firmas **SQLite es la fuente de verdad**, porque Hostinger (hasta donde sabemos) no ofrece CardDAV. Sincronizar contactos entre dispositivos queda como problema futuro, fuera de alcance.

### Pantalla principal (layout "A", elegido por Fabricio en mockups)

Tres paneles redimensionables al estilo del ejemplo "Mail" de shadcn:

1. **Izquierda:** carpetas, cuentas (cada una con un color y un contador de no leídos) y acceso a la Libreta de contactos (desde la Fase 4).
2. **Centro:** lista de correos con buscador y filtro "Todos / No leídos". Cada fila muestra remitente, asunto, extracto, hora, indicador de adjuntos y el color de la cuenta.
3. **Derecha:** correo abierto, con acciones (Responder, Responder a todos, Reenviar, Borrar, …), encabezado, cuerpo y adjuntos.

**Bandeja unificada, opcional (decisión de Fabricio):** es una opción en Configuración.
- **Activada:** arriba a la izquierda aparece "Todas las cuentas" con las carpetas comunes combinadas (Entrada, Destacados, Enviados, Borradores, Papelera), y debajo cada cuenta para filtrar.
- **Desactivada:** cada cuenta aparece con su propio árbol de carpetas, una debajo de la otra, como en Thunderbird.
- **Valor por defecto:** desactivada (decisión mía, para que el cambio desde Thunderbird sea suave).
- En ambos modos se puede ver una sola cuenta, y la app recuerda la última vista abierta al reiniciar.

**Tema claro y oscuro,** que sigue al sistema (decisión mía).

### Comportamiento (decisiones mías, confirmadas en la conversación)

- Al responder o reenviar, el correo sale desde la cuenta que recibió el original, con la firma asignada a esa cuenta.
- **Imágenes remotas bloqueadas por defecto,** con un botón "Mostrar imágenes" por mensaje. Se puede agregar "siempre mostrar de este remitente" si hace falta.
- **El HTML recibido se muestra aislado:** iframe con sandbox, sin scripts, HTML sanitizado. Un correo malicioso no puede ejecutar código ni acceder a la app.
- **Alta de cuenta:** email y contraseña. La app intenta detectar los servidores IMAP/SMTP (autoconfiguración); si falla, se cargan host, puerto y seguridad a mano. Antes de guardar la cuenta, se prueba la conexión IMAP y SMTP.
- **Correo nuevo al instante** con IMAP IDLE y notificación nativa del escritorio (desde la Fase 2).
- **Búsqueda local** sobre SQLite, rápida y offline (desde la Fase 2).

### Sincronización, búsqueda y uso sin conexión (decisiones mías, tomadas después de la revisión)

- **Encabezados:** se bajan los de todas las carpetas sincronizadas, de lo más nuevo a lo más viejo y de forma progresiva.
- **Cuerpos:** se bajan en segundo plano las partes de texto (texto plano y HTML, sin adjuntos), también de lo más nuevo a lo más viejo, y se indexan en FTS5. Los adjuntos solo se bajan cuando se piden.
- **Búsqueda:** es local sobre lo que ya está indexado. Si hay conexión y el índice todavía no cubre todo el historial, se ofrece "buscar también en el servidor" con IMAP `SEARCH`.
- **Sin conexión, solo lectura:** se puede leer y buscar lo que está en caché. Las acciones (marcar, mover, borrar, enviar) necesitan conexión. Si fallan, se avisa con claridad y lo redactado no se pierde, porque queda como borrador local. No hay cola de operaciones pendientes en la Fase 1. Igual, el núcleo concentra las acciones sobre el servidor en una sola capa, para poder sumar una cola más adelante sin reescribir.

### Lista de correos: sueltos o conversaciones (decisión de Fabricio)

- **Fase 1:** lista plana, un mensaje por fila, ordenada por fecha.
- **Fase 2:** se agrega un interruptor para ver la lista agrupada en conversaciones. Cada hilo incluye las respuestas propias que están en Enviados.
- **Desde la Fase 1,** el esquema guarda e indexa `Message-ID`, `In-Reply-To` y `References`. Así, la agrupación de la Fase 2 no exige migrar ni volver a sincronizar.

### Ciclo de vida de la app (decisión de Fabricio)

- **Al cerrar la ventana,** la app no termina: se esconde en la **bandeja del sistema** y sigue sincronizando (IMAP IDLE) y mostrando notificaciones. Para salir de verdad se usa "Salir" desde el menú del ícono de la bandeja o desde el menú de la app.
- **Opción "Iniciar al abrir sesión"** en Configuración. Por defecto está desactivada (decisión mía).
- **Llega en la Fase 2,** junto con IDLE y las notificaciones. En la Fase 1, cerrar es salir.

### Responder y reenviar (decisiones mías, tomadas después de la revisión)

- **El HTML del original nunca se carga dentro de TipTap.** El redactor tiene lo que escribís; el original va debajo como bloque citado no editable, sanitizado y mostrado en el mismo iframe con sandbox que usa el lector.
- **Al enviar,** el MIME se arma con lo redactado (convertido a HTML compatible con email), seguido del original citado (`blockquote` con "El <fecha>, <remitente> escribió:"), y una versión en texto plano con el original citado con `>`.
- **Reenviar incluye los adjuntos originales ya en la Fase 1,** como hace Outlook. Esto implica armar MIME con adjuntos desde la Fase 1. Lo que queda para la Fase 3 es adjuntar archivos propios desde el disco.

### Firmas (decisión de Fabricio)

- **Biblioteca global de firmas,** independiente de las cuentas. Una misma firma puede usarse en varias cuentas.
- Cada cuenta tiene una firma por defecto (o ninguna).
- Desde el redactor se puede cambiar de firma antes de enviar.
- Las firmas se editan con el mismo editor TipTap que los correos, así que admiten formato y una imagen (por ejemplo un logo).

### Redactor (decisión de Fabricio)

- Editor TipTap estilo Notion: al escribir "/" se abre un menú para insertar elementos (títulos, listas, citas, tabla, imagen, separador, link).
- Además, una **barra de formato** al seleccionar texto, para que sea amigable sin conocer comandos (referencia: Outlook).
- **Restricción técnica:** el HTML de correo no es HTML web, porque muchos clientes (Gmail, Outlook) ignoran gran parte del CSS. Antes de enviar, el contenido del editor se convierte a HTML compatible con email (estilos inline, estructuras simples) y se genera también una versión de **texto plano**, enviando `multipart/alternative`. Los bloques que no se puedan representar de forma fiable en email (columnas, toggles) no se ofrecen en el menú "/".

### Contactos (decisión de Fabricio)

- **Fichas amplias:** nombre, uno o varios emails, teléfonos, empresa y notas.
- **Autocompletado en Para/CC/CCO** a partir de la libreta y de las direcciones con las que ya hubo intercambio de correo.
- **Desde un correo abierto,** "agregar remitente a contactos" con un clic.
- No se importa nada de Thunderbird.

## Fases

El orden de las fases 2 a 4 lo propuse yo y Fabricio lo aceptó.

### Fase 1 — "Primer correo" (criterio de éxito de la v1)
- Proyecto Wails v3 + React + shadcn con el layout de 3 paneles, tema claro y oscuro.
- Núcleo Go separado de la cáscara desde el inicio.
- Alta de **una** cuenta (aunque el modelo de datos ya soporta varias): autodetección o carga manual, prueba de conexión y contraseña en el llavero.
- Sincronización de la lista de carpetas y de los mensajes de la Entrada (y de la carpeta abierta).
- Lista de mensajes y lectura segura (sandbox e imágenes bloqueadas). Ver y descargar adjuntos recibidos.
- Marcar como leído al abrir.
- Redactar, responder y reenviar con TipTap básico (negrita, cursiva, listas, links), sin menú "/".
- Envío por SMTP (`multipart/alternative`) y copia en la carpeta Enviados del servidor, salvo que el servidor la guarde solo.

### Fase 2 — Multicuenta
- Varias cuentas (las 4), árbol de carpetas por cuenta y bandeja unificada opcional.
- IMAP IDLE y notificaciones nativas, con la app viviendo en la bandeja del sistema e inicio opcional al abrir sesión.
- Interruptor para agrupar la lista en conversaciones.
- Mover, borrar (a Papelera), destacar y marcar como leído o no leído.
- Búsqueda local.

### Fase 3 — Redacción rica
- Menú "/" y barra de formato al seleccionar texto.
- Biblioteca de firmas globales con asignación por cuenta.
- Adjuntar archivos al enviar.
- Borradores que se guardan solos (localmente y en la carpeta Borradores del servidor).

### Fase 4 — Contactos
- Libreta con fichas, autocompletado y "agregar remitente".

### Más adelante (sin fecha, fuera de este diseño)
Modo servidor en el VPS, app móvil, integración con Taskapp, sincronización de contactos.

## Restricciones

- Un solo usuario y una sola máquina. No hace falta auth propia de la app más allá de las credenciales IMAP/SMTP.
- **Servidores Hostinger (a verificar durante la implementación):** IMAP `imap.hostinger.com:993` (SSL/TLS) y SMTP `smtp.hostinger.com:465` (SSL/TLS). La cuarta cuenta es de otro hosting común, y sus datos los carga Fabricio.
- **Autenticación:** las 4 cuentas usan usuario y contraseña por IMAP/SMTP. OAuth2 (Gmail, Microsoft) queda **fuera de alcance**. El modelo de cuenta no debería impedir agregarlo más adelante, pero no se construye.
- **Pruebas:** nunca contra las cuentas reales de forma automática. Los tests usan un servidor IMAP/SMTP local o en memoria. Las pruebas con cuentas reales son manuales.
- **Seguridad TLS:** se verifican los certificados siempre. No hay opción de "aceptar cualquier certificado" en la Fase 1.

## Lo que queda a criterio del builder

- **Librerías Go concretas.** Referencia: `emersion/go-imap` v2 (todavía en beta, la última es v2.0.0-beta.8; evaluar si alcanza o si conviene v1), `emersion/go-message` para MIME, `emersion/go-smtp` o `net/smtp` para el envío, `zalando/go-keyring` para el llavero, `bluemonday` para sanitizar HTML, driver SQLite (`modernc.org/sqlite` o `mattn/go-sqlite3`). Mostrador usaba `sqlc`, y repetirlo es razonable.
- **Esquema exacto de SQLite.** Tiene que respetar la semántica IMAP (UID + UIDVALIDITY por carpeta), soportar varias cuentas desde el día 1 y prever búsqueda de texto completo (FTS5) para la Fase 2.
- **Detalles de la sincronización** (tamaño de los lotes, prioridades, límites de concurrencia por cuenta), dentro de lo definido en "Sincronización, búsqueda y uso sin conexión".
- **Mecanismo de autoconfiguración** (por ejemplo la base ISPDB de Mozilla, registros DNS SRV o heurísticas `imap.<dominio>`), con fallback manual.
- **Conversión del HTML de TipTap a HTML compatible con email.**
- **Estructura de carpetas del repo, Taskfile y tooling** (Wails v3 ya genera un Taskfile; Mostrador tenía `task check` con vet, staticcheck, tests, tsc y eslint, y conviene replicarlo).
- **Componentes shadcn concretos** y detalles visuales finos, dentro de la estética del mockup elegido.

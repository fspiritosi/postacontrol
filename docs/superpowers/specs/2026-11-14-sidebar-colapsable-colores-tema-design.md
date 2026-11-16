# Sidebar colapsable, color de carpetas y tema — diseño

Fecha: 2026-11-14

## Intención

Ganar espacio en pantallas chicas sin perder la navegación entre cuentas y
carpetas. Además, dos ajustes de personalización: colorear carpetas y elegir
el tema (claro, oscuro o el del sistema).

Son tres cambios independientes y chicos. Se pueden construir y probar por
separado, en este orden: tema → color de carpetas → sidebar colapsable. El
sidebar va último porque el menú flotante muestra los colores de carpeta.

## 1. Sidebar colapsable

**Qué se ve.** Colapsado, el sidebar es una columna de ~52px con íconos.
Cada ícono tiene un tooltip con el nombre:

```
┌────┐
│ ✎  │  Redactar
│ ⇥  │  Expandir sidebar
│────│
│ ▣  │  vistas unificadas (solo si están activadas): Entrada, Destacados…
│────│
│ (F)│  burbuja de cada cuenta: su color + inicial, con contador de no leídos
│ (T)│
│────│
│ ⏲  │  Bandeja de salida (solo si hay pendientes)
│ 👥 │  Contactos
│ ⚙  │  Ajustes
└────┘
```

- **Clic en la burbuja de una cuenta:** se abre un popover a la derecha con
  el árbol de carpetas de esa cuenta, con contadores, menú contextual, color
  de carpeta y soltar correos arrastrados. Elegir una carpeta cierra el
  popover. El tooltip muestra el nombre y el email de la cuenta.
- **Plegar/desplegar:** un botón junto a Redactar (expandido) o arriba de la
  columna (colapsado). Arrastrar el borde por debajo del mínimo también lo
  pliega.
- **Se recuerda** entre sesiones, como los tamaños de los paneles.

**Approaches.**
- *Elegido — A: colapso nativo del panel.* `react-resizable-panels` 4.x ya
  tiene `collapsible` + `collapsedSize="52px"` y la API imperativa
  (`collapse`/`expand`/`isCollapsed`, `onResize`). El panel del sidebar en
  `MailShell` pasa a ser colapsable. El layout colapsado se guarda con el
  `onLayoutChanged` que ya existe. MailShell sabe si está colapsado (por
  `onResize`) y se lo pasa al slot `sidebar`.
- *Descartado — B: barra aparte que reemplaza al panel.* Pierde el plegado
  arrastrando y obliga a rearmar cómo se guardan los tamaños.

**Menú flotante — Approaches.**
- *Elegido — A: reusar el árbol de `AccountSection` dentro de un `Popover`.*
  Se extrae la parte que dibuja el árbol de carpetas de una cuenta para que
  la usen la sección expandida y el popover. Así el popover tiene todo lo
  del sidebar normal sin duplicar código.
- *Descartado — B: menú simple con la lista de carpetas.* Pierde el arrastre
  y el menú contextual.

## 2. Color de carpetas

- Se elige desde el menú contextual de la carpeta ("Color" → la paleta de 12
  de las etiquetas, `LABEL_COLORS`, + "Sin color"). Vale para cualquier
  carpeta real, incluidas las especiales (Entrada, Enviados…). No vale para
  los padres sintetizados (id 0).
- El color tiñe el ícono de la carpeta (también el ícono de rol) en el
  sidebar y en el popover.
- **Dónde se guarda:** columna nueva `folders.color TEXT NOT NULL DEFAULT ''`
  (migración `schema_v16.sql`), `Folder.Color` en el modelo y un método de
  servicio `SetFolderColor(folderID, color)`. Es local: no se sube a IMAP.
  La sincronización (`ReplaceFolders` hace upsert) no la pisa, como pasa hoy
  con `position`. Renombrar o mover mantiene la fila y el color. Entra en los
  respaldos porque está en la base.
- *Descartado:* guardarlo en `localStorage` por id. No entra en los
  respaldos y se pierde al quitar y volver a agregar la cuenta.

## 3. Tema claro / oscuro / sistema

- Nuevo campo `Settings.Theme`: `"light"`, `"dark"` o `"system"`. Vacío
  (configuración vieja) se lee como `"system"`.
- Se cambia en Ajustes → General con un selector de tres opciones.
- `ThemeProvider` aplica la clase `dark` según el ajuste. Con "sistema" sigue
  escuchando `prefers-color-scheme`, como hoy.
- Para que no parpadee al arrancar, el último tema aplicado se copia a
  `localStorage` y se usa en el primer render, antes de que lleguen los
  ajustes.
- La ventana del redactor usa el mismo `ThemeProvider`, así que sigue el
  ajuste. Al cambiarlo se actualiza al recargar los ajustes en esa ventana.
- El `Toaster` (sonner) recibe el tema efectivo en lugar de `"system"`.

## No-objetivos

- Sincronizar colores de carpeta con el servidor u otros clientes.
- Colores personalizados fuera de la paleta.
- Atajo de teclado para plegar el sidebar (se puede agregar después).
- Plegado automático según el ancho de la ventana.
- Agregar cuenta desde la columna colapsada (se hace expandida).

## Queda a criterio del que construye

- Íconos exactos (p. ej. `PanelLeftCloseIcon` / `PanelLeftOpenIcon`).
- Cómo se ve el contador de no leídos sobre la burbuja.
- Cómo se avisa a la ventana del redactor que cambió el tema si ya estaba
  abierta (evento existente de ajustes o recarga de la query).

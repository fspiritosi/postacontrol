import { computePosition, flip, offset, shift } from "@floating-ui/dom";
import { Extension, ReactRenderer, type Editor } from "@tiptap/react";
import { PluginKey } from "@tiptap/pm/state";
import Suggestion, { type SuggestionKeyDownProps, type SuggestionProps } from "@tiptap/suggestion";
import { forwardRef, useEffect, useImperativeHandle, useState } from "react";
import { cn } from "@/lib/utils";
import { filterSlashItems, templateItems, type SlashContext, type SlashItem } from "./slashCommands";

type ListHandle = { onKeyDown: (e: KeyboardEvent) => boolean };
type ListProps = { items: SlashItem[]; command: (item: SlashItem) => void };

// Lista del menú: la selección vive acá y el teclado llega por el handle imperativo.
const SlashList = forwardRef<ListHandle, ListProps>(function SlashList({ items, command }, ref) {
  const [selected, setSelected] = useState(0);
  // Cada vez que cambia la consulta se vuelve al primer ítem.
  useEffect(() => setSelected(0), [items]);

  useImperativeHandle(ref, () => ({
    onKeyDown: (e) => {
      if (items.length === 0) return false;
      if (e.key === "ArrowDown") {
        setSelected((s) => (s + 1) % items.length);
        return true;
      }
      if (e.key === "ArrowUp") {
        setSelected((s) => (s - 1 + items.length) % items.length);
        return true;
      }
      if (e.key === "Enter") {
        command(items[selected]);
        return true;
      }
      return false;
    },
  }));

  return (
    <div
      role="listbox"
      aria-label="Insertar"
      className="z-50 w-56 rounded-md bg-popover p-1 text-sm text-popover-foreground shadow-md ring-1 ring-foreground/10"
    >
      {items.length === 0 ? (
        <p className="px-2 py-1.5 text-muted-foreground">Sin resultados</p>
      ) : (
        items.map((item, i) => (
          <div
            key={item.id}
            role="option"
            aria-selected={i === selected}
            // mousedown (y no click) para no sacarle el foco al editor.
            onMouseDown={(e) => {
              e.preventDefault();
              command(item);
            }}
            onMouseEnter={() => setSelected(i)}
            className={cn("cursor-pointer rounded-sm px-2 py-1.5", i === selected && "bg-accent text-accent-foreground")}
          >
            {item.title}
          </div>
        ))
      )}
    </div>
  );
});

type Range = { from: number; to: number };

// createSlashCommand arma el menú "/". Solo abre al principio del renglón o después de un espacio
// (allowedPrefixes por defecto), así "y/o" no lo dispara. isAvailable oculta ítems que dependen de
// algo opcional, como "Imagen" cuando no hay selector de imágenes.
// onOpenChange avisa cuándo abre (con una función para cerrarlo) y cuándo cierra. Sirve para que el
// diálogo no se cierre con el Esc que es del menú: Radix escucha antes que el editor y, si le
// hacen preventDefault, ProseMirror ya no ve la tecla, así que el menú se cierra desde afuera.
export function createSlashCommand(
  getCtx: () => SlashContext,
  isAvailable: (item: SlashItem) => boolean = () => true,
  onOpenChange: (close: (() => void) | null) => void = () => {},
) {
  const pluginKey = new PluginKey("slashCommand");
  const search = (query: string) =>
    filterSlashItems(query, templateItems(getCtx().templates())).filter(isAvailable);

  return Extension.create({
    name: "slashCommand",
    addProseMirrorPlugins() {
      return [
        Suggestion<SlashItem, SlashItem>({
          editor: this.editor,
          pluginKey,
          char: "/",
          // Los títulos tienen espacios ("/titulo 1").
          allowSpaces: true,
          // Con espacios, solo vale "/titulo 1" y similares: nada de "a / b" ni consultas sin coincidencias.
          shouldShow: ({ query }) => {
            if (/^\s/.test(query)) return false;
            return !/\s/.test(query.trim()) || search(query).length > 0;
          },
          items: ({ query }) => search(query),
          command: ({ editor, range, props }: { editor: Editor; range: Range; props: SlashItem }) => {
            // Primero se borra "/consulta" y recién después se ejecuta el ítem.
            editor.chain().focus().deleteRange(range).run();
            props.run(editor, getCtx());
          },
          render: () => {
            let renderer: ReactRenderer<ListHandle, ListProps> | null = null;

            const place = (props: SuggestionProps<SlashItem, SlashItem>) => {
              const rect = props.clientRect?.();
              if (!renderer || !rect) return;
              const el = renderer.element as HTMLElement;
              const reference = { getBoundingClientRect: () => rect };
              void computePosition(reference, el, {
                placement: "bottom-start",
                strategy: "fixed",
                middleware: [offset(6), flip(), shift({ padding: 8 })],
              }).then(({ x, y }) => {
                el.style.left = `${x}px`;
                el.style.top = `${y}px`;
              });
            };

            return {
              onStart: (props) => {
                renderer = new ReactRenderer(SlashList, {
                  props: { items: props.items, command: props.command },
                  editor: props.editor,
                });
                const el = renderer.element as HTMLElement;
                el.style.position = "fixed";
                el.style.zIndex = "60";
                // Dentro del diálogo (Radix lo deja por encima y con eventos solo adentro).
                (props.editor.view.dom.closest('[role="dialog"]') ?? document.body).appendChild(el);
                onOpenChange(() => props.editor.view.dispatch(props.editor.state.tr.setMeta(pluginKey, { exit: true })));
                place(props);
              },
              onUpdate: (props) => {
                renderer?.updateProps({ items: props.items, command: props.command });
                place(props);
              },
              onKeyDown: ({ event }: SuggestionKeyDownProps) => renderer?.ref?.onKeyDown(event) ?? false,
              onExit: () => {
                renderer?.element.remove();
                renderer?.destroy();
                renderer = null;
                onOpenChange(null);
              },
            };
          },
        }),
      ];
    },
  });
}

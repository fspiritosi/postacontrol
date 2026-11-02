import type { KeyboardEvent } from "react";
import { markAll, NO_MARKS, rangeMark, toggleMark, type Marks } from "@/lib/selection";

export type ClickMods = { ctrl: boolean; shift: boolean };

// Lo que comparten las listas para marcar varios: clic con Ctrl o Shift,
// casilla, Ctrl + A y Esc. `order` son las claves cargadas, en el orden de la
// pantalla; `open` la que está abierta en el lector.
export function useMarking<K>({
  marks,
  onMark,
  order,
  open,
  onOpen,
}: {
  marks: Marks<K>;
  onMark: (m: Marks<K>) => void;
  order: K[];
  open: K | null;
  // Abre una (y el shell suelta las marcas).
  onOpen: (k: K) => void;
}) {
  // Uno solo marcado no es una selección: se abre ese en el lector.
  const apply = (next: Marks<K>) => {
    if (next.keys.length === 1) onOpen(next.keys[0]);
    else onMark(next);
  };

  const select = (k: K, { ctrl, shift }: ClickMods) => {
    if (shift) apply(rangeMark(marks, k, order, open));
    else if (ctrl) apply(toggleMark(marks, k, open));
    else onOpen(k);
  };

  const toggle = (k: K) => apply(toggleMark(marks, k, open));

  // Va en el contenedor de la lista, que recibe el foco al hacer clic.
  const onKeyDown = (e: KeyboardEvent) => {
    if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "a") {
      e.preventDefault();
      apply(markAll(order));
    } else if (e.key === "Escape" && marks.keys.length > 0) {
      onMark(NO_MARKS);
    }
  };

  return { select, toggle, onKeyDown, any: marks.keys.length > 0 };
}

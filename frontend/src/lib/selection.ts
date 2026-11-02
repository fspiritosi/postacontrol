// Correos (o conversaciones) marcados para actuar sobre varios a la vez.
// `anchor` es el último que se marcó a mano: de ahí parte el rango de Shift.
export type Marks<K> = { keys: K[]; anchor: K | null };

export const NO_MARKS: Marks<never> = { keys: [], anchor: null };

// Ctrl + clic (o la casilla): suma o saca uno. Si no había nada marcado y hay
// uno abierto distinto, el abierto también queda marcado: ya se lo estaba mirando.
export function toggleMark<K>(m: Marks<K>, k: K, open: K | null): Marks<K> {
  if (m.keys.includes(k)) return { keys: m.keys.filter((x) => x !== k), anchor: k };
  const base = m.keys.length === 0 && open != null && open !== k ? [open] : m.keys;
  return { keys: [...base, k], anchor: k };
}

// Shift + clic: marca de donde se partió (el ancla, o el abierto) hasta `k`,
// según el orden de la lista, y reemplaza lo marcado. El ancla no cambia.
export function rangeMark<K>(m: Marks<K>, k: K, order: K[], open: K | null): Marks<K> {
  const to = order.indexOf(k);
  let from = order.indexOf(m.anchor ?? open ?? k);
  if (from < 0) from = to;
  if (to < 0) return { keys: [k], anchor: m.anchor };
  const [a, b] = from < to ? [from, to] : [to, from];
  return { keys: order.slice(a, b + 1), anchor: m.anchor };
}

// Ctrl + A: todo lo que está cargado.
export function markAll<K>(order: K[]): Marks<K> {
  return { keys: [...order], anchor: order[0] ?? null };
}

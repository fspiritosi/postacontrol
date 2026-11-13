import type { InfiniteData, QueryClient } from "@tanstack/react-query";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import type { MessageSummary, ThreadSummary } from "../../bindings/postacontrol/core/model/models";
import { loadedTargets, threadLabelTargets, threadTargets, type Target } from "@/lib/bulk";
import type { View } from "@/lib/view";

// Foco donde se escribe: ahí las teclas son texto, no atajos.
export function typing(el: EventTarget | null): boolean {
  return el instanceof Element && !!el.closest(
    "input:not([type=checkbox], [type=radio]), textarea, select, [contenteditable]:not([contenteditable='false'])",
  );
}

// Un diálogo o un menú abierto se quedan con el teclado.
export function overlayOpen(): boolean {
  return document.querySelector("[role=dialog], [role=alertdialog], [role=menu]") != null;
}

// Si la tecla le toca a los atajos: sin modificadores (salvo Shift, que
// cambia el carácter, como en "?" o "#"), sin estar escribiendo y sin un
// diálogo o un menú abierto.
export function shortcutKey(e: KeyboardEvent): boolean {
  if (e.ctrlKey || e.altKey || e.metaKey || e.defaultPrevented) return false;
  if (typing(e.target) || typing(document.activeElement)) return false;
  return !overlayOpen();
}

// Los correos de las listas planas y la búsqueda que se están mirando. Solo
// las consultas activas: las inactivas guardan copias viejas (otro filtro, otra
// búsqueda) que no se refrescan. Cada id aparece una sola vez.
export function loadedSummaries(queryClient: QueryClient): MessageSummary[] {
  const flat = queryClient
    .getQueriesData<InfiniteData<MessageSummary[]>>({ queryKey: ["messages"], type: "active" })
    .flatMap(([, d]) => d?.pages.flat() ?? []);
  const found = queryClient
    .getQueriesData<InfiniteData<{ messages: MessageSummary[] }>>({ queryKey: ["search"], type: "active" })
    .flatMap(([, d]) => d?.pages.flatMap((p) => p.messages ?? []) ?? []);
  const byId = new Map<number, MessageSummary>();
  for (const m of [...flat, ...found]) if (!byId.has(m.id)) byId.set(m.id, m);
  return [...byId.values()];
}

// El mensaje más nuevo de la carpeta en una conversación de la lista.
export function latestOfThread(queryClient: QueryClient, root: string): number | null {
  for (const [, d] of queryClient.getQueriesData<InfiniteData<ThreadSummary[]>>({
    queryKey: ["threads"],
    type: "active",
  })) {
    const t = d?.pages.flat().find((x) => x.threadRoot === root);
    if (t) return t.latest.id;
  }
  return null;
}

// Lo que mira un atajo: la vista, lo marcado y lo abierto.
export type KeyScope = {
  view: View | null;
  messageId: number | null;
  threadRoot: string | null;
  marked: { keys: number[] };
  markedThreads: { keys: string[] };
};

// Sobre qué actúa un atajo: lo marcado y, si no hay nada marcado, el correo o
// la conversación abiertos (fromOpen). Con labels, una conversación trae
// también sus mensajes de Enviados, porque la fila muestra sus etiquetas.
export async function keyTargets(
  queryClient: QueryClient,
  s: KeyScope,
  opts: { labels?: boolean } = {},
): Promise<{ targets: Target[]; fromOpen: boolean }> {
  const folderId = s.view?.kind === "folder" ? s.view.folderId : null;
  const threads = (roots: string[]) =>
    opts.labels ? threadLabelTargets(folderId!, roots) : threadTargets(folderId!, roots);
  if (s.markedThreads.keys.length > 0 && folderId != null)
    return { targets: await threads(s.markedThreads.keys), fromOpen: false };
  if (s.marked.keys.length > 0)
    return { targets: loadedTargets(loadedSummaries(queryClient), s.marked.keys), fromOpen: false };
  if (s.messageId != null) {
    const id = s.messageId;
    const row = loadedTargets(loadedSummaries(queryClient), [id]);
    if (row.length > 0) return { targets: row, fromOpen: true };
    const m = await queryClient.fetchQuery({
      queryKey: ["message", id],
      queryFn: () => svc.GetMessage(id),
      staleTime: 0,
    });
    return {
      targets: [
        { id: m.id, accountId: m.accountId, folderId: m.folderId, seen: m.seen, flagged: m.flagged, labels: m.labels ?? [] },
      ],
      fromOpen: true,
    };
  }
  if (s.threadRoot != null && folderId != null) return { targets: await threads([s.threadRoot]), fromOpen: true };
  return { targets: [], fromOpen: false };
}

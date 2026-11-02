import { useQueryClient, type InfiniteData } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import { loadedTargets, threadTargets, type Target } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { useLabels } from "@/lib/queries";
import type { ShellState } from "../layout/MailShell";
import { applyLabel } from "./applyLabel";

// Foco donde se escribe: ahí los números son texto.
function typing(el: Element | null): boolean {
  return !!el?.closest(
    "input, textarea, select, [contenteditable]:not([contenteditable='false'])",
  );
}

// Un diálogo o un menú abierto se quedan con el teclado.
function overlayOpen(): boolean {
  return (
    document.querySelector(
      "[role=dialog], [role=alertdialog], [role=menu]",
    ) != null
  );
}

// Los correos cargados en las listas planas y en la búsqueda, ya refrescados
// tras cada cambio (el detalle `["message", id]` puede quedar viejo).
function loadedSummaries(
  queryClient: ReturnType<typeof useQueryClient>,
): MessageSummary[] {
  const flat = queryClient
    .getQueriesData<InfiniteData<MessageSummary[]>>({ queryKey: ["messages"] })
    .flatMap(([, d]) => d?.pages.flat() ?? []);
  const found = queryClient
    .getQueriesData<InfiniteData<{ messages: MessageSummary[] }>>({
      queryKey: ["search"],
    })
    .flatMap(([, d]) => d?.pages.flatMap((p) => p.messages ?? []) ?? []);
  return [...flat, ...found];
}

// Teclas 1 a 9: ponen o sacan la etiqueta de esa posición en lo marcado, y si
// no hay nada marcado, en el correo o la conversación abiertos.
export function useLabelKeys(state: ShellState) {
  const queryClient = useQueryClient();
  const labels = useLabels().data;
  // El oyente se registra una vez y lee siempre lo último.
  const latest = useRef({ state, labels });
  useEffect(() => {
    latest.current = { state, labels };
  });

  useEffect(() => {
    const resolve = async (s: ShellState): Promise<Target[]> => {
      const folderId = s.view?.kind === "folder" ? s.view.folderId : null;
      if (s.markedThreads.keys.length > 0 && folderId != null)
        return threadTargets(folderId, s.markedThreads.keys);
      if (s.marked.keys.length > 0)
        return loadedTargets(loadedSummaries(queryClient), s.marked.keys);
      if (s.messageId != null) {
        const id = s.messageId;
        const row = loadedTargets(loadedSummaries(queryClient), [id]);
        if (row.length > 0) return row;
        const m = await queryClient.fetchQuery({
          queryKey: ["message", id],
          queryFn: () => svc.GetMessage(id),
          staleTime: 0,
        });
        return [
          {
            id: m.id,
            accountId: m.accountId,
            folderId: m.folderId,
            seen: m.seen,
            flagged: m.flagged,
            labels: m.labels ?? [],
          },
        ];
      }
      if (s.threadRoot != null && folderId != null)
        return threadTargets(folderId, [s.threadRoot]);
      return [];
    };

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.ctrlKey || e.altKey || e.metaKey || e.shiftKey || e.repeat) return;
      if (!/^[1-9]$/.test(e.key)) return;
      if (typing(e.target as Element | null) || typing(document.activeElement))
        return;
      if (overlayOpen()) return;
      const { state: s, labels: ls } = latest.current;
      const label = ls?.[Number(e.key) - 1];
      if (!label) return;
      void (async () => {
        try {
          const targets = await resolve(s);
          if (targets.length > 0)
            await applyLabel(queryClient, targets, label.id);
        } catch (err) {
          toast.error(errorMessage(err));
        }
      })();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [queryClient]);
}

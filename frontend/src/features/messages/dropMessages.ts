import type { QueryClient } from "@tanstack/react-query";
import type { Folder } from "../../../bindings/postacontrol/core/model/models";
import { moveAndReport, threadTargets, type Target } from "@/lib/bulk";
import type { MessageDrag } from "@/lib/dnd";
import type { ShellState } from "../layout/MailShell";
import type { BulkResult } from "./SelectionBar";
import { finishFlat, finishThreads } from "./useMarking";

// Soltar correos o conversaciones sobre una carpeta: mismo movimiento, mismos
// avisos y misma actualización de la selección que "Mover a…" en la barra.
// Si lo arrastrado no estaba marcado, las marcas que hubiera no se tocan.
export async function dropMessages({
  drag,
  dest,
  state,
  queryClient,
  nameOf,
}: {
  drag: MessageDrag;
  dest: Folder;
  state: ShellState;
  queryClient: QueryClient;
  nameOf: (accountId: number) => string;
}) {
  if (drag.folderId === dest.id) return;
  const rootOf = new Map<number, string>();
  let targets: Target[];
  let touchesMarks: boolean;
  if (drag.roots && drag.folderId != null) {
    // Solo lo de la carpeta que se mira: lo de Enviados no se mueve.
    const per = await Promise.all(
      drag.roots.map((r) => threadTargets(drag.folderId!, [r])),
    );
    per.forEach((ts, i) => ts.forEach((t) => rootOf.set(t.id, drag.roots![i])));
    targets = per.flat();
    touchesMarks = drag.roots.some((r) => state.markedThreads.keys.includes(r));
  } else {
    // Una sola cuenta: el destino es de la misma que lo arrastrado.
    targets = (drag.ids ?? []).map((id) => ({
      id,
      accountId: dest.accountId,
      folderId: drag.folderId ?? 0,
      seen: false,
      flagged: false,
    }));
    touchesMarks = (drag.ids ?? []).some((id) =>
      state.marked.keys.includes(id),
    );
  }
  if (targets.length === 0) return;
  const r = await moveAndReport(targets, dest, nameOf, queryClient);
  const result: BulkResult = { ...r, removed: true };
  if (rootOf.size > 0) finishThreads(state, result, rootOf, !touchesMarks);
  else finishFlat(state, result, !touchesMarks);
}

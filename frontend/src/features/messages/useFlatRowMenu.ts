import { loadedTargets, type Target } from "@/lib/bulk";
import type { MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import type { ShellState } from "../layout/MailShell";

// Clic derecho en las filas de una lista plana (carpeta o búsqueda): sobre las
// marcadas si la fila está marcada, o solo sobre la fila. No toca las marcas.
export function useFlatRowMenu(
  state: ShellState,
  barTargets: () => Promise<Target[]>,
) {
  const marked = state.marked.keys;
  return {
    menuTargets: (row: MessageSummary): Promise<Target[]> =>
      marked.includes(row.id)
        ? barTargets()
        : Promise.resolve(loadedTargets([row], [row.id])),
    // Volver a abrir el lector lo marcaría leído otra vez: se cierra.
    onUnread: (row: MessageSummary) => {
      const ids = marked.includes(row.id) ? marked : [row.id];
      if (state.messageId != null && ids.includes(state.messageId))
        state.selectMessage(null);
    },
  };
}

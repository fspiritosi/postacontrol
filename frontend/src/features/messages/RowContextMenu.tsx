import { useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { invalidateMailQueries, runPerAccount, type Target } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { LabelMenuItems } from "../labels/LabelMenu";

type Props = {
  // Sobre qué actúa el menú: lo marcado si la fila está marcada, o solo la fila.
  // Se pide al abrir el menú, así que refleja el estado de ese momento.
  targets: () => Promise<Target[]>;
  // Se marcaron como no leídos: quien lo usa cierra el lector si hace falta.
  onUnread?: () => void;
  children: ReactNode;
};

// Clic derecho en una fila: etiquetas, leído/no leído y destacado. Mover y
// borrar quedan en la barra de selección y en el lector, porque piden destino
// o confirmación.
export function RowContextMenu({ targets, onUnread, children }: Props) {
  const queryClient = useQueryClient();
  const [current, setCurrent] = useState<Target[]>([]);

  const load = (open: boolean) => {
    if (!open) return;
    setCurrent([]);
    targets().then(setCurrent, (e) => toast.error(errorMessage(e)));
  };

  const allSeen = current.length > 0 && current.every((t) => t.seen);
  const allFlagged = current.length > 0 && current.every((t) => t.flagged);

  const run = async (
    fn: (accountId: number, ids: number[]) => Promise<void>,
  ) => {
    const { failed } = await runPerAccount(current, fn);
    for (const f of failed) toast.error(errorMessage(f.error));
    invalidateMailQueries(queryClient);
    return failed;
  };

  const setSeen = async (seen: boolean) => {
    await run((_a, ids) => svc.SetSeen(ids, seen));
    if (!seen) onUnread?.();
  };
  const setFlagged = () =>
    run((_a, ids) => svc.SetFlagged(ids, !allFlagged));

  return (
    <ContextMenu onOpenChange={load}>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent>
        <ContextMenuSub>
          <ContextMenuSubTrigger>Etiquetas</ContextMenuSubTrigger>
          <ContextMenuSubContent>
            <LabelMenuItems kind="context" targets={current} />
          </ContextMenuSubContent>
        </ContextMenuSub>
        <ContextMenuItem
          disabled={current.length === 0}
          onSelect={() => void setSeen(!allSeen)}
        >
          {allSeen ? "Marcar como no leído" : "Marcar como leído"}
        </ContextMenuItem>
        <ContextMenuItem
          disabled={current.length === 0}
          onSelect={() => void setFlagged()}
        >
          {allFlagged ? "Quitar destacado" : "Destacar"}
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  );
}

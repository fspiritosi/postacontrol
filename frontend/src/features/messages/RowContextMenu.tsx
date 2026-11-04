import { useQueryClient } from "@tanstack/react-query";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import { invalidateMailQueries, runPerAccount, type Target } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { requestNewRule } from "@/lib/events";
import { ruleDraftFromSender } from "@/lib/rules";
import type { Address } from "../../../bindings/postacontrol/core/model/models";
import { LabelMenuItems } from "../labels/LabelMenu";

type Props = {
  // Sobre qué actúa el menú: lo marcado si la fila está marcada, o solo la fila.
  // Se pide al abrir el menú, así que refleja el estado de ese momento.
  targets: () => Promise<Target[]>;
  // Sobre qué actúan las etiquetas, si no es lo mismo que `targets` (en una
  // conversación, todos sus mensajes y no solo los de la carpeta).
  labelTargets?: () => Promise<Target[]>;
  // Se marcaron como no leídos: quien lo usa cierra el lector si hace falta.
  onUnread?: () => void;
  // Remitente de la fila: ofrece crear una regla para lo que llega de él.
  sender?: { accountId: number; from: Address };
  children: ReactNode;
};

// Clic derecho en una fila: etiquetas, leído/no leído y destacado. Mover y
// borrar quedan en la barra de selección y en el lector, porque piden destino
// o confirmación.
export function RowContextMenu({
  targets,
  labelTargets,
  onUnread,
  sender,
  children,
}: Props) {
  const queryClient = useQueryClient();
  const [current, setCurrent] = useState<Target[]>([]);
  const [labelCurrent, setLabelCurrent] = useState<Target[]>([]);

  const load = (open: boolean) => {
    if (!open) return;
    setCurrent([]);
    setLabelCurrent([]);
    targets().then(setCurrent, (e) => toast.error(errorMessage(e)));
    // Si falla, ya avisó `targets`, que pide lo mismo.
    labelTargets?.().then(setLabelCurrent, () => {});
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
            <LabelMenuItems
              kind="context"
              targets={labelTargets ? labelCurrent : current}
            />
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
        {sender?.from.email ? (
          <>
            <ContextMenuSeparator />
            <ContextMenuItem onSelect={() => requestNewRule(ruleDraftFromSender(sender.accountId, sender.from))}>
              Crear regla para este remitente…
            </ContextMenuItem>
          </>
        ) : null}
      </ContextMenuContent>
    </ContextMenu>
  );
}

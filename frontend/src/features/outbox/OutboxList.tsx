import { LoaderCircleIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { LocalDraft } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { errorMessage } from "@/lib/errors";
import { relativeDate } from "@/lib/format";
import { useDrafts } from "@/lib/queries";
import { useComposer } from "../compose/ComposerContext";
import { sendStarted } from "../compose/sendNotices";

function recipients(d: LocalDraft): string {
  const all = [...(d.request.to ?? []), ...(d.request.cc ?? []), ...(d.request.bcc ?? [])];
  return all.map((a) => a.name || a.email).join(", ");
}

function OutboxItem({ draft, onDelete }: { draft: LocalDraft; onDelete: () => void }) {
  const { openDraft } = useComposer();
  const sending = draft.status === "sending";

  const retry = async () => {
    try {
      sendStarted(await svc.QueueSend({ ...draft.request, draftId: draft.id }));
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <li className="space-y-1.5 border-b px-4 py-3">
      <div className="flex items-baseline justify-between gap-2">
        <p className="truncate text-sm font-medium">Para: {recipients(draft)}</p>
        <span className="shrink-0 text-xs text-muted-foreground">{relativeDate(draft.createdAt)}</span>
      </div>
      <p className="truncate text-sm">{draft.request.subject || "(sin asunto)"}</p>
      {sending ? (
        <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <LoaderCircleIcon className="size-3.5 animate-spin" /> Enviando…
        </p>
      ) : (
        <p className="text-xs text-destructive">{draft.error}</p>
      )}
      <div className="flex gap-1.5 pt-1">
        <Button size="sm" variant="outline" disabled={sending} onClick={() => void retry()}>
          Reintentar
        </Button>
        <Button size="sm" variant="ghost" disabled={sending} onClick={() => openDraft(draft)}>
          Abrir
        </Button>
        <Button size="sm" variant="ghost" disabled={sending} onClick={onDelete}>
          Eliminar
        </Button>
      </div>
    </li>
  );
}

// OutboxList muestra la Bandeja de salida: los correos en camino y los que
// no se pudieron enviar.
export function OutboxList() {
  const drafts = useDrafts();
  const [toDelete, setToDelete] = useState<LocalDraft | null>(null);
  const items = drafts.data ?? [];

  const remove = async () => {
    if (!toDelete) return;
    try {
      await svc.DeleteLocalDraft(toDelete.id);
      void drafts.refetch();
    } catch (e) {
      toast.error(errorMessage(e));
    }
    setToDelete(null);
  };

  return (
    <div className="flex h-full flex-col">
      <header className="border-b px-4 py-3">
        <h2 className="text-sm font-semibold">Bandeja de salida</h2>
      </header>
      {drafts.isError && <p className="p-4 text-xs text-destructive">{errorMessage(drafts.error)}</p>}
      {items.length === 0 && !drafts.isPending ? (
        <p className="p-6 text-center text-sm text-muted-foreground">No hay correos pendientes de envío.</p>
      ) : (
        <ul className="min-h-0 flex-1 overflow-y-auto">
          {items.map((d) => (
            <OutboxItem key={d.id} draft={d} onDelete={() => setToDelete(d)} />
          ))}
        </ul>
      )}

      <Dialog open={toDelete !== null} onOpenChange={(o) => !o && setToDelete(null)}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Eliminar este correo?</DialogTitle>
            <DialogDescription>No se envía y se pierde lo que escribiste.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setToDelete(null)}>
              Cancelar
            </Button>
            <Button variant="destructive" onClick={() => void remove()}>
              Eliminar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

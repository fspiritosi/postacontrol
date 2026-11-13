import { useQueryClient } from "@tanstack/react-query";
import { AlarmClockIcon, ArchiveIcon, FilterIcon, FolderInputIcon, MailIcon, PencilIcon, StarIcon, TagIcon, Trash2Icon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { MessageDetail } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { invalidateMailQueries } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { requestNewRule } from "@/lib/events";
import { folderLabel } from "@/lib/folders";
import { useFolders } from "@/lib/queries";
import { ruleDraftFromSender } from "@/lib/rules";
import { undoableToast } from "@/lib/undo";
import { formatWhen } from "@/lib/when";
import { WhenMenu } from "../when/WhenMenu";
import { LabelMenuItems } from "../labels/LabelMenu";
import { useComposer } from "../compose/ComposerContext";

type Props = {
  message: MessageDetail;
  selectMessage: (id: number | null) => void;
};

export function MessageActions({ message, selectMessage }: Props) {
  const queryClient = useQueryClient();
  const composer = useComposer();
  const folders = useFolders(message.accountId).data ?? [];
  const [confirmOpen, setConfirmOpen] = useState(false);
  const current = folders.find((f) => f.id === message.folderId);
  // Hasta saber en qué carpeta está no se puede borrar: en la Papelera el
  // borrado es definitivo y tiene que pedir confirmación.
  const folderKnown = current !== undefined;
  const inTrash = current?.role === "trash";
  // En Borradores no se responde ni se reenvía: se sigue editando.
  const inDrafts = current?.role === "drafts";
  // Archivar no tiene sentido en Archivo ni en Borradores (ahí se sigue editando).
  const canArchive = folderKnown && current.role !== "archive" && !inDrafts;
  // Posponer: no en Borradores, Enviados ni Papelera.
  const canSnooze = folderKnown && !inDrafts && current.role !== "sent" && current.role !== "trash";
  const targets = folders.filter((f) => f.selectable && f.id !== message.folderId);

  const labelTarget = {
    id: message.id,
    accountId: message.accountId,
    folderId: message.folderId,
    seen: message.seen,
    flagged: message.flagged,
    labels: message.labels ?? [],
  };

  const refresh = () => invalidateMailQueries(queryClient);
  const run = async (fn: () => Promise<unknown>, after?: () => void) => {
    try {
      await fn();
      after?.();
      refresh();
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const toggleFlag = () =>
    run(
      () => svc.SetFlagged([message.id], !message.flagged),
      () => void queryClient.invalidateQueries({ queryKey: ["message", message.id] }),
    );
  // Se cierra el lector en vez de refrescarlo: GetMessage lo marcaría leído.
  const markUnread = () =>
    run(
      () => svc.SetSeen([message.id], false),
      () => selectMessage(null),
    );
  // Mover, archivar y borrar cierran el lector y avisan con Deshacer.
  const moveAway = (fn: () => Promise<number>, message: string | null) => {
    let undo = 0;
    return run(
      async () => {
        undo = await fn();
      },
      () => {
        selectMessage(null);
        if (message) undoableToast(queryClient, message, [undo]);
      },
    );
  };
  const snooze = (at: Date) =>
    moveAway(() => svc.SnoozeMessages([message.id], Math.floor(at.getTime() / 1000)), `Pospuesto hasta ${formatWhen(at)}`);
  const moveTo = (id: number, label: string) => moveAway(() => svc.MoveMessages([message.id], id), `Movido a ${label}`);
  const archive = () => moveAway(() => svc.ArchiveMessages([message.id]), "Archivado");
  // En la Papelera es definitivo: no hay nada que avisar ni deshacer.
  const remove = () => moveAway(() => svc.DeleteMessages([message.id]), inTrash ? null : "Movido a la Papelera");

  const keepEditing = async () => {
    try {
      await composer.openComposeDraft(await svc.OpenDraftMessage(message.id));
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <>
      {inDrafts && (
        <Button size="sm" onClick={() => void keepEditing()}>
          <PencilIcon />
          Seguir editando
        </Button>
      )}
      <Button
        variant="outline"
        size="sm"
        aria-pressed={message.flagged}
        aria-label={message.flagged ? "Quitar destacado" : "Destacar"}
        onClick={() => void toggleFlag()}
      >
        <StarIcon className={message.flagged ? "fill-current" : undefined} />
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm">
            <TagIcon />
            Etiquetas
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <LabelMenuItems kind="dropdown" targets={[labelTarget]} />
        </DropdownMenuContent>
      </DropdownMenu>
      <Button variant="outline" size="sm" onClick={() => void markUnread()}>
        <MailIcon />
        Marcar como no leído
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" aria-label="Mover a…" disabled={!folderKnown}>
            <FolderInputIcon />
            Mover a…
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          {targets.map((f) => (
            <DropdownMenuItem key={f.id} onSelect={() => void moveTo(f.id, folderLabel(f))}>
              {folderLabel(f)}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
      {message.from?.email && !inDrafts ? (
        <Button
          variant="outline"
          size="sm"
          title="Crear una regla para lo que llega de este remitente"
          onClick={() => requestNewRule(ruleDraftFromSender(message.accountId, message.from))}
        >
          <FilterIcon />
          Crear regla…
        </Button>
      ) : null}
      {canSnooze && (
        <WhenMenu
          title="Posponer hasta…"
          align="start"
          onPick={(at) => void snooze(at)}
          trigger={
            <Button variant="outline" size="sm">
              <AlarmClockIcon />
              Posponer
            </Button>
          }
        />
      )}
      {canArchive && (
        <Button variant="outline" size="sm" onClick={() => void archive()}>
          <ArchiveIcon />
          Archivar
        </Button>
      )}
      {inTrash ? (
        <Button variant="outline" size="sm" onClick={() => setConfirmOpen(true)}>
          <Trash2Icon />
          Eliminar definitivamente
        </Button>
      ) : (
        <Button variant="outline" size="sm" disabled={!folderKnown} onClick={() => void remove()}>
          <Trash2Icon />
          Borrar
        </Button>
      )}
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Eliminar definitivamente</DialogTitle>
            <DialogDescription>¿Eliminar definitivamente? No se puede deshacer.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>
              Cancelar
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                setConfirmOpen(false);
                void remove();
              }}
            >
              Eliminar definitivamente
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

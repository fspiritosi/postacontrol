import { useQueryClient } from "@tanstack/react-query";
import { FolderInputIcon, MailIcon, StarIcon, Trash2Icon } from "lucide-react";
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
import { errorMessage } from "@/lib/errors";
import { folderLabel } from "@/lib/folders";
import { useFolders } from "@/lib/queries";

type Props = {
  message: MessageDetail;
  selectMessage: (id: number | null) => void;
};

export function MessageActions({ message, selectMessage }: Props) {
  const queryClient = useQueryClient();
  const folders = useFolders(message.accountId).data ?? [];
  const [confirmOpen, setConfirmOpen] = useState(false);
  const current = folders.find((f) => f.id === message.folderId);
  // Hasta saber en qué carpeta está no se puede borrar: en la Papelera el
  // borrado es definitivo y tiene que pedir confirmación.
  const folderKnown = current !== undefined;
  const inTrash = current?.role === "trash";
  const targets = folders.filter((f) => f.selectable && f.id !== message.folderId);

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ["messages"] });
    void queryClient.invalidateQueries({ queryKey: ["threads"] });
    void queryClient.invalidateQueries({ queryKey: ["thread"] });
    void queryClient.invalidateQueries({ queryKey: ["folders"] });
  };
  const run = async (fn: () => Promise<void>, after?: () => void) => {
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
  const moveTo = (id: number, label: string) =>
    run(
      () => svc.MoveMessages([message.id], id),
      () => {
        selectMessage(null);
        toast.success(`Movido a ${label}`);
      },
    );
  const remove = () =>
    run(
      () => svc.DeleteMessages([message.id]),
      () => {
        selectMessage(null);
        if (!inTrash) toast.success("Movido a la Papelera");
      },
    );

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        aria-pressed={message.flagged}
        aria-label={message.flagged ? "Quitar destacado" : "Destacar"}
        onClick={() => void toggleFlag()}
      >
        <StarIcon className={message.flagged ? "fill-current" : undefined} />
      </Button>
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

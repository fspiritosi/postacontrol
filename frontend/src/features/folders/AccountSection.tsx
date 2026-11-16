import { useQueryClient } from "@tanstack/react-query";
import { ChevronRightIcon, MoreHorizontalIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { AccountEditDialog } from "../accounts/AccountEditDialog";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
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
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { errorMessage } from "@/lib/errors";
import { toastSyncError } from "@/lib/events";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";
import { FolderTreeRows, useFolderTree } from "./FolderTree";

// ROLE_ICON vive en FolderTree; se reexporta para quien ya lo importaba de acá.
export { ROLE_ICON } from "./FolderTree";

type SectionProps = {
  account: Account;
  state: ShellState;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChangePassword?: (account: Account) => void;
};

export function AccountSection({ account, state, open, onOpenChange, onChangePassword }: SectionProps) {
  const queryClient = useQueryClient();
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [editing, setEditing] = useState(false);
  const [removing, setRemoving] = useState(false);

  const { tree, list, query: folders, actions, dialogs, openCreate } = useFolderTree(account, state);
  const inboxUnread = list.filter((f) => f.role === "inbox").reduce((n, f) => n + f.unread, 0);
  const name = account.displayName || account.email;

  const refresh = () => {
    svc.SyncAccount(account.id).catch((e) => toastSyncError(account.id, e));
  };

  const setSaveSentCopy = async (save: boolean) => {
    try {
      await svc.SetSaveSentCopy(account.id, save);
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const remove = async () => {
    setRemoving(true);
    try {
      await svc.RemoveAccount(account.id);
      setConfirmOpen(false);
      // La vista de esta cuenta vuelve sola a la de por defecto (MailShell).
      state.selectMessage(null);
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setRemoving(false);
    }
  };

  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <div className="flex items-center gap-1">
        <CollapsibleTrigger asChild>
          <button
            type="button"
            className="flex min-w-0 flex-1 items-center gap-2 rounded-md py-1.5 pl-2 text-left text-sm hover:bg-sidebar-accent"
          >
            <ChevronRightIcon
              className={cn("size-3.5 shrink-0 text-muted-foreground transition-transform", open && "rotate-90")}
            />
            <span
              className="size-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: account.color }}
              aria-hidden
            />
            <span className="min-w-0 flex-1 truncate font-medium" title={account.email}>
              {name}
            </span>
            {inboxUnread > 0 && (
              <span className="text-xs font-medium tabular-nums text-muted-foreground">
                {inboxUnread}
              </span>
            )}
          </button>
        </CollapsibleTrigger>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label={`Opciones de ${name}`}>
              <MoreHorizontalIcon />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onSelect={refresh}>Actualizar</DropdownMenuItem>
            <DropdownMenuCheckboxItem
              checked={account.saveSentCopy}
              onCheckedChange={(v) => void setSaveSentCopy(v === true)}
            >
              Guardar copia en Enviados
            </DropdownMenuCheckboxItem>
            <DropdownMenuItem onSelect={() => setEditing(true)}>Editar cuenta…</DropdownMenuItem>
            {onChangePassword && (
              <DropdownMenuItem onSelect={() => onChangePassword(account)}>
                Cambiar contraseña
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onSelect={openCreate}>
              Nueva carpeta…
            </DropdownMenuItem>
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={() => setConfirmOpen(true)}>
              Quitar cuenta
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <CollapsibleContent className="space-y-0.5">
        {folders.isError && (
          <p className="p-2 text-xs text-destructive">{errorMessage(folders.error)}</p>
        )}
        <FolderTreeRows tree={tree} state={state} accountName={name} actions={actions} />
      </CollapsibleContent>

      {dialogs}

      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Quitar cuenta</DialogTitle>
            <DialogDescription>
              Se borra la copia local y la contraseña guardada. Tus correos en el servidor no se
              tocan.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)} disabled={removing}>
              Cancelar
            </Button>
            <Button variant="destructive" onClick={remove} disabled={removing}>
              Quitar cuenta
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <AccountEditDialog account={editing ? account : null} open={editing} onOpenChange={setEditing} />
    </Collapsible>
  );
}

import { useQueryClient } from "@tanstack/react-query";
import {
  ArchiveIcon,
  ClockArrowUpIcon,
  FileIcon,
  FolderIcon,
  InboxIcon,
  MoreHorizontalIcon,
  SendIcon,
  ShieldAlertIcon,
  Trash2Icon,
} from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
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
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { errorMessage } from "@/lib/errors";
import { toastSyncError } from "@/lib/events";
import { buildFolderTree, type FolderNode } from "@/lib/folders";
import { useDrafts, useFolders } from "@/lib/queries";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";

const ROLE_ICON: Record<string, ReactNode> = {
  inbox: <InboxIcon />,
  drafts: <FileIcon />,
  sent: <SendIcon />,
  archive: <ArchiveIcon />,
  junk: <ShieldAlertIcon />,
  trash: <Trash2Icon />,
};

function FolderRow({
  node,
  depth,
  state,
}: {
  node: FolderNode;
  depth: number;
  state: ShellState;
}) {
  const active = node.id !== 0 && state.view?.kind === "folder" && state.view.folderId === node.id;
  // selectView también pide sincronizar la carpeta.
  const select = () => state.selectView({ kind: "folder", folderId: node.id });
  return (
    <>
      {node.selectable && node.id !== 0 ? (
        <button
          type="button"
          onClick={select}
          aria-current={active ? "true" : undefined}
          style={{ paddingLeft: 8 + depth * 14 }}
          className={cn(
            "flex w-full items-center gap-2 rounded-md py-1.5 pr-2 text-left text-sm transition-colors hover:bg-sidebar-accent",
            active && "bg-sidebar-accent font-medium",
          )}
        >
          <span className="text-muted-foreground [&>svg]:size-4">
            {ROLE_ICON[node.role] ?? <FolderIcon />}
          </span>
          <span className="min-w-0 flex-1 truncate">{node.label}</span>
          {node.unread > 0 && (
            <span className="text-xs font-medium tabular-nums text-muted-foreground">
              {node.unread}
            </span>
          )}
        </button>
      ) : (
        <div
          style={{ paddingLeft: 8 + depth * 14 }}
          className="flex items-center gap-2 py-1.5 pr-2 text-sm text-muted-foreground"
        >
          <FolderIcon className="size-4" />
          <span className="truncate">{node.label}</span>
        </div>
      )}
      {node.children.map((c) => (
        <FolderRow key={c.name} node={c} depth={depth + 1} state={state} />
      ))}
    </>
  );
}

// Solo aparece si hay correos pendientes; en rojo si alguno falló.
function OutboxRow({ state }: { state: ShellState }) {
  const drafts = useDrafts();
  const list = drafts.data ?? [];
  if (list.length === 0) return null;
  const failed = list.some((d) => d.status === "failed");
  const active = state.view?.kind === "outbox";
  return (
    <button
      type="button"
      onClick={() => state.selectView({ kind: "outbox" })}
      aria-current={active ? "true" : undefined}
      className={cn(
        "flex w-full items-center gap-2 rounded-md py-1.5 pr-2 pl-2 text-left text-sm transition-colors hover:bg-sidebar-accent",
        active && "bg-sidebar-accent font-medium",
      )}
    >
      <span className="text-muted-foreground [&>svg]:size-4">
        <ClockArrowUpIcon />
      </span>
      <span className="min-w-0 flex-1 truncate">Bandeja de salida</span>
      <span
        className={cn(
          "text-xs font-medium tabular-nums",
          failed ? "text-destructive" : "text-muted-foreground",
        )}
      >
        {list.length}
      </span>
    </button>
  );
}

export function FolderSidebar({ account, state }: { account: Account; state: ShellState }) {
  const queryClient = useQueryClient();
  const folders = useFolders(account.id);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [removing, setRemoving] = useState(false);

  // La vista inicial (y la de reemplazo si esta cuenta se quita) la elige MailShell.
  const tree = buildFolderTree(folders.data ?? []);

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
      // El mensaje abierto puede ser de la cuenta borrada; la vista vuelve
      // sola a la de por defecto cuando deja de existir su carpeta.
      state.selectMessage(null);
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setRemoving(false);
    }
  };

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2.5 border-b p-3">
        <span
          className="size-2.5 shrink-0 rounded-full"
          style={{ backgroundColor: account.color }}
          aria-hidden
        />
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-medium">{account.displayName || account.email}</p>
          <p className="truncate text-xs text-muted-foreground">{account.email}</p>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon-sm" aria-label="Opciones de la cuenta">
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
            <DropdownMenuSeparator />
            <DropdownMenuItem variant="destructive" onSelect={() => setConfirmOpen(true)}>
              Quitar cuenta
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      <nav className="flex-1 space-y-0.5 overflow-y-auto p-2" aria-label="Carpetas">
        {folders.isError && (
          <p className="p-2 text-xs text-destructive">{errorMessage(folders.error)}</p>
        )}
        <OutboxRow state={state} />
        {tree.map((n) => (
          <FolderRow key={n.name} node={n} depth={0} state={state} />
        ))}
      </nav>

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
    </div>
  );
}

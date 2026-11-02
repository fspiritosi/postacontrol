import { useQueryClient } from "@tanstack/react-query";
import {
  ArchiveIcon,
  ChevronRightIcon,
  FileIcon,
  FolderIcon,
  InboxIcon,
  MoreHorizontalIcon,
  SendIcon,
  ShieldAlertIcon,
  Trash2Icon,
} from "lucide-react";
import { useRef, useState, type DragEvent, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
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
import {
  dragAccount,
  dropZone,
  hasFolderDrag,
  hasMessageDrag,
  readFolderDrag,
  readMessageDrag,
  setFolderDrag,
} from "@/lib/dnd";
import { errorMessage } from "@/lib/errors";
import { toastSyncError } from "@/lib/events";
import {
  buildFolderTree,
  canModify,
  findById,
  inSubtree,
  isSpecialTree,
  moveTargets,
  parentOf,
  siblingsOf,
  type FolderNode,
} from "@/lib/folders";
import { useFolders } from "@/lib/queries";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";
import { dropMessages } from "../messages/dropMessages";
import { DeleteDialog, MoveDialog, NameDialog, type NameMode } from "./FolderDialogs";

export const ROLE_ICON: Record<string, ReactNode> = {
  inbox: <InboxIcon />,
  drafts: <FileIcon />,
  sent: <SendIcon />,
  archive: <ArchiveIcon />,
  junk: <ShieldAlertIcon />,
  trash: <Trash2Icon />,
};

// Lo que el menú contextual de una carpeta le pide a la sección de la cuenta.
type FolderActions = {
  tree: FolderNode[];
  onCreate: (parent: FolderNode) => void;
  onRename: (node: FolderNode) => void;
  onMove: (node: FolderNode) => void;
  onDelete: (node: FolderNode) => void;
  onShift: (node: FolderNode, dir: -1 | 1) => void;
  // Carpeta que se está arrastrando: en `dragover` el navegador no deja leer
  // los datos, así que se guarda acá desde `dragstart`.
  dragging: () => number | null;
  setDragging: (id: number | null) => void;
  onDropFolder: (dragged: FolderNode, target: FolderNode, zone: FolderZone) => Promise<void>;
};

type FolderZone = "before" | "into" | "after";

// Si `dragged` puede soltarse en esa zona de `target`. El núcleo vuelve a
// validar todo; esto solo evita ofrecer lo que no va.
function folderZoneAllowed(
  tree: FolderNode[],
  dragged: FolderNode,
  target: FolderNode,
  zone: FolderZone,
): boolean {
  // Ni sobre sí misma ni sobre una descendiente.
  if (target.id === 0 || inSubtree(dragged, target.name)) return false;
  const sameLevel = siblingsOf(tree, dragged).some((n) => n.name === target.name);
  // Una especial solo se reordena entre sus hermanos.
  if (isSpecialTree(dragged)) return zone !== "into" && sameLevel;
  if (zone === "into") return moveTargets(tree, dragged).some((n) => n.name === target.name);
  if (sameLevel) return true;
  // Cambiar de nivel pide un padre real (los sintetizados no tienen id).
  const parent = parentOf(tree, target);
  return parent == null || parent.id !== 0;
}

function FolderRow({
  node,
  depth,
  state,
  accountName,
  actions,
}: {
  node: FolderNode;
  depth: number;
  state: ShellState;
  accountName: string;
  actions: FolderActions;
}) {
  const queryClient = useQueryClient();
  // Qué indicador de soltar se muestra: adentro (recuadro) o antes/después (línea).
  const [hint, setHint] = useState<FolderZone | null>(null);
  const active = node.id !== 0 && state.view?.kind === "folder" && state.view.folderId === node.id;
  // selectView también pide sincronizar la carpeta.
  const select = () => state.selectView({ kind: "folder", folderId: node.id });
  // Zona para soltar correos: carpeta seleccionable de la misma cuenta que lo
  // arrastrado y que no sea la que se está mirando. La carpeta de origen exacta
  // se vuelve a chequear al soltar (en `dragover` no se pueden leer los datos).
  const droppable = (e: DragEvent) =>
    node.selectable &&
    node.id !== 0 &&
    hasMessageDrag(e.dataTransfer) &&
    dragAccount(e.dataTransfer) === node.accountId &&
    !(
      state.search == null &&
      state.view?.kind === "folder" &&
      state.view.folderId === node.id
    );
  // Zona de una carpeta arrastrada sobre esta fila, o null si no se acepta.
  // Los correos nunca pasan por acá: siempre cuentan como "adentro".
  const folderZone = (e: DragEvent): FolderZone | null => {
    if (
      node.id === 0 ||
      hasMessageDrag(e.dataTransfer) ||
      !hasFolderDrag(e.dataTransfer) ||
      dragAccount(e.dataTransfer) !== node.accountId
    )
      return null;
    const id = actions.dragging() ?? readFolderDrag(e.dataTransfer);
    const dragged = id == null ? null : findById(actions.tree, id);
    if (!dragged) return null;
    const rect = e.currentTarget.getBoundingClientRect();
    const zone = dropZone(e.clientY - rect.top, rect.height);
    return folderZoneAllowed(actions.tree, dragged, node, zone) ? zone : null;
  };
  const onDragOver = (e: DragEvent) => {
    const zone = folderZone(e);
    if (zone || droppable(e)) {
      e.preventDefault();
      e.dataTransfer.dropEffect = "move";
      setHint(zone ?? "into");
    } else setHint(null);
  };
  const onDragLeave = (e: DragEvent) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setHint(null);
  };
  const onDragStart = (e: DragEvent) => {
    actions.setDragging(node.id);
    setFolderDrag(e.dataTransfer, node.accountId, node.id);
  };
  const onDragEnd = () => {
    actions.setDragging(null);
    setHint(null);
  };
  const onDrop = (e: DragEvent) => {
    setHint(null);
    const zone = folderZone(e);
    if (zone) {
      e.preventDefault();
      const dragged = findById(actions.tree, actions.dragging() ?? readFolderDrag(e.dataTransfer) ?? -1);
      actions.setDragging(null);
      if (dragged) void actions.onDropFolder(dragged, node, zone);
      return;
    }
    if (!droppable(e)) return;
    e.preventDefault();
    const drag = readMessageDrag(e.dataTransfer);
    if (!drag) return;
    void dropMessages({
      drag,
      dest: node,
      state,
      queryClient,
      nameOf: () => accountName,
    }).catch((err) => toast.error(errorMessage(err)));
  };
  // Las carpetas reales se arrastran; las sintetizadas (id 0) no.
  const dragProps =
    node.id === 0
      ? {}
      : { draggable: true, onDragStart, onDragEnd, onDragOver, onDragLeave, onDrop };
  const indicator = (hint === "before" || hint === "after") && (
    <span
      aria-hidden
      data-drop={hint}
      className={cn(
        "pointer-events-none absolute inset-x-0 h-0.5 bg-primary",
        hint === "before" ? "top-0" : "bottom-0",
      )}
    />
  );
  const row =
    node.selectable && node.id !== 0 ? (
        <button
          type="button"
          onClick={select}
          {...dragProps}
          aria-current={active ? "true" : undefined}
          style={{ paddingLeft: 8 + depth * 14 }}
          className={cn(
            "relative flex w-full items-center gap-2 rounded-md py-1.5 pr-2 text-left text-sm transition-colors hover:bg-sidebar-accent",
            active && "bg-sidebar-accent font-medium",
            hint === "into" && "outline outline-2 outline-primary bg-primary/10",
          )}
        >
          {indicator}
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
          {...dragProps}
          style={{ paddingLeft: 8 + depth * 14 }}
          className={cn(
            "relative flex items-center gap-2 py-1.5 pr-2 text-sm text-muted-foreground",
            hint === "into" && "outline outline-2 outline-primary bg-primary/10",
          )}
        >
          {indicator}
          <FolderIcon className="size-4" />
          <span className="truncate">{node.label}</span>
        </div>
      );
  // Menú contextual: los padres sintetizados (id 0) no tienen. Una especial
  // no se renombra, mueve ni elimina, pero sí admite subcarpetas y orden.
  // Subir/Bajar se mueven entre hermanos reales (los sintetizados no se guardan).
  const siblings = siblingsOf(actions.tree, node).filter((n) => n.id !== 0);
  const idx = siblings.findIndex((n) => n.name === node.name);
  const modifiable = canModify(node);
  return (
    <>
      {node.id === 0 ? (
        row
      ) : (
        <ContextMenu>
          <ContextMenuTrigger asChild>{row}</ContextMenuTrigger>
          <ContextMenuContent>
            <ContextMenuItem onSelect={() => actions.onCreate(node)}>
              Nueva subcarpeta…
            </ContextMenuItem>
            <ContextMenuItem disabled={!modifiable} onSelect={() => actions.onRename(node)}>
              Renombrar…
            </ContextMenuItem>
            <ContextMenuItem disabled={!modifiable} onSelect={() => actions.onMove(node)}>
              Mover a…
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem disabled={idx <= 0} onSelect={() => actions.onShift(node, -1)}>
              Subir
            </ContextMenuItem>
            <ContextMenuItem
              disabled={idx < 0 || idx >= siblings.length - 1}
              onSelect={() => actions.onShift(node, 1)}
            >
              Bajar
            </ContextMenuItem>
            <ContextMenuSeparator />
            <ContextMenuItem
              variant="destructive"
              disabled={!modifiable}
              onSelect={() => actions.onDelete(node)}
            >
              Eliminar…
            </ContextMenuItem>
          </ContextMenuContent>
        </ContextMenu>
      )}
      {node.children.map((c) => (
        <FolderRow
          key={c.name}
          node={c}
          depth={depth + 1}
          state={state}
          accountName={accountName}
          actions={actions}
        />
      ))}
    </>
  );
}


type SectionProps = {
  account: Account;
  state: ShellState;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onChangePassword?: (account: Account) => void;
};

export function AccountSection({ account, state, open, onOpenChange, onChangePassword }: SectionProps) {
  const queryClient = useQueryClient();
  const folders = useFolders(account.id);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [nameMode, setNameMode] = useState<NameMode | null>(null);
  const [moving, setMoving] = useState<FolderNode | null>(null);
  const [deleting, setDeleting] = useState<FolderNode | null>(null);

  const list = folders.data ?? [];
  const tree = buildFolderTree(list);
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

  // Sube o baja una carpeta entre sus hermanos y guarda el orden completo.
  const shift = async (node: FolderNode, dir: -1 | 1) => {
    const sibs = siblingsOf(tree, node).filter((n) => n.id !== 0);
    const i = sibs.findIndex((n) => n.name === node.name);
    const j = i + dir;
    if (i < 0 || j < 0 || j >= sibs.length) return;
    const next = [...sibs];
    [next[i], next[j]] = [next[j], next[i]];
    try {
      await svc.SetFolderOrder(
        account.id,
        next.map((n) => n.id),
      );
      void queryClient.invalidateQueries({ queryKey: ["folders", account.id] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const dragging = useRef<number | null>(null);

  // Suelta una carpeta: adentro de otra, o antes/después de una hermana (si
  // viene de otro nivel, primero se muda al padre de destino).
  const dropFolder = async (dragged: FolderNode, target: FolderNode, zone: FolderZone) => {
    try {
      if (zone === "into") {
        await svc.MoveFolder(dragged.id, target.id);
      } else {
        const sibs = siblingsOf(tree, target).filter((n) => n.id !== 0);
        const sameLevel = sibs.some((n) => n.name === dragged.name);
        const rest = sibs.filter((n) => n.name !== dragged.name);
        const at = rest.findIndex((n) => n.name === target.name) + (zone === "after" ? 1 : 0);
        const next = [...rest.slice(0, at), dragged, ...rest.slice(at)];
        if (sameLevel && next.every((n, i) => n.name === sibs[i].name)) return;
        if (!sameLevel) await svc.MoveFolder(dragged.id, parentOf(tree, target)?.id ?? 0);
        await svc.SetFolderOrder(
          account.id,
          next.map((n) => n.id),
        );
      }
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void queryClient.invalidateQueries({ queryKey: ["folders", account.id] });
    }
  };

  const actions: FolderActions = {
    tree,
    dragging: () => dragging.current,
    setDragging: (id) => {
      dragging.current = id;
    },
    onDropFolder: dropFolder,
    onCreate: (parent) => setNameMode({ kind: "create", accountId: account.id, parent }),
    onRename: (node) => setNameMode({ kind: "rename", node }),
    onMove: setMoving,
    onDelete: setDeleting,
    onShift: (node, dir) => void shift(node, dir),
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
            {onChangePassword && (
              <DropdownMenuItem onSelect={() => onChangePassword(account)}>
                Cambiar contraseña
              </DropdownMenuItem>
            )}
            <DropdownMenuItem onSelect={() => setNameMode({ kind: "create", accountId: account.id })}>
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
        {tree.map((n) => (
          <FolderRow
            key={n.name}
            node={n}
            depth={1}
            state={state}
            accountName={name}
            actions={actions}
          />
        ))}
      </CollapsibleContent>

      {nameMode && <NameDialog mode={nameMode} onClose={() => setNameMode(null)} />}
      {moving && <MoveDialog node={moving} tree={tree} onClose={() => setMoving(null)} />}
      {deleting && <DeleteDialog node={deleting} onClose={() => setDeleting(null)} />}

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
    </Collapsible>
  );
}

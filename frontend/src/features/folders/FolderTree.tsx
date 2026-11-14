import { useQueryClient, type UseQueryResult } from "@tanstack/react-query";
import {
  ArchiveIcon,
  ChevronRightIcon,
  FileIcon,
  FolderIcon,
  InboxIcon,
  SendIcon,
  ShieldAlertIcon,
  Trash2Icon,
} from "lucide-react";
import { useEffect, useRef, useState, type DragEvent, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account, Folder } from "../../../bindings/postacontrol/core/model/models";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu";
import {
  dragAccount,
  dropZone,
  hasFolderDrag,
  hasMessageDrag,
  readFolderDrag,
  readMessageDrag,
  setFolderDrag,
} from "@/lib/dnd";
import { invalidateMailQueries } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import {
  buildFolderTree,
  canModify,
  findById,
  inSubtree,
  isFirstLevel,
  isSpecialTree,
  moveTargets,
  parentOf,
  siblingsOf,
  subtreeUnread,
  type FolderNode,
} from "@/lib/folders";
import { useFolders } from "@/lib/queries";
import { LABEL_COLORS } from "@/lib/labels";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";
import { dropMessages } from "../messages/dropMessages";
import { DeleteDialog, EmptyTrashDialog, MoveDialog, NameDialog, type NameMode } from "./FolderDialogs";
import { useCollapsedFolders } from "./useCollapsedFolders";

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
  // La lista plana: lo especial se decide por nombre (ver isSpecialTree).
  folders: Folder[];
  onCreate: (parent: FolderNode) => void;
  onRename: (node: FolderNode) => void;
  onMove: (node: FolderNode) => void;
  onDelete: (node: FolderNode) => void;
  onEmptyTrash: (node: FolderNode) => void;
  onShift: (node: FolderNode, dir: -1 | 1) => void;
  // Color local de la carpeta ("" = sin color).
  onColor: (node: FolderNode, color: string) => void;
  // Carpeta que se está arrastrando: en `dragover` el navegador no deja leer
  // los datos, así que se guarda acá desde `dragstart`.
  dragging: () => number | null;
  setDragging: (id: number | null) => void;
  onDropFolder: (dragged: FolderNode, target: FolderNode, zone: FolderZone) => Promise<void>;
  // Subcarpetas plegadas (se recuerda por cuenta).
  isCollapsed: (node: FolderNode) => boolean;
  setCollapsed: (node: FolderNode, collapsed: boolean) => void;
};

// Cuánto hay que quedarse arrastrando sobre una carpeta cerrada para que se abra.
const EXPAND_DELAY_MS = 600;

type FolderZone = "before" | "into" | "after";

// Si `dragged` puede soltarse en esa zona de `target`. El núcleo vuelve a
// validar todo; esto solo evita ofrecer lo que no va.
function folderZoneAllowed(
  tree: FolderNode[],
  folders: Folder[],
  dragged: FolderNode,
  target: FolderNode,
  zone: FolderZone,
): boolean {
  // Ni sobre sí misma ni sobre una descendiente.
  if (target.id === 0 || inSubtree(dragged, target.name)) return false;
  const sameLevel = siblingsOf(tree, dragged).some((n) => n.name === target.name);
  // Una especial (o la que contiene una) solo se reordena entre sus hermanos.
  if (isSpecialTree(folders, dragged)) return zone !== "into" && sameLevel;
  if (zone === "into") return moveTargets(tree, dragged).some((n) => n.name === target.name);
  if (sameLevel) return true;
  // Cambiar de nivel pide un padre real (los sintetizados no tienen id).
  const parent = parentOf(tree, target);
  // A la raíz no se muda la que ya está en el primer nivel del servidor
  // (colgando de INBOX): MoveFolder a 0 la dejaría igual.
  if (parent == null) return !isFirstLevel(dragged);
  return parent.id !== 0;
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
  const hasChildren = node.children.length > 0;
  const collapsed = hasChildren && actions.isCollapsed(node);
  // Cerrada, muestra también los no leídos que quedan escondidos adentro.
  const unread = collapsed ? subtreeUnread(node) : node.unread;
  // Arrastrar algo de esta cuenta y quedarse encima de una carpeta cerrada la abre.
  const expandTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const disarm = () => {
    if (expandTimer.current) clearTimeout(expandTimer.current);
    expandTimer.current = null;
  };
  useEffect(() => {
    const timer = expandTimer;
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, []);
  const armExpand = (e: DragEvent) => {
    if (!collapsed || expandTimer.current) return;
    const dt = e.dataTransfer;
    if (!(hasMessageDrag(dt) || hasFolderDrag(dt)) || dragAccount(dt) !== node.accountId) return;
    if (actions.dragging() === node.id) return;
    expandTimer.current = setTimeout(() => {
      expandTimer.current = null;
      actions.setCollapsed(node, false);
    }, EXPAND_DELAY_MS);
  };
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
    return folderZoneAllowed(actions.tree, actions.folders, dragged, node, zone) ? zone : null;
  };
  const onDragOver = (e: DragEvent) => {
    armExpand(e);
    const zone = folderZone(e);
    if (zone || droppable(e)) {
      e.preventDefault();
      e.dataTransfer.dropEffect = "move";
      setHint(zone ?? "into");
    } else setHint(null);
  };
  const onDragLeave = (e: DragEvent) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) {
      setHint(null);
      disarm();
    }
  };
  const onDragStart = (e: DragEvent) => {
    actions.setDragging(node.id);
    setFolderDrag(e.dataTransfer, node.accountId, node.id);
  };
  const onDragEnd = () => {
    actions.setDragging(null);
    setHint(null);
    disarm();
  };
  const onDrop = (e: DragEvent) => {
    setHint(null);
    disarm();
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
  // Las carpetas reales se arrastran; las sintetizadas (id 0) no, pero igual
  // se abren si se arrastra algo encima.
  const dragProps =
    node.id === 0
      ? { onDragOver: armExpand, onDragLeave, onDrop: disarm }
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
          <span
            data-slot="folder-icon"
            className="text-muted-foreground [&>svg]:size-4"
            style={{ color: node.color || undefined }}
          >
            {ROLE_ICON[node.role] ?? <FolderIcon />}
          </span>
          <span className="min-w-0 flex-1 truncate">{node.label}</span>
          {unread > 0 && (
            <span className="text-xs font-medium tabular-nums text-muted-foreground">
              {unread}
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
          <span data-slot="folder-icon" className="[&>svg]:size-4" style={{ color: node.color || undefined }}>
            <FolderIcon />
          </span>
          <span className="min-w-0 flex-1 truncate">{node.label}</span>
          {unread > 0 && <span className="text-xs font-medium tabular-nums">{unread}</span>}
        </div>
      );
  // Menú contextual: los padres sintetizados (id 0) no tienen. Una especial
  // no se renombra, mueve ni elimina, pero sí admite subcarpetas y orden.
  // Subir/Bajar se mueven entre hermanos reales (los sintetizados no se guardan).
  const siblings = siblingsOf(actions.tree, node).filter((n) => n.id !== 0);
  const idx = siblings.findIndex((n) => n.name === node.name);
  const modifiable = canModify(actions.folders, node);
  return (
    <>
      <div className="relative">
        {hasChildren && (
          <button
            type="button"
            aria-label={`${collapsed ? "Expandir" : "Contraer"} ${node.label}`}
            aria-expanded={!collapsed}
            onClick={() => actions.setCollapsed(node, !collapsed)}
            style={{ left: 4 + (depth - 1) * 14 }}
            className="absolute top-1/2 z-10 flex size-4 -translate-y-1/2 items-center justify-center rounded-sm text-muted-foreground hover:bg-sidebar-accent hover:text-foreground"
          >
            <ChevronRightIcon
              className={cn("size-3.5 transition-transform", !collapsed && "rotate-90")}
            />
          </button>
        )}
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
              <ContextMenuSub>
                <ContextMenuSubTrigger>Color</ContextMenuSubTrigger>
                <ContextMenuSubContent>
                  <div className="grid grid-cols-6 gap-1 p-1">
                    {LABEL_COLORS.map((c) => (
                      <ContextMenuItem
                        key={c}
                        aria-label={c}
                        aria-checked={c === node.color}
                        className={cn(
                          "size-6 justify-center rounded-full p-0 ring-offset-2 ring-offset-popover focus:ring-2 focus:ring-ring",
                          c === node.color && "ring-2 ring-ring",
                        )}
                        style={{ backgroundColor: c }}
                        onSelect={() => actions.onColor(node, c)}
                      />
                    ))}
                  </div>
                  <ContextMenuItem onSelect={() => actions.onColor(node, "")}>
                    Sin color
                  </ContextMenuItem>
                </ContextMenuSubContent>
              </ContextMenuSub>
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
              {node.role === "trash" && node.selectable && (
                <ContextMenuItem variant="destructive" onSelect={() => actions.onEmptyTrash(node)}>
                  Vaciar Papelera…
                </ContextMenuItem>
              )}
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
      </div>
      {!collapsed &&
        node.children.map((c) => (
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

export type FolderTree = {
  tree: FolderNode[];
  list: Folder[];
  query: UseQueryResult<Folder[]>;
  actions: FolderActions;
  // Los diálogos de las acciones (crear, renombrar, mover, eliminar, vaciar).
  // Los dibuja quien usa el hook donde quiera: el popover de la columna
  // plegada los deja afuera para que sigan abiertos cuando él se cierra.
  dialogs: ReactNode;
  // Abre "Nueva carpeta…" en el primer nivel de la cuenta.
  openCreate: () => void;
};

// Todo lo que necesita el árbol de carpetas de una cuenta: los datos, las
// acciones del menú contextual y del arrastre, y sus diálogos.
export function useFolderTree(account: Account, state: ShellState): FolderTree {
  const queryClient = useQueryClient();
  const folders = useFolders(account.id);
  const [nameMode, setNameMode] = useState<NameMode | null>(null);
  const [moving, setMoving] = useState<FolderNode | null>(null);
  const [deleting, setDeleting] = useState<FolderNode | null>(null);
  const [emptying, setEmptying] = useState<FolderNode | null>(null);
  const { collapsed, setCollapsed } = useCollapsedFolders(account.id);

  const list = folders.data ?? [];
  const tree = buildFolderTree(list);

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

  const setColor = async (node: FolderNode, color: string) => {
    try {
      await svc.SetFolderColor(node.id, color);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void queryClient.invalidateQueries({ queryKey: ["folders", account.id] });
    }
  };

  const dragging = useRef<number | null>(null);

  // Suelta una carpeta: adentro de otra, o antes/después de una hermana (si
  // viene de otro nivel, primero se muda al padre de destino).
  const dropFolder = async (dragged: FolderNode, target: FolderNode, zone: FolderZone) => {
    // Mover cambia dónde están los correos (también si falla a medias);
    // reordenar no.
    let moved = false;
    try {
      if (zone === "into") {
        moved = true;
        await svc.MoveFolder(dragged.id, target.id);
      } else {
        const sibs = siblingsOf(tree, target).filter((n) => n.id !== 0);
        const sameLevel = sibs.some((n) => n.name === dragged.name);
        const rest = sibs.filter((n) => n.name !== dragged.name);
        const at = rest.findIndex((n) => n.name === target.name) + (zone === "after" ? 1 : 0);
        const next = [...rest.slice(0, at), dragged, ...rest.slice(at)];
        if (sameLevel && next.every((n, i) => n.name === sibs[i].name)) return;
        if (!sameLevel) {
          moved = true;
          await svc.MoveFolder(dragged.id, parentOf(tree, target)?.id ?? 0);
        }
        await svc.SetFolderOrder(
          account.id,
          next.map((n) => n.id),
        );
      }
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      if (moved) invalidateMailQueries(queryClient);
      else void queryClient.invalidateQueries({ queryKey: ["folders", account.id] });
    }
  };

  const actions: FolderActions = {
    tree,
    folders: list,
    dragging: () => dragging.current,
    setDragging: (id) => {
      dragging.current = id;
    },
    onDropFolder: dropFolder,
    onCreate: (parent) => setNameMode({ kind: "create", accountId: account.id, parent }),
    onRename: (node) => setNameMode({ kind: "rename", node }),
    onMove: setMoving,
    onDelete: setDeleting,
    onEmptyTrash: setEmptying,
    onShift: (node, dir) => void shift(node, dir),
    onColor: (node, color) => void setColor(node, color),
    isCollapsed: (node) => collapsed.has(node.name),
    setCollapsed: (node, value) => setCollapsed(node.name, value),
  };

  const dialogs = (
    <>
        {nameMode && <NameDialog mode={nameMode} onClose={() => setNameMode(null)} />}
        {moving && <MoveDialog node={moving} tree={tree} onClose={() => setMoving(null)} />}
        {deleting && <DeleteDialog node={deleting} onClose={() => setDeleting(null)} />}
        {emptying && (
          <EmptyTrashDialog
            folder={emptying}
            onClose={() => setEmptying(null)}
            onEmptied={() => {
              // Lo abierto o marcado en la Papelera ya no existe.
              const v = state.view;
              if (v?.kind === "folder" && v.folderId === emptying.id) state.selectView(v);
            }}
          />
        )}
    </>
  );

  return {
    tree,
    list,
    query: folders,
    actions,
    dialogs,
    openCreate: () => setNameMode({ kind: "create", accountId: account.id }),
  };
}

// Las filas del árbol desde el primer nivel.
export function FolderTreeRows({
  tree,
  state,
  accountName,
  actions,
}: {
  tree: FolderNode[];
  state: ShellState;
  accountName: string;
  actions: FolderActions;
}) {
  return (
    <>
      {tree.map((n) => (
        <FolderRow key={n.name} node={n} depth={1} state={state} accountName={accountName} actions={actions} />
      ))}
    </>
  );
}

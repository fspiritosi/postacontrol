import { useQueryClient } from "@tanstack/react-query";
import {
  FolderInputIcon,
  MailIcon,
  MailOpenIcon,
  StarIcon,
  TagIcon,
  Trash2Icon,
  ArchiveIcon,
  XIcon,
} from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type {
  Account,
  Folder,
} from "../../../bindings/postacontrol/core/model/models";
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
import {
  invalidateMailQueries,
  moveAndReport,
  runPerAccount,
  type AccountFailure,
  type Target,
} from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { folderLabel } from "@/lib/folders";
import { useAllFolders } from "@/lib/queries";
import { undoableToast } from "@/lib/undo";
import { LabelMenuItems } from "../labels/LabelMenu";

// Cómo terminó una acción: qué ids salieron bien, cuáles fallaron y si los
// correos dejaron de estar donde se los miraba (movidos o borrados).
export type BulkResult = {
  ok: number[];
  failed: number[];
  removed: boolean;
  // Se marcaron como no leídos: el lector abierto se cierra, porque volver a
  // abrirlo (GetMessage) los marcaría leídos otra vez.
  unread?: boolean;
};

type Props = {
  count: number;
  // Conversaciones en lugar de correos sueltos.
  threads?: boolean;
  accounts: Account[];
  // Carpeta que se está viendo (null en la unificada o en una búsqueda general).
  folderId: number | null;
  // Los correos marcados que siguen cargados. Tiene que ser estable entre
  // renders mientras no cambien las marcas.
  targets: () => Promise<Target[]>;
  // Sobre qué actúan las etiquetas, si no es lo mismo que `targets` (con
  // conversaciones, todos sus mensajes). También tiene que ser estable.
  labelTargets?: () => Promise<Target[]>;
  onDone: (r: BulkResult) => void;
  onClear: () => void;
};

// Barra sobre la lista cuando hay varios marcados.
export function SelectionBar({
  count,
  threads,
  accounts,
  folderId,
  targets,
  labelTargets,
  onDone,
  onClear,
}: Props) {
  const queryClient = useQueryClient();
  const { folders, pending: foldersPending } = useAllFolders(accounts);
  const [current, setCurrent] = useState<Target[] | null>(null);
  const [busy, setBusy] = useState(false);
  // Borrado que espera confirmación, con los correos que va a tocar.
  const [confirm, setConfirm] = useState<Target[] | null>(null);

  // Para decidir qué ofrecer hay que saber qué es lo marcado.
  useEffect(() => {
    let live = true;
    targets().then(
      (t) => live && setCurrent(t),
      (e) => {
        if (!live) return;
        setCurrent([]);
        toast.error(errorMessage(e));
      },
    );
    return () => {
      live = false;
    };
  }, [targets]);

  const [labelCurrent, setLabelCurrent] = useState<Target[] | null>(null);
  useEffect(() => {
    if (!labelTargets) return;
    let live = true;
    // Si falla, ya avisó `targets`, que pide lo mismo.
    labelTargets().then(
      (t) => live && setLabelCurrent(t),
      () => live && setLabelCurrent([]),
    );
    return () => {
      live = false;
    };
  }, [labelTargets]);

  const inTrash = (t: Target) =>
    folders.find((f) => f.id === t.folderId)?.role === "trash";
  const list = current ?? [];
  const ready = current != null && current.length > 0 && !busy;
  // Borrar necesita saber en qué carpeta está cada correo: en la Papelera el
  // borrado es definitivo y tiene que pedir confirmación.
  const foldersKnown =
    !foldersPending &&
    list.every((t) => folders.some((f) => f.id === t.folderId));
  const canRemove = ready && foldersKnown;
  const allFlagged = list.length > 0 && list.every((t) => t.flagged);
  const allTrash = list.length > 0 && list.every(inTrash);
  const allArchived =
    list.length > 0 &&
    list.every((t) => folders.find((f) => f.id === t.folderId)?.role === "archive");
  const accountIds = new Set(list.map((t) => t.accountId));
  const destinations: Folder[] =
    accountIds.size === 1
      ? folders.filter(
          (f) =>
            f.accountId === [...accountIds][0] &&
            f.selectable &&
            f.id !== folderId,
        )
      : [];

  const nameOf = (accountId: number) => {
    const a = accounts.find((x) => x.id === accountId);
    return a ? a.displayName || a.email : String(accountId);
  };

  // Corre la acción sobre lo marcado de ahora (no de cuando se pintó la barra),
  // avisa lo que falló por cuenta y deja marcado solo eso.
  const act = async (
    fn: (accountId: number, ids: number[]) => Promise<unknown>,
    opts: {
      removed: boolean;
      unread?: boolean;
      onOk?: (done: Target[]) => void;
      fixed?: Target[];
    },
  ) => {
    setBusy(true);
    try {
      const all = opts.fixed ?? (await targets());
      const { failed } = await runPerAccount(all, fn);
      finish(all, failed, opts);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const finish = (
    all: Target[],
    failed: AccountFailure[],
    opts: {
      removed: boolean;
      unread?: boolean;
      onOk?: (done: Target[]) => void;
    },
  ) => {
    const failedIds = new Set(failed.flatMap((f) => f.ids));
    const done = all.filter((t) => !failedIds.has(t.id));
    for (const f of failed)
      toast.error(
        `No se pudo en ${nameOf(f.accountId)}: ${errorMessage(f.error)}`,
      );
    if (done.length > 0) opts.onOk?.(done);
    invalidateMailQueries(queryClient);
    onDone({
      ok: done.map((t) => t.id),
      failed: [...failedIds],
      removed: opts.removed,
      unread: opts.unread,
    });
  };

  const setSeen = (seen: boolean) =>
    act((_a, ids) => svc.SetSeen(ids, seen), { removed: false, unread: !seen });
  const setFlagged = () =>
    act((_a, ids) => svc.SetFlagged(ids, !allFlagged), { removed: false });

  const moveTo = async (dest: Folder) => {
    setBusy(true);
    try {
      const all = await targets();
      const r = await moveAndReport(all, dest, nameOf, queryClient);
      onDone({ ...r, removed: true });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const archive = () => {
    const undos: number[] = [];
    return act(
      async (_a, ids) => {
        undos.push(await svc.ArchiveMessages(ids));
      },
      {
        removed: true,
        onOk: (done) =>
          undoableToast(
            queryClient,
            done.length === 1 ? "Archivado" : `${done.length} archivados`,
            undos,
          ),
      },
    );
  };

  const remove = (all: Target[]) => {
    const undos: number[] = [];
    return act(
      async (_a, ids) => {
        undos.push(await svc.DeleteMessages(ids));
      },
      {
        removed: true,
        fixed: all,
        onOk: (done) => {
          const toTrash = done.filter((t) => !inTrash(t)).length;
          if (toTrash > 0)
            undoableToast(queryClient, `${toTrash} movidos a la Papelera`, undos);
        },
      },
    );
  };

  // En la Papelera el borrado es definitivo: si hay algo ahí, se confirma.
  const askRemove = async () => {
    setBusy(true);
    let all: Target[];
    try {
      all = await targets();
    } catch (e) {
      toast.error(errorMessage(e));
      setBusy(false);
      return;
    }
    setBusy(false);
    // Si cambió algo mientras tanto y ya no se sabe dónde está, no se borra.
    if (all.some((t) => !folders.some((f) => f.id === t.folderId))) return;
    if (all.some(inTrash)) setConfirm(all);
    else void remove(all);
  };

  const inTrashCount = confirm?.filter(inTrash).length ?? 0;

  return (
    <div className="flex flex-wrap items-center gap-1.5 border-b bg-primary/10 px-3 py-1.5">
      <span className="min-w-0 flex-1 truncate text-sm font-medium">
        {threads
          ? `${count} conversaciones seleccionadas`
          : `${count} seleccionados`}
      </span>
      <Button
        variant="outline"
        size="sm"
        disabled={!ready}
        onClick={() => void setSeen(true)}
      >
        <MailOpenIcon />
        Marcar leído
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={!ready}
        onClick={() => void setSeen(false)}
      >
        <MailIcon />
        Marcar no leído
      </Button>
      <Button
        variant="outline"
        size="sm"
        disabled={!ready}
        onClick={() => void setFlagged()}
      >
        <StarIcon className={allFlagged ? "fill-current" : undefined} />
        {allFlagged ? "Quitar destacado" : "Destacar"}
      </Button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button variant="outline" size="sm" disabled={!ready}>
            <TagIcon />
            Etiquetas
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <LabelMenuItems
            kind="dropdown"
            targets={labelTargets ? (labelCurrent ?? []) : list}
          />
        </DropdownMenuContent>
      </DropdownMenu>
      {destinations.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline" size="sm" disabled={!ready}>
              <FolderInputIcon />
              Mover a…
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {destinations.map((f) => (
              <DropdownMenuItem key={f.id} onSelect={() => void moveTo(f)}>
                {folderLabel(f)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {!allArchived && (
        <Button
          variant="outline"
          size="sm"
          disabled={!ready}
          onClick={() => void archive()}
        >
          <ArchiveIcon />
          Archivar
        </Button>
      )}
      <Button
        variant="outline"
        size="sm"
        disabled={!canRemove}
        onClick={() => void askRemove()}
      >
        <Trash2Icon />
        {allTrash ? "Eliminar definitivamente" : "Eliminar"}
      </Button>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="Quitar selección"
        onClick={onClear}
      >
        <XIcon />
      </Button>
      <Dialog
        open={confirm != null}
        onOpenChange={(o) => !o && setConfirm(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Eliminar definitivamente</DialogTitle>
            <DialogDescription>
              {confirm != null && inTrashCount === confirm.length
                ? `¿Eliminar definitivamente ${confirm.length} correos? No se puede deshacer.`
                : `${inTrashCount} de los ${confirm?.length ?? 0} correos están en la Papelera y se eliminan definitivamente. Los demás van a la Papelera. No se puede deshacer.`}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirm(null)}>
              Cancelar
            </Button>
            <Button
              variant="destructive"
              onClick={() => {
                const all = confirm;
                setConfirm(null);
                if (all) void remove(all);
              }}
            >
              Eliminar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

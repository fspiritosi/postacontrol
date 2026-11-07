import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useState, type FormEvent } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { FolderDeleteInfo } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { invalidateMailQueries } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { folderLabel, isFirstLevel, isRootLevel, moveTargets, type FolderNode } from "@/lib/folders";

// Los cambios de carpetas también llegan por `mail:folders-updated`, pero
// invalidamos acá para no depender del evento.
function useRefreshFolders(accountId: number) {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: ["folders", accountId] });
}

// Renombrar, mover y eliminar cambian dónde están los correos: la búsqueda y
// las listas abiertas pueden tener los viejos, así que se invalida todo lo de
// correo (incluidas las carpetas).
function useRefreshMail() {
  const queryClient = useQueryClient();
  return () => invalidateMailQueries(queryClient);
}

export type NameMode =
  | { kind: "create"; accountId: number; parent?: FolderNode }
  | { kind: "rename"; node: FolderNode };

// Crear una carpeta (de primer nivel o subcarpeta) o renombrar una.
export function NameDialog({ mode, onClose }: { mode: NameMode; onClose: () => void }) {
  const accountId = mode.kind === "create" ? mode.accountId : mode.node.accountId;
  const refreshFolders = useRefreshFolders(accountId);
  const refreshMail = useRefreshMail();
  const refresh = mode.kind === "rename" ? refreshMail : refreshFolders;
  const [name, setName] = useState(mode.kind === "rename" ? mode.node.label : "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const title =
    mode.kind === "rename"
      ? `Renombrar «${mode.node.label}»`
      : mode.parent
        ? `Nueva subcarpeta de «${mode.parent.label}»`
        : "Nueva carpeta";

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (mode.kind === "rename") {
        await svc.RenameFolder(mode.node.id, name);
      } else {
        await svc.CreateFolder(accountId, mode.parent?.id ?? 0, name);
        toast.success(`Carpeta «${name.trim()}» creada`);
      }
      refresh();
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="folder-name">Nombre</Label>
            <Input
              id="folder-name"
              value={name}
              autoFocus
              onChange={(e) => setName(e.target.value)}
              disabled={busy}
            />
            {error && (
              <p role="alert" className="text-sm text-destructive">
                {error}
              </p>
            )}
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
              Cancelar
            </Button>
            <Button type="submit" disabled={busy}>
              {mode.kind === "rename" ? "Renombrar" : "Crear"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

const FIRST_LEVEL = "root";

// Elegir el nuevo padre de una carpeta.
export function MoveDialog({
  node,
  tree,
  onClose,
}: {
  node: FolderNode;
  tree: FolderNode[];
  onClose: () => void;
}) {
  const refresh = useRefreshMail();
  const targets = moveTargets(tree, node);
  const [dest, setDest] = useState(FIRST_LEVEL);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const options = [
    // Si ya está en el primer nivel (también colgando de INBOX, el prefijo
    // personal de Dovecot) moverla ahí no cambia nada.
    ...(isRootLevel(tree, node) || isFirstLevel(node) ? [] : [{ value: FIRST_LEVEL, label: "Primer nivel" }]),
    ...targets.map((t) => ({ value: String(t.id), label: t.name.split(t.delimiter || "\u0000").join(" / ") })),
  ];
  // Si "Primer nivel" no se ofrece, arranca en el primer destino.
  const current = options.some((o) => o.value === dest) ? dest : (options[0]?.value ?? "");

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await svc.MoveFolder(node.id, current === FIRST_LEVEL ? 0 : Number(current));
      onClose();
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      // También si falló: un movido a medias ya cambió carpetas y correos.
      refresh();
      setBusy(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{`Mover «${node.label}»`}</DialogTitle>
          </DialogHeader>
          <div role="radiogroup" aria-label="Destino" className="max-h-64 space-y-1 overflow-y-auto">
            {options.map((o) => (
              <label key={o.value} className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-accent">
                <input
                  type="radio"
                  name="folder-dest"
                  value={o.value}
                  checked={current === o.value}
                  onChange={() => setDest(o.value)}
                  disabled={busy}
                />
                {o.label}
              </label>
            ))}
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
              Cancelar
            </Button>
            <Button type="submit" disabled={busy || options.length === 0}>
              Mover
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

const plural = (n: number, one: string, many: string) => `${n} ${n === 1 ? one : many}`;

// null cuando no hay nada que avisar: carpeta vacía y sin subcarpetas.
export function deleteDescription(info: FolderDeleteInfo, delimiter = ""): string | null {
  const subs = info.subfolders ?? [];
  const total = info.total + subs.reduce((n, s) => n + s.total, 0);
  if (total === 0 && subs.length === 0) return null;
  const list = subs.map((s) => `«${folderLabel({ name: s.name, delimiter, role: "" })}», con ${plural(s.total, "correo", "correos")}`).join("; ");
  const subsText = `${plural(subs.length, "subcarpeta", "subcarpetas")} (${list})`;
  // Sin ningún correo no hay nada que vaya a la Papelera.
  if (total === 0) return `Tiene ${subsText}. Las carpetas se borran del servidor.`;
  let head = `Tiene ${plural(info.total, "correo", "correos")}`;
  if (subs.length > 0) head += ` y ${subsText}`;
  const trash = total === 1 ? "El correo va" : `Los ${total} correos van`;
  return `${head}. ${trash} a la Papelera y las carpetas se borran del servidor.`;
}

// Confirma y elimina una carpeta (los correos van a la Papelera).
export function DeleteDialog({ node, onClose }: { node: FolderNode; onClose: () => void }) {
  const refresh = useRefreshMail();
  const [info, setInfo] = useState<FolderDeleteInfo | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  // Cada vez que cambia se vuelve a contar en el servidor.
  const [count, setCount] = useState(0);

  useEffect(() => {
    let alive = true;
    svc
      .DeleteFolderPreview(node.id)
      .then((i) => alive && setInfo(i))
      // Si ya hay un error (el de eliminar), ese es el que importa.
      .catch((e) => alive && setError((prev) => prev || errorMessage(e)));
    return () => {
      alive = false;
    };
  }, [node.id, count]);

  const run = async () => {
    setBusy(true);
    setError("");
    try {
      const moved = await svc.DeleteFolder(node.id);
      toast.success(
        moved > 0 ? `Carpeta eliminada; ${plural(moved, "correo", "correos")} a la Papelera` : "Carpeta eliminada",
      );
      onClose();
    } catch (e) {
      // Si cortó a mitad, parte ya pudo ir a la Papelera: los conteos viejos
      // no valen. Se vuelve a contar antes de dejar reintentar.
      setError(errorMessage(e));
      setInfo(null);
      setCount((n) => n + 1);
    } finally {
      refresh();
      setBusy(false);
    }
  };

  const description = info ? deleteDescription(info, node.delimiter) : null;
  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{`¿Eliminar «${node.label}»?`}</DialogTitle>
          {!info && !error && <DialogDescription>Contando correos…</DialogDescription>}
          {description && <DialogDescription>{description}</DialogDescription>}
        </DialogHeader>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancelar
          </Button>
          <Button variant="destructive" onClick={run} disabled={busy || !info}>
            {busy ? "Eliminando…" : "Eliminar"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// Confirma y vacía la Papelera: los correos se borran del servidor para siempre.
export function EmptyTrashDialog({
  folder,
  onClose,
  onEmptied,
}: {
  folder: { id: number; total: number };
  onClose: () => void;
  onEmptied?: () => void;
}) {
  const refresh = useRefreshMail();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const run = async () => {
    setBusy(true);
    setError("");
    try {
      const deleted = await svc.EmptyTrash(folder.id);
      toast.success(
        deleted > 0
          ? `Papelera vaciada; ${plural(deleted, "correo borrado", "correos borrados")}`
          : "La Papelera ya estaba vacía",
      );
      onEmptied?.();
      onClose();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      refresh();
      setBusy(false);
    }
  };

  // El total es el de la caché: el servidor puede tener alguno más.
  const what =
    folder.total > 0
      ? `${folder.total === 1 ? "El correo" : `Los ${folder.total} correos`} de la Papelera se borran`
      : "Los correos de la Papelera se borran";
  return (
    <Dialog open onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>¿Vaciar la Papelera?</DialogTitle>
          <DialogDescription>{`${what} del servidor para siempre. No se puede deshacer.`}</DialogDescription>
        </DialogHeader>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose} disabled={busy}>
            Cancelar
          </Button>
          <Button variant="destructive" onClick={run} disabled={busy}>
            {busy ? "Vaciando…" : "Vaciar Papelera"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

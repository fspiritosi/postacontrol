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
import { errorMessage } from "@/lib/errors";
import { isRootLevel, moveTargets, type FolderNode } from "@/lib/folders";

// Los cambios de carpetas también llegan por `mail:folders-updated`, pero
// invalidamos acá para no depender del evento.
function useRefreshFolders(accountId: number) {
  const queryClient = useQueryClient();
  return () => void queryClient.invalidateQueries({ queryKey: ["folders", accountId] });
}

export type NameMode =
  | { kind: "create"; accountId: number; parent?: FolderNode }
  | { kind: "rename"; node: FolderNode };

// Crear una carpeta (de primer nivel o subcarpeta) o renombrar una.
export function NameDialog({ mode, onClose }: { mode: NameMode; onClose: () => void }) {
  const accountId = mode.kind === "create" ? mode.accountId : mode.node.accountId;
  const refresh = useRefreshFolders(accountId);
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
  const refresh = useRefreshFolders(node.accountId);
  const targets = moveTargets(tree, node);
  const [dest, setDest] = useState(FIRST_LEVEL);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const options = [
    ...(isRootLevel(tree, node) ? [] : [{ value: FIRST_LEVEL, label: "Primer nivel" }]),
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
export function deleteDescription(info: FolderDeleteInfo): string | null {
  const subs = info.subfolders ?? [];
  const total = info.total + subs.reduce((n, s) => n + s.total, 0);
  if (total === 0 && subs.length === 0) return null;
  let head = `Tiene ${plural(info.total, "correo", "correos")}`;
  if (subs.length > 0) {
    const list = subs.map((s) => `«${s.name}», con ${plural(s.total, "correo", "correos")}`).join("; ");
    head += ` y ${plural(subs.length, "subcarpeta", "subcarpetas")} (${list})`;
  }
  return `${head}. Los ${total} correos van a la Papelera y las carpetas se borran del servidor.`;
}

// Confirma y elimina una carpeta (los correos van a la Papelera).
export function DeleteDialog({ node, onClose }: { node: FolderNode; onClose: () => void }) {
  const refresh = useRefreshFolders(node.accountId);
  const [info, setInfo] = useState<FolderDeleteInfo | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    svc
      .DeleteFolderPreview(node.id)
      .then((i) => alive && setInfo(i))
      .catch((e) => alive && setError(errorMessage(e)));
    return () => {
      alive = false;
    };
  }, [node.id]);

  const run = async () => {
    setBusy(true);
    setError("");
    try {
      const moved = await svc.DeleteFolder(node.id);
      toast.success(moved > 0 ? `Carpeta eliminada; ${moved} correos a la Papelera` : "Carpeta eliminada");
      refresh();
      onClose();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const description = info ? deleteDescription(info) : null;
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

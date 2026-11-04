import { useQueryClient } from "@tanstack/react-query";
import { ChevronDownIcon, ChevronUpIcon, PencilIcon, Trash2Icon, TriangleAlertIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Rule } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { invalidateMailQueries } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import type { RuleDraft } from "@/lib/events";
import { useAccounts, useFolders, useLabels, useRules } from "@/lib/queries";
import { brokenAction, describeRule, moveTargets } from "@/lib/rules";
import { RuleEditDialog } from "./RuleEditDialog";
import { PageHeader } from "./SettingsPages";

function runText(n: number): string {
  if (n === 0) return "Ningún correo cumple las reglas.";
  return n === 1 ? "Las reglas se aplicaron a 1 correo." : `Las reglas se aplicaron a ${n} correos.`;
}

type Props = {
  // Regla nueva que llega desde un correo: se abre su editor.
  draft?: RuleDraft | null;
  // Se llama al cerrar el editor de draft.
  onDraftDone?: () => void;
};

export function RulesSection({ draft = null, onDraftDone }: Props) {
  const queryClient = useQueryClient();
  const accounts = useAccounts().data ?? [];
  const [pickedId, setPickedId] = useState<number | null>(draft?.accountId ?? null);
  const accountId = accounts.some((a) => a.id === pickedId) ? pickedId : (accounts[0]?.id ?? null);
  const rules = useRules(accountId).data ?? [];
  const folders = useFolders(accountId).data ?? [];
  const labels = useLabels().data ?? [];
  const targets = moveTargets(folders);
  const inboxId = folders.find((f) => f.role === "inbox" && f.selectable)?.id ?? null;

  // Editor: undefined = cerrado; null = regla vacía; si no, la regla o su base.
  const [editing, setEditing] = useState<Partial<Rule> | null | undefined>(
    draft ? { name: draft.name, conditions: draft.conditions } : undefined,
  );
  // Un pedido nuevo desde un correo con el diálogo ya abierto (ajuste en el render).
  const [seenDraft, setSeenDraft] = useState(draft);
  if (draft !== seenDraft) {
    setSeenDraft(draft);
    if (draft) {
      setPickedId(draft.accountId);
      setEditing({ name: draft.name, conditions: draft.conditions });
    }
  }
  const [deleting, setDeleting] = useState<Rule | null>(null);
  const [runFolder, setRunFolder] = useState<number | null>(null);
  const runFolderId = targets.some((t) => t.id === runFolder) ? runFolder : inboxId;
  const [running, setRunning] = useState(false);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["rules", accountId] });

  const toggle = async (r: Rule, enabled: boolean) => {
    try {
      await svc.SaveRule({ ...r, enabled });
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void refresh();
    }
  };

  const move = async (index: number, delta: -1 | 1) => {
    if (accountId == null) return;
    const ids = rules.map((r) => r.id);
    [ids[index], ids[index + delta]] = [ids[index + delta], ids[index]];
    try {
      await svc.ReorderRules(accountId, ids);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void refresh();
    }
  };

  const remove = async (r: Rule) => {
    setDeleting(null);
    try {
      await svc.DeleteRule(r.id);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void refresh();
    }
  };

  const runNow = async () => {
    if (runFolderId == null) return;
    setRunning(true);
    try {
      const res = await svc.RunRules(runFolderId);
      toast.success(runText(res.matched));
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setRunning(false);
      invalidateMailQueries(queryClient);
    }
  };

  const enabledCount = rules.filter((r) => r.enabled).length;

  return (
    <section aria-labelledby="rules-title" className="space-y-4">
      <PageHeader
        id="rules-title"
        title="Reglas"
        description="Ordenan solas el correo nuevo que llega a la Entrada. Se aplican de arriba hacia abajo."
      >
        <Button size="sm" variant="outline" disabled={accountId == null} onClick={() => setEditing(null)}>
          Nueva regla
        </Button>
      </PageHeader>

      {accounts.length === 0 ? (
        <p className="text-sm text-muted-foreground">Agregá una cuenta para crear reglas.</p>
      ) : (
        <>
          {accounts.length > 1 ? (
            <div className="flex items-center gap-3">
              <Label htmlFor="rules-account">Reglas de</Label>
              <Select value={String(accountId)} onValueChange={(v) => setPickedId(Number(v))}>
                <SelectTrigger id="rules-account" className="w-72">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {accounts.map((a) => (
                    <SelectItem key={a.id} value={String(a.id)}>
                      <span className="size-2.5 rounded-full" style={{ backgroundColor: a.color }} aria-hidden />
                      {a.displayName ? `${a.displayName} · ${a.email}` : a.email}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          ) : null}

          {rules.length === 0 ? (
            <p className="rounded-md border border-dashed p-4 text-sm text-muted-foreground">
              Esta cuenta no tiene reglas. Con una regla podés, por ejemplo, mover a una carpeta todo lo que
              llega de un remitente. También podés crearla desde un correo, con «Crear regla…».
            </p>
          ) : (
            <ul className="divide-y rounded-md border">
              {rules.map((r, i) => {
                const broken = (r.actions ?? []).some((a) => brokenAction(a, folders, labels));
                return (
                  <li key={r.id} className="flex items-center gap-3 px-3 py-2">
                    <Switch
                      checked={r.enabled}
                      aria-label={`Regla ${r.name} prendida`}
                      onCheckedChange={(v) => void toggle(r, v)}
                    />
                    <div className="min-w-0 flex-1">
                      <p className={r.enabled ? "truncate text-sm font-medium" : "truncate text-sm text-muted-foreground"}>
                        {r.name}
                      </p>
                      <p className="truncate text-xs text-muted-foreground" title={describeRule(r, folders, labels)}>
                        {describeRule(r, folders, labels)}
                      </p>
                    </div>
                    {broken ? (
                      <TriangleAlertIcon
                        className="size-4 shrink-0 text-amber-600"
                        aria-label="Usa una carpeta o etiqueta que ya no existe"
                      />
                    ) : null}
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={`Subir ${r.name}`}
                      disabled={i === 0}
                      onClick={() => void move(i, -1)}
                    >
                      <ChevronUpIcon />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={`Bajar ${r.name}`}
                      disabled={i === rules.length - 1}
                      onClick={() => void move(i, 1)}
                    >
                      <ChevronDownIcon />
                    </Button>
                    <Button size="icon-sm" variant="ghost" aria-label={`Editar ${r.name}`} onClick={() => setEditing(r)}>
                      <PencilIcon />
                    </Button>
                    <Button size="icon-sm" variant="ghost" aria-label={`Borrar ${r.name}`} onClick={() => setDeleting(r)}>
                      <Trash2Icon />
                    </Button>
                  </li>
                );
              })}
            </ul>
          )}

          {rules.length > 0 ? (
            <div className="space-y-2 rounded-md bg-muted/40 p-3">
              <p className="text-sm font-medium">Aplicar ahora</p>
              <p className="text-sm text-muted-foreground">
                Pasa las reglas prendidas por todos los correos de una carpeta, también los leídos.
              </p>
              <div className="flex items-center gap-2">
                <Select
                  value={runFolderId != null ? String(runFolderId) : ""}
                  onValueChange={(v) => setRunFolder(Number(v))}
                >
                  <SelectTrigger className="w-64" aria-label="Carpeta donde aplicar las reglas">
                    <SelectValue placeholder="Elegir carpeta…" />
                  </SelectTrigger>
                  <SelectContent>
                    {targets.map((f) => (
                      <SelectItem key={f.id} value={String(f.id)}>
                        {f.path}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={running || runFolderId == null || enabledCount === 0}
                  onClick={() => void runNow()}
                >
                  {running ? "Aplicando…" : "Aplicar"}
                </Button>
              </div>
            </div>
          ) : null}
        </>
      )}

      {editing !== undefined && accountId != null ? (
        <RuleEditDialog
          key={`${accountId}-${editing?.id ?? "new"}`}
          accountId={accountId}
          rule={editing}
          onClose={() => {
            setEditing(undefined);
            onDraftDone?.();
          }}
        />
      ) : null}

      <Dialog open={deleting != null} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Borrar la regla «{deleting?.name}»?</DialogTitle>
            <DialogDescription>Los correos que ya ordenó quedan donde están.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleting(null)}>
              Cancelar
            </Button>
            <Button variant="destructive" onClick={() => deleting && void remove(deleting)}>
              Borrar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

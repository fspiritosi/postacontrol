import { useQueryClient } from "@tanstack/react-query";
import { PlusIcon, XIcon } from "lucide-react";
import { useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Rule, RuleAction, RuleCondition } from "../../../bindings/postacontrol/core/model/models";
import { Alert, AlertDescription } from "@/components/ui/alert";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { errorMessage } from "@/lib/errors";
import { useFolders, useLabels } from "@/lib/queries";
import { moveTargets, RULE_ACTIONS, RULE_FIELDS, RULE_OPS } from "@/lib/rules";

const emptyCondition = (): RuleCondition => ({ field: "from", op: "contains", value: "" });

// Lo que se edita. Las condiciones y acciones vacías de una regla vieja se
// completan para que siempre haya una fila.
function initialRule(accountId: number, rule: Partial<Rule> | null): Rule {
  return {
    id: rule?.id ?? 0,
    accountId,
    name: rule?.name ?? "",
    enabled: rule?.enabled ?? true,
    matchAll: rule?.matchAll ?? true,
    conditions: rule?.conditions?.length ? rule.conditions.map((c) => ({ ...c })) : [emptyCondition()],
    actions: rule?.actions?.length ? rule.actions.map((a) => ({ ...a })) : [{ type: "move", folderId: 0, labelId: 0 }],
    position: rule?.position ?? 0,
  };
}

type Props = {
  accountId: number;
  // Regla a editar (con id) o la base de una nueva (sin id, o null = vacía).
  rule: Partial<Rule> | null;
  onClose: () => void;
};

// Se monta abierto y se desmonta al cerrar: cada apertura arranca de cero.
export function RuleEditDialog({ accountId, rule, onClose }: Props) {
  const queryClient = useQueryClient();
  const folders = useFolders(accountId).data ?? [];
  const labels = useLabels().data ?? [];
  const targets = moveTargets(folders);
  const [draft, setDraft] = useState(() => initialRule(accountId, rule));
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const isNew = draft.id === 0;
  const conditions = draft.conditions ?? [];
  const actions = draft.actions ?? [];

  const setCondition = (i: number, patch: Partial<RuleCondition>) =>
    setDraft((d) => ({ ...d, conditions: (d.conditions ?? []).map((c, j) => (j === i ? { ...c, ...patch } : c)) }));
  const setAction = (i: number, patch: Partial<RuleAction>) =>
    setDraft((d) => ({ ...d, actions: (d.actions ?? []).map((a, j) => (j === i ? { ...a, ...patch } : a)) }));

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      await svc.SaveRule(draft);
      await queryClient.invalidateQueries({ queryKey: ["rules", accountId] });
      onClose();
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{isNew ? "Nueva regla" : "Editar regla"}</DialogTitle>
          <DialogDescription>
            Se aplica sola a los correos sin leer que llegan a la Entrada de esta cuenta.
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-5"
          onSubmit={(e) => {
            e.preventDefault();
            void save();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="rule-name">Nombre</Label>
            <Input
              id="rule-name"
              value={draft.name}
              maxLength={80}
              placeholder="Por ejemplo: Resúmenes del banco"
              onChange={(e) => setDraft((d) => ({ ...d, name: e.target.value }))}
            />
          </div>

          <fieldset className="space-y-2">
            <legend className="mb-2 flex flex-wrap items-center gap-2 text-sm font-medium">
              <span>Si el correo cumple</span>
              <Select
                value={draft.matchAll ? "all" : "any"}
                onValueChange={(v) => setDraft((d) => ({ ...d, matchAll: v === "all" }))}
              >
                <SelectTrigger size="sm" className="w-48" aria-label="Cuántas condiciones">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="all">todas las condiciones</SelectItem>
                  <SelectItem value="any">alguna de las condiciones</SelectItem>
                </SelectContent>
              </Select>
            </legend>
            {conditions.map((c, i) => (
              <div key={i} className="flex items-center gap-2">
                <Select value={c.field} onValueChange={(v) => setCondition(i, { field: v })}>
                  <SelectTrigger className="w-32 shrink-0" aria-label={`Campo de la condición ${i + 1}`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RULE_FIELDS.map((f) => (
                      <SelectItem key={f.value} value={f.value}>
                        {f.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Select value={c.op} onValueChange={(v) => setCondition(i, { op: v })}>
                  <SelectTrigger className="w-36 shrink-0" aria-label={`Comparación de la condición ${i + 1}`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RULE_OPS.map((o) => (
                      <SelectItem key={o.value} value={o.value}>
                        {o.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Input
                  className="min-w-0 flex-1"
                  aria-label={`Texto de la condición ${i + 1}`}
                  value={c.value}
                  maxLength={200}
                  placeholder={c.field === "subject" ? "Texto del asunto" : "nombre o correo"}
                  onChange={(e) => setCondition(i, { value: e.target.value })}
                />
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={`Quitar la condición ${i + 1}`}
                  disabled={conditions.length === 1}
                  onClick={() =>
                    setDraft((d) => ({ ...d, conditions: (d.conditions ?? []).filter((_, j) => j !== i) }))
                  }
                >
                  <XIcon />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={conditions.length >= 20}
              onClick={() => setDraft((d) => ({ ...d, conditions: [...(d.conditions ?? []), emptyCondition()] }))}
            >
              <PlusIcon /> Agregar condición
            </Button>
          </fieldset>

          <fieldset className="space-y-2">
            <legend className="mb-2 text-sm font-medium">Hacer esto</legend>
            {actions.map((a, i) => (
              <div key={i} className="flex items-center gap-2">
                <Select
                  value={a.type}
                  onValueChange={(v) => setAction(i, { type: v, folderId: 0, labelId: 0 })}
                >
                  <SelectTrigger className="w-64 shrink-0" aria-label={`Acción ${i + 1}`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {RULE_ACTIONS.map((t) => (
                      <SelectItem key={t.value} value={t.value}>
                        {t.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                {a.type === "move" ? (
                  <Select
                    value={a.folderId ? String(a.folderId) : ""}
                    onValueChange={(v) => setAction(i, { folderId: Number(v) })}
                  >
                    <SelectTrigger className="min-w-0 flex-1" aria-label={`Carpeta de la acción ${i + 1}`}>
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
                ) : a.type === "label" ? (
                  <Select
                    value={a.labelId ? String(a.labelId) : ""}
                    onValueChange={(v) => setAction(i, { labelId: Number(v) })}
                  >
                    <SelectTrigger className="min-w-0 flex-1" aria-label={`Etiqueta de la acción ${i + 1}`}>
                      <SelectValue placeholder={labels.length ? "Elegir etiqueta…" : "No hay etiquetas"} />
                    </SelectTrigger>
                    <SelectContent>
                      {labels.map((l) => (
                        <SelectItem key={l.id} value={String(l.id)}>
                          <span className="size-2.5 rounded-full" style={{ backgroundColor: l.color }} aria-hidden />
                          {l.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                ) : (
                  <span className="flex-1" />
                )}
                <Button
                  type="button"
                  size="icon-sm"
                  variant="ghost"
                  aria-label={`Quitar la acción ${i + 1}`}
                  disabled={actions.length === 1}
                  onClick={() => setDraft((d) => ({ ...d, actions: (d.actions ?? []).filter((_, j) => j !== i) }))}
                >
                  <XIcon />
                </Button>
              </div>
            ))}
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={actions.length >= 10}
              onClick={() =>
                setDraft((d) => ({ ...d, actions: [...(d.actions ?? []), { type: "markRead", folderId: 0, labelId: 0 }] }))
              }
            >
              <PlusIcon /> Agregar acción
            </Button>
          </fieldset>

          {error ? (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancelar
            </Button>
            <Button type="submit" disabled={saving}>
              Guardar
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

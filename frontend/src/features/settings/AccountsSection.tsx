import { useQueryClient } from "@tanstack/react-query";
import { ChevronDownIcon, ChevronUpIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import { useAccounts } from "@/lib/queries";
import { AccountEditDialog } from "../accounts/AccountEditDialog";
import { PageHeader } from "./SettingsPages";

export function AccountsSection() {
  const queryClient = useQueryClient();
  const accounts = useAccounts().data ?? [];
  const [editingId, setEditingId] = useState<number | null>(null);
  const editing = accounts.find((a) => a.id === editingId) ?? null;

  const move = async (index: number, delta: -1 | 1) => {
    const ids = accounts.map((a) => a.id);
    [ids[index], ids[index + delta]] = [ids[index + delta], ids[index]];
    try {
      await svc.ReorderAccounts(ids);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    }
  };

  return (
    <section aria-labelledby="accounts-title" className="space-y-3">
      <PageHeader
        id="accounts-title"
        title="Cuentas"
        description="El orden se usa en la barra lateral y al elegir el remitente."
      />
      <ul className="space-y-1">
        {accounts.map((a, i) => {
          const name = a.displayName || a.email;
          return (
            <li key={a.id} className="flex items-center gap-2 text-sm">
              <span
                className="size-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: a.color }}
                aria-hidden
              />
              <span className="min-w-0 flex-1 truncate">
                <span className="font-medium">{name}</span>
                {a.displayName ? <span className="ml-2 text-muted-foreground">{a.email}</span> : null}
              </span>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={`Subir ${name}`}
                disabled={i === 0}
                onClick={() => void move(i, -1)}
              >
                <ChevronUpIcon />
              </Button>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={`Bajar ${name}`}
                disabled={i === accounts.length - 1}
                onClick={() => void move(i, 1)}
              >
                <ChevronDownIcon />
              </Button>
              <Button size="sm" variant="outline" aria-label={`Editar ${name}`} onClick={() => setEditingId(a.id)}>
                Editar
              </Button>
            </li>
          );
        })}
      </ul>
      <AccountEditDialog
        account={editing}
        open={editing != null}
        onOpenChange={(o) => !o && setEditingId(null)}
      />
    </section>
  );
}

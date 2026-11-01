import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Signature } from "../../../bindings/postacontrol/core/model/models";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { errorMessage } from "@/lib/errors";
import { useAccounts, useSignatures } from "@/lib/queries";
import { SignatureDialog } from "./SignatureDialog";

const NONE = "0";

export function SignaturesSection() {
  const queryClient = useQueryClient();
  const signatures = useSignatures().data ?? [];
  const accounts = useAccounts().data ?? [];
  // undefined = diálogo cerrado; null = firma nueva.
  const [editing, setEditing] = useState<Signature | null | undefined>(undefined);
  const [deleting, setDeleting] = useState<Signature | null>(null);

  const remove = async (sig: Signature) => {
    setDeleting(null);
    try {
      await svc.DeleteSignature(sig.id);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const setDefault = async (accountId: number, value: string) => {
    try {
      await svc.SetAccountSignature(accountId, Number(value));
    } catch (e) {
      toast.error(errorMessage(e));
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
    }
  };

  return (
    <section aria-labelledby="signatures-title" className="space-y-3">
      <div className="flex items-center justify-between gap-4">
        <h3 id="signatures-title" className="text-sm font-medium">
          Firmas
        </h3>
        <Button size="sm" variant="outline" onClick={() => setEditing(null)}>
          Nueva firma
        </Button>
      </div>
      {signatures.length === 0 ? (
        <p className="text-sm text-muted-foreground">Todavía no creaste ninguna firma.</p>
      ) : (
        <ul className="space-y-1">
          {signatures.map((s) => (
            <li key={s.id} className="flex items-center justify-between gap-2 text-sm">
              <span className="truncate">{s.name}</span>
              <span className="flex shrink-0 gap-1">
                <Button size="sm" variant="ghost" aria-label={`Editar ${s.name}`} onClick={() => setEditing(s)}>
                  Editar
                </Button>
                <Button size="sm" variant="ghost" aria-label={`Eliminar ${s.name}`} onClick={() => setDeleting(s)}>
                  Eliminar
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}
      {accounts.map((a) => (
        <div key={a.id} className="flex items-center justify-between gap-4">
          <Label id={`sig-label-${a.id}`} className="min-w-0 truncate">
            Firma por defecto
            <span className="font-normal text-muted-foreground"> · {a.email}</span>
          </Label>
          <Select value={String(a.defaultSignatureId || 0)} onValueChange={(v) => void setDefault(a.id, v)}>
            <SelectTrigger className="w-48" aria-labelledby={`sig-label-${a.id}`}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NONE}>Sin firma</SelectItem>
              {signatures.map((s) => (
                <SelectItem key={s.id} value={String(s.id)}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      ))}
      {editing !== undefined && <SignatureDialog signature={editing} onClose={() => setEditing(undefined)} />}
      <Dialog open={deleting != null} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Eliminar la firma «{deleting?.name}»?</DialogTitle>
            <DialogDescription>Las cuentas que la usaban quedan sin firma por defecto.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleting(null)}>
              Cancelar
            </Button>
            <Button variant="destructive" onClick={() => deleting && void remove(deleting)}>
              Eliminar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

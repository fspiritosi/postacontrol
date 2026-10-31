import { Loader2Icon } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
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

type Props = { account: Account | null; open: boolean; onOpenChange: (open: boolean) => void };

export function PasswordDialog({ account, open, onOpenChange }: Props) {
  const [password, setPassword] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function handleOpenChange(next: boolean) {
    if (!next) {
      if (saving) return;
      setPassword("");
      setError(null);
    }
    onOpenChange(next);
  }

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!account) return;
    setSaving(true);
    setError(null);
    try {
      await svc.UpdatePassword(account.id, password);
      toast.success("Contraseña actualizada");
      setSaving(false);
      handleOpenChange(false);
    } catch (err) {
      setError(errorMessage(err));
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Cambiar contraseña</DialogTitle>
          <DialogDescription>{account?.email}</DialogDescription>
        </DialogHeader>
        <form onSubmit={save} className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="pwd-new">Contraseña</Label>
            <Input
              id="pwd-new"
              type="password"
              autoComplete="off"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={saving}
              required
            />
          </div>
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}
          <DialogFooter>
            <Button type="submit" disabled={saving}>
              {saving ? (
                <>
                  <Loader2Icon className="animate-spin" /> Probando…
                </>
              ) : (
                "Probar y guardar"
              )}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

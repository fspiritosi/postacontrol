import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
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
import { Switch } from "@/components/ui/switch";
import { errorMessage } from "@/lib/errors";
import { useSettings } from "@/lib/queries";

type Props = { open: boolean; onOpenChange: (open: boolean) => void };

export function SettingsDialog({ open, onOpenChange }: Props) {
  const queryClient = useQueryClient();
  const settings = useSettings();
  const unified = settings.data?.unifiedInbox ?? false;

  // Se guarda al instante, sin botón Guardar.
  const setUnified = async (v: boolean) => {
    try {
      await svc.SetSettings({ ...settings.data, unifiedInbox: v });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Configuración</DialogTitle>
          <DialogDescription className="sr-only">Ajustes de la aplicación.</DialogDescription>
        </DialogHeader>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="unified-inbox">Bandeja unificada</Label>
            <p className="text-sm text-muted-foreground">
              Muestra arriba las carpetas de todas las cuentas juntas.
            </p>
          </div>
          <Switch
            id="unified-inbox"
            checked={unified}
            disabled={settings.isPending}
            onCheckedChange={(v) => void setUnified(v)}
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cerrar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

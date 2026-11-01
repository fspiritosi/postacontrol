import { useQuery, useQueryClient } from "@tanstack/react-query";
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
import { SignaturesSection } from "./SignaturesSection";

type Props = { open: boolean; onOpenChange: (open: boolean) => void };

export function SettingsDialog({ open, onOpenChange }: Props) {
  const queryClient = useQueryClient();
  const settings = useSettings();
  const unified = settings.data?.unifiedInbox ?? false;

  // Se guarda al instante, sin botón Guardar.
  const setUnified = async (v: boolean) => {
    try {
      await svc.SetSettings({ disableNotifications: false, threadedList: false, ...settings.data, unifiedInbox: v });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const startAtLogin = useQuery({
    queryKey: ["startAtLogin"],
    queryFn: () => svc.GetStartAtLogin(),
  });

  const setNotifications = async (enabled: boolean) => {
    try {
      await svc.SetSettings({ unifiedInbox: false, threadedList: false, ...settings.data, disableNotifications: !enabled });
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  // Si falla, el estado queda el de la consulta: el interruptor vuelve atrás.
  const setStartAtLogin = async (v: boolean) => {
    try {
      await svc.SetStartAtLogin(v);
    } catch (e) {
      toast.error(errorMessage(e));
    }
    await queryClient.invalidateQueries({ queryKey: ["startAtLogin"] });
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-2xl">
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
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="notifications">Notificaciones de correo nuevo</Label>
            <p className="text-sm text-muted-foreground">
              Avisa con una notificación del escritorio cuando la ventana no está a la vista.
            </p>
          </div>
          <Switch
            id="notifications"
            checked={!(settings.data?.disableNotifications ?? false)}
            disabled={settings.isPending}
            onCheckedChange={(v) => void setNotifications(v)}
          />
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="start-at-login">Iniciar al abrir sesión</Label>
            <p className="text-sm text-muted-foreground">
              PostaControl arranca escondido en la bandeja del sistema.
            </p>
          </div>
          <Switch
            id="start-at-login"
            checked={startAtLogin.data ?? false}
            disabled={startAtLogin.isPending}
            onCheckedChange={(v) => void setStartAtLogin(v)}
          />
        </div>
        <SignaturesSection />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cerrar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

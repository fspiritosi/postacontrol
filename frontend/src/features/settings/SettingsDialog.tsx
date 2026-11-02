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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { errorMessage } from "@/lib/errors";
import { useBackground, useSettings } from "@/lib/queries";
import { readingPane, saveSettings, type ReadingPane } from "@/lib/settings";
import { LabelsSection } from "./LabelsSection";
import { SignaturesSection } from "./SignaturesSection";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Sección a la que ir al abrir.
  focusSection?: "labels";
};

export function SettingsDialog({ open, onOpenChange, focusSection }: Props) {
  const queryClient = useQueryClient();
  const settings = useSettings();
  const background = useBackground();
  const unified = settings.data?.unifiedInbox ?? false;

  const pickBackground = async () => {
    try {
      // El nombre nuevo del fondo llega con la configuración.
      if (await svc.PickBackground()) await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  const clearBackground = async () => {
    try {
      await svc.ClearBackground();
      await queryClient.invalidateQueries({ queryKey: ["settings"] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  // Se guarda al instante, sin botón Guardar.
  const setUnified = async (v: boolean) => {
    try {
      await saveSettings(queryClient, settings.data, { unifiedInbox: v });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const setReadingPane = async (v: ReadingPane) => {
    try {
      await saveSettings(queryClient, settings.data, { readingPane: v });
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
      await saveSettings(queryClient, settings.data, { disableNotifications: !enabled });
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
      <DialogContent
        className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-2xl"
        // En un campo que usa Escape para cancelar (las etiquetas), Escape no cierra.
        onEscapeKeyDown={(e) => {
          if ((e.target as Element | null)?.closest?.("[data-escape-local]"))
            e.preventDefault();
        }}
      >
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
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="reading-pane">Panel de lectura</Label>
            <p className="text-sm text-muted-foreground">Dónde se muestra el correo abierto.</p>
          </div>
          <Select
            value={readingPane(settings.data)}
            disabled={settings.isPending}
            onValueChange={(v) => void setReadingPane(v as ReadingPane)}
          >
            <SelectTrigger id="reading-pane" className="w-32">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="right">Derecha</SelectItem>
              <SelectItem value="bottom">Abajo</SelectItem>
              <SelectItem value="top">Arriba</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label>Fondo del panel de lectura</Label>
            <p className="text-sm text-muted-foreground">Se ve cuando no hay un correo abierto.</p>
          </div>
          <div className="flex items-center gap-2">
            {background.data ? (
              <img src={background.data} alt="Fondo actual" className="h-10 w-16 rounded object-cover" />
            ) : null}
            <Button variant="outline" size="sm" onClick={() => void pickBackground()}>
              Elegir imagen…
            </Button>
            {background.data ? (
              <Button variant="ghost" size="sm" onClick={() => void clearBackground()}>
                Quitar
              </Button>
            ) : null}
          </div>
        </div>
        <SignaturesSection />
        <LabelsSection focus={focusSection === "labels"} />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cerrar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

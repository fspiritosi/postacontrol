import { useQuery, useQueryClient } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
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
import type { Settings } from "../../../bindings/postacontrol/core/model/models";
import { SignaturesSection } from "./SignaturesSection";

// Encabezado de una página de Configuración.
export function PageHeader({ id, title, description, children }: {
  id: string;
  title: string;
  description?: string;
  children?: ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-4">
      <div className="space-y-1">
        <h2 id={id} className="text-base font-semibold">
          {title}
        </h2>
        {description ? <p className="text-sm text-muted-foreground">{description}</p> : null}
      </div>
      {children}
    </div>
  );
}

// Una opción: nombre y explicación a la izquierda, el control a la derecha.
function SettingRow({ htmlFor, label, description, children }: {
  htmlFor?: string;
  label: string;
  description: string;
  children: ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-4 py-3">
      <div className="space-y-1">
        <Label htmlFor={htmlFor}>{label}</Label>
        <p className="text-sm text-muted-foreground">{description}</p>
      </div>
      {children}
    </div>
  );
}

// Guarda al instante, sin botón Guardar.
function useSaveSetting() {
  const queryClient = useQueryClient();
  const settings = useSettings();
  const save = async (patch: Partial<Settings>) => {
    try {
      await saveSettings(queryClient, settings.data, patch);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  return { settings, save };
}

export function GeneralPage() {
  const queryClient = useQueryClient();
  const { settings, save } = useSaveSetting();
  const startAtLogin = useQuery({
    queryKey: ["startAtLogin"],
    queryFn: () => svc.GetStartAtLogin(),
  });

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
    <section aria-labelledby="page-general" className="space-y-3">
      <PageHeader id="page-general" title="General" description="Cómo arranca PostaControl y cómo avisa." />
      <div className="divide-y">
        <SettingRow
          htmlFor="unified-inbox"
          label="Bandeja unificada"
          description="Muestra arriba las carpetas de todas las cuentas juntas."
        >
          <Switch
            id="unified-inbox"
            checked={settings.data?.unifiedInbox ?? false}
            disabled={settings.isPending}
            onCheckedChange={(v) => void save({ unifiedInbox: v })}
          />
        </SettingRow>
        <SettingRow
          htmlFor="notifications"
          label="Notificaciones de correo nuevo"
          description="Avisa con una notificación del escritorio cuando la ventana no está a la vista."
        >
          <Switch
            id="notifications"
            checked={!(settings.data?.disableNotifications ?? false)}
            disabled={settings.isPending}
            onCheckedChange={(v) => void save({ disableNotifications: !v })}
          />
        </SettingRow>
        <SettingRow
          htmlFor="start-at-login"
          label="Iniciar al abrir sesión"
          description="PostaControl arranca escondido en la bandeja del sistema."
        >
          <Switch
            id="start-at-login"
            checked={startAtLogin.data ?? false}
            disabled={startAtLogin.isPending}
            onCheckedChange={(v) => void setStartAtLogin(v)}
          />
        </SettingRow>
      </div>
    </section>
  );
}

export function ReadingPage() {
  const queryClient = useQueryClient();
  const { settings, save } = useSaveSetting();
  const background = useBackground();

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

  return (
    <section aria-labelledby="page-reading" className="space-y-3">
      <PageHeader id="page-reading" title="Lectura" description="Cómo se ve el correo abierto." />
      <div className="divide-y">
        <SettingRow htmlFor="reading-pane" label="Panel de lectura" description="Dónde se muestra el correo abierto.">
          <Select
            value={readingPane(settings.data)}
            disabled={settings.isPending}
            onValueChange={(v) => void save({ readingPane: v as ReadingPane })}
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
        </SettingRow>
        <SettingRow label="Fondo del panel de lectura" description="Se ve cuando no hay un correo abierto.">
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
        </SettingRow>
      </div>
    </section>
  );
}

export function ComposePage() {
  const { settings, save } = useSaveSetting();
  return (
    <section aria-labelledby="page-compose" className="space-y-6">
      <div className="space-y-3">
        <PageHeader id="page-compose" title="Redacción" description="Cómo se escriben los correos." />
        <div className="divide-y">
          <SettingRow
            htmlFor="spellcheck"
            label="Corregir ortografía al escribir"
            description="Marca las palabras mal escritas en español y en inglés."
          >
            <Switch
              id="spellcheck"
              checked={!(settings.data?.disableSpellcheck ?? false)}
              disabled={settings.isPending}
              onCheckedChange={(v) => void save({ disableSpellcheck: !v })}
            />
          </SettingRow>
        </div>
      </div>
      <SignaturesSection />
    </section>
  );
}

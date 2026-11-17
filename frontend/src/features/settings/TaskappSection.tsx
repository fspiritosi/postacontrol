import { useQueryClient } from "@tanstack/react-query";
import { ChevronRightIcon } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
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
import { TASKAPP_KEYS } from "@/lib/events";
import { errorMessage } from "@/lib/errors";
import { useTaskappStatus } from "@/lib/queries";
import { PageHeader, useSaveSetting } from "./SettingsPages";

export function TaskappSection() {
  const queryClient = useQueryClient();
  const status = useTaskappStatus();
  const [waiting, setWaiting] = useState(false);
  const [confirming, setConfirming] = useState(false);
  // Cada intento tiene un número: solo cuenta el último y solo si no se canceló.
  const attempt = useRef(0);

  const connect = () => {
    const id = ++attempt.current;
    setWaiting(true);
    svc
      .ConnectTaskapp()
      .then(() => {
        if (id !== attempt.current) return;
        setWaiting(false);
        for (const key of TASKAPP_KEYS) void queryClient.invalidateQueries({ queryKey: [key] });
        toast.success("Conectado con Taskapp.");
      })
      .catch((e) => {
        if (id !== attempt.current) return;
        setWaiting(false);
        toast.error(errorMessage(e));
      });
  };

  // Solo cierra el estado local: core vence a los 5 minutos.
  const cancel = () => {
    attempt.current++;
    setWaiting(false);
  };

  const disconnect = async () => {
    setConfirming(false);
    try {
      await svc.DisconnectTaskapp();
    } catch (e) {
      toast.error(errorMessage(e));
    }
    for (const key of TASKAPP_KEYS) void queryClient.invalidateQueries({ queryKey: [key] });
  };

  const s = status.data;

  return (
    <section aria-labelledby="page-taskapp" className="space-y-4">
      <PageHeader
        id="page-taskapp"
        title="Taskapp"
        description="Creá tareas desde el correo y mirá tus pendientes de Taskapp."
      />
      {status.isPending ? null : s?.connected ? (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-sm">
            Conectado como {s.userName} ({s.userEmail})
          </p>
          <Button type="button" variant="outline" size="sm" onClick={() => setConfirming(true)}>
            Desconectar
          </Button>
        </div>
      ) : waiting ? (
        <div className="flex flex-wrap items-center gap-3">
          <p className="text-sm text-muted-foreground">Esperando la autorización en el navegador…</p>
          <Button type="button" variant="outline" size="sm" onClick={cancel}>
            Cancelar
          </Button>
        </div>
      ) : (
        <div className="space-y-2">
          <p className="text-sm text-muted-foreground">
            Conectá tu cuenta de Taskapp para crear tareas desde el correo y ver tus pendientes.
          </p>
          <Button type="button" onClick={connect}>
            Conectar con Taskapp
          </Button>
          <p className="text-xs text-muted-foreground">
            Se abre el navegador para que autorices la conexión. La página dice «Autorizar TaskApp CLI»: es el mismo
            permiso.
          </p>
        </div>
      )}
      <Advanced />
      <Dialog open={confirming} onOpenChange={setConfirming}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Desconectar Taskapp?</DialogTitle>
            <DialogDescription>
              Dejás de ver tus pendientes y de crear tareas allá. Las tareas locales no se tocan.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => setConfirming(false)}>
              Cancelar
            </Button>
            <Button type="button" variant="destructive" onClick={() => void disconnect()}>
              Desconectar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

function Advanced() {
  const { settings, save } = useSaveSetting();
  return (
    <Collapsible className="border-t pt-3">
      <CollapsibleTrigger className="group flex items-center gap-1 text-sm font-medium">
        <ChevronRightIcon className="size-4 transition-transform group-data-[state=open]:rotate-90" aria-hidden />
        Avanzado
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-3 pt-3">
        <p className="text-sm text-muted-foreground">Dejalas vacías para usar las direcciones por defecto.</p>
        <UrlField
          id="taskapp-api-url"
          label="URL de la API"
          value={settings.data?.taskappApiUrl ?? ""}
          disabled={settings.isPending}
          onSave={(v) => save({ taskappApiUrl: v })}
        />
        <UrlField
          id="taskapp-web-url"
          label="URL de Taskapp"
          value={settings.data?.taskappWebUrl ?? ""}
          disabled={settings.isPending}
          onSave={(v) => save({ taskappWebUrl: v })}
        />
      </CollapsibleContent>
    </Collapsible>
  );
}

// Guarda al salir del campo. Si el core la rechaza, avisa y vuelve al valor guardado.
function UrlField({ id, label, value, disabled, onSave }: {
  id: string;
  label: string;
  value: string;
  disabled: boolean;
  onSave: (v: string) => Promise<boolean>;
}) {
  const [draft, setDraft] = useState<string | null>(null);
  const shown = draft ?? value;
  const commit = async () => {
    if (draft == null) return;
    const v = draft.trim();
    if (v === value) {
      setDraft(null);
      return;
    }
    const ok = await onSave(v);
    // Con éxito, el valor guardado llega por la consulta; si no, se revierte.
    if (!ok) setDraft(null);
  };
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={shown}
        disabled={disabled}
        placeholder="Por defecto"
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => void commit()}
      />
    </div>
  );
}

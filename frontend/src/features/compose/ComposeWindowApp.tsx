import { useQueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { useEffect, useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { parseInit } from "@/lib/composeWindow";
import { errorMessage } from "@/lib/errors";
import { useComposeDataEvents } from "@/lib/events";
import { Composer } from "./Composer";
import type { ComposerInit } from "./ComposerContext";

type State = { init: ComposerInit } | { error: string } | null;

// ComposeWindowApp es todo lo que corre en una ventana del redactor: pide con
// qué arranca y monta el Composer. No hay MailShell ni useMailEvents; las
// cuentas, firmas y sugerencias las pide el Composer con sus queries, que se
// recargan cuando cambian.
export function ComposeWindowApp({ id }: { id: number }) {
  const [state, setState] = useState<State>(null);
  useComposeDataEvents(useQueryClient());

  useEffect(() => {
    let alive = true;
    svc
      .ComposeInit(id)
      .then((json) => {
        if (alive) setState({ init: parseInit(json) });
      })
      .catch((e) => {
        if (alive) setState({ error: errorMessage(e) });
      });
    return () => {
      alive = false;
    };
  }, [id]);

  const ready = state !== null && "init" in state;

  // Sin redactor (cargando o con error) no hay nada que guardar: la ✕ cierra
  // y la salida de la app no espera.
  useEffect(() => {
    if (ready) return;
    const offs = [
      Events.On("compose:close-request", () => void svc.CloseComposeWindow(id)),
      Events.On("compose:flush", () => void svc.ComposeFlushed(id, true, "")),
    ];
    return () => offs.forEach((off) => off());
  }, [id, ready]);

  if (state === null) {
    return <div className="flex h-screen items-center justify-center text-sm text-muted-foreground">Cargando…</div>;
  }
  if ("error" in state) {
    return (
      <div className="p-4">
        <Alert variant="destructive">
          <AlertDescription>
            <p>No pudimos abrir este correo.</p>
            <p>{state.error}</p>
          </AlertDescription>
        </Alert>
      </div>
    );
  }
  return <Composer init={state.init} windowId={id} onClose={() => void svc.CloseComposeWindow(id)} />;
}

import { Events } from "@wailsio/runtime";
import { useEffect, useRef } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "@/lib/errors";

// Payload de mail:send-finished (core/mail/events.go).
type SendFinished = { draftId: number; ok: boolean; message: string; warning: string };

export function sendToastId(draftId: number): string {
  return `send-${draftId}`;
}

// El evento puede llegar antes que la respuesta de QueueSend: lo que ya
// terminó no vuelve a mostrar "Enviando…".
const started = new Set<number>();
const finished = new Set<number>();

// Cuánto espera el correo antes de salir (app/mailservice.go, undoSendDelay):
// mientras tanto el aviso ofrece Deshacer.
export const UNDO_SEND_MS = 10_000;

// Lo registra useSendNotices: si se puede deshacer y cómo abrir un correo de
// la bandeja de salida.
const undo: { enabled: boolean; open: (draftId: number) => void } = { enabled: false, open: () => {} };

async function cancelSend(draftId: number) {
  try {
    await svc.CancelSend(draftId);
  } catch (e) {
    toast.error(errorMessage(e));
    return;
  }
  started.delete(draftId);
  toast.success("Envío cancelado", {
    id: sendToastId(draftId),
    description: "Quedó en la Bandeja de salida. Lo abrimos para que sigas editando.",
    duration: 5000,
  });
  undo.open(draftId);
}

// sendStarted muestra el aviso "Enviando…" del correo draftId de la bandeja
// de salida, que se reemplaza cuando llega el resultado. Con deshacer envío
// ofrece Deshacer mientras el correo espera para salir.
export function sendStarted(draftId: number): void {
  if (finished.delete(draftId)) return;
  started.add(draftId);
  const id = sendToastId(draftId);
  if (!undo.enabled) {
    toast.loading("Enviando correo…", { id });
    return;
  }
  toast.loading("Enviando correo…", { id, action: { label: "Deshacer", onClick: () => void cancelSend(draftId) } });
  setTimeout(() => {
    if (started.has(draftId)) toast.loading("Enviando correo…", { id });
  }, UNDO_SEND_MS);
}

function sendFinished(ev: SendFinished, open: (draftId: number) => void) {
  if (!started.delete(ev.draftId)) finished.add(ev.draftId);
  const id = sendToastId(ev.draftId);
  if (ev.ok) {
    toast.success("Correo enviado", { id, duration: 4000 });
    if (ev.warning) toast.warning(ev.warning);
    return;
  }
  toast.error("No se pudo enviar el correo", {
    id,
    description: `${ev.message} Quedó en la Bandeja de salida.`,
    duration: Infinity,
    closeButton: true,
    action: { label: "Abrir", onClick: () => open(ev.draftId) },
  });
}

// useSendNotices convierte los resultados de los envíos en avisos. open abre
// el correo de la bandeja de salida que falló (o cuyo envío se deshizo).
// undoEnabled: si los envíos esperan para poder deshacerse.
export function useSendNotices(open: (draftId: number) => void, undoEnabled = false): void {
  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
    undo.open = (id) => openRef.current(id);
    undo.enabled = undoEnabled;
  }, [open, undoEnabled]);
  useEffect(
    () =>
      Events.On("mail:send-finished", (ev) => {
        sendFinished(ev.data as SendFinished, (id) => openRef.current(id));
      }),
    [],
  );
}

// Solo para tests.
export function resetSendNoticesForTests(): void {
  started.clear();
  finished.clear();
  undo.enabled = false;
}

import { Events } from "@wailsio/runtime";
import { useEffect, useRef } from "react";
import { toast } from "sonner";

// Payload de mail:send-finished (core/mail/events.go).
type SendFinished = { draftId: number; ok: boolean; message: string; warning: string };

export function sendToastId(draftId: number): string {
  return `send-${draftId}`;
}

// El evento puede llegar antes que la respuesta de QueueSend: lo que ya
// terminó no vuelve a mostrar "Enviando…".
const started = new Set<number>();
const finished = new Set<number>();

// sendStarted muestra el aviso "Enviando…" del correo draftId de la bandeja
// de salida, que se reemplaza cuando llega el resultado.
export function sendStarted(draftId: number): void {
  if (finished.delete(draftId)) return;
  started.add(draftId);
  toast.loading("Enviando correo…", { id: sendToastId(draftId) });
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
// el correo de la bandeja de salida que falló.
export function useSendNotices(open: (draftId: number) => void): void {
  const openRef = useRef(open);
  useEffect(() => {
    openRef.current = open;
  }, [open]);
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
}

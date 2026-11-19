import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { CalendarEvent, EventSave } from "../../../bindings/postacontrol/core/model/models";
import { toast } from "sonner";
import { goTime } from "@/lib/calendar";
import { isSendPartialError, errorMessage } from "@/lib/errors";
import type { useChoice } from "./useChoice";

type Ask = Pick<ReturnType<typeof useChoice>, "askScope" | "askNotify">;

// De qué ocurrencia se trata: solo importa si el evento se repite.
export type Occurrence = { recurring: boolean; occurrenceAt: string };

// Guarda un evento local: alcance (si se repite y ya existe), aviso (si hace
// falta) y guardado. false = se canceló en alguna pregunta y no se guardó.
// Un alta no pregunta: con invitados siempre manda la invitación.
export async function saveEventFlow(
  ask: Ask,
  event: CalendarEvent,
  occ: Occurrence | null,
  // Se tocó la repetición: "Solo este" no la cambiaría, no se ofrece.
  repeatChanged = false,
): Promise<boolean> {
  const isNew = event.id === 0;
  let scope: "" | "this" | "following" = "";
  if (!isNew && occ?.recurring) {
    const s = await ask.askScope(!repeatChanged);
    if (s == null) return false;
    scope = s;
  }
  const req: EventSave = { event, scope, occurrenceAt: goTime(occ?.occurrenceAt), notify: false };
  if (!isNew) {
    const prev = await svc.PreviewEventSave(req);
    if (prev.notifyNeeded) {
      const notify = await ask.askNotify(prev.added, prev.removed);
      if (notify == null) return false;
      req.notify = notify;
    }
  }
  try {
    await svc.SaveEvent(req);
  } catch (e) {
    // Quedó guardado; falló solo el correo: se avisa y se da por hecho.
    if (!isSendPartialError(e)) throw e;
    toast.warning(errorMessage(e));
  }
  return true;
}

// Borra un evento local: alcance (si se repite), aviso (si hace falta) y
// borrado. false = se canceló y no se borró.
export async function deleteEventFlow(ask: Ask, id: number, occ: Occurrence): Promise<boolean> {
  let scope: "" | "this" | "following" = "";
  if (occ.recurring) {
    const s = await ask.askScope();
    if (s == null) return false;
    scope = s;
  }
  const req = { id, scope, occurrenceAt: goTime(occ.occurrenceAt), notify: false };
  const prev = await svc.PreviewEventDelete(req);
  if (prev.notifyNeeded) {
    const notify = await ask.askNotify(prev.added, prev.removed);
    if (notify == null) return false;
    req.notify = notify;
  }
  try {
    await svc.DeleteEvent(req);
  } catch (e) {
    // Quedó borrado; falló solo el correo: se avisa y se da por hecho.
    if (!isSendPartialError(e)) throw e;
    toast.warning(errorMessage(e));
  }
  return true;
}

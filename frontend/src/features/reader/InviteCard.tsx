import { CalendarIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Attachment } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { formatItemWhen } from "@/lib/calendar";
import { errorMessage } from "@/lib/errors";
import { requestOpenEvent } from "@/lib/events";
import { useMessageInvite } from "@/lib/queries";

export function isCalendar(a: Pick<Attachment, "mimeType" | "filename">): boolean {
  const mime = a.mimeType.toLowerCase();
  return mime.startsWith("text/calendar") || mime === "application/ics" || a.filename.toLowerCase().endsWith(".ics");
}

const RESPONSES = [
  { partstat: "ACCEPTED", label: "Aceptar" },
  { partstat: "TENTATIVE", label: "Tal vez" },
  { partstat: "DECLINED", label: "Rechazar" },
] as const;

const REPLY_VERB: Record<string, string> = {
  ACCEPTED: "aceptó la invitación a",
  DECLINED: "rechazó la invitación a",
  TENTATIVE: "respondió tal vez a la invitación a",
};

// La invitación de un correo, según la vista del núcleo: se responde desde acá.
// No muestra nada mientras carga ni si el correo no trae una invitación.
export function InviteCard({
  messageId,
  attachment,
  accountEmail,
}: {
  messageId: number;
  // El adjunto de calendario del correo, para "Guardar .ics".
  attachment?: Attachment;
  // La cuenta que va a responder (se nombra en el aviso de alias).
  accountEmail?: string;
}) {
  const query = useMessageInvite(messageId);
  const [busy, setBusy] = useState(false);
  const inv = query.data;
  if (!inv?.found) return null;

  const respond = async (partstat: string) => {
    setBusy(true);
    try {
      const res = await svc.RespondInvite(inv.eventId, partstat);
      if (res?.warning) toast.warning(res.warning);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const remove = async () => {
    setBusy(true);
    try {
      await svc.RemoveCancelledEvent(inv.eventId);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };
  const save = async () => {
    if (!attachment) return;
    try {
      await svc.SaveAttachment(messageId, attachment.index);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  const openInCalendar = () =>
    requestOpenEvent({ key: `e:${inv.eventId}:${Math.floor(Date.parse(inv.start) / 1000)}`, at: inv.start });

  const inCalendar = inv.eventId > 0;
  const organizer = inv.organizer.name || inv.organizer.email;
  const replier = inv.replyFrom.name || inv.replyFrom.email;
  const canRespond =
    inv.method === "REQUEST" && inCalendar && !inv.cancelled && !inv.outdated && !inv.past;

  return (
    <section aria-label="Invitación" className="mx-6 mt-4 flex items-start gap-3 rounded-md border px-3 py-2.5 text-sm">
      <CalendarIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1 space-y-1">
        {inv.method === "REPLY" ? (
          <p>
            {replier} {REPLY_VERB[inv.replyStatus] ?? "respondió la invitación a"} «{inv.title}».
          </p>
        ) : (
          <>
            <p className="font-medium">
              {inv.cancelled && <span className="mr-1.5 text-destructive">Cancelada</span>}
              {inv.title || "Invitación"}
            </p>
            {inv.start && (
              <p>
                <span className="capitalize">{formatItemWhen(inv)}</span>
                {inv.recurring && <span className="text-muted-foreground"> · se repite</span>}
              </p>
            )}
            {inv.location && <p className="text-muted-foreground">{inv.location}</p>}
            {organizer && <p className="text-xs text-muted-foreground">Organiza {organizer}</p>}
          </>
        )}
        {inv.method === "REQUEST" && !inv.cancelled && inv.outdated && (
          <p className="text-xs text-muted-foreground">Hay una versión más nueva de esta invitación</p>
        )}
        {inv.method === "REQUEST" && !inv.cancelled && !inv.outdated && inv.past && (
          <p className="text-xs text-muted-foreground">Este evento ya pasó</p>
        )}
        {canRespond && inv.needsReview && (
          <p className="text-xs text-amber-600 dark:text-amber-400">Cambió la hora: volvé a responder</p>
        )}
        {canRespond && inv.aliasReply && (
          <p className="text-xs text-muted-foreground">
            Ninguna de tus cuentas figura como invitada; vas a responder desde {accountEmail}
          </p>
        )}
        {canRespond && (
          <div className="flex flex-wrap gap-2 pt-1">
            {RESPONSES.map((r) => (
              <Button
                key={r.partstat}
                size="sm"
                variant={inv.partstat === r.partstat ? "default" : "outline"}
                aria-pressed={inv.partstat === r.partstat}
                disabled={busy}
                onClick={() => void respond(r.partstat)}
              >
                {r.label}
              </Button>
            ))}
          </div>
        )}
      </div>
      <div className="flex shrink-0 flex-col items-end gap-2">
        {inv.method === "CANCEL" && inCalendar && (
          <Button size="sm" variant="outline" disabled={busy} onClick={() => void remove()}>
            Quitar del calendario
          </Button>
        )}
        {inv.method === "REQUEST" && inCalendar && !inv.cancelled && (
          <Button size="sm" variant="ghost" onClick={openInCalendar}>
            Ver en el calendario
          </Button>
        )}
        {attachment && (
          <Button size="sm" variant="outline" onClick={() => void save()}>
            Guardar .ics
          </Button>
        )}
      </div>
    </section>
  );
}

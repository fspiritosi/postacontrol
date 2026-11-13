import { useQuery } from "@tanstack/react-query";
import { CalendarIcon } from "lucide-react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Attachment } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { base64ToBytes } from "@/lib/attachments";
import { errorMessage } from "@/lib/errors";
import { formatInviteWhen, parseIcs } from "@/lib/ics";

export function isCalendar(a: Pick<Attachment, "mimeType" | "filename">): boolean {
  const mime = a.mimeType.toLowerCase();
  return mime.startsWith("text/calendar") || mime === "application/ics" || a.filename.toLowerCase().endsWith(".ics");
}

// Una invitación de calendario adjunta, leída: qué, cuándo, dónde y quién.
export function InviteCard({ messageId, attachment }: { messageId: number; attachment: Attachment }) {
  const invite = useQuery({
    queryKey: ["invite", messageId, attachment.index],
    queryFn: async () => {
      const data = await svc.AttachmentData(messageId, attachment.index);
      return parseIcs(new TextDecoder().decode(base64ToBytes(data.data ?? "")));
    },
    retry: false,
    staleTime: Infinity,
  });
  const ev = invite.data;
  if (!ev) return null;

  const save = async () => {
    try {
      await svc.SaveAttachment(messageId, attachment.index);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <section aria-label="Invitación" className="mx-6 mt-4 flex items-start gap-3 rounded-md border px-3 py-2.5 text-sm">
      <CalendarIcon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="font-medium">
          {ev.cancelled && <span className="mr-1.5 text-destructive">Cancelada:</span>}
          {ev.summary || "Invitación"}
        </p>
        {ev.start && <p>{formatInviteWhen(ev)}</p>}
        {ev.location && <p className="text-muted-foreground">{ev.location}</p>}
        {ev.organizer && <p className="text-xs text-muted-foreground">Organiza {ev.organizer}</p>}
      </div>
      <Button size="sm" variant="outline" onClick={() => void save()}>
        Guardar .ics
      </Button>
    </section>
  );
}

import { useQueryClient } from "@tanstack/react-query";
import { ExternalLinkIcon, PencilIcon, Trash2Icon } from "lucide-react";
import type { ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { CalendarItem } from "../../../bindings/postacontrol/core/model/models";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { formatItemWhen } from "@/lib/calendar";
import { errorMessage } from "@/lib/errors";
import { useEvent } from "@/lib/queries";
import type { ShellState } from "../layout/MailShell";
import { useChoice } from "./useChoice";

const PARTSTAT: Record<string, string> = {
  ACCEPTED: "Aceptó",
  TENTATIVE: "Tal vez",
  DECLINED: "Rechazó",
  "NEEDS-ACTION": "Sin responder",
};

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="select-text">{children}</dd>
    </div>
  );
}

// Detalle de lo elegido en el calendario; lo que se puede hacer depende de su origen.
export function EventDetail({
  item,
  state,
  onEdit,
}: {
  item: CalendarItem;
  state: ShellState;
  onEdit?: (item: CalendarItem) => void;
}) {
  const qc = useQueryClient();
  const { askScope, askNotify, dialogs } = useChoice();
  const hasEvent = item.source === "local" || item.source === "invite";
  const event = useEvent(hasEvent ? item.eventId : null);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["calendar"] });
    void qc.invalidateQueries({ queryKey: ["event"] });
  };
  // Corre una acción: avisa si falla y recarga si salió bien.
  const run = async (fn: () => Promise<void | { warning: string }>, after?: () => void) => {
    try {
      const r = await fn();
      if (r && r.warning) toast.warning(r.warning);
      refresh();
      after?.();
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const remove = async () => {
    try {
      let scope: "" | "this" | "following" = "";
      if (item.recurring) {
        const s = await askScope();
        if (s == null) return;
        scope = s;
      }
      const req = { id: item.eventId, scope, occurrenceAt: item.occurrenceAt, notify: false };
      const prev = await svc.PreviewEventDelete(req);
      if (prev.notifyNeeded) {
        const notify = await askNotify(prev.added, prev.removed);
        if (notify == null) return;
        req.notify = notify;
      }
      await svc.DeleteEvent(req);
      refresh();
      state.selectEvent(null);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const attendees = event.data?.attendees ?? [];
  const organizer = event.data?.organizer;

  return (
    <div className="h-full overflow-y-auto p-5">
      <div className="space-y-4">
        <h2 className={`text-lg font-semibold select-text ${item.cancelled ? "line-through" : ""}`}>{item.title}</h2>
        <dl className="space-y-3 text-sm">
          <Row label="Cuándo">
            <span className="capitalize">{formatItemWhen(item)}</span>
            {item.recurring && <span className="text-muted-foreground"> · se repite</span>}
          </Row>
          {item.location && <Row label="Lugar">{item.location}</Row>}
          {item.projectName && <Row label="Proyecto">{item.projectName}</Row>}
          {item.source === "invite" && organizer && (organizer.name || organizer.email) && (
            <Row label="Organiza">{organizer.name ? `${organizer.name} <${organizer.email}>` : organizer.email}</Row>
          )}
          {item.source === "taskapp" && item.attendees > 0 && (
            <Row label="Asistentes">
              {item.attendees} · {item.confirmed} confirmaron
            </Row>
          )}
          {hasEvent && attendees.length > 0 && (
            <Row label={`Invitados (${item.confirmed}/${item.attendees} confirmaron)`}>
              <ul className="space-y-0.5">
                {attendees.map((a) => (
                  <li key={a.email} className="flex justify-between gap-2">
                    <span className="truncate">{a.name || a.email}</span>
                    <span className="shrink-0 text-xs text-muted-foreground">{PARTSTAT[a.partstat] ?? a.partstat}</span>
                  </li>
                ))}
              </ul>
            </Row>
          )}
          {item.source === "invite" && item.partstat && (
            <Row label="Tu respuesta">{PARTSTAT[item.partstat] ?? item.partstat}</Row>
          )}
          {event.data?.notes && <Row label="Notas">{event.data.notes}</Row>}
        </dl>

        {item.source === "local" && (
          <div className="flex gap-2">
            <Button variant="outline" size="sm" onClick={() => onEdit?.(item)}>
              <PencilIcon /> Editar
            </Button>
            <Button variant="destructive" size="sm" onClick={() => void remove()}>
              <Trash2Icon /> Borrar
            </Button>
          </div>
        )}

        {item.source === "invite" && (
          <div className="space-y-3">
            {item.needsReview && (
              <Alert>
                <AlertDescription>Cambió: revisá</AlertDescription>
              </Alert>
            )}
            {item.cancelled ? (
              <Button
                variant="outline"
                size="sm"
                onClick={() => void run(() => svc.RemoveCancelledEvent(item.eventId), () => state.selectEvent(null))}
              >
                Quitar
              </Button>
            ) : (
              <div className="flex flex-wrap gap-2">
                {(
                  [
                    ["Aceptar", "ACCEPTED"],
                    ["Tal vez", "TENTATIVE"],
                    ["Rechazar", "DECLINED"],
                  ] as const
                ).map(([label, partstat]) => (
                  <Button
                    key={partstat}
                    variant={item.partstat === partstat ? "default" : "outline"}
                    size="sm"
                    onClick={() => void run(() => svc.RespondInvite(item.eventId, partstat))}
                  >
                    {label}
                  </Button>
                ))}
              </div>
            )}
          </div>
        )}

        {item.source === "taskapp" && (
          <div className="space-y-3">
            {item.taskappChange && (
              <Alert>
                <AlertDescription className="space-y-2">
                  <p>Cambió en Taskapp: ¿avisar a los invitados externos?</p>
                  <div className="flex gap-2">
                    <Button size="xs" onClick={() => void run(() => svc.NotifyTaskappChange(item.eventId))}>
                      Avisar
                    </Button>
                    <Button variant="outline" size="xs" onClick={() => void run(() => svc.DismissTaskappChange(item.eventId))}>
                      Ahora no
                    </Button>
                  </div>
                </AlertDescription>
              </Alert>
            )}
            <div className="flex flex-wrap gap-2">
              <Button size="sm" onClick={() => void run(() => svc.OpenTaskInTaskapp(item.url))}>
                <ExternalLinkIcon /> Abrir en Taskapp
              </Button>
              {item.attendees > 0 && (
                <Button variant="outline" size="sm" onClick={() => void run(() => svc.RemoveTaskappExternals(item.eventId))}>
                  Quitar invitados externos
                </Button>
              )}
            </div>
          </div>
        )}

        {item.source === "task" && (
          <Button size="sm" onClick={() => state.openTask(item.taskId)}>
            Abrir tarea
          </Button>
        )}
      </div>
      {dialogs}
    </div>
  );
}

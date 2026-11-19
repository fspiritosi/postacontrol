import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Address, CalendarEvent } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { localDay, localTime } from "@/lib/calendar";
import { errorMessage, isOrphanError } from "@/lib/errors";
import { useAccounts, useEvent, useTaskappMembers, useTaskappStatus, useTaskappWorkspaces } from "@/lib/queries";
import { formatRecipients, parseRecipients } from "@/lib/recipients";
import { buildRRule, describeRRule, parseRRule, type Repeat } from "@/lib/rrule";
import { REMINDER_OPTIONS } from "@/lib/tasks";
import { RecipientInput } from "../compose/RecipientInput";
import { showTaskError } from "../tasks/CreateTaskDialog";
import { deleteEventFlow, saveEventFlow } from "./eventActions";
import { useChoice } from "./useChoice";

// Con qué se abre el formulario. Los tiempos son los que se ven: en un día
// completo, medianoches locales y fin exclusivo (como los da FullCalendar).
export type EventInitial = {
  start: Date;
  end: Date;
  allDay: boolean;
  // Se abrió desde una ocurrencia de un evento que se repite.
  recurring?: boolean;
  occurrenceAt?: string;
};

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  initial: EventInitial;
  // null = evento nuevo
  eventId: number | null;
};

type Dest = "local" | "taskapp";

const DAY_MS = 86_400_000;
const DAY_LETTERS = ["L", "M", "X", "J", "V", "S", "D"];
const DAY_NAMES = ["lunes", "martes", "miércoles", "jueves", "viernes", "sábado", "domingo"];
const FREQ_OPTIONS: [Repeat["freq"], string][] = [
  ["none", "No se repite"],
  ["daily", "Todos los días"],
  ["weekly", "Todas las semanas"],
  ["monthly", "Todos los meses"],
  ["yearly", "Todos los años"],
];
const DEFAULT_REMINDER = "default";
const CUSTOM = "custom";

const EMPTY_EVENT: CalendarEvent = {
  id: 0, uid: "", kind: "local", title: "", description: "", notes: "", location: "", allDay: false,
  start: "", end: "", tzid: "", rrule: "", remindBefore: null, accountId: 0,
  organizer: { name: "", email: "" }, sequence: 0, status: "confirmed", myPartstat: "", myEmail: "",
  aliasReply: false, needsReview: false, taskappMeetingId: 0, taskappChange: "", attendees: [], exceptions: [],
};

// Un día completo es la medianoche UTC de la fecha elegida.
const utcDay = (day: string) => {
  const [y, m, d] = day.split("-").map(Number);
  return Date.UTC(y, m - 1, d);
};
const localDate = (day: string, time: string) => new Date(`${day}T${time || "00:00"}`);

export function EventDialog({ open, onOpenChange, initial, eventId }: Props) {
  // Se monta de nuevo en cada apertura: parte siempre de lo que llega.
  return open ? <Flow initial={initial} eventId={eventId} onOpenChange={onOpenChange} /> : null;
}

function Flow({ initial, eventId, onOpenChange }: Omit<Props, "open">) {
  const event = useEvent(eventId);
  if (eventId != null && event.isError) {
    return (
      <Dialog open onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>Editar evento</DialogTitle>
            <DialogDescription>{errorMessage(event.error)}</DialogDescription>
          </DialogHeader>
        </DialogContent>
      </Dialog>
    );
  }
  if (eventId != null && !event.data) return null;
  return <Form initial={initial} eventId={eventId} event={event.data ?? EMPTY_EVENT} onOpenChange={onOpenChange} />;
}

function Form({
  initial,
  eventId,
  event,
  onOpenChange,
}: Omit<Props, "open"> & { event: CalendarEvent }) {
  const qc = useQueryClient();
  const choice = useChoice();
  const editing = eventId != null;
  const status = useTaskappStatus();
  const accounts = useAccounts();
  const connected = status.data?.connected === true;

  const [chosen, setChosen] = useState<Dest>("local");
  const dest: Dest = connected && !editing ? chosen : "local";

  const [title, setTitle] = useState(event.title);
  const [allDay, setAllDay] = useState(initial.allDay);
  const [startDay, setStartDay] = useState(localDay(initial.start));
  const [startTime, setStartTime] = useState(initial.allDay ? "09:00" : localTime(initial.start));
  // El fin de un día completo se muestra inclusivo (el último día).
  const [endDay, setEndDay] = useState(() =>
    localDay(initial.allDay ? new Date(initial.end.getFullYear(), initial.end.getMonth(), initial.end.getDate() - 1) : initial.end),
  );
  const [endTime, setEndTime] = useState(initial.allDay ? "10:00" : localTime(initial.end));
  const [location, setLocation] = useState(event.location);
  const [description, setDescription] = useState(event.description);
  const [notes, setNotes] = useState(event.notes);
  const [saving, setSaving] = useState(false);

  // Repetición: la que armó el formulario, o la del evento si es de otra clase.
  const parsed = parseRRule(event.rrule);
  const [repeat, setRepeat] = useState<Repeat | typeof CUSTOM>(parsed ?? CUSTOM);
  const [repeatTouched, setRepeatTouched] = useState(false);
  const [reminder, setReminder] = useState(event.remindBefore == null ? DEFAULT_REMINDER : String(event.remindBefore));
  const [invitees, setInvitees] = useState(() => formatRecipients(event.attendees));
  const [fromId, setFromId] = useState<number | null>(event.accountId || null);

  // Reunión en Taskapp
  const [projectId, setProjectId] = useState(0);
  const [workspaceChoice, setWorkspaceChoice] = useState<number | null>(null);
  const [checked, setChecked] = useState<Set<number> | null>(null);
  const [external, setExternal] = useState("");

  const projects = useQuery({
    queryKey: ["taskapp-projects"],
    queryFn: async () => (await svc.TaskappProjects()) ?? [],
    enabled: connected && dest === "taskapp",
  });
  const workspaces = useTaskappWorkspaces();
  const project = projects.data?.find((p) => p.id === projectId);
  const workspaceList = workspaces.data ?? [];
  const workspaceId = project?.workspaceId ?? workspaceChoice ?? (workspaceList.length === 1 ? workspaceList[0].id : null);
  const members = useTaskappMembers(dest === "taskapp" ? workspaceId : null);
  const memberList = members.data ?? [];
  const userId = status.data?.userId ?? 0;
  // El backend no suma al que crea: viene marcado de entrada.
  const attendeeIds = checked ?? new Set(userId ? [userId] : []);

  const mine = parseRecipients(dest === "local" ? invitees : external);
  const hasGuests = mine.valid.length > 0;
  const lowestAccount = [...(accounts.data ?? [])].sort((a, b) => a.id - b.id)[0]?.id ?? 0;
  const accountId = fromId ?? lowestAccount;

  // Mover el inicio mueve el fin igual: la duración se conserva.
  const changeStart = (day: string, time: string) => {
    if (allDay) {
      const span = Math.max(0, Math.round((utcDay(endDay) - utcDay(startDay)) / DAY_MS));
      setEndDay(new Date(utcDay(day) + span * DAY_MS).toISOString().slice(0, 10));
    } else {
      const dur = localDate(endDay, endTime).getTime() - localDate(startDay, startTime).getTime();
      const end = new Date(localDate(day, time).getTime() + (dur > 0 ? dur : 0));
      setEndDay(localDay(end));
      setEndTime(localTime(end));
    }
    setStartDay(day);
    setStartTime(time);
  };

  // Los instantes a guardar: con hora, el momento local; todo el día, la
  // medianoche UTC de la fecha, con el fin exclusivo.
  const times = () => {
    if (allDay) return { start: new Date(utcDay(startDay)), end: new Date(utcDay(endDay) + DAY_MS) };
    return { start: localDate(startDay, startTime), end: localDate(endDay, endTime) };
  };
  const t = times();
  const validTimes = !isNaN(t.start.getTime()) && !isNaN(t.end.getTime()) && t.end > t.start;

  const canSave =
    !saving &&
    title.trim() !== "" &&
    validTimes &&
    mine.invalid.length === 0 &&
    (dest === "local" || workspaceId != null);

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: ["calendar"] });
    void qc.invalidateQueries({ queryKey: ["event"] });
  };

  const saveLocal = async () => {
    // Sin tocar la repetición, la regla va tal cual (una importada no se rearma).
    const rrule = repeatTouched && repeat !== CUSTOM ? buildRRule(repeat, t.start, allDay) : event.rrule;
    const next: CalendarEvent = {
      ...event,
      title: title.trim(),
      description,
      notes,
      location,
      allDay,
      start: t.start.toISOString(),
      end: t.end.toISOString(),
      rrule,
      remindBefore: reminder === DEFAULT_REMINDER ? null : Number(reminder),
      accountId,
      attendees: mine.valid.map((a) => ({
        email: a.email,
        name: a.name,
        partstat: event.attendees?.find((x) => x.email.toLowerCase() === a.email.toLowerCase())?.partstat ?? "",
      })),
    };
    const occ = { recurring: initial.recurring ?? false, occurrenceAt: initial.occurrenceAt ?? "" };
    try {
      if (await saveEventFlow(choice, next, occ, repeatTouched)) onOpenChange(false);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      refresh();
    }
  };

  const saveMeeting = async () => {
    try {
      await svc.CreateTaskappMeeting({
        workspaceId: workspaceId!,
        projectId,
        title: title.trim(),
        description,
        location,
        allDay,
        start: t.start.toISOString(),
        end: t.end.toISOString(),
        attendeeIds: memberList.filter((m) => attendeeIds.has(m.id)).map((m) => m.id),
        external: mine.valid as Address[],
        accountId,
      });
      toast.success("Reunión creada en Taskapp");
      onOpenChange(false);
    } catch (e) {
      showTaskError(e);
      // La reunión quedó creada a medias: no se puede volver a crear.
      if (isOrphanError(e)) onOpenChange(false);
    } finally {
      refresh();
    }
  };

  const submit = async () => {
    if (!canSave) return;
    setSaving(true);
    try {
      await (dest === "local" ? saveLocal() : saveMeeting());
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    setSaving(true);
    try {
      const occ = { recurring: initial.recurring ?? false, occurrenceAt: initial.occurrenceAt ?? "" };
      if (await deleteEventFlow(choice, eventId!, occ)) onOpenChange(false);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      refresh();
      setSaving(false);
    }
  };

  const weekly = repeat !== CUSTOM && repeat.freq === "weekly";
  const showFrom = hasGuests;

  return (
    <>
      <Dialog open onOpenChange={onOpenChange}>
        <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editing ? "Editar evento" : "Nuevo evento"}</DialogTitle>
            <DialogDescription>
              {dest === "local" ? "Un evento de tu calendario local." : "Una reunión que se crea en Taskapp."}
            </DialogDescription>
          </DialogHeader>
          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              void submit();
            }}
          >
            {connected && !editing && (
              <fieldset className="space-y-1.5">
                <legend className="mb-1.5 text-sm font-medium">Destino</legend>
                {(
                  [
                    ["local", "Calendario local"],
                    ["taskapp", "Reunión en Taskapp"],
                  ] as const
                ).map(([v, label]) => (
                  <label key={v} className="flex items-center gap-2 text-sm">
                    <input type="radio" name="event-dest" checked={dest === v} onChange={() => setChosen(v)} />
                    {label}
                  </label>
                ))}
              </fieldset>
            )}

            <div className="space-y-2">
              <Label htmlFor="ev-title">Título</Label>
              <Input id="ev-title" value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
            </div>

            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={allDay} onChange={(e) => setAllDay(e.target.checked)} />
              Todo el día
            </label>

            <div className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-3">
              <div className="space-y-2">
                <Label htmlFor="ev-start-day">Inicio</Label>
                <Input
                  id="ev-start-day"
                  type="date"
                  value={startDay}
                  onChange={(e) => e.target.value && changeStart(e.target.value, startTime)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="ev-start-time">Hora de inicio</Label>
                <Input
                  id="ev-start-time"
                  type="time"
                  value={startTime}
                  disabled={allDay}
                  onChange={(e) => e.target.value && changeStart(startDay, e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="ev-end-day">Fin</Label>
                <Input id="ev-end-day" type="date" value={endDay} onChange={(e) => setEndDay(e.target.value)} />
              </div>
              <div className="space-y-2">
                <Label htmlFor="ev-end-time">Hora de fin</Label>
                <Input
                  id="ev-end-time"
                  type="time"
                  value={endTime}
                  disabled={allDay}
                  onChange={(e) => setEndTime(e.target.value)}
                />
              </div>
            </div>
            {!validTimes && <p className="text-xs text-destructive">El fin tiene que ser después del inicio.</p>}

            <div className="space-y-2">
              <Label htmlFor="ev-location">Lugar</Label>
              <Input id="ev-location" value={location} onChange={(e) => setLocation(e.target.value)} />
            </div>
            <div className="space-y-2">
              <Label htmlFor="ev-description">Descripción</Label>
              <Textarea
                id="ev-description"
                className="max-h-40"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
              <p className="text-xs text-muted-foreground">Va en la invitación</p>
            </div>
            {dest === "local" && (
              <div className="space-y-2">
                <Label htmlFor="ev-notes">Notas privadas</Label>
                <Textarea id="ev-notes" className="max-h-40" value={notes} onChange={(e) => setNotes(e.target.value)} />
                <p className="text-xs text-muted-foreground">Solo en esta PC</p>
              </div>
            )}

            {dest === "local" && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="ev-repeat">Repetición</Label>
                  <Select
                    value={repeat === CUSTOM ? CUSTOM : repeat.freq}
                    onValueChange={(v) => {
                      setRepeatTouched(true);
                      setRepeat(
                        v === CUSTOM
                          ? CUSTOM
                          : { freq: v as Repeat["freq"], days: v === "weekly" ? [(t.start.getDay() + 6) % 7] : [], until: repeat === CUSTOM ? "" : repeat.until },
                      );
                    }}
                  >
                    <SelectTrigger id="ev-repeat" className="w-full" aria-label="Repetición">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {parsed == null && <SelectItem value={CUSTOM}>{describeRRule(event.rrule)}</SelectItem>}
                      {FREQ_OPTIONS.map(([v, l]) => (
                        <SelectItem key={v} value={v}>
                          {l}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  {weekly && (
                    <div className="flex gap-1" role="group" aria-label="Días de la semana">
                      {DAY_LETTERS.map((l, i) => {
                        const on = repeat.days.includes(i);
                        return (
                          <Button
                            key={l + i}
                            type="button"
                            size="icon-sm"
                            variant={on ? "default" : "outline"}
                            aria-label={DAY_NAMES[i]}
                            aria-pressed={on}
                            onClick={() => {
                              setRepeatTouched(true);
                              setRepeat({ ...repeat, days: on ? repeat.days.filter((d) => d !== i) : [...repeat.days, i] });
                            }}
                          >
                            {l}
                          </Button>
                        );
                      })}
                    </div>
                  )}
                  {repeat !== CUSTOM && repeat.freq !== "none" && (
                    <div className="space-y-2">
                      <Label htmlFor="ev-until">Hasta</Label>
                      <Input
                        id="ev-until"
                        type="date"
                        value={repeat.until}
                        onChange={(e) => {
                          setRepeatTouched(true);
                          setRepeat({ ...repeat, until: e.target.value });
                        }}
                      />
                    </div>
                  )}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="ev-reminder">Aviso</Label>
                  <Select value={reminder} onValueChange={setReminder}>
                    <SelectTrigger id="ev-reminder" className="w-full" aria-label="Aviso">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value={DEFAULT_REMINDER}>Por defecto</SelectItem>
                      {REMINDER_OPTIONS.map((o) => (
                        <SelectItem key={o.minutes} value={String(o.minutes)}>
                          {o.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <RecipientInput id="ev-invitees" label="Invitados" value={invitees} onChange={setInvitees} />
              </>
            )}

            {dest === "taskapp" && (
              <>
                <div className="space-y-2">
                  <Label htmlFor="ev-project">Proyecto</Label>
                  <Select value={String(projectId)} onValueChange={(v) => setProjectId(Number(v))}>
                    <SelectTrigger id="ev-project" className="w-full" aria-label="Proyecto">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="0">Sin proyecto</SelectItem>
                      {(projects.data ?? []).map((p) => (
                        <SelectItem key={p.id} value={String(p.id)}>
                          <span className="inline-block size-2 rounded-full" style={{ backgroundColor: p.color }} aria-hidden />
                          {p.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                {project == null && workspaceList.length > 1 && (
                  <div className="space-y-2">
                    <Label htmlFor="ev-workspace">Workspace</Label>
                    <Select
                      value={workspaceId == null ? "" : String(workspaceId)}
                      onValueChange={(v) => {
                        setWorkspaceChoice(Number(v));
                        setChecked(null);
                      }}
                    >
                      <SelectTrigger id="ev-workspace" className="w-full" aria-label="Workspace">
                        <SelectValue placeholder="Elegí un workspace" />
                      </SelectTrigger>
                      <SelectContent>
                        {workspaceList.map((w) => (
                          <SelectItem key={w.id} value={String(w.id)}>
                            {w.name}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                )}
                {memberList.length > 0 && (
                  <fieldset className="space-y-1.5">
                    <legend className="mb-1.5 text-sm font-medium">Asistentes</legend>
                    <div className="max-h-40 space-y-1.5 overflow-y-auto">
                      {memberList.map((m) => (
                        <label key={m.id} className="flex items-center gap-2 text-sm">
                          <input
                            type="checkbox"
                            checked={attendeeIds.has(m.id)}
                            onChange={(e) => {
                              const next = new Set(attendeeIds);
                              if (e.target.checked) next.add(m.id);
                              else next.delete(m.id);
                              setChecked(next);
                            }}
                          />
                          {m.name || m.email}
                        </label>
                      ))}
                    </div>
                  </fieldset>
                )}
                <RecipientInput id="ev-external" label="Invitados externos" value={external} onChange={setExternal} />
              </>
            )}

            {showFrom && (
              <div className="space-y-2">
                <Label htmlFor="ev-from">Enviar desde</Label>
                <Select value={accountId ? String(accountId) : ""} onValueChange={(v) => setFromId(Number(v))}>
                  <SelectTrigger id="ev-from" className="w-full" aria-label="Enviar desde">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(accounts.data ?? []).map((a) => (
                      <SelectItem key={a.id} value={String(a.id)}>
                        {a.email}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            )}
            {mine.invalid.length > 0 && (
              <p className="text-xs text-destructive">Revisá los invitados: {mine.invalid.join(", ")}</p>
            )}

            <DialogFooter className="sm:justify-between">
              {editing ? (
                <Button type="button" variant="destructive" disabled={saving} onClick={() => void remove()}>
                  Borrar
                </Button>
              ) : (
                <span />
              )}
              <div className="flex flex-col-reverse gap-2 sm:flex-row">
                <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
                  Cancelar
                </Button>
                <Button type="submit" disabled={!canSave}>
                  Guardar
                </Button>
              </div>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      {choice.dialogs}
    </>
  );
}

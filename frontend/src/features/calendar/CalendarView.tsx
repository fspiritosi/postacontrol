import type { DatesSetArg, EventDropArg } from "@fullcalendar/core";
import esLocale from "@fullcalendar/core/locales/es";
import dayGridPlugin from "@fullcalendar/daygrid";
import interactionPlugin, { type EventResizeDoneArg } from "@fullcalendar/interaction";
import listPlugin from "@fullcalendar/list";
import FullCalendar from "@fullcalendar/react";
import timeGridPlugin from "@fullcalendar/timegrid";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { CalendarItem, EventSave } from "../../../bindings/postacontrol/core/model/models";
import { loadCalendarView, saveCalendarView, toFcEvent, utcMidnightOfLocal, type CalendarViewId } from "@/lib/calendar";
import { errorMessage } from "@/lib/errors";
import { useCalendar } from "@/lib/queries";
import type { ShellState } from "../layout/MailShell";
import { CalendarToolbar } from "./CalendarToolbar";
import { EventDetail } from "./EventDetail";
import { useChoice } from "./useChoice";
import "./calendar.css";

const PLUGINS = [dayGridPlugin, timeGridPlugin, listPlugin, interactionPlugin];
const VIEWS = { workWeek: { type: "timeGrid", duration: { weeks: 1 }, weekends: false } };
const HOUR = 3_600_000;

// Rango inicial hasta que FullCalendar avise el real con datesSet.
function initialRange() {
  const from = new Date();
  from.setHours(0, 0, 0, 0);
  from.setDate(from.getDate() - 7);
  return { from, to: new Date(from.getTime() + 21 * 24 * HOUR) };
}

export function CalendarView({
  state,
  onNewEvent,
  onEditEvent,
}: {
  state: ShellState;
  onNewEvent?: (start: Date, end: Date, allDay: boolean) => void;
  onEditEvent?: (item: CalendarItem) => void;
}) {
  const qc = useQueryClient();
  const calRef = useRef<FullCalendar>(null);
  const [view, setView] = useState<CalendarViewId>(loadCalendarView);
  const [initial] = useState(view);
  const [title, setTitle] = useState("");
  const [range, setRange] = useState(initialRange);
  const { askScope, askNotify, dialogs } = useChoice();
  const cal = useCalendar(range.from, range.to);
  const items = useMemo(() => cal.data?.items ?? [], [cal.data]);
  const selected = items.find((i) => i.key === state.eventKey) ?? null;

  const events = useMemo(
    () => items.map((i) => {
      const ev = toFcEvent(i);
      return i.key === state.eventKey ? { ...ev, classNames: [...(ev.classNames as string[]), "ev-selected"] } : ev;
    }),
    [items, state.eventKey],
  );

  // El aviso de un evento trae la fecha: se va a ella una sola vez.
  const wentTo = useRef<string | null>(null);
  useEffect(() => {
    if (!state.calendarDate || wentTo.current === state.calendarDate) return;
    wentTo.current = state.calendarDate;
    calRef.current?.getApi().gotoDate(new Date(state.calendarDate));
  }, [state.calendarDate]);

  const changeView = (v: CalendarViewId) => {
    setView(v);
    saveCalendarView(v);
    calRef.current?.getApi().changeView(v);
  };

  const onDatesSet = (a: DatesSetArg) => {
    setTitle(a.view.title);
    setRange({ from: a.start, to: a.end });
  };

  // Mueve o estira un evento local: alcance (si se repite), aviso (si hace
  // falta) y guardado. Si se cancela o falla, el evento vuelve a su lugar.
  const reschedule = async (info: EventDropArg | EventResizeDoneArg) => {
    const item = info.event.extendedProps.item as CalendarItem;
    try {
      let scope: "" | "this" | "following" = "";
      if (item.recurring) {
        const s = await askScope();
        if (s == null) return info.revert();
        scope = s;
      }
      const ev = info.event;
      const allDay = ev.allDay;
      const startD = ev.start!;
      let start: string;
      let end: string;
      if (allDay) {
        const s = utcMidnightOfLocal(startD);
        start = s.toISOString();
        end = (ev.end ? utcMidnightOfLocal(ev.end) : new Date(s.getTime() + 24 * HOUR)).toISOString();
      } else {
        start = startD.toISOString();
        end = (ev.end ?? new Date(startD.getTime() + HOUR)).toISOString();
      }
      const event = await svc.GetEvent(item.eventId);
      const req: EventSave = {
        event: { ...event, allDay, start, end },
        scope,
        occurrenceAt: item.occurrenceAt,
        notify: false,
      };
      const prev = await svc.PreviewEventSave(req);
      if (prev.notifyNeeded) {
        const notify = await askNotify(prev.added, prev.removed);
        if (notify == null) return info.revert();
        req.notify = notify;
      }
      await svc.SaveEvent(req);
      void qc.invalidateQueries({ queryKey: ["calendar"] });
      void qc.invalidateQueries({ queryKey: ["event"] });
    } catch (e) {
      toast.error(errorMessage(e));
      info.revert();
    }
  };

  const newEvent = () => {
    const start = new Date();
    start.setMinutes(0, 0, 0);
    start.setHours(start.getHours() + 1);
    onNewEvent?.(start, new Date(start.getTime() + HOUR), false);
  };

  return (
    <div className="flex h-full min-h-0">
      <div className="flex min-w-0 flex-1 flex-col p-3">
        <CalendarToolbar
          title={title}
          view={view}
          onView={changeView}
          onToday={() => calRef.current?.getApi().today()}
          onPrev={() => calRef.current?.getApi().prev()}
          onNext={() => calRef.current?.getApi().next()}
          onNew={newEvent}
          stale={cal.data?.taskappStale ?? false}
          fetchedAt={cal.data?.taskappFetchedAt ?? ""}
        />
        <div className="min-h-0 flex-1">
          <FullCalendar
            ref={calRef}
            plugins={PLUGINS}
            locale={esLocale}
            initialView={initial}
            views={VIEWS}
            headerToolbar={false}
            firstDay={1}
            nowIndicator
            scrollTime="08:00"
            height="100%"
            selectable
            selectMirror
            events={events}
            datesSet={onDatesSet}
            eventClick={(info) => state.selectEvent(info.event.id)}
            select={(info) => {
              onNewEvent?.(info.start, info.end, info.allDay);
              calRef.current?.getApi().unselect();
            }}
            eventDrop={(info) => void reschedule(info)}
            eventResize={(info) => void reschedule(info)}
          />
        </div>
      </div>
      <aside className="w-80 shrink-0 border-l border-border">
        {selected ? (
          <EventDetail key={selected.key} item={selected} state={state} onEdit={onEditEvent} />
        ) : (
          <div className="flex h-full items-center justify-center p-6 text-center text-sm text-muted-foreground">
            Elegí un evento para ver el detalle
          </div>
        )}
      </aside>
      {dialogs}
    </div>
  );
}

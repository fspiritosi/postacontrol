import type { EventInput } from "@fullcalendar/core";
import type { CalendarItem } from "../../bindings/postacontrol/core/model/models";

export type CalendarViewId = "timeGridDay" | "workWeek" | "timeGridWeek" | "dayGridMonth" | "listWeek";

const VIEW_KEY = "pc:calendar:view";
const VIEWS: CalendarViewId[] = ["timeGridDay", "workWeek", "timeGridWeek", "dayGridMonth", "listWeek"];

export function loadCalendarView(): CalendarViewId {
  try {
    const v = localStorage.getItem(VIEW_KEY);
    if (VIEWS.includes(v as CalendarViewId)) return v as CalendarViewId;
  } catch {
    // sin almacenamiento: se usa la vista por defecto
  }
  return "workWeek";
}

export function saveCalendarView(v: CalendarViewId) {
  try {
    localStorage.setItem(VIEW_KEY, v);
  } catch {
    // no se pudo recordar; no pasa nada
  }
}

// Un día completo se guarda como medianoche UTC: se muestra la fecha UTC, sin
// pasarla por la zona local, para que no se corra de día.
export function allDayDate(iso: string): string {
  return new Date(iso).toISOString().slice(0, 10);
}

// Medianoche UTC del día local (año/mes/día) de una fecha de FullCalendar.
export function utcMidnightOfLocal(d: Date): Date {
  return new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate()));
}

export function toFcEvent(it: CalendarItem): EventInput {
  const classNames: string[] = [];
  if (it.source === "local") classNames.push("ev-local");
  if (it.source === "task") classNames.push("ev-task");
  if (it.source === "invite" && it.partstat === "NEEDS-ACTION") classNames.push("ev-invite-pending");
  if (it.cancelled) classNames.push("ev-cancelled");
  const ev: EventInput = {
    id: it.key,
    title: it.title,
    allDay: it.allDay,
    start: it.allDay ? allDayDate(it.start) : it.start,
    end: it.allDay ? allDayDate(it.end) : it.end,
    editable: it.source === "local" && !it.cancelled,
    classNames,
    extendedProps: { item: it },
  };
  if (it.source === "taskapp" && it.color) ev.backgroundColor = it.color;
  return ev;
}

// "viernes 20 de noviembre · 10:00 a 11:00" (o solo la fecha, en día completo).
export function formatItemWhen(it: Pick<CalendarItem, "start" | "end" | "allDay">): string {
  if (it.allDay) {
    const day = (iso: string) =>
      new Date(iso).toLocaleDateString("es-AR", { weekday: "long", day: "numeric", month: "long", timeZone: "UTC" });
    // El fin es exclusivo: el último día es el anterior.
    const last = new Date(new Date(it.end).getTime() - 86_400_000).toISOString();
    return allDayDate(last) > allDayDate(it.start) ? `${day(it.start)} al ${day(last)}` : day(it.start);
  }
  const s = new Date(it.start);
  const e = new Date(it.end);
  const date = s.toLocaleDateString("es-AR", { weekday: "long", day: "numeric", month: "long" });
  const hm = (d: Date) => d.toLocaleTimeString("es-AR", { hour: "2-digit", minute: "2-digit" });
  return `${date} · ${hm(s)} a ${hm(e)}`;
}

// Fecha local "AAAA-MM-DD" y hora local "HH:MM" para los campos del formulario.
const two = (n: number) => String(n).padStart(2, "0");
export const localDay = (d: Date) => `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())}`;
export const localTime = (d: Date) => `${two(d.getHours())}:${two(d.getMinutes())}`;

// Medianoche local de la fecha de un día completo (la inversa de utcMidnightOfLocal).
export function allDayLocal(iso: string): Date {
  const [y, m, d] = allDayDate(iso).split("-").map(Number);
  return new Date(y, m - 1, d);
}

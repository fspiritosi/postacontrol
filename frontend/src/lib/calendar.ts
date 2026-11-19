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

// Texto sobre el color de un proyecto: el que más contrasta con el fondo.
export const DARK_TEXT = "#111111";
export const LIGHT_TEXT = "#ffffff";

// Luminancia relativa (WCAG) de un color "#rgb" o "#rrggbb"; null si no se entiende.
function luminance(hex: string): number | null {
  let h = hex.trim().replace(/^#/, "");
  if (/^[0-9a-f]{3}$/i.test(h)) h = [...h].map((c) => c + c).join("");
  if (!/^[0-9a-f]{6}$/i.test(h)) return null;
  const [r, g, b] = [0, 2, 4].map((i) => {
    const c = parseInt(h.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function textOn(bg: string): string | undefined {
  const l = luminance(bg);
  if (l == null) return undefined;
  // Contraste contra blanco (L=1) y contra el texto oscuro (L≈0.0056).
  const dark = luminance(DARK_TEXT) ?? 0;
  return 1.05 / (l + 0.05) >= (l + 0.05) / (dark + 0.05) ? LIGHT_TEXT : DARK_TEXT;
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
  if (it.source === "taskapp") {
    if (it.color) {
      ev.backgroundColor = it.color;
      ev.borderColor = it.color;
      ev.textColor = textOn(it.color);
    } else {
      classNames.push("ev-taskapp"); // sin proyecto: neutro, distinto de los locales
    }
  }
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

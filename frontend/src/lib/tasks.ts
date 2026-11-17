// Lista de tareas: junta las locales y las de Taskapp, y las agrupa por vencimiento.
// Funciones puras, sin React.

import type { LocalTask, TaskappTask } from "../../bindings/postacontrol/core/model/models";

export type TaskSource = "all" | "taskapp" | "local";

export type TaskItem =
  | { source: "local"; key: string; due: Date | null; hasTime: boolean; task: LocalTask }
  | { source: "taskapp"; key: string; due: Date | null; hasTime: false; task: TaskappTask };

export type TaskGroup = {
  id: "overdue" | "today" | "tomorrow" | "week" | "later" | "none";
  label: string;
  items: TaskItem[];
};

const ORDER: TaskGroup["id"][] = ["overdue", "today", "tomorrow", "week", "later", "none"];

const LABELS: Record<TaskGroup["id"], string> = {
  overdue: "Vencidas",
  today: "Hoy",
  tomorrow: "Mañana",
  week: "Esta semana",
  later: "Más adelante",
  none: "Sin fecha",
};

// Opciones de recordatorio: minutos antes del vencimiento.
export const REMINDER_OPTIONS: { minutes: number; label: string }[] = [
  { minutes: 0, label: "A la hora" },
  { minutes: 5, label: "5 minutos antes" },
  { minutes: 15, label: "15 minutos antes" },
  { minutes: 30, label: "30 minutos antes" },
  { minutes: 60, label: "1 hora antes" },
  { minutes: 1440, label: "1 día antes" },
];

// "AAAA-MM-DD" en hora local. No usar new Date("AAAA-MM-DD"): eso es UTC.
function parseDay(s: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  return m ? new Date(+m[1], +m[2] - 1, +m[3]) : null;
}

// Sin hora, el vencimiento es la medianoche local de ese día.
function parseDue(dueDate: string, dueTime: string): { due: Date | null; hasTime: boolean } {
  const day = parseDay(dueDate);
  if (!day) return { due: null, hasTime: false };
  const t = /^(\d{1,2}):(\d{2})$/.exec(dueTime);
  if (t) return { due: new Date(day.getFullYear(), day.getMonth(), day.getDate(), +t[1], +t[2]), hasTime: true };
  return { due: day, hasTime: false };
}

function startOfDay(d: Date): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

// Une las tareas según el origen elegido en la vista.
export function toItems(local: LocalTask[], remote: TaskappTask[], source: TaskSource): TaskItem[] {
  const out: TaskItem[] = [];
  if (source !== "taskapp") {
    for (const task of local) {
      const { due, hasTime } = parseDue(task.dueDate, task.dueTime);
      out.push({ source: "local", key: `l:${task.id}`, due, hasTime, task });
    }
  }
  if (source !== "local") {
    for (const task of remote) {
      out.push({ source: "taskapp", key: `t:${task.id}`, due: parseDay(task.dueDate), hasTime: false, task });
    }
  }
  return out;
}

function groupOf(
  item: TaskItem,
  now: Date,
  today: Date,
  tomorrow: Date,
  sunday: Date,
): TaskGroup["id"] {
  if (!item.due) return "none";
  const day = startOfDay(item.due).getTime();
  if (day < today.getTime()) return "overdue";
  if (day === today.getTime()) return item.hasTime && item.due.getTime() < now.getTime() ? "overdue" : "today";
  if (day === tomorrow.getTime()) return "tomorrow";
  // "Esta semana" llega hasta el domingo; si hoy es sábado, mañana ya es domingo y cae arriba.
  if (day <= sunday.getTime()) return "week";
  return "later";
}

// Agrupa por vencimiento. Los grupos vacíos se omiten.
export function groupByDue(items: TaskItem[], now: Date): TaskGroup[] {
  const today = startOfDay(now);
  const tomorrow = addDays(today, 1);
  // Semana de lunes a domingo: getDay() vale 0 para domingo.
  const mondayIndex = (today.getDay() + 6) % 7;
  const sunday = addDays(today, 6 - mondayIndex);

  const buckets: Record<TaskGroup["id"], TaskItem[]> = {
    overdue: [], today: [], tomorrow: [], week: [], later: [], none: [],
  };
  for (const item of items) buckets[groupOf(item, now, today, tomorrow, sunday)].push(item);

  return ORDER.filter((id) => buckets[id].length > 0).map((id) => ({
    id,
    label: LABELS[id],
    items: buckets[id].sort((a, b) =>
      (a.due?.getTime() ?? 0) - (b.due?.getTime() ?? 0) || a.task.title.localeCompare(b.task.title, "es")),
  }));
}

// Horas para programar un envío o posponer un correo.

export type WhenPreset = { label: string; at: Date };

function at(day: Date, hours: number, minutes = 0): Date {
  return new Date(day.getFullYear(), day.getMonth(), day.getDate(), hours, minutes);
}

function addDays(d: Date, n: number): Date {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate() + n);
}

// Atajos según la hora: esta tarde (si todavía falta), mañana y el lunes.
export function whenPresets(now: Date): WhenPreset[] {
  const out: WhenPreset[] = [];
  const evening = at(now, 18);
  if (evening.getTime() - now.getTime() >= 60 * 60 * 1000) out.push({ label: "Esta tarde, 18:00", at: evening });
  out.push({ label: "Mañana, 8:00", at: at(addDays(now, 1), 8) });
  // Lunes que viene; si mañana ya es lunes, el de la otra semana.
  const daysToMonday = ((8 - now.getDay()) % 7) || 7;
  const monday = at(addDays(now, daysToMonday), 8);
  out.push(daysToMonday === 1
    ? { label: "El lunes que viene, 8:00", at: at(addDays(now, 8), 8) }
    : { label: "El lunes, 8:00", at: monday });
  return out;
}

const DAYS = ["dom", "lun", "mar", "mié", "jue", "vie", "sáb"];

const hhmm = (d: Date) => `${d.getHours()}:${String(d.getMinutes()).padStart(2, "0")}`;

// "hoy a las 18:00", "mañana a las 8:00", "el lun 16/11 a las 8:00".
export function formatWhen(d: Date, now = new Date()): string {
  const sameDay = (a: Date, b: Date) => a.toDateString() === b.toDateString();
  if (sameDay(d, now)) return `hoy a las ${hhmm(d)}`;
  if (sameDay(d, addDays(now, 1))) return `mañana a las ${hhmm(d)}`;
  return `el ${DAYS[d.getDay()]} ${d.getDate()}/${d.getMonth() + 1} a las ${hhmm(d)}`;
}

const pad = (n: number) => String(n).padStart(2, "0");

// Para el <input type="datetime-local">, en hora local.
export function toLocalInput(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function fromLocalInput(s: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})$/.exec(s);
  return m ? new Date(+m[1], +m[2] - 1, +m[3], +m[4], +m[5]) : null;
}

// Los días que ofrece el selector a mano, desde hoy: "Hoy, vie 13/11",
// "Mañana, sáb 14/11", "dom 15/11"…
export function dayOptions(now: Date, count = 60): { label: string; date: Date }[] {
  return Array.from({ length: count }, (_, i) => {
    const date = addDays(now, i);
    const base = `${DAYS[date.getDay()]} ${date.getDate()}/${date.getMonth() + 1}`;
    return { label: i === 0 ? `Hoy, ${base}` : i === 1 ? `Mañana, ${base}` : base, date };
  });
}

// Lo que propone el selector a mano: una hora después de now, redondeada
// hacia arriba a 5 minutos.
export function firstPick(now: Date): { dayIndex: number; hour: number; minute: number } {
  const t = new Date(now.getTime() + 60 * 60 * 1000);
  const extra = (5 - (t.getMinutes() % 5)) % 5;
  const r = new Date(t.getTime() + extra * 60 * 1000);
  const dayIndex = Math.round((at(r, 0).getTime() - at(now, 0).getTime()) / (24 * 60 * 60 * 1000));
  return { dayIndex, hour: r.getHours(), minute: r.getMinutes() };
}

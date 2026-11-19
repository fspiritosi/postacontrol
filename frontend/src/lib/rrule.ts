// Repetición simple de un evento: lo que arma el formulario. Una regla que no
// salga de buildRRule (intervalos, COUNT, BYMONTHDAY…) es "personalizada" y el
// formulario la conserva sin tocarla.
export type Repeat = {
  freq: "none" | "daily" | "weekly" | "monthly" | "yearly";
  // 0 = lunes … 6 = domingo
  days: number[];
  // AAAA-MM-DD o ""
  until: string;
};

const DAYS = ["MO", "TU", "WE", "TH", "FR", "SA", "SU"];
const DAY_NAMES = ["lun", "mar", "mié", "jue", "vie", "sáb", "dom"];
const FREQS: Record<string, Repeat["freq"]> = { DAILY: "daily", WEEKLY: "weekly", MONTHLY: "monthly", YEARLY: "yearly" };
const FREQ_TEXT: Record<string, string> = { daily: "Cada día", weekly: "Cada semana", monthly: "Cada mes", yearly: "Cada año" };

const pad = (n: number) => String(n).padStart(2, "0");

// Lunes = 0, como en el formulario (Date.getDay() arranca en domingo).
const mondayIndex = (d: Date) => (d.getDay() + 6) % 7;

// Pasa AAAA-MM-DD al UNTIL de la regla. Con hora: las 23:59:59 locales en UTC.
// En un día completo se usa la fecha, del mismo tipo que el DTSTART (RFC 5545).
function untilValue(day: string, allDay: boolean): string {
  const [y, m, d] = day.split("-").map(Number);
  if (allDay) return `${y}${pad(m)}${pad(d)}`;
  return new Date(y, m - 1, d, 23, 59, 59).toISOString().replace(/[-:]|\.\d{3}/g, "");
}

// Al revés: la fecha local del UNTIL (o la misma fecha si ya es una fecha).
function untilDay(v: string): string | null {
  let m = /^(\d{4})(\d{2})(\d{2})$/.exec(v);
  if (m) return `${m[1]}-${m[2]}-${m[3]}`;
  m = /^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})Z$/.exec(v);
  if (!m) return null;
  const d = new Date(Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5], +m[6]));
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function buildRRule(r: Repeat, start: Date, allDay = false): string {
  if (r.freq === "none") return "";
  const parts = [`FREQ=${r.freq.toUpperCase()}`];
  if (r.freq === "weekly") {
    const days = r.days.length > 0 ? [...r.days].sort((a, b) => a - b) : [mondayIndex(start)];
    parts.push(`BYDAY=${days.map((d) => DAYS[d]).join(",")}`);
  }
  if (r.until) parts.push(`UNTIL=${untilValue(r.until, allDay)}`);
  return parts.join(";");
}

// null si la regla no la armó buildRRule. "" es "no se repite".
export function parseRRule(s: string): Repeat | null {
  if (s === "") return { freq: "none", days: [], until: "" };
  const out: Repeat = { freq: "none", days: [], until: "" };
  let byday = false;
  for (const part of s.split(";")) {
    const [k, v = ""] = part.split("=");
    if (k === "FREQ" && FREQS[v]) out.freq = FREQS[v];
    else if (k === "BYDAY") {
      const days = v.split(",").map((d) => DAYS.indexOf(d));
      if (days.some((d) => d < 0)) return null;
      out.days = days;
      byday = true;
    } else if (k === "UNTIL") {
      const day = untilDay(v);
      if (!day) return null;
      out.until = day;
    } else return null;
  }
  if (out.freq === "none" || (byday && out.freq !== "weekly")) return null;
  return out;
}

export function describeRRule(s: string): string {
  const r = parseRRule(s);
  if (!r || r.freq === "none") return r ? "No se repite" : "Repetición personalizada";
  let text = FREQ_TEXT[r.freq];
  if (r.days.length > 0) text += `: ${r.days.map((d) => DAY_NAMES[d]).join(", ")}`;
  if (r.until) {
    const [y, m, d] = r.until.split("-");
    text += ` · hasta ${d}/${m}/${y}`;
  }
  return text;
}

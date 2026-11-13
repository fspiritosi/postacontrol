// Lee lo esencial de una invitación de calendario (.ics, RFC 5545) para
// mostrarla en el lector: qué, cuándo, dónde y quién la manda.

export type Invite = {
  summary: string;
  start: Date | null;
  end: Date | null;
  allDay: boolean;
  location: string;
  organizer: string;
  description: string;
  // METHOD:CANCEL: el evento se canceló.
  cancelled: boolean;
};

function unescapeText(v: string): string {
  return v.replace(/\\([\\;,nN])/g, (_, c: string) => (c === "n" || c === "N" ? "\n" : c));
}

// 20261117T130000Z (UTC), 20261117T100000 (hora local; con TZID se toma como
// local, aproximado) o 20261117 (día entero).
function parseDate(v: string): { date: Date; allDay: boolean } | null {
  const m = /^(\d{4})(\d{2})(\d{2})(?:T(\d{2})(\d{2})(\d{2})(Z)?)?$/.exec(v.trim());
  if (!m) return null;
  const [y, mo, d] = [+m[1], +m[2] - 1, +m[3]];
  if (m[4] === undefined) return { date: new Date(y, mo, d), allDay: true };
  const [h, mi, s] = [+m[4], +m[5], +m[6]];
  return { date: m[7] ? new Date(Date.UTC(y, mo, d, h, mi, s)) : new Date(y, mo, d, h, mi, s), allDay: false };
}

export function parseIcs(text: string): Invite | null {
  const lines = text.replace(/\r?\n[ \t]/g, "").split(/\r?\n/);
  let inEvent = false;
  let found = false;
  let cancelled = false;
  const ev: Invite = { summary: "", start: null, end: null, allDay: false, location: "", organizer: "", description: "", cancelled: false };
  for (const line of lines) {
    const colon = line.indexOf(":");
    if (colon < 0) continue;
    const [name, ...params] = line.slice(0, colon).split(";");
    const key = name.toUpperCase();
    const value = line.slice(colon + 1);
    if (key === "BEGIN" && value.toUpperCase() === "VEVENT") {
      inEvent = found = true;
      continue;
    }
    if (key === "END" && value.toUpperCase() === "VEVENT") {
      if (inEvent) break; // alcanza con el primero
      continue;
    }
    if (key === "METHOD") cancelled = value.trim().toUpperCase() === "CANCEL";
    if (!inEvent) continue;
    switch (key) {
      case "SUMMARY":
        ev.summary = unescapeText(value);
        break;
      case "LOCATION":
        ev.location = unescapeText(value);
        break;
      case "DESCRIPTION":
        ev.description = unescapeText(value);
        break;
      case "DTSTART": {
        const d = parseDate(value);
        if (d) [ev.start, ev.allDay] = [d.date, d.allDay];
        break;
      }
      case "DTEND": {
        const d = parseDate(value);
        if (d) ev.end = d.date;
        break;
      }
      case "ORGANIZER": {
        const cn = params.find((p) => p.toUpperCase().startsWith("CN="));
        ev.organizer = cn ? cn.slice(3).replace(/^"|"$/g, "") : value.replace(/^mailto:/i, "");
        break;
      }
      case "STATUS":
        if (value.trim().toUpperCase() === "CANCELLED") cancelled = true;
        break;
    }
  }
  if (!found) return null;
  ev.cancelled = cancelled;
  return ev;
}

const DAYS = ["dom", "lun", "mar", "mié", "jue", "vie", "sáb"];
const day = (d: Date) => `${DAYS[d.getDay()]} ${d.getDate()}/${d.getMonth() + 1}/${d.getFullYear()}`;
const hhmm = (d: Date) => `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;

// "mar 17/11/2026, 10:00 a 11:30" o "mar 8/12/2026, todo el día".
export function formatInviteWhen(ev: Pick<Invite, "start" | "end" | "allDay">): string {
  if (!ev.start) return "";
  if (ev.allDay) return `${day(ev.start)}, todo el día`;
  const sameDay = ev.end && ev.end.toDateString() === ev.start.toDateString();
  if (!ev.end) return `${day(ev.start)}, ${hhmm(ev.start)}`;
  return sameDay
    ? `${day(ev.start)}, ${hhmm(ev.start)} a ${hhmm(ev.end)}`
    : `${day(ev.start)}, ${hhmm(ev.start)} a ${day(ev.end)}, ${hhmm(ev.end)}`;
}

const timeFmt = new Intl.DateTimeFormat("es", { hour: "2-digit", minute: "2-digit" });
const dayFmt = new Intl.DateTimeFormat("es", { day: "numeric", month: "short" });
const fullFmt = new Intl.DateTimeFormat("es", { dateStyle: "full", timeStyle: "short" });

function startOfDay(d: Date): number {
  return new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
}

// Hora si es de hoy, "Ayer", o "d MMM".
export function relativeDate(iso: string, now = new Date()): string {
  const d = new Date(iso);
  const days = Math.round((startOfDay(now) - startOfDay(d)) / 86_400_000);
  if (days === 0) return timeFmt.format(d);
  if (days === 1) return "Ayer";
  return dayFmt.format(d);
}

export function fullDate(iso: string): string {
  return fullFmt.format(new Date(iso));
}

export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.round(bytes / 1024)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

export function initials(name: string, email: string): string {
  const src = (name || email).trim();
  const parts = src.split(/[\s@.]+/).filter(Boolean);
  const letters = (parts.length > 1 ? parts[0][0] + parts[1][0] : src.slice(0, 2)) || "?";
  return letters.toUpperCase();
}

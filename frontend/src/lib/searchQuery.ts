// Los filtros de la búsqueda viajan en el texto, con la misma sintaxis que
// entiende el núcleo (core/store/searchquery.go): de:, para:, asunto:,
// tiene:adjunto, es:no-leído, es:destacado, desde: y hasta:. Esto los separa
// del texto libre para mostrarlos en el panel de filtros y los vuelve a armar.

export type SearchFilters = {
  from: string;
  to: string;
  subject: string;
  // AAAA-MM-DD (como el <input type="date">); "" = sin límite. Los dos incluidos.
  since: string;
  until: string;
  unread: boolean;
  flagged: boolean;
  attachment: boolean;
};

export const NO_FILTERS: SearchFilters = {
  from: "",
  to: "",
  subject: "",
  since: "",
  until: "",
  unread: false,
  flagged: false,
  attachment: false,
};

const fold = (s: string) =>
  s
    .toLowerCase()
    .normalize("NFD")
    .replace(/\p{Mn}/gu, "");

// Parte por espacios, salvo entre comillas; las comillas no quedan.
function tokens(text: string): string[] {
  const out: string[] = [];
  let cur = "";
  let quoted = false;
  let started = false;
  for (const ch of text) {
    if (ch === '"') {
      quoted = !quoted;
      started = true;
    } else if (/\s/.test(ch) && !quoted) {
      if (started) out.push(cur);
      cur = "";
      started = false;
    } else {
      cur += ch;
      started = true;
    }
  }
  if (started) out.push(cur);
  return out;
}

// "31/10/2026" o "2026-10-31" → "2026-10-31"; null si no es una fecha.
function isoDate(s: string): string | null {
  if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return s;
  const m = /^(\d{1,2})\/(\d{1,2})\/(\d{4})$/.exec(s);
  return m ? `${m[3]}-${m[2].padStart(2, "0")}-${m[1].padStart(2, "0")}` : null;
}

// Quoted: con comillas si hace falta para que llegue entero.
const quoted = (s: string) => (/[\s"]/.test(s) ? `"${s.replace(/"/g, "")}"` : s);

export function splitQuery(text: string): { free: string; filters: SearchFilters } {
  const filters = { ...NO_FILTERS };
  const free: string[] = [];
  for (const tok of tokens(text)) {
    const i = tok.indexOf(":");
    const key = i > 0 ? fold(tok.slice(0, i)) : "";
    const val = i > 0 ? tok.slice(i + 1) : "";
    const v = fold(val);
    let used = val !== "";
    if (used) {
      if (key === "de" || key === "from") filters.from = val;
      else if (key === "para" || key === "to") filters.to = val;
      else if (key === "asunto" || key === "subject") filters.subject = val;
      else if ((key === "tiene" || key === "has") && ["adjunto", "adjuntos", "attachment"].includes(v)) filters.attachment = true;
      else if ((key === "es" || key === "is") && ["no-leido", "noleido", "sin-leer", "unread"].includes(v)) filters.unread = true;
      else if ((key === "es" || key === "is") && ["destacado", "starred", "flagged"].includes(v)) filters.flagged = true;
      else if ((key === "desde" || key === "after" || key === "since") && isoDate(val)) filters.since = isoDate(val)!;
      else if (key === "hasta" && isoDate(val)) filters.until = isoDate(val)!;
      else used = false;
    }
    if (!used) free.push(/\s/.test(tok) ? `"${tok}"` : tok);
  }
  return { free: free.join(" "), filters };
}

export function joinQuery(free: string, f: SearchFilters): string {
  const parts = [free.trim()];
  if (f.from.trim()) parts.push(`de:${quoted(f.from.trim())}`);
  if (f.to.trim()) parts.push(`para:${quoted(f.to.trim())}`);
  if (f.subject.trim()) parts.push(`asunto:${quoted(f.subject.trim())}`);
  if (f.attachment) parts.push("tiene:adjunto");
  if (f.unread) parts.push("es:no-leído");
  if (f.flagged) parts.push("es:destacado");
  if (f.since) parts.push(`desde:${f.since}`);
  if (f.until) parts.push(`hasta:${f.until}`);
  return parts.filter(Boolean).join(" ");
}

export function activeFilterCount(f: SearchFilters): number {
  return (
    [f.from, f.to, f.subject, f.since, f.until].filter((v) => v.trim() !== "").length +
    [f.unread, f.flagged, f.attachment].filter(Boolean).length
  );
}

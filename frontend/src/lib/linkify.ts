export type TextPart =
  | { kind: "text"; text: string }
  | { kind: "link"; text: string; href: string };

// URL (http/https o www.) o dirección de correo, en orden de aparición.
// La alternativa de URL va primero: un correo dentro de una URL queda dentro.
const PATTERN =
  /(?:https?:\/\/|www\.)[^\s<>"]+|[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+/gi;

const TRAILING = ".,;:!?)";

// Saca la puntuación final; un ")" se conserva si hay un "(" sin cerrar.
function trimTrailing(raw: string): string {
  let s = raw;
  while (s.length > 0 && TRAILING.includes(s[s.length - 1])) {
    if (s.endsWith(")")) {
      const open = s.split("(").length - 1;
      const close = s.split(")").length - 1;
      if (close <= open) break;
    }
    s = s.slice(0, -1);
  }
  return s;
}

export function linkify(text: string): TextPart[] {
  const parts: TextPart[] = [];
  let last = 0;
  for (const m of text.matchAll(PATTERN)) {
    const start = m.index ?? 0;
    const link = trimTrailing(m[0]);
    if (link === "") continue;
    if (start > last) parts.push({ kind: "text", text: text.slice(last, start) });
    const href = /^https?:\/\//i.test(link)
      ? link
      : /^www\./i.test(link)
        ? `https://${link}`
        : `mailto:${link}`;
    parts.push({ kind: "link", text: link, href });
    last = start + link.length;
  }
  if (last < text.length) parts.push({ kind: "text", text: text.slice(last) });
  return parts.length > 0 ? parts : [{ kind: "text", text }];
}

import type { Address } from "../../bindings/postacontrol/core/model/models";

const EMAIL_RE = /^[^\s@<>",;]+@[^\s@<>",;]+\.[^\s@<>",;]+$/;

// Parte por coma, punto y coma o salto de línea, salvo dentro de comillas
// (nombres como "Peña, Fabricio") o de <...>. Devuelve los pedazos sin
// recortar; el último es lo que sigue al último separador.
function splitRaw(input: string): string[] {
  const out: string[] = [];
  let cur = "";
  let quoted = false;
  let angle = false;
  for (const ch of input) {
    if (ch === '"') quoted = !quoted;
    else if (!quoted && ch === "<") angle = true;
    else if (!quoted && ch === ">") angle = false;
    if (!quoted && !angle && (ch === "," || ch === ";" || ch === "\n" || ch === "\r")) {
      out.push(cur);
      cur = "";
    } else {
      cur += ch;
    }
  }
  out.push(cur);
  return out;
}

const clean = (parts: string[]) => parts.map((t) => t.trim()).filter(Boolean);

export function splitRecipientTokens(input: string): string[] {
  return clean(splitRaw(input));
}

// Para el campo con chips: los tokens ya cerrados por un separador y lo que
// sigue escribiéndose después del último.
export function takeRecipientTokens(input: string): { tokens: string[]; rest: string } {
  const parts = splitRaw(input);
  const rest = parts.pop()!;
  return { tokens: clean(parts), rest: parts.length > 0 ? rest.trimStart() : rest };
}

function unquote(name: string): string {
  const n = name.trim();
  return n.length >= 2 && n.startsWith('"') && n.endsWith('"')
    ? n.slice(1, -1).replace(/\\(.)/g, "$1").trim()
    : n;
}

export function parseRecipients(input: string): { valid: Address[]; invalid: string[] } {
  const valid: Address[] = [];
  const invalid: string[] = [];
  for (const token of splitRecipientTokens(input)) {
    const m = token.match(/^(.*?)\s*<([^<>]*)>$/);
    const email = (m ? m[2] : token).trim();
    const name = m ? unquote(m[1]) : "";
    if (EMAIL_RE.test(email)) valid.push({ name, email });
    else invalid.push(token);
  }
  return { valid, invalid };
}

// Inversa de parseRecipients, para precargar los campos de texto.
export function formatRecipients(list: Address[] | null | undefined): string {
  return (list ?? [])
    .map((a) => {
      if (!a.name) return a.email;
      const name = /[",;<>]/.test(a.name) ? `"${a.name.replace(/(["\\])/g, "\\$1")}"` : a.name;
      return `${name} <${a.email}>`;
    })
    .join(", ");
}

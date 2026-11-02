import type { Address } from "../../bindings/postacontrol/core/model/models";

const EMAIL_RE = /^[^\s@<>",;]+@[^\s@<>",;]+\.[^\s@<>",;]+$/;

const SEPARATORS = new Set([",", ";", "\n", "\r"]);

// Estado al recorrer un token: dentro de comillas (con \" escapadas, como las
// escribe formatRecipients) o de <...>.
type ScanState = { quoted: boolean; angle: boolean; escaped: boolean };

const newScan = (): ScanState => ({ quoted: false, angle: false, escaped: false });

function step(s: ScanState, ch: string): void {
  if (s.escaped) s.escaped = false;
  else if (s.quoted && ch === "\\") s.escaped = true;
  else if (ch === '"') s.quoted = !s.quoted;
  else if (!s.quoted && ch === "<") s.angle = true;
  else if (!s.quoted && ch === ">") s.angle = false;
}

// Parte por coma, punto y coma o salto de línea, salvo dentro de comillas
// (nombres como "Peña, Fabricio") o de <...>. Con closeAngle, un separador
// dentro de <...> lo cierra: un correo no lleva comas, así que es un ">" que
// faltó. Devuelve los pedazos sin recortar; el último es lo que sigue al
// último separador.
function splitRaw(input: string, closeAngle = false): string[] {
  const out: string[] = [];
  let cur = "";
  const s = newScan();
  for (const ch of input) {
    step(s, ch);
    if (!s.quoted && (!s.angle || closeAngle) && SEPARATORS.has(ch)) {
      s.angle = false;
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

// Cierra lo que quedó abierto en un token, para que al volver a partir el
// texto no se coma lo que viene después: saca la comilla que no se cerró y
// cierra un "<" sin ">".
export function balanceRecipientToken(token: string): string {
  let t = token;
  for (;;) {
    const s = newScan();
    let openedAt = -1;
    for (let i = 0; i < t.length; i++) {
      const wasQuoted = s.quoted;
      step(s, t[i]);
      if (!wasQuoted && s.quoted) openedAt = i;
    }
    if (!s.quoted) return s.angle ? `${t}>` : t;
    // Sin esa comilla, lo que seguía se vuelve a leer: puede abrir otra.
    t = t.slice(0, openedAt) + t.slice(openedAt + 1);
  }
}

// Para el campo con chips: los tokens ya cerrados por un separador (listos
// para ser chips) y lo que sigue escribiéndose después del último.
export function takeRecipientTokens(input: string): { tokens: string[]; rest: string } {
  const parts = splitRaw(input, true);
  const rest = parts.pop()!;
  return {
    tokens: clean(parts).map(balanceRecipientToken),
    rest: parts.length > 0 ? rest.trimStart() : rest,
  };
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

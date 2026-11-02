import type { Attachment } from "../../bindings/postacontrol/core/model/models";

export type ViewKind = "pdf" | "image";

// Lo que el visor abre. SVG queda afuera a propósito: puede traer scripts.
const BY_MIME: Record<string, ViewKind> = {
  "application/pdf": "pdf",
  "image/png": "image",
  "image/jpeg": "image",
  "image/gif": "image",
  "image/webp": "image",
};

// Extensión → MIME real, para cuando el correo manda un MIME genérico.
const BY_EXT: Record<string, string> = {
  pdf: "application/pdf",
  png: "image/png",
  jpg: "image/jpeg",
  jpeg: "image/jpeg",
  gif: "image/gif",
  webp: "image/webp",
};

// MIME que no dicen nada del contenido: ahí se decide por la extensión.
const GENERIC = new Set(["", "application/octet-stream", "binary/octet-stream", "application/x-download", "application/force-download"]);

function baseMime(mimeType: string): string {
  return mimeType.split(";")[0].trim().toLowerCase();
}

function extension(filename: string): string {
  const dot = filename.lastIndexOf(".");
  return dot < 0 ? "" : filename.slice(dot + 1).toLowerCase();
}

// viewMime da el MIME con el que el visor muestra el adjunto, o null si no lo
// previsualiza. Un MIME específico manda; uno genérico cede a la extensión.
export function viewMime(a: Pick<Attachment, "mimeType" | "filename">): string | null {
  const m = baseMime(a.mimeType);
  if (m in BY_MIME) return m;
  if (!GENERIC.has(m)) return null;
  return BY_EXT[extension(a.filename)] ?? null;
}

export function viewKind(a: Pick<Attachment, "mimeType" | "filename">): ViewKind | null {
  const m = viewMime(a);
  return m ? BY_MIME[m] : null;
}

export function base64ToBytes(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
  return out;
}

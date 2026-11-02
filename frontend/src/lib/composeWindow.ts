import type { ComposerInit } from "@/features/compose/ComposerContext";

// composeWindowId lee el id de una ventana de redactor de #/compose/<id>.
// Algo que no matchea, o un id que no es entero positivo, da null.
export function composeWindowId(hash: string = window.location.hash): number | null {
  const m = /^#\/compose\/(\d+)$/.exec(hash);
  if (!m) return null;
  const id = Number(m[1]);
  return Number.isSafeInteger(id) && id > 0 ? id : null;
}

export function serializeInit(init: ComposerInit): string {
  return JSON.stringify(init);
}

// parseInit valida lo mínimo que el redactor necesita para arrancar.
export function parseInit(json: string): ComposerInit {
  const v = JSON.parse(json) as Partial<ComposerInit> | null;
  if (!v || typeof v !== "object" || typeof v.accountId !== "number") {
    throw new Error("datos del redactor inválidos");
  }
  for (const k of ["to", "cc", "bcc", "attachments", "originalAttachments", "skipOriginalAttachments"] as const) {
    if (!Array.isArray(v[k])) throw new Error("datos del redactor inválidos");
  }
  return v as ComposerInit;
}

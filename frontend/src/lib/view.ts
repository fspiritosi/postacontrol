import type { Account, Folder } from "../../bindings/postacontrol/core/model/models";

export type UnifiedRole = "inbox" | "flagged" | "sent" | "drafts" | "trash";

// Lo que se ve en la lista: una carpeta de una cuenta, una vista que junta
// las carpetas de un rol de todas las cuentas, la Bandeja de salida o la
// libreta de contactos.
export type View =
  | { kind: "folder"; folderId: number }
  | { kind: "unified"; role: UnifiedRole }
  | { kind: "outbox" }
  | { kind: "contacts" };

export const UNIFIED_LABEL: Record<UnifiedRole, string> = {
  inbox: "Entrada",
  flagged: "Destacados",
  sent: "Enviados",
  drafts: "Borradores",
  trash: "Papelera",
};

const VIEW_KEY = "pc:view";

function isView(v: unknown): v is View {
  if (typeof v !== "object" || v === null) return false;
  const o = v as Record<string, unknown>;
  switch (o.kind) {
    case "folder":
      return typeof o.folderId === "number";
    case "unified":
      return typeof o.role === "string" && o.role in UNIFIED_LABEL;
    case "outbox":
    case "contacts":
      return true;
    default:
      return false;
  }
}

// La Bandeja de salida no se recuerda: solo existe mientras hay correos en
// camino y al reabrir la app se mostraría vacía. Se vuelve a la vista
// anterior (o a la de por defecto).
export function loadView(): View | null {
  try {
    const raw = localStorage.getItem(VIEW_KEY);
    if (raw) {
      const v: unknown = JSON.parse(raw);
      if (isView(v) && v.kind !== "outbox") return v;
    }
  } catch {
    // sin almacenamiento o JSON inválido: se arranca con la vista por defecto
  }
  return null;
}

export function saveView(v: View): void {
  if (v.kind === "outbox") return;
  try {
    localStorage.setItem(VIEW_KEY, JSON.stringify(v));
  } catch {
    // ignorar: recordar la vista es opcional
  }
}

export function viewIsValid(v: View, folders: Folder[], unifiedEnabled: boolean): boolean {
  switch (v.kind) {
    case "folder":
      return folders.some((f) => f.id === v.folderId && f.selectable);
    case "unified":
      return unifiedEnabled;
    case "outbox":
    case "contacts":
      return true;
  }
}

// La Entrada seleccionable de la primera cuenta (la de id menor).
export function defaultView(folders: Folder[]): View | null {
  let best: Folder | undefined;
  for (const f of folders) {
    if (f.role !== "inbox" || !f.selectable) continue;
    if (!best || f.accountId < best.accountId) best = f;
  }
  return best ? { kind: "folder", folderId: best.id } : null;
}

// La cuenta de la carpeta abierta; sin carpeta (vista unificada o Bandeja de
// salida), la primera cuenta.
export function viewAccount(
  v: View | null,
  folders: Folder[],
  accounts: Account[],
): Account | undefined {
  if (v?.kind === "folder") {
    const f = folders.find((x) => x.id === v.folderId);
    const a = f && accounts.find((x) => x.id === f.accountId);
    if (a) return a;
  }
  return accounts[0];
}

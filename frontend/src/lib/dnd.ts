import type {
  MessageSummary,
  ThreadSummary,
} from "../../bindings/postacontrol/core/model/models";

// Arrastrar correos de la lista a una carpeta del panel. La 5B suma acá el
// arrastre de carpetas.
export const MSG_MIME = "application/x-postacontrol-messages";
const ACCOUNT_MIME = "application/x-postacontrol-account-";

// En `dragover` no se pueden leer los datos, solo los tipos: la cuenta viaja
// en el tipo para saber ahí mismo si la carpeta sirve de destino.
export function accountMime(accountId: number): string {
  return ACCOUNT_MIME + accountId;
}

export type MessageDrag = {
  // Correos sueltos...
  ids?: number[];
  // ...o conversaciones (`roots`) de la carpeta `folderId`.
  roots?: string[];
  // Carpeta de origen, si es una sola.
  folderId?: number;
};

export function setMessageDrag(
  dt: DataTransfer,
  accountId: number,
  payload: MessageDrag,
) {
  dt.setData(MSG_MIME, JSON.stringify(payload));
  dt.setData(accountMime(accountId), "1");
  dt.effectAllowed = "move";
}

export function readMessageDrag(dt: DataTransfer): MessageDrag | null {
  const raw = dt.getData(MSG_MIME);
  if (!raw) return null;
  try {
    const v: unknown = JSON.parse(raw);
    return v != null && typeof v === "object" ? (v as MessageDrag) : null;
  } catch {
    return null;
  }
}

export function hasMessageDrag(dt: DataTransfer): boolean {
  return Array.from(dt.types).includes(MSG_MIME);
}

export function dragAccount(dt: DataTransfer): number | null {
  for (const t of Array.from(dt.types)) {
    if (t.startsWith(ACCOUNT_MIME)) {
      const n = Number(t.slice(ACCOUNT_MIME.length));
      if (Number.isInteger(n)) return n;
    }
  }
  return null;
}

// Lo que arranca una fila: de qué cuenta es y qué lleva.
export type DragInfo = { accountId: number; payload: MessageDrag };

// Si la fila está marcada van todos los marcados que siguen cargados; si no,
// solo ella y la selección no cambia. Con marcados de varias cuentas no se
// arrastra nada (null).
export function flatDrag(
  items: MessageSummary[],
  marked: number[],
  row: MessageSummary,
): DragInfo | null {
  const rows = marked.includes(row.id)
    ? items.filter((m) => marked.includes(m.id))
    : [row];
  return dragInfo(
    rows.map((m) => m.accountId),
    rows.map((m) => m.folderId),
    (folderId) => ({ ids: rows.map((m) => m.id), folderId }),
  );
}

export function threadDrag(
  folderId: number,
  items: ThreadSummary[],
  marked: string[],
  row: ThreadSummary,
): DragInfo | null {
  const rows = marked.includes(row.threadRoot)
    ? items.filter((t) => marked.includes(t.threadRoot))
    : [row];
  return dragInfo(
    rows.map((t) => t.latest.accountId),
    [folderId],
    () => ({ roots: rows.map((t) => t.threadRoot), folderId }),
  );
}

function dragInfo(
  accounts: number[],
  folders: number[],
  payload: (folderId: number | undefined) => MessageDrag,
): DragInfo | null {
  if (accounts.length === 0 || new Set(accounts).size !== 1) return null;
  const one = new Set(folders);
  return {
    accountId: accounts[0],
    payload: payload(one.size === 1 ? folders[0] : undefined),
  };
}

// dragstart de una fila.
export function startMessageDrag(
  e: { dataTransfer: DataTransfer; preventDefault: () => void },
  info: DragInfo | null,
) {
  if (info == null) e.preventDefault();
  else setMessageDrag(e.dataTransfer, info.accountId, info.payload);
}

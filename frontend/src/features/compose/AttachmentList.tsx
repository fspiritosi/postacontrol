import { FileIcon, XIcon } from "lucide-react";
import { formatSize } from "@/lib/format";

export type AttachmentItem = { key: string; filename: string; size: number };

type Props = { items: AttachmentItem[]; onRemove: (key: string) => void };

// AttachmentList muestra los adjuntos del correo como chips que se pueden quitar.
export function AttachmentList({ items, onRemove }: Props) {
  if (items.length === 0) return null;
  return (
    <ul className="flex flex-wrap gap-1.5" aria-label="Adjuntos">
      {items.map((it) => (
        <li
          key={it.key}
          className="flex max-w-56 items-center gap-1.5 rounded-md border bg-muted/40 py-1 pr-1 pl-2 text-xs"
        >
          <FileIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <span className="truncate" title={it.filename}>
            {it.filename}
          </span>
          <span className="shrink-0 text-muted-foreground">{formatSize(it.size)}</span>
          <button
            type="button"
            aria-label={`Quitar ${it.filename}`}
            className="shrink-0 rounded p-0.5 text-muted-foreground hover:bg-accent hover:text-foreground"
            onClick={() => onRemove(it.key)}
          >
            <XIcon className="size-3.5" />
          </button>
        </li>
      ))}
    </ul>
  );
}

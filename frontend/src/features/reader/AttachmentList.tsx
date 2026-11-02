import { useState } from "react";
import { PaperclipIcon } from "lucide-react";
import type { Attachment } from "../../../bindings/postacontrol/core/model/models";
import { viewKind } from "@/lib/attachments";
import { formatSize } from "@/lib/format";
import { AttachmentViewer } from "./AttachmentViewer";
import { saveAttachment } from "./saveAttachment";

// AttachmentList muestra los adjuntos del correo. PDF e imágenes se abren en
// el visor; el resto se guarda con "Guardar como".
export function AttachmentList({
  messageId,
  attachments,
}: {
  messageId: number;
  attachments: Attachment[];
}) {
  // Adjunto abierto en el visor (con su correo, por si el lector cambia de
  // correo sin desmontar la lista), o null si está cerrado.
  const [viewing, setViewing] = useState<{ messageId: number; index: number } | null>(null);
  if (attachments.length === 0) return null;
  return (
    <>
      <ul className="flex flex-wrap gap-2 border-t px-6 py-3">
        {attachments.map((a) => (
          <li key={a.index}>
            <button
              type="button"
              onClick={() => (viewKind(a) ? setViewing({ messageId, index: a.index }) : void saveAttachment(messageId, a.index))}
              className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1.5 text-xs transition-colors hover:bg-accent"
            >
              <PaperclipIcon className="size-3.5 text-muted-foreground" />
              <span className="max-w-48 truncate">{a.filename || "adjunto"}</span>
              <span className="text-muted-foreground">{formatSize(a.size)}</span>
            </button>
          </li>
        ))}
      </ul>
      {viewing?.messageId === messageId && (
        <AttachmentViewer
          messageId={messageId}
          attachments={attachments}
          index={viewing.index}
          onClose={() => setViewing(null)}
        />
      )}
    </>
  );
}

import { PaperclipIcon } from "lucide-react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Attachment } from "../../../bindings/postacontrol/core/model/models";
import { errorMessage } from "@/lib/errors";
import { formatSize } from "@/lib/format";

export function AttachmentList({
  messageId,
  attachments,
}: {
  messageId: number;
  attachments: Attachment[];
}) {
  if (attachments.length === 0) return null;
  const save = async (index: number) => {
    try {
      // saved=false es cancelar el diálogo: no es un error.
      if (await svc.SaveAttachment(messageId, index)) toast.success("Adjunto guardado");
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  return (
    <ul className="flex flex-wrap gap-2 border-t px-6 py-3">
      {attachments.map((a) => (
        <li key={a.index}>
          <button
            type="button"
            onClick={() => void save(a.index)}
            className="flex items-center gap-2 rounded-md border bg-card px-2.5 py-1.5 text-xs transition-colors hover:bg-accent"
          >
            <PaperclipIcon className="size-3.5 text-muted-foreground" />
            <span className="max-w-48 truncate">{a.filename || "adjunto"}</span>
            <span className="text-muted-foreground">{formatSize(a.size)}</span>
          </button>
        </li>
      ))}
    </ul>
  );
}

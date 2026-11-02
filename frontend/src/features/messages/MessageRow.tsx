import { PaperclipIcon, StarIcon } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "@/lib/errors";
import { relativeDate } from "@/lib/format";
import { useReadingPane } from "@/lib/settings";
import { cn } from "@/lib/utils";

type Props = {
  message: MessageSummary;
  color?: string;
  selected: boolean;
  onSelect: (id: number) => void;
  // Etiqueta de la carpeta (en los resultados de búsqueda).
  folderLabel?: string;
};

export function MessageRow({ message: m, color, selected, onSelect, folderLabel }: Props) {
  const queryClient = useQueryClient();
  // Con el lector abajo o arriba la lista es ancha: cada correo ocupa un renglón.
  const compact = useReadingPane() !== "right";

  const toggleFlag = async () => {
    try {
      await svc.SetFlagged([m.id], !m.flagged);
      void queryClient.invalidateQueries({ queryKey: ["messages"] });
      void queryClient.invalidateQueries({ queryKey: ["search"] });
      void queryClient.invalidateQueries({ queryKey: ["message", m.id] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <li className="relative" data-density={compact ? "compact" : "comfortable"}>
      <button
        type="button"
        onClick={() => onSelect(m.id)}
        aria-current={selected ? "true" : undefined}
        className={cn(
          compact
            ? "flex w-full items-center gap-2 border-b px-4 py-1.5 text-left transition-colors hover:bg-accent/60"
            : "flex w-full flex-col gap-0.5 border-b px-4 py-2.5 text-left transition-colors hover:bg-accent/60",
          selected && "bg-accent",
        )}
      >
        {compact ? (
          <>
            <span
              className="size-2 shrink-0 rounded-full"
              style={{ backgroundColor: color }}
              aria-hidden
            />
            <span
              className={cn(
                "w-[28%] shrink-0 truncate text-sm",
                m.seen ? "text-muted-foreground" : "font-semibold",
              )}
            >
              {m.from.name || m.from.email}
            </span>
            <span className="min-w-0 flex-1 truncate text-sm">
              <span className={cn(!m.seen && "font-medium")}>{m.subject || "(sin asunto)"}</span>
              {m.snippet && <span className="text-muted-foreground"> — {m.snippet}</span>}
            </span>
            {folderLabel && (
              <span className="shrink-0 truncate text-xs text-muted-foreground/80" data-testid="folder-label">
                {folderLabel}
              </span>
            )}
            {m.hasAttachments && (
              <PaperclipIcon className="size-3.5 shrink-0 text-muted-foreground" aria-label="Tiene adjuntos" />
            )}
            <span className="shrink-0 text-xs text-muted-foreground">{relativeDate(m.date)}</span>
            <span className="size-6 shrink-0" aria-hidden />
          </>
        ) : (
          <>
            <span className="flex items-center gap-2">
              <span
                className="size-2 shrink-0 rounded-full"
                style={{ backgroundColor: color }}
                aria-hidden
              />
              <span
                className={cn(
                  "min-w-0 flex-1 truncate text-sm",
                  m.seen ? "text-muted-foreground" : "font-semibold",
                )}
              >
                {m.from.name || m.from.email}
              </span>
              {m.hasAttachments && (
                <PaperclipIcon className="size-3.5 shrink-0 text-muted-foreground" aria-label="Tiene adjuntos" />
              )}
              <span className="shrink-0 text-xs text-muted-foreground">{relativeDate(m.date)}</span>
              <span className="size-6 shrink-0" aria-hidden />
            </span>
            <span className={cn("truncate pl-4 text-sm", !m.seen && "font-medium")}>
              {m.subject || "(sin asunto)"}
            </span>
            <span className="truncate pl-4 text-xs text-muted-foreground">{m.snippet}</span>
            {folderLabel && (
              <span className="truncate pl-4 text-xs text-muted-foreground/80" data-testid="folder-label">
                {folderLabel}
              </span>
            )}
          </>
        )}
      </button>
      <button
        type="button"
        aria-label={m.flagged ? "Quitar destacado" : "Destacar"}
        aria-pressed={m.flagged}
        onClick={(e) => {
          e.stopPropagation();
          void toggleFlag();
        }}
        className={cn(
          "absolute right-3 inline-flex size-6 items-center justify-center rounded text-muted-foreground hover:text-foreground",
          compact ? "top-1/2 -translate-y-1/2" : "top-2",
        )}
      >
        <StarIcon className={cn("size-3.5", m.flagged && "fill-yellow-400 text-yellow-500")} />
      </button>
    </li>
  );
}

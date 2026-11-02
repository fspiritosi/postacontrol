import { PaperclipIcon, StarIcon } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import type { MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { startMessageDrag, type DragInfo } from "@/lib/dnd";
import { errorMessage } from "@/lib/errors";
import { relativeDate } from "@/lib/format";
import { useReadingPane } from "@/lib/settings";
import { cn } from "@/lib/utils";
import { LabelChips, LabelLine } from "../labels/LabelStripe";
import type { ClickMods } from "./useMarking";

type Props = {
  message: MessageSummary;
  color?: string;
  selected: boolean;
  // Clic en la fila, con las teclas que tenía apretadas.
  onSelect: (id: number, mods: ClickMods) => void;
  // Marcado para actuar sobre varios, y si hay algo marcado en la lista.
  marked: boolean;
  anyMarked: boolean;
  // La casilla: marca o desmarca sin abrir.
  onToggle: (id: number) => void;
  // Etiqueta de la carpeta (en los resultados de búsqueda).
  folderLabel?: string;
  // Qué lleva al arrastrarla a una carpeta (null: no se puede arrastrar ahora).
  dragOf?: (m: MessageSummary) => DragInfo | null;
};

export function MessageRow({
  message: m,
  color,
  selected,
  onSelect,
  marked,
  anyMarked,
  onToggle,
  folderLabel,
  dragOf,
}: Props) {
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
    <li
      className={cn("group relative", marked && "bg-primary/10")}
      data-unread={!m.seen || undefined}
      data-density={compact ? "compact" : "comfortable"}
      draggable={dragOf != null}
      onDragStart={dragOf && ((e) => startMessageDrag(e, dragOf(m)))}
    >
      <LabelLine ids={m.labels} />
      <input
        type="checkbox"
        aria-label="Seleccionar"
        checked={marked}
        onChange={() => onToggle(m.id)}
        className={cn(
          "absolute left-0.5 z-10 size-3.5 cursor-pointer accent-primary focus-visible:opacity-100",
          compact ? "top-1/2 -translate-y-1/2" : "top-3.5",
          !marked && !anyMarked && "opacity-0 group-hover:opacity-100",
        )}
      />
      <button
        type="button"
        onClick={(e) =>
          onSelect(m.id, { ctrl: e.ctrlKey || e.metaKey, shift: e.shiftKey })
        }
        aria-current={selected ? "true" : undefined}
        className={cn(
          compact
            ? "flex w-full items-center gap-2 border-b px-4 py-1.5 text-left transition-colors hover:bg-accent/60"
            : "flex w-full flex-col gap-0.5 border-b px-4 py-2.5 text-left transition-colors hover:bg-accent/60",
          selected && !marked && "bg-accent",
          !m.seen && !selected && !marked && "bg-unread",
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
                m.seen
                  ? "text-muted-foreground"
                  : "font-semibold text-unread-foreground",
              )}
            >
              {m.from.name || m.from.email}
            </span>
            <LabelChips ids={m.labels} max={2} />
            <span className="min-w-0 flex-1 truncate text-sm">
              <span className={cn(!m.seen && "font-medium")}>
                {m.subject || "(sin asunto)"}
              </span>
              {m.snippet && (
                <span className="text-muted-foreground"> — {m.snippet}</span>
              )}
            </span>
            {folderLabel && (
              <span
                className="shrink-0 truncate text-xs text-muted-foreground/80"
                data-testid="folder-label"
              >
                {folderLabel}
              </span>
            )}
            {m.hasAttachments && (
              <PaperclipIcon
                className="size-3.5 shrink-0 text-muted-foreground"
                aria-label="Tiene adjuntos"
              />
            )}
            <span className="shrink-0 text-xs text-muted-foreground">
              {relativeDate(m.date)}
            </span>
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
                  m.seen
                  ? "text-muted-foreground"
                  : "font-semibold text-unread-foreground",
                )}
              >
                {m.from.name || m.from.email}
              </span>
              {m.hasAttachments && (
                <PaperclipIcon
                  className="size-3.5 shrink-0 text-muted-foreground"
                  aria-label="Tiene adjuntos"
                />
              )}
              <span className="shrink-0 text-xs text-muted-foreground">
                {relativeDate(m.date)}
              </span>
              <span className="size-6 shrink-0" aria-hidden />
            </span>
            <span className="flex items-center gap-1.5 pl-4">
              <span
                className={cn(
                  "min-w-0 truncate text-sm",
                  !m.seen && "font-medium",
                )}
              >
                {m.subject || "(sin asunto)"}
              </span>
              <LabelChips ids={m.labels} max={3} />
            </span>
            <span className="truncate pl-4 text-xs text-muted-foreground">
              {m.snippet}
            </span>
            {folderLabel && (
              <span
                className="truncate pl-4 text-xs text-muted-foreground/80"
                data-testid="folder-label"
              >
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
        <StarIcon
          className={cn(
            "size-3.5",
            m.flagged && "fill-yellow-400 text-yellow-500",
          )}
        />
      </button>
    </li>
  );
}

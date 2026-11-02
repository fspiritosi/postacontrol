import { PaperclipIcon, StarIcon } from "lucide-react";
import type { ThreadSummary } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import { relativeDate } from "@/lib/format";
import { useThreads } from "@/lib/queries";
import type { Marks } from "@/lib/selection";
import { useReadingPane } from "@/lib/settings";
import { cn } from "@/lib/utils";
import { useMarking, type ClickMods } from "./useMarking";

type Props = {
  folderId: number;
  unreadOnly: boolean;
  // La carpeta se está sincronizando (para el aviso con la lista vacía).
  syncing: boolean;
  // Color de la cuenta de la carpeta.
  color?: string;
  selected: string | null;
  onSelect: (root: string) => void;
  // Conversaciones marcadas para actuar sobre varias.
  marked: Marks<string>;
  onMark: (m: Marks<string>) => void;
};

// Fila de un hilo: mismos estilos que MessageRow, con los participantes en
// lugar del remitente y un contador si tiene más de un mensaje.
function ThreadRow({
  thread: t,
  color,
  selected,
  onSelect,
  marked,
  anyMarked,
  onToggle,
}: {
  thread: ThreadSummary;
  color?: string;
  selected: boolean;
  onSelect: (root: string, mods: ClickMods) => void;
  marked: boolean;
  anyMarked: boolean;
  onToggle: (root: string) => void;
}) {
  const compact = useReadingPane() !== "right";
  const m = t.latest;
  const unread = t.unread > 0;
  const participants = (t.participants ?? []).join(", ") || m.from.name || m.from.email;
  return (
    <li
      className={cn("group relative", marked && "bg-primary/10")}
      data-density={compact ? "compact" : "comfortable"}
    >
      <input
        type="checkbox"
        aria-label="Seleccionar"
        checked={marked}
        onChange={() => onToggle(t.threadRoot)}
        className={cn(
          "absolute left-0.5 z-10 size-3.5 cursor-pointer accent-primary focus-visible:opacity-100",
          compact ? "top-1/2 -translate-y-1/2" : "top-3.5",
          !marked && !anyMarked && "opacity-0 group-hover:opacity-100",
        )}
      />
      <button
        type="button"
        onClick={(e) => onSelect(t.threadRoot, { ctrl: e.ctrlKey || e.metaKey, shift: e.shiftKey })}
        aria-current={selected ? "true" : undefined}
        className={cn(
          compact
            ? "flex w-full items-center gap-2 border-b px-4 py-1.5 text-left transition-colors hover:bg-accent/60"
            : "flex w-full flex-col gap-0.5 border-b px-4 py-2.5 text-left transition-colors hover:bg-accent/60",
          selected && !marked && "bg-accent",
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
                unread ? "font-semibold" : "text-muted-foreground",
              )}
            >
              {participants}
            </span>
            {t.count > 1 && (
              <span
                className="shrink-0 rounded bg-muted px-1.5 text-xs text-muted-foreground tabular-nums"
                data-testid="thread-count"
              >
                {t.count}
              </span>
            )}
            <span className="min-w-0 flex-1 truncate text-sm">
              <span className={cn(unread && "font-medium")}>{m.subject || "(sin asunto)"}</span>
              {m.snippet && <span className="text-muted-foreground"> — {m.snippet}</span>}
            </span>
            {m.hasAttachments && (
              <PaperclipIcon className="size-3.5 shrink-0 text-muted-foreground" aria-label="Tiene adjuntos" />
            )}
            <span className="shrink-0 text-xs text-muted-foreground">{relativeDate(m.date)}</span>
            {m.flagged && (
              <StarIcon
                className="size-3.5 shrink-0 fill-yellow-400 text-yellow-500"
                aria-label="Destacado"
              />
            )}
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
                  unread ? "font-semibold" : "text-muted-foreground",
                )}
              >
                {participants}
              </span>
              {t.count > 1 && (
                <span
                  className="shrink-0 rounded bg-muted px-1.5 text-xs text-muted-foreground tabular-nums"
                  data-testid="thread-count"
                >
                  {t.count}
                </span>
              )}
              {m.hasAttachments && (
                <PaperclipIcon className="size-3.5 shrink-0 text-muted-foreground" aria-label="Tiene adjuntos" />
              )}
              <span className="shrink-0 text-xs text-muted-foreground">{relativeDate(m.date)}</span>
              {m.flagged && (
                <StarIcon
                  className="size-3.5 shrink-0 fill-yellow-400 text-yellow-500"
                  aria-label="Destacado"
                />
              )}
            </span>
            <span className={cn("truncate pl-4 text-sm", unread && "font-medium")}>
              {m.subject || "(sin asunto)"}
            </span>
            <span className="truncate pl-4 text-xs text-muted-foreground">{m.snippet}</span>
          </>
        )}
      </button>
    </li>
  );
}

export function ThreadList({
  folderId,
  unreadOnly,
  syncing,
  color,
  selected,
  onSelect,
  marked,
  onMark,
}: Props) {
  const threads = useThreads(folderId, unreadOnly);
  const items = threads.data?.pages.flat() ?? [];
  const marking = useMarking({
    marks: marked,
    onMark,
    order: items.map((t) => t.threadRoot),
    open: selected,
    onOpen: onSelect,
  });
  return (
    <div className="flex-1 overflow-y-auto outline-none" tabIndex={-1} onKeyDown={marking.onKeyDown}>
      {threads.isError && (
        <p className="p-4 text-sm text-destructive">{errorMessage(threads.error)}</p>
      )}
      {!threads.isPending && !threads.isError && items.length === 0 && (
        <p className="p-6 text-center text-sm text-muted-foreground" aria-live="polite">
          {syncing ? "Sincronizando…" : "No hay correos en esta carpeta."}
        </p>
      )}
      <ul>
        {items.map((t) => (
          <ThreadRow
            key={t.threadRoot}
            thread={t}
            color={color}
            selected={selected === t.threadRoot}
            onSelect={marking.select}
            marked={marked.keys.includes(t.threadRoot)}
            anyMarked={marking.any}
            onToggle={marking.toggle}
          />
        ))}
      </ul>
      {threads.hasNextPage && (
        <div className="p-3 text-center">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void threads.fetchNextPage()}
            disabled={threads.isFetchingNextPage}
          >
            Cargar más
          </Button>
        </div>
      )}
    </div>
  );
}

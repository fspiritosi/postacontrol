import { PaperclipIcon } from "lucide-react";
import { useState } from "react";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import { useSyncProgress } from "@/lib/events";
import { folderLabel } from "@/lib/folders";
import { relativeDate } from "@/lib/format";
import { useFolders, useMessages } from "@/lib/queries";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";

export function MessageList({ account, state }: { account: Account; state: ShellState }) {
  const [unreadOnly, setUnreadOnly] = useState(false);
  const folders = useFolders(account.id);
  const messages = useMessages(state.folderId, unreadOnly);
  const progress = useSyncProgress();

  const folder = folders.data?.find((f) => f.id === state.folderId);
  const items = messages.data?.pages.flat() ?? [];
  const syncing = progress != null && progress.folderId === state.folderId;

  if (state.folderId == null) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground">
        Elegí una carpeta
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between gap-2 border-b px-4 py-3">
        <h2 className="truncate text-sm font-semibold">
          {folder ? folderLabel(folder) : ""}
        </h2>
        <div className="flex rounded-md bg-muted p-0.5 text-xs" role="group" aria-label="Filtro">
          {[
            { label: "Todos", value: false },
            { label: "No leídos", value: true },
          ].map((o) => (
            <button
              key={o.label}
              type="button"
              aria-pressed={unreadOnly === o.value}
              onClick={() => setUnreadOnly(o.value)}
              className={cn(
                "rounded px-2.5 py-1 text-muted-foreground transition-colors",
                unreadOnly === o.value && "bg-background text-foreground shadow-sm",
              )}
            >
              {o.label}
            </button>
          ))}
        </div>
      </header>

      <div className="h-0.5 bg-transparent">
        {syncing && progress.total > 0 && (
          <div
            className="h-full bg-primary transition-[width]"
            style={{ width: `${Math.min(100, (progress.done / progress.total) * 100)}%` }}
            role="progressbar"
            aria-label="Sincronizando"
          />
        )}
      </div>

      <div className="flex-1 overflow-y-auto">
        {messages.isError && (
          <p className="p-4 text-sm text-destructive">{errorMessage(messages.error)}</p>
        )}
        {!messages.isPending && !messages.isError && items.length === 0 && (
          <p className="p-6 text-center text-sm text-muted-foreground">
            No hay correos en esta carpeta.
          </p>
        )}
        <ul>
          {items.map((m) => {
            const selected = state.messageId === m.id;
            return (
              <li key={m.id}>
                <button
                  type="button"
                  onClick={() => state.selectMessage(m.id)}
                  aria-current={selected ? "true" : undefined}
                  className={cn(
                    "flex w-full flex-col gap-0.5 border-b px-4 py-2.5 text-left transition-colors hover:bg-accent/60",
                    selected && "bg-accent",
                  )}
                >
                  <span className="flex items-center gap-2">
                    <span
                      className="size-2 shrink-0 rounded-full"
                      style={{ backgroundColor: account.color }}
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
                    <span className="shrink-0 text-xs text-muted-foreground">
                      {relativeDate(m.date)}
                    </span>
                  </span>
                  <span className={cn("truncate pl-4 text-sm", !m.seen && "font-medium")}>
                    {m.subject || "(sin asunto)"}
                  </span>
                  <span className="truncate pl-4 text-xs text-muted-foreground">{m.snippet}</span>
                </button>
              </li>
            );
          })}
        </ul>
        {messages.hasNextPage && (
          <div className="p-3 text-center">
            <Button
              variant="outline"
              size="sm"
              onClick={() => void messages.fetchNextPage()}
              disabled={messages.isFetchingNextPage}
            >
              Cargar más
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

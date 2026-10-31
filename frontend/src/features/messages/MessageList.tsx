import { PaperclipIcon, StarIcon } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "@/lib/errors";
import { useFolderSyncing, useSyncProgress } from "@/lib/events";
import { folderLabel } from "@/lib/folders";
import { relativeDate } from "@/lib/format";
import { useAllFolders, useMessages } from "@/lib/queries";
import { cn } from "@/lib/utils";
import { UNIFIED_LABEL } from "@/lib/view";
import type { ShellState } from "../layout/MailShell";

export function MessageList({ accounts, state }: { accounts: Account[]; state: ShellState }) {
  const queryClient = useQueryClient();
  const [unreadOnly, setUnreadOnly] = useState(false);
  const { view } = state;
  const folderId = view?.kind === "folder" ? view.folderId : null;
  const { folders } = useAllFolders(accounts);
  const messages = useMessages(view, unreadOnly);
  const progress = useSyncProgress();
  const folderSyncing = useFolderSyncing(folderId);

  const folder = folders.find((f) => f.id === folderId);
  const title =
    view?.kind === "unified" ? UNIFIED_LABEL[view.role] : folder ? folderLabel(folder) : "";
  const items = messages.data?.pages.flat() ?? [];
  const syncing = progress != null && folderId != null && progress.folderId === folderId;
  const colorOf = (accountId: number) => accounts.find((a) => a.id === accountId)?.color;

  const toggleFlag = async (id: number, flagged: boolean) => {
    try {
      await svc.SetFlagged([id], !flagged);
      void queryClient.invalidateQueries({ queryKey: ["messages"] });
      void queryClient.invalidateQueries({ queryKey: ["message", id] });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  if (view == null || view.kind === "outbox") {
    return (
      <div className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground">
        Elegí una carpeta
      </div>
    );
  }

  return (
    <div className="flex h-full flex-col">
      <header className="flex items-center justify-between gap-2 border-b px-4 py-3">
        <h2 className="truncate text-sm font-semibold">{title}</h2>
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
          <p className="p-6 text-center text-sm text-muted-foreground" aria-live="polite">
            {folderSyncing
              ? "Sincronizando…"
              : view.kind === "unified"
                ? "No hay correos en esta vista."
                : "No hay correos en esta carpeta."}
          </p>
        )}
        <ul>
          {items.map((m) => {
            const selected = state.messageId === m.id;
            return (
              <li key={m.id} className="relative">
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
                      style={{ backgroundColor: colorOf(m.accountId) }}
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
                    <span className="size-6 shrink-0" aria-hidden />
                  </span>
                  <span className={cn("truncate pl-4 text-sm", !m.seen && "font-medium")}>
                    {m.subject || "(sin asunto)"}
                  </span>
                  <span className="truncate pl-4 text-xs text-muted-foreground">{m.snippet}</span>
                </button>
                <button
                  type="button"
                  aria-label={m.flagged ? "Quitar destacado" : "Destacar"}
                  aria-pressed={m.flagged}
                  onClick={(e) => {
                    e.stopPropagation();
                    void toggleFlag(m.id, m.flagged);
                  }}
                  className="absolute right-3 top-2 inline-flex size-6 items-center justify-center rounded text-muted-foreground hover:text-foreground"
                >
                  <StarIcon
                    className={cn("size-3.5", m.flagged && "fill-yellow-400 text-yellow-500")}
                  />
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

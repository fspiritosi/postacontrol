import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { errorMessage } from "@/lib/errors";
import { useFolderSyncing, useSyncProgress } from "@/lib/events";
import { folderLabel } from "@/lib/folders";
import { useAllFolders, useMessages, useSettings } from "@/lib/queries";
import { cn } from "@/lib/utils";
import { UNIFIED_LABEL } from "@/lib/view";
import type { ShellState } from "../layout/MailShell";
import { SearchBox } from "../search/SearchBox";
import { SearchResults } from "../search/SearchResults";
import { MessageRow } from "./MessageRow";
import { ThreadList } from "./ThreadList";
import { UnsyncedDraftsBanner } from "./UnsyncedDraftsBanner";

export function MessageList({
  accounts,
  state,
}: {
  accounts: Account[];
  state: ShellState;
}) {
  const [unreadOnly, setUnreadOnly] = useState(false);
  const { view, search } = state;
  const folderId = view?.kind === "folder" ? view.folderId : null;
  const { folders } = useAllFolders(accounts);
  const queryClient = useQueryClient();
  const settings = useSettings();
  const threadedOn = settings.data?.threadedList ?? false;
  // Las conversaciones son por carpeta: ni en la unificada ni en la búsqueda.
  const canThread = folderId != null && search == null;
  const threaded = canThread && threadedOn;
  const messages = useMessages(view, unreadOnly, !threaded);
  const progress = useSyncProgress();
  const folderSyncing = useFolderSyncing(folderId);

  const folder = folders.find((f) => f.id === folderId);
  const title =
    view?.kind === "unified"
      ? UNIFIED_LABEL[view.role]
      : folder
        ? folderLabel(folder)
        : "";
  // En Borradores se avisa de los que todavía no llegaron al servidor.
  const draftsView =
    view?.kind === "unified" ? view.role === "drafts" : folder?.role === "drafts";
  const items = messages.data?.pages.flat() ?? [];
  const syncing =
    progress != null && folderId != null && progress.folderId === folderId;
  const colorOf = (accountId: number) =>
    accounts.find((a) => a.id === accountId)?.color;

  // Se guarda al instante, conservando el resto de la configuración.
  const setThreaded = async (v: boolean) => {
    try {
      await svc.SetSettings({
        unifiedInbox: false,
        disableNotifications: false,
        ...settings.data,
        threadedList: v,
      });
      if (!v) state.selectThread(null);
      void queryClient.invalidateQueries({ queryKey: ["settings"] });
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
      <header className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
        <h2 className="min-w-0 flex-1 truncate text-sm font-semibold">{title}</h2>
        {canThread && (
          <div className="flex items-center gap-1.5">
            <Switch
              id="threaded-list"
              size="sm"
              checked={threadedOn}
              disabled={settings.isPending}
              onCheckedChange={(v) => void setThreaded(v)}
            />
            <Label htmlFor="threaded-list" className="text-xs font-normal text-muted-foreground">
              Conversaciones
            </Label>
          </div>
        )}
        <div
          className="flex rounded-md bg-muted p-0.5 text-xs"
          role="group"
          aria-label="Filtro"
        >
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
                unreadOnly === o.value &&
                  "bg-background text-foreground shadow-sm",
              )}
            >
              {o.label}
            </button>
          ))}
        </div>
      </header>
      <div className="border-b px-3 py-2">
        <SearchBox
          active={search?.text ?? null}
          onSearch={(text) =>
            state.setSearch(
              text == null ? null : { text, scope: search?.scope ?? "all" },
            )
          }
        />
      </div>

      <div className="h-0.5 bg-transparent">
        {syncing && progress.total > 0 && (
          <div
            className="h-full bg-primary transition-[width]"
            style={{
              width: `${Math.min(100, (progress.done / progress.total) * 100)}%`,
            }}
            role="progressbar"
            aria-label="Sincronizando"
          />
        )}
      </div>

      {draftsView && search == null && (
        <UnsyncedDraftsBanner accountId={folder ? folder.accountId : null} />
      )}

      {search != null ? (
        <SearchResults accounts={accounts} state={state} search={search} />
      ) : threaded && folderId != null ? (
        <ThreadList
          folderId={folderId}
          unreadOnly={unreadOnly}
          syncing={folderSyncing}
          color={folder ? colorOf(folder.accountId) : undefined}
          selected={state.threadRoot}
          onSelect={state.selectThread}
        />
      ) : (
        <div className="flex-1 overflow-y-auto">
          {messages.isError && (
            <p className="p-4 text-sm text-destructive">
              {errorMessage(messages.error)}
            </p>
          )}
          {!messages.isPending && !messages.isError && items.length === 0 && (
            <p
              className="p-6 text-center text-sm text-muted-foreground"
              aria-live="polite"
            >
              {folderSyncing
                ? "Sincronizando…"
                : view.kind === "unified"
                  ? "No hay correos en esta vista."
                  : "No hay correos en esta carpeta."}
            </p>
          )}
          <ul>
            {items.map((m) => (
              <MessageRow
                key={m.id}
                message={m}
                color={colorOf(m.accountId)}
                selected={state.messageId === m.id}
                onSelect={state.selectMessage}
              />
            ))}
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
      )}
    </div>
  );
}

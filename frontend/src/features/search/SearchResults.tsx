import { useState } from "react";
import type { Account, MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import { folderLabel } from "@/lib/folders";
import { useAllFolders, useSearch } from "@/lib/queries";
import { cn } from "@/lib/utils";
import { MessageRow } from "../messages/MessageRow";
import { useMarking } from "../messages/useMarking";
import type { SearchScope, SearchState, ShellState } from "../layout/MailShell";

type Props = {
  accounts: Account[];
  state: ShellState;
  search: SearchState;
};

const SCOPES: { label: string; value: SearchScope }[] = [
  { label: "En todo", value: "all" },
  { label: "Solo en esta carpeta", value: "folder" },
];

export function SearchResults({ accounts, state, search }: Props) {
  const { view } = state;
  const inFolder = view?.kind === "folder";
  const folderId = inFolder && search.scope === "folder" ? view.folderId : 0;
  const { folders } = useAllFolders(accounts);
  const results = useSearch(search.text, folderId);
  // Los resultados del servidor se descartan al cambiar el texto o el alcance.
  const [server, setServer] = useState<{ key: string; items: MessageSummary[] } | null>(null);
  const [serverLoading, setServerLoading] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);

  const key = `${search.text}\u0000${folderId}`;
  // Si entra un correo entre página y página los resultados se corren y uno
  // puede repetirse: cada mensaje aparece una sola vez.
  const seen = new Set<number>();
  const items: MessageSummary[] = [];
  for (const m of [
    ...(results.data?.pages.flatMap((p) => p.messages) ?? []),
    ...(server?.key === key ? server.items : []),
  ]) {
    if (seen.has(m.id)) continue;
    seen.add(m.id);
    items.push(m);
  }
  const pending = results.data?.pages[0]?.pending ?? 0;

  const marking = useMarking({
    marks: state.marked,
    onMark: state.setMarked,
    order: items.map((m) => m.id),
    open: state.messageId,
    onOpen: state.selectMessage,
  });

  const colorOf = (accountId: number) => accounts.find((a) => a.id === accountId)?.color;
  const labelOf = (id: number) => {
    const f = folders.find((x) => x.id === id);
    return f ? folderLabel(f) : undefined;
  };

  const currentLabel = inFolder ? labelOf(view.folderId) : undefined;
  const serverLabel = currentLabel ? `Buscar en el servidor en ${currentLabel}` : "Buscar en el servidor";

  const searchServer = async () => {
    if (!inFolder) return;
    setServerLoading(true);
    setServerError(null);
    try {
      const found = (await svc.SearchServer(view.folderId, search.text)) ?? [];
      setServer({ key, items: found });
    } catch (e) {
      setServerError(errorMessage(e));
    } finally {
      setServerLoading(false);
    }
  };

  return (
    <div className="flex-1 overflow-y-auto outline-none" tabIndex={-1} onKeyDown={marking.onKeyDown}>
      {inFolder && (
        <div className="flex rounded-md bg-muted p-0.5 text-xs m-2" role="group" aria-label="Alcance">
          {SCOPES.map((o) => (
            <button
              key={o.value}
              type="button"
              aria-pressed={search.scope === o.value}
              onClick={() => state.setSearch({ text: search.text, scope: o.value })}
              className={cn(
                "flex-1 rounded px-2.5 py-1 text-muted-foreground transition-colors",
                search.scope === o.value && "bg-background text-foreground shadow-sm",
              )}
            >
              {o.label}
            </button>
          ))}
        </div>
      )}
      {pending > 0 && (
        <div className="flex flex-col items-start gap-2 border-b px-4 py-2 text-xs text-muted-foreground">
          <p>Todavía faltan indexar {pending} correos.</p>
          {inFolder && (
            <Button variant="outline" size="sm" onClick={() => void searchServer()} disabled={serverLoading}>
              {serverLabel}
            </Button>
          )}
        </div>
      )}
      {serverError && <p className="p-4 text-sm text-destructive">{serverError}</p>}
      {results.isError && <p className="p-4 text-sm text-destructive">{errorMessage(results.error)}</p>}
      {!results.isPending && !results.isError && items.length === 0 && (
        <p className="p-6 text-center text-sm text-muted-foreground" aria-live="polite">
          No encontramos correos.
        </p>
      )}
      <ul>
        {items.map((m) => (
          <MessageRow
            key={m.id}
            message={m}
            color={colorOf(m.accountId)}
            selected={state.messageId === m.id}
            onSelect={marking.select}
            marked={state.marked.keys.includes(m.id)}
            anyMarked={marking.any}
            onToggle={marking.toggle}
            folderLabel={labelOf(m.folderId)}
          />
        ))}
      </ul>
      {results.hasNextPage && (
        <div className="p-3 text-center">
          <Button
            variant="outline"
            size="sm"
            onClick={() => void results.fetchNextPage()}
            disabled={results.isFetchingNextPage}
          >
            Cargar más
          </Button>
        </div>
      )}
    </div>
  );
}

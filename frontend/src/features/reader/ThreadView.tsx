import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { useState } from "react";
import type { Account, MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import { fullDate, relativeDate } from "@/lib/format";
import { useAllFolders, useThread } from "@/lib/queries";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";
import { MessageView, type ComposeMode } from "./MessageView";

type Props = {
  accounts: Account[];
  state: ShellState;
  onCompose?: (messageId: number, mode: ComposeMode) => void;
};

// Encabezado de un mensaje del hilo: remitente, "Enviado" si es de Enviados,
// fecha y, plegado, el snippet.
function Header({ message: m, sent, expanded }: { message: MessageSummary; sent: boolean; expanded: boolean }) {
  return (
    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
      <span className="flex items-center gap-2">
        <span className={cn("min-w-0 truncate text-sm", m.seen ? "font-medium" : "font-semibold")}>
          {m.from.name || m.from.email}
        </span>
        {sent && (
          <Badge variant="secondary" className="shrink-0">
            Enviado
          </Badge>
        )}
        <span className="ml-auto shrink-0 text-xs text-muted-foreground" title={fullDate(m.date)}>
          {relativeDate(m.date)}
        </span>
      </span>
      {!expanded && <span className="truncate text-xs text-muted-foreground">{m.snippet}</span>}
    </span>
  );
}

export function ThreadView({ accounts, state, onCompose }: Props) {
  const folderId = state.view?.kind === "folder" ? state.view.folderId : null;
  const thread = useThread(folderId, state.threadRoot);
  const { folders, pending: foldersPending } = useAllFolders(accounts);
  // Mensajes plegados que se abrieron; el enfocado siempre está abierto.
  const [opened, setOpened] = useState<Set<number>>(() => new Set());
  // El que se abre solo: se fija al cargar el hilo para que no salte a otro
  // cuando abrirlo lo marca leído y el hilo se recarga.
  const [focusId, setFocusId] = useState<number | null>(null);

  // Hacen falta las carpetas para saber cuáles son de Enviados.
  if (thread.isPending || foldersPending) return <div className="h-full" aria-busy />;
  if (thread.isError) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm">
        <p className="text-destructive">{errorMessage(thread.error)}</p>
        <Button size="sm" variant="outline" onClick={() => void thread.refetch()}>
          Reintentar
        </Button>
      </div>
    );
  }
  const messages = thread.data;
  if (messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground">
        Esta conversación ya no tiene correos.
      </div>
    );
  }

  const sentIds = new Set(folders.filter((f) => f.role === "sent").map((f) => f.id));
  const last = messages[messages.length - 1];
  // Se abre el no leído más nuevo (los de Enviados no cuentan); si no hay, el último.
  const pinned = messages.find((m) => m.id === focusId);
  const focus = pinned ?? [...messages].reverse().find((m) => !m.seen && !sentIds.has(m.folderId)) ?? last;
  if (!pinned) setFocusId(focus.id);
  const toggle = (id: number) =>
    setOpened((prev) => {
      const next = new Set(prev);
      if (!next.delete(id)) next.add(id);
      return next;
    });
  // Cerrar un mensaje (mover, borrar, marcar como no leído) cierra el hilo.
  const selectMessage = (id: number | null) =>
    id == null ? state.selectThread(null) : state.selectMessage(id);

  return (
    <div className="flex h-full flex-col">
      <h1 className="border-b px-6 py-4 text-lg font-semibold leading-snug">
        {last.subject || "(sin asunto)"}
      </h1>
      <ul className="flex-1 overflow-y-auto">
        {messages.map((m) => {
          const isFocus = m.id === focus.id;
          const expanded = isFocus || opened.has(m.id);
          const sent = sentIds.has(m.folderId);
          return (
            <li key={m.id} className="border-b" data-testid="thread-message">
              {isFocus ? (
                <div className="flex items-center gap-2 px-4 py-2.5">
                  <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                  <Header message={m} sent={sent} expanded />
                </div>
              ) : (
                <button
                  type="button"
                  aria-expanded={expanded}
                  onClick={() => toggle(m.id)}
                  className="flex w-full items-center gap-2 px-4 py-2.5 text-left transition-colors hover:bg-accent/60"
                >
                  {expanded ? (
                    <ChevronDownIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                  ) : (
                    <ChevronRightIcon className="size-4 shrink-0 text-muted-foreground" aria-hidden />
                  )}
                  <Header message={m} sent={sent} expanded={expanded} />
                </button>
              )}
              {expanded && (
                <div className="pb-2">
                  <MessageView
                    accounts={accounts}
                    state={state}
                    onCompose={onCompose}
                    messageId={m.id}
                    selectMessage={selectMessage}
                    embedded
                  />
                </div>
              )}
            </li>
          );
        })}
      </ul>
    </div>
  );
}

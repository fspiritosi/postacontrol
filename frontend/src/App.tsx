import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { ComposerProvider } from "./features/compose/ComposerContext";
import { Welcome } from "./features/accounts/Welcome";
import { FolderSidebar } from "./features/folders/FolderSidebar";
import { ContactList } from "./features/contacts/ContactList";
import { ContactPanel } from "./features/contacts/ContactPanel";
import { MailShell } from "./features/layout/MailShell";
import { MessageList } from "./features/messages/MessageList";
import { OutboxList } from "./features/outbox/OutboxList";
import { MessageView } from "./features/reader/MessageView";
import { ThreadView } from "./features/reader/ThreadView";
import { useMailEvents } from "./lib/events";
import { useAccounts, useDrafts } from "./lib/queries";
import { errorMessage } from "./lib/errors";

// Avisa una vez al arrancar si quedaron correos sin enviar en la Bandeja de salida.
function DraftsNotice({ onOpen }: { onOpen: () => void }) {
  const drafts = useDrafts();
  const shown = useRef(false);
  const openRef = useRef(onOpen);
  useEffect(() => {
    openRef.current = onOpen;
  }, [onOpen]);
  useEffect(() => {
    if (shown.current || !drafts.data) return;
    shown.current = true; // solo la primera carga: lo que falle después ya se avisó
    const n = drafts.data.filter((d) => d.status === "failed").length;
    if (n === 0) return;
    toast(`Tenés ${n} ${n === 1 ? "correo" : "correos"} sin enviar`, {
      id: "local-drafts",
      duration: Infinity,
      action: { label: "Ver", onClick: () => openRef.current() },
    });
  }, [drafts.data]);
  return null;
}

export default function App() {
  const queryClient = useQueryClient();
  useMailEvents(queryClient);
  const accounts = useAccounts();

  if (accounts.isPending) return <div className="h-screen bg-background" />;
  if (accounts.isError) {
    return (
      <div className="flex h-screen items-center justify-center p-6 text-sm text-destructive">
        {errorMessage(accounts.error)}
      </div>
    );
  }
  if (accounts.data.length === 0) return <Welcome />;
  const all = accounts.data;
  return (
    <ComposerProvider>
      <MailShell
        accounts={all}
        extras={(s) => <DraftsNotice onOpen={() => s.selectView({ kind: "outbox" })} />}
        sidebar={(s) => <FolderSidebar accounts={all} state={s} />}
        list={(s) =>
          s.view?.kind === "outbox" ? (
            <OutboxList />
          ) : s.view?.kind === "contacts" ? (
            <ContactList state={s} />
          ) : (
            <MessageList accounts={all} state={s} />
          )
        }
        reader={(s) =>
          s.view?.kind === "contacts" ? (
            <ContactPanel state={s} />
          ) : s.view?.kind === "outbox" ? (
            <div className="flex h-full items-center justify-center p-6 text-center text-sm text-muted-foreground">
              Abrí un correo de la Bandeja de salida para revisarlo, o reintentá el envío.
            </div>
          ) : s.threadRoot != null ? (
            <ThreadView key={s.threadRoot} accounts={all} state={s} />
          ) : (
            <MessageView accounts={all} state={s} />
          )
        }
      />
    </ComposerProvider>
  );
}

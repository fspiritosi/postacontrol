import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { useQueryClient } from "@tanstack/react-query";
import { ComposerProvider, useComposer } from "./features/compose/ComposerContext";
import { Welcome } from "./features/accounts/Welcome";
import { FolderSidebar } from "./features/folders/FolderSidebar";
import { MailShell } from "./features/layout/MailShell";
import { MessageList } from "./features/messages/MessageList";
import { MessageView } from "./features/reader/MessageView";
import { useMailEvents } from "./lib/events";
import { useAccounts, useDrafts } from "./lib/queries";
import { errorMessage } from "./lib/errors";

// Avisa una vez al arrancar si quedaron correos sin enviar.
function DraftsNotice() {
  const drafts = useDrafts();
  const { openDraft } = useComposer();
  const shown = useRef(false);
  useEffect(() => {
    if (shown.current || !drafts.data || drafts.data.length === 0) return;
    shown.current = true;
    const list = drafts.data;
    const newest = list.reduce((a, b) => (Date.parse(b.createdAt) > Date.parse(a.createdAt) ? b : a));
    const n = list.length;
    toast(`Tenés ${n} ${n === 1 ? "correo" : "correos"} sin enviar`, {
      id: "local-drafts",
      duration: Infinity,
      action: { label: "Abrir", onClick: () => openDraft(newest) },
    });
  }, [drafts.data, openDraft]);
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
  const acc = (id: number) => all.find((a) => a.id === id) ?? all[0];
  return (
    <ComposerProvider>
      <DraftsNotice />
      <MailShell
      accounts={all}
      sidebar={(s) => <FolderSidebar account={acc(s.accountId)} state={s} />}
      list={(s) => <MessageList account={acc(s.accountId)} state={s} />}
      reader={(s) => <MessageView account={acc(s.accountId)} state={s} />}
      />
    </ComposerProvider>
  );
}

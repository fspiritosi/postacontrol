import { useQueryClient } from "@tanstack/react-query";
import { Welcome } from "./features/accounts/Welcome";
import { FolderSidebar } from "./features/folders/FolderSidebar";
import { MailShell } from "./features/layout/MailShell";
import { MessageList } from "./features/messages/MessageList";
import { MessageView } from "./features/reader/MessageView";
import { useMailEvents } from "./lib/events";
import { useAccounts } from "./lib/queries";
import { errorMessage } from "./lib/errors";

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
    <MailShell
      accounts={all}
      sidebar={(s) => <FolderSidebar account={acc(s.accountId)} state={s} />}
      list={(s) => <MessageList account={acc(s.accountId)} state={s} />}
      reader={(s) => <MessageView account={acc(s.accountId)} state={s} />}
    />
  );
}

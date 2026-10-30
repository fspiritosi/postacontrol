import { useQueryClient } from "@tanstack/react-query";
import { Welcome } from "./features/accounts/Welcome";
import { MailShell } from "./features/layout/MailShell";
import { useMailEvents } from "./lib/events";
import { useAccounts } from "./lib/queries";

export default function App() {
  const queryClient = useQueryClient();
  useMailEvents(queryClient);
  const accounts = useAccounts();

  if (accounts.isPending) return <div className="h-screen bg-background" />;
  if (accounts.isError) {
    return (
      <div className="flex h-screen items-center justify-center p-6 text-sm text-destructive">
        {accounts.error.message}
      </div>
    );
  }
  if (accounts.data.length === 0) return <Welcome />;
  return <MailShell accounts={accounts.data} />;
}

import { useQueryClient } from "@tanstack/react-query";
import { ForwardIcon, ReplyAllIcon, ReplyIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account, Address, MessageDetail } from "../../../bindings/postacontrol/core/model/models";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { errorMessage, isStaleError } from "@/lib/errors";
import { fullDate, initials } from "@/lib/format";
import { useMessage } from "@/lib/queries";
import type { ShellState } from "../layout/MailShell";
import { AttachmentList } from "./AttachmentList";
import { SafeHtmlFrame } from "./SafeHtmlFrame";

export type ComposeMode = "reply" | "replyAll" | "forward";

type Props = {
  account: Account;
  state: ShellState;
  // Lo provee el redactor (Task 17); sin él los botones quedan deshabilitados.
  onCompose?: (messageId: number, mode: ComposeMode) => void;
};

const addr = (a: Address) => a.name || a.email;

function Body({ message, account, onCompose }: { message: MessageDetail; account: Account; onCompose?: Props["onCompose"] }) {
  // El estado vive acá, con key por mensaje: "Mostrar imágenes" vale solo para este.
  const [allowRemote, setAllowRemote] = useState(false);
  const to = message.to ?? [];
  const actions: { mode: ComposeMode; label: string; icon: React.ReactNode }[] = [
    { mode: "reply", label: "Responder", icon: <ReplyIcon /> },
    { mode: "replyAll", label: "Responder a todos", icon: <ReplyAllIcon /> },
    { mode: "forward", label: "Reenviar", icon: <ForwardIcon /> },
  ];
  return (
    <div className="flex h-full flex-col">
      <div className="flex gap-2 border-b px-4 py-2">
        {actions.map((a) => (
          <Button
            key={a.mode}
            variant="outline"
            size="sm"
            disabled={!onCompose}
            onClick={() => onCompose?.(message.id, a.mode)}
          >
            {a.icon}
            {a.label}
          </Button>
        ))}
      </div>
      <div className="flex-1 overflow-y-auto">
        <div className="space-y-3 px-6 pt-5">
          <h1 className="text-lg font-semibold leading-snug">{message.subject || "(sin asunto)"}</h1>
          <div className="flex items-start gap-3">
            <Avatar className="size-9">
              <AvatarFallback>{initials(message.from.name, message.from.email)}</AvatarFallback>
            </Avatar>
            <div className="min-w-0 flex-1 text-sm">
              <p className="truncate">
                <span className="font-medium">{message.from.name || message.from.email}</span>
                {message.from.name && (
                  <span className="ml-1.5 text-muted-foreground">&lt;{message.from.email}&gt;</span>
                )}
              </p>
              <p className="truncate text-xs text-muted-foreground">
                para {to.length ? to.map(addr).join(", ") : "—"}
              </p>
              <p className="truncate text-xs text-muted-foreground">
                <span
                  className="mr-1.5 inline-block size-2 rounded-full align-middle"
                  style={{ backgroundColor: account.color }}
                  aria-hidden
                />
                {account.email} · {fullDate(message.date)}
              </p>
            </div>
          </div>
        </div>

        {message.hasRemoteImages && !allowRemote && (
          <div className="mx-6 mt-4 flex items-center justify-between gap-3 rounded-md border bg-muted/50 px-3 py-2 text-xs">
            <span>Este correo tiene imágenes remotas bloqueadas para proteger tu privacidad.</span>
            <Button size="sm" variant="outline" onClick={() => setAllowRemote(true)}>
              Mostrar imágenes
            </Button>
          </div>
        )}

        <SafeHtmlFrame html={message.html} text={message.text} allowRemote={allowRemote} />
      </div>
      <AttachmentList messageId={message.id} attachments={message.attachments ?? []} />
    </div>
  );
}

export function MessageView({ account, state, onCompose }: Props) {
  const queryClient = useQueryClient();
  const message = useMessage(state.messageId);

  if (state.messageId == null) {
    return (
      <div className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground">
        Elegí un correo para leerlo.
      </div>
    );
  }
  if (message.isPending) return <div className="h-full" aria-busy />;
  if (message.isError) {
    const msg = errorMessage(message.error);
    const stale = isStaleError(message.error);
    const refreshFolder = async () => {
      if (state.folderId == null) return;
      try {
        await svc.SyncFolder(state.folderId);
        state.selectMessage(null);
        void queryClient.invalidateQueries({ queryKey: ["messages", state.folderId] });
      } catch (e) {
        toast.error(errorMessage(e));
      }
    };
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-6 text-center text-sm">
        <p className="text-destructive">{msg}</p>
        {stale ? (
          <Button size="sm" variant="outline" onClick={() => void refreshFolder()}>
            Actualizar carpeta
          </Button>
        ) : (
          <Button size="sm" variant="outline" onClick={() => void message.refetch()}>
            Reintentar
          </Button>
        )}
      </div>
    );
  }
  return (
    <Body key={message.data.id} message={message.data} account={account} onCompose={onCompose} />
  );
}

import { useQueryClient } from "@tanstack/react-query";
import { ForwardIcon, ReplyAllIcon, ReplyIcon, UserIcon, UserPlusIcon, XIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account, Address, MessageDetail } from "../../../bindings/postacontrol/core/model/models";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { labelChipStyle, orderedLabels } from "@/lib/labels";
import { contactDisplayName } from "@/lib/contacts";
import { errorMessage, isStaleError } from "@/lib/errors";
import { syncFolder, syncUnified } from "@/lib/events";
import { isSnoozeFolder } from "@/lib/folders";
import { fullDate, initials } from "@/lib/format";
import { useAllFolders, useContactForEmail, useFolders, useLabels, useMessage } from "@/lib/queries";
import { applyLabel } from "../labels/applyLabel";
import { useComposer } from "../compose/ComposerContext";
import type { ShellState } from "../layout/MailShell";
import { AttachmentList } from "./AttachmentList";
import { MessageActions } from "./MessageActions";
import { SafeHtmlFrame } from "./SafeHtmlFrame";
import { UnsubscribeButton } from "./UnsubscribeButton";
import { SnoozeBanner } from "./SnoozeBanner";
import { InviteCard, isCalendar } from "./InviteCard";
import { ReaderBackdrop } from "./ReaderBackdrop";

export type ComposeMode = "reply" | "replyAll" | "forward";

type Props = {
  accounts: Account[];
  state: ShellState;
  // Por defecto abre el redactor del contexto; se puede reemplazar (tests).
  onCompose?: (messageId: number, mode: ComposeMode) => void;
  // Para embeberlo en un hilo: el mensaje a mostrar (por defecto el abierto),
  // qué hacer al cerrarlo (por defecto state.selectMessage) y sin scroll propio.
  messageId?: number | null;
  selectMessage?: (id: number | null) => void;
  embedded?: boolean;
};

const addr = (a: Address) => a.name || a.email;

// Junto al remitente: lo agrega a la libreta o, si ya está, abre su ficha.
// Al agregar, el evento mail:contacts-changed refresca useContactForEmail.
function SenderContactButton({
  from,
  openContact,
}: {
  from: Address;
  openContact: (id: number) => void;
}) {
  const contact = useContactForEmail(from.email);
  if (contact.isPending) return null;
  const add = async () => {
    try {
      const c = await svc.AddSenderToContacts(from);
      toast.success(`«${contactDisplayName(c)}» se agregó a tus contactos.`);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  const existing = contact.data;
  const label = existing ? "Ver contacto" : "Agregar a contactos";
  return (
    <Button
      variant="ghost"
      size="icon-sm"
      aria-label={label}
      title={label}
      onClick={() => (existing ? openContact(existing.id) : void add())}
    >
      {existing ? <UserIcon /> : <UserPlusIcon />}
    </Button>
  );
}

// Las etiquetas del correo abierto; la ✕ saca esa etiqueta de este correo solamente.
function MessageLabels({ message }: { message: MessageDetail }) {
  const queryClient = useQueryClient();
  const labels = orderedLabels(message.labels ?? [], useLabels().data ?? []);
  if (labels.length === 0) return null;
  const target = {
    id: message.id,
    accountId: message.accountId,
    folderId: message.folderId,
    seen: message.seen,
    flagged: message.flagged,
    labels: message.labels ?? [],
  };
  return (
    <div className="flex flex-wrap gap-1.5">
      {labels.map((l) => (
        <span
          key={l.id}
          className="inline-flex items-center gap-0.5 rounded border py-0.5 pr-0.5 pl-2 text-xs"
          style={labelChipStyle(l.color)}
        >
          {l.name}
          <button
            type="button"
            aria-label={`Quitar etiqueta ${l.name}`}
            className="inline-flex size-4 items-center justify-center rounded hover:bg-foreground/10"
            onClick={() => void applyLabel(queryClient, [target], l.id)}
          >
            <XIcon className="size-3" />
          </button>
        </span>
      ))}
    </div>
  );
}

function Body({
  message,
  account,
  onCompose,
  selectMessage,
  openContact,
  embedded,
}: {
  message: MessageDetail;
  // Puede faltar si la cuenta se quitó mientras el correo estaba abierto.
  account: Account | undefined;
  onCompose?: Props["onCompose"];
  selectMessage: (id: number | null) => void;
  openContact: (id: number) => void;
  embedded: boolean;
}) {
  // El estado vive acá, con key por mensaje: "Mostrar imágenes" vale solo para
  // este. Un remitente de confianza ya las trae permitidas.
  const [allowRemote, setAllowRemote] = useState(message.remoteImagesTrusted);
  const sender = message.from.email;
  const trustSender = async () => {
    try {
      await svc.TrustImageSender(sender);
      setAllowRemote(true);
      toast.success(`Los correos de ${sender} van a mostrar sus imágenes.`);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  const composer = useComposer();
  const folder = useFolders(message.accountId).data?.find((f) => f.id === message.folderId);
  const role = folder?.role;
  // Un borrador no se responde ni se reenvía (MessageActions ofrece seguir editándolo).
  const inDrafts = role === "drafts";
  const compose = onCompose ?? ((id: number, mode: ComposeMode) => void composer.openFrom(id, mode));
  const to = message.to ?? [];
  // La primera invitación de calendario adjunta, si hay.
  const invite = (message.attachments ?? []).find(isCalendar);
  const actions: { mode: ComposeMode; label: string; icon: React.ReactNode }[] = inDrafts
    ? []
    : [
        { mode: "reply", label: "Responder", icon: <ReplyIcon /> },
        { mode: "replyAll", label: "Responder a todos", icon: <ReplyAllIcon /> },
        { mode: "forward", label: "Reenviar", icon: <ForwardIcon /> },
      ];
  return (
    <div className={embedded ? "flex flex-col" : "flex h-full flex-col"}>
      <div className="flex flex-wrap gap-2 border-b px-4 py-2">
        {actions.map((a) => (
          <Button
            key={a.mode}
            variant="outline"
            size="sm"
            onClick={() => compose(message.id, a.mode)}
          >
            {a.icon}
            {a.label}
          </Button>
        ))}
        <MessageActions message={message} selectMessage={selectMessage} />
      </div>
      <div className={embedded ? undefined : "flex-1 overflow-y-auto"}>
        <div className="space-y-3 px-6 pt-5">
          {/* En un hilo el asunto va una sola vez, arriba de todo. */}
          {!embedded && (
            <h1 className="text-lg font-semibold leading-snug">{message.subject || "(sin asunto)"}</h1>
          )}
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
                {account && (
                  <>
                    <span
                      className="mr-1.5 inline-block size-2 rounded-full align-middle"
                      style={{ backgroundColor: account.color }}
                      aria-hidden
                    />
                    {account.email} ·{" "}
                  </>
                )}
                {fullDate(message.date)}
              </p>
            </div>
            {/* En un borrador o un enviado el remitente es uno mismo. */}
            {!inDrafts && role !== "sent" && (
              <div className="flex shrink-0 items-center gap-1">
                <UnsubscribeButton messageId={message.id} sender={message.from.email} />
                <SenderContactButton from={message.from} openContact={openContact} />
              </div>
            )}
          </div>
          <MessageLabels message={message} />
        </div>

        {folder && isSnoozeFolder(folder) && (
          <SnoozeBanner messageId={message.id} onReturned={() => selectMessage(null)} />
        )}

        {message.hasRemoteImages && !allowRemote && (
          <div className="mx-6 mt-4 flex items-center justify-between gap-3 rounded-md border bg-muted/50 px-3 py-2 text-xs">
            <span>Este correo tiene imágenes remotas bloqueadas para proteger tu privacidad.</span>
            <div className="flex shrink-0 gap-2">
              <Button size="sm" variant="outline" onClick={() => setAllowRemote(true)}>
                Mostrar imágenes
              </Button>
              {/* Uno mismo no se manda imágenes de seguimiento: en Enviados no hace falta. */}
              {sender && role !== "sent" && !inDrafts && (
                <Button size="sm" variant="ghost" onClick={() => void trustSender()}>
                  Siempre de este remitente
                </Button>
              )}
            </div>
          </div>
        )}

        <SafeHtmlFrame html={message.html} text={message.text} allowRemote={allowRemote} accountId={message.accountId} />
      </div>
      {invite && <InviteCard messageId={message.id} attachment={invite} />}
      <AttachmentList messageId={message.id} attachments={message.attachments ?? []} />
    </div>
  );
}

export function MessageView({
  accounts,
  state,
  onCompose,
  messageId = state.messageId,
  selectMessage = state.selectMessage,
  embedded = false,
}: Props) {
  const queryClient = useQueryClient();
  const message = useMessage(messageId);
  const { folders } = useAllFolders(accounts);

  if (messageId == null) {
    return (
      <ReaderBackdrop>
        <p className="text-sm text-muted-foreground">Elegí un correo para leerlo.</p>
      </ReaderBackdrop>
    );
  }
  if (message.isPending) return <div className={embedded ? "h-24" : "h-full"} aria-busy />;
  if (message.isError) {
    const msg = errorMessage(message.error);
    const stale = isStaleError(message.error);
    // El correo ya no está en el servidor: se vuelve a sincronizar lo que
    // muestra la lista y se cierra el correo.
    const refreshView = async () => {
      const v = state.view;
      let keys: unknown[][] | null = null;
      if (v?.kind === "folder") {
        const f = folders.find((x) => x.id === v.folderId);
        if (f && (await syncFolder(f.accountId, f.id))) keys = [["messages", f.id], ["threads", f.id]];
      } else if (v?.kind === "unified") {
        if (await syncUnified(v.role)) keys = [["messages", "unified", v.role]];
      }
      if (!keys) return;
      selectMessage(null);
      keys.forEach((queryKey) => void queryClient.invalidateQueries({ queryKey }));
    };
    return (
      <div
        className={`flex flex-col items-center justify-center gap-3 p-6 text-center text-sm ${embedded ? "" : "h-full"}`}
      >
        <p className="text-destructive">{msg}</p>
        {stale ? (
          <Button size="sm" variant="outline" onClick={() => void refreshView()}>
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
    <Body
      key={message.data.id}
      message={message.data}
      account={accounts.find((a) => a.id === message.data.accountId)}
      onCompose={onCompose}
      selectMessage={selectMessage}
      openContact={state.openContact}
      embedded={embedded}
    />
  );
}

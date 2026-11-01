import { PaperclipIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { errorMessage } from "@/lib/errors";
import { onFilesStaged } from "@/lib/events";
import { formatSize } from "@/lib/format";
import { useAccounts } from "@/lib/queries";
import { formatRecipients, parseRecipients } from "@/lib/recipients";
import { SafeHtmlFrame } from "../reader/SafeHtmlFrame";
import type { StagedAttachment } from "../../../bindings/postacontrol/core/model/models";
import { AttachmentList } from "./AttachmentList";
import type { ComposerInit } from "./ComposerContext";
import { RecipientInput } from "./RecipientInput";
import { RichEditor } from "./RichEditor";
import { sendStarted } from "./sendNotices";

// Cuánto se ve "Enviando…" antes de cerrar: el envío sigue en segundo plano.
export const SENDING_FEEDBACK_MS = 800;
// Tope del total de adjuntos; el mismo que valida el núcleo al encolar.
const MAX_ATTACHMENTS_BYTES = 25 * 1024 * 1024;

const TITLES: Record<string, string> = {
  reply: "Responder",
  replyAll: "Responder a todos",
  forward: "Reenviar",
};

type Props = { init: ComposerInit; onClose: () => void };

export function Composer({ init, onClose }: Props) {
  const accounts = useAccounts();
  const [accountId, setAccountId] = useState(init.accountId);
  const [to, setTo] = useState(formatRecipients(init.to));
  const [cc, setCc] = useState(formatRecipients(init.cc));
  const [bcc, setBcc] = useState(formatRecipients(init.bcc));
  const [showCc, setShowCc] = useState(init.cc.length > 0 || init.bcc.length > 0);
  const [subject, setSubject] = useState(init.subject);
  const [error, setError] = useState<string | null>(null);
  const [sending, setSending] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [staged, setStaged] = useState<StagedAttachment[]>(init.attachments);
  const [skipped, setSkipped] = useState<number[]>(init.skipOriginalAttachments);
  const bodyRef = useRef<() => string>(() => init.bodyHtml);

  const addStaged = (added: StagedAttachment[]) => {
    if (added.length === 0) return;
    setStaged((cur) => [...cur, ...added]);
    setDirty(true);
  };

  // Los archivos soltados sobre el redactor llegan por evento mientras está abierto.
  useEffect(
    () =>
      onFilesStaged(({ attachments, error }) => {
        addStaged(attachments ?? []);
        if (error) setError(error);
      }),
    [],
  );

  const pickAttachments = () => {
    void svc
      .PickAttachments()
      .then((picked) => addStaged(picked ?? []))
      .catch((e) => setError(errorMessage(e)));
  };

  const keptOriginals = init.originalAttachments.filter((a) => !skipped.includes(a.index));
  const attachmentItems = [
    ...keptOriginals.map((a) => ({ key: `o${a.index}`, filename: a.filename, size: a.size })),
    ...staged.map((a) => ({ key: `s${a.id}`, filename: a.filename, size: a.size })),
  ];
  const attachmentsTotal = attachmentItems.reduce((n, a) => n + a.size, 0);
  const tooBig = attachmentsTotal > MAX_ATTACHMENTS_BYTES;

  const removeAttachment = (key: string) => {
    setDirty(true);
    const n = Number(key.slice(1));
    if (key[0] === "o") setSkipped((cur) => [...cur, n]);
    else setStaged((cur) => cur.filter((a) => a.id !== n));
  };

  const parsedTo = parseRecipients(to);
  const parsedCc = parseRecipients(cc);
  const parsedBcc = parseRecipients(bcc);
  const invalid = [...parsedTo.invalid, ...parsedCc.invalid, ...parsedBcc.invalid];
  const recipientCount = parsedTo.valid.length + parsedCc.valid.length + parsedBcc.valid.length;
  const canSend = recipientCount > 0 && invalid.length === 0 && !sending && !tooBig;

  const edit = <T,>(set: (v: T) => void) => (v: T) => {
    setDirty(true);
    set(v);
  };

  // El correo entra a la bandeja de salida y se envía en segundo plano; el
  // resultado lo avisa useSendNotices. Solo los errores de validación dejan
  // el redactor abierto.
  const send = async () => {
    setSending(true);
    setError(null);
    const shownAt = Date.now();
    try {
      const id = await svc.QueueSend({
        accountId,
        to: parsedTo.valid,
        cc: parsedCc.valid,
        bcc: parsedBcc.valid,
        subject,
        bodyHtml: bodyRef.current(),
        reply: init.reply && init.reply.mode !== "new" ? init.reply : null,
        draftId: init.draftId,
        attachments: staged.map((a) => a.id),
        skipOriginalAttachments: skipped,
        signatureId: 0,
        composeDraftId: 0,
        draftMessageId: "",
      });
      sendStarted(id);
    } catch (e) {
      setError(errorMessage(e));
      setSending(false);
      return;
    }
    const left = SENDING_FEEDBACK_MS - (Date.now() - shownAt);
    if (left > 0) await new Promise((r) => setTimeout(r, left));
    onClose();
  };

  const closeSlashRef = useRef<(() => void) | null>(null);

  const requestClose = () => {
    if (sending) return;
    if (dirty) setConfirmDiscard(true);
    else onClose();
  };

  const mode = init.reply?.mode ?? "new";
  const accountList = accounts.data ?? [];

  return (
    <>
      <Dialog open onOpenChange={(o) => !o && requestClose()}>
        <DialogContent
          id="compose-drop"
          data-file-drop-target
          className="group/compose flex h-[80vh] max-w-3xl flex-col gap-4 sm:max-w-3xl"
          // Radix escucha Esc antes que el redactor: si el menú "/" está abierto, ese Esc es suyo.
          onEscapeKeyDown={(e) => {
            if (closeSlashRef.current) {
              e.preventDefault();
              closeSlashRef.current();
            }
          }}
        >
          <div
            aria-hidden
            className="pointer-events-none absolute inset-0 z-10 hidden items-center justify-center rounded-lg border-2 border-dashed border-primary bg-background/90 text-sm font-medium group-[.file-drop-target-active]/compose:flex"
          >
            Soltá los archivos para adjuntarlos
          </div>
          <DialogHeader>
            <DialogTitle>{TITLES[mode] ?? "Nuevo correo"}</DialogTitle>
            <DialogDescription className="sr-only">Redactá y enviá un correo.</DialogDescription>
          </DialogHeader>

          <div className="flex-1 space-y-4 overflow-y-auto pr-1">
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <Label htmlFor="compose-from" className="w-14 shrink-0 text-muted-foreground">
                  De
                </Label>
                <Select
                  value={String(accountId)}
                  onValueChange={(v) => edit(setAccountId)(Number(v))}
                >
                  <SelectTrigger id="compose-from" className="w-full">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {accountList.map((a) => (
                      <SelectItem key={a.id} value={String(a.id)}>
                        {a.displayName ? `${a.displayName} <${a.email}>` : a.email}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="flex items-start gap-2">
                <div className="flex-1">
                  <RecipientInput id="compose-to" label="Para" value={to} onChange={edit(setTo)} />
                </div>
                {!showCc && (
                  <Button type="button" variant="ghost" size="sm" onClick={() => setShowCc(true)}>
                    CC/CCO
                  </Button>
                )}
              </div>
              {showCc && (
                <>
                  <RecipientInput id="compose-cc" label="CC" value={cc} onChange={edit(setCc)} />
                  <RecipientInput id="compose-bcc" label="CCO" value={bcc} onChange={edit(setBcc)} />
                </>
              )}
              <div className="flex items-center gap-2">
                <Label htmlFor="compose-subject" className="w-14 shrink-0 text-muted-foreground">
                  Asunto
                </Label>
                <Input
                  id="compose-subject"
                  value={subject}
                  onChange={(e) => edit(setSubject)(e.target.value)}
                />
              </div>
              <AttachmentList items={attachmentItems} onRemove={removeAttachment} />
              {invalid.length > 0 && (
                <p className="text-xs text-destructive">Revisá estas direcciones: {invalid.join(", ")}</p>
              )}
            </div>

            <RichEditor
              content={init.bodyHtml}
              onError={setError}
              onPickImage={(editor) => {
                void svc
                  .PickImage()
                  .then(({ dataUrl, filename }) => {
                    if (dataUrl) editor.chain().focus().setImage({ src: dataUrl, alt: filename }).run();
                  })
                  .catch((e) => setError(errorMessage(e)));
              }}
              onSlashMenuChange={(close) => {
                closeSlashRef.current = close;
              }}
              onChange={(getHTML) => {
                bodyRef.current = getHTML;
                setDirty(true);
              }}
            />

            {init.quotedHtml && (
              <section aria-label="Mensaje original">
                <p className="text-xs font-medium text-muted-foreground">Mensaje original</p>
                <SafeHtmlFrame html={init.quotedHtml} text="" allowRemote={false} bare />
              </section>
            )}
          </div>

          {tooBig && (
            <p className="text-xs text-destructive">
              Los adjuntos suman {formatSize(attachmentsTotal)}; el máximo es {formatSize(MAX_ATTACHMENTS_BYTES)}.
            </p>
          )}
          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <DialogFooter className="sm:justify-between">
            <Button type="button" variant="ghost" onClick={pickAttachments} disabled={sending}>
              <PaperclipIcon /> Adjuntar
            </Button>
            <div className="flex flex-col-reverse gap-2 sm:flex-row">
              <Button type="button" variant="ghost" onClick={requestClose} disabled={sending}>
                Cancelar
              </Button>
              <Button type="button" onClick={() => void send()} disabled={!canSend}>
                {sending ? "Enviando…" : "Enviar"}
              </Button>
            </div>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={confirmDiscard} onOpenChange={setConfirmDiscard}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Descartar este correo?</DialogTitle>
            <DialogDescription>Se pierde lo que escribiste.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setConfirmDiscard(false)}>
              Seguir editando
            </Button>
            <Button variant="destructive" onClick={onClose}>
              Descartar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

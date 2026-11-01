import { PaperclipIcon, Trash2Icon } from "lucide-react";
import { useEffect, useEffectEvent, useRef, useState } from "react";
import { toast } from "sonner";
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
import { errorMessage, isNotFoundError } from "@/lib/errors";
import { onFilesStaged, type FilesStaged } from "@/lib/events";
import { formatSize } from "@/lib/format";
import { useAccounts, useSignatures } from "@/lib/queries";
import { formatRecipients, parseRecipients } from "@/lib/recipients";
import { SafeHtmlFrame } from "../reader/SafeHtmlFrame";
import type { SendRequest, StagedAttachment } from "../../../bindings/postacontrol/core/model/models";
import { AttachmentList } from "./AttachmentList";
import type { ComposerInit } from "./ComposerContext";
import { RecipientInput } from "./RecipientInput";
import { RichEditor } from "./RichEditor";
import { SignaturePicker } from "./SignaturePicker";
import { sendStarted } from "./sendNotices";
import { useDraftAutosave, type DraftStatus } from "./useDraftAutosave";

// Cuánto se ve "Enviando…" antes de cerrar: el envío sigue en segundo plano.
export const SENDING_FEEDBACK_MS = 800;
// Tope del total de adjuntos; el mismo que valida el núcleo al encolar.
const MAX_ATTACHMENTS_BYTES = 25 * 1024 * 1024;

const TITLES: Record<string, string> = {
  reply: "Responder",
  replyAll: "Responder a todos",
  forward: "Reenviar",
};

const DRAFT_STATUS_TEXT: Record<DraftStatus, string> = {
  idle: "",
  saving: "Guardando…",
  saved: "Guardado",
  local: "Guardado solo en esta compu",
};

// Sin texto ni imágenes: lo que deja el editor vacío ("<p></p>").
function isBlankHtml(html: string): boolean {
  if (/<(img|hr|table)\b/i.test(html)) return false;
  return html.replace(/<[^>]*>/g, "").replace(/&nbsp;|\u00a0/g, " ").trim() === "";
}

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
  const [confirmDiscardDraft, setConfirmDiscardDraft] = useState(false);
  const [closing, setClosing] = useState(false);
  const [dirty, setDirty] = useState(false);
  const [staged, setStaged] = useState<StagedAttachment[]>(init.attachments);
  const [skipped, setSkipped] = useState<number[]>(init.skipOriginalAttachments);
  const [picked, setPicked] = useState<number | null>(null); // firma elegida a mano
  const signatures = useSignatures();
  const bodyRef = useRef<() => string>(() => init.bodyHtml);

  const accountList = accounts.data ?? [];
  const isNew = !init.reply && init.draftId === undefined && !init.composeDraftId;
  // Un reintento de la Bandeja de salida no autoguarda: ya está en la cola.
  const autosaveOn = init.draftId === undefined;

  // Mientras la firma no se elija a mano, sigue a la por defecto de la cuenta.
  // Responder, reenviar y reabrir conservan la que trae el pedido hasta que se
  // cambie "De". Una firma borrada con el redactor abierto pasa a "Sin firma".
  const followsAccount = isNew || accountId !== init.accountId;
  const accountDefault = accountList.find((a) => a.id === accountId)?.defaultSignatureId;
  const wanted = picked ?? (followsAccount ? (accountDefault ?? init.signatureId) : init.signatureId);
  const signatureId = signatures.data && !signatures.data.some((s) => s.id === wanted) ? 0 : wanted;

  const keptOriginals = init.originalAttachments.filter((a) => !skipped.includes(a.index));
  const attachmentItems = [
    ...keptOriginals.map((a) => ({ key: `o${a.index}`, filename: a.filename, size: a.size })),
    ...staged.map((a) => ({ key: `s${a.id}`, filename: a.filename, size: a.size })),
  ];
  const attachmentsTotal = attachmentItems.reduce((n, a) => n + a.size, 0);
  const tooBig = attachmentsTotal > MAX_ATTACHMENTS_BYTES;

  const parsedTo = parseRecipients(to);
  const parsedCc = parseRecipients(cc);
  const parsedBcc = parseRecipients(bcc);
  const invalid = [...parsedTo.invalid, ...parsedCc.invalid, ...parsedBcc.invalid];
  const recipientCount = parsedTo.valid.length + parsedCc.valid.length + parsedBcc.valid.length;
  const canSend = recipientCount > 0 && invalid.length === 0 && !sending && !tooBig;

  // El mismo pedido sirve para autoguardar y para QueueSend; el envío le suma
  // el id y el Message-ID del borrador.
  const buildRequest = (): SendRequest => ({
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
    signatureId,
    composeDraftId: 0,
    draftMessageId: "",
  });

  const isEmpty = () =>
    [to, cc, bcc, subject].every((v) => v.trim() === "") &&
    attachmentItems.length === 0 &&
    isBlankHtml(bodyRef.current());

  const autosave = useDraftAutosave({
    enabled: autosaveOn,
    initialDraftId: init.composeDraftId ?? 0,
    initialMessageId: init.draftMessageId,
    buildRequest,
    isEmpty,
  });

  // Cada cambio marca el redactor como modificado y programa el autoguardado.
  const touch = () => {
    setDirty(true);
    autosave.notifyChange();
  };

  const addStaged = (added: StagedAttachment[]) => {
    if (added.length === 0) return;
    setStaged((cur) => [...cur, ...added]);
    touch();
  };

  // Los archivos soltados sobre el redactor llegan por evento mientras está abierto.
  const onStaged = useEffectEvent(({ attachments, error }: FilesStaged) => {
    addStaged(attachments ?? []);
    if (error) setError(error);
  });
  useEffect(() => onFilesStaged((p) => onStaged(p)), []);

  const pickAttachments = () => {
    void svc
      .PickAttachments()
      .then((picked) => addStaged(picked ?? []))
      .catch((e) => setError(errorMessage(e)));
  };

  const removeAttachment = (key: string) => {
    touch();
    const n = Number(key.slice(1));
    if (key[0] === "o") setSkipped((cur) => [...cur, n]);
    else setStaged((cur) => cur.filter((a) => a.id !== n));
  };

  const edit = <T,>(set: (v: T) => void) => (v: T) => {
    touch();
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
      // Un guardado en vuelo podría llegar después del envío (y recrear el
      // borrador): se cancelan los timers y se espera antes de encolar.
      await autosave.cancel();
      const { draftId: composeDraftId, messageId: draftMessageId } = autosave.current();
      const id = await svc.QueueSend({ ...buildRequest(), composeDraftId, draftMessageId });
      sendStarted(id);
    } catch (e) {
      setError(errorMessage(e));
      setSending(false);
      // Sigue abierto: vuelve a autoguardar lo que no se guardó.
      autosave.resume();
      return;
    }
    const left = SENDING_FEEDBACK_MS - (Date.now() - shownAt);
    if (left > 0) await new Promise((r) => setTimeout(r, left));
    onClose();
  };

  const closeSlashRef = useRef<(() => void) | null>(null);

  // Con autoguardado, cerrar guarda (sin preguntar). Un redactor vacío o sin
  // tocar no crea borrador.
  const closeSaving = async () => {
    const hasDraft = autosave.current().draftId !== 0;
    if (!hasDraft && (!dirty || isEmpty())) {
      onClose();
      return;
    }
    setClosing(true);
    try {
      await autosave.flush();
    } catch (e) {
      setError(errorMessage(e));
      setClosing(false);
      return;
    }
    toast.success("Guardado en Borradores.");
    onClose();
  };

  const requestClose = () => {
    if (sending || closing) return;
    if (autosaveOn) void closeSaving();
    else if (dirty) setConfirmDiscard(true);
    else onClose();
  };

  const requestDiscard = () => {
    if (sending || closing) return;
    if (!dirty && autosave.current().draftId === 0) {
      void autosave.discard();
      onClose();
    } else {
      setConfirmDiscardDraft(true);
    }
  };

  const discardDraft = async () => {
    setConfirmDiscardDraft(false);
    setClosing(true);
    try {
      await autosave.discard();
    } catch (e) {
      // Ya no existía (se envió o se descartó en otro lado): nada que avisar.
      if (!isNotFoundError(e)) toast.error(errorMessage(e));
    }
    onClose();
  };

  const mode = init.reply?.mode ?? "new";

  const signature = (signatures.data ?? []).find((s) => s.id === signatureId);

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
              <SignaturePicker
                value={signatureId}
                signatures={signatures.data ?? []}
                onChange={(id) => {
                  edit(setPicked)(id);
                }}
              />
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
                touch();
              }}
            />

            {signature && (
              <section aria-label="Vista previa de la firma">
                <p className="text-xs font-medium text-muted-foreground">Firma</p>
                <SafeHtmlFrame html={signature.html} text="" allowRemote={false} bare />
              </section>
            )}

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
            <div className="flex items-center gap-1">
              {autosaveOn && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label="Descartar"
                  title="Descartar"
                  onClick={requestDiscard}
                  disabled={sending || closing}
                >
                  <Trash2Icon />
                </Button>
              )}
              <Button type="button" variant="ghost" onClick={pickAttachments} disabled={sending}>
                <PaperclipIcon /> Adjuntar
              </Button>
              {autosaveOn && (
                <span className="ml-2 text-xs text-muted-foreground" aria-live="polite">
                  {DRAFT_STATUS_TEXT[autosave.status]}
                </span>
              )}
            </div>
            <div className="flex flex-col-reverse gap-2 sm:flex-row">
              {!autosaveOn && (
                <Button type="button" variant="ghost" onClick={requestClose} disabled={sending}>
                  Cancelar
                </Button>
              )}
              <Button type="button" onClick={() => void send()} disabled={!canSend || closing}>
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

      <Dialog open={confirmDiscardDraft} onOpenChange={setConfirmDiscardDraft}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Descartar este borrador?</DialogTitle>
            <DialogDescription>Se borra también de la carpeta Borradores del servidor.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setConfirmDiscardDraft(false)}>
              Seguir editando
            </Button>
            <Button variant="destructive" onClick={() => void discardDraft()}>
              Descartar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

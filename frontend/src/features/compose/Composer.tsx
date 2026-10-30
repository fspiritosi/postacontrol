import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
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
import { errorMessage } from "@/lib/errors";
import { useAccounts } from "@/lib/queries";
import { formatRecipients, parseRecipients } from "@/lib/recipients";
import { SafeHtmlFrame } from "../reader/SafeHtmlFrame";
import type { ComposerInit } from "./ComposerContext";
import { RecipientInput } from "./RecipientInput";
import { RichEditor } from "./RichEditor";

const TITLES: Record<string, string> = {
  reply: "Responder",
  replyAll: "Responder a todos",
  forward: "Reenviar",
};

type Props = { init: ComposerInit; onClose: () => void };

export function Composer({ init, onClose }: Props) {
  const queryClient = useQueryClient();
  const accounts = useAccounts();
  const [accountId, setAccountId] = useState(init.accountId);
  const [to, setTo] = useState(formatRecipients(init.to));
  const [cc, setCc] = useState(formatRecipients(init.cc));
  const [bcc, setBcc] = useState(formatRecipients(init.bcc));
  const [showCc, setShowCc] = useState(init.cc.length > 0 || init.bcc.length > 0);
  const [subject, setSubject] = useState(init.subject);
  const [error, setError] = useState<string | null>(null);
  const [draftSaved, setDraftSaved] = useState(false);
  const [sending, setSending] = useState(false);
  const [confirmDiscard, setConfirmDiscard] = useState(false);
  const [dirty, setDirty] = useState(false);
  const bodyRef = useRef(init.bodyHtml);
  // Borrador local que corresponde a este correo (si hay): se borra al enviarlo.
  const draftIdRef = useRef<number | undefined>(init.draftId);

  const parsedTo = parseRecipients(to);
  const parsedCc = parseRecipients(cc);
  const parsedBcc = parseRecipients(bcc);
  const invalid = [...parsedTo.invalid, ...parsedCc.invalid, ...parsedBcc.invalid];
  const recipientCount = parsedTo.valid.length + parsedCc.valid.length + parsedBcc.valid.length;
  const canSend = recipientCount > 0 && invalid.length === 0 && !sending;

  const edit = <T,>(set: (v: T) => void) => (v: T) => {
    setDirty(true);
    set(v);
  };

  const deleteDraft = async (id: number) => {
    try {
      await svc.DeleteLocalDraft(id);
    } catch (e) {
      toast.error(errorMessage(e));
    }
    void queryClient.invalidateQueries({ queryKey: ["drafts"] });
  };

  const listDraftIds = async (): Promise<number[] | null> => {
    try {
      return ((await svc.ListLocalDrafts()) ?? []).map((d) => d.id);
    } catch {
      return null;
    }
  };

  // Tras un fallo el backend ya guardó un borrador (no guardamos otro). Es
  // "nuevo" solo lo que no estaba en la lista previa al envío. Devuelve true
  // si se detectó exactamente uno.
  const trackNewDraft = async (before: number[] | null): Promise<boolean> => {
    if (before === null) return false;
    const after = await listDraftIds();
    if (after === null) return false;
    const fresh = after.filter((id) => !before.includes(id));
    if (fresh.length !== 1) return false;
    const previous = draftIdRef.current;
    if (previous !== undefined && after.includes(previous)) await deleteDraft(previous);
    draftIdRef.current = fresh[0];
    void queryClient.invalidateQueries({ queryKey: ["drafts"] });
    return true;
  };

  const send = async () => {
    setSending(true);
    setError(null);
    setDraftSaved(false);
    const before = await listDraftIds();
    try {
      const res = await svc.Send({
        accountId,
        to: parsedTo.valid,
        cc: parsedCc.valid,
        bcc: parsedBcc.valid,
        subject,
        bodyHtml: bodyRef.current,
        reply: init.reply && init.reply.mode !== "new" ? init.reply : null,
      });
      if (draftIdRef.current !== undefined) await deleteDraft(draftIdRef.current);
      toast.success("Correo enviado");
      if (res.warning) toast.warning(res.warning);
      onClose();
    } catch (e) {
      setError(errorMessage(e));
      setDraftSaved(await trackNewDraft(before));
      setSending(false);
    }
  };

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
        <DialogContent className="flex h-[80vh] max-w-3xl flex-col gap-4 sm:max-w-3xl">
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
              {invalid.length > 0 && (
                <p className="text-xs text-destructive">Revisá estas direcciones: {invalid.join(", ")}</p>
              )}
            </div>

            <RichEditor
              content={init.bodyHtml}
              onChange={(html) => {
                bodyRef.current = html;
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

          {error && (
            <Alert variant="destructive">
              <AlertDescription>
                {error}{draftSaved && " Guardamos una copia como borrador local."}
              </AlertDescription>
            </Alert>
          )}

          <DialogFooter>
            <Button type="button" variant="ghost" onClick={requestClose} disabled={sending}>
              Cancelar
            </Button>
            <Button type="button" onClick={() => void send()} disabled={!canSend}>
              {sending ? "Enviando…" : "Enviar"}
            </Button>
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

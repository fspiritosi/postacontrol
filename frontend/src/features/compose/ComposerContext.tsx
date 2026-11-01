import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type {
  Account,
  Address,
  Attachment,
  Draft,
  LocalDraft,
  ReplyContext,
  SendRequest,
  StagedAttachment,
} from "../../../bindings/postacontrol/core/model/models";
import { errorMessage } from "@/lib/errors";
import { Composer } from "./Composer";
import { useSendNotices } from "./sendNotices";

export type ComposerInit = {
  accountId: number;
  to: Address[];
  cc: Address[];
  bcc: Address[];
  subject: string;
  bodyHtml: string;
  quotedHtml: string;
  reply: ReplyContext | null; // null: correo nuevo
  draftId?: number; // correo de la bandeja de salida que se está reintentando (no autoguarda)
  composeDraftId?: number; // borrador autoguardado que se sigue editando
  draftMessageId?: string; // su Message-ID
  attachments: StagedAttachment[];
  originalAttachments: Attachment[]; // solo al reenviar
  skipOriginalAttachments: number[];
  signatureId: number; // 0: sin firma
};

type ComposerApi = {
  // `to` precarga Para (p. ej. "Escribirle" desde un contacto).
  openNew(accountId: number, to?: Address[]): void;
  openFrom(messageId: number, mode: "reply" | "replyAll" | "forward"): Promise<void>;
  openDraft(d: LocalDraft): Promise<void>;
  // Sigue editando un borrador autoguardado (o uno de la carpeta Borradores).
  openComposeDraft(d: Draft): Promise<void>;
  // Abre un correo de la bandeja de salida por id (p. ej. desde un aviso).
  openDraftById(id: number): Promise<void>;
};

const Ctx = createContext<ComposerApi | null>(null);

export function useComposer(): ComposerApi {
  const c = useContext(Ctx);
  if (!c) throw new Error("useComposer requiere ComposerProvider");
  return c;
}

export function ComposerProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<{ key: number; init: ComposerInit } | null>(null);
  const qc = useQueryClient();
  const open = useCallback((init: ComposerInit) => setState((s) => ({ key: (s?.key ?? 0) + 1, init })), []);

  const api = useMemo<ComposerApi>(() => {
    // Arma el redactor a partir de un pedido guardado (de la bandeja de salida
    // o de un borrador).
    const fromRequest = async (req: SendRequest) => {
      const ids = req.attachments ?? [];
      // Los nombres y tamaños no viajan en el pedido: se piden por id.
      const attachments = ids.length > 0 ? ((await svc.StagedInfo(ids)) ?? []) : [];
      // Una respuesta o un reenvío reabierto sigue citando el original (y un
      // reenvío mandando los originales no salteados): se piden de nuevo para
      // mostrarlos. Si el original ya no está, no se muestra nada.
      let quotedHtml = "";
      let originalAttachments: Attachment[] = [];
      const reply = req.reply;
      if (reply && reply.mode !== "new") {
        try {
          const prepared = await svc.PrepareCompose(reply.originalMessageId, reply.mode);
          quotedHtml = prepared.quotedHtml;
          if (reply.mode === "forward") originalAttachments = prepared.originalAttachments ?? [];
        } catch {
          quotedHtml = "";
          originalAttachments = [];
        }
      }
      return {
        accountId: req.accountId,
        to: req.to ?? [],
        cc: req.cc ?? [],
        bcc: req.bcc ?? [],
        subject: req.subject,
        bodyHtml: req.bodyHtml,
        quotedHtml,
        reply: req.reply,
        attachments,
        originalAttachments,
        skipOriginalAttachments: req.skipOriginalAttachments ?? [],
        signatureId: req.signatureId,
      } satisfies ComposerInit;
    };
    const openDraft = async (d: LocalDraft) => open({ ...(await fromRequest(d.request)), draftId: d.id });
    const openComposeDraft = async (d: Draft) =>
      open({ ...(await fromRequest(d.request)), composeDraftId: d.id, draftMessageId: d.messageId });
    return {
      openNew: (accountId, to = []) =>
        open({ accountId, to, cc: [], bcc: [], subject: "", bodyHtml: "", quotedHtml: "", reply: null,
          attachments: [], originalAttachments: [], skipOriginalAttachments: [],
          signatureId: qc.getQueryData<Account[]>(["accounts"])?.find((a) => a.id === accountId)?.defaultSignatureId ?? 0 }),
      openFrom: async (messageId, mode) => {
        try {
          const d = await svc.PrepareCompose(messageId, mode);
          open({
            accountId: d.accountId,
            to: d.to ?? [],
            cc: d.cc ?? [],
            bcc: [],
            subject: d.subject,
            bodyHtml: "",
            quotedHtml: d.quotedHtml,
            reply: d.reply,
            attachments: [],
            originalAttachments: d.originalAttachments ?? [],
            skipOriginalAttachments: [],
            signatureId: d.signatureId,
          });
        } catch (e) {
          toast.error(errorMessage(e));
        }
      },
      openDraft,
      openComposeDraft,
      openDraftById: async (id) => {
        try {
          const d = ((await svc.ListLocalDrafts()) ?? []).find((x) => x.id === id);
          if (d) await openDraft(d);
          else toast.error("Ese correo ya no está en la Bandeja de salida.");
        } catch (e) {
          toast.error(errorMessage(e));
        }
      },
    };
  }, [open, qc]);
  useSendNotices(api.openDraftById);

  return (
    <Ctx.Provider value={api}>
      {children}
      {state && <Composer key={state.key} init={state.init} onClose={() => setState(null)} />}
    </Ctx.Provider>
  );
}

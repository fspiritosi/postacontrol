import { createContext, useCallback, useContext, useEffect, useMemo, type ReactNode } from "react";
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
import { Events } from "@wailsio/runtime";
import { serializeInit } from "@/lib/composeWindow";
import { sendStarted, useSendNotices } from "./sendNotices";

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
  const qc = useQueryClient();
  // Abre el redactor en su propia ventana. Con key, si ya hay una ventana con
  // esa clave se trae adelante en vez de abrir otra.
  const open = useCallback(async (init: ComposerInit, key: string) => {
    try {
      await svc.OpenComposeWindow(serializeInit(init), key);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  }, []);

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
    const openDraft = async (d: LocalDraft) => open({ ...(await fromRequest(d.request)), draftId: d.id }, `outbox:${d.id}`);
    const openComposeDraft = async (d: Draft) =>
      open({ ...(await fromRequest(d.request)), composeDraftId: d.id, draftMessageId: d.messageId }, `draft:${d.id}`);
    return {
      openNew: (accountId, to = []) =>
        void open({ accountId, to, cc: [], bcc: [], subject: "", bodyHtml: "", quotedHtml: "", reply: null,
          attachments: [], originalAttachments: [], skipOriginalAttachments: [],
          signatureId: qc.getQueryData<Account[]>(["accounts"])?.find((a) => a.id === accountId)?.defaultSignatureId ?? 0 }, ""),
      openFrom: async (messageId, mode) => {
        try {
          const d = await svc.PrepareCompose(messageId, mode);
          await open({
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
          }, "");
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
  // El envío arranca en la ventana del redactor; el aviso se muestra en todas.
  useEffect(() => Events.On("compose:send-started", (ev) => sendStarted(ev.data as number)), []);
  // Los avisos de un redactor que se cerró (su ventana ya no está para mostrarlos).
  useEffect(
    () =>
      Events.On("compose:notice", (ev) => {
        const { kind, message } = ev.data as { kind: "success" | "error"; message: string };
        if (kind === "success") toast.success(message);
        else if (kind === "error") toast.error(message);
      }),
    [],
  );

  return <Ctx.Provider value={api}>{children}</Ctx.Provider>;
}

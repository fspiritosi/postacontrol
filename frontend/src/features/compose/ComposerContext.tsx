import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type {
  Address,
  Attachment,
  LocalDraft,
  ReplyContext,
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
  draftId?: number; // correo de la bandeja de salida que se está reintentando
  attachments: StagedAttachment[];
  originalAttachments: Attachment[]; // solo al reenviar
  skipOriginalAttachments: number[];
};

type ComposerApi = {
  openNew(accountId: number): void;
  openFrom(messageId: number, mode: "reply" | "replyAll" | "forward"): Promise<void>;
  openDraft(d: LocalDraft): Promise<void>;
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
  const open = useCallback((init: ComposerInit) => setState((s) => ({ key: (s?.key ?? 0) + 1, init })), []);

  const api = useMemo<ComposerApi>(() => {
    const openDraft = async (d: LocalDraft) => {
      const ids = d.request.attachments ?? [];
      // Los nombres y tamaños no viajan en el pedido: se piden por id.
      const attachments = ids.length > 0 ? ((await svc.StagedInfo(ids)) ?? []) : [];
      // Una respuesta o un reenvío reabierto sigue citando el original (y un
      // reenvío mandando los originales no salteados): se piden de nuevo para
      // mostrarlos. Si el original ya no está, no se muestra nada.
      let quotedHtml = "";
      let originalAttachments: Attachment[] = [];
      const reply = d.request.reply;
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
      open({
        accountId: d.request.accountId,
        to: d.request.to ?? [],
        cc: d.request.cc ?? [],
        bcc: d.request.bcc ?? [],
        subject: d.request.subject,
        bodyHtml: d.request.bodyHtml,
        quotedHtml,
        reply: d.request.reply,
        draftId: d.id,
        attachments,
        originalAttachments,
        skipOriginalAttachments: d.request.skipOriginalAttachments ?? [],
      });
    };
    return {
      openNew: (accountId) =>
        open({ accountId, to: [], cc: [], bcc: [], subject: "", bodyHtml: "", quotedHtml: "", reply: null,
          attachments: [], originalAttachments: [], skipOriginalAttachments: [] }),
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
          });
        } catch (e) {
          toast.error(errorMessage(e));
        }
      },
      openDraft,
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
  }, [open]);
  useSendNotices(api.openDraftById);

  return (
    <Ctx.Provider value={api}>
      {children}
      {state && <Composer key={state.key} init={state.init} onClose={() => setState(null)} />}
    </Ctx.Provider>
  );
}

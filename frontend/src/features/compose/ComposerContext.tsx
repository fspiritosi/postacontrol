import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type {
  Address,
  LocalDraft,
  ReplyContext,
} from "../../../bindings/postacontrol/core/model/models";
import { errorMessage } from "@/lib/errors";
import { Composer } from "./Composer";

export type ComposerInit = {
  accountId: number;
  to: Address[];
  cc: Address[];
  bcc: Address[];
  subject: string;
  bodyHtml: string;
  quotedHtml: string;
  reply: ReplyContext | null; // null: correo nuevo
  draftId?: number; // borrador local de origen
};

type ComposerApi = {
  openNew(accountId: number): void;
  openFrom(messageId: number, mode: "reply" | "replyAll" | "forward"): Promise<void>;
  openDraft(d: LocalDraft): void;
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

  const api = useMemo<ComposerApi>(
    () => ({
      openNew: (accountId) =>
        open({ accountId, to: [], cc: [], bcc: [], subject: "", bodyHtml: "", quotedHtml: "", reply: null }),
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
          });
        } catch (e) {
          toast.error(errorMessage(e));
        }
      },
      openDraft: (d) =>
        open({
          accountId: d.request.accountId,
          to: d.request.to ?? [],
          cc: d.request.cc ?? [],
          bcc: d.request.bcc ?? [],
          subject: d.request.subject,
          bodyHtml: d.request.bodyHtml,
          quotedHtml: "",
          reply: d.request.reply,
          draftId: d.id,
        }),
    }),
    [open],
  );

  return (
    <Ctx.Provider value={api}>
      {children}
      {state && <Composer key={state.key} init={state.init} onClose={() => setState(null)} />}
    </Ctx.Provider>
  );
}

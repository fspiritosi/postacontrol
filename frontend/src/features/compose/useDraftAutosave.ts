import { useEffect, useRef, useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Draft, SendRequest } from "../../../bindings/postacontrol/core/model/models";
import { isNotFoundError } from "@/lib/errors";

// Cuánto después del último cambio se guarda en la compu y se sube al servidor.
export const DRAFT_LOCAL_DEBOUNCE_MS = 1500;
export const DRAFT_UPLOAD_DEBOUNCE_MS = 30000;

// "local": la última subida no llegó al servidor (queda pendiente en la compu).
export type DraftStatus = "idle" | "saving" | "saved" | "local";

type Options = {
  enabled: boolean;
  initialDraftId: number; // 0: todavía no hay borrador
  initialMessageId?: string;
  buildRequest: () => SendRequest;
  isEmpty: () => boolean;
};

export type DraftAutosave = {
  draftId: number;
  status: DraftStatus;
  // Programa el guardado local y la subida (reinicia los dos timers).
  notifyChange(): void;
  // Guarda ya lo que falte; resuelve cuando quedó guardado en la compu y deja
  // la subida en marcha detrás. Rechaza si no se pudo guardar.
  flush(): Promise<void>;
  // Cancela los timers y espera la llamada en vuelo. Lo usa el envío antes de
  // QueueSend: después ya no se guarda nada (ni al desmontar) hasta resume().
  cancel(): Promise<void>;
  // Vuelve a autoguardar después de un cancel() (el envío no se pudo encolar)
  // y reprograma lo que haya quedado sin guardar o sin subir.
  resume(): void;
  // Cancela y borra el borrador (también del servidor). Puede rechazar con el
  // aviso de que no se pudo borrar del servidor (lo local ya se borró).
  discard(): Promise<void>;
  // El id y el Message-ID actuales, para leerlos después de un await. El id
  // es 0 mientras no haya un borrador guardado (todavía, o porque se borró en
  // otro lado y no hubo cambios después).
  current(): { draftId: number; messageId: string };
};

type Timer = ReturnType<typeof setTimeout>;

type State = {
  id: number;
  messageId: string;
  unsaved: boolean; // hay cambios sin guardar en la compu
  unuploaded: boolean; // hay una versión guardada que no se intentó subir
  local: boolean; // la última subida dejó needsUpload en true
  stopped: boolean; // cancelado para enviar: no se guarda nada hasta resume()
  dead: boolean; // descartado: no se guarda nada más
  chain: Promise<void>;
  localTimer?: Timer;
  uploadTimer?: Timer;
};

const noop = () => {};

// Autoguardado del redactor. Las llamadas al núcleo se encadenan: nunca hay
// dos a la vez, así una subida no se cruza con un guardado ni con el envío.
export function useDraftAutosave(opts: Options): DraftAutosave {
  const [draftId, setDraftId] = useState(opts.initialDraftId);
  const [status, setStatus] = useState<DraftStatus>("idle");
  const optsRef = useRef(opts);
  useEffect(() => {
    optsRef.current = opts;
  });

  const [api] = useState(() => {
    const st: State = {
      id: opts.initialDraftId,
      messageId: opts.initialMessageId ?? "",
      unsaved: false,
      unuploaded: false,
      local: false,
      stopped: false,
      dead: false,
      chain: Promise.resolve(),
    };

    const run = (op: () => Promise<void>): Promise<void> => {
      const p = st.chain.then(op);
      st.chain = p.catch(noop);
      return p;
    };

    const clearTimers = () => {
      clearTimeout(st.localTimer);
      clearTimeout(st.uploadTimer);
      st.localTimer = st.uploadTimer = undefined;
    };

    const settledStatus = (): DraftStatus => (st.id === 0 ? "idle" : st.local ? "local" : "saved");

    // El borrador ya no existe (por ejemplo, se borró de Borradores con el
    // redactor abierto): el próximo guardado crea uno nuevo, con otro
    // Message-ID, para no perder lo escrito.
    const forget = () => {
      st.id = 0;
      st.messageId = "";
      st.unuploaded = false;
      st.local = false;
      setDraftId(0);
    };

    const saveDraft = async (): Promise<Draft | null> => {
      try {
        return await svc.SaveDraft(st.id, optsRef.current.buildRequest());
      } catch (e) {
        if (st.id === 0 || !isNotFoundError(e)) throw e;
        forget();
        if (optsRef.current.isEmpty()) return null;
        return svc.SaveDraft(0, optsRef.current.buildRequest());
      }
    };

    const save = async () => {
      if (st.dead || !st.unsaved) return;
      st.unsaved = false;
      // Un redactor vacío nunca crea borrador.
      if (st.id === 0 && optsRef.current.isEmpty()) return;
      setStatus("saving");
      try {
        const d = await saveDraft();
        if (!d) {
          setStatus(settledStatus());
          return;
        }
        st.id = d.id;
        st.messageId = d.messageId;
        st.unuploaded = true;
        setDraftId(d.id);
        setStatus(settledStatus());
      } catch (e) {
        st.unsaved = true;
        setStatus(settledStatus());
        throw e;
      }
    };

    const upload = async () => {
      if (st.dead || st.id === 0 || !st.unuploaded) return;
      st.unuploaded = false;
      setStatus("saving");
      try {
        const d = await svc.UploadDraft(st.id);
        // Si no se pudo, el núcleo lo reintenta en su pasada de fondo.
        st.local = d.needsUpload;
        setStatus(settledStatus());
      } catch (e) {
        if (isNotFoundError(e)) {
          // Lo guardado ya no está; lo que se cambie después lo vuelve a crear.
          forget();
          setStatus(settledStatus());
          return;
        }
        st.unuploaded = true;
        st.local = true;
        setStatus(settledStatus());
        throw e;
      }
    };

    const flushNow = async () => {
      clearTimers();
      await run(save);
      void run(upload).catch(noop);
    };

    const cancel = async () => {
      clearTimers();
      st.stopped = true;
      await st.chain;
    };

    const schedule = () => {
      clearTimers();
      st.localTimer = setTimeout(() => {
        st.localTimer = undefined;
        void run(save).catch(noop);
      }, DRAFT_LOCAL_DEBOUNCE_MS);
      st.uploadTimer = setTimeout(() => {
        st.uploadTimer = undefined;
        void run(async () => {
          await save();
          await upload();
        }).catch(noop);
      }, DRAFT_UPLOAD_DEBOUNCE_MS);
    };

    return {
      notifyChange() {
        if (!optsRef.current.enabled || st.dead) return;
        st.unsaved = true;
        // Mientras se envía no se programa nada: si el envío falla, resume().
        if (!st.stopped) schedule();
      },
      resume() {
        st.stopped = false;
        if (optsRef.current.enabled && !st.dead && (st.unsaved || st.unuploaded)) schedule();
      },
      flush: async () => {
        if (!optsRef.current.enabled) return;
        await flushNow();
      },
      cancel,
      discard: async () => {
        await cancel();
        st.dead = true;
        if (st.id !== 0) await svc.DeleteDraft(st.id);
      },
      current: () => ({ draftId: st.id, messageId: st.messageId }),
      // Al desmontar (otro redactor lo reemplazó) se guarda lo pendiente.
      unmount() {
        const pending = st.unsaved || st.unuploaded;
        clearTimers();
        if (pending && !st.stopped && !st.dead && optsRef.current.enabled) {
          void flushNow().catch(noop);
        }
      },
    };
  });

  useEffect(() => api.unmount, [api]);

  return {
    draftId,
    status,
    notifyChange: api.notifyChange,
    flush: api.flush,
    cancel: api.cancel,
    resume: api.resume,
    discard: api.discard,
    current: api.current,
  };
}

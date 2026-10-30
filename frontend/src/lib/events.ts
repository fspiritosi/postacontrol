import type { QueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { useEffect, useSyncExternalStore } from "react";
import { toast } from "sonner";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "./errors";

export type SyncProgress = { folderId: number; done: number; total: number };

// Cada cuánto, como mucho, se recarga la lista mientras llegan lotes.
export const PROGRESS_REFRESH_MS = 500;

let progress: SyncProgress | null = null;
// Sincronizaciones pedidas desde la UI que todavía no terminaron, por carpeta.
const pending = new Map<number, number>();
const listeners = new Set<() => void>();

function notify() {
  listeners.forEach((l) => l());
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  return () => {
    listeners.delete(cb);
  };
}

function setProgress(p: SyncProgress | null) {
  progress = p;
  notify();
}

export function useSyncProgress(): SyncProgress | null {
  return useSyncExternalStore(subscribe, () => progress);
}

// Dice si la carpeta se está sincronizando: hay lotes en curso o una
// sincronización pedida desde la UI que todavía no volvió.
export function useFolderSyncing(folderId: number | null): boolean {
  return useSyncExternalStore(
    subscribe,
    () =>
      folderId != null && ((pending.get(folderId) ?? 0) > 0 || progress?.folderId === folderId),
  );
}

export function syncErrorToastId(accountId: number): string {
  return `sync-error-${accountId}`;
}

// Un solo aviso por cuenta: el evento mail:sync-error y el rechazo de la
// llamada traen el mismo mensaje y comparten id, así que se reemplazan.
export function toastSyncError(accountId: number, e: unknown): void {
  toast.error(errorMessage(e), { id: syncErrorToastId(accountId) });
}

// syncFolder pide sincronizar una carpeta, la marca como "sincronizando"
// mientras dura y avisa del error. Devuelve si salió bien.
export async function syncFolder(accountId: number, folderId: number): Promise<boolean> {
  pending.set(folderId, (pending.get(folderId) ?? 0) + 1);
  notify();
  try {
    await svc.SyncFolder(folderId);
    return true;
  } catch (e) {
    toastSyncError(accountId, e);
    return false;
  } finally {
    const n = (pending.get(folderId) ?? 1) - 1;
    if (n > 0) pending.set(folderId, n);
    else pending.delete(folderId);
    // Si la pasada se cortó sin avisar (p. ej. "stale"), la barra no queda congelada.
    if (n <= 0 && progress?.folderId === folderId) progress = null;
    notify();
  }
}

// Solo para tests: vuelve el estado del módulo a cero.
export function resetSyncStateForTests(): void {
  progress = null;
  pending.clear();
  notify();
}

// El payload emitido por Go llega en `ev.data`.
export function useMailEvents(queryClient: QueryClient): void {
  useEffect(() => {
    // Recarga la lista de la carpeta como mucho cada PROGRESS_REFRESH_MS
    // mientras llegan lotes, con una última recarga al final del intervalo.
    const lastRefresh = new Map<number, number>();
    const trailing = new Map<number, ReturnType<typeof setTimeout>>();
    const refreshMessages = (folderId: number) => {
      lastRefresh.set(folderId, Date.now());
      void queryClient.invalidateQueries({ queryKey: ["messages", folderId] });
    };
    const refreshThrottled = (folderId: number) => {
      const wait = PROGRESS_REFRESH_MS - (Date.now() - (lastRefresh.get(folderId) ?? 0));
      if (wait <= 0) {
        refreshMessages(folderId);
      } else if (!trailing.has(folderId)) {
        trailing.set(
          folderId,
          setTimeout(() => {
            trailing.delete(folderId);
            refreshMessages(folderId);
          }, wait),
        );
      }
    };

    const offs = [
      Events.On("mail:accounts-changed", () => {
        void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      }),
      Events.On("mail:folders-updated", (ev) => {
        const { accountId } = ev.data as { accountId: number };
        void queryClient.invalidateQueries({ queryKey: ["folders", accountId] });
      }),
      Events.On("mail:folder-updated", (ev) => {
        const { folderId } = ev.data as { folderId: number };
        if (progress?.folderId === folderId) setProgress(null);
        void queryClient.invalidateQueries({ queryKey: ["messages", folderId] });
        void queryClient.invalidateQueries({ queryKey: ["folders"] });
      }),
      Events.On("mail:sync-progress", (ev) => {
        const p = ev.data as SyncProgress;
        setProgress(p.done >= p.total ? null : p);
        refreshThrottled(p.folderId);
      }),
      Events.On("mail:sync-error", (ev) => {
        const { accountId, message } = ev.data as { accountId: number; message: string };
        setProgress(null);
        toastSyncError(accountId, message);
      }),
    ];
    return () => {
      offs.forEach((off) => off());
      trailing.forEach((t) => clearTimeout(t));
    };
  }, [queryClient]);
}

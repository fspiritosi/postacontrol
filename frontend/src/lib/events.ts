import type { QueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { useEffect, useSyncExternalStore } from "react";
import { toast } from "sonner";

export type SyncProgress = { folderId: number; done: number; total: number };

let progress: SyncProgress | null = null;
const listeners = new Set<() => void>();

function setProgress(p: SyncProgress | null) {
  progress = p;
  listeners.forEach((l) => l());
}

export function useSyncProgress(): SyncProgress | null {
  return useSyncExternalStore(
    (cb) => {
      listeners.add(cb);
      return () => listeners.delete(cb);
    },
    () => progress,
  );
}

// El payload emitido por Go llega en `ev.data`.
export function useMailEvents(queryClient: QueryClient): void {
  useEffect(() => {
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
        void queryClient.invalidateQueries({ queryKey: ["messages", folderId] });
        void queryClient.invalidateQueries({ queryKey: ["folders"] });
      }),
      Events.On("mail:sync-progress", (ev) => {
        const p = ev.data as SyncProgress;
        setProgress(p.done >= p.total ? null : p);
      }),
      Events.On("mail:sync-error", (ev) => {
        toast.error((ev.data as { message: string }).message);
      }),
    ];
    return () => offs.forEach((off) => off());
  }, [queryClient]);
}

import type { QueryClient } from "@tanstack/react-query";
import { Events } from "@wailsio/runtime";
import { useEffect, useSyncExternalStore } from "react";
import { toast } from "sonner";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import type { StagedAttachment, UnifiedView } from "../../bindings/postacontrol/core/model/models";
import { errorMessage } from "./errors";
import type { UnifiedRole } from "./view";

export type SyncProgress = { folderId: number; done: number; total: number };

// Cada cuánto, como mucho, se recarga la lista mientras llegan lotes.
export const PROGRESS_REFRESH_MS = 500;
// Cada cuánto, como mucho, se rehace la búsqueda abierta mientras se actualizan carpetas.
export const SEARCH_REFRESH_MS = 2000;

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

const REJECTED_CREDENTIALS = "El servidor rechazó el usuario o la contraseña.";

// Emisor simple de módulo: el toast vive fuera del árbol de la barra lateral,
// que es quien abre el diálogo de contraseña.
const passwordListeners = new Set<(accountId: number) => void>();

export function onPasswordRequest(cb: (accountId: number) => void): () => void {
  passwordListeners.add(cb);
  return () => {
    passwordListeners.delete(cb);
  };
}

export function requestPassword(accountId: number): void {
  passwordListeners.forEach((l) => l(accountId));
}

// Pedido de abrir un correo (o solo su carpeta, con messageId 0) que llega al
// tocar una notificación. Lo escucha MailShell, que es quien tiene la selección.
export type OpenMessageRequest = { folderId: number; messageId: number };
const openListeners = new Set<(r: OpenMessageRequest) => void>();

export function onOpenMessage(cb: (r: OpenMessageRequest) => void): () => void {
  openListeners.add(cb);
  return () => {
    openListeners.delete(cb);
  };
}

export function requestOpenMessage(r: OpenMessageRequest): void {
  openListeners.forEach((l) => l(r));
}

// Archivos soltados sobre el redactor (ya preparados por Go) o el motivo por
// el que no se pudieron preparar. Lo escucha el redactor mientras está abierto.
export type FilesStaged = { attachments: StagedAttachment[]; error: string };

export function onFilesStaged(cb: (p: FilesStaged) => void): () => void {
  return Events.On("files-staged", (ev) => cb(ev.data as FilesStaged));
}

export function syncErrorToastId(accountId: number): string {
  return `sync-error-${accountId}`;
}

// Un solo aviso por cuenta: el evento mail:sync-error y el rechazo de la
// llamada traen el mismo mensaje y comparten id, así que se reemplazan.
export function toastSyncError(accountId: number, e: unknown): void {
  const message = errorMessage(e);
  toast.error(message, {
    id: syncErrorToastId(accountId),
    ...(message === REJECTED_CREDENTIALS && {
      action: { label: "Cambiar contraseña", onClick: () => requestPassword(accountId) },
    }),
  });
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

// syncUnified pide sincronizar las carpetas de un rol de todas las cuentas.
// Los errores de cada cuenta llegan por mail:sync-error; acá solo se avisa si
// falla la llamada entera.
export async function syncUnified(role: UnifiedRole): Promise<boolean> {
  try {
    await svc.SyncUnified(role as UnifiedView);
    return true;
  } catch (e) {
    toast.error(errorMessage(e), { id: "sync-error-unified" });
    return false;
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
      // Los hilos no: agruparlos es más caro y se recargan con mail:folder-updated.
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

    // La búsqueda abierta (y su "falta indexar") se rehace como mucho cada
    // SEARCH_REFRESH_MS, con una última al final del intervalo.
    let lastSearch = 0;
    let searchTrailing: ReturnType<typeof setTimeout> | null = null;
    const refreshSearch = () => {
      lastSearch = Date.now();
      void queryClient.invalidateQueries({ queryKey: ["search"] });
    };
    const refreshSearchThrottled = () => {
      const wait = SEARCH_REFRESH_MS - (Date.now() - lastSearch);
      if (wait <= 0) {
        refreshSearch();
      } else if (searchTrailing == null) {
        searchTrailing = setTimeout(() => {
          searchTrailing = null;
          refreshSearch();
        }, wait);
      }
    };

    const offs = [
      Events.On("mail:accounts-changed", () => {
        void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      }),
      Events.On("mail:signatures-changed", () => {
        void queryClient.invalidateQueries({ queryKey: ["signatures"] });
      }),
      Events.On("mail:contacts-changed", () => {
        for (const key of ["contacts", "contact", "contact-for", "suggest"]) {
          void queryClient.invalidateQueries({ queryKey: [key] });
        }
      }),
      Events.On("mail:folders-updated", (ev) => {
        const { accountId } = ev.data as { accountId: number };
        void queryClient.invalidateQueries({ queryKey: ["folders", accountId] });
      }),
      Events.On("mail:folder-updated", (ev) => {
        const { folderId } = ev.data as { folderId: number };
        if (progress?.folderId === folderId) setProgress(null);
        void queryClient.invalidateQueries({ queryKey: ["messages", folderId] });
        // La carpeta puede formar parte de una vista unificada.
        void queryClient.invalidateQueries({ queryKey: ["messages", "unified"] });
        // Los hilos de una carpeta suman los de Enviados: se recargan todos.
        void queryClient.invalidateQueries({ queryKey: ["threads"] });
        void queryClient.invalidateQueries({ queryKey: ["thread"] });
        void queryClient.invalidateQueries({ queryKey: ["folders"] });
        refreshSearchThrottled();
      }),
      Events.On("mail:settings-changed", () => {
        void queryClient.invalidateQueries({ queryKey: ["settings"] });
      }),
      Events.On("mail:sync-progress", (ev) => {
        const p = ev.data as SyncProgress;
        setProgress(p.done >= p.total ? null : p);
        refreshThrottled(p.folderId);
      }),
      // Se registra acá (y no en MailShell) porque este hook se monta una sola
      // vez en App; así hay un único Events.On aunque MailShell se remonte.
      Events.On("app:open-message", (ev) => {
        requestOpenMessage(ev.data as OpenMessageRequest);
      }),
      Events.On("mail:outbox-updated", () => {
        void queryClient.invalidateQueries({ queryKey: ["drafts"] });
      }),
      Events.On("mail:drafts-changed", () => {
        void queryClient.invalidateQueries({ queryKey: ["pending-drafts"] });
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
      if (searchTrailing != null) clearTimeout(searchTrailing);
    };
  }, [queryClient]);
}

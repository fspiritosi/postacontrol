import { QueryClient } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const handlers = new Map<string, (ev: { data: unknown }) => void>();
vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, cb: (ev: { data: unknown }) => void) => {
      handlers.set(name, cb);
      return () => handlers.delete(name);
    },
  },
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));
vi.mock("../../bindings/postacontrol/app/mailservice", () => ({ SyncFolder: vi.fn() }));

import { toast } from "sonner";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import {
  PROGRESS_REFRESH_MS,
  resetSyncStateForTests,
  syncFolder,
  useFolderSyncing,
  useMailEvents,
  useSyncProgress,
} from "./events";

function emit(name: string, data: unknown) {
  act(() => handlers.get(name)!({ data }));
}

describe("useMailEvents", () => {
  let qc: QueryClient;
  let invalidate: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.useFakeTimers();
    resetSyncStateForTests();
    qc = new QueryClient();
    invalidate = vi.spyOn(qc, "invalidateQueries").mockResolvedValue();
    renderHook(() => useMailEvents(qc));
  });
  afterEach(() => {
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  const messageRefreshes = () =>
    invalidate.mock.calls.filter(
      ([f]: [{ queryKey: unknown[] }]) => f.queryKey[0] === "messages" && f.queryKey[1] === 3,
    ).length;

  it("recarga la lista mientras llegan lotes, como mucho cada 500 ms", () => {
    emit("mail:sync-progress", { folderId: 3, done: 200, total: 1000 });
    expect(messageRefreshes()).toBe(1);
    emit("mail:sync-progress", { folderId: 3, done: 400, total: 1000 });
    emit("mail:sync-progress", { folderId: 3, done: 600, total: 1000 });
    expect(messageRefreshes()).toBe(1);
    act(() => vi.advanceTimersByTime(PROGRESS_REFRESH_MS));
    expect(messageRefreshes()).toBe(2);
  });

  it("limpia la barra de progreso si la sincronización falla o termina", () => {
    const { result } = renderHook(() => useSyncProgress());
    emit("mail:sync-progress", { folderId: 3, done: 200, total: 1000 });
    expect(result.current).toEqual({ folderId: 3, done: 200, total: 1000 });
    emit("mail:sync-error", { accountId: 1, message: "No pudimos conectarnos al servidor." });
    expect(result.current).toBeNull();

    emit("mail:sync-progress", { folderId: 3, done: 200, total: 1000 });
    emit("mail:folder-updated", { folderId: 4 });
    expect(result.current).not.toBeNull();
    emit("mail:folder-updated", { folderId: 3 });
    expect(result.current).toBeNull();
  });

  it("el aviso de error de sincronización usa un id por cuenta", () => {
    emit("mail:sync-error", { accountId: 1, message: "Sin red" });
    emit("mail:sync-error", { accountId: 1, message: "Sin red" });
    expect(toast.error).toHaveBeenCalledTimes(2);
    for (const call of vi.mocked(toast.error).mock.calls) {
      expect(call).toEqual(["Sin red", { id: "sync-error-1" }]);
    }
  });
});

describe("syncFolder", () => {
  beforeEach(() => resetSyncStateForTests());
  afterEach(() => vi.clearAllMocks());

  it("marca la carpeta como sincronizando mientras dura la llamada", async () => {
    let resolve!: () => void;
    vi.mocked(svc.SyncFolder).mockReturnValue(
      new Promise<void>((r) => (resolve = r)) as ReturnType<typeof svc.SyncFolder>,
    );
    const { result } = renderHook(() => useFolderSyncing(3));
    expect(result.current).toBe(false);
    let done!: Promise<boolean>;
    act(() => {
      done = syncFolder(1, 3);
    });
    expect(result.current).toBe(true);
    await act(async () => {
      resolve();
      await done;
    });
    expect(result.current).toBe(false);
    await expect(done).resolves.toBe(true);
  });

  it("un error comparte el id del aviso de mail:sync-error", async () => {
    vi.mocked(svc.SyncFolder).mockRejectedValue(new Error("No pudimos conectarnos al servidor."));
    await expect(syncFolder(1, 3)).resolves.toBe(false);
    expect(toast.error).toHaveBeenCalledWith("No pudimos conectarnos al servidor.", {
      id: "sync-error-1",
    });
  });
});

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  SaveDraft: vi.fn(),
  UploadDraft: vi.fn(),
  DeleteDraft: vi.fn(),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Draft, SendRequest } from "../../../bindings/postacontrol/core/model/models";
import { DRAFT_LOCAL_DEBOUNCE_MS, DRAFT_UPLOAD_DEBOUNCE_MS, useDraftAutosave } from "./useDraftAutosave";

const req = { accountId: 1, subject: "Hola" } as SendRequest;

function draft(over: Partial<Draft> = {}): Draft {
  return { id: 9, request: req, messageId: "<b@x>", needsUpload: true, uploadError: "", updatedAt: "", ...over };
}

function setup(opts: { empty?: boolean; initialDraftId?: number } = {}) {
  const buildRequest = vi.fn(() => req);
  return renderHook(() =>
    useDraftAutosave({
      enabled: true,
      initialDraftId: opts.initialDraftId ?? 0,
      buildRequest,
      isEmpty: () => opts.empty ?? false,
    }),
  );
}

const notFound = () => new Error("No encontramos lo que buscabas.");

const advance = (ms: number) => act(() => vi.advanceTimersByTimeAsync(ms));

describe("useDraftAutosave", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    vi.mocked(svc.SaveDraft).mockResolvedValue(draft());
    vi.mocked(svc.UploadDraft).mockResolvedValue(draft({ needsUpload: false }));
    vi.mocked(svc.DeleteDraft).mockResolvedValue();
  });
  afterEach(() => vi.useRealTimers());

  it("guarda local a los 1500 ms y sube a los 30000 ms", async () => {
    expect(DRAFT_LOCAL_DEBOUNCE_MS).toBe(1500);
    expect(DRAFT_UPLOAD_DEBOUNCE_MS).toBe(30000);
    const { result } = setup();
    act(() => result.current.notifyChange());
    await advance(1499);
    expect(svc.SaveDraft).not.toHaveBeenCalled();
    await advance(1);
    expect(svc.SaveDraft).toHaveBeenCalledWith(0, req);
    expect(result.current.draftId).toBe(9);
    expect(result.current.status).toBe("saved");
    expect(svc.UploadDraft).not.toHaveBeenCalled();
    await advance(30000 - 1500);
    expect(svc.UploadDraft).toHaveBeenCalledWith(9);
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    expect(result.current.status).toBe("saved");
  });

  it("cambios seguidos reinician los timers", async () => {
    const { result } = setup();
    act(() => result.current.notifyChange());
    await advance(1000);
    act(() => result.current.notifyChange());
    await advance(1000);
    expect(svc.SaveDraft).not.toHaveBeenCalled();
    await advance(500);
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    act(() => result.current.notifyChange());
    await advance(29000);
    expect(svc.UploadDraft).not.toHaveBeenCalled();
    await advance(1000);
    expect(svc.UploadDraft).toHaveBeenCalledTimes(1);
    // El segundo cambio se guardó antes de subir; la misma fila, ya con id.
    expect(svc.SaveDraft).toHaveBeenLastCalledWith(9, req);
  });

  it("vacío no guarda", async () => {
    const { result } = setup({ empty: true });
    act(() => result.current.notifyChange());
    await advance(DRAFT_UPLOAD_DEBOUNCE_MS);
    await act(() => result.current.flush());
    expect(svc.SaveDraft).not.toHaveBeenCalled();
    expect(svc.UploadDraft).not.toHaveBeenCalled();
    expect(result.current.status).toBe("idle");
  });

  it("flush guarda y sube ya", async () => {
    const { result } = setup();
    act(() => result.current.notifyChange());
    await act(() => result.current.flush());
    expect(svc.SaveDraft).toHaveBeenCalledWith(0, req);
    await advance(0);
    expect(svc.UploadDraft).toHaveBeenCalledWith(9);
    // Los timers quedaron cancelados: no se vuelve a guardar ni a subir.
    await advance(DRAFT_UPLOAD_DEBOUNCE_MS);
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    expect(svc.UploadDraft).toHaveBeenCalledTimes(1);
  });

  it("discard borra y cancela", async () => {
    const { result } = setup();
    act(() => result.current.notifyChange());
    await advance(DRAFT_LOCAL_DEBOUNCE_MS);
    act(() => result.current.notifyChange());
    await act(() => result.current.discard());
    expect(svc.DeleteDraft).toHaveBeenCalledWith(9);
    await advance(DRAFT_UPLOAD_DEBOUNCE_MS);
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    expect(svc.UploadDraft).not.toHaveBeenCalled();
  });

  it("upload con needsUpload true deja estado local", async () => {
    vi.mocked(svc.UploadDraft).mockResolvedValue(draft({ needsUpload: true, uploadError: "Sin conexión." }));
    const { result } = setup();
    act(() => result.current.notifyChange());
    await advance(DRAFT_UPLOAD_DEBOUNCE_MS);
    expect(svc.UploadDraft).toHaveBeenCalledWith(9);
    expect(result.current.status).toBe("local");
  });

  it("las llamadas no se pisan: la subida espera al guardado en vuelo", async () => {
    let finishSave: (d: Draft) => void = () => {};
    vi.mocked(svc.SaveDraft).mockReturnValueOnce(
      new Promise<Draft>((r) => (finishSave = r)) as ReturnType<typeof svc.SaveDraft>,
    );
    const { result } = setup();
    act(() => result.current.notifyChange());
    await advance(DRAFT_LOCAL_DEBOUNCE_MS);
    expect(result.current.status).toBe("saving");
    act(() => void result.current.flush());
    await advance(0);
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    expect(svc.UploadDraft).not.toHaveBeenCalled();
    await act(async () => finishSave(draft()));
    await advance(0);
    expect(svc.UploadDraft).toHaveBeenCalledWith(9);
  });

  // Se borró de Borradores con el redactor abierto: lo escrito no se pierde.
  it("un borrador que ya no existe se vuelve a crear con lo escrito", async () => {
    vi.mocked(svc.SaveDraft)
      .mockRejectedValueOnce(notFound())
      .mockResolvedValue(draft({ id: 12, messageId: "<n@x>" }));
    const { result } = setup({ initialDraftId: 4 });
    act(() => result.current.notifyChange());
    await act(() => result.current.flush());
    expect(vi.mocked(svc.SaveDraft).mock.calls).toEqual([[4, req], [0, req]]);
    expect(result.current.current()).toEqual({ draftId: 12, messageId: "<n@x>" });
    expect(result.current.draftId).toBe(12);
    // Sigue autoguardando sobre el nuevo.
    act(() => result.current.notifyChange());
    await advance(DRAFT_UPLOAD_DEBOUNCE_MS);
    expect(svc.SaveDraft).toHaveBeenLastCalledWith(12, req);
    expect(svc.UploadDraft).toHaveBeenLastCalledWith(12);
  });

  it("si no se puede volver a crear, flush rechaza y lo deja pendiente", async () => {
    vi.mocked(svc.SaveDraft).mockRejectedValueOnce(notFound()).mockRejectedValueOnce(new Error("disco lleno"));
    const { result } = setup({ initialDraftId: 4 });
    act(() => result.current.notifyChange());
    await expect(act(() => result.current.flush())).rejects.toThrow("disco lleno");
    expect(result.current.current().draftId).toBe(0);
    await act(() => result.current.flush());
    expect(svc.SaveDraft).toHaveBeenLastCalledWith(0, req);
    expect(result.current.current().draftId).toBe(9);
  });

  it("si desaparece al subirlo, no guarda nada hasta el próximo cambio", async () => {
    vi.mocked(svc.UploadDraft).mockRejectedValue(notFound());
    const { result } = setup({ initialDraftId: 4 });
    act(() => result.current.notifyChange());
    await act(() => result.current.flush());
    await advance(0);
    expect(svc.UploadDraft).toHaveBeenCalledWith(9);
    expect(result.current.current().draftId).toBe(0);
    expect(result.current.status).toBe("idle");
    // Sin cambios nuevos no hay nada que guardar.
    await act(() => result.current.flush());
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    // Un cambio después lo vuelve a crear.
    act(() => result.current.notifyChange());
    await act(() => result.current.flush());
    expect(svc.SaveDraft).toHaveBeenLastCalledWith(0, req);
  });

  it("cancel espera la llamada en vuelo", async () => {
    let finishSave: (d: Draft) => void = () => {};
    vi.mocked(svc.SaveDraft).mockReturnValueOnce(
      new Promise<Draft>((r) => (finishSave = r)) as ReturnType<typeof svc.SaveDraft>,
    );
    const { result } = setup();
    act(() => result.current.notifyChange());
    await advance(DRAFT_LOCAL_DEBOUNCE_MS);
    let done = false;
    act(() => void result.current.cancel().then(() => (done = true)));
    await advance(0);
    expect(done).toBe(false);
    await act(async () => finishSave(draft({ id: 21, messageId: "<n@x>" })));
    await advance(0);
    expect(done).toBe(true);
    expect(result.current.current()).toEqual({ draftId: 21, messageId: "<n@x>" });
    await advance(DRAFT_UPLOAD_DEBOUNCE_MS);
    expect(svc.UploadDraft).not.toHaveBeenCalled();
  });

  it("después de cancel no guarda (ni al desmontar) hasta resume", async () => {
    const { result, unmount } = setup();
    await act(() => result.current.cancel());
    act(() => result.current.notifyChange());
    await advance(DRAFT_UPLOAD_DEBOUNCE_MS);
    expect(svc.SaveDraft).not.toHaveBeenCalled();
    // El envío falló: vuelve a guardar el cambio que quedó pendiente.
    act(() => result.current.resume());
    await advance(DRAFT_LOCAL_DEBOUNCE_MS);
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
    await act(() => result.current.cancel());
    act(() => result.current.notifyChange());
    unmount();
    await advance(0);
    expect(svc.SaveDraft).toHaveBeenCalledTimes(1);
  });

  it("al desmontar con cambios sin guardar los guarda", async () => {
    const { result, unmount } = setup();
    act(() => result.current.notifyChange());
    unmount();
    await advance(0);
    expect(svc.SaveDraft).toHaveBeenCalledWith(0, req);
    await advance(0);
    expect(svc.UploadDraft).toHaveBeenCalledWith(9);
  });
});

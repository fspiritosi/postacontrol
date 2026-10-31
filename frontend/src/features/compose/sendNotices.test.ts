import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const handlers = new Map<string, (ev: { data: unknown }) => void>();
vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, cb: (ev: { data: unknown }) => void) => {
      handlers.set(name, cb);
      return () => handlers.delete(name);
    },
  },
}));
vi.mock("sonner", () => ({
  toast: { loading: vi.fn(), success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));

import { toast } from "sonner";
import { resetSendNoticesForTests, sendStarted, sendToastId, useSendNotices } from "./sendNotices";

function finish(data: { draftId: number; ok: boolean; message?: string; warning?: string }) {
  act(() => handlers.get("mail:send-finished")!({ data: { message: "", warning: "", ...data } }));
}

describe("avisos de envío", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetSendNoticesForTests();
  });

  it("muestra Enviando… y lo reemplaza por Correo enviado", () => {
    renderHook(() => useSendNotices(vi.fn()));
    sendStarted(4);
    expect(toast.loading).toHaveBeenCalledWith("Enviando correo…", { id: sendToastId(4) });
    finish({ draftId: 4, ok: true });
    expect(toast.success).toHaveBeenCalledWith("Correo enviado", expect.objectContaining({ id: sendToastId(4) }));
    expect(toast.warning).not.toHaveBeenCalled();
  });

  it("si salió con advertencia la muestra aparte", () => {
    renderHook(() => useSendNotices(vi.fn()));
    sendStarted(4);
    finish({ draftId: 4, ok: true, warning: "Sin copia en Enviados." });
    expect(toast.warning).toHaveBeenCalledWith("Sin copia en Enviados.");
  });

  it("si falla avisa que quedó en la Bandeja de salida, con Abrir", () => {
    const open = vi.fn();
    renderHook(() => useSendNotices(open));
    sendStarted(4);
    finish({ draftId: 4, ok: false, message: "El servidor rechazó el destinatario." });
    const [title, opts] = vi.mocked(toast.error).mock.calls[0] as [string, Record<string, unknown>];
    expect(title).toBe("No se pudo enviar el correo");
    expect(opts.id).toBe(sendToastId(4));
    expect(opts.description).toBe("El servidor rechazó el destinatario. Quedó en la Bandeja de salida.");
    expect(opts.duration).toBe(Infinity);
    const action = opts.action as { label: string; onClick: () => void };
    expect(action.label).toBe("Abrir");
    action.onClick();
    expect(open).toHaveBeenCalledWith(4);
  });

  it("si el resultado llega antes que la respuesta, no queda un Enviando… colgado", () => {
    renderHook(() => useSendNotices(vi.fn()));
    finish({ draftId: 8, ok: true });
    sendStarted(8);
    expect(toast.loading).not.toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledTimes(1);
  });
});

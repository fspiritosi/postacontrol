import { useEffect } from "react";
import { act, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
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
vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  OpenComposeWindow: vi.fn(),
  ListLocalDrafts: vi.fn(),
  StagedInfo: vi.fn(),
  PrepareCompose: vi.fn(),
}));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { ComposerProvider, useComposer } from "./ComposerContext";

let api: ReturnType<typeof useComposer>;
function Probe() {
  const c = useComposer();
  useEffect(() => {
    api = c;
  }, [c]);
  return null;
}
function setup() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ComposerProvider>
        <Probe />
      </ComposerProvider>
    </QueryClientProvider>,
  );
}
const open = vi.mocked(svc.OpenComposeWindow);

describe("ComposerProvider", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    open.mockResolvedValue(undefined as never);
  });

  it("openNew abre una ventana con clave vacía", async () => {
    setup();
    act(() => api.openNew(7));
    await waitFor(() => expect(open).toHaveBeenCalled());
    const [json, key] = open.mock.calls[0];
    expect(JSON.parse(json).accountId).toBe(7);
    expect(key).toBe("");
  });

  it("openComposeDraft usa la clave draft:<id>", async () => {
    vi.mocked(svc.StagedInfo).mockResolvedValue([] as never);
    setup();
    await act(() =>
      api.openComposeDraft({ id: 12, messageId: "m", request: { accountId: 1, to: [], subject: "s", bodyHtml: "", reply: null, signatureId: 0 } } as never),
    );
    expect(open.mock.calls[0][1]).toBe("draft:12");
  });

  it("openDraftById usa la clave outbox:<id>", async () => {
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([
      { id: 5, request: { accountId: 1, to: [], subject: "s", bodyHtml: "", reply: null, signatureId: 0 } },
    ] as never);
    setup();
    await act(() => api.openDraftById(5));
    expect(open.mock.calls[0][1]).toBe("outbox:5");
  });

  it("si la ventana no se abre muestra el error", async () => {
    open.mockRejectedValue(new Error("boom"));
    setup();
    act(() => api.openNew(1));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("boom")));
  });

  it("no renderiza ningún diálogo", async () => {
    setup();
    act(() => api.openNew(1));
    await waitFor(() => expect(open).toHaveBeenCalled());
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("compose:send-started muestra Enviando…", () => {
    setup();
    act(() => handlers.get("compose:send-started")!({ data: 42 }));
    expect(toast.loading).toHaveBeenCalledWith("Enviando correo…", { id: "send-42" });
  });

  it("compose:notice muestra el aviso de un redactor que se cerró", () => {
    setup();
    act(() => handlers.get("compose:notice")!({ data: { kind: "success", message: "Guardado en Borradores." } }));
    expect(toast.success).toHaveBeenCalledWith("Guardado en Borradores.");
    act(() => handlers.get("compose:notice")!({ data: { kind: "error", message: "No pudimos borrarlo del servidor." } }));
    expect(toast.error).toHaveBeenCalledWith("No pudimos borrarlo del servidor.");
  });
});

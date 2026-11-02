import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { serializeInit } from "@/lib/composeWindow";
import { ComposeWindowApp } from "./ComposeWindowApp";
import type { ComposerInit } from "./ComposerContext";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ComposeInit: vi.fn(),
  CloseComposeWindow: vi.fn(),
  ComposeFlushed: vi.fn(),
  ComposeSendStarted: vi.fn(),
  SetComposeKey: vi.fn(),
  ListAccounts: vi.fn(),
  ListSignatures: vi.fn(),
  SuggestRecipients: vi.fn(),
  SaveDraft: vi.fn(),
  UploadDraft: vi.fn(),
  DeleteDraft: vi.fn(),
  QueueSend: vi.fn(),
  PickImage: vi.fn(),
  PickAttachments: vi.fn(),
}));
vi.mock("sonner", () => ({
  toast: { loading: vi.fn(), success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));
// Events.On guarda los handlers para dispararlos a mano.
const wails = vi.hoisted(() => ({
  handlers: new Map<string, Set<(ev: { data: unknown }) => void>>(),
  setTitle: (() => {}) as (title: string) => void,
}));
vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, cb: (ev: { data: unknown }) => void) => {
      const set = wails.handlers.get(name) ?? new Set();
      set.add(cb);
      wails.handlers.set(name, set);
      return () => set.delete(cb);
    },
  },
  Window: { SetTitle: (t: string) => wails.setTitle(t) },
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";

const emit = (name: string, data?: unknown) =>
  act(() => wails.handlers.get(name)?.forEach((cb) => cb({ data })));

const account = {
  id: 1, email: "yo@ej.com", displayName: "Yo", username: "yo@ej.com", color: "#000",
  imap: { host: "i", port: 993, security: "tls" },
  smtp: { host: "s", port: 465, security: "tls" },
  saveSentCopy: true,
};

const init: ComposerInit = {
  accountId: 1, to: [], cc: [], bcc: [], subject: "Desde la ventana", bodyHtml: "", quotedHtml: "",
  reply: null, attachments: [], originalAttachments: [], skipOriginalAttachments: [], signatureId: 0,
};

let qc: QueryClient;

function setup() {
  const user = userEvent.setup();
  qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ComposeWindowApp id={3} />
    </QueryClientProvider>,
  );
  return user;
}

beforeAll(() => {
  const noRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] }) as unknown as DOMRectList;
  Range.prototype.getClientRects = noRects;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      unobserve() {}
      disconnect() {}
    },
  );
});

describe("ComposeWindowApp", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    wails.handlers.clear();
    wails.setTitle = vi.fn();
    vi.mocked(svc.ListAccounts).mockResolvedValue([account] as never);
    vi.mocked(svc.ListSignatures).mockResolvedValue([]);
    vi.mocked(svc.SuggestRecipients).mockResolvedValue([]);
    vi.mocked(svc.CloseComposeWindow).mockResolvedValue();
    vi.mocked(svc.ComposeFlushed).mockResolvedValue();
  });

  it("carga el init y muestra el redactor", async () => {
    vi.mocked(svc.ComposeInit).mockResolvedValue(serializeInit(init));
    setup();
    expect(await screen.findByLabelText("Asunto")).toHaveValue("Desde la ventana");
    expect(svc.ComposeInit).toHaveBeenCalledWith(3);
  });

  it("init inválido muestra el error", async () => {
    vi.mocked(svc.ComposeInit).mockResolvedValue("{roto");
    setup();
    expect(await screen.findByText("No pudimos abrir este correo.")).toBeInTheDocument();
    expect(screen.queryByLabelText("Asunto")).toBeNull();
    // Sin redactor, la ✕ y la salida igual tienen que responder.
    emit("compose:close-request");
    expect(svc.CloseComposeWindow).toHaveBeenCalledWith(3);
    emit("compose:flush");
    expect(svc.ComposeFlushed).toHaveBeenCalledWith(3, true, "");
  });

  // La ventana no monta useMailEvents: escucha lo que usa el redactor.
  it("recarga cuentas, firmas y sugerencias cuando cambian", async () => {
    vi.mocked(svc.ComposeInit).mockResolvedValue(serializeInit(init));
    setup();
    await screen.findByLabelText("Asunto");
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const keys = () => invalidate.mock.calls.map(([f]) => (f as { queryKey: unknown[] }).queryKey[0]);
    emit("mail:accounts-changed");
    expect(keys()).toEqual(["accounts"]);
    emit("mail:signatures-changed");
    expect(keys()).toEqual(["accounts", "signatures"]);
    emit("mail:contacts-changed");
    expect(keys()).toEqual(["accounts", "signatures", "contacts", "contact", "contact-for", "suggest"]);
    await waitFor(() => expect(svc.ListAccounts).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(svc.ListSignatures).toHaveBeenCalledTimes(2));
  });

  it("cerrar llama a CloseComposeWindow", async () => {
    vi.mocked(svc.ComposeInit).mockResolvedValue(serializeInit(init));
    const user = setup();
    await screen.findByLabelText("Asunto");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(svc.CloseComposeWindow).toHaveBeenCalledWith(3));
    expect(svc.SaveDraft).not.toHaveBeenCalled();
  });
});

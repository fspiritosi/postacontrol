import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ComposerProvider, useComposer } from "./ComposerContext";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListAccounts: vi.fn(),
  PrepareCompose: vi.fn(),
  QueueSend: vi.fn(),
  ListLocalDrafts: vi.fn(),
  DeleteLocalDraft: vi.fn(),
  OpenExternal: vi.fn(),
}));
vi.mock("sonner", () => ({
  toast: { loading: vi.fn(), success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { toast } from "sonner";

const account = {
  id: 1, email: "yo@ej.com", displayName: "Yo", username: "yo@ej.com", color: "#000",
  imap: { host: "i", port: 993, security: "tls" },
  smtp: { host: "s", port: 465, security: "tls" },
  saveSentCopy: true,
};

function Harness() {
  const c = useComposer();
  return (
    <>
      <button onClick={() => c.openNew(1)}>nuevo</button>
      <button onClick={() => void c.openFrom(7, "reply")}>responder</button>
    </>
  );
}

const draft = {
  id: 5,
  createdAt: "2026-10-30T10:00:00Z",
  request: {
    accountId: 1,
    to: [{ name: "", email: "z@x.com" }],
    cc: [],
    bcc: [],
    subject: "Pendiente",
    bodyHtml: "<p>hola</p>",
    reply: null,
  },
  status: "failed",
  error: "x",
};

function DraftHarness() {
  const c = useComposer();
  return <button onClick={() => c.openDraft(draft as never)}>borrador</button>;
}

function setup() {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ComposerProvider>
        <Harness />
        <DraftHarness />
      </ComposerProvider>
    </QueryClientProvider>,
  );
  return user;
}

describe("Composer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([account] as never);
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([]);
    vi.mocked(svc.DeleteLocalDraft).mockResolvedValue();
    vi.mocked(svc.PrepareCompose).mockResolvedValue({
      accountId: 1,
      to: [{ name: "Ana", email: "ana@x.com" }],
      cc: [],
      subject: "Re: Hola",
      quotedHtml: "",
      reply: { mode: "reply", originalMessageId: 7 },
    } as never);
  });

  it("pone el correo en cola, muestra Enviando… y se cierra solo", async () => {
    vi.mocked(svc.QueueSend).mockResolvedValue(12);
    const user = setup();
    await user.click(screen.getByText("responder"));
    expect(await screen.findByLabelText("Para")).toHaveValue("Ana <ana@x.com>");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    expect(await screen.findByRole("button", { name: "Enviando…" })).toBeDisabled();
    const req = vi.mocked(svc.QueueSend).mock.calls[0][0];
    expect(req.reply).toEqual({ mode: "reply", originalMessageId: 7 });
    expect(req.to).toEqual([{ name: "Ana", email: "ana@x.com" }]);
    expect(req.subject).toBe("Re: Hola");
    expect(req.draftId).toBeUndefined();
    expect(toast.loading).toHaveBeenCalledWith("Enviando correo…", { id: "send-12" });
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument(), {
      timeout: 2000,
    });
  });

  it("si no se puede poner en cola queda abierto con el error", async () => {
    vi.mocked(svc.QueueSend).mockRejectedValue(new Error("Agregá al menos un destinatario."));
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    expect(await screen.findByText("Agregá al menos un destinatario.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Enviar" })).toBeEnabled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(toast.loading).not.toHaveBeenCalled();
  });

  it("deshabilita Enviar sin destinatarios", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    const send = await screen.findByRole("button", { name: "Enviar" });
    expect(send).toBeDisabled();
    await user.type(screen.getByLabelText("Para"), "a@b.com");
    expect(send).toBeEnabled();
  });

  it("un correo de la bandeja de salida se reintenta con su id", async () => {
    vi.mocked(svc.QueueSend).mockResolvedValue(5);
    const user = setup();
    await user.click(screen.getByText("borrador"));
    expect(await screen.findByLabelText("Asunto")).toHaveValue("Pendiente");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalledTimes(1));
    expect(vi.mocked(svc.QueueSend).mock.calls[0][0].draftId).toBe(5);
    expect(svc.DeleteLocalDraft).not.toHaveBeenCalled();
  });

  it("una dirección inválida deshabilita Enviar y la señala", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Para"), "a@b.com, sin-arroba");
    expect(screen.getByRole("button", { name: "Enviar" })).toBeDisabled();
    expect(screen.getByText("Revisá estas direcciones: sin-arroba")).toBeInTheDocument();
  });

  it("pide confirmación al cerrar con cambios", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Asunto"), "algo");
    await user.keyboard("{Escape}");
    expect(await screen.findByText("¿Descartar este correo?")).toBeInTheDocument();
  });
});

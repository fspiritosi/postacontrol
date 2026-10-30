import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ComposerProvider, useComposer } from "./ComposerContext";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListAccounts: vi.fn(),
  PrepareCompose: vi.fn(),
  Send: vi.fn(),
  ListLocalDrafts: vi.fn(),
  DeleteLocalDraft: vi.fn(),
  OpenExternal: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));

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

  it("envía con reply context", async () => {
    vi.mocked(svc.Send).mockResolvedValue({ sentCopySaved: true, warning: "" });
    const user = setup();
    await user.click(screen.getByText("responder"));
    expect(await screen.findByLabelText("Para")).toHaveValue("Ana <ana@x.com>");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.Send).toHaveBeenCalledTimes(1));
    const req = vi.mocked(svc.Send).mock.calls[0][0];
    expect(req.reply).toEqual({ mode: "reply", originalMessageId: 7 });
    expect(req.to).toEqual([{ name: "Ana", email: "ana@x.com" }]);
    expect(req.subject).toBe("Re: Hola");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(toast.success).toHaveBeenCalledWith("Correo enviado");
  });

  it("mantiene abierto y avisa si Send falla", async () => {
    vi.mocked(svc.Send).mockRejectedValue(new Error("El servidor rechazó el envío: 550"));
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    expect(await screen.findByText(/borrador local/)).toBeInTheDocument();
    expect(screen.getByText(/El servidor rechazó el envío/)).toBeInTheDocument();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("deshabilita Enviar sin destinatarios", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    const send = await screen.findByRole("button", { name: "Enviar" });
    expect(send).toBeDisabled();
    await user.type(screen.getByLabelText("Para"), "a@b.com");
    expect(send).toBeEnabled();
  });

  it("avisa con toast de advertencia si no se guardó la copia en Enviados", async () => {
    vi.mocked(svc.Send).mockResolvedValue({ sentCopySaved: false, warning: "No pudimos guardar la copia." });
    const user = setup();
    await user.click(screen.getByText("responder"));
    await screen.findByLabelText("Para");
    await user.click(screen.getByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(toast.warning).toHaveBeenCalledWith("No pudimos guardar la copia."));
  });

  it("al reenviar un borrador con éxito lo borra", async () => {
    vi.mocked(svc.Send).mockResolvedValue({ sentCopySaved: true, warning: "" });
    const user = setup();
    await user.click(screen.getByText("borrador"));
    await user.click(await screen.findByRole("button", { name: "Enviar" }));
    await waitFor(() => expect(svc.DeleteLocalDraft).toHaveBeenCalledWith(5));
  });

  it("si el reenvío de un borrador falla, borra el viejo y conserva el nuevo", async () => {
    vi.mocked(svc.Send).mockRejectedValue(new Error("Sin conexión"));
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([{ ...draft, id: 5 }, { ...draft, id: 9 }] as never);
    const user = setup();
    await user.click(screen.getByText("borrador"));
    await user.click(await screen.findByRole("button", { name: "Enviar" }));
    await screen.findByText(/borrador local/);
    expect(svc.DeleteLocalDraft).toHaveBeenCalledTimes(1);
    expect(svc.DeleteLocalDraft).toHaveBeenCalledWith(5);
  });

  it("pide confirmación al cerrar con cambios", async () => {
    const user = setup();
    await user.click(screen.getByText("nuevo"));
    await user.type(await screen.findByLabelText("Asunto"), "algo");
    await user.keyboard("{Escape}");
    expect(await screen.findByText("¿Descartar este correo?")).toBeInTheDocument();
  });
});

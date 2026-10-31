import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListAccounts: vi.fn(() => Promise.resolve([])),
  ListLocalDrafts: vi.fn(),
  DeleteLocalDraft: vi.fn(() => Promise.resolve()),
  QueueSend: vi.fn(),
}));
vi.mock("sonner", () => ({
  toast: { loading: vi.fn(), success: vi.fn(), error: vi.fn(), warning: vi.fn() },
}));
vi.mock("@wailsio/runtime", () => ({ Events: { On: () => () => {} } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { toast } from "sonner";
import { ComposerProvider } from "../compose/ComposerContext";
import { OutboxList } from "./OutboxList";

const request = {
  accountId: 1,
  to: [{ name: "Ana", email: "ana@x.com" }],
  cc: [],
  bcc: [],
  subject: "Presupuesto",
  bodyHtml: "<p>hola</p>",
  reply: null,
};
const failed = { id: 3, createdAt: "2026-10-31T10:00:00Z", request, status: "failed", error: "El servidor rechazó el destinatario." };
const sending = { ...failed, id: 4, request: { ...request, subject: "En camino" }, status: "sending", error: "" };

function setup() {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ComposerProvider>
        <OutboxList />
      </ComposerProvider>
    </QueryClientProvider>,
  );
  return user;
}

describe("OutboxList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([failed, sending] as never);
  });

  it("muestra destinatario, asunto y por qué falló", async () => {
    setup();
    expect(await screen.findAllByText("Para: Ana")).toHaveLength(2);
    expect(screen.getByText("Presupuesto")).toBeInTheDocument();
    expect(screen.getByText("El servidor rechazó el destinatario.")).toBeInTheDocument();
    expect(screen.getByText("Enviando…")).toBeInTheDocument();
  });

  it("Reintentar vuelve a poner en cola el mismo correo", async () => {
    vi.mocked(svc.QueueSend).mockResolvedValue(3);
    const user = setup();
    await screen.findByText("Presupuesto");
    const [retry] = screen.getAllByRole("button", { name: "Reintentar" });
    await user.click(retry);
    await waitFor(() => expect(svc.QueueSend).toHaveBeenCalledWith({ ...request, draftId: 3 }));
    expect(toast.loading).toHaveBeenCalledWith("Enviando correo…", { id: "send-3" });
  });

  it("no deja tocar un correo que se está enviando", async () => {
    setup();
    await screen.findByText("En camino");
    const retries = screen.getAllByRole("button", { name: "Reintentar" });
    expect(retries[0]).toBeEnabled();
    expect(retries[1]).toBeDisabled();
  });

  it("Abrir lo lleva al redactor", async () => {
    const user = setup();
    await screen.findByText("Presupuesto");
    await user.click(screen.getAllByRole("button", { name: "Abrir" })[0]);
    expect(await screen.findByLabelText("Asunto")).toHaveValue("Presupuesto");
  });

  it("Eliminar pide confirmación", async () => {
    const user = setup();
    await screen.findByText("Presupuesto");
    await user.click(screen.getAllByRole("button", { name: "Eliminar" })[0]);
    expect(await screen.findByText("¿Eliminar este correo?")).toBeInTheDocument();
    expect(svc.DeleteLocalDraft).not.toHaveBeenCalled();
    await user.click(screen.getAllByRole("button", { name: "Eliminar" }).pop()!);
    await waitFor(() => expect(svc.DeleteLocalDraft).toHaveBeenCalledWith(3));
  });

  it("vacía, lo dice", async () => {
    vi.mocked(svc.ListLocalDrafts).mockResolvedValue([]);
    setup();
    expect(await screen.findByText("No hay correos pendientes de envío.")).toBeInTheDocument();
  });
});

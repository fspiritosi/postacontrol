import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AccountEditDialog } from "./AccountEditDialog";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  UpdateAccount: vi.fn(),
  ListSignatures: vi.fn(() => Promise.resolve([{ id: 1, name: "Trabajo", html: "<p>x</p>" }])),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import { toast } from "sonner";

const account: Account = {
  id: 7,
  email: "yo@ejemplo.com",
  displayName: "Yo",
  username: "",
  color: "",
  imap: { host: "imap.ejemplo.com", port: 993, security: Security.SecurityTLS },
  smtp: { host: "smtp.ejemplo.com", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
  defaultSignatureId: 0,
};

function setup(acc: Account = account) {
  const user = userEvent.setup();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AccountEditDialog account={acc} open onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  );
  return { user, onOpenChange };
}

describe("AccountEditDialog", () => {
  beforeEach(() => vi.clearAllMocks());

  it("abre con los datos de la cuenta y la contraseña vacía", () => {
    setup();
    expect(screen.getByLabelText("Nombre para mostrar")).toHaveValue("Yo");
    expect(screen.getByLabelText("Dirección")).toHaveValue("yo@ejemplo.com");
    expect(screen.getByLabelText("Servidor IMAP")).toHaveValue("imap.ejemplo.com");
    expect(screen.getByLabelText("Contraseña")).toHaveValue("");
    expect(screen.getByText("Dejala vacía para conservar la actual.")).toBeInTheDocument();
  });

  it("cambiar el nombre y guardar llama a UpdateAccount", async () => {
    vi.mocked(svc.UpdateAccount).mockResolvedValue({} as never);
    const { user, onOpenChange } = setup();
    const name = screen.getByLabelText("Nombre para mostrar");
    await user.clear(name);
    await user.type(name, "Nuevo");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(svc.UpdateAccount).toHaveBeenCalledWith(
      7,
      expect.objectContaining({ id: 7, displayName: "Nuevo", password: "" }),
    );
    expect(toast.success).toHaveBeenCalledWith("Cuenta guardada.");
  });

  it("elegir un color lo manda en color", async () => {
    vi.mocked(svc.UpdateAccount).mockResolvedValue({} as never);
    const { user } = setup();
    await user.click(screen.getByRole("button", { name: "#dc2626" }));
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(svc.UpdateAccount).toHaveBeenCalled());
    expect(vi.mocked(svc.UpdateAccount).mock.calls[0][1]).toMatchObject({ color: "#dc2626" });
  });

  it("un color fuera de la paleta aparece elegido", () => {
    setup({ ...account, color: "#9333ea" });
    expect(screen.getByRole("button", { name: "#9333ea" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "#dc2626" })).toHaveAttribute("aria-pressed", "false");
  });

  it("cambiar el host IMAP muestra el aviso de cambio de buzón", async () => {
    const { user } = setup();
    expect(screen.queryByText(/se van a volver a bajar/)).toBeNull();
    const host = screen.getByLabelText("Servidor IMAP");
    await user.clear(host);
    await user.type(host, "otro.ejemplo.com");
    expect(screen.getByText(/se van a volver a bajar/)).toBeInTheDocument();
  });

  it("cambiar solo mayúsculas del host no avisa", async () => {
    const { user } = setup();
    const host = screen.getByLabelText("Servidor IMAP");
    await user.clear(host);
    await user.type(host, "IMAP.Ejemplo.com");
    expect(screen.queryByText(/se van a volver a bajar/)).toBeNull();
  });

  it("un rechazo muestra el mensaje y no cierra", async () => {
    vi.mocked(svc.UpdateAccount).mockRejectedValue(new Error("El servidor rechazó la contraseña."));
    const { user, onOpenChange } = setup();
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    expect(await screen.findByText("El servidor rechazó la contraseña.")).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(toast.success).not.toHaveBeenCalled();
  });
});

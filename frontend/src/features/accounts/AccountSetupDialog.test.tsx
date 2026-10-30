import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { AccountSetupDialog } from "./AccountSetupDialog";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  DetectSettings: vi.fn(),
  AddAccount: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { toast } from "sonner";

const hostinger = {
  imap: { host: "imap.hostinger.com", port: 993, security: "tls" },
  smtp: { host: "smtp.hostinger.com", port: 465, security: "tls" },
  username: "yo@ejemplo.com",
  source: "known",
};

async function openStep2() {
  const user = userEvent.setup();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AccountSetupDialog open onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  );
  await user.type(screen.getByLabelText("Correo electrónico"), "yo@ejemplo.com");
  await user.type(screen.getByLabelText("Contraseña"), "secreta");
  await user.click(screen.getByRole("button", { name: "Continuar" }));
  return { user, onOpenChange };
}

describe("AccountSetupDialog", () => {
  beforeEach(() => vi.clearAllMocks());

  it("muestra los servidores detectados", async () => {
    vi.mocked(svc.DetectSettings).mockResolvedValue(hostinger as never);
    await openStep2();
    expect(await screen.findByText("Encontramos la configuración")).toBeInTheDocument();
    expect(screen.getByLabelText("Servidor IMAP")).toHaveValue("imap.hostinger.com");
    expect(screen.getByLabelText("Puerto IMAP")).toHaveValue(993);
  });

  it("permite carga manual si no detecta", async () => {
    vi.mocked(svc.DetectSettings).mockRejectedValue(new Error("no se pudo"));
    await openStep2();
    expect(await screen.findByText(/cargalos a mano/)).toBeInTheDocument();
    const host = screen.getByLabelText("Servidor IMAP");
    expect(host).toHaveValue("");
    expect(host).toBeEnabled();
    expect(screen.getByLabelText("Puerto IMAP")).toHaveValue(null);
    expect(screen.getByLabelText("Servidor SMTP")).toBeEnabled();
  });

  it("muestra el error de AddAccount sin cerrar", async () => {
    vi.mocked(svc.DetectSettings).mockResolvedValue(hostinger as never);
    vi.mocked(svc.AddAccount).mockRejectedValue(
      new Error("El servidor rechazó el usuario o la contraseña."),
    );
    const { user, onOpenChange } = await openStep2();
    await user.click(await screen.findByRole("button", { name: "Probar y guardar" }));
    expect(
      await screen.findByText("El servidor rechazó el usuario o la contraseña."),
    ).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("cierra y avisa cuando AddAccount funciona", async () => {
    vi.mocked(svc.DetectSettings).mockResolvedValue(hostinger as never);
    vi.mocked(svc.AddAccount).mockResolvedValue({} as never);
    const { user, onOpenChange } = await openStep2();
    await user.click(await screen.findByRole("button", { name: "Probar y guardar" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(toast.success).toHaveBeenCalledWith("Cuenta agregada. Sincronizando…");
    expect(svc.AddAccount).toHaveBeenCalledWith(
      expect.objectContaining({ email: "yo@ejemplo.com", password: "secreta", username: "yo@ejemplo.com" }),
    );
  });
});

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

  it("muestra el mensaje de un RuntimeError real de Wails", async () => {
    vi.mocked(svc.DetectSettings).mockResolvedValue(hostinger as never);
    vi.mocked(svc.AddAccount).mockRejectedValue(Object.assign(new Error("No pudimos conectar."), { name: "RuntimeError", cause: {} }));
    const { user } = await openStep2();
    await user.click(await screen.findByRole("button", { name: "Probar y guardar" }));
    expect(await screen.findByText("No pudimos conectar.")).toBeInTheDocument();
  });

  it("deshabilita guardar sin host o con puerto inválido", async () => {
    vi.mocked(svc.DetectSettings).mockRejectedValue(new Error("x"));
    const { user } = await openStep2();
    const save = await screen.findByRole("button", { name: "Probar y guardar" });
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText("Servidor IMAP"), "imap.x.com");
    await user.type(screen.getByLabelText("Puerto IMAP"), "993");
    await user.type(screen.getByLabelText("Servidor SMTP"), "smtp.x.com");
    await user.type(screen.getByLabelText("Puerto SMTP"), "70000");
    expect(save).toBeDisabled();
    await user.clear(screen.getByLabelText("Puerto SMTP"));
    await user.type(screen.getByLabelText("Puerto SMTP"), "465");
    expect(save).toBeEnabled();
    expect(svc.AddAccount).not.toHaveBeenCalled();
  });

  it("muestra los textos de carga", async () => {
    let resolveDetect!: (v: unknown) => void;
    vi.mocked(svc.DetectSettings).mockReturnValue(new Promise((r) => (resolveDetect = r)) as never);
    let rejectAdd!: (e: Error) => void;
    vi.mocked(svc.AddAccount).mockReturnValue(new Promise((_, r) => (rejectAdd = r)) as never);
    const { user } = await openStep2();
    expect(screen.getByText("Buscando la configuración…")).toBeInTheDocument();
    resolveDetect(hostinger);
    await user.click(await screen.findByRole("button", { name: "Probar y guardar" }));
    expect(screen.getByText("Probando conexión…")).toBeInTheDocument();
    rejectAdd(new Error("fallo"));
    expect(await screen.findByText("fallo")).toBeInTheDocument();
  });

  it("no cierra durante la detección y descarta resultados viejos", async () => {
    let resolveDetect!: (v: unknown) => void;
    vi.mocked(svc.DetectSettings).mockReturnValue(new Promise((r) => (resolveDetect = r)) as never);
    const { user, onOpenChange } = await openStep2();
    await user.keyboard("{Escape}");
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    resolveDetect(hostinger);
    expect(await screen.findByText("Encontramos la configuración")).toBeInTheDocument();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() => Promise.resolve({ unifiedInbox: false, disableNotifications: false, threadedList: false })),
  SetSettings: vi.fn(() => Promise.resolve()),
  GetStartAtLogin: vi.fn(() => Promise.resolve(false)),
  SetStartAtLogin: vi.fn(() => Promise.resolve()),
  ListAccounts: vi.fn(() => Promise.resolve([])),
  ListSignatures: vi.fn(() => Promise.resolve([])),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { SettingsDialog } from "./SettingsDialog";

function renderDialog() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SettingsDialog open onOpenChange={() => {}} />
    </QueryClientProvider>,
  );
}

describe("SettingsDialog", () => {
  beforeEach(() => vi.clearAllMocks());

  it("el interruptor guarda la bandeja unificada", async () => {
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SettingsDialog open onOpenChange={() => {}} />
      </QueryClientProvider>,
    );
    const sw = await screen.findByRole("switch", { name: "Bandeja unificada" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    await user.click(sw);
    await waitFor(() => expect(svc.SetSettings).toHaveBeenCalledWith({ unifiedInbox: true, disableNotifications: false, threadedList: false }));
  });

  it("el interruptor de notificaciones guarda disableNotifications", async () => {
    const user = userEvent.setup();
    renderDialog();
    const sw = await screen.findByRole("switch", { name: "Notificaciones de correo nuevo" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    expect(sw.getAttribute("aria-checked")).toBe("true");
    await user.click(sw);
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({ unifiedInbox: false, disableNotifications: true, threadedList: false }),
    );
  });

  it("Iniciar al abrir sesión llama SetStartAtLogin", async () => {
    const user = userEvent.setup();
    renderDialog();
    const sw = await screen.findByRole("switch", { name: "Iniciar al abrir sesión" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    await user.click(sw);
    await waitFor(() => expect(svc.SetStartAtLogin).toHaveBeenCalledWith(true));
  });

  it("si SetStartAtLogin falla, vuelve atrás y avisa", async () => {
    const user = userEvent.setup();
    vi.mocked(svc.SetStartAtLogin).mockRejectedValueOnce(new Error("No se pudo"));
    renderDialog();
    const sw = await screen.findByRole("switch", { name: "Iniciar al abrir sesión" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    await user.click(sw);
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    await waitFor(() => expect(sw.getAttribute("aria-checked")).toBe("false"));
  });
});

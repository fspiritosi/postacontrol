import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListAccounts: vi.fn(),
  ReorderAccounts: vi.fn(),
  ListSignatures: vi.fn(() => Promise.resolve([])),
  UpdateAccount: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { AccountsSection } from "./AccountsSection";

const mk = (id: number, displayName: string) => ({
  id,
  email: `${displayName.toLowerCase()}@ejemplo.com`,
  displayName,
  username: "",
  color: "#dc2626",
  imap: { host: "imap.ejemplo.com", port: 993, security: "tls" },
  smtp: { host: "smtp.ejemplo.com", port: 465, security: "tls" },
  saveSentCopy: true,
  defaultSignatureId: 0,
});
const accounts = [mk(1, "A"), mk(2, "B"), mk(3, "C")];

function setup() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <AccountsSection />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("Cuentas en Configuración", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue(accounts as never);
    vi.mocked(svc.ReorderAccounts).mockResolvedValue();
  });

  it("Bajar A llama a ReorderAccounts con el orden nuevo", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Bajar A" }));
    await waitFor(() => expect(svc.ReorderAccounts).toHaveBeenCalledWith([2, 1, 3]));
  });

  it("Subir en la primera y Bajar en la última están deshabilitados", async () => {
    setup();
    expect(await screen.findByRole("button", { name: "Subir A" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Bajar C" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Subir B" })).toBeEnabled();
  });

  it("un rechazo muestra el toast y vuelve a pedir las cuentas", async () => {
    vi.mocked(svc.ReorderAccounts).mockRejectedValue(new Error("La lista de cuentas cambió. Probá de nuevo."));
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Bajar A" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    await waitFor(() => expect(svc.ListAccounts).toHaveBeenCalledTimes(2));
  });

  it("Editar abre el diálogo con esa cuenta", async () => {
    const user = setup();
    await screen.findByRole("button", { name: "Bajar A" });
    await user.click(screen.getAllByRole("button", { name: /^Editar/ })[1]);
    expect(await screen.findByLabelText("Dirección")).toHaveValue("b@ejemplo.com");
  });
});

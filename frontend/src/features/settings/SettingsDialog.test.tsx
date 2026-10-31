import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() => Promise.resolve({ unifiedInbox: false })),
  SetSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { SettingsDialog } from "./SettingsDialog";

describe("SettingsDialog", () => {
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
    await waitFor(() => expect(svc.SetSettings).toHaveBeenCalledWith({ unifiedInbox: true }));
  });
});

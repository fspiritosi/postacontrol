import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(() => Promise.resolve([])),
  ListLocalDrafts: vi.fn(() => Promise.resolve([])),
  RemoveAccount: vi.fn(() => Promise.resolve()),
  SetSaveSentCopy: vi.fn(() => Promise.resolve()),
  SyncAccount: vi.fn(() => Promise.resolve()),
  SyncFolder: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { FolderSidebar } from "./FolderSidebar";

const account: Account = {
  id: 7, email: "a@b.c", displayName: "A", username: "a", color: "#f00",
  imap: { host: "", port: 993, security: Security.SecurityTLS },
  smtp: { host: "", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
};

const baseState = {
  accountId: 7, folderId: 3, messageId: null, outbox: false,
  selectFolder: vi.fn(), selectMessage: vi.fn(), openOutbox: vi.fn(),
};

describe("FolderSidebar", () => {
  it("muestra la Bandeja de salida solo si hay pendientes, y la abre", async () => {
    vi.mocked(svc.ListLocalDrafts).mockResolvedValueOnce([
      { id: 1, status: "failed" },
      { id: 2, status: "sending" },
    ] as never);
    const user = userEvent.setup();
    const openOutbox = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar account={account} state={{ ...baseState, openOutbox }} />
      </QueryClientProvider>,
    );
    const row = await screen.findByRole("button", { name: /Bandeja de salida/ });
    expect(row.textContent).toContain("2");
    await user.click(row);
    expect(openOutbox).toHaveBeenCalled();
  });

  it("sin pendientes no muestra la Bandeja de salida", async () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar account={account} state={baseState} />
      </QueryClientProvider>,
    );
    await waitFor(() => expect(svc.ListLocalDrafts).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: /Bandeja de salida/ })).toBeNull();
  });

  it("al quitar la cuenta limpia la carpeta y el mensaje seleccionados", async () => {
    const user = userEvent.setup();
    const selectFolder = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar
          account={account}
          state={{ ...baseState, messageId: 9, selectFolder }}
        />
      </QueryClientProvider>,
    );
    await user.click(screen.getByLabelText("Opciones de la cuenta"));
    await user.click(await screen.findByText("Quitar cuenta"));
    expect(screen.getByText(/Se borra la copia local/)).toBeTruthy();
    await user.click(screen.getAllByRole("button", { name: "Quitar cuenta" }).pop()!);
    await waitFor(() => expect(svc.RemoveAccount).toHaveBeenCalledWith(7));
    await waitFor(() => expect(selectFolder).toHaveBeenCalledWith(null));
  });

  it("el menú de la cuenta muestra y cambia Guardar copia en Enviados", async () => {
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar
          account={account}
          state={baseState}
        />
      </QueryClientProvider>,
    );
    await user.click(screen.getByLabelText("Opciones de la cuenta"));
    const item = await screen.findByRole("menuitemcheckbox", { name: "Guardar copia en Enviados" });
    expect(item.getAttribute("aria-checked")).toBe("true");
    await user.click(item);
    await waitFor(() => expect(svc.SetSaveSentCopy).toHaveBeenCalledWith(7, false));
  });
});

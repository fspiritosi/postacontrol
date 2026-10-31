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
import type { ShellState } from "../layout/MailShell";
import { FolderSidebar } from "./FolderSidebar";

const account: Account = {
  id: 7, email: "a@b.c", displayName: "A", username: "a", color: "#f00",
  imap: { host: "", port: 993, security: Security.SecurityTLS },
  smtp: { host: "", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
};

const baseState: ShellState = {
  view: { kind: "folder", folderId: 3 },
  messageId: null,
  selectView: vi.fn(),
  selectMessage: vi.fn(),
};

describe("FolderSidebar", () => {
  it("muestra la Bandeja de salida solo si hay pendientes, y la abre", async () => {
    vi.mocked(svc.ListLocalDrafts).mockResolvedValueOnce([
      { id: 1, status: "failed" },
      { id: 2, status: "sending" },
    ] as never);
    const user = userEvent.setup();
    const selectView = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar account={account} state={{ ...baseState, selectView }} />
      </QueryClientProvider>,
    );
    const row = await screen.findByRole("button", { name: /Bandeja de salida/ });
    expect(row.textContent).toContain("2");
    await user.click(row);
    expect(selectView).toHaveBeenCalledWith({ kind: "outbox" });
  });

  it("al elegir una carpeta abre su vista y marca la activa", async () => {
    vi.mocked(svc.ListFolders).mockResolvedValueOnce([
      { id: 3, accountId: 7, name: "INBOX", delimiter: "/", role: "inbox", unread: 0, total: 0, selectable: true },
      { id: 4, accountId: 7, name: "Sent", delimiter: "/", role: "sent", unread: 0, total: 0, selectable: true },
    ]);
    const user = userEvent.setup();
    const selectView = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar account={account} state={{ ...baseState, selectView }} />
      </QueryClientProvider>,
    );
    const inbox = await screen.findByRole("button", { name: "Entrada" });
    expect(inbox.getAttribute("aria-current")).toBe("true");
    await user.click(screen.getByRole("button", { name: "Enviados" }));
    expect(selectView).toHaveBeenCalledWith({ kind: "folder", folderId: 4 });
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

  it("al quitar la cuenta limpia el mensaje seleccionado", async () => {
    const user = userEvent.setup();
    const selectMessage = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar
          account={account}
          state={{ ...baseState, messageId: 9, selectMessage }}
        />
      </QueryClientProvider>,
    );
    await user.click(screen.getByLabelText("Opciones de la cuenta"));
    await user.click(await screen.findByText("Quitar cuenta"));
    expect(screen.getByText(/Se borra la copia local/)).toBeTruthy();
    await user.click(screen.getAllByRole("button", { name: "Quitar cuenta" }).pop()!);
    await waitFor(() => expect(svc.RemoveAccount).toHaveBeenCalledWith(7));
    await waitFor(() => expect(selectMessage).toHaveBeenCalledWith(null));
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

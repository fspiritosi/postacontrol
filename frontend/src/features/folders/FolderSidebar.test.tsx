import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(() => Promise.resolve([])),
  RemoveAccount: vi.fn(() => Promise.resolve()),
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

describe("FolderSidebar", () => {
  it("al quitar la cuenta limpia la carpeta y el mensaje seleccionados", async () => {
    const user = userEvent.setup();
    const selectFolder = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar
          account={account}
          state={{ accountId: 7, folderId: 3, messageId: 9, selectFolder, selectMessage: vi.fn() }}
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
});

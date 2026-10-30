import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(() => Promise.resolve([])),
  ListMessages: vi.fn(() => Promise.resolve([])),
  SyncFolder: vi.fn(() => new Promise(() => {})), // nunca termina
}));

import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import { resetSyncStateForTests, syncFolder } from "@/lib/events";
import { MessageList } from "./MessageList";

const account: Account = {
  id: 7, email: "a@b.c", displayName: "A", username: "a", color: "#f00",
  imap: { host: "", port: 993, security: Security.SecurityTLS },
  smtp: { host: "", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
};

function renderList(folderId: number) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MessageList
        account={account}
        state={{ accountId: 7, folderId, messageId: null, selectFolder: vi.fn(), selectMessage: vi.fn() }}
      />
    </QueryClientProvider>,
  );
}

describe("MessageList", () => {
  beforeEach(() => resetSyncStateForTests());

  it("con la carpeta vacía y sin sincronizar, dice que no hay correos", async () => {
    renderList(3);
    expect(await screen.findByText("No hay correos en esta carpeta.")).toBeTruthy();
  });

  it("mientras la carpeta se sincroniza y está vacía, muestra Sincronizando…", async () => {
    renderList(3);
    act(() => void syncFolder(7, 3));
    expect(await screen.findByText("Sincronizando…")).toBeTruthy();
    expect(screen.queryByText("No hay correos en esta carpeta.")).toBeNull();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(() => Promise.resolve([])),
  ListMessages: vi.fn(() => Promise.resolve([])),
  ListUnified: vi.fn(() => Promise.resolve([])),
  SyncFolder: vi.fn(() => new Promise(() => {})), // nunca termina
  SetFlagged: vi.fn(() => Promise.resolve()),
}));

import type { Account, MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { resetSyncStateForTests, syncFolder } from "@/lib/events";
import type { View } from "@/lib/view";
import { MessageList } from "./MessageList";

function account(id: number, color: string): Account {
  return {
    id, email: `${id}@b.c`, displayName: "A", username: "a", color,
    imap: { host: "", port: 993, security: Security.SecurityTLS },
    smtp: { host: "", port: 465, security: Security.SecurityTLS },
    saveSentCopy: true,
  };
}

const accounts = [account(7, "#ff0000"), account(8, "#00ff00")];

function summary(id: number, accountId: number): MessageSummary {
  return {
    id, accountId, folderId: accountId * 10, uid: id,
    from: { name: `Remitente ${id}`, email: "x@y.z" },
    subject: `Asunto ${id}`, snippet: "", date: "2026-10-30T10:00:00Z",
    seen: true, flagged: false, answered: false, hasAttachments: false,
  };
}

function renderList(view: View, selectMessage = vi.fn()) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MessageList
        accounts={accounts}
        state={{ view, messageId: null, selectView: vi.fn(), selectMessage, search: null, setSearch: vi.fn() }}
      />
    </QueryClientProvider>,
  );
}

describe("MessageList", () => {
  beforeEach(() => resetSyncStateForTests());

  it("con la carpeta vacía y sin sincronizar, dice que no hay correos", async () => {
    renderList({ kind: "folder", folderId: 3 });
    expect(await screen.findByText("No hay correos en esta carpeta.")).toBeTruthy();
  });

  it("mientras la carpeta se sincroniza y está vacía, muestra Sincronizando…", async () => {
    renderList({ kind: "folder", folderId: 3 });
    act(() => void syncFolder(7, 3));
    expect(await screen.findByText("Sincronizando…")).toBeTruthy();
    expect(screen.queryByText("No hay correos en esta carpeta.")).toBeNull();
  });

  it("en vista unificada muestra el color de la cuenta de cada mensaje y el título Entrada", async () => {
    vi.mocked(svc.ListUnified).mockResolvedValueOnce([summary(1, 7), summary(2, 8)]);
    renderList({ kind: "unified", role: "inbox" });
    expect(screen.getByRole("heading", { name: "Entrada" })).toBeTruthy();
    const first = await screen.findByRole("button", { name: /Asunto 1/ });
    const second = screen.getByRole("button", { name: /Asunto 2/ });
    const dot = (el: HTMLElement) =>
      (el.querySelector("[aria-hidden]") as HTMLElement).style.backgroundColor;
    expect(dot(first)).toBe("rgb(255, 0, 0)");
    expect(dot(second)).toBe("rgb(0, 255, 0)");
    expect(svc.ListUnified).toHaveBeenCalledWith("inbox", { unreadOnly: false, offset: 0, limit: 100 });
  });

  it("la estrella destaca sin abrir el mensaje", async () => {
    vi.mocked(svc.ListUnified).mockResolvedValueOnce([summary(1, 7)]);
    const selectMessage = vi.fn();
    renderList({ kind: "unified", role: "inbox" }, selectMessage);
    const star = await screen.findByRole("button", { name: "Destacar" });
    fireEvent.click(star);
    expect(svc.SetFlagged).toHaveBeenCalledWith([1], true);
    expect(selectMessage).not.toHaveBeenCalled();
  });
});

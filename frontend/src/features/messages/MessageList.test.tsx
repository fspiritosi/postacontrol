import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(() => Promise.resolve([])),
  ListMessages: vi.fn(() => Promise.resolve([])),
  ListUnified: vi.fn(() => Promise.resolve([])),
  SyncFolder: vi.fn(() => new Promise(() => {})), // nunca termina
  SetFlagged: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(() =>
    Promise.resolve({ unifiedInbox: false, disableNotifications: false, threadedList: false }),
  ),
  SetSettings: vi.fn(() => Promise.resolve()),
  ListThreads: vi.fn(() => Promise.resolve([])),
  ListPendingDrafts: vi.fn(() => Promise.resolve([])),
}));
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => ({ openComposeDraft: vi.fn() }) }));

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
    defaultSignatureId: 0,
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
        state={{
          view, messageId: null, selectView: vi.fn(), selectMessage, search: null, setSearch: vi.fn(),
          threadRoot: null, selectThread: vi.fn(),
          contactId: null, selectContact: vi.fn(), openContact: vi.fn(),
        }}
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

describe("MessageList conversaciones", () => {
  beforeEach(() => {
    resetSyncStateForTests();
    vi.clearAllMocks();
  });

  it("el interruptor guarda threadedList y muestra hilos", async () => {
    const settings = { unifiedInbox: true, disableNotifications: true, threadedList: false };
    vi.mocked(svc.GetSettings).mockResolvedValue(settings);
    vi.mocked(svc.ListThreads).mockResolvedValue([
      { threadRoot: "<r@x>", latest: summary(1, 7), count: 2, unread: 0, participants: ["Ana", "Beto"] },
    ]);
    renderList({ kind: "folder", folderId: 3 });
    const sw = await screen.findByRole("switch", { name: "Conversaciones" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    vi.mocked(svc.GetSettings).mockResolvedValue({ ...settings, threadedList: true });
    fireEvent.click(sw);
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({ ...settings, threadedList: true }),
    );
    expect(await screen.findByText("Ana, Beto")).toBeTruthy();
    expect(svc.ListThreads).toHaveBeenCalledWith(3, { unreadOnly: false, offset: 0, limit: 100 });
  });

  it("en vista unificada no aparece el interruptor", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValue({
      unifiedInbox: true, disableNotifications: false, threadedList: true,
    });
    renderList({ kind: "unified", role: "inbox" });
    expect(await screen.findByText("No hay correos en esta vista.")).toBeTruthy();
    expect(screen.queryByRole("switch", { name: "Conversaciones" })).toBeNull();
    expect(svc.ListThreads).not.toHaveBeenCalled();
  });
});

describe("MessageList borradores sin subir", () => {
  const pending = (id: number, accountId: number) => ({
    id, messageId: "", needsUpload: true, uploadError: "", updatedAt: "",
    request: { accountId, subject: `Borrador ${id}` },
  });

  beforeEach(() => {
    resetSyncStateForTests();
    vi.clearAllMocks();
    vi.mocked(svc.ListPendingDrafts).mockResolvedValue([pending(1, 7), pending(2, 8)] as never);
  });

  it("en la carpeta Borradores avisa de los pendientes de esa cuenta", async () => {
    vi.mocked(svc.ListFolders).mockImplementation((accountId: number) =>
      Promise.resolve(
        accountId === 7
          ? [{ id: 70, accountId: 7, name: "Drafts", delimiter: "/", role: "drafts", unread: 0, total: 0, selectable: true }]
          : [],
      ) as never,
    );
    renderList({ kind: "folder", folderId: 70 });
    expect(await screen.findByText("1 borrador todavía no se subió al servidor.")).toBeTruthy();
    expect(screen.getByText("Borrador 1")).toBeTruthy();
  });

  it("en la vista unificada de Borradores avisa de todos", async () => {
    renderList({ kind: "unified", role: "drafts" });
    expect(await screen.findByText("2 borradores todavía no se subieron al servidor.")).toBeTruthy();
  });

  it("en otras carpetas no avisa", async () => {
    renderList({ kind: "unified", role: "inbox" });
    expect(await screen.findByText("No hay correos en esta vista.")).toBeTruthy();
    expect(svc.ListPendingDrafts).not.toHaveBeenCalled();
  });
});

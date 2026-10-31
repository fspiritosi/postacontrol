import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(),
  GetSettings: vi.fn(),
  SyncFolder: vi.fn(() => Promise.resolve()),
  SyncUnified: vi.fn(() => Promise.resolve()),
}));
const openNew = vi.fn();
// jsdom no trae ResizeObserver, que usan los paneles redimensionables.
vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    unobserve() {}
    disconnect() {}
  },
);
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => ({ openNew }) }));

import type { Account, Folder } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { requestOpenMessage, resetSyncStateForTests } from "@/lib/events";
import type { View } from "@/lib/view";
import { MailShell, type ShellState } from "./MailShell";

function account(id: number): Account {
  return {
    id, email: `${id}@b.c`, displayName: "", username: "a", color: "#f00",
    imap: { host: "", port: 993, security: Security.SecurityTLS },
    smtp: { host: "", port: 465, security: Security.SecurityTLS },
    saveSentCopy: true,
  };
}

function folder(id: number, accountId: number, role: string): Folder {
  return { id, accountId, name: role, delimiter: "/", role, unread: 0, total: 0, selectable: true };
}

const FOLDERS: Record<number, Folder[]> = {
  2: [folder(21, 2, "inbox"), folder(22, 2, "sent")],
  5: [folder(51, 5, "inbox")],
};

function setup(saved: View | null, unifiedInbox: boolean) {
  if (saved) localStorage.setItem("pc:view", JSON.stringify(saved));
  vi.mocked(svc.ListFolders).mockImplementation((id: number) => Promise.resolve(FOLDERS[id]) as never);
  vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox, disableNotifications: false, threadedList: false });
  let current: ShellState | null = null;
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MailShell
        accounts={[account(5), account(2)].sort((a, b) => a.id - b.id)}
        list={(s) => {
          current = s;
          return <p data-testid="view">{JSON.stringify(s.view)}</p>;
        }}
      />
    </QueryClientProvider>,
  );
  return { state: () => current! };
}

const shown = () => JSON.parse(screen.getByTestId("view").textContent!) as View | null;

describe("MailShell", () => {
  afterEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
    resetSyncStateForTests();
  });

  it("sin vista guardada abre la Entrada de la primera cuenta y la sincroniza", async () => {
    setup(null, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
    await waitFor(() => expect(svc.SyncFolder).toHaveBeenCalledWith(21));
    expect(JSON.parse(localStorage.getItem("pc:view")!)).toEqual({ kind: "folder", folderId: 21 });
  });

  it("arranca con la vista guardada si sigue valiendo", async () => {
    setup({ kind: "folder", folderId: 51 }, false);
    await waitFor(() => expect(svc.SyncFolder).toHaveBeenCalledWith(51));
    expect(shown()).toEqual({ kind: "folder", folderId: 51 });
  });

  it("una unificada guardada con la unificada apagada vuelve a la vista por defecto", async () => {
    setup({ kind: "unified", role: "inbox" }, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
    expect(svc.SyncUnified).not.toHaveBeenCalled();
  });

  it("una carpeta guardada que ya no existe vuelve a la vista por defecto", async () => {
    setup({ kind: "folder", folderId: 999 }, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
  });

  it("al elegir una unificada la sincroniza y limpia el mensaje", async () => {
    const { state } = setup(null, true);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
    act(() => state().selectMessage(7));
    await waitFor(() => expect(state().messageId).toBe(7));
    act(() => state().selectView({ kind: "unified", role: "flagged" }));
    await waitFor(() => expect(svc.SyncUnified).toHaveBeenCalledWith("flagged"));
    expect(state().messageId).toBeNull();
    expect(shown()).toEqual({ kind: "unified", role: "flagged" });
  });

  it("abrir desde la notificación selecciona la carpeta y el correo", async () => {
    const { state } = setup(null, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
    act(() => requestOpenMessage({ folderId: 51, messageId: 8 }));
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 51 }));
    expect(state().messageId).toBe(8);
    // messageId 0 (resumen): solo la carpeta, sin correo.
    act(() => requestOpenMessage({ folderId: 22, messageId: 0 }));
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 22 }));
    expect(state().messageId).toBeNull();
  });

  it("Redactar usa la cuenta de la carpeta abierta, o la primera en una unificada", async () => {
    const user = userEvent.setup();
    const { state } = setup({ kind: "folder", folderId: 51 }, true);
    await waitFor(() => expect(svc.SyncFolder).toHaveBeenCalledWith(51));
    await user.click(screen.getByRole("button", { name: "Redactar" }));
    expect(openNew).toHaveBeenLastCalledWith(5);
    act(() => state().selectView({ kind: "unified", role: "inbox" }));
    await waitFor(() => expect(shown()).toEqual({ kind: "unified", role: "inbox" }));
    await user.click(screen.getByRole("button", { name: "Redactar" }));
    expect(openNew).toHaveBeenLastCalledWith(2);
  });
});

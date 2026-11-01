import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(),
  ListMessages: vi.fn(() => Promise.resolve([])),
  ListUnified: vi.fn(() => Promise.resolve([])),
  Search: vi.fn(),
  SearchServer: vi.fn(),
  SetFlagged: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(() =>
    Promise.resolve({ unifiedInbox: false, disableNotifications: false, threadedList: false }),
  ),
}));

import type { Account, Folder, MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { resetSyncStateForTests } from "@/lib/events";
import type { View } from "@/lib/view";
import { MessageList } from "../messages/MessageList";
import type { SearchState } from "../layout/MailShell";

const accounts: Account[] = [7].map((id) => ({
  id, email: `${id}@b.c`, displayName: "A", username: "a", color: "#ff0000",
  imap: { host: "", port: 993, security: Security.SecurityTLS },
  smtp: { host: "", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
  defaultSignatureId: 0,
}));
const FOLDERS: Folder[] = [
  { id: 3, accountId: 7, name: "INBOX", delimiter: "/", role: "inbox", unread: 0, total: 0, selectable: true },
  { id: 4, accountId: 7, name: "Facturas", delimiter: "/", role: "", unread: 0, total: 0, selectable: true },
];

function summary(id: number, folderId: number): MessageSummary {
  return {
    id, accountId: 7, folderId, uid: id,
    from: { name: `Remitente ${id}`, email: "x@y.z" },
    subject: `Asunto ${id}`, snippet: "", date: "2026-10-30T10:00:00Z",
    seen: true, flagged: false, answered: false, hasAttachments: false,
  };
}

const selectMessage = vi.fn();

// Hace de MailShell: guarda la vista y la búsqueda.
function Harness({ initial }: { initial: View }) {
  const [view, setView] = useState<View>(initial);
  const [search, setSearch] = useState<SearchState | null>(null);
  return (
    <>
      <button onClick={() => { setView({ kind: "folder", folderId: 4 }); setSearch(null); }}>otra</button>
      <MessageList
        accounts={accounts}
        state={{
          view, messageId: null, selectView: vi.fn(), selectMessage, search, setSearch,
          threadRoot: null, selectThread: vi.fn(),
        }}
      />
    </>
  );
}

function setup(initial: View = { kind: "folder", folderId: 3 }) {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.mocked(svc.ListFolders).mockResolvedValue(FOLDERS);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Harness initial={initial} />
    </QueryClientProvider>,
  );
  return user;
}

describe("buscador", () => {
  beforeEach(() => {
    resetSyncStateForTests();
    vi.mocked(svc.Search).mockReset();
    vi.mocked(svc.SearchServer).mockReset();
    selectMessage.mockReset();
  });
  afterEach(() => vi.useRealTimers());

  it("escribir busca después de la pausa y muestra resultados con su carpeta", async () => {
    vi.mocked(svc.Search).mockResolvedValue({ messages: [summary(1, 4)], pending: 0 });
    const user = setup();
    await user.type(screen.getByRole("searchbox", { name: "Buscar correo" }), "factura");
    expect(svc.Search).not.toHaveBeenCalled();
    await vi.advanceTimersByTimeAsync(300);
    expect(await screen.findByRole("button", { name: /Asunto 1/ })).toBeTruthy();
    expect(svc.Search).toHaveBeenCalledWith({ text: "factura", folderId: 0, limit: 100, offset: 0 });
    expect((await screen.findByTestId("folder-label")).textContent).toBe("Facturas");
  });

  it("Esc vuelve a la lista", async () => {
    vi.mocked(svc.Search).mockResolvedValue({ messages: [summary(1, 4)], pending: 0 });
    vi.mocked(svc.ListMessages).mockResolvedValue([summary(9, 3)]);
    const user = setup();
    const box = screen.getByRole("searchbox", { name: "Buscar correo" });
    await user.type(box, "factura");
    await vi.advanceTimersByTimeAsync(300);
    await screen.findByRole("button", { name: /Asunto 1/ });
    await user.keyboard("{Escape}");
    expect(await screen.findByRole("button", { name: /Asunto 9/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /Asunto 1/ })).toBeNull();
    expect((box as HTMLInputElement).value).toBe("");
  });

  it("Solo en esta carpeta manda folderId", async () => {
    vi.mocked(svc.Search).mockResolvedValue({ messages: [], pending: 0 });
    const user = setup();
    await user.type(screen.getByRole("searchbox", { name: "Buscar correo" }), "hola{Enter}");
    await waitFor(() => expect(svc.Search).toHaveBeenCalled());
    await user.click(await screen.findByRole("button", { name: "Solo en esta carpeta" }));
    await waitFor(() =>
      expect(svc.Search).toHaveBeenLastCalledWith({ text: "hola", folderId: 3, limit: 100, offset: 0 }),
    );
  });

  it("en vista unificada no ofrece buscar solo en la carpeta", async () => {
    vi.mocked(svc.Search).mockResolvedValue({ messages: [], pending: 0 });
    const user = setup({ kind: "unified", role: "inbox" });
    await user.type(screen.getByRole("searchbox", { name: "Buscar correo" }), "hola{Enter}");
    await screen.findByText("No encontramos correos.");
    expect(screen.queryByRole("button", { name: "Solo en esta carpeta" })).toBeNull();
  });

  it("con pendientes ofrece buscar en el servidor y suma sin duplicar", async () => {
    vi.mocked(svc.Search).mockResolvedValue({ messages: [summary(1, 3)], pending: 5 });
    vi.mocked(svc.SearchServer).mockResolvedValue([summary(1, 3), summary(2, 3)]);
    const user = setup();
    await user.type(screen.getByRole("searchbox", { name: "Buscar correo" }), "hola{Enter}");
    expect(await screen.findByText("Todavía faltan indexar 5 correos.")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Buscar en el servidor en Entrada" }));
    expect(await screen.findByRole("button", { name: /Asunto 2/ })).toBeTruthy();
    expect(svc.SearchServer).toHaveBeenCalledWith(3, "hola");
    expect(screen.getAllByRole("button", { name: /Asunto 1/ })).toHaveLength(1);
  });

  it("si las páginas se corren no repite resultados", async () => {
    vi.mocked(svc.Search).mockImplementation(({ offset }) => {
      // Entró un correo nuevo entre página y página: el 100 vuelve a aparecer.
      const ids = offset === 0 ? Array.from({ length: 100 }, (_, i) => i + 1) : [100, 101];
      return Promise.resolve({ messages: ids.map((id) => summary(id, 3)), pending: 0 }) as ReturnType<
        typeof svc.Search
      >;
    });
    const user = setup();
    await user.type(screen.getByRole("searchbox", { name: "Buscar correo" }), "hola{Enter}");
    await user.click(await screen.findByRole("button", { name: "Cargar más" }));
    expect(await screen.findByRole("button", { name: /Asunto 101(?!\d)/ })).toBeTruthy();
    expect(screen.getAllByRole("button", { name: /Asunto 100(?!\d)/ })).toHaveLength(1);
  });

  it("elegir un resultado lo abre", async () => {
    vi.mocked(svc.Search).mockResolvedValue({ messages: [summary(1, 3)], pending: 0 });
    const user = setup();
    await user.type(screen.getByRole("searchbox", { name: "Buscar correo" }), "hola{Enter}");
    await user.click(await screen.findByRole("button", { name: /Asunto 1/ }));
    expect(selectMessage).toHaveBeenCalledWith(1);
  });

  it("al cambiar de vista se vacía el campo", async () => {
    vi.mocked(svc.Search).mockResolvedValue({ messages: [summary(1, 3)], pending: 0 });
    const user = setup();
    const box = screen.getByRole("searchbox", { name: "Buscar correo" }) as HTMLInputElement;
    await user.type(box, "hola{Enter}");
    await screen.findByRole("button", { name: /Asunto 1/ });
    await user.click(screen.getByRole("button", { name: "otra" }));
    await waitFor(() => expect(box.value).toBe(""));
  });
});

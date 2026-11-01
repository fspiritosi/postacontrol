import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() => Promise.resolve({ unifiedInbox: false })),
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
  defaultSignatureId: 0,
};

const baseState: ShellState = {
  view: { kind: "folder", folderId: 3 },
  messageId: null,
  selectView: vi.fn(),
  selectMessage: vi.fn(),
  search: null,
  setSearch: vi.fn(),
  threadRoot: null,
  selectThread: vi.fn(),
  contactId: null,
  selectContact: vi.fn(),
  openContact: vi.fn(),
};

const account2: Account = { ...account, id: 8, email: "x@y.z", displayName: "X", color: "#0f0" };
const inbox = (id: number, accountId: number, unread: number) =>
  ({ id, accountId, name: "INBOX", delimiter: "/", role: "inbox", unread, total: 0, selectable: true }) as const;

// Carpeta sin rol: no choca con la fila "Entrada" de Todas las cuentas.
const proyectos = { id: 9, accountId: 7, name: "Proyectos", delimiter: "/", role: "", unread: 0, total: 0, selectable: true } as const;

function renderSidebar(accounts: Account[], state: ShellState = baseState) {
  return render(
    <QueryClientProvider client={new QueryClient()}>
      <FolderSidebar accounts={accounts} state={state} />
    </QueryClientProvider>,
  );
}

describe("FolderSidebar", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: false, disableNotifications: false, threadedList: false });
    vi.mocked(svc.ListFolders).mockResolvedValue([inbox(3, 7, 0)]);
  });

  it("muestra una sección por cuenta con su árbol", async () => {
    vi.mocked(svc.ListFolders).mockImplementation(((id: number) =>
      Promise.resolve(id === 7 ? [inbox(3, 7, 2)] : [inbox(4, 8, 0)])) as never);
    renderSidebar([account, account2]);
    expect(await screen.findByText("A")).toBeTruthy();
    expect(screen.getByText("X")).toBeTruthy();
    await waitFor(() => expect(screen.getAllByRole("button", { name: /Entrada/ })).toHaveLength(2));
  });

  it("con la bandeja unificada muestra Todas las cuentas y suma los no leídos de las Entradas", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: true, disableNotifications: false, threadedList: false });
    vi.mocked(svc.ListFolders).mockImplementation(((id: number) =>
      Promise.resolve(id === 7 ? [inbox(3, 7, 2)] : [inbox(4, 8, 5)])) as never);
    const selectView = vi.fn();
    const user = userEvent.setup();
    renderSidebar([account, account2], { ...baseState, selectView });
    expect(await screen.findByText("Todas las cuentas")).toBeTruthy();
    const row = await screen.findByRole("button", { name: /^Entrada\s*7$/ });
    await user.click(row);
    expect(selectView).toHaveBeenCalledWith({ kind: "unified", role: "inbox" });
    expect(screen.getByRole("button", { name: "Destacados" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Papelera" })).toBeNull();
  });

  it("plegar una cuenta oculta sus carpetas y se recuerda", async () => {
    vi.mocked(svc.ListFolders).mockResolvedValue([inbox(3, 7, 0)]);
    const user = userEvent.setup();
    const { unmount } = renderSidebar([account]);
    expect(await screen.findByRole("button", { name: "Entrada" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: /^A\s*$/ }));
    await waitFor(() => expect(screen.queryByRole("button", { name: "Entrada" })).toBeNull());
    expect(JSON.parse(localStorage.getItem("pc:collapsed")!)).toEqual([7]);
    unmount();
    renderSidebar([account]);
    await waitFor(() => expect(svc.ListFolders).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Entrada" })).toBeNull();
  });

  it("la fila Contactos abre la libreta y queda marcada cuando es la vista", async () => {
    const selectView = vi.fn();
    const user = userEvent.setup();
    const { unmount } = renderSidebar([account], { ...baseState, selectView });
    await user.click(await screen.findByRole("button", { name: "Contactos" }));
    expect(selectView).toHaveBeenCalledWith({ kind: "contacts" });
    expect(screen.getByRole("button", { name: "Contactos" }).getAttribute("aria-current")).toBeNull();
    unmount();
    renderSidebar([account], { ...baseState, view: { kind: "contacts" } });
    expect((await screen.findByRole("button", { name: "Contactos" })).getAttribute("aria-current")).toBe("true");
  });

  it("sin la bandeja unificada no muestra Todas las cuentas", async () => {
    renderSidebar([account]);
    expect(await screen.findByRole("button", { name: "Entrada" })).toBeTruthy();
    expect(screen.queryByText("Todas las cuentas")).toBeNull();
  });

  it("con la unificada y sin estado guardado, las secciones arrancan plegadas", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: true, disableNotifications: false, threadedList: false });
    vi.mocked(svc.ListFolders).mockResolvedValue([proyectos]);
    renderSidebar([account]);
    expect(await screen.findByText("Todas las cuentas")).toBeTruthy();
    await waitFor(() => expect(svc.ListFolders).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 50)); // deja que la carpeta llegue y se renderice, si pudiera
    expect(screen.queryByRole("button", { name: "Proyectos" })).toBeNull();
  });

  it("con la unificada, una sección expandida a mano (pc:expanded) arranca abierta", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: true, disableNotifications: false, threadedList: false });
    localStorage.setItem("pc:expanded", JSON.stringify([7]));
    vi.mocked(svc.ListFolders).mockResolvedValue([proyectos]);
    renderSidebar([account]);
    expect(await screen.findByText("Todas las cuentas")).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Proyectos" })).toBeTruthy();
  });

  it("sin la unificada, lo plegado guardado (pc:collapsed) gana", async () => {
    localStorage.setItem("pc:collapsed", JSON.stringify([7]));
    vi.mocked(svc.ListFolders).mockResolvedValue([proyectos]);
    renderSidebar([account]);
    await waitFor(() => expect(svc.ListFolders).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 50)); // deja que la carpeta llegue y se renderice, si pudiera
    expect(screen.queryByRole("button", { name: "Proyectos" })).toBeNull();
  });

  it("Agregar cuenta abre el alta", async () => {
    const user = userEvent.setup();
    renderSidebar([account]);
    await user.click(screen.getByRole("button", { name: "Agregar cuenta" }));
    expect(await screen.findByRole("dialog")).toBeTruthy();
  });

  it("muestra la Bandeja de salida solo si hay pendientes, y la abre", async () => {
    vi.mocked(svc.ListLocalDrafts).mockResolvedValueOnce([
      { id: 1, status: "failed" },
      { id: 2, status: "sending" },
    ] as never);
    const user = userEvent.setup();
    const selectView = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <FolderSidebar accounts={[account]} state={{ ...baseState, selectView }} />
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
        <FolderSidebar accounts={[account]} state={{ ...baseState, selectView }} />
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
        <FolderSidebar accounts={[account]} state={baseState} />
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
          accounts={[account]}
          state={{ ...baseState, messageId: 9, selectMessage }}
        />
      </QueryClientProvider>,
    );
    await user.click(screen.getByLabelText("Opciones de A"));
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
          accounts={[account]}
          state={baseState}
        />
      </QueryClientProvider>,
    );
    await user.click(screen.getByLabelText("Opciones de A"));
    const item = await screen.findByRole("menuitemcheckbox", { name: "Guardar copia en Enviados" });
    expect(item.getAttribute("aria-checked")).toBe("true");
    await user.click(item);
    await waitFor(() => expect(svc.SetSaveSentCopy).toHaveBeenCalledWith(7, false));
  });
});

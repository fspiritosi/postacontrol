import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() => Promise.resolve({ unifiedInbox: false })),
  ListFolders: vi.fn(() => Promise.resolve([])),
  CreateFolder: vi.fn(() => Promise.resolve({})),
  RenameFolder: vi.fn(() => Promise.resolve({})),
  MoveFolder: vi.fn(() => Promise.resolve({})),
  SetFolderOrder: vi.fn(() => Promise.resolve()),
  DeleteFolderPreview: vi.fn(() => Promise.resolve({ total: 0, subfolders: [] })),
  DeleteFolder: vi.fn(() => Promise.resolve(0)),
  MoveMessages: vi.fn(() => Promise.resolve()),
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
import { setMessageDrag } from "@/lib/dnd";
import { FakeDataTransfer } from "../../test/dataTransfer";
import { toast } from "sonner";
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
  marked: { keys: [], anchor: null }, markedThreads: { keys: [], anchor: null },
  setMarked: vi.fn(), setMarkedThreads: vi.fn(), clearMarks: vi.fn(),
};

const account2: Account = { ...account, id: 8, email: "x@y.z", displayName: "X", color: "#0f0" };
const inbox = (id: number, accountId: number, unread: number) =>
  ({ id, accountId, name: "INBOX", delimiter: "/", role: "inbox", unread, total: 0, selectable: true, position: 0 }) as const;

// Carpeta sin rol: no choca con la fila "Entrada" de Todas las cuentas.
const proyectos = { id: 9, accountId: 7, name: "Proyectos", delimiter: "/", role: "", unread: 0, total: 0, selectable: true, position: 0 } as const;

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
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: false, disableNotifications: false, threadedList: false, readingPane: "" });
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
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: true, disableNotifications: false, threadedList: false, readingPane: "" });
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
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: true, disableNotifications: false, threadedList: false, readingPane: "" });
    vi.mocked(svc.ListFolders).mockResolvedValue([proyectos]);
    renderSidebar([account]);
    expect(await screen.findByText("Todas las cuentas")).toBeTruthy();
    await waitFor(() => expect(svc.ListFolders).toHaveBeenCalled());
    await new Promise((r) => setTimeout(r, 50)); // deja que la carpeta llegue y se renderice, si pudiera
    expect(screen.queryByRole("button", { name: "Proyectos" })).toBeNull();
  });

  it("con la unificada, una sección expandida a mano (pc:expanded) arranca abierta", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox: true, disableNotifications: false, threadedList: false, readingPane: "" });
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
      { id: 3, accountId: 7, name: "INBOX", delimiter: "/", role: "inbox", unread: 0, total: 0, selectable: true, position: 0 },
      { id: 4, accountId: 7, name: "Sent", delimiter: "/", role: "sent", unread: 0, total: 0, selectable: true, position: 0 },
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

  describe("soltar correos sobre una carpeta", () => {
    const drag = (accountId: number, payload: Parameters<typeof setMessageDrag>[2]) => {
      const dt = new FakeDataTransfer();
      setMessageDrag(dt as unknown as DataTransfer, accountId, payload);
      return dt;
    };
    beforeEach(() => {
      vi.mocked(svc.MoveMessages).mockClear();
      vi.mocked(toast.success).mockClear();
      vi.mocked(svc.ListFolders).mockImplementation(((id: number) =>
        Promise.resolve(id === 7 ? [inbox(3, 7, 0), proyectos] : [inbox(4, 8, 0)])) as never);
    });

    it("soltar correos en una carpeta los mueve", async () => {
      renderSidebar([account]);
      const row = await screen.findByRole("button", { name: "Proyectos" });
      const dt = drag(7, { ids: [1, 2], folderId: 3 });
      expect(fireEvent.dragOver(row, { dataTransfer: dt })).toBe(false);
      expect(row.className).toContain("outline-primary");
      fireEvent.drop(row, { dataTransfer: dt });
      await waitFor(() => expect(svc.MoveMessages).toHaveBeenCalledWith([1, 2], 9));
      await waitFor(() => expect(toast.success).toHaveBeenCalledWith("2 movidos a Proyectos"));
      expect(row.className).not.toContain("outline-primary");
    });

    it("soltar en una carpeta de otra cuenta no hace nada", async () => {
      renderSidebar([account, account2]);
      const others = await screen.findAllByRole("button", { name: /Entrada/ });
      const dt = drag(7, { ids: [1], folderId: 9 });
      const target = others[others.length - 1]; // la de la cuenta 8
      expect(fireEvent.dragOver(target, { dataTransfer: dt })).toBe(true);
      fireEvent.drop(target, { dataTransfer: dt });
      expect(svc.MoveMessages).not.toHaveBeenCalled();
    });

    it("soltar en la misma carpeta no hace nada", async () => {
      renderSidebar([account]);
      const row = await screen.findByRole("button", { name: "Proyectos" });
      const dt = drag(7, { ids: [1], folderId: 9 });
      fireEvent.dragOver(row, { dataTransfer: dt });
      fireEvent.drop(row, { dataTransfer: dt });
      expect(svc.MoveMessages).not.toHaveBeenCalled();
      // La carpeta que se está mirando tampoco es destino.
      const inboxRow = await screen.findByRole("button", { name: "Entrada" });
      expect(fireEvent.dragOver(inboxRow, { dataTransfer: drag(7, { ids: [1] }) })).toBe(true);
    });
  });

  describe("gestión de carpetas", () => {
    const folder = (id: number, name: string, position = 0) =>
      ({ id, accountId: 7, name, delimiter: "/", role: "", unread: 0, total: 0, selectable: true, position }) as const;

    beforeAll(() => {
      // Radix pide estas APIs y jsdom no las trae.
      Element.prototype.hasPointerCapture = () => false;
      Element.prototype.setPointerCapture = () => {};
      Element.prototype.releasePointerCapture = () => {};
      Element.prototype.scrollIntoView = () => {};
    });

    beforeEach(() => {
      vi.mocked(svc.ListFolders).mockResolvedValue([
        inbox(3, 7, 0),
        folder(10, "Clientes"),
        folder(11, "Clientes/Acme"),
        folder(12, "Zeta"),
      ] as never);
    });

    const menuOf = async (name: string) => {
      fireEvent.contextMenu(await screen.findByRole("button", { name }));
      return screen.findByRole("menu");
    };
    const item = (name: string) => screen.getByRole("menuitem", { name });

    it("Nueva carpeta… desde el menú de la cuenta crea en primer nivel", async () => {
      const user = userEvent.setup();
      renderSidebar([account]);
      await user.click(await screen.findByRole("button", { name: "Opciones de A" }));
      await user.click(await screen.findByRole("menuitem", { name: "Nueva carpeta…" }));
      await user.type(await screen.findByLabelText("Nombre"), "Clientes");
      await user.click(screen.getByRole("button", { name: "Crear" }));
      await waitFor(() => expect(svc.CreateFolder).toHaveBeenCalledWith(7, 0, "Clientes"));
      expect(toast.success).toHaveBeenCalledWith("Carpeta «Clientes» creada");
    });

    it("Nueva subcarpeta… usa el id del padre", async () => {
      const user = userEvent.setup();
      renderSidebar([account]);
      await menuOf("Clientes");
      await user.click(item("Nueva subcarpeta…"));
      expect(await screen.findByText("Nueva subcarpeta de «Clientes»")).toBeTruthy();
      await user.type(screen.getByLabelText("Nombre"), "Beta");
      await user.click(screen.getByRole("button", { name: "Crear" }));
      await waitFor(() => expect(svc.CreateFolder).toHaveBeenCalledWith(7, 10, "Beta"));
    });

    it("Renombrar… viene con el nombre actual", async () => {
      const user = userEvent.setup();
      renderSidebar([account]);
      await menuOf("Zeta");
      await user.click(item("Renombrar…"));
      const input = (await screen.findByLabelText("Nombre")) as HTMLInputElement;
      expect(input.value).toBe("Zeta");
      await user.clear(input);
      await user.type(input, "Omega");
      await user.click(screen.getByRole("button", { name: "Renombrar" }));
      await waitFor(() => expect(svc.RenameFolder).toHaveBeenCalledWith(12, "Omega"));
    });

    it("Mover a… ofrece Primer nivel y los destinos válidos", async () => {
      const user = userEvent.setup();
      renderSidebar([account]);
      await menuOf("Acme");
      await user.click(item("Mover a…"));
      expect(await screen.findByText("Mover «Acme»")).toBeTruthy();
      const radios = screen.getAllByRole("radio");
      const labels = radios.map((r) => r.closest("label")!.textContent);
      expect(labels).toContain("Primer nivel");
      expect(labels).toContain("Zeta");
      expect(labels).not.toContain("Clientes"); // su padre actual
      expect(labels).not.toContain("Acme");
      await user.click(screen.getByLabelText("Zeta"));
      await user.click(screen.getByRole("button", { name: "Mover" }));
      await waitFor(() => expect(svc.MoveFolder).toHaveBeenCalledWith(11, 12));
    });

    it("Subir y Bajar guardan el orden de los hermanos; Subir está deshabilitado en el primero", async () => {
      const user = userEvent.setup();
      renderSidebar([account]);
      await menuOf("Zeta");
      expect(item("Bajar").getAttribute("aria-disabled")).toBe("true");
      await user.click(item("Subir"));
      // Raíz: Entrada, Clientes, Zeta -> Zeta sube un lugar.
      await waitFor(() => expect(svc.SetFolderOrder).toHaveBeenCalledWith(7, [3, 12, 10]));
      await user.keyboard("{Escape}");
      await menuOf("Entrada");
      expect(item("Subir").getAttribute("aria-disabled")).toBe("true");
    });

    it("en una especial solo hay Subir y Bajar habilitados", async () => {
      renderSidebar([account]);
      await menuOf("Entrada");
      for (const n of ["Nueva subcarpeta…", "Renombrar…", "Mover a…", "Eliminar…"])
        expect(item(n).getAttribute("aria-disabled")).toBe("true");
      expect(item("Bajar").getAttribute("aria-disabled")).toBeNull();
    });

    it("Eliminar… muestra los conteos, elimina y avisa", async () => {
      vi.mocked(svc.DeleteFolderPreview).mockResolvedValue({ total: 48, subfolders: [{ name: "Acme", total: 7 }] });
      vi.mocked(svc.DeleteFolder).mockResolvedValue(55);
      const user = userEvent.setup();
      renderSidebar([account]);
      await menuOf("Clientes");
      await user.click(item("Eliminar…"));
      expect(await screen.findByText("¿Eliminar «Clientes»?")).toBeTruthy();
      expect(await screen.findByText(/Los 55 correos van a la Papelera/)).toBeTruthy();
      await user.click(screen.getByRole("button", { name: "Eliminar" }));
      await waitFor(() => expect(svc.DeleteFolder).toHaveBeenCalledWith(10));
      expect(toast.success).toHaveBeenCalledWith("Carpeta eliminada; 55 correos a la Papelera");
    });
  });
});

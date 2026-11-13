import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(() => Promise.resolve([])),
  ListMessages: vi.fn(() => Promise.resolve([])),
  ListUnified: vi.fn(() => Promise.resolve([])),
  SyncFolder: vi.fn(() => new Promise(() => {})), // nunca termina
  SetFlagged: vi.fn(() => Promise.resolve()),
  SetLabel: vi.fn(() => Promise.resolve()),
  ListLabels: vi.fn(() =>
    Promise.resolve([{ id: 11, name: "Clientes", color: "#dc2626", position: 0 }]),
  ),
  GetSettings: vi.fn(() =>
    Promise.resolve({
      unifiedInbox: false,
      background: "",
      disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "",
      threadedList: false,
      readingPane: "",
    }),
  ),
  SetSettings: vi.fn(() => Promise.resolve()),
  ListThreads: vi.fn(() => Promise.resolve([])),
  ListLabeled: vi.fn(() => Promise.resolve([])),
  RefreshLabel: vi.fn(() => Promise.resolve()),
  ListPendingDrafts: vi.fn(() => Promise.resolve([])),
  EmptyTrash: vi.fn(() => Promise.resolve(2)),
}));
vi.mock("../compose/ComposerContext", () => ({
  useComposer: () => ({ openComposeDraft: vi.fn() }),
}));

import type {
  Account,
  MessageSummary,
} from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { relativeDate } from "@/lib/format";
import { resetSyncStateForTests, syncFolder } from "@/lib/events";
import type { View } from "@/lib/view";
import { NO_MARKS, type Marks } from "@/lib/selection";
import type { ShellState } from "../layout/MailShell";
import { MessageList } from "./MessageList";

function account(id: number, color: string): Account {
  return {
    id,
    email: `${id}@b.c`,
    displayName: "A",
    username: "a",
    color,
    imap: { host: "", port: 993, security: Security.SecurityTLS },
    smtp: { host: "", port: 465, security: Security.SecurityTLS },
    saveSentCopy: true,
    defaultSignatureId: 0,
  };
}

const accounts = [account(7, "#ff0000"), account(8, "#00ff00")];

function summary(id: number, accountId: number): MessageSummary {
  return {
    id,
    accountId,
    folderId: accountId * 10,
    uid: id,
    from: { name: `Remitente ${id}`, email: "x@y.z" },
    subject: `Asunto ${id}`,
    snippet: "",
    date: "2026-10-30T10:00:00Z",
    seen: true,
    flagged: false,
    answered: false,
    hasAttachments: false,
    labels: [],
  };
}

function renderList(
  view: View,
  selectMessage = vi.fn(),
  extra: Partial<ShellState> = {},
) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MessageList
        accounts={accounts}
        state={{
          view,
          messageId: null,
          selectView: vi.fn(),
          selectMessage,
          search: null,
          setSearch: vi.fn(),
          threadRoot: null,
          selectThread: vi.fn(),
          contactId: null,
          selectContact: vi.fn(),
          openContact: vi.fn(),
          marked: { keys: [], anchor: null },
          markedThreads: { keys: [], anchor: null },
          setMarked: vi.fn(),
          setMarkedThreads: vi.fn(),
          clearMarks: vi.fn(),
          labelFilter: null,
          setLabelFilter: vi.fn(),
          ...extra,
        }}
      />
    </QueryClientProvider>,
  );
}

describe("MessageList", () => {
  beforeEach(() => resetSyncStateForTests());

  it("con la carpeta vacía y sin sincronizar, dice que no hay correos", async () => {
    renderList({ kind: "folder", folderId: 3 });
    expect(
      await screen.findByText("No hay correos en esta carpeta."),
    ).toBeTruthy();
  });

  it("mientras la carpeta se sincroniza y está vacía, muestra Sincronizando…", async () => {
    renderList({ kind: "folder", folderId: 3 });
    act(() => void syncFolder(7, 3));
    expect(await screen.findByText("Sincronizando…")).toBeTruthy();
    expect(screen.queryByText("No hay correos en esta carpeta.")).toBeNull();
  });

  it("en vista unificada muestra el color de la cuenta de cada mensaje y el título Entrada", async () => {
    vi.mocked(svc.ListUnified).mockResolvedValueOnce([
      summary(1, 7),
      summary(2, 8),
    ]);
    renderList({ kind: "unified", role: "inbox" });
    expect(screen.getByRole("heading", { name: "Entrada" })).toBeTruthy();
    const first = await screen.findByRole("button", { name: /Asunto 1/ });
    const second = screen.getByRole("button", { name: /Asunto 2/ });
    const dot = (el: HTMLElement) =>
      (el.querySelector("[aria-hidden]") as HTMLElement).style.backgroundColor;
    expect(dot(first)).toBe("rgb(255, 0, 0)");
    expect(dot(second)).toBe("rgb(0, 255, 0)");
    expect(svc.ListUnified).toHaveBeenCalledWith("inbox", {
      unreadOnly: false,
      offset: 0,
      limit: 100,
    });
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

describe("MessageList posición del panel", () => {
  beforeEach(() => resetSyncStateForTests());

  const withPane = (readingPane: string) =>
    vi.mocked(svc.GetSettings).mockResolvedValue({
      unifiedInbox: false,
      background: "",
      disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "",
      threadedList: false,
      readingPane,
    });

  it("con el lector abajo, las filas son de un renglón", async () => {
    withPane("bottom");
    vi.mocked(svc.ListUnified).mockResolvedValueOnce([summary(1, 7)]);
    renderList({ kind: "unified", role: "inbox" });
    const star = await screen.findByRole("button", { name: "Destacar" });
    const li = star.closest("li")!;
    await waitFor(() =>
      expect(li.getAttribute("data-density")).toBe("compact"),
    );
    expect(li.textContent).toContain("Remitente 1");
    expect(li.textContent).toContain("Asunto 1");
    expect(li.textContent).toContain(relativeDate("2026-10-30T10:00:00Z"));
    fireEvent.click(star);
    expect(svc.SetFlagged).toHaveBeenCalledWith([1], true);
  });

  it("con el lector a la derecha, las filas son de tres renglones", async () => {
    withPane("right");
    vi.mocked(svc.ListUnified).mockResolvedValueOnce([summary(1, 7)]);
    renderList({ kind: "unified", role: "inbox" });
    const star = await screen.findByRole("button", { name: "Destacar" });
    await waitFor(() =>
      expect(star.closest("li")!.getAttribute("data-density")).toBe(
        "comfortable",
      ),
    );
  });
});

describe("MessageList conversaciones", () => {
  beforeEach(() => {
    resetSyncStateForTests();
    vi.clearAllMocks();
  });

  it("el interruptor guarda threadedList y muestra hilos", async () => {
    const settings = {
      unifiedInbox: true,
      background: "",
      disableNotifications: true, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "",
      threadedList: false,
      readingPane: "",
    };
    vi.mocked(svc.GetSettings).mockResolvedValue(settings);
    vi.mocked(svc.ListThreads).mockResolvedValue([
      {
        threadRoot: "<r@x>",
        latest: summary(1, 7),
        count: 2,
        unread: 0,
        participants: ["Ana", "Beto"],
        labels: [],
      },
    ]);
    renderList({ kind: "folder", folderId: 3 });
    const sw = await screen.findByRole("switch", { name: "Conversaciones" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    vi.mocked(svc.GetSettings).mockResolvedValue({
      ...settings,
      threadedList: true,
    });
    fireEvent.click(sw);
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({
        ...settings,
        threadedList: true,
      }),
    );
    expect(await screen.findByText("Ana, Beto")).toBeTruthy();
    expect(svc.ListThreads).toHaveBeenCalledWith(3, {
      unreadOnly: false,
      offset: 0,
      limit: 100,
    });
  });

  it("en vista unificada no aparece el interruptor", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValue({
      unifiedInbox: true,
      background: "",
      disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "",
      threadedList: true,
      readingPane: "",
    });
    renderList({ kind: "unified", role: "inbox" });
    expect(
      await screen.findByText("No hay correos en esta vista."),
    ).toBeTruthy();
    expect(screen.queryByRole("switch", { name: "Conversaciones" })).toBeNull();
    expect(svc.ListThreads).not.toHaveBeenCalled();
  });
});

describe("MessageList borradores sin subir", () => {
  const pending = (id: number, accountId: number) => ({
    id,
    messageId: "",
    needsUpload: true,
    uploadError: "",
    updatedAt: "",
    request: { accountId, subject: `Borrador ${id}` },
  });

  beforeEach(() => {
    resetSyncStateForTests();
    vi.clearAllMocks();
    vi.mocked(svc.ListPendingDrafts).mockResolvedValue([
      pending(1, 7),
      pending(2, 8),
    ] as never);
  });

  it("en la carpeta Borradores avisa de los pendientes de esa cuenta", async () => {
    vi.mocked(svc.ListFolders).mockImplementation(
      (accountId: number) =>
        Promise.resolve(
          accountId === 7
            ? [
                {
                  id: 70,
                  accountId: 7,
                  name: "Drafts",
                  delimiter: "/",
                  role: "drafts",
                  unread: 0,
                  total: 0,
                  selectable: true,
                },
              ]
            : [],
        ) as never,
    );
    renderList({ kind: "folder", folderId: 70 });
    expect(
      await screen.findByText("1 borrador todavía no se subió al servidor."),
    ).toBeTruthy();
    expect(screen.getByText("Borrador 1")).toBeTruthy();
  });

  it("en la vista unificada de Borradores avisa de todos", async () => {
    renderList({ kind: "unified", role: "drafts" });
    expect(
      await screen.findByText(
        "2 borradores todavía no se subieron al servidor.",
      ),
    ).toBeTruthy();
  });

  it("en otras carpetas no avisa", async () => {
    renderList({ kind: "unified", role: "inbox" });
    expect(
      await screen.findByText("No hay correos en esta vista."),
    ).toBeTruthy();
    expect(svc.ListPendingDrafts).not.toHaveBeenCalled();
  });
});

// Hace de MailShell para las marcas: abrir algo suelta la selección.
function Stateful({ view, open = null }: { view: View; open?: number | null }) {
  const [marked, setMarked] = useState<Marks<number>>(NO_MARKS);
  const [markedThreads, setMarkedThreads] = useState<Marks<string>>(NO_MARKS);
  const [messageId, setMessageId] = useState<number | null>(open);
  const [threadRoot, setThreadRoot] = useState<string | null>(null);
  const clearMarks = () => {
    setMarked(NO_MARKS);
    setMarkedThreads(NO_MARKS);
  };
  const state: ShellState = {
    view,
    messageId,
    selectView: vi.fn(),
    search: null,
    setSearch: vi.fn(),
    selectMessage: (id) => {
      setMessageId(id);
      setThreadRoot(null);
      clearMarks();
    },
    threadRoot,
    selectThread: (r) => {
      setThreadRoot(r);
      setMessageId(null);
      clearMarks();
    },
    contactId: null,
    selectContact: vi.fn(),
    openContact: vi.fn(),
    marked,
    markedThreads,
    setMarked,
    setMarkedThreads,
    clearMarks,
    labelFilter: null,
    setLabelFilter: vi.fn(),
  };
  return (
    <>
      <p data-testid="open">{messageId ?? ""}</p>
      <MessageList accounts={accounts} state={state} />
    </>
  );
}

function renderStateful(view: View, open: number | null = null) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <Stateful view={view} open={open} />
    </QueryClientProvider>,
  );
}

const inbox: View = { kind: "unified", role: "inbox" };
const five = () => [1, 2, 3, 4, 5].map((i) => summary(i, 7));
const row = (n: number) =>
  screen.getByRole("button", { name: new RegExp(`Remitente ${n}Asunto`) });
const boxOf = (n: number) =>
  row(n)
    .closest("li")!
    .querySelector('input[type="checkbox"]') as HTMLInputElement;
const openId = () => screen.getByTestId("open").textContent;

describe("MessageList selección múltiple", () => {
  beforeEach(() => {
    resetSyncStateForTests();
    vi.clearAllMocks();
    vi.mocked(svc.ListUnified).mockResolvedValue(five());
  });

  it("Ctrl + clic marca y la barra muestra la cantidad", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    expect(
      screen.queryByRole("button", { name: "Quitar selección" }),
    ).toBeNull();
    fireEvent.click(row(3), { ctrlKey: true });
    expect(screen.getByText("2 seleccionados")).toBeTruthy();
    expect(boxOf(1).checked).toBe(true);
    expect(boxOf(3).checked).toBe(true);
    expect(boxOf(2).checked).toBe(false);
    expect(row(3).closest("li")!.className).toContain("bg-primary/10");
  });

  it("Cmd + clic también marca", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(row(3), { metaKey: true });
    expect(screen.getByText("2 seleccionados")).toBeTruthy();
  });

  it("Shift + clic marca el rango", async () => {
    renderStateful(inbox, 2);
    await screen.findByRole("button", { name: /Asunto 4/ });
    fireEvent.click(row(4), { shiftKey: true });
    expect(screen.getByText("3 seleccionados")).toBeTruthy();
    expect([1, 2, 3, 4, 5].map((n) => boxOf(n).checked)).toEqual([
      false,
      true,
      true,
      true,
      false,
    ]);
  });

  it("Ctrl + A marca todos los cargados, con el foco en la lista", async () => {
    renderStateful(inbox);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.keyDown(row(3), { key: "a", ctrlKey: true });
    expect(screen.getByText("5 seleccionados")).toBeTruthy();
  });

  it("la casilla marca sin abrir", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(boxOf(3));
    expect(screen.getByText("2 seleccionados")).toBeTruthy();
    expect(openId()).toBe("1");
  });

  it("la primera casilla, sin nada abierto, marca sin abrir ni leer en el servidor", async () => {
    renderStateful(inbox);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(boxOf(3));
    expect(openId()).toBe("");
    expect(boxOf(3).checked).toBe(true);
    expect(boxOf(2).className).not.toContain("opacity-0");
    expect(
      screen.queryByRole("button", { name: "Quitar selección" }),
    ).toBeNull();
    fireEvent.click(boxOf(4));
    expect(screen.getByText("2 seleccionados")).toBeTruthy();
    expect(openId()).toBe("");
    fireEvent.click(boxOf(3));
    expect(openId()).toBe("4");
    expect(
      screen.queryByRole("button", { name: "Quitar selección" }),
    ).toBeNull();
  });

  it("la casilla del correo abierto lo marca y el lector lo sigue mostrando", async () => {
    renderStateful(inbox, 2);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(boxOf(2));
    expect(boxOf(2).checked).toBe(true);
    expect(openId()).toBe("2");
  });

  it("cambiar el filtro Todos / No leídos suelta la selección", async () => {
    const clearMarks = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MessageList
          accounts={accounts}
          state={{
            view: inbox,
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
            marked: NO_MARKS,
            markedThreads: NO_MARKS,
            setMarked: vi.fn(),
            setMarkedThreads: vi.fn(),
            clearMarks,
            labelFilter: null,
            setLabelFilter: vi.fn(),
          }}
        />
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: "No leídos" }));
    expect(clearMarks).toHaveBeenCalled();
  });

  it("la casilla está en el DOM, oculta hasta pasar el mouse, y a la vista si hay algo marcado", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    expect(boxOf(3).className).toContain("opacity-0");
    expect(boxOf(3).className).toContain("group-hover:opacity-100");
    fireEvent.click(boxOf(3));
    expect(boxOf(2).className).not.toContain("opacity-0");
  });

  it("un clic común abre y suelta la selección", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(row(3), { ctrlKey: true });
    expect(screen.getByText("2 seleccionados")).toBeTruthy();
    fireEvent.click(row(5));
    expect(openId()).toBe("5");
    expect(screen.queryByText("2 seleccionados")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Quitar selección" }),
    ).toBeNull();
    expect(boxOf(1).checked).toBe(false);
  });

  it("Esc suelta la selección, con el foco dentro de la lista", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(row(3), { ctrlKey: true });
    fireEvent.keyDown(row(3), { key: "Escape" });
    expect(screen.queryByText("2 seleccionados")).toBeNull();
  });

  it("la ✕ de la barra suelta la selección", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(row(3), { ctrlKey: true });
    fireEvent.click(screen.getByRole("button", { name: "Quitar selección" }));
    expect(screen.queryByText("2 seleccionados")).toBeNull();
  });

  it("un solo marcado (al sacar uno de dos) abre ese correo y no hay barra", async () => {
    renderStateful(inbox, 1);
    await screen.findByRole("button", { name: /Asunto 3/ });
    fireEvent.click(row(3), { ctrlKey: true });
    fireEvent.click(boxOf(1));
    expect(openId()).toBe("3");
    expect(
      screen.queryByRole("button", { name: "Quitar selección" }),
    ).toBeNull();
    expect(screen.queryByText(/seleccionados/)).toBeNull();
  });

  it("prender Conversaciones suelta la selección", async () => {
    const settings = {
      unifiedInbox: false,
      background: "",
      disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "",
      threadedList: false,
      readingPane: "",
    };
    vi.mocked(svc.GetSettings).mockResolvedValue(settings);
    vi.mocked(svc.ListMessages).mockResolvedValue(five());
    const clearMarks = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MessageList
          accounts={accounts}
          state={{
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
            marked: NO_MARKS,
            markedThreads: NO_MARKS,
            setMarked: vi.fn(),
            setMarkedThreads: vi.fn(),
            clearMarks,
            labelFilter: null,
            setLabelFilter: vi.fn(),
          }}
        />
      </QueryClientProvider>,
    );
    const sw = await screen.findByRole("switch", { name: "Conversaciones" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    fireEvent.click(sw);
    await waitFor(() => expect(clearMarks).toHaveBeenCalled());
  });
});

describe("MessageList no leídos", () => {
  beforeEach(() => {
    resetSyncStateForTests();
    vi.clearAllMocks();
  });

  const unreadPair = () => [
    { ...summary(1, 7), seen: false },
    { ...summary(2, 7), seen: true },
  ];

  it.each(["bottom", "right"])(
    "una fila no leída tiene fondo y remitente resaltados (%s)",
    async (readingPane) => {
      vi.mocked(svc.GetSettings).mockResolvedValue({
        unifiedInbox: false,
        background: "",
        disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "",
        threadedList: false,
        readingPane,
      });
      vi.mocked(svc.ListUnified).mockResolvedValue(unreadPair());
      renderStateful(inbox);
      const unread = await screen.findByRole("button", { name: /Asunto 1/ });
      const read = screen.getByRole("button", { name: /Asunto 2/ });
      expect(unread.closest("li")!.getAttribute("data-unread")).toBe("true");
      expect(unread.className).toContain("bg-unread");
      expect(screen.getByText("Remitente 1").className).toContain(
        "text-unread-foreground",
      );
      expect(read.closest("li")!.hasAttribute("data-unread")).toBe(false);
      expect(read.className).not.toContain("bg-unread");
      expect(screen.getByText("Remitente 2").className).not.toContain(
        "text-unread-foreground",
      );
    },
  );

  it("abierto o marcado gana el fondo, pero el remitente sigue azul", async () => {
    vi.mocked(svc.ListUnified).mockResolvedValue([
      { ...summary(1, 7), seen: false },
      { ...summary(2, 7), seen: false },
      { ...summary(3, 7), seen: false },
    ]);
    renderStateful(inbox, 1);
    const opened = await screen.findByRole("button", { name: /Asunto 1/ });
    expect(opened.getAttribute("aria-current")).toBe("true");
    expect(opened.className).toContain("bg-accent");
    expect(opened.className).not.toContain("bg-unread");
    expect(screen.getByText("Remitente 1").className).toContain(
      "text-unread-foreground",
    );

    const marked = screen.getByRole("button", { name: /Asunto 3/ });
    fireEvent.click(marked, { ctrlKey: true });
    expect(marked.closest("li")!.className).toContain("bg-primary/10");
    expect(marked.className).not.toContain("bg-unread");
    expect(screen.getByText("Remitente 3").className).toContain(
      "text-unread-foreground",
    );
  });

  describe("clic derecho", () => {
    function renderMarked(marked: number[], messageId: number | null) {
      vi.mocked(svc.ListMessages).mockResolvedValue([
        summary(1, 7),
        summary(2, 7),
        summary(3, 7),
      ] as never);
      const setMarked = vi.fn();
      const selectMessage = vi.fn();
      render(
        <QueryClientProvider client={new QueryClient()}>
          <MessageList
            accounts={accounts}
            state={
              {
                view: { kind: "folder", folderId: 70 },
                messageId,
                selectView: vi.fn(),
                selectMessage,
                search: null,
                setSearch: vi.fn(),
                threadRoot: null,
                selectThread: vi.fn(),
                contactId: null,
                selectContact: vi.fn(),
                openContact: vi.fn(),
                marked: { keys: marked, anchor: null },
                markedThreads: { keys: [], anchor: null },
                setMarked,
                setMarkedThreads: vi.fn(),
                clearMarks: vi.fn(),
                labelFilter: null,
                setLabelFilter: vi.fn(),
              } as ShellState
            }
          />
        </QueryClientProvider>,
      );
      return { setMarked, selectMessage };
    }

    async function pickLabel(subject: string) {
      fireEvent.contextMenu(await screen.findByText(subject));
      const sub = await screen.findByRole("menuitem", { name: "Etiquetas" });
      await waitFor(() =>
        expect(
          screen.getByRole("menuitem", { name: "Destacar" }).getAttribute("data-disabled"),
        ).toBeNull(),
      );
      fireEvent.keyDown(sub, { key: "ArrowRight" });
      fireEvent.click(await screen.findByRole("menuitem", { name: /Clientes/ }));
    }

    it("sobre una fila marcada actúa sobre todas las marcadas", async () => {
      const { setMarked, selectMessage } = renderMarked([1, 2], 3);
      await pickLabel("Asunto 1");
      await waitFor(() =>
        expect(svc.SetLabel).toHaveBeenCalledWith([1, 2], 11, true),
      );
      expect(setMarked).not.toHaveBeenCalled();
      expect(selectMessage).not.toHaveBeenCalled();
    });

    it("sobre una fila sin marcar actúa solo sobre ella", async () => {
      const { setMarked, selectMessage } = renderMarked([1, 2], 3);
      await pickLabel("Asunto 3");
      await waitFor(() =>
        expect(svc.SetLabel).toHaveBeenCalledWith([3], 11, true),
      );
      expect(setMarked).not.toHaveBeenCalled();
      expect(selectMessage).not.toHaveBeenCalled();
    });
  });
});

describe("MessageList filtro por etiqueta", () => {
  beforeEach(() => {
    resetSyncStateForTests();
    vi.mocked(svc.ListLabeled).mockClear();
    vi.mocked(svc.RefreshLabel).mockClear();
    vi.mocked(svc.ListMessages).mockClear();
    vi.mocked(svc.ListFolders).mockImplementation(
      (id: number) =>
        Promise.resolve([
          {
            id: id * 10,
            accountId: id,
            name: "Facturas",
            delimiter: "/",
            role: "",
            unread: 0,
            total: 0,
            selectable: true,
            position: 0,
          },
        ]) as never,
    );
  });

  it("en una carpeta lista por etiqueta con la cuenta de la carpeta, muestra la carpeta de cada fila y refresca una vez", async () => {
    vi.mocked(svc.ListLabeled).mockResolvedValueOnce([summary(1, 7)]);
    renderList({ kind: "folder", folderId: 70 }, vi.fn(), { labelFilter: 11 });
    expect(await screen.findByRole("button", { name: /Asunto 1/ })).toBeTruthy();
    expect(svc.ListLabeled).toHaveBeenCalledWith(11, 7, {
      unreadOnly: false,
      offset: 0,
      limit: 100,
    });
    expect(svc.ListMessages).not.toHaveBeenCalled();
    expect((await screen.findByTestId("folder-label")).textContent).toBe("Facturas");
    await waitFor(() => expect(svc.RefreshLabel).toHaveBeenCalledTimes(1));
    expect(svc.RefreshLabel).toHaveBeenCalledWith(11, 7);
  });

  it("en la unificada usa la cuenta 0", async () => {
    renderList({ kind: "unified", role: "inbox" }, vi.fn(), { labelFilter: 11 });
    await waitFor(() =>
      expect(svc.ListLabeled).toHaveBeenCalledWith(11, 0, expect.anything()),
    );
    expect(svc.ListUnified).not.toHaveBeenCalled();
    await waitFor(() => expect(svc.RefreshLabel).toHaveBeenCalledWith(11, 0));
  });

  it("No leídos manda unreadOnly a ListLabeled", async () => {
    renderList({ kind: "unified", role: "inbox" }, vi.fn(), { labelFilter: 11 });
    await waitFor(() => expect(svc.ListLabeled).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: "No leídos" }));
    await waitFor(() =>
      expect(svc.ListLabeled).toHaveBeenCalledWith(11, 0, {
        unreadOnly: true,
        offset: 0,
        limit: 100,
      }),
    );
  });

  it("vacía dice que no hay correos con la etiqueta", async () => {
    renderList({ kind: "unified", role: "inbox" }, vi.fn(), { labelFilter: 11 });
    expect(
      await screen.findByText("No hay correos con esta etiqueta."),
    ).toBeTruthy();
  });

  it("con conversaciones activas y el filtro, la lista es plana", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValue({
      unifiedInbox: false,
      background: "",
      disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "",
      threadedList: true,
      readingPane: "",
    });
    vi.mocked(svc.ListLabeled).mockResolvedValueOnce([summary(1, 7)]);
    renderList({ kind: "folder", folderId: 70 }, vi.fn(), { labelFilter: 11 });
    expect(await screen.findByRole("button", { name: /Asunto 1/ })).toBeTruthy();
    expect(svc.ListThreads).not.toHaveBeenCalled();
    expect(screen.queryByLabelText("Conversaciones")).toBeNull();
  });

  it("si la etiqueta filtrada ya no existe, suelta el filtro", async () => {
    const setLabelFilter = vi.fn();
    renderList({ kind: "unified", role: "inbox" }, vi.fn(), {
      labelFilter: 99,
      setLabelFilter,
    });
    await waitFor(() => expect(setLabelFilter).toHaveBeenCalledWith(null));
  });

  it("el filtro del encabezado elige una etiqueta", async () => {
    const setLabelFilter = vi.fn();
    renderList({ kind: "unified", role: "inbox" }, vi.fn(), { setLabelFilter });
    fireEvent.pointerDown(
      await screen.findByRole("button", { name: "Filtrar por etiqueta" }),
      { button: 0, ctrlKey: false },
    );
    fireEvent.click(await screen.findByRole("menuitem", { name: /Clientes/ }));
    expect(setLabelFilter).toHaveBeenCalledWith(11);
  });
});

describe("MessageList en la Papelera", () => {
  const trashFolder = (total: number) => ({
    id: 90, accountId: 7, name: "Trash", delimiter: "/", role: "trash", unread: 0, total, selectable: true,
  });

  beforeEach(() => {
    resetSyncStateForTests();
    vi.clearAllMocks();
  });

  it("con dos clics (botón y confirmar) vacía la Papelera y refresca la vista", async () => {
    vi.mocked(svc.ListFolders).mockImplementation(
      (accountId: number) => Promise.resolve(accountId === 7 ? [trashFolder(2)] : []) as never,
    );
    const selectView = vi.fn();
    renderList({ kind: "folder", folderId: 90 }, vi.fn(), { selectView });
    await userEvent.click(await screen.findByRole("button", { name: "Vaciar Papelera" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Vaciar Papelera" }));
    await waitFor(() => expect(svc.EmptyTrash).toHaveBeenCalledWith(90));
    await waitFor(() => expect(selectView).toHaveBeenCalledWith({ kind: "folder", folderId: 90 }));
  });

  it("vacía, el botón queda deshabilitado", async () => {
    vi.mocked(svc.ListFolders).mockImplementation(
      (accountId: number) => Promise.resolve(accountId === 7 ? [trashFolder(0)] : []) as never,
    );
    renderList({ kind: "folder", folderId: 90 });
    const btn = await screen.findByRole("button", { name: "Vaciar Papelera" });
    expect((btn as HTMLButtonElement).disabled).toBe(true);
  });

  it("en otra carpeta no aparece", async () => {
    vi.mocked(svc.ListFolders).mockImplementation(
      (accountId: number) =>
        Promise.resolve(accountId === 7 ? [{ ...trashFolder(2), id: 3, name: "INBOX", role: "inbox" }] : []) as never,
    );
    renderList({ kind: "folder", folderId: 3 });
    expect(await screen.findByRole("heading", { name: "Entrada" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Vaciar Papelera" })).toBeNull();
  });
});

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
import { MailShell, expandedSidebarSize, saveOuterLayout, sidebarResizeAction, type ShellState } from "./MailShell";

function account(id: number): Account {
  return {
    id, email: `${id}@b.c`, displayName: "", username: "a", color: "#f00",
    imap: { host: "", port: 993, security: Security.SecurityTLS },
    smtp: { host: "", port: 465, security: Security.SecurityTLS },
    saveSentCopy: true,
    defaultSignatureId: 0,
  };
}

function folder(id: number, accountId: number, role: string): Folder {
  return { id, accountId, name: role, delimiter: "/", role, unread: 0, total: 0, selectable: true, position: 0, color: "" };
}

const FOLDERS: Record<number, Folder[]> = {
  2: [folder(21, 2, "inbox"), folder(22, 2, "sent")],
  5: [folder(51, 5, "inbox")],
};

function setup(saved: View | null, unifiedInbox: boolean, readingPane = "", withSidebar = false) {
  if (saved) localStorage.setItem("pc:view", JSON.stringify(saved));
  vi.mocked(svc.ListFolders).mockImplementation((id: number) => Promise.resolve(FOLDERS[id]) as never);
  vi.mocked(svc.GetSettings).mockResolvedValue({ unifiedInbox, disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, autoBackupDir: "", taskReminderMinutes: null, taskappApiUrl: "", taskappWebUrl: "", theme: "", threadedList: false, readingPane, background: "" });
  let current: ShellState | null = null;
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MailShell
        accounts={[account(5), account(2)].sort((a, b) => a.id - b.id)}
        list={(s) => {
          current = s;
          return <p data-testid="view">{JSON.stringify(s.view)}</p>;
        }}
        reader={() => <p>lector</p>}
        sidebar={
          withSidebar ? (_s, mode) => <p data-testid="sidebar-mode">{String(mode.collapsed)}</p> : undefined
        }
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

describe("MailShell búsqueda", () => {
  afterEach(() => localStorage.clear());

  it("cambiar de carpeta limpia la búsqueda", async () => {
    const { state } = setup({ kind: "folder", folderId: 21 }, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
    act(() => state().setSearch({ text: "factura", scope: "all" }));
    expect(state().search).toEqual({ text: "factura", scope: "all" });
    act(() => state().selectView({ kind: "folder", folderId: 22 }));
    expect(state().search).toBeNull();
  });
});

describe("MailShell conversaciones", () => {
  afterEach(() => localStorage.clear());

  it("elegir un hilo limpia el mensaje; cambiar de vista, buscar o elegir un mensaje lo cierra", async () => {
    const { state } = setup({ kind: "folder", folderId: 21 }, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
    act(() => state().selectMessage(7));
    act(() => state().selectThread("<r@x>"));
    expect(state().threadRoot).toBe("<r@x>");
    expect(state().messageId).toBeNull();
    act(() => state().selectMessage(8));
    expect(state().threadRoot).toBeNull();
    expect(state().messageId).toBe(8);
    act(() => state().selectThread("<r@x>"));
    act(() => state().setSearch({ text: "factura", scope: "all" }));
    expect(state().threadRoot).toBeNull();
    act(() => state().setSearch(null));
    act(() => state().selectThread("<r@x>"));
    act(() => state().selectView({ kind: "folder", folderId: 22 }));
    expect(state().threadRoot).toBeNull();
  });
});

describe("MailShell contactos", () => {
  afterEach(() => localStorage.clear());

  it("openContact pasa a la libreta con la ficha abierta, sin sincronizar; selectView la cierra", async () => {
    const { state } = setup({ kind: "folder", folderId: 21 }, false);
    await waitFor(() => expect(svc.SyncFolder).toHaveBeenCalledWith(21));
    act(() => state().openContact(4));
    expect(shown()).toEqual({ kind: "contacts" });
    expect(state().contactId).toBe(4);
    act(() => state().selectContact(0));
    expect(state().contactId).toBe(0);
    act(() => state().selectView({ kind: "folder", folderId: 22 }));
    expect(state().contactId).toBeNull();
    await waitFor(() => expect(svc.SyncFolder).toHaveBeenCalledWith(22));
    expect(vi.mocked(svc.SyncFolder)).toHaveBeenCalledTimes(2);
  });

  it("la libreta guardada vuelve al reabrir", async () => {
    setup({ kind: "contacts" }, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "contacts" }));
    expect(svc.SyncFolder).not.toHaveBeenCalled();
  });
});

describe("MailShell posición del panel de lectura", () => {
  afterEach(() => localStorage.clear());

  const panel = (id: string) => document.querySelector(`[data-panel][id="${id}"]`);
  // El grupo no rinde aria-orientation: la orientación se ve en su flex-direction.
  const innerGroup = () => panel("list")!.parentElement as HTMLElement;

  async function ready() {
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
  }

  it("con el lector abajo, la lista va antes que el lector en un grupo vertical", async () => {
    setup(null, false, "bottom");
    await ready();
    expect(innerGroup().style.flexDirection).toBe("column");
    expect(panel("list")!.compareDocumentPosition(panel("reader")!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("con el lector arriba, el lector va primero en un grupo vertical", async () => {
    setup(null, false, "top");
    await ready();
    expect(innerGroup().style.flexDirection).toBe("column");
    expect(panel("reader")!.compareDocumentPosition(panel("list")!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("con el lector a la derecha, el grupo es horizontal", async () => {
    setup(null, false, "right");
    await ready();
    expect(innerGroup().style.flexDirection).toBe("row");
    expect(panel("list")!.compareDocumentPosition(panel("reader")!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });
});

describe("MailShell selección múltiple", () => {
  afterEach(() => localStorage.clear());

  const marks = { keys: [1, 2], anchor: 2 };
  const none = { keys: [], anchor: null };

  async function marked() {
    const h = setup({ kind: "folder", folderId: 21 }, false);
    await waitFor(() => expect(shown()).toEqual({ kind: "folder", folderId: 21 }));
    act(() => {
      h.state().setMarked(marks);
      h.state().setMarkedThreads({ keys: ["<a@x>", "<b@x>"], anchor: "<b@x>" });
    });
    expect(h.state().marked).toEqual(marks);
    return h.state;
  }

  it("cambiar de vista suelta la selección", async () => {
    const state = await marked();
    act(() => state().selectView({ kind: "folder", folderId: 22 }));
    expect(state().marked).toEqual(none);
    expect(state().markedThreads).toEqual(none);
  });

  it("buscar o cerrar la búsqueda suelta la selección", async () => {
    const state = await marked();
    act(() => state().setSearch({ text: "factura", scope: "all" }));
    expect(state().marked).toEqual(none);
    expect(state().markedThreads).toEqual(none);
  });

  it("abrir un correo o un hilo suelta la selección; selectMessage(null) no", async () => {
    const state = await marked();
    act(() => state().selectMessage(null));
    expect(state().marked).toEqual(marks);
    act(() => state().selectMessage(9));
    expect(state().marked).toEqual(none);
    expect(state().markedThreads).toEqual(none);
    act(() => state().setMarked(marks));
    act(() => state().selectThread("<r@x>"));
    expect(state().marked).toEqual(none);
  });

  it("openContact suelta la selección y clearMarks también", async () => {
    const state = await marked();
    act(() => state().openContact(4));
    expect(state().marked).toEqual(none);
    act(() => state().setMarked(marks));
    act(() => state().clearMarks());
    expect(state().marked).toEqual(none);
  });

  it("selectView suelta el filtro de etiqueta y cambiarlo suelta las marcas", async () => {
    const state = await marked();
    act(() => state().setLabelFilter(11));
    expect(state().labelFilter).toBe(11);
    expect(state().marked).toEqual(none);
    act(() => state().setMarked(marks));
    act(() => state().setLabelFilter(12));
    expect(state().marked).toEqual(none);
    act(() => state().selectView({ kind: "folder", folderId: 22 }));
    expect(state().labelFilter).toBeNull();
  });
});

describe("MailShell sidebar plegable", () => {
  afterEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });

  const mode = () => screen.getByTestId("sidebar-mode").textContent;

  it("el botón Plegar sidebar pasa collapsed=true al slot y lo recuerda", async () => {
    const user = userEvent.setup();
    setup(null, false, "", true);
    expect(mode()).toBe("false");
    await user.click(screen.getByRole("button", { name: "Plegar sidebar" }));
    expect(mode()).toBe("true");
    expect(localStorage.getItem("pc:sidebar-collapsed")).toBe("1");
  });

  it("arranca plegado si estaba guardado y Expandir sidebar lo despliega", async () => {
    const user = userEvent.setup();
    localStorage.setItem("pc:sidebar-collapsed", "1");
    setup(null, false, "", true);
    expect(mode()).toBe("true");
    await user.click(screen.getByRole("button", { name: "Expandir sidebar" }));
    expect(mode()).toBe("false");
    expect(localStorage.getItem("pc:sidebar-collapsed")).toBe("0");
  });

  it("plegado, Redactar es un botón de ícono que abre el redactor", async () => {
    const user = userEvent.setup();
    localStorage.setItem("pc:sidebar-collapsed", "1");
    setup(null, false, "", true);
    const btn = screen.getByRole("button", { name: "Redactar" });
    expect(btn).not.toHaveTextContent("Redactar");
    await user.click(btn);
    expect(openNew).toHaveBeenCalledWith(2);
  });

  it("mientras está plegado no guarda el layout exterior", () => {
    localStorage.setItem("pc:layout:bottom", JSON.stringify({ outer: { sidebar: 25, main: 75 }, inner: { list: 40, reader: 60 } }));
    saveOuterLayout("bottom", { sidebar: 4, main: 96 }, true);
    expect(JSON.parse(localStorage.getItem("pc:layout:bottom")!).outer).toEqual({ sidebar: 25, main: 75 });
    saveOuterLayout("bottom", { sidebar: 30, main: 70 }, false);
    expect(JSON.parse(localStorage.getItem("pc:layout:bottom")!).outer).toEqual({ sidebar: 30, main: 70 });
  });
});

describe("sincronización del sidebar plegado con el panel", () => {
  afterEach(() => localStorage.clear());

  it("la primera medición real (sin tamaño previo o desde 0 px) vuelve a aplicar lo guardado", () => {
    // Arranque oculto: al mostrarse, el panel se rearma desplegado; manda el flag.
    expect(sidebarResizeAction(undefined, false, true)).toBe("reapply");
    expect(sidebarResizeAction(0, false, true)).toBe("reapply");
    expect(sidebarResizeAction(undefined, true, true)).toBe("none");
    expect(sidebarResizeAction(0, true, false)).toBe("reapply");
  });

  it("después, el panel manda: arrastrar o el teclado cambian el modo", () => {
    expect(sidebarResizeAction(240, true, false)).toBe("adopt");
    expect(sidebarResizeAction(52, false, true)).toBe("adopt");
    expect(sidebarResizeAction(240, false, false)).toBe("none");
  });

  it("al desplegar vuelve al último ancho guardado, no al mínimo", () => {
    localStorage.setItem("pc:layout:right", JSON.stringify({ outer: { sidebar: 23, main: 77 }, inner: { list: 36, reader: 64 } }));
    expect(expandedSidebarSize("right")).toBe("23%");
    expect(expandedSidebarSize("bottom")).toBe("18%");
  });
});

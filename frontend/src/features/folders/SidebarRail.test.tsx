import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() => Promise.resolve({ unifiedInbox: false })),
  ListFolders: vi.fn(() => Promise.resolve([])),
  RenameFolder: vi.fn(() => Promise.resolve({})),
  MoveMessages: vi.fn(() => Promise.resolve()),
  ListLocalDrafts: vi.fn(() => Promise.resolve([])),
  SetFolderColor: vi.fn(() => Promise.resolve()),
  SetFolderOrder: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { TooltipProvider } from "@/components/ui/tooltip";
import { setMessageDrag } from "@/lib/dnd";
import type { UnifiedRole } from "@/lib/view";
import { FakeDataTransfer } from "../../test/dataTransfer";
import type { ShellState } from "../layout/MailShell";
import { EXPAND_DELAY_MS } from "./FolderTree";
import { SidebarRail } from "./SidebarRail";

const acc = (id: number, displayName: string, color: string): Account => ({
  id, email: `${displayName.toLowerCase()}@b.c`, displayName, username: "a", color,
  imap: { host: "", port: 993, security: Security.SecurityTLS },
  smtp: { host: "", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
  defaultSignatureId: 0,
});
const fabri = acc(7, "Fabri", "#f00");
const trabajo = acc(8, "Trabajo", "#0f0");

const folder = (id: number, accountId: number, name: string, role = "", unread = 0) =>
  ({ id, accountId, name, delimiter: "/", role, unread, total: 0, selectable: true, position: 0, color: "" });

const state = (): ShellState => ({
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
  openContact: vi.fn(), taskKey: null, selectTask: vi.fn(), openTask: vi.fn(), eventKey: null, selectEvent: vi.fn(), calendarDate: null,
  marked: { keys: [], anchor: null }, markedThreads: { keys: [], anchor: null },
  setMarked: vi.fn(), setMarkedThreads: vi.fn(), clearMarks: vi.fn(),
  labelFilter: null, setLabelFilter: vi.fn(),
});

function renderRail(s: ShellState, unifiedRows: UnifiedRole[] = [], onOpenSettings = vi.fn()) {
  const { unmount } = render(
    <QueryClientProvider client={new QueryClient()}>
      <TooltipProvider>
        <SidebarRail accounts={[fabri, trabajo]} state={s} unifiedRows={unifiedRows} onOpenSettings={onOpenSettings} />
      </TooltipProvider>
    </QueryClientProvider>,
  );
  return { onOpenSettings, unmount };
}

describe("SidebarRail", () => {
  beforeAll(() => {
    // Radix usa APIs de puntero que jsdom no trae.
    Element.prototype.hasPointerCapture = () => false;
    Element.prototype.releasePointerCapture = () => {};
    Element.prototype.scrollIntoView = () => {};
  });
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListFolders).mockImplementation(((id: number) =>
      Promise.resolve(
        id === 7 ? [folder(3, 7, "INBOX", "inbox", 4), folder(9, 7, "Work")] : [folder(4, 8, "INBOX", "inbox", 0)],
      )) as never);
  });

  it("muestra una burbuja por cuenta con su nombre como etiqueta y el contador de no leídos", async () => {
    renderRail(state());
    const bubble = screen.getByRole("button", { name: "Fabri · fabri@b.c" });
    expect(bubble).toHaveTextContent("F");
    await waitFor(() => expect(bubble).toHaveTextContent("4"));
    expect(screen.getByRole("button", { name: "Trabajo · trabajo@b.c" })).toHaveTextContent(/^T$/);
  });

  it("clic en la burbuja abre sus carpetas; elegir una selecciona la vista y cierra el popover", async () => {
    const user = userEvent.setup();
    const s = state();
    renderRail(s);
    await user.click(screen.getByRole("button", { name: "Fabri · fabri@b.c" }));
    const work = await screen.findByRole("button", { name: "Work" });
    await user.click(work);
    expect(s.selectView).toHaveBeenCalledWith({ kind: "folder", folderId: 9 });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Work" })).toBeNull());
  });

  it("Renombrar desde el popover deja el diálogo abierto aunque el popover se cierre", async () => {
    const user = userEvent.setup();
    renderRail(state());
    const bubble = screen.getByRole("button", { name: "Fabri · fabri@b.c" });
    await user.click(bubble);
    fireEvent.contextMenu(await screen.findByRole("button", { name: "Work" }));
    await user.click(await screen.findByRole("menuitem", { name: "Renombrar…" }));
    expect(await screen.findByLabelText("Nombre")).toBeTruthy();
    // Cerrar el popover (la burbuja lo alterna) no se lleva el diálogo. Se
    // mira el DOM: el diálogo modal esconde el resto del árbol accesible.
    const popover = () => document.querySelector("[data-slot=popover-content]");
    if (popover()) fireEvent.click(bubble);
    await waitFor(() => expect(popover()).toBeNull());
    expect(screen.getByLabelText("Nombre")).toHaveValue("Work");
  });

  it("arrastrar correos de la cuenta sobre la burbuja abre el popover después de la demora", async () => {
    renderRail(state());
    const bubble = screen.getByRole("button", { name: "Fabri · fabri@b.c" });
    await waitFor(() => expect(bubble).toHaveTextContent("4"));
    const dt = new FakeDataTransfer();
    setMessageDrag(dt as unknown as DataTransfer, 7, { ids: [1, 2], folderId: 3 });
    vi.useFakeTimers();
    try {
      fireEvent.dragOver(bubble, { dataTransfer: dt });
      act(() => vi.advanceTimersByTime(EXPAND_DELAY_MS - 1));
      expect(screen.queryByRole("button", { name: "Work" })).toBeNull();
      act(() => vi.advanceTimersByTime(1));
    } finally {
      vi.useRealTimers();
    }
    const work = await screen.findByRole("button", { name: "Work" });
    fireEvent.dragOver(work, { dataTransfer: dt });
    fireEvent.drop(work, { dataTransfer: dt });
    await waitFor(() => expect(svc.MoveMessages).toHaveBeenCalledWith([1, 2], 9));
  });

  it("arrastrar correos de otra cuenta sobre la burbuja no abre nada", async () => {
    renderRail(state());
    const bubble = screen.getByRole("button", { name: "Fabri · fabri@b.c" });
    const dt = new FakeDataTransfer();
    setMessageDrag(dt as unknown as DataTransfer, 8, { ids: [1], folderId: 4 });
    vi.useFakeTimers();
    try {
      fireEvent.dragOver(bubble, { dataTransfer: dt });
      act(() => vi.advanceTimersByTime(EXPAND_DELAY_MS * 2));
    } finally {
      vi.useRealTimers();
    }
    expect(screen.queryByRole("button", { name: "Work" })).toBeNull();
  });

  it("las vistas unificadas solo aparecen si se pasan", () => {
    renderRail(state());
    expect(screen.queryByRole("button", { name: "Todas las cuentas · Entrada" })).toBeNull();
  });

  it("una vista unificada selecciona esa vista", async () => {
    const user = userEvent.setup();
    const s = state();
    renderRail(s, ["inbox", "flagged"]);
    await user.click(screen.getByRole("button", { name: "Todas las cuentas · Destacados" }));
    expect(s.selectView).toHaveBeenCalledWith({ kind: "unified", role: "flagged" });
  });

  it("Calendario selecciona la vista y queda activo", async () => {
    const user = userEvent.setup();
    const s = state();
    const { unmount } = renderRail(s);
    expect(screen.getByRole("button", { name: "Calendario" }).getAttribute("aria-current")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Calendario" }));
    expect(s.selectView).toHaveBeenCalledWith({ kind: "calendar" });
    unmount();
    renderRail({ ...state(), view: { kind: "calendar" } });
    expect(screen.getByRole("button", { name: "Calendario" }).getAttribute("aria-current")).toBe("true");
  });

  it("Contactos y Configuración", async () => {
    const user = userEvent.setup();
    const s = state();
    const { onOpenSettings } = renderRail(s);
    await user.click(screen.getByRole("button", { name: "Contactos" }));
    expect(s.selectView).toHaveBeenCalledWith({ kind: "contacts" });
    await user.click(screen.getByRole("button", { name: "Configuración" }));
    expect(onOpenSettings).toHaveBeenCalled();
  });
});

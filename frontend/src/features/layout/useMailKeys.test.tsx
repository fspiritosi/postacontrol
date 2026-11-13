import { QueryClient, QueryClientProvider, QueryObserver } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(),
  ArchiveMessages: vi.fn(() => Promise.resolve()),
  DeleteMessages: vi.fn(() => Promise.resolve()),
  SetFlagged: vi.fn(() => Promise.resolve()),
  SetSeen: vi.fn(() => Promise.resolve()),
  GetMessage: vi.fn(),
  GetThread: vi.fn(),
  UndoMove: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
const composer = vi.hoisted(() => ({ openFrom: vi.fn(() => Promise.resolve()), openNew: vi.fn() }));
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => composer }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Folder, MessageSummary } from "../../../bindings/postacontrol/core/model/models";
import type { ShellState } from "./MailShell";
import { useMailKeys } from "./useMailKeys";

function folder(id: number, role = ""): Folder {
  return { id, accountId: 7, name: `f${id}`, delimiter: "/", role, unread: 0, total: 0, selectable: true, position: 0 };
}

function summary(id: number, folderId = 1, flagged = false): MessageSummary {
  return {
    id, accountId: 7, folderId, uid: id, from: { name: "", email: "a@b.c" }, subject: `m${id}`, snippet: "",
    date: "2026-10-30T10:00:00Z", seen: true, flagged, answered: false, hasAttachments: false, labels: [],
  } as MessageSummary;
}

type Over = Partial<Pick<ShellState, "messageId" | "threadRoot" | "marked">>;

function shell(over: Over = {}): ShellState {
  return {
    view: { kind: "folder", folderId: 1 },
    messageId: null,
    threadRoot: null,
    marked: { keys: [], anchor: null },
    markedThreads: { keys: [], anchor: null },
    selectMessage: vi.fn(),
    selectThread: vi.fn(),
    clearMarks: vi.fn(),
    ...over,
  } as unknown as ShellState;
}

function Harness({ state }: { state: ShellState }) {
  const keys = useMailKeys(state, 7);
  return (
    <>
      <input aria-label="Buscar correo" />
      <ul>
        {[10, 11, 12].map((id) => (
          <li key={id} data-row-id={id}>
            m{id}
          </li>
        ))}
      </ul>
      {keys.helpOpen && <p>ayuda abierta</p>}
    </>
  );
}

function setup(state: ShellState, folders: Folder[] = [folder(1, "inbox"), folder(3, "trash")]) {
  vi.mocked(svc.ListFolders).mockResolvedValue(folders);
  const qc = new QueryClient();
  // La lista cargada, como la dejaría useMessages.
  qc.setQueryData(["messages", "folder", 1], { pages: [[summary(10), summary(11), summary(12)]], pageParams: [0] });
  render(
    <QueryClientProvider client={qc}>
      <Harness state={state} />
    </QueryClientProvider>,
  );
  // Un observador la deja activa, como la lista que la está mostrando.
  const page = { pages: [[summary(10), summary(11), summary(12)]], pageParams: [0] };
  new QueryObserver(qc, { queryKey: ["messages", "folder", 1], queryFn: () => page, staleTime: Infinity }).subscribe(
    () => {},
  );
}

const press = (key: string, init: KeyboardEventInit = {}) => fireEvent.keyDown(window, { key, ...init });

describe("useMailKeys", () => {
  beforeEach(() => vi.clearAllMocks());

  it("j y k abren el correo siguiente y el anterior de la lista", () => {
    const s = shell({ messageId: 11 });
    setup(s);
    press("j");
    expect(s.selectMessage).toHaveBeenLastCalledWith(12);
    press("k");
    expect(s.selectMessage).toHaveBeenLastCalledWith(10);
  });

  it("sin nada abierto, j abre el primero", () => {
    const s = shell();
    setup(s);
    press("j");
    expect(s.selectMessage).toHaveBeenLastCalledWith(10);
  });

  it("escribiendo en un campo las teclas son texto", () => {
    const s = shell({ messageId: 11 });
    setup(s);
    const input = screen.getByLabelText("Buscar correo");
    input.focus();
    fireEvent.keyDown(input, { key: "j" });
    fireEvent.keyDown(input, { key: "e" });
    expect(s.selectMessage).not.toHaveBeenCalled();
    expect(svc.ArchiveMessages).not.toHaveBeenCalled();
  });

  it("e archiva el correo abierto y abre el siguiente", async () => {
    const s = shell({ messageId: 11 });
    setup(s);
    press("e");
    await waitFor(() => expect(svc.ArchiveMessages).toHaveBeenCalledWith([11]));
    await waitFor(() => expect(s.selectMessage).toHaveBeenLastCalledWith(12));
    expect(toast.success).toHaveBeenCalledWith("Archivado");
  });

  it("e con correos marcados archiva esos y suelta la selección", async () => {
    const s = shell({ marked: { keys: [10, 12], anchor: 10 } as never });
    setup(s);
    press("e");
    await waitFor(() => expect(svc.ArchiveMessages).toHaveBeenCalledWith([10, 12]));
    await waitFor(() => expect(s.clearMarks).toHaveBeenCalled());
    expect(toast.success).toHaveBeenCalledWith("2 archivados");
  });

  it("Supr manda a la Papelera y abre el siguiente; el último abre el anterior", async () => {
    const s = shell({ messageId: 12 });
    setup(s);
    press("Delete");
    await waitFor(() => expect(svc.DeleteMessages).toHaveBeenCalledWith([12]));
    await waitFor(() => expect(s.selectMessage).toHaveBeenLastCalledWith(11));
    expect(toast.success).toHaveBeenCalledWith("Movido a la Papelera");
  });

  it("en la Papelera Supr no elimina definitivamente: avisa", async () => {
    const s = shell({ messageId: 11 });
    vi.mocked(svc.ListFolders).mockResolvedValue([folder(1, "trash")]);
    setup(s, [folder(1, "trash")]);
    press("Delete");
    await waitFor(() => expect(toast.warning).toHaveBeenCalled());
    expect(svc.DeleteMessages).not.toHaveBeenCalled();
  });

  it("r, a y f responden, responden a todos y reenvían el abierto", () => {
    setup(shell({ messageId: 11 }));
    press("r");
    press("a");
    press("f");
    expect(composer.openFrom).toHaveBeenNthCalledWith(1, 11, "reply");
    expect(composer.openFrom).toHaveBeenNthCalledWith(2, 11, "replyAll");
    expect(composer.openFrom).toHaveBeenNthCalledWith(3, 11, "forward");
  });

  it("s destaca y u marca como no leído y cierra el lector", async () => {
    const s = shell({ messageId: 11 });
    setup(s);
    press("s");
    await waitFor(() => expect(svc.SetFlagged).toHaveBeenCalledWith([11], true));
    press("u");
    await waitFor(() => expect(svc.SetSeen).toHaveBeenCalledWith([11], false));
    await waitFor(() => expect(s.selectMessage).toHaveBeenLastCalledWith(null));
  });

  it("n redacta, / va a la búsqueda y ? abre la ayuda", async () => {
    setup(shell());
    press("n");
    expect(composer.openNew).toHaveBeenCalledWith(7);
    press("/");
    expect(screen.getByLabelText("Buscar correo")).toHaveFocus();
    screen.getByLabelText("Buscar correo").blur();
    press("?", { shiftKey: true });
    expect(await screen.findByText("ayuda abierta")).toBeInTheDocument();
  });

  it("z deshace lo último que se archivó", async () => {
    vi.mocked(svc.ArchiveMessages).mockResolvedValueOnce(21 as never);
    setup(shell({ messageId: 11 }));
    press("e");
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    press("z");
    await waitFor(() => expect(svc.UndoMove).toHaveBeenCalledWith(21));
  });

  it("con Ctrl no es un atajo", () => {
    const s = shell({ messageId: 11 });
    setup(s);
    press("j", { ctrlKey: true });
    press("e", { ctrlKey: true });
    expect(s.selectMessage).not.toHaveBeenCalled();
    expect(svc.ArchiveMessages).not.toHaveBeenCalled();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetThread: vi.fn(),
  GetMessage: vi.fn(),
  ListFolders: vi.fn(),
  MoveMessages: vi.fn(() => Promise.resolve()),
}));
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => ({ openFrom: vi.fn() }) }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type {
  Account,
  Folder,
  MessageDetail,
  MessageSummary,
} from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import type { ShellState } from "../layout/MailShell";
import { ThreadView } from "./ThreadView";

const account: Account = {
  id: 7, email: "yo@b.c", displayName: "Yo", username: "a", color: "#f00",
  imap: { host: "", port: 993, security: Security.SecurityTLS },
  smtp: { host: "", port: 465, security: Security.SecurityTLS },
  saveSentCopy: true,
  defaultSignatureId: 0,
};

function folder(id: number, name: string, role: string): Folder {
  return { id, accountId: 7, name, delimiter: "/", role, unread: 0, total: 0, selectable: true };
}
const folders = [
  folder(3, "INBOX", "inbox"),
  folder(4, "Sent", "sent"),
  folder(5, "Trabajo", ""),
  folder(6, "Drafts", "drafts"),
];

function summary(id: number, folderId: number, from: string, day: number): MessageSummary {
  return {
    id, accountId: 7, folderId, uid: id, from: { name: from, email: `${id}@x.y` },
    subject: "Presupuesto", snippet: `Resumen ${id}`, date: `2026-10-${day}T10:00:00Z`,
    seen: true, flagged: false, answered: false, hasAttachments: false,
  };
}

const thread = [summary(1, 3, "Ana", 20), summary(2, 4, "Yo", 21), summary(3, 3, "Ana", 22)];

function detail(s: MessageSummary): MessageDetail {
  return {
    ...s, messageId: "", inReplyTo: "", references: null, to: null, cc: null, replyTo: null,
    html: "", text: `Cuerpo ${s.id}`, hasRemoteImages: false, attachments: null,
  };
}

function setup(msgs: MessageSummary[] = thread) {
  vi.mocked(svc.GetThread).mockResolvedValue(msgs);
  vi.mocked(svc.GetMessage).mockImplementation(
    (id: number) => Promise.resolve(detail(msgs.find((m) => m.id === id)!)) as never,
  );
  vi.mocked(svc.ListFolders).mockResolvedValue(folders);
  const selectThread = vi.fn();
  const selectMessage = vi.fn();
  const state: ShellState = {
    view: { kind: "folder", folderId: 3 },
    messageId: null,
    selectView: vi.fn(),
    selectMessage,
    search: null,
    setSearch: vi.fn(),
    threadRoot: "<r@x>",
    selectThread,
  };
  const user = userEvent.setup();
  const qc = new QueryClient();
  render(
    <QueryClientProvider client={qc}>
      <ThreadView accounts={[account]} state={state} />
    </QueryClientProvider>,
  );
  return { user, selectThread, selectMessage, qc };
}

const items = () => screen.getAllByTestId("thread-message");

describe("ThreadView", () => {
  beforeEach(() => vi.clearAllMocks());

  it("abrir un hilo muestra los mensajes en orden con el último expandido", async () => {
    setup();
    expect(await screen.findByText("Cuerpo 3")).toBeTruthy();
    expect(svc.GetThread).toHaveBeenCalledWith(3, "<r@x>");
    const list = items();
    expect(list).toHaveLength(3);
    expect(list[0].textContent).toContain("Resumen 1");
    expect(list[1].textContent).toContain("Resumen 2");
    // Los plegados no se cargan (abrirlos lo marcaría leído).
    expect(svc.GetMessage).toHaveBeenCalledTimes(1);
    expect(svc.GetMessage).toHaveBeenCalledWith(3);
    expect(screen.queryByText("Cuerpo 1")).toBeNull();
    // Cada mensaje expandido trae sus propias acciones.
    expect(within(list[2]).getByRole("button", { name: "Responder" })).toBeTruthy();
  });

  it("expandir un mensaje plegado lo carga", async () => {
    const { user } = setup();
    await screen.findByText("Cuerpo 3");
    await user.click(within(items()[0]).getByRole("button", { name: /Ana/ }));
    expect(await screen.findByText("Cuerpo 1")).toBeTruthy();
    expect(svc.GetMessage).toHaveBeenCalledWith(1);
  });

  it("los de Enviados llevan la etiqueta Enviado", async () => {
    setup();
    await screen.findByText("Cuerpo 3");
    await waitFor(() => expect(within(items()[1]).getByText("Enviado")).toBeTruthy());
    expect(within(items()[0]).queryByText("Enviado")).toBeNull();
    expect(within(items()[2]).queryByText("Enviado")).toBeNull();
  });

  it("mover un mensaje del hilo cierra el hilo", async () => {
    const { user, selectThread } = setup();
    await screen.findByText("Cuerpo 3");
    const last = items()[2];
    await waitFor(() =>
      expect(within(last).getByRole("button", { name: "Mover a…" }).hasAttribute("disabled")).toBe(false),
    );
    await user.click(within(last).getByRole("button", { name: "Mover a…" }));
    await user.click(await screen.findByRole("menuitem", { name: "Trabajo" }));
    await waitFor(() => expect(selectThread).toHaveBeenCalledWith(null));
    expect(svc.MoveMessages).toHaveBeenCalledWith([3], 5);
  });

  it("expande el no leído más nuevo (sin contar Enviados), y solo ese", async () => {
    const unread = (m: MessageSummary) => ({ ...m, seen: false });
    const msgs = [
      unread(summary(1, 3, "Ana", 19)),
      unread(summary(2, 3, "Ana", 20)),
      unread(summary(3, 4, "Yo", 21)),
      summary(4, 3, "Ana", 22),
    ];
    const { qc } = setup(msgs);
    expect(await screen.findByText("Cuerpo 2")).toBeTruthy();
    expect(svc.GetMessage).toHaveBeenCalledTimes(1);
    expect(svc.GetMessage).toHaveBeenCalledWith(2);
    expect(screen.queryByText("Cuerpo 4")).toBeNull();
    // El último queda plegado y se puede abrir.
    expect(within(items()[3]).getByRole("button", { name: /Ana/ }).getAttribute("aria-expanded")).toBe(
      "false",
    );
    // Abrirlo lo marca leído y el hilo se recarga: sigue abierto el mismo.
    vi.mocked(svc.GetThread).mockResolvedValue(msgs.map((m) => ({ ...m, seen: true })));
    await qc.invalidateQueries({ queryKey: ["thread"] });
    await waitFor(() => expect(svc.GetThread).toHaveBeenCalledTimes(2));
    expect(screen.getByText("Cuerpo 2")).toBeTruthy();
    expect(screen.queryByText("Cuerpo 4")).toBeNull();
  });

  it("un borrador no se responde ni se reenvía: se sigue editando", async () => {
    setup([summary(9, 6, "Yo", 23)]);
    expect(await screen.findByText("Cuerpo 9")).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Seguir editando" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Responder" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Responder a todos" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Reenviar" })).toBeNull();
  });
});

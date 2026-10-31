import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(),
  SetSeen: vi.fn(() => Promise.resolve()),
  SetFlagged: vi.fn(() => Promise.resolve()),
  MoveMessages: vi.fn(() => Promise.resolve()),
  DeleteMessages: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Folder, MessageDetail } from "../../../bindings/postacontrol/core/model/models";
import { toast } from "sonner";
import { MessageActions } from "./MessageActions";

function folder(id: number, name: string, role = "", selectable = true): Folder {
  return { id, accountId: 7, name, delimiter: "/", role, unread: 0, total: 0, selectable };
}
const folders = [
  folder(1, "INBOX", "inbox"),
  folder(2, "Trabajo"),
  folder(3, "Trash", "trash"),
  folder(4, "Padre", "", false),
];

function msg(folderId: number, flagged = false): MessageDetail {
  return {
    id: 55, accountId: 7, folderId, uid: 1, from: { name: "", email: "a@b.c" },
    subject: "s", snippet: "", date: "2026-10-30T10:00:00Z", seen: true, flagged,
    answered: false, hasAttachments: false, messageId: "", inReplyTo: "",
    references: null, to: null, cc: null, replyTo: null, html: "", text: "",
    hasRemoteImages: false, attachments: null,
  };
}

function setup(m: MessageDetail) {
  vi.mocked(svc.ListFolders).mockResolvedValue(folders);
  const selectMessage = vi.fn();
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MessageActions message={m} selectMessage={selectMessage} />
    </QueryClientProvider>,
  );
  return { selectMessage, user };
}

describe("MessageActions", () => {
  beforeEach(() => vi.clearAllMocks());

  it("Marcar como no leído llama SetSeen(false) y cierra el lector", async () => {
    const { selectMessage, user } = setup(msg(1));
    await user.click(screen.getByRole("button", { name: "Marcar como no leído" }));
    await waitFor(() => expect(selectMessage).toHaveBeenCalledWith(null));
    expect(svc.SetSeen).toHaveBeenCalledWith([55], false);
  });

  it("Mover a… lista las carpetas de la cuenta salvo la actual y mueve", async () => {
    const { selectMessage, user } = setup(msg(1));
    await user.click(await screen.findByRole("button", { name: "Mover a…" }));
    expect(await screen.findByRole("menuitem", { name: "Trabajo" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Papelera" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Entrada" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Padre" })).toBeNull();
    await user.click(screen.getByRole("menuitem", { name: "Trabajo" }));
    await waitFor(() => expect(svc.MoveMessages).toHaveBeenCalledWith([55], 2));
    expect(selectMessage).toHaveBeenCalledWith(null);
    expect(toast.success).toHaveBeenCalledWith("Movido a Trabajo");
  });

  it("Borrar fuera de Papelera no pide confirmación", async () => {
    const { selectMessage, user } = setup(msg(1));
    await user.click(screen.getByRole("button", { name: "Borrar" }));
    await waitFor(() => expect(svc.DeleteMessages).toHaveBeenCalledWith([55]));
    expect(selectMessage).toHaveBeenCalledWith(null);
    expect(toast.success).toHaveBeenCalledWith("Movido a la Papelera");
  });

  it("en Papelera pide confirmación y dice Eliminar definitivamente", async () => {
    const { selectMessage, user } = setup(msg(3));
    await screen.findByRole("button", { name: "Eliminar definitivamente" });
    await user.click(screen.getByRole("button", { name: "Eliminar definitivamente" }));
    expect(svc.DeleteMessages).not.toHaveBeenCalled();
    expect(await screen.findByText("¿Eliminar definitivamente? No se puede deshacer.")).toBeTruthy();
    const confirm = screen.getAllByRole("button", { name: "Eliminar definitivamente" }).slice(-1)[0];
    await user.click(confirm);
    await waitFor(() => expect(svc.DeleteMessages).toHaveBeenCalledWith([55]));
    expect(selectMessage).toHaveBeenCalledWith(null);
  });

  it("Destacar alterna", async () => {
    const { user } = setup(msg(1, false));
    const star = screen.getByRole("button", { name: "Destacar" });
    expect(star.getAttribute("aria-pressed")).toBe("false");
    await user.click(star);
    await waitFor(() => expect(svc.SetFlagged).toHaveBeenCalledWith([55], true));
  });

  it("Quitar destacado cuando ya está destacado", async () => {
    const { user } = setup(msg(1, true));
    const star = screen.getByRole("button", { name: "Quitar destacado" });
    expect(star.getAttribute("aria-pressed")).toBe("true");
    await user.click(star);
    await waitFor(() => expect(svc.SetFlagged).toHaveBeenCalledWith([55], false));
  });
});

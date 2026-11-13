import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListFolders: vi.fn(),
  SetSeen: vi.fn(() => Promise.resolve()),
  SetFlagged: vi.fn(() => Promise.resolve()),
  MoveMessages: vi.fn(() => Promise.resolve()),
  DeleteMessages: vi.fn(() => Promise.resolve()),
  ArchiveMessages: vi.fn(() => Promise.resolve()),
  UndoMove: vi.fn(() => Promise.resolve()),
  SnoozeMessages: vi.fn(() => Promise.resolve(0)),
  SaveMessageSource: vi.fn(() => Promise.resolve(true)),
  MessageSource: vi.fn(() => Promise.resolve({ text: "From: a@b.c\r\nSubject: s\r\n\r\nhola", truncated: false })),
  OpenDraftMessage: vi.fn(),
  ListLabels: vi.fn(),
  SetLabel: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
const composer = vi.hoisted(() => ({ openComposeDraft: vi.fn(() => Promise.resolve()) }));
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => composer }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Folder, MessageDetail } from "../../../bindings/postacontrol/core/model/models";
import { toast } from "sonner";
import { MessageActions } from "./MessageActions";

function folder(id: number, name: string, role = "", selectable = true): Folder {
  return { id, accountId: 7, name, delimiter: "/", role, unread: 0, total: 0, selectable, position: 0 };
}
const folders = [
  folder(1, "INBOX", "inbox"),
  folder(2, "Trabajo"),
  folder(3, "Trash", "trash"),
  folder(4, "Padre", "", false),
  folder(5, "Drafts", "drafts"),
  folder(6, "Archivo", "archive"),
];

function msg(folderId: number, flagged = false): MessageDetail {
  return {
    id: 55, accountId: 7, folderId, uid: 1, from: { name: "", email: "a@b.c" },
    subject: "s", snippet: "", date: "2026-10-30T10:00:00Z", seen: true, flagged,
    answered: false, hasAttachments: false, messageId: "", inReplyTo: "",
    references: null, to: null, cc: null, replyTo: null, html: "", text: "",
    hasRemoteImages: false, remoteImagesTrusted: false, attachments: null, labels: [],
  };
}

function setup(m: MessageDetail, qc = new QueryClient()) {
  vi.mocked(svc.ListFolders).mockResolvedValue(folders);
  const selectMessage = vi.fn();
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={qc}>
      <MessageActions message={m} selectMessage={selectMessage} />
    </QueryClientProvider>,
  );
  return { selectMessage, user };
}

// Las acciones van en el menú "Más acciones" (salvo Seguir editando).
const openMenu = async (user: ReturnType<typeof userEvent.setup>) =>
  user.click(screen.getByRole("button", { name: "Más acciones" }));
// Los ítems con atajo (Destacar S, Archivar E…) llevan la tecla en su nombre.
const item = (name: string) => screen.findByRole("menuitem", { name: new RegExp(`^${name}`) });
// Los submenús se abren con la flecha derecha y se elige con un clic directo:
// en jsdom, mover el puntero de userEvent cierra el submenú antes del clic.
const openSub = async (user: ReturnType<typeof userEvent.setup>, name: string) => {
  await openMenu(user);
  await enabled(name);
  fireEvent.keyDown(await item(name), { key: "ArrowRight" });
};
const pick = async (name: string) => fireEvent.click(await screen.findByRole("menuitem", { name }));
// Hasta saber la carpeta, algunos ítems quedan deshabilitados.
const enabled = async (name: string) =>
  waitFor(async () => expect((await item(name)).getAttribute("data-disabled")).toBeNull());

describe("MessageActions", () => {
  beforeEach(() => vi.clearAllMocks());

  it("a la vista solo queda el botón del menú", () => {
    setup(msg(1));
    expect(screen.getAllByRole("button").map((b) => b.getAttribute("aria-label") ?? b.textContent)).toEqual(["Más acciones"]);
  });

  it("Marcar como no leído llama SetSeen(false) y cierra el lector", async () => {
    const { selectMessage, user } = setup(msg(1));
    await openMenu(user);
    await user.click(await item("Marcar como no leído"));
    await waitFor(() => expect(selectMessage).toHaveBeenCalledWith(null));
    expect(svc.SetSeen).toHaveBeenCalledWith([55], false);
  });

  it("Mover a lista las carpetas de la cuenta salvo la actual y mueve", async () => {
    const { selectMessage, user } = setup(msg(1));
    await openSub(user, "Mover a");
    expect(await screen.findByRole("menuitem", { name: "Trabajo" })).toBeTruthy();
    expect(screen.getByRole("menuitem", { name: "Papelera" })).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Entrada" })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: "Padre" })).toBeNull();
    await pick("Trabajo");
    await waitFor(() => expect(svc.MoveMessages).toHaveBeenCalledWith([55], 2));
    expect(selectMessage).toHaveBeenCalledWith(null);
    expect(toast.success).toHaveBeenCalledWith("Movido a Trabajo");
  });

  it("Archivar lo manda a Archivo y cierra el lector", async () => {
    const { selectMessage, user } = setup(msg(1));
    await openMenu(user);
    await user.click(await item("Archivar"));
    await waitFor(() => expect(svc.ArchiveMessages).toHaveBeenCalledWith([55]));
    expect(selectMessage).toHaveBeenCalledWith(null);
    expect(toast.success).toHaveBeenCalledWith("Archivado");
  });

  it("mover ofrece Deshacer, que devuelve el correo", async () => {
    vi.mocked(svc.MoveMessages).mockResolvedValueOnce(9 as never);
    const { user } = setup(msg(1));
    await openSub(user, "Mover a");
    await pick("Trabajo");
    await waitFor(() =>
      expect(toast.success).toHaveBeenCalledWith(
        "Movido a Trabajo",
        expect.objectContaining({ action: expect.objectContaining({ label: "Deshacer" }) }),
      ),
    );
    const opts = vi.mocked(toast.success).mock.calls[0][1] as unknown as { action: { onClick: () => void } };
    opts.action.onClick();
    await waitFor(() => expect(svc.UndoMove).toHaveBeenCalledWith(9));
  });

  it("Posponer hasta mañana a las 8 cierra el lector y avisa", async () => {
    const { selectMessage, user } = setup(msg(1));
    await openSub(user, "Posponer");
    await pick("Mañana, 8:00");
    const tomorrow8 = new Date();
    tomorrow8.setDate(tomorrow8.getDate() + 1);
    tomorrow8.setHours(8, 0, 0, 0);
    await waitFor(() => expect(svc.SnoozeMessages).toHaveBeenCalledWith([55], tomorrow8.getTime() / 1000));
    expect(selectMessage).toHaveBeenCalledWith(null);
    expect(toast.success).toHaveBeenCalledWith("Pospuesto hasta mañana a las 8:00");
  });

  it("en Borradores no se ofrece Posponer ni Archivar", async () => {
    const { user } = setup(msg(5));
    await openMenu(user);
    await enabled("Mover a");
    expect(screen.queryByRole("menuitem", { name: /^Posponer/ })).toBeNull();
    expect(screen.queryByRole("menuitem", { name: /^Archivar/ })).toBeNull();
  });

  it("ver el código fuente y guardar como .eml", async () => {
    const { user } = setup(msg(1));
    await openMenu(user);
    await user.click(await item("Ver código fuente"));
    expect(await screen.findByLabelText("Código fuente del correo")).toHaveTextContent("Subject: s");
    expect(svc.MessageSource).toHaveBeenCalledWith(55);
    await user.click(screen.getByRole("button", { name: "Guardar como .eml" }));
    await waitFor(() => expect(svc.SaveMessageSource).toHaveBeenCalledWith(55));
    expect(toast.success).toHaveBeenCalledWith("Guardado.");
  });

  it("imprimir arma un iframe aparte con el correo", async () => {
    const { user } = setup(msg(1));
    await openMenu(user);
    await user.click(await item("Imprimir"));
    const frame = document.querySelector("iframe[sandbox='allow-same-origin allow-modals']") as HTMLIFrameElement;
    expect(frame).not.toBeNull();
    expect(frame.srcdoc).toContain("Content-Security-Policy");
  });

  it("en Archivo no se ofrece Archivar", async () => {
    const { user } = setup(msg(6));
    await openMenu(user);
    await enabled("Mover a");
    expect(screen.queryByRole("menuitem", { name: /^Archivar/ })).toBeNull();
  });

  it("Borrar fuera de Papelera no pide confirmación", async () => {
    const { selectMessage, user } = setup(msg(1));
    await openMenu(user);
    await enabled("Borrar");
    await user.click(await item("Borrar"));
    await waitFor(() => expect(svc.DeleteMessages).toHaveBeenCalledWith([55]));
    expect(selectMessage).toHaveBeenCalledWith(null);
    expect(toast.success).toHaveBeenCalledWith("Movido a la Papelera");
  });

  it("en Papelera pide confirmación y dice Eliminar definitivamente", async () => {
    const { selectMessage, user } = setup(msg(3));
    await openMenu(user);
    await user.click(await item("Eliminar definitivamente"));
    expect(svc.DeleteMessages).not.toHaveBeenCalled();
    expect(await screen.findByText("¿Eliminar definitivamente? No se puede deshacer.")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Eliminar definitivamente" }));
    await waitFor(() => expect(svc.DeleteMessages).toHaveBeenCalledWith([55]));
    expect(selectMessage).toHaveBeenCalledWith(null);
  });

  it("mientras no se sabe en qué carpeta está, Borrar y Mover a quedan deshabilitados", async () => {
    let resolve: (fs: Folder[]) => void = () => {};
    const pending = new Promise<Folder[]>((r) => (resolve = r));
    // La CancellablePromise de los bindings no hace falta para useQuery.
    vi.mocked(svc.ListFolders).mockReturnValue(pending as ReturnType<typeof svc.ListFolders>);
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MessageActions message={msg(3)} selectMessage={vi.fn()} />
      </QueryClientProvider>,
    );
    await openMenu(user);
    expect((await item("Borrar")).getAttribute("data-disabled")).not.toBeNull();
    expect((await item("Mover a")).getAttribute("data-disabled")).not.toBeNull();

    resolve(folders);
    expect(await item("Eliminar definitivamente")).toBeTruthy();
    await enabled("Mover a");
  });

  it("Destacar alterna", async () => {
    const { user } = setup(msg(1, false));
    await openMenu(user);
    await user.click(await item("Destacar"));
    await waitFor(() => expect(svc.SetFlagged).toHaveBeenCalledWith([55], true));
  });

  it("Quitar destacado cuando ya está destacado", async () => {
    const { user } = setup(msg(1, true));
    await openMenu(user);
    await user.click(await item("Quitar destacado"));
    await waitFor(() => expect(svc.SetFlagged).toHaveBeenCalledWith([55], false));
  });

  it("después de una acción rehace la búsqueda abierta", async () => {
    const qc = new QueryClient();
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const { user } = setup(msg(1), qc);
    await openMenu(user);
    await user.click(await item("Marcar como no leído"));
    await waitFor(() => expect(invalidate).toHaveBeenCalledWith({ queryKey: ["search"] }));
  });

  it("en Borradores muestra Seguir editando y abre el redactor con el borrador", async () => {
    const d = {
      id: 8, messageId: "<b8@x>", needsUpload: false, uploadError: "", updatedAt: "",
      request: { accountId: 7, subject: "s" },
    };
    vi.mocked(svc.OpenDraftMessage).mockResolvedValue(d as never);
    const { user } = setup(msg(5));
    const keep = await screen.findByRole("button", { name: "Seguir editando" });
    await user.click(keep);
    await waitFor(() => expect(composer.openComposeDraft).toHaveBeenCalledWith(d));
    expect(svc.OpenDraftMessage).toHaveBeenCalledWith(55);
  });

  it("fuera de Borradores no muestra Seguir editando", async () => {
    setup(msg(1));
    await screen.findByRole("button", { name: "Más acciones" });
    expect(screen.queryByRole("button", { name: "Seguir editando" })).toBeNull();
  });

  it("si no se puede abrir el borrador avisa", async () => {
    vi.mocked(svc.OpenDraftMessage).mockRejectedValue(new Error("Ese correo no es un borrador."));
    const { user } = setup(msg(5));
    await user.click(await screen.findByRole("button", { name: "Seguir editando" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Ese correo no es un borrador."));
    expect(composer.openComposeDraft).not.toHaveBeenCalled();
  });
});

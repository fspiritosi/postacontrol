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
  ArchiveMessages: vi.fn(() => Promise.resolve()),
  UndoMove: vi.fn(() => Promise.resolve()),
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
    hasRemoteImages: false, attachments: null, labels: [],
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

  it("Archivar lo manda a Archivo y cierra el lector", async () => {
    const { selectMessage, user } = setup(msg(1));
    await user.click(await screen.findByRole("button", { name: "Archivar" }));
    await waitFor(() => expect(svc.ArchiveMessages).toHaveBeenCalledWith([55]));
    expect(selectMessage).toHaveBeenCalledWith(null);
    expect(toast.success).toHaveBeenCalledWith("Archivado");
  });

  it("mover ofrece Deshacer, que devuelve el correo", async () => {
    vi.mocked(svc.MoveMessages).mockResolvedValueOnce(9 as never);
    const { user } = setup(msg(1));
    await user.click(await screen.findByRole("button", { name: "Mover a…" }));
    await user.click(await screen.findByRole("menuitem", { name: "Trabajo" }));
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

  it("en Archivo no se ofrece Archivar", async () => {
    setup(msg(6));
    await screen.findByRole("button", { name: "Mover a…" });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Archivar" })).toBeNull());
  });

  it("Borrar fuera de Papelera no pide confirmación", async () => {
    const { selectMessage, user } = setup(msg(1));
    const del = screen.getByRole("button", { name: "Borrar" });
    await waitFor(() => expect((del as HTMLButtonElement).disabled).toBe(false));
    await user.click(del);
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

  it("mientras no se sabe en qué carpeta está, Borrar y Mover a… quedan deshabilitados", async () => {
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
    const del = screen.getByRole("button", { name: "Borrar" }) as HTMLButtonElement;
    expect(del.disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Mover a…" }) as HTMLButtonElement).disabled).toBe(true);
    await user.click(del);
    expect(svc.DeleteMessages).not.toHaveBeenCalled();

    resolve(folders);
    const perm = await screen.findByRole("button", { name: "Eliminar definitivamente" });
    expect((perm as HTMLButtonElement).disabled).toBe(false);
    expect((screen.getByRole("button", { name: "Mover a…" }) as HTMLButtonElement).disabled).toBe(false);
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

  it("después de una acción rehace la búsqueda abierta", async () => {
    const qc = new QueryClient();
    const invalidate = vi.spyOn(qc, "invalidateQueries");
    const { user } = setup(msg(1), qc);
    await user.click(screen.getByRole("button", { name: "Marcar como no leído" }));
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
    // Borrar y mover siguen estando.
    expect(screen.getByRole("button", { name: "Borrar" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Mover a…" })).toBeTruthy();
    await user.click(keep);
    await waitFor(() => expect(composer.openComposeDraft).toHaveBeenCalledWith(d));
    expect(svc.OpenDraftMessage).toHaveBeenCalledWith(55);
  });

  it("fuera de Borradores no muestra Seguir editando", async () => {
    setup(msg(1));
    await waitFor(() => expect((screen.getByRole("button", { name: "Borrar" }) as HTMLButtonElement).disabled).toBe(false));
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

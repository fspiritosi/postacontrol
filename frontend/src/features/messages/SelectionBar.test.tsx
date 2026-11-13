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
  SnoozeMessages: vi.fn(() => Promise.resolve(0)),
  GetThread: vi.fn(),
  ListLabels: vi.fn(() => Promise.resolve([])),
  SetLabel: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type {
  Account,
  Folder,
} from "../../../bindings/postacontrol/core/model/models";
import { Security } from "../../../bindings/postacontrol/core/model/models";
import { threadLabelTargets, threadTargets, type Target } from "@/lib/bulk";
import { SelectionBar } from "./SelectionBar";

function account(id: number, displayName: string): Account {
  return {
    id,
    email: `${id}@b.c`,
    displayName,
    username: "a",
    color: "#000",
    imap: { host: "", port: 993, security: Security.SecurityTLS },
    smtp: { host: "", port: 465, security: Security.SecurityTLS },
    saveSentCopy: true,
    defaultSignatureId: 0,
  };
}
const accounts = [account(7, "Trabajo"), account(8, "")];

function folder(
  id: number,
  accountId: number,
  name: string,
  role = "",
  selectable = true,
): Folder {
  return {
    id,
    accountId,
    name,
    delimiter: "/",
    role,
    unread: 0,
    total: 0,
    selectable,
    position: 0,
  };
}
const FOLDERS: Record<number, Folder[]> = {
  7: [
    folder(70, 7, "INBOX", "inbox"),
    folder(71, 7, "Clientes"),
    folder(72, 7, "Trash", "trash"),
    folder(73, 7, "Sent", "sent"),
    folder(74, 7, "Archivo", "archive"),
  ],
  8: [folder(80, 8, "INBOX", "inbox")],
};

const t = (
  id: number,
  accountId: number,
  folderId: number,
  extra: Partial<Target> = {},
): Target => ({
  id,
  accountId,
  folderId,
  seen: true,
  flagged: false,
  labels: [],
  ...extra,
});

function renderBar(
  targets: Target[],
  over: Partial<React.ComponentProps<typeof SelectionBar>> = {},
) {
  const onDone = vi.fn();
  const onClear = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SelectionBar
        count={targets.length}
        accounts={accounts}
        folderId={70}
        targets={() => Promise.resolve(targets)}
        onDone={onDone}
        onClear={onClear}
        {...over}
      />
    </QueryClientProvider>,
  );
  return { onDone, onClear };
}

describe("SelectionBar", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListFolders).mockImplementation(((id: number) =>
      Promise.resolve(FOLDERS[id] ?? [])) as never);
  });

  it("Marcar leído llama SetSeen(ids, true) y suelta", async () => {
    const { onDone } = renderBar([t(1, 7, 70, { seen: false }), t(2, 7, 70)]);
    await userEvent.click(
      await screen.findByRole("button", { name: "Marcar leído" }),
    );
    await waitFor(() => expect(svc.SetSeen).toHaveBeenCalledWith([1, 2], true));
    expect(onDone).toHaveBeenCalledWith({
      ok: [1, 2],
      failed: [],
      removed: false,
      unread: false,
    });
  });

  it("Archivar llama ArchiveMessages por cuenta, avisa y suelta", async () => {
    const { onDone } = renderBar([t(1, 7, 70), t(2, 7, 70), t(3, 8, 80)]);
    await userEvent.click(await screen.findByRole("button", { name: "Archivar" }));
    await waitFor(() => expect(svc.ArchiveMessages).toHaveBeenCalledTimes(2));
    expect(svc.ArchiveMessages).toHaveBeenCalledWith([1, 2]);
    expect(svc.ArchiveMessages).toHaveBeenCalledWith([3]);
    expect(toast.success).toHaveBeenCalledWith("3 archivados");
    expect(onDone).toHaveBeenCalledWith(expect.objectContaining({ ok: [1, 2, 3], removed: true }));
  });

  it("Posponer pospone lo marcado por cuenta", async () => {
    const { onDone } = renderBar([t(1, 7, 70), t(2, 8, 80)]);
    await userEvent.click(await screen.findByRole("button", { name: "Posponer" }));
    await userEvent.click(await screen.findByRole("menuitem", { name: "Mañana, 8:00" }));
    await waitFor(() => expect(svc.SnoozeMessages).toHaveBeenCalledTimes(2));
    expect(vi.mocked(svc.SnoozeMessages).mock.calls.map((c) => c[0])).toEqual([[1], [2]]);
    expect(onDone).toHaveBeenCalledWith(expect.objectContaining({ ok: [1, 2], removed: true }));
  });

  it("si todo ya está en Archivo no se ofrece Archivar", async () => {
    renderBar([t(1, 7, 74)], { folderId: 74 });
    await screen.findByRole("button", { name: "Marcar leído" });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Archivar" })).toBeNull());
  });

  it("Marcar no leído llama SetSeen(ids, false)", async () => {
    renderBar([t(1, 7, 70), t(2, 7, 70)]);
    await userEvent.click(
      await screen.findByRole("button", { name: "Marcar no leído" }),
    );
    await waitFor(() =>
      expect(svc.SetSeen).toHaveBeenCalledWith([1, 2], false),
    );
  });

  it("Destacar con todos destacados dice Quitar destacado", async () => {
    renderBar([t(1, 7, 70, { flagged: true }), t(2, 7, 70, { flagged: true })]);
    await userEvent.click(
      await screen.findByRole("button", { name: "Quitar destacado" }),
    );
    await waitFor(() =>
      expect(svc.SetFlagged).toHaveBeenCalledWith([1, 2], false),
    );
  });

  it("Destacar con alguno sin destacar destaca todos", async () => {
    renderBar([t(1, 7, 70, { flagged: true }), t(2, 7, 70)]);
    await userEvent.click(
      await screen.findByRole("button", { name: "Destacar" }),
    );
    await waitFor(() =>
      expect(svc.SetFlagged).toHaveBeenCalledWith([1, 2], true),
    );
  });

  it("Mover a… lista las carpetas seleccionables de la cuenta salvo la actual y mueve", async () => {
    const { onDone } = renderBar([t(1, 7, 70), t(2, 7, 70)]);
    await userEvent.click(
      await screen.findByRole("button", { name: "Mover a…" }),
    );
    expect(
      await screen.findByRole("menuitem", { name: "Clientes" }),
    ).toBeTruthy();
    expect(screen.queryByRole("menuitem", { name: "Entrada" })).toBeNull();
    await userEvent.click(screen.getByRole("menuitem", { name: "Clientes" }));
    await waitFor(() =>
      expect(svc.MoveMessages).toHaveBeenCalledWith([1, 2], 71),
    );
    expect(toast.success).toHaveBeenCalledWith("2 movidos a Clientes");
    expect(onDone).toHaveBeenCalledWith({
      ok: [1, 2],
      failed: [],
      removed: true,
    });
  });

  it("Mover a… no aparece si son de dos cuentas", async () => {
    renderBar([t(1, 7, 70), t(2, 8, 80)]);
    // Con las carpetas cargadas (Eliminar habilitado) tampoco aparece.
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Eliminar" })).toHaveProperty(
        "disabled",
        false,
      ),
    );
    expect(screen.queryByRole("button", { name: "Mover a…" })).toBeNull();
  });

  it("Mover a… sí aparece con una sola cuenta (control)", async () => {
    renderBar([t(1, 7, 70), t(2, 7, 70)]);
    expect(
      await screen.findByRole("button", { name: "Mover a…" }),
    ).toBeTruthy();
  });

  it("Eliminar espera a conocer las carpetas", async () => {
    vi.mocked(svc.ListFolders).mockImplementation(
      (() => new Promise(() => {})) as never,
    );
    renderBar([t(1, 7, 72), t(2, 7, 72)], { folderId: 72 });
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Marcar leído" }),
      ).toHaveProperty("disabled", false),
    );
    expect(screen.getByRole("button", { name: "Eliminar" })).toHaveProperty(
      "disabled",
      true,
    );
    expect(svc.DeleteMessages).not.toHaveBeenCalled();
  });

  it("Marcar no leído avisa unread para cerrar el lector", async () => {
    const { onDone } = renderBar([t(1, 7, 70), t(2, 7, 70)]);
    await userEvent.click(
      await screen.findByRole("button", { name: "Marcar no leído" }),
    );
    await waitFor(() =>
      expect(onDone).toHaveBeenCalledWith({
        ok: [1, 2],
        failed: [],
        removed: false,
        unread: true,
      }),
    );
  });

  it("si no se pueden cargar los marcados avisa con un toast", async () => {
    renderBar([], { targets: () => Promise.reject(new Error("sin red")) });
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });

  it("Eliminar fuera de la Papelera no pregunta", async () => {
    const { onDone } = renderBar([t(1, 7, 70), t(2, 7, 70)]);
    await userEvent.click(
      await screen.findByRole("button", { name: "Eliminar" }),
    );
    await waitFor(() =>
      expect(svc.DeleteMessages).toHaveBeenCalledWith([1, 2]),
    );
    expect(toast.success).toHaveBeenCalledWith("2 movidos a la Papelera");
    expect(onDone).toHaveBeenCalledWith({
      ok: [1, 2],
      failed: [],
      removed: true,
    });
  });

  it("todos en la Papelera: Eliminar definitivamente pide confirmación", async () => {
    renderBar([t(1, 7, 72), t(2, 7, 72)], { folderId: 72 });
    await userEvent.click(
      await screen.findByRole("button", { name: "Eliminar definitivamente" }),
    );
    expect(svc.DeleteMessages).not.toHaveBeenCalled();
    expect(
      await screen.findByText(
        "¿Eliminar definitivamente 2 correos? No se puede deshacer.",
      ),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Eliminar" }));
    await waitFor(() =>
      expect(svc.DeleteMessages).toHaveBeenCalledWith([1, 2]),
    );
    expect(toast.success).not.toHaveBeenCalled();
  });

  it("mezcla con la Papelera pide confirmación", async () => {
    renderBar([t(1, 7, 70), t(2, 7, 72), t(3, 7, 70)], { folderId: null });
    await userEvent.click(
      await screen.findByRole("button", { name: "Eliminar" }),
    );
    expect(svc.DeleteMessages).not.toHaveBeenCalled();
    expect(
      await screen.findByText(
        "1 de los 3 correos están en la Papelera y se eliminan definitivamente. Los demás van a la Papelera. No se puede deshacer.",
      ),
    ).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: "Eliminar" }));
    await waitFor(() =>
      expect(svc.DeleteMessages).toHaveBeenCalledWith([1, 2, 3]),
    );
  });

  it("una conversación marcada no toca lo de Enviados", async () => {
    vi.mocked(svc.GetThread).mockResolvedValue([
      { id: 1, accountId: 7, folderId: 70, seen: true, flagged: false, labels: [] },
      { id: 2, accountId: 7, folderId: 73, seen: true, flagged: false, labels: [] },
    ] as never);
    renderBar([], {
      threads: true,
      count: 2,
      targets: () => threadTargets(70, ["a", "b"]),
    });
    await userEvent.click(
      await screen.findByRole("button", { name: "Eliminar" }),
    );
    await waitFor(() => expect(svc.DeleteMessages).toHaveBeenCalled());
    // GetThread devuelve lo mismo para las dos raíces: solo importa que 2 (Enviados) no esté.
    for (const [ids] of vi.mocked(svc.DeleteMessages).mock.calls)
      expect(ids).not.toContain(2);
    expect(vi.mocked(svc.DeleteMessages).mock.calls[0][0]).toContain(1);
  });

  it("con una conversación marcada, Etiquetas actúa también sobre lo de Enviados", async () => {
    vi.mocked(svc.ListLabels).mockResolvedValue([
      { id: 11, name: "Clientes", color: "#dc2626", position: 0 },
    ] as never);
    vi.mocked(svc.GetThread).mockResolvedValue([
      { id: 1, accountId: 7, folderId: 70, seen: true, flagged: false, labels: [11] },
      { id: 2, accountId: 7, folderId: 73, seen: true, flagged: false, labels: [11] },
    ] as never);
    renderBar([], {
      threads: true,
      count: 1,
      targets: () => threadTargets(70, ["a"]),
      labelTargets: () => threadLabelTargets(70, ["a"]),
    });
    const button = await screen.findByRole("button", { name: "Etiquetas" });
    await waitFor(() => expect(button).toBeEnabled());
    await userEvent.click(button);
    const item = await screen.findByRole("menuitem", { name: /Clientes/ });
    await waitFor(() =>
      expect(item.querySelector('[data-testid="label-all"]')).not.toBeNull(),
    );
    await userEvent.click(item);
    await waitFor(() =>
      expect(svc.SetLabel).toHaveBeenCalledWith([1, 2], 11, false),
    );
  });

  it("falla parcial deja marcados los de esa cuenta", async () => {
    vi.mocked(svc.SetSeen).mockImplementation(((ids: number[]) =>
      ids.includes(2)
        ? Promise.reject(new Error("sin conexión"))
        : Promise.resolve()) as never);
    const { onDone } = renderBar([t(1, 7, 70), t(2, 8, 80), t(3, 8, 80)], {
      folderId: null,
    });
    await userEvent.click(
      await screen.findByRole("button", { name: "Marcar no leído" }),
    );
    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(svc.SetSeen).toHaveBeenCalledTimes(2);
    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(vi.mocked(toast.error).mock.calls[0][0]).toMatch(
      /^No se pudo en 8@b\.c: /,
    );
    expect(onDone).toHaveBeenCalledWith({
      ok: [1],
      failed: [2, 3],
      removed: false,
      unread: true,
    });
  });

  it("muestra el conteo y quita la selección", async () => {
    const { onClear } = renderBar([t(1, 7, 70), t(2, 7, 70)]);
    expect(screen.getByText("2 seleccionados")).toBeTruthy();
    await userEvent.click(
      screen.getByRole("button", { name: "Quitar selección" }),
    );
    expect(onClear).toHaveBeenCalled();
  });

  it("el botón Etiquetas muestra la raya si solo algunos tienen la etiqueta y la pone en todos", async () => {
    vi.mocked(svc.ListLabels).mockResolvedValue([
      { id: 11, name: "Clientes", color: "#dc2626", position: 0 },
    ] as never);
    renderBar([t(1, 7, 70, { labels: [11] }), t(2, 7, 70)]);
    const button = await screen.findByRole("button", { name: "Etiquetas" });
    await waitFor(() => expect(button).toBeEnabled());
    await userEvent.click(button);
    expect(await screen.findByTestId("label-some")).toBeTruthy();
    await userEvent.click(screen.getByRole("menuitem", { name: /Clientes/ }));
    await waitFor(() =>
      expect(svc.SetLabel).toHaveBeenCalledWith([1, 2], 11, true),
    );
  });
});

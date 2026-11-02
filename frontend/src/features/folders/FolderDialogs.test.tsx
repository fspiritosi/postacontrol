import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  CreateFolder: vi.fn(() => Promise.resolve({})),
  RenameFolder: vi.fn(() => Promise.resolve({})),
  MoveFolder: vi.fn(() => Promise.resolve({})),
  DeleteFolderPreview: vi.fn(() => Promise.resolve({ total: 0, subfolders: [] })),
  DeleteFolder: vi.fn(() => Promise.resolve(0)),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Folder } from "../../../bindings/postacontrol/core/model/models";
import { buildFolderTree, type FolderNode } from "@/lib/folders";
import { viewIsValid } from "@/lib/view";
import { toast } from "sonner";
import { DeleteDialog, MoveDialog, NameDialog, deleteDescription } from "./FolderDialogs";

const folder = (id: number, name: string, role = ""): Folder => ({
  id, accountId: 7, name, delimiter: "/", role, unread: 0, total: 0, selectable: true, position: 0,
});
const clientes: FolderNode = buildFolderTree([folder(10, "Clientes")])[0];

function wrap(ui: React.ReactNode, client = new QueryClient()) {
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

// Un QueryClient que anota qué consultas se invalidaron.
function spiedClient() {
  const client = new QueryClient();
  const spy = vi.spyOn(client, "invalidateQueries");
  const invalidated = () => spy.mock.calls.map(([f]) => JSON.stringify(f?.queryKey));
  return { client, invalidated };
}

const findIn = (list: FolderNode[], name: string): FolderNode | undefined => {
  for (const n of list) {
    if (n.name === name) return n;
    const r = findIn(n.children, name);
    if (r) return r;
  }
};

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
});
beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(svc.DeleteFolderPreview).mockResolvedValue({ total: 0, subfolders: [] });
});

describe("NameDialog", () => {
  it("un error del núcleo se muestra en el diálogo y no lo cierra", async () => {
    vi.mocked(svc.CreateFolder).mockRejectedValueOnce(new Error("Ya hay una carpeta «Clientes» ahí."));
    const onClose = vi.fn();
    const user = userEvent.setup();
    wrap(<NameDialog mode={{ kind: "create", accountId: 7 }} onClose={onClose} />);
    await user.type(screen.getByLabelText("Nombre"), "Clientes");
    await user.click(screen.getByRole("button", { name: "Crear" }));
    expect((await screen.findByRole("alert")).textContent).toContain("Ya hay una carpeta");
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Nombre")).toBeTruthy();
  });

  it("renombrar invalida también las consultas de correo (búsqueda incluida)", async () => {
    const { client, invalidated } = spiedClient();
    const onClose = vi.fn();
    const user = userEvent.setup();
    wrap(<NameDialog mode={{ kind: "rename", node: clientes }} onClose={onClose} />, client);
    await user.clear(screen.getByLabelText("Nombre"));
    await user.type(screen.getByLabelText("Nombre"), "Otros");
    await user.click(screen.getByRole("button", { name: "Renombrar" }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(invalidated()).toContain(JSON.stringify(["search"]));
    expect(invalidated()).toContain(JSON.stringify(["messages"]));
  });

  it("Renombrar viene con el nombre actual", () => {
    wrap(<NameDialog mode={{ kind: "rename", node: clientes }} onClose={vi.fn()} />);
    expect(screen.getByText("Renombrar «Clientes»")).toBeTruthy();
    expect((screen.getByLabelText("Nombre") as HTMLInputElement).value).toBe("Clientes");
  });
});

describe("MoveDialog", () => {
  const list = [
    folder(3, "INBOX", "inbox"),
    folder(5, "INBOX/Varios"),
    folder(10, "Clientes"),
    folder(11, "Clientes/Acme"),
    folder(12, "Zeta"),
  ];
  const tree = buildFolderTree(list);
  const labels = () => screen.getAllByRole("radio").map((r) => r.closest("label")!.textContent);

  it("no ofrece Primer nivel a una que cuelga de INBOX (ya está ahí con el prefijo «INBOX.»)", () => {
    wrap(<MoveDialog node={findIn(tree, "INBOX/Varios")!} tree={tree} onClose={vi.fn()} />);
    expect(labels()).not.toContain("Primer nivel");
    expect(labels()).toContain("Zeta");
  });

  it("a una subcarpeta común sí le ofrece Primer nivel", () => {
    wrap(<MoveDialog node={findIn(tree, "Clientes/Acme")!} tree={tree} onClose={vi.fn()} />);
    expect(labels()).toContain("Primer nivel");
  });

  it("mover invalida también las consultas de correo", async () => {
    const { client, invalidated } = spiedClient();
    const onClose = vi.fn();
    const user = userEvent.setup();
    wrap(<MoveDialog node={findIn(tree, "Clientes/Acme")!} tree={tree} onClose={onClose} />, client);
    await user.click(screen.getByLabelText("Zeta"));
    await user.click(screen.getByRole("button", { name: "Mover" }));
    await waitFor(() => expect(svc.MoveFolder).toHaveBeenCalledWith(11, 12));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(invalidated()).toContain(JSON.stringify(["search"]));
  });
});

describe("deleteDescription", () => {
  it("sin correos pero con subcarpetas no habla de correos", () => {
    expect(
      deleteDescription({ total: 0, subfolders: [{ name: "C/A", total: 0 }, { name: "C/B", total: 0 }] }, "/"),
    ).toBe("Tiene 2 subcarpetas («A», con 0 correos; «B», con 0 correos). Las carpetas se borran del servidor.");
    expect(deleteDescription({ total: 0, subfolders: [{ name: "C/A", total: 0 }] }, "/")).toBe(
      "Tiene 1 subcarpeta («A», con 0 correos). Las carpetas se borran del servidor.",
    );
  });

  it("un solo correo va en singular", () => {
    expect(deleteDescription({ total: 1, subfolders: [] }, "/")).toBe(
      "Tiene 1 correo. El correo va a la Papelera y las carpetas se borran del servidor.",
    );
  });
});

describe("DeleteDialog", () => {
  it("muestra Contando correos… y después el texto con los conteos del preview", async () => {
    vi.mocked(svc.DeleteFolderPreview).mockResolvedValue({
      total: 48,
      subfolders: [{ name: "Clientes/Acme", total: 7 }],
    });
    wrap(<DeleteDialog node={clientes} onClose={vi.fn()} />);
    expect(screen.getByText("Contando correos…")).toBeTruthy();
    expect((screen.getByRole("button", { name: "Eliminar" }) as HTMLButtonElement).disabled).toBe(true);
    expect(
      await screen.findByText(
        "Tiene 48 correos y 1 subcarpeta («Acme», con 7 correos). Los 55 correos van a la Papelera y las carpetas se borran del servidor.",
      ),
    ).toBeTruthy();
  });

  it("muestra solo el último segmento del nombre de cada subcarpeta", async () => {
    vi.mocked(svc.DeleteFolderPreview).mockResolvedValue({
      total: 1,
      subfolders: [{ name: "Clientes/Acme", total: 2 }, { name: "Clientes/Acme/2025", total: 3 }],
    });
    wrap(<DeleteDialog node={clientes} onClose={vi.fn()} />);
    expect(
      await screen.findByText(
        "Tiene 1 correo y 2 subcarpetas («Acme», con 2 correos; «2025», con 3 correos). Los 6 correos van a la Papelera y las carpetas se borran del servidor.",
      ),
    ).toBeTruthy();
  });

  it("en una vacía no lleva descripción", async () => {
    wrap(<DeleteDialog node={clientes} onClose={vi.fn()} />);
    await waitFor(() => expect(screen.queryByText("Contando correos…")).toBeNull());
    expect(screen.queryByText(/Tiene/)).toBeNull();
    await waitFor(() =>
      expect((screen.getByRole("button", { name: "Eliminar" }) as HTMLButtonElement).disabled).toBe(false),
    );
  });

  it("si el preview falla muestra el error y no deja eliminar", async () => {
    vi.mocked(svc.DeleteFolderPreview).mockRejectedValueOnce(new Error("sin conexión"));
    wrap(<DeleteDialog node={clientes} onClose={vi.fn()} />);
    expect((await screen.findByRole("alert")).textContent).toContain("sin conexión");
    expect((screen.getByRole("button", { name: "Eliminar" }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("mientras elimina dice Eliminando… y avisa al terminar sin correos", async () => {
    let done!: (n: number) => void;
    vi.mocked(svc.DeleteFolder).mockReturnValueOnce(new Promise((r) => (done = r)) as never);
    const onClose = vi.fn();
    const user = userEvent.setup();
    wrap(<DeleteDialog node={clientes} onClose={onClose} />);
    await waitFor(() =>
      expect((screen.getByRole("button", { name: "Eliminar" }) as HTMLButtonElement).disabled).toBe(false),
    );
    await user.click(screen.getByRole("button", { name: "Eliminar" }));
    const busy = await screen.findByRole("button", { name: "Eliminando…" });
    expect((busy as HTMLButtonElement).disabled).toBe(true);
    done(0);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(toast.success).toHaveBeenCalledWith("Carpeta eliminada");
  });

  it("con un solo correo el aviso va en singular e invalida las consultas de correo", async () => {
    vi.mocked(svc.DeleteFolderPreview).mockResolvedValue({ total: 1, subfolders: [] });
    vi.mocked(svc.DeleteFolder).mockResolvedValueOnce(1);
    const { client, invalidated } = spiedClient();
    const onClose = vi.fn();
    const user = userEvent.setup();
    wrap(<DeleteDialog node={clientes} onClose={onClose} />, client);
    await waitFor(() =>
      expect((screen.getByRole("button", { name: "Eliminar" }) as HTMLButtonElement).disabled).toBe(false),
    );
    await user.click(screen.getByRole("button", { name: "Eliminar" }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(toast.success).toHaveBeenCalledWith("Carpeta eliminada; 1 correo a la Papelera");
    expect(invalidated()).toContain(JSON.stringify(["search"]));
  });

  it("si falla a mitad, vuelve a contar en el servidor y muestra lo que queda", async () => {
    vi.mocked(svc.DeleteFolderPreview)
      .mockResolvedValueOnce({ total: 55, subfolders: [] })
      .mockResolvedValueOnce({ total: 20, subfolders: [] });
    vi.mocked(svc.DeleteFolder).mockRejectedValueOnce(new Error("se cortó la conexión"));
    const { client, invalidated } = spiedClient();
    const onClose = vi.fn();
    const user = userEvent.setup();
    wrap(<DeleteDialog node={clientes} onClose={onClose} />, client);
    expect(await screen.findByText(/Los 55 correos/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Eliminar" }));
    expect((await screen.findByRole("alert")).textContent).toContain("se cortó la conexión");
    expect(await screen.findByText(/Los 20 correos/)).toBeTruthy();
    expect(screen.queryByText(/Los 55 correos/)).toBeNull();
    expect(svc.DeleteFolderPreview).toHaveBeenCalledTimes(2);
    expect(onClose).not.toHaveBeenCalled();
    // Algunos correos ya pudieron ir a la Papelera.
    expect(invalidated()).toContain(JSON.stringify(["search"]));
    await waitFor(() =>
      expect((screen.getByRole("button", { name: "Eliminar" }) as HTMLButtonElement).disabled).toBe(false),
    );
  });

  it("después de eliminar, si estaba abierta, la vista deja de ser válida y MailShell vuelve a la de por defecto", () => {
    const view = { kind: "folder", folderId: 10 } as const;
    expect(viewIsValid(view, [folder(10, "Clientes")], false)).toBe(true);
    expect(viewIsValid(view, [], false)).toBe(false);
  });
});

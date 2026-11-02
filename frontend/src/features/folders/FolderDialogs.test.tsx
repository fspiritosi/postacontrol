import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  CreateFolder: vi.fn(() => Promise.resolve({})),
  RenameFolder: vi.fn(() => Promise.resolve({})),
  DeleteFolderPreview: vi.fn(() => Promise.resolve({ total: 0, subfolders: [] })),
  DeleteFolder: vi.fn(() => Promise.resolve(0)),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Folder } from "../../../bindings/postacontrol/core/model/models";
import { buildFolderTree, type FolderNode } from "@/lib/folders";
import { viewIsValid } from "@/lib/view";
import { toast } from "sonner";
import { DeleteDialog, NameDialog } from "./FolderDialogs";

const folder = (id: number, name: string): Folder => ({
  id, accountId: 7, name, delimiter: "/", role: "", unread: 0, total: 0, selectable: true, position: 0,
});
const clientes: FolderNode = buildFolderTree([folder(10, "Clientes")])[0];

function wrap(ui: React.ReactNode) {
  return render(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>);
}

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

  it("Renombrar viene con el nombre actual", () => {
    wrap(<NameDialog mode={{ kind: "rename", node: clientes }} onClose={vi.fn()} />);
    expect(screen.getByText("Renombrar «Clientes»")).toBeTruthy();
    expect((screen.getByLabelText("Nombre") as HTMLInputElement).value).toBe("Clientes");
  });
});

describe("DeleteDialog", () => {
  it("muestra Contando correos… y después el texto con los conteos del preview", async () => {
    vi.mocked(svc.DeleteFolderPreview).mockResolvedValue({
      total: 48,
      subfolders: [{ name: "Acme", total: 7 }],
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

  it("después de eliminar, si estaba abierta, la vista deja de ser válida y MailShell vuelve a la de por defecto", () => {
    const view = { kind: "folder", folderId: 10 } as const;
    expect(viewIsValid(view, [folder(10, "Clientes")], false)).toBe(true);
    expect(viewIsValid(view, [], false)).toBe(false);
  });
});

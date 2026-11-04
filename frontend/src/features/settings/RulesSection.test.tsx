import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListAccounts: vi.fn(),
  ListFolders: vi.fn(),
  ListLabels: vi.fn(),
  ListRules: vi.fn(),
  SaveRule: vi.fn(),
  DeleteRule: vi.fn(),
  ReorderRules: vi.fn(),
  RunRules: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { RuleDraft } from "@/lib/events";
import { RulesSection } from "./RulesSection";

const account = { id: 1, email: "yo@ej.com", displayName: "Yo", color: "#2563eb" };
const folders = [
  { id: 10, accountId: 1, name: "INBOX", delimiter: ".", role: "inbox", selectable: true },
  { id: 11, accountId: 1, name: "Banco", delimiter: ".", role: "", selectable: true },
  { id: 12, accountId: 1, name: "Trash", delimiter: ".", role: "trash", selectable: true },
];
const rules = [
  {
    id: 100, accountId: 1, name: "Galicia", enabled: true, matchAll: true, position: 1,
    conditions: [{ field: "from", op: "endsWith", value: "@galicia.com.ar" }],
    actions: [{ type: "move", folderId: 11, labelId: 0 }],
  },
  {
    id: 101, accountId: 1, name: "Viejo", enabled: true, matchAll: true, position: 2,
    conditions: [{ field: "subject", op: "contains", value: "x" }],
    actions: [{ type: "move", folderId: 999, labelId: 0 }],
  },
];

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
});

function setup(draft: RuleDraft | null = null, onDraftDone = vi.fn()) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <RulesSection draft={draft} onDraftDone={onDraftDone} />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

async function pick(user: ReturnType<typeof userEvent.setup>, combobox: string, option: string) {
  await user.click(screen.getByRole("combobox", { name: combobox }));
  await user.click(await screen.findByRole("option", { name: option }));
}

describe("Reglas en Configuración", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([account] as never);
    vi.mocked(svc.ListFolders).mockResolvedValue(folders as never);
    vi.mocked(svc.ListLabels).mockResolvedValue([]);
    vi.mocked(svc.ListRules).mockResolvedValue(rules as never);
    vi.mocked(svc.SaveRule).mockImplementation((r) => Promise.resolve(r) as never);
    vi.mocked(svc.DeleteRule).mockResolvedValue();
    vi.mocked(svc.ReorderRules).mockResolvedValue();
    vi.mocked(svc.RunRules).mockResolvedValue({ matched: 3 });
  });

  it("lista las reglas con su resumen y avisa la que usa una carpeta que ya no existe", async () => {
    setup();
    expect(await screen.findByText("Galicia")).toBeTruthy();
    await screen.findByText("Si De termina con «@galicia.com.ar» → Mover a Banco");
    expect(screen.getByText("Si Asunto contiene «x» → Mover a una carpeta que ya no existe")).toBeTruthy();
    expect(screen.getAllByLabelText("Usa una carpeta o etiqueta que ya no existe")).toHaveLength(1);
  });

  it("sin reglas explica para qué sirven", async () => {
    vi.mocked(svc.ListRules).mockResolvedValue([]);
    setup();
    expect(await screen.findByText(/Esta cuenta no tiene reglas/)).toBeTruthy();
    expect(screen.queryByText("Aplicar ahora")).toBeNull();
  });

  it("Nueva regla guarda nombre, condición y carpeta", async () => {
    vi.mocked(svc.ListRules).mockResolvedValue([]);
    const user = setup();
    await screen.findByText(/Esta cuenta no tiene reglas/);
    await user.click(screen.getByRole("button", { name: "Nueva regla" }));
    await user.type(await screen.findByLabelText("Nombre"), "Banco");
    await pick(user, "Comparación de la condición 1", "termina con");
    await user.type(screen.getByLabelText("Texto de la condición 1"), "@galicia.com.ar");
    await pick(user, "Carpeta de la acción 1", "Banco");
    await user.click(screen.getByRole("button", { name: "Agregar acción" }));
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(svc.SaveRule).toHaveBeenCalledWith({
        id: 0, accountId: 1, name: "Banco", enabled: true, matchAll: true, position: 0,
        conditions: [{ field: "from", op: "endsWith", value: "@galicia.com.ar" }],
        actions: [{ type: "move", folderId: 11, labelId: 0 }, { type: "markRead", folderId: 0, labelId: 0 }],
      }),
    );
    await waitFor(() => expect(screen.queryByRole("heading", { name: "Nueva regla" })).toBeNull());
  });

  it("el error del núcleo se muestra en el editor", async () => {
    vi.mocked(svc.SaveRule).mockRejectedValue(new Error("Poné un nombre para la regla."));
    const user = setup();
    await screen.findByText("Galicia");
    await user.click(screen.getByRole("button", { name: "Nueva regla" }));
    await user.click(await screen.findByRole("button", { name: "Guardar" }));
    expect(await screen.findByText("Poné un nombre para la regla.")).toBeTruthy();
  });

  it("editar precarga la regla y conserva su id", async () => {
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Editar Galicia" }));
    expect((await screen.findByLabelText<HTMLInputElement>("Nombre")).value).toBe("Galicia");
    expect(screen.getByLabelText<HTMLInputElement>("Texto de la condición 1").value).toBe("@galicia.com.ar");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(svc.SaveRule).toHaveBeenCalledWith(expect.objectContaining({ id: 100, name: "Galicia" })));
  });

  it("el interruptor, el orden y borrar llaman al núcleo", async () => {
    const user = setup();
    await user.click(await screen.findByRole("switch", { name: "Regla Galicia prendida" }));
    await waitFor(() => expect(svc.SaveRule).toHaveBeenCalledWith(expect.objectContaining({ id: 100, enabled: false })));
    await user.click(screen.getByRole("button", { name: "Bajar Galicia" }));
    await waitFor(() => expect(svc.ReorderRules).toHaveBeenCalledWith(1, [101, 100]));
    await user.click(screen.getByRole("button", { name: "Borrar Viejo" }));
    const dialog = await screen.findByRole("dialog");
    await user.click(within(dialog).getByRole("button", { name: "Borrar" }));
    await waitFor(() => expect(svc.DeleteRule).toHaveBeenCalledWith(101));
  });

  it("Aplicar corre las reglas en la Entrada y avisa cuántos correos", async () => {
    const user = setup();
    await screen.findByText("Galicia");
    await waitFor(() => expect(screen.getByRole("button", { name: "Aplicar" }).hasAttribute("disabled")).toBe(false));
    await user.click(screen.getByRole("button", { name: "Aplicar" }));
    await waitFor(() => expect(svc.RunRules).toHaveBeenCalledWith(10));
    expect(toast.success).toHaveBeenCalledWith("Las reglas se aplicaron a 3 correos.");
  });

  it("una regla pedida desde un correo abre el editor completado", async () => {
    const onDraftDone = vi.fn();
    const user = setup(
      { accountId: 1, name: "Bea", conditions: [{ field: "from", op: "is", value: "bea@ej.com" }] },
      onDraftDone,
    );
    expect(await screen.findByRole("heading", { name: "Nueva regla" })).toBeTruthy();
    expect(screen.getByLabelText<HTMLInputElement>("Nombre").value).toBe("Bea");
    expect(screen.getByLabelText<HTMLInputElement>("Texto de la condición 1").value).toBe("bea@ej.com");
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(onDraftDone).toHaveBeenCalled();
  });
});

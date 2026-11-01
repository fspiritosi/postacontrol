import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListAccounts: vi.fn(),
  GetContact: vi.fn(),
  SaveContact: vi.fn(),
  DeleteContact: vi.fn(),
}));
const openNew = vi.fn();
vi.mock("../compose/ComposerContext", () => ({ useComposer: () => ({ openNew }) }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { ShellState } from "../layout/MailShell";
import { ContactPanel } from "./ContactPanel";

const ana = {
  id: 5,
  name: "Ana Pérez",
  emails: ["ana@ej.com", "ana2@ej.com"],
  phones: ["+54 11 5555"],
  company: "ACME",
  notes: "Conocida de la facultad",
};

// Radix Dialog pide APIs que jsdom no trae.
beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
});

function setup(contactId: number | null) {
  const selectContact = vi.fn();
  const state: ShellState = {
    view: { kind: "contacts" },
    messageId: null,
    selectView: vi.fn(),
    selectMessage: vi.fn(),
    search: null,
    setSearch: vi.fn(),
    threadRoot: null,
    selectThread: vi.fn(),
    contactId,
    selectContact,
    openContact: vi.fn(),
  };
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ContactPanel state={state} />
    </QueryClientProvider>,
  );
  return { user, selectContact };
}

describe("ContactPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([{ id: 3 }, { id: 9 }] as never);
    vi.mocked(svc.GetContact).mockResolvedValue(ana);
  });

  it("sin contacto elegido invita a elegir o crear", () => {
    setup(null);
    expect(screen.getByText("Elegí un contacto o creá uno nuevo.")).toBeTruthy();
  });

  it("la ficha muestra correos, teléfonos, empresa y notas", async () => {
    setup(5);
    expect(await screen.findByText("ana@ej.com")).toBeTruthy();
    expect(screen.getByText("ana2@ej.com")).toBeTruthy();
    expect(screen.getByText("+54 11 5555")).toBeTruthy();
    expect(screen.getByText("ACME")).toBeTruthy();
    expect(screen.getByText("Conocida de la facultad")).toBeTruthy();
  });

  it("Editar, cambiar la empresa y Guardar manda la ficha completa", async () => {
    vi.mocked(svc.SaveContact).mockResolvedValue({ ...ana, company: "Otra" });
    const { user, selectContact } = setup(5);
    await user.click(await screen.findByRole("button", { name: "Editar" }));
    const empresa = screen.getByLabelText("Empresa");
    await user.clear(empresa);
    await user.type(empresa, "Otra");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(svc.SaveContact).toHaveBeenCalledWith({ ...ana, company: "Otra" }));
    // Sigue en la misma ficha, ya en modo lectura.
    expect(await screen.findByRole("button", { name: "Editar" })).toBeTruthy();
    expect(selectContact).not.toHaveBeenCalled();
  });

  it("Agregar correo suma un campo", async () => {
    const { user } = setup(5);
    await user.click(await screen.findByRole("button", { name: "Editar" }));
    expect(screen.getAllByLabelText(/^Correo \d/)).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: "Agregar correo" }));
    expect(screen.getAllByLabelText(/^Correo \d/)).toHaveLength(3);
  });

  it("una ficha nueva se guarda con id 0 y después se abre", async () => {
    vi.mocked(svc.SaveContact).mockResolvedValue({ ...ana, id: 77, name: "Beto", emails: ["b@ej.com"] });
    const { user, selectContact } = setup(0);
    await user.type(screen.getByLabelText("Nombre"), "Beto");
    await user.type(screen.getByLabelText("Correo 1"), "b@ej.com");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() =>
      expect(svc.SaveContact).toHaveBeenCalledWith({
        id: 0, name: "Beto", emails: ["b@ej.com"], phones: [], company: "", notes: "",
      }),
    );
    await waitFor(() => expect(selectContact).toHaveBeenCalledWith(77));
  });

  it("si el núcleo rechaza, muestra el error y deja abierto el formulario", async () => {
    vi.mocked(svc.SaveContact).mockRejectedValue(new Error("«x» ya está en la ficha de Ana."));
    const { user } = setup(0);
    await user.type(screen.getByLabelText("Correo 1"), "x");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    const alert = await screen.findByRole("alert");
    expect(alert.textContent).toContain("«x» ya está en la ficha de Ana.");
    expect(screen.getByRole("button", { name: "Guardar" })).toBeTruthy();
  });

  it("Cancelar en una ficha nueva la cierra", async () => {
    const { user, selectContact } = setup(0);
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(selectContact).toHaveBeenCalledWith(null);
  });

  it("Eliminar pide confirmación y borra", async () => {
    vi.mocked(svc.DeleteContact).mockResolvedValue(undefined as never);
    const { user, selectContact } = setup(5);
    await user.click(await screen.findByRole("button", { name: "Eliminar" }));
    expect(await screen.findByText("¿Eliminar a «Ana Pérez»?")).toBeTruthy();
    expect(screen.getByText("Se borra de la libreta. Los correos no se tocan.")).toBeTruthy();
    expect(svc.DeleteContact).not.toHaveBeenCalled();
    const buttons = screen.getAllByRole("button", { name: "Eliminar" });
    await user.click(buttons[buttons.length - 1]);
    await waitFor(() => expect(svc.DeleteContact).toHaveBeenCalledWith(5));
    await waitFor(() => expect(selectContact).toHaveBeenCalledWith(null));
  });

  it("Escribirle abre el redactor con la primera cuenta y el primer correo", async () => {
    const { user } = setup(5);
    await user.click(await screen.findByRole("button", { name: "Escribirle" }));
    expect(openNew).toHaveBeenCalledWith(3, [{ name: "Ana Pérez", email: "ana@ej.com" }]);
  });

  it("sin correos no hay Escribirle", async () => {
    vi.mocked(svc.GetContact).mockResolvedValue({ ...ana, emails: [] });
    setup(5);
    await screen.findByRole("button", { name: "Editar" });
    expect(screen.queryByRole("button", { name: "Escribirle" })).toBeNull();
  });
});

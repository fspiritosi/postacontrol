import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListContacts: vi.fn(),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { ShellState } from "../layout/MailShell";
import { ContactList } from "./ContactList";

const contacts = [
  { id: 1, name: "Ana Pérez", emails: ["ana@ej.com"], phones: [], company: "", notes: "" },
  { id: 2, name: "", emails: ["sin@nombre.com"], phones: [], company: "", notes: "" },
];

function setup(contactId: number | null = null) {
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
      <ContactList state={state} />
    </QueryClientProvider>,
  );
  return { user, selectContact };
}

describe("ContactList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListContacts).mockResolvedValue(contacts);
  });

  it("lista los nombres que se muestran (sin nombre, el primer correo)", async () => {
    setup();
    expect(await screen.findByText("Ana Pérez")).toBeTruthy();
    expect(screen.getByText("sin@nombre.com")).toBeTruthy();
  });

  it("escribir en el buscador consulta con el texto", async () => {
    const { user } = setup();
    await screen.findByText("Ana Pérez");
    await user.type(screen.getByPlaceholderText("Buscar contactos"), "ana");
    await waitFor(() => expect(svc.ListContacts).toHaveBeenLastCalledWith("ana"));
  });

  it("sin contactos avisa que la libreta está vacía", async () => {
    vi.mocked(svc.ListContacts).mockResolvedValue(null);
    setup();
    expect(await screen.findByText("Todavía no tenés contactos.")).toBeTruthy();
  });

  it("con búsqueda y sin resultados lo dice", async () => {
    vi.mocked(svc.ListContacts).mockImplementation(((q: string) =>
      Promise.resolve(q ? [] : contacts)) as never);
    const { user } = setup();
    await screen.findByText("Ana Pérez");
    await user.type(screen.getByPlaceholderText("Buscar contactos"), "zzz");
    expect(await screen.findByText("Ningún contacto coincide con «zzz».")).toBeTruthy();
  });

  it("mientras busca deja la lista anterior a la vista", async () => {
    vi.mocked(svc.ListContacts).mockImplementation(((q: string) =>
      q ? new Promise(() => {}) : Promise.resolve(contacts)) as never);
    const { user } = setup();
    await screen.findByText("Ana Pérez");
    await user.type(screen.getByPlaceholderText("Buscar contactos"), "a");
    await waitFor(() => expect(svc.ListContacts).toHaveBeenLastCalledWith("a"));
    expect(screen.getByText("Ana Pérez")).toBeTruthy();
  });

  it("mientras busca no dice que no hay resultados con la lista anterior vacía", async () => {
    vi.mocked(svc.ListContacts).mockImplementation(((q: string) =>
      q === "zz" ? Promise.resolve([]) : q === "z" ? new Promise(() => {}) : Promise.resolve(contacts)) as never);
    const { user } = setup();
    await screen.findByText("Ana Pérez");
    const box = screen.getByPlaceholderText("Buscar contactos");
    await user.type(box, "zz");
    await screen.findByText("Ningún contacto coincide con «zz».");
    // "z" quedó colgada desde que se escribió: vuelve a esa búsqueda, todavía sin respuesta.
    await user.type(box, "{Backspace}");
    expect(box).toHaveValue("z");
    expect(screen.queryByText("Ningún contacto coincide con «z».")).toBeNull();
  });

  it("Nuevo contacto abre una ficha nueva", async () => {
    const { user, selectContact } = setup();
    await user.click(screen.getByRole("button", { name: "Nuevo contacto" }));
    expect(selectContact).toHaveBeenCalledWith(0);
  });

  it("click en una fila abre su ficha y marca la elegida", async () => {
    const { user, selectContact } = setup(2);
    await user.click(await screen.findByRole("button", { name: /Ana Pérez/ }));
    expect(selectContact).toHaveBeenCalledWith(1);
    expect(screen.getByRole("button", { name: /sin@nombre.com/ }).getAttribute("aria-current")).toBe("true");
  });
});

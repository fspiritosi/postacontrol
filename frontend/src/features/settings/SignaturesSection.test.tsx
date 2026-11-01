import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() => Promise.resolve({ unifiedInbox: false, disableNotifications: false, threadedList: false })),
  SetSettings: vi.fn(() => Promise.resolve()),
  GetStartAtLogin: vi.fn(() => Promise.resolve(false)),
  SetStartAtLogin: vi.fn(() => Promise.resolve()),
  ListAccounts: vi.fn(),
  ListSignatures: vi.fn(),
  SaveSignature: vi.fn(),
  DeleteSignature: vi.fn(),
  SetAccountSignature: vi.fn(),
  PickImage: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { SettingsDialog } from "./SettingsDialog";

const account = { id: 1, email: "yo@ej.com", displayName: "Yo", defaultSignatureId: 0 };
const sigs = [
  { id: 10, name: "Trabajo", html: "<p>Saludos, Yo</p>" },
  { id: 11, name: "Personal", html: "<p>Chau</p>" },
];

// Radix Select y Dialog piden APIs que jsdom no trae.
beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
  // ProseMirror pide rectángulos al mover la selección; jsdom no calcula layout.
  const noRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] }) as unknown as DOMRectList;
  Range.prototype.getClientRects = noRects;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
});

function setup() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SettingsDialog open onOpenChange={() => {}} />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("Firmas en Configuración", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListAccounts).mockResolvedValue([account] as never);
    vi.mocked(svc.ListSignatures).mockResolvedValue(sigs as never);
    vi.mocked(svc.SaveSignature).mockImplementation((s) => Promise.resolve(s) as never);
    vi.mocked(svc.DeleteSignature).mockResolvedValue();
    vi.mocked(svc.SetAccountSignature).mockResolvedValue();
  });

  it("lista las firmas y Nueva firma guarda nombre y HTML", async () => {
    const user = setup();
    await screen.findByText("Trabajo");
    expect(screen.getByText("Personal")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Nueva firma" }));
    await user.type(await screen.findByLabelText("Nombre"), "Nueva");
    const body = await screen.findByLabelText("Cuerpo del mensaje");
    act(() => body.focus());
    await user.keyboard("Hola mundo");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(svc.SaveSignature).toHaveBeenCalled());
    const sent = vi.mocked(svc.SaveSignature).mock.calls[0][0];
    expect(sent.id).toBe(0);
    expect(sent.name).toBe("Nueva");
    expect(sent.html).toContain("Hola mundo");
  });

  it("editar precarga nombre y contenido", async () => {
    const user = setup();
    await screen.findByText("Trabajo");
    await user.click(screen.getByRole("button", { name: "Editar Trabajo" }));
    expect(await screen.findByLabelText("Nombre")).toHaveValue("Trabajo");
    const body = await screen.findByLabelText("Cuerpo del mensaje");
    expect(body).toHaveTextContent("Saludos, Yo");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(svc.SaveSignature).toHaveBeenCalled());
    const sent = vi.mocked(svc.SaveSignature).mock.calls[0][0];
    expect(sent.id).toBe(10);
    expect(sent.html).toContain("Saludos, Yo");
  });

  it("eliminar pide confirmación", async () => {
    const user = setup();
    await screen.findByText("Trabajo");
    await user.click(screen.getByRole("button", { name: "Eliminar Trabajo" }));
    expect(await screen.findByText("¿Eliminar la firma «Trabajo»?")).toBeInTheDocument();
    expect(svc.DeleteSignature).not.toHaveBeenCalled();
    const confirm = screen.getByText("¿Eliminar la firma «Trabajo»?").closest("[role=dialog]") as HTMLElement;
    await user.click(within(confirm).getByRole("button", { name: "Eliminar" }));
    await waitFor(() => expect(svc.DeleteSignature).toHaveBeenCalledWith(10));
  });

  it("el selector por cuenta llama a SetAccountSignature con el id o 0", async () => {
    vi.mocked(svc.ListAccounts).mockResolvedValue([{ ...account, defaultSignatureId: 11 }] as never);
    const user = setup();
    await screen.findByText("Trabajo");
    const combo = await screen.findByRole("combobox", { name: /Firma por defecto/ });
    await user.click(combo);
    await user.click(await screen.findByRole("option", { name: "Sin firma" }));
    await waitFor(() => expect(svc.SetAccountSignature).toHaveBeenCalledWith(1, 0));
    await user.click(combo);
    await user.click(await screen.findByRole("option", { name: "Trabajo" }));
    await waitFor(() => expect(svc.SetAccountSignature).toHaveBeenCalledWith(1, 10));
  });

  it("nombre vacío muestra el error del núcleo", async () => {
    vi.mocked(svc.SaveSignature).mockRejectedValue(new Error("Poné un nombre para la firma."));
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Nueva firma" }));
    await user.click(await screen.findByRole("button", { name: "Guardar" }));
    expect(await screen.findByText("Poné un nombre para la firma.")).toBeInTheDocument();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLabels: vi.fn(),
  CreateLabel: vi.fn(),
  UpdateLabel: vi.fn(),
  ReorderLabels: vi.fn(),
  LabelUsage: vi.fn(),
  DeleteLabel: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { LabelsSection } from "./LabelsSection";

const labels = [
  { id: 1, name: "Trabajo", color: "#dc2626", position: 0 },
  { id: 2, name: "Casa", color: "#ea580c", position: 1 },
];

beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn();
});

function setup() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LabelsSection />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("Etiquetas en Configuración", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.ListLabels).mockResolvedValue(labels as never);
    vi.mocked(svc.CreateLabel).mockImplementation((name, color) => Promise.resolve({ id: 9, name, color, position: 2 }) as never);
    vi.mocked(svc.UpdateLabel).mockImplementation((l) => Promise.resolve(l) as never);
    vi.mocked(svc.ReorderLabels).mockResolvedValue();
    vi.mocked(svc.DeleteLabel).mockResolvedValue();
    vi.mocked(svc.LabelUsage).mockResolvedValue(3);
  });

  it("sin etiquetas muestra el vacío", async () => {
    vi.mocked(svc.ListLabels).mockResolvedValue(null as never);
    setup();
    expect(await screen.findByText("Todavía no tenés etiquetas.")).toBeInTheDocument();
  });

  it("lista las etiquetas con el número de tecla de las primeras nueve", async () => {
    vi.mocked(svc.ListLabels).mockResolvedValue(
      Array.from({ length: 10 }, (_, i) => ({ id: i + 1, name: `E${i + 1}`, color: "#dc2626", position: i })) as never,
    );
    setup();
    await screen.findByDisplayValue("E1");
    const rows = screen.getAllByRole("listitem");
    expect(within(rows[0]).getByText("1")).toBeInTheDocument();
    expect(within(rows[8]).getByText("9")).toBeInTheDocument();
    expect(within(rows[9]).queryByText("10")).not.toBeInTheDocument();
  });

  it("Nueva etiqueta crea con el nombre tipeado y el primer color libre", async () => {
    const user = setup();
    await screen.findByDisplayValue("Trabajo");
    await user.click(screen.getByRole("button", { name: "Nueva etiqueta" }));
    await user.type(screen.getByLabelText("Nombre de la etiqueta nueva"), "Banco{Enter}");
    await waitFor(() => expect(svc.CreateLabel).toHaveBeenCalledWith("Banco", "#d97706"));
  });

  it("muestra el error del núcleo al crear", async () => {
    vi.mocked(svc.CreateLabel).mockRejectedValue(new Error("Ya hay una etiqueta con ese nombre."));
    const user = setup();
    await screen.findByDisplayValue("Trabajo");
    await user.click(screen.getByRole("button", { name: "Nueva etiqueta" }));
    await user.type(screen.getByLabelText("Nombre de la etiqueta nueva"), "Casa{Enter}");
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Ya hay una etiqueta con ese nombre."));
  });

  it("renombrar con Enter llama a UpdateLabel", async () => {
    const user = setup();
    const input = await screen.findByDisplayValue("Trabajo");
    await user.clear(input);
    await user.type(input, "Oficina{Enter}");
    await waitFor(() => expect(svc.UpdateLabel).toHaveBeenCalledWith({ ...labels[0], name: "Oficina" }));
  });

  it("renombrar al salir del campo llama a UpdateLabel; sin cambios no", async () => {
    const user = setup();
    const input = await screen.findByDisplayValue("Trabajo");
    await user.click(input);
    await user.tab();
    expect(svc.UpdateLabel).not.toHaveBeenCalled();
    await user.click(input);
    await user.type(input, "s");
    await user.tab();
    await waitFor(() => expect(svc.UpdateLabel).toHaveBeenCalledWith({ ...labels[0], name: "Trabajos" }));
  });

  it("Escape restaura el nombre sin guardar", async () => {
    const user = setup();
    const input = await screen.findByDisplayValue("Trabajo");
    await user.type(input, "xyz{Escape}");
    expect(input).toHaveValue("Trabajo");
    await user.tab();
    expect(svc.UpdateLabel).not.toHaveBeenCalled();
  });

  it("Escape en la etiqueta nueva no la crea", async () => {
    const user = setup();
    await screen.findByDisplayValue("Trabajo");
    await user.click(screen.getByRole("button", { name: "Nueva etiqueta" }));
    await user.type(screen.getByLabelText("Nombre de la etiqueta nueva"), "Banco{Escape}");
    expect(screen.queryByLabelText("Nombre de la etiqueta nueva")).toBeNull();
    await user.tab();
    expect(svc.CreateLabel).not.toHaveBeenCalled();
  });

  it("elegir un color de la paleta llama a UpdateLabel", async () => {
    const user = setup();
    await screen.findByDisplayValue("Trabajo");
    await user.click(screen.getByRole("button", { name: "Color de Trabajo" }));
    await user.click(await screen.findByRole("button", { name: "#2563eb" }));
    await waitFor(() => expect(svc.UpdateLabel).toHaveBeenCalledWith({ ...labels[0], color: "#2563eb" }));
  });

  it("la paleta ofrece los 12 colores", async () => {
    const user = setup();
    await screen.findByDisplayValue("Trabajo");
    await user.click(screen.getByRole("button", { name: "Color de Trabajo" }));
    await screen.findByRole("button", { name: "#2563eb" });
    expect(screen.getAllByRole("button", { name: /^#[0-9a-f]{6}$/ })).toHaveLength(12);
  });

  it("las flechas reordenan; subir está deshabilitado en la primera y bajar en la última", async () => {
    const user = setup();
    await screen.findByDisplayValue("Trabajo");
    expect(screen.getByRole("button", { name: "Subir Trabajo" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Bajar Casa" })).toBeDisabled();
    await user.click(screen.getByRole("button", { name: "Bajar Trabajo" }));
    await waitFor(() => expect(svc.ReorderLabels).toHaveBeenCalledWith([2, 1]));
    await user.click(screen.getByRole("button", { name: "Subir Casa" }));
    await waitFor(() => expect(svc.ReorderLabels).toHaveBeenLastCalledWith([2, 1]));
  });

  it.each([
    [3, "Se saca de 3 correos. Los correos no se borran."],
    [1, "Se saca de 1 correo. Los correos no se borran."],
    [0, "Ningún correo la tiene todavía. Los correos no se borran."],
  ])("Borrar con uso %i pide confirmación", async (n, text) => {
    vi.mocked(svc.LabelUsage).mockResolvedValue(n);
    const user = setup();
    await screen.findByDisplayValue("Trabajo");
    await user.click(screen.getByRole("button", { name: "Borrar Trabajo" }));
    expect(await screen.findByText("¿Borrar la etiqueta «Trabajo»?")).toBeInTheDocument();
    expect(await screen.findByText(text)).toBeInTheDocument();
    expect(svc.DeleteLabel).not.toHaveBeenCalled();
    const dlg = screen.getByText("¿Borrar la etiqueta «Trabajo»?").closest("[role=dialog]") as HTMLElement;
    await user.click(within(dlg).getByRole("button", { name: "Borrar" }));
    await waitFor(() => expect(svc.DeleteLabel).toHaveBeenCalledWith(1));
  });
});

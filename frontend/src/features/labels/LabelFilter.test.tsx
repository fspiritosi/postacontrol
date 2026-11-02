import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLabels: vi.fn(),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { onSettingsRequest } from "@/lib/events";
import { LabelFilter } from "./LabelFilter";

const labels = [
  { id: 1, name: "Facturas proveedores", color: "#dc2626", position: 0 },
  { id: 2, name: "Viaje", color: "#2563eb", position: 1 },
];

function wrap(value: number | null, onChange = vi.fn()) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LabelFilter value={value} onChange={onChange} />
    </QueryClientProvider>,
  );
  return onChange;
}

const open = async () =>
  fireEvent.pointerDown(
    await screen.findByRole("button", { name: "Filtrar por etiqueta" }),
    { button: 0, ctrlKey: false },
  );

describe("LabelFilter", () => {
  it("elegir una etiqueta del menú llama a onChange con su id", async () => {
    vi.mocked(svc.ListLabels).mockResolvedValue(labels);
    const onChange = wrap(null);
    expect(await screen.findByText("Etiqueta")).toBeTruthy();
    await open();
    fireEvent.click(await screen.findByRole("menuitem", { name: /Facturas proveedores/ }));
    expect(onChange).toHaveBeenCalledWith(1);
  });

  it("con una elegida muestra el nombre, la marca en el menú y la ✕ la quita", async () => {
    vi.mocked(svc.ListLabels).mockResolvedValue(labels);
    const onChange = wrap(1);
    expect(await screen.findByText("Facturas proveedores")).toBeTruthy();
    expect(screen.queryByText("Etiqueta")).toBeNull();
    await open();
    expect(await screen.findByTestId("label-all")).toBeTruthy();
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    fireEvent.click(screen.getByRole("button", { name: "Quitar filtro de etiqueta" }));
    expect(onChange).toHaveBeenCalledWith(null);
  });

  it("sin etiquetas el menú lo dice y ofrece administrarlas", async () => {
    vi.mocked(svc.ListLabels).mockResolvedValue([]);
    wrap(null);
    const seen = vi.fn();
    const off = onSettingsRequest(seen);
    await open();
    expect(await screen.findByText("Todavía no tenés etiquetas.")).toBeTruthy();
    fireEvent.click(screen.getByRole("menuitem", { name: "Administrar etiquetas…" }));
    expect(seen).toHaveBeenCalledWith("labels");
    off();
  });
});

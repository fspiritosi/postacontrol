import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLabels: vi.fn(),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Label } from "../../../bindings/postacontrol/core/model/models";
import { LabelChips, LabelLine } from "./LabelStripe";

const lab = (id: number, name: string, color: string): Label => ({
  id, name, color, position: id,
});
const all = [
  lab(1, "Facturas", "#dc2626"),
  lab(2, "Viaje", "#2563eb"),
  lab(3, "Casa", "#16a34a"),
  lab(4, "Trabajo", "#7c3aed"),
];

function wrap(ui: React.ReactNode) {
  vi.mocked(svc.ListLabels).mockResolvedValue(all);
  return render(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>);
}

describe("LabelLine", () => {
  it("dos ids dibujan dos tramos con sus colores", async () => {
    wrap(<LabelLine ids={[1, 2]} />);
    const segs = await screen.findAllByTestId("label-segment");
    expect(segs).toHaveLength(2);
    expect(segs[0].style.backgroundColor).toBe("rgb(220, 38, 38)");
    expect(segs[1].style.backgroundColor).toBe("rgb(37, 99, 235)");
    expect(screen.getByTestId("label-line").className).toContain("pointer-events-none");
  });

  it("sin ids o con ids desconocidos no dibuja nada", async () => {
    wrap(<LabelLine ids={[99]} />);
    await vi.waitFor(() => expect(svc.ListLabels).toHaveBeenCalled());
    expect(screen.queryByTestId("label-line")).toBeNull();
  });
});

describe("LabelChips", () => {
  it("con 4 ids y max 2 muestra 2 nombres y +2", async () => {
    wrap(<LabelChips ids={[1, 2, 3, 4]} max={2} />);
    expect(await screen.findByText("Facturas")).toBeTruthy();
    expect(screen.getByText("Viaje")).toBeTruthy();
    expect(screen.queryByText("Casa")).toBeNull();
    expect(screen.getByText("+2")).toBeTruthy();
  });

  it("los ids desconocidos no se dibujan ni cuentan", async () => {
    wrap(<LabelChips ids={[1, 99]} max={2} />);
    expect(await screen.findByText("Facturas")).toBeTruthy();
    expect(screen.queryByText(/^\+/)).toBeNull();
  });
});

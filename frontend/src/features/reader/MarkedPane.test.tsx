import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render as rtlRender, screen } from "@testing-library/react";
import type { ReactElement } from "react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  BackgroundImage: vi.fn(() => Promise.resolve("")),
}));

function render(ui: ReactElement) {
  return rtlRender(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>);
}
import { MarkedPane } from "./MarkedPane";

describe("MarkedPane", () => {
  it("dice cuántos correos hay seleccionados", () => {
    render(<MarkedPane count={2} />);
    expect(screen.getByText("2 correos seleccionados")).toBeTruthy();
    expect(screen.getByText("Arrastralos a una carpeta o usá la barra de arriba.")).toBeTruthy();
  });

  it("o cuántas conversaciones", () => {
    render(<MarkedPane count={3} threads />);
    expect(screen.getByText("3 conversaciones seleccionadas")).toBeTruthy();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  BackgroundImage: vi.fn(() => Promise.resolve("")),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { ReaderBackdrop } from "./ReaderBackdrop";

function renderBackdrop() {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <ReaderBackdrop>
        <p>Hola</p>
      </ReaderBackdrop>
    </QueryClientProvider>,
  );
}

describe("ReaderBackdrop", () => {
  beforeEach(() => vi.clearAllMocks());

  it("sin fondo muestra el contenido como siempre", async () => {
    renderBackdrop();
    await waitFor(() => expect(svc.BackgroundImage).toHaveBeenCalled());
    expect(screen.queryByTestId("reader-backdrop")).toBeNull();
    expect(screen.getByText("Hola")).toBeTruthy();
  });

  it("con fondo pinta la imagen detrás y el texto sobre un recuadro", async () => {
    vi.mocked(svc.BackgroundImage).mockResolvedValue("data:image/png;base64,AAAA");
    renderBackdrop();
    const el = await screen.findByTestId("reader-backdrop");
    expect(el.style.backgroundImage).toContain("data:image/png;base64,AAAA");
    expect(el.style.backgroundSize).toBe("cover");
    expect(el.style.backgroundPosition).toContain("center");
    expect(screen.getByText("Hola").closest(".backdrop-blur-sm")).toBeTruthy();
  });
});

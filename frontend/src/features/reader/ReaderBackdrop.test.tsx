import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(),
  BackgroundImage: vi.fn(() => Promise.resolve("")),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { ReaderBackdrop } from "./ReaderBackdrop";

// Configuración guardada con el fondo `background` (nombre del archivo).
function stored(background: string) {
  vi.mocked(svc.GetSettings).mockResolvedValue({
    unifiedInbox: false,
    disableNotifications: false, disableSpellcheck: false,
    threadedList: false,
    readingPane: "",
    background,
  });
}

function renderBackdrop(client = new QueryClient()) {
  render(
    <QueryClientProvider client={client}>
      <ReaderBackdrop>
        <p>Hola</p>
      </ReaderBackdrop>
    </QueryClientProvider>,
  );
}

describe("ReaderBackdrop", () => {
  beforeEach(() => vi.clearAllMocks());

  it("sin fondo muestra el contenido como siempre", async () => {
    stored("");
    renderBackdrop();
    await waitFor(() => expect(svc.GetSettings).toHaveBeenCalled());
    // Sin fondo guardado no hay nada que leer.
    expect(svc.BackgroundImage).not.toHaveBeenCalled();
    expect(screen.queryByTestId("reader-backdrop")).toBeNull();
    expect(screen.getByText("Hola")).toBeTruthy();
  });

  it("con fondo pinta la imagen detrás y el texto sobre un recuadro", async () => {
    stored("background-1.png");
    vi.mocked(svc.BackgroundImage).mockResolvedValue("data:image/png;base64,AAAA");
    renderBackdrop();
    const el = await screen.findByTestId("reader-backdrop");
    expect(el.style.backgroundImage).toContain("data:image/png;base64,AAAA");
    expect(el.style.backgroundSize).toBe("cover");
    expect(el.style.backgroundPosition).toContain("center");
    expect(screen.getByText("Hola").closest(".backdrop-blur-sm")).toBeTruthy();
  });

  it("relee la imagen solo cuando cambia el archivo del fondo", async () => {
    stored("background-1.png");
    vi.mocked(svc.BackgroundImage).mockResolvedValue("data:image/png;base64,AAAA");
    const client = new QueryClient();
    renderBackdrop(client);
    await screen.findByTestId("reader-backdrop");
    expect(svc.BackgroundImage).toHaveBeenCalledTimes(1);

    // Cambió otra opción: la configuración se recarga, el fondo no.
    await act(() => client.invalidateQueries({ queryKey: ["settings"] }));
    expect(svc.GetSettings).toHaveBeenCalledTimes(2);
    expect(svc.BackgroundImage).toHaveBeenCalledTimes(1);

    // Fondo nuevo: otro nombre, se lee.
    stored("background-2.png");
    vi.mocked(svc.BackgroundImage).mockResolvedValue("data:image/png;base64,BBBB");
    await act(() => client.invalidateQueries({ queryKey: ["settings"] }));
    await waitFor(() => expect(screen.getByTestId("reader-backdrop").style.backgroundImage).toContain("BBBB"));
    expect(svc.BackgroundImage).toHaveBeenCalledTimes(2);
  });
});

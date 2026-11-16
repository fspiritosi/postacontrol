import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { ThemeProvider, useResolvedTheme } from "./ThemeProvider";

// matchMedia falso: `dark` dice si el sistema está en oscuro y `fire` avisa el cambio.
function fakeMatchMedia(initialDark: boolean) {
  const listeners = new Set<() => void>();
  const mq = {
    matches: initialDark,
    addEventListener: (_: string, cb: () => void) => listeners.add(cb),
    removeEventListener: (_: string, cb: () => void) => listeners.delete(cb),
  };
  window.matchMedia = vi.fn(() => mq) as unknown as typeof window.matchMedia;
  return {
    set(dark: boolean) {
      mq.matches = dark;
      listeners.forEach((cb) => cb());
    },
  };
}

function Probe() {
  return <span data-testid="resolved">{useResolvedTheme()}</span>;
}

function renderProvider() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <ThemeProvider>
        <Probe />
      </ThemeProvider>
    </QueryClientProvider>,
  );
}

const isDark = () => document.documentElement.classList.contains("dark");

describe("ThemeProvider", () => {
  beforeEach(() => {
    localStorage.clear();
    document.documentElement.classList.remove("dark");
  });
  afterEach(() => vi.clearAllMocks());

  it("con theme dark pone la clase dark aunque el sistema sea claro", async () => {
    fakeMatchMedia(false);
    vi.mocked(svc.GetSettings).mockResolvedValue({ theme: "dark" } as never);
    renderProvider();
    await waitFor(() => expect(isDark()).toBe(true));
    expect(screen.getByTestId("resolved")).toHaveTextContent("dark");
  });

  it("con theme system sigue al sistema y cambia al disparar change", async () => {
    const media = fakeMatchMedia(false);
    vi.mocked(svc.GetSettings).mockResolvedValue({ theme: "system" } as never);
    renderProvider();
    await waitFor(() => expect(screen.getByTestId("resolved")).toHaveTextContent("light"));
    expect(isDark()).toBe(false);
    act(() => media.set(true));
    expect(isDark()).toBe(true);
    expect(screen.getByTestId("resolved")).toHaveTextContent("dark");
  });

  it("con theme light ignora que el sistema esté en oscuro", async () => {
    fakeMatchMedia(true);
    vi.mocked(svc.GetSettings).mockResolvedValue({ theme: "light" } as never);
    renderProvider();
    await waitFor(() => expect(screen.getByTestId("resolved")).toHaveTextContent("light"));
    expect(isDark()).toBe(false);
  });

  it("antes de cargar los ajustes usa el tema guardado en pc:theme", () => {
    fakeMatchMedia(false);
    localStorage.setItem("pc:theme", "dark");
    vi.mocked(svc.GetSettings).mockReturnValue(new Promise(() => {}) as never);
    renderProvider();
    expect(isDark()).toBe(true);
  });

  it("guarda en pc:theme la preferencia que llega en los ajustes", async () => {
    fakeMatchMedia(false);
    vi.mocked(svc.GetSettings).mockResolvedValue({ theme: "light" } as never);
    localStorage.setItem("pc:theme", "dark");
    renderProvider();
    await waitFor(() => expect(localStorage.getItem("pc:theme")).toBe("light"));
  });
});

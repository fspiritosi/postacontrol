import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({ OpenExternal: vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    disconnect() {}
  },
);

import { SafeHtmlFrame } from "./SafeHtmlFrame";

describe("SafeHtmlFrame", () => {
  it("usa sandbox sin scripts ni popups", () => {
    render(<SafeHtmlFrame html="<p>hola</p>" text="" allowRemote={false} />);
    const frame = screen.getByTitle("Contenido del correo");
    expect(frame.getAttribute("sandbox")).toBe("allow-same-origin");
  });

  it("sin HTML muestra el texto plano", () => {
    render(<SafeHtmlFrame html="" text="solo texto" allowRemote={false} />);
    expect(screen.getByText("solo texto")).toBeTruthy();
  });
});

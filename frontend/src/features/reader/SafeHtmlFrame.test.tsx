import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  OpenExternal: vi.fn(() => Promise.resolve()),
  OpenMailto: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.stubGlobal(
  "ResizeObserver",
  class {
    observe() {}
    disconnect() {}
  },
);

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { SafeHtmlFrame, handleFrameClick } from "./SafeHtmlFrame";

describe("SafeHtmlFrame", () => {
  it("usa sandbox sin scripts ni popups", () => {
    render(<SafeHtmlFrame html="<p>hola</p>" text="" allowRemote={false} accountId={1} />);
    const frame = screen.getByTitle("Contenido del correo");
    expect(frame.getAttribute("sandbox")).toBe("allow-same-origin");
  });

  it("sin HTML muestra el texto plano", () => {
    render(<SafeHtmlFrame html="" text="solo texto" allowRemote={false} accountId={1} />);
    expect(screen.getByText("solo texto")).toBeTruthy();
  });
});

describe("handleFrameClick", () => {
  it("abre afuera los enlaces HTML y cancela la navegación", () => {
    document.body.innerHTML = '<a href="https://x.y/"><span id="in">x</span></a>';
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    const spy = vi.spyOn(ev, "preventDefault");
    document.body.addEventListener("click", (e) => handleFrameClick(e, 7), { once: true });
    document.getElementById("in")!.dispatchEvent(ev);
    expect(spy).toHaveBeenCalled();
    expect(svc.OpenExternal).toHaveBeenCalledWith("https://x.y/");
  });

  it("ignora los <a> de SVG", () => {
    vi.mocked(svc.OpenExternal).mockClear();
    document.body.innerHTML =
      '<svg><a href="https://x.y/"><text id="t">x</text></a></svg>';
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    document.body.addEventListener("click", (e) => handleFrameClick(e, 7), { once: true });
    document.getElementById("t")!.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(false);
    expect(svc.OpenExternal).not.toHaveBeenCalled();
  });
});

describe("handleFrameClick entre realms", () => {
  it("funciona con un <a> creado en el documento de un iframe", () => {
    vi.mocked(svc.OpenExternal).mockClear();
    const frame = document.createElement("iframe");
    document.body.appendChild(frame);
    const idoc = frame.contentDocument!;
    const a = idoc.createElement("a");
    a.setAttribute("href", "https://x.y/");
    a.textContent = "x";
    idoc.body.appendChild(a);
    // Precondición: el constructor del iframe es otro.
    expect(a instanceof window.HTMLAnchorElement).toBe(false);
    const ev = new (frame.contentWindow as unknown as typeof globalThis).MouseEvent("click", { bubbles: true, cancelable: true });
    idoc.addEventListener("click", (e) => handleFrameClick(e, 7), { once: true });
    a.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(svc.OpenExternal).toHaveBeenCalledWith("https://x.y/");
  });

  it("los mailto: abren el redactor con la cuenta y no el navegador", () => {
    vi.mocked(svc.OpenExternal).mockClear();
    document.body.innerHTML = '<a href="mailto:a@x.com"><span id="m">x</span></a>';
    const ev = new MouseEvent("click", { bubbles: true, cancelable: true });
    document.body.addEventListener("click", (e) => handleFrameClick(e, 7), { once: true });
    document.getElementById("m")!.dispatchEvent(ev);
    expect(svc.OpenMailto).toHaveBeenCalledWith("mailto:a@x.com", 7);
    expect(svc.OpenExternal).not.toHaveBeenCalled();
  });
});

import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/pdf", () => ({ loadPdf: vi.fn() }));

import { loadPdf, type PdfDoc } from "@/lib/pdf";
import { PdfView } from "./PdfView";

// Páginas de 500 × 700 a escala 1.
function fakeDoc(numPages: number) {
  const doc = {
    numPages,
    pageSize: vi.fn(async () => ({ width: 500, height: 700 })),
    renderPage: vi.fn(async (_n: number, _c: HTMLCanvasElement, scale: number) => ({
      width: 500 * scale,
      height: 700 * scale,
    })),
    releaseCanvas: vi.fn(),
    destroy: vi.fn(),
  };
  vi.mocked(loadPdf).mockResolvedValue(doc satisfies PdfDoc);
  return doc;
}

const bytes = new Uint8Array([37, 80, 68, 70]);

describe("PdfView", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
  });

  it("dibuja una canvas por página", async () => {
    const doc = fakeDoc(3);
    const { container } = render(<PdfView data={bytes} zoom={1} />);
    await waitFor(() => expect(container.querySelectorAll("canvas")).toHaveLength(3));
    await waitFor(() => expect(doc.renderPage).toHaveBeenCalledTimes(3));
    const pages = doc.renderPage.mock.calls.map((c) => c[0]).sort();
    expect(pages).toEqual([1, 2, 3]);
    expect(loadPdf).toHaveBeenCalledWith(bytes);
  });

  it("zoom null ajusta al ancho; con número usa esa escala", async () => {
    const doc = fakeDoc(1);
    const { unmount } = render(<PdfView data={bytes} zoom={1.5} />);
    await waitFor(() => expect(doc.renderPage).toHaveBeenCalled());
    for (const c of doc.renderPage.mock.calls) expect(c[2]).toBe(1.5);
    unmount();

    // El contenedor mide 1032 de ancho; con 16 de margen a cada lado quedan
    // 1000 para una página de 500: escala 2, sin dibujar antes a escala 1.
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1032);
    const fit = fakeDoc(1);
    const { container } = render(<PdfView data={bytes} zoom={null} />);
    await waitFor(() => expect(fit.renderPage).toHaveBeenCalledWith(1, expect.any(HTMLCanvasElement), 2));
    expect(fit.renderPage).toHaveBeenCalledTimes(1);
    // El recuadro de la página ya tiene su tamaño.
    const box = container.querySelector("canvas")!.parentElement!;
    expect(box.style.width).toBe("1000px");
    expect(box.style.height).toBe("1400px");
    // Una canvas liberada no agranda el recuadro.
    expect(box).toHaveClass("overflow-hidden");
  });

  // Geometría falsa: páginas de 500 × 700 · zoom apiladas desde 16, con 16 de
  // separación. El scroller mide 400 de ancho y su contenido 500 · zoom + 32.
  function stubLayout() {
    const boxHeight = (el: HTMLElement) => parseFloat((el.firstChild as HTMLElement | null)?.style.height ?? "0");
    vi.spyOn(HTMLElement.prototype, "offsetHeight", "get").mockImplementation(function (this: HTMLElement) {
      return this.dataset.page ? boxHeight(this) : 0;
    });
    vi.spyOn(HTMLElement.prototype, "offsetTop", "get").mockImplementation(function (this: HTMLElement) {
      const n = Number(this.dataset.page ?? 0);
      return n ? 16 + (n - 1) * (boxHeight(this) + 16) : 0;
    });
  }

  it("al cambiar el zoom se queda en la misma página y en el mismo punto", async () => {
    fakeDoc(30);
    stubLayout();
    let zoom = 1;
    const { container, rerender } = render(<PdfView data={bytes} zoom={zoom} />);
    await waitFor(() => expect(container.querySelectorAll("canvas")).toHaveLength(30));
    const scroller = container.firstChild as HTMLDivElement;
    Object.defineProperty(scroller, "clientWidth", { get: () => 400 });
    Object.defineProperty(scroller, "scrollWidth", { get: () => 500 * zoom + 32 });

    // Mitad de la página 16 a 100% (alto 700 + 16 de separación), con el
    // centro de la vista en el centro del contenido (532 / 2 = 266).
    scroller.scrollTop = 16 + 15 * 716 + 350;
    scroller.scrollLeft = 66;
    fireEvent.scroll(scroller);

    zoom = 2;
    rerender(<PdfView data={bytes} zoom={zoom} />);
    // A 200% la página mide 1400: mitad de la 16 = 16 + 15 · 1416 + 700.
    expect(scroller.scrollTop).toBe(16 + 15 * 1416 + 700);
    // Contenido de 1032: el centro (516) sigue en el centro de la vista.
    expect(scroller.scrollLeft).toBe(516 - 200);

    // Y de vuelta, sin moverse en el medio.
    zoom = 0.5;
    rerender(<PdfView data={bytes} zoom={zoom} />);
    expect(scroller.scrollTop).toBe(16 + 15 * 366 + 175);
  });

  it("sin haberse movido, cambiar el zoom deja la vista arriba", async () => {
    fakeDoc(3);
    stubLayout();
    const { container, rerender } = render(<PdfView data={bytes} zoom={1} />);
    await waitFor(() => expect(container.querySelectorAll("canvas")).toHaveLength(3));
    const scroller = container.firstChild as HTMLDivElement;
    rerender(<PdfView data={bytes} zoom={2} />);
    expect(scroller.scrollTop).toBe(0);
  });

  it("PDF que no abre muestra el error", async () => {
    vi.mocked(loadPdf).mockRejectedValue(new Error("Invalid PDF structure"));
    render(<PdfView data={bytes} zoom={null} />);
    expect(
      await screen.findByText("No pudimos abrir el PDF. Podés descargarlo y abrirlo con otro programa."),
    ).toBeInTheDocument();
  });

  it("avisa la página visible", async () => {
    fakeDoc(3);
    const onPageChange = vi.fn();
    render(<PdfView data={bytes} zoom={null} onPageChange={onPageChange} />);
    await waitFor(() => expect(onPageChange).toHaveBeenCalledWith(1, 3));
  });

  it("destruye el documento al desmontar", async () => {
    const doc = fakeDoc(2);
    const { container, unmount } = render(<PdfView data={bytes} zoom={null} />);
    await waitFor(() => expect(container.querySelectorAll("canvas")).toHaveLength(2));
    expect(doc.destroy).not.toHaveBeenCalled();
    unmount();
    expect(doc.destroy).toHaveBeenCalledTimes(1);
  });

  it("si se desmonta antes de abrir, destruye el documento cuando llega", async () => {
    let resolve!: (d: PdfDoc) => void;
    vi.mocked(loadPdf).mockReturnValue(new Promise<PdfDoc>((r) => (resolve = r)));
    const { unmount } = render(<PdfView data={bytes} zoom={null} />);
    unmount();
    const destroy = vi.fn();
    resolve({ numPages: 1, pageSize: vi.fn(), renderPage: vi.fn(), releaseCanvas: vi.fn(), destroy });
    await waitFor(() => expect(destroy).toHaveBeenCalledTimes(1));
  });

  it("una página que falla muestra su error; una cancelada no", async () => {
    const doc = fakeDoc(3);
    doc.renderPage.mockImplementation(async (n: number) => {
      if (n === 2) throw new Error("broken page");
      if (n === 3) throw Object.assign(new Error("cancelada"), { name: "RenderCancelled" });
      return { width: 500, height: 700 };
    });
    const { container } = render(<PdfView data={bytes} zoom={1} />);
    expect(await screen.findByText("No pudimos mostrar esta página.")).toBeInTheDocument();
    expect(screen.getAllByText("No pudimos mostrar esta página.")).toHaveLength(1);
    const box2 = container.querySelector('[data-page="2"]')!;
    expect(box2).toHaveTextContent("No pudimos mostrar esta página.");
  });

  it("con IntersectionObserver dibuja solo las páginas cercanas y libera las que se alejan", async () => {
    type Fake = { cb: IntersectionObserverCallback; options?: IntersectionObserverInit; targets: Element[] };
    const observers: Fake[] = [];
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        f: Fake;
        constructor(cb: IntersectionObserverCallback, options?: IntersectionObserverInit) {
          this.f = { cb, options, targets: [] };
          observers.push(this.f);
        }
        observe(el: Element) {
          this.f.targets.push(el);
        }
        disconnect() {}
      },
    );
    const doc = fakeDoc(5);
    const { container } = render(<PdfView data={bytes} zoom={1} />);
    await waitFor(() => expect(container.querySelectorAll("canvas")).toHaveLength(5));
    // Nada se dibuja hasta que el observer avisa.
    expect(doc.renderPage).not.toHaveBeenCalled();

    const nearby = observers.find((o) => o.options?.rootMargin)!;
    const page = (n: number) => container.querySelector(`[data-page="${n}"]`)!;
    const entry = (n: number, isIntersecting: boolean) =>
      ({ target: page(n), isIntersecting, intersectionRatio: isIntersecting ? 1 : 0 }) as IntersectionObserverEntry;
    act(() => nearby.cb([entry(1, true), entry(2, true), entry(3, false)], {} as IntersectionObserver));
    await waitFor(() => expect(doc.renderPage).toHaveBeenCalledTimes(2));
    expect(doc.renderPage.mock.calls.map((c) => c[0]).sort()).toEqual([1, 2]);
    // Todas las páginas tienen su recuadro, dibujadas o no.
    for (let n = 1; n <= 5; n++) expect((page(n).firstChild as HTMLElement).style.height).toBe("700px");

    // La 1 se aleja: su canvas se libera y no se vuelve a dibujar.
    const canvas1 = page(1).querySelector("canvas");
    act(() => nearby.cb([entry(1, false)], {} as IntersectionObserver));
    await waitFor(() => expect(doc.releaseCanvas).toHaveBeenCalledWith(canvas1));
    expect(doc.renderPage).toHaveBeenCalledTimes(2);
  });
});

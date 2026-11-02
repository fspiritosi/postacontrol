import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("@/lib/pdf", () => ({ loadPdf: vi.fn() }));

import { loadPdf, type PdfDoc } from "@/lib/pdf";
import { PdfView } from "./PdfView";

// Páginas de 500 × 700 a escala 1.
function fakeDoc(numPages: number) {
  const doc = {
    numPages,
    renderPage: vi.fn(async (_n: number, _c: HTMLCanvasElement, scale: number) => ({
      width: 500 * scale,
      height: 700 * scale,
    })),
    destroy: vi.fn(),
  };
  vi.mocked(loadPdf).mockResolvedValue(doc satisfies PdfDoc);
  return doc;
}

const bytes = new Uint8Array([37, 80, 68, 70]);

describe("PdfView", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

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
    // 1000 para una página de 500: escala 2.
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(1032);
    const fit = fakeDoc(1);
    render(<PdfView data={bytes} zoom={null} />);
    await waitFor(() => expect(fit.renderPage).toHaveBeenLastCalledWith(1, expect.any(HTMLCanvasElement), 2));
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
    resolve({ numPages: 1, renderPage: vi.fn(), destroy });
    await waitFor(() => expect(destroy).toHaveBeenCalledTimes(1));
  });
});

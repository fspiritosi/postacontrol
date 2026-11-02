import { describe, expect, it, vi } from "vitest";

// pdf.js no corre en jsdom: un documento falso de páginas de 100 × 200.
const page = {
  getViewport: ({ scale }: { scale: number }) => ({ width: 100 * scale, height: 200 * scale }),
  render: vi.fn(() => ({ promise: Promise.resolve(), cancel: vi.fn() })),
};
vi.mock("pdfjs-dist", () => ({
  GlobalWorkerOptions: {},
  getDocument: vi.fn(() => ({
    promise: Promise.resolve({ numPages: 1, getPage: async () => page }),
    destroy: vi.fn(async () => {}),
  })),
}));
vi.mock("pdfjs-dist/build/pdf.worker.min.mjs?worker", () => ({ default: class {} }));
vi.mock("pdfjs-dist/wasm/jbig2.wasm?url", () => ({ default: "jbig2.wasm" }));
vi.mock("pdfjs-dist/wasm/openjpeg.wasm?url", () => ({ default: "openjpeg.wasm" }));

import { loadPdf } from "./pdf";

describe("loadPdf", () => {
  it("releaseCanvas deja la canvas en 0×0 también en CSS", async () => {
    const doc = await loadPdf(new Uint8Array([37, 80, 68, 70]));
    const canvas = document.createElement("canvas");
    await doc.renderPage(1, canvas, 2);
    expect(canvas.style.width).toBe("200px");
    expect(canvas.style.height).toBe("400px");

    doc.releaseCanvas(canvas);
    expect(canvas.width).toBe(0);
    expect(canvas.height).toBe(0);
    // Con el tamaño CSS viejo la canvas vacía seguiría ocupando lugar.
    expect(canvas.style.width).toBe("");
    expect(canvas.style.height).toBe("");
  });
});

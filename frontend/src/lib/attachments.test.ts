import { describe, expect, it } from "vitest";
import { base64ToBytes, viewKind } from "./attachments";

const k = (mimeType: string, filename = "x") => viewKind({ mimeType, filename });

describe("viewKind", () => {
  it("PDF", () => {
    expect(k("application/pdf")).toBe("pdf");
  });

  it("imágenes png, jpeg, gif y webp", () => {
    for (const m of ["image/png", "image/jpeg", "image/gif", "image/webp"]) {
      expect(k(m)).toBe("image");
    }
  });

  it("ignora mayúsculas y parámetros del MIME", () => {
    expect(k("Image/PNG; name=a.png")).toBe("image");
  });

  it("no previsualiza Excel ni SVG", () => {
    expect(k("application/vnd.ms-excel", "a.xls")).toBeNull();
    // SVG puede traer scripts.
    expect(k("image/svg+xml", "a.svg")).toBeNull();
  });

  it("con application/octet-stream decide por la extensión", () => {
    expect(k("application/octet-stream", "factura.PDF")).toBe("pdf");
    expect(k("application/octet-stream", "foto.jpeg")).toBe("image");
    expect(k("application/octet-stream", "x.zip")).toBeNull();
    expect(k("application/octet-stream", "sin-extension")).toBeNull();
    expect(k("application/octet-stream", "dibujo.svg")).toBeNull();
  });

  it("un MIME específico manda sobre la extensión", () => {
    expect(k("application/zip", "trampa.pdf")).toBeNull();
  });
});

describe("base64ToBytes", () => {
  it("decodifica", () => {
    expect(Array.from(base64ToBytes("AAEC"))).toEqual([0, 1, 2]);
  });

  it("vacío da vacío", () => {
    expect(base64ToBytes("").length).toBe(0);
  });
});

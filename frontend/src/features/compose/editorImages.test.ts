import { describe, expect, it } from "vitest";
import { imageError } from "./editorImages";

describe("imageError", () => {
  it("acepta un PNG de 1 KB", () => {
    expect(imageError({ type: "image/png", size: 1024 })).toBeNull();
  });

  it("rechaza tipos que no son imagen soportada", () => {
    expect(imageError({ type: "application/pdf", size: 1024 })).toBe(
      "Solo se pueden insertar imágenes PNG, JPG, GIF o WebP.",
    );
  });

  it("rechaza una imagen de 6 MB", () => {
    expect(imageError({ type: "image/jpeg", size: 6 * 1024 * 1024 })).toBe(
      "La imagen pesa 6.0 MB; el máximo es 5.0 MB.",
    );
  });

  it("con tamaños que se muestran iguales no repite el número", () => {
    expect(imageError({ type: "image/png", size: 5 * 1024 * 1024 + 1 })).toBe("La imagen supera el máximo de 5.0 MB.");
  });
});

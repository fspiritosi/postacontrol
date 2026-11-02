import { describe, expect, it } from "vitest";
import type { Label } from "../../bindings/postacontrol/core/model/models";
import { LABEL_COLORS, labelChipStyle, nextColor, orderedLabels } from "./labels";

describe("nextColor", () => {
  it("toma el primer color sin usar", () => {
    expect(nextColor([])).toBe("#dc2626");
    expect(nextColor(["#dc2626"])).toBe("#ea580c");
    expect(nextColor(["#ea580c"])).toBe("#dc2626");
  });
  it("con todos usados vuelve al primero", () => {
    expect(nextColor([...LABEL_COLORS])).toBe("#dc2626");
  });
});

describe("orderedLabels", () => {
  it("deja las que existen, en el orden de la lista de etiquetas", () => {
    const labels = [{ id: 1 }, { id: 2 }, { id: 3 }] as Label[];
    expect(orderedLabels([3, 1, 99], labels).map((l) => l.id)).toEqual([1, 3]);
  });
});

describe("labelChipStyle", () => {
  it("mezcla el color con el texto y el fondo del tema", () => {
    expect(labelChipStyle("#2563eb")).toEqual({
      color: "color-mix(in oklab, #2563eb 78%, var(--foreground))",
      backgroundColor: "color-mix(in oklab, #2563eb 12%, transparent)",
      borderColor: "color-mix(in oklab, #2563eb 40%, transparent)",
    });
  });
});

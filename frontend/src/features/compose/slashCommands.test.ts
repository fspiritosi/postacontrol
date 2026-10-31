import { describe, expect, it } from "vitest";
import { filterSlashItems, SLASH_ITEMS } from "./slashCommands";

describe("slashCommands", () => {
  it("mantiene el orden y los títulos acordados", () => {
    expect(SLASH_ITEMS.map((i) => i.title)).toEqual([
      "Título 1",
      "Título 2",
      "Título 3",
      "Lista con viñetas",
      "Lista numerada",
      "Cita",
      "Tabla",
      "Imagen",
      "Separador",
      "Enlace",
    ]);
  });

  it('"titulo" devuelve los 3 títulos', () => {
    expect(filterSlashItems("titulo").map((i) => i.title)).toEqual(["Título 1", "Título 2", "Título 3"]);
  });

  it('"VIÑETAS" devuelve "Lista con viñetas"', () => {
    expect(filterSlashItems("VIÑETAS").map((i) => i.title)).toEqual(["Lista con viñetas"]);
  });

  it('"zzz" devuelve vacío', () => {
    expect(filterSlashItems("zzz")).toEqual([]);
  });

  it("sin consulta devuelve todo", () => {
    expect(filterSlashItems("")).toHaveLength(10);
  });
});

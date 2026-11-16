import { afterEach, describe, expect, it, vi } from "vitest";
import { loadCachedTheme, resolveTheme, saveCachedTheme, themePref } from "./theme";

describe("themePref", () => {
  it("lo vacío, lo desconocido o sin ajustes es system", () => {
    expect(themePref(undefined)).toBe("system");
    expect(themePref({ theme: "" } as never)).toBe("system");
    expect(themePref({ theme: "sepia" } as never)).toBe("system");
  });
  it("devuelve el tema guardado", () => {
    expect(themePref({ theme: "dark" } as never)).toBe("dark");
    expect(themePref({ theme: "light" } as never)).toBe("light");
  });
});

describe("resolveTheme", () => {
  it("system sigue al sistema; lo explícito manda", () => {
    expect(resolveTheme("system", true)).toBe("dark");
    expect(resolveTheme("system", false)).toBe("light");
    expect(resolveTheme("light", true)).toBe("light");
    expect(resolveTheme("dark", false)).toBe("dark");
  });
});

describe("tema guardado en el navegador", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });
  it("guarda y lee la preferencia", () => {
    saveCachedTheme("dark");
    expect(localStorage.getItem("pc:theme")).toBe("dark");
    expect(loadCachedTheme()).toBe("dark");
  });
  it("un valor inválido es system", () => {
    localStorage.setItem("pc:theme", "garbage");
    expect(loadCachedTheme()).toBe("system");
  });
  it("sin almacenamiento es system y guardar no rompe", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("bloqueado");
    });
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("bloqueado");
    });
    expect(loadCachedTheme()).toBe("system");
    expect(() => saveCachedTheme("dark")).not.toThrow();
  });
});

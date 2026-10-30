import { describe, expect, it } from "vitest";
import { errorMessage } from "./errors";

// Forma real: @wailsio/runtime rechaza con Error name="RuntimeError" + cause.
const runtimeError = (msg: string, cause?: unknown) =>
  Object.assign(new Error(msg), { name: "RuntimeError", cause });

describe("errorMessage", () => {
  it("devuelve el mensaje de un RuntimeError de Wails", () => {
    const e = runtimeError("El servidor rechazó el usuario o la contraseña.", { kind: "auth" });
    expect(errorMessage(e)).toBe("El servidor rechazó el usuario o la contraseña.");
  });
  it("acepta strings y objetos con message", () => {
    expect(errorMessage("hola")).toBe("hola");
    expect(errorMessage({ message: "chau", kind: "x" })).toBe("chau");
  });
  it("usa un texto genérico si no hay mensaje", () => {
    expect(errorMessage(undefined)).toBe("Ocurrió un error inesperado.");
    expect(errorMessage({})).toBe("Ocurrió un error inesperado.");
  });
});

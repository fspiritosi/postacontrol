import { afterEach, describe, expect, it, vi } from "vitest";
import type { Folder } from "../../bindings/postacontrol/core/model/models";
import { defaultView, loadView, saveView, viewIsValid } from "./view";

function folder(p: Partial<Folder>): Folder {
  return {
    id: 1, accountId: 1, name: "INBOX", delimiter: "/", role: "inbox",
    unread: 0, total: 0, selectable: true, ...p,
  };
}

describe("viewIsValid", () => {
  const folders = [folder({ id: 3 })];

  it("falla para una carpeta que no existe", () => {
    expect(viewIsValid({ kind: "folder", folderId: 99 }, folders, true)).toBe(false);
    expect(viewIsValid({ kind: "folder", folderId: 3 }, folders, true)).toBe(true);
  });

  it("falla para una unificada con la vista unificada apagada", () => {
    expect(viewIsValid({ kind: "unified", role: "inbox" }, folders, false)).toBe(false);
    expect(viewIsValid({ kind: "unified", role: "inbox" }, folders, true)).toBe(true);
  });

  it("pasa para la bandeja de salida", () => {
    expect(viewIsValid({ kind: "outbox" }, [], false)).toBe(true);
  });
});

describe("defaultView", () => {
  it("elige la Entrada de la cuenta de id menor", () => {
    const folders = [
      folder({ id: 20, accountId: 5, role: "inbox" }),
      folder({ id: 11, accountId: 2, name: "Sent", role: "sent" }),
      folder({ id: 12, accountId: 2, role: "inbox", selectable: false }),
      folder({ id: 13, accountId: 2, name: "Bandeja", role: "inbox" }),
      folder({ id: 30, accountId: 9, role: "inbox" }),
    ];
    expect(defaultView(folders)).toEqual({ kind: "folder", folderId: 13 });
  });

  it("devuelve null sin carpetas", () => {
    expect(defaultView([])).toBeNull();
  });
});

describe("loadView / saveView", () => {
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("guarda y vuelve a leer la vista", () => {
    saveView({ kind: "unified", role: "flagged" });
    expect(loadView()).toEqual({ kind: "unified", role: "flagged" });
  });

  it("no guarda la Bandeja de salida: al reabrir se vuelve a la vista anterior", () => {
    saveView({ kind: "folder", folderId: 3 });
    saveView({ kind: "outbox" });
    expect(loadView()).toEqual({ kind: "folder", folderId: 3 });
  });

  it("no restaura una Bandeja de salida guardada por una versión anterior", () => {
    localStorage.setItem("pc:view", JSON.stringify({ kind: "outbox" }));
    expect(loadView()).toBeNull();
  });

  it("devuelve null con JSON inválido", () => {
    localStorage.setItem("pc:view", "{no es json");
    expect(loadView()).toBeNull();
  });

  it("devuelve null con una forma desconocida", () => {
    localStorage.setItem("pc:view", JSON.stringify({ kind: "unified", role: "spam" }));
    expect(loadView()).toBeNull();
  });

  it("devuelve null si localStorage tira error", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("sin almacenamiento");
    });
    expect(loadView()).toBeNull();
  });

  it("saveView no tira si localStorage falla", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("lleno");
    });
    expect(() => saveView({ kind: "unified", role: "inbox" })).not.toThrow();
  });
});

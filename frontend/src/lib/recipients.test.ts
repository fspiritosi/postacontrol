import { describe, expect, it } from "vitest";
import {
  balanceRecipientToken,
  formatRecipients,
  moveRecipient,
  parseRecipients,
  splitRecipientTokens,
  takeRecipientTokens,
} from "./recipients";

describe("parseRecipients", () => {
  it("parsea nombre y email", () => {
    const r = parseRecipients("Fabricio Peña <f@ej.com>; b@x.com");
    expect(r.valid).toHaveLength(2);
    expect(r.valid[0]).toEqual({ name: "Fabricio Peña", email: "f@ej.com" });
    expect(r.valid[1]).toEqual({ name: "", email: "b@x.com" });
    expect(r.invalid).toEqual([]);
  });

  it("separa inválidos", () => {
    const r = parseRecipients("a@b.com, sin-arroba");
    expect(r.valid).toHaveLength(1);
    expect(r.invalid).toEqual(["sin-arroba"]);
  });

  it("separa por saltos de línea y respeta comas entre comillas", () => {
    const r = parseRecipients('"Peña, Fabricio" <f@ej.com>\nb@x.com\n\n');
    expect(r.valid.map((a) => a.email)).toEqual(["f@ej.com", "b@x.com"]);
    expect(r.valid[0].name).toBe("Peña, Fabricio");
  });

  it("formatRecipients es la inversa", () => {
    const list = [
      { name: "Peña, Fabricio", email: "f@ej.com" },
      { name: "", email: "b@x.com" },
    ];
    expect(parseRecipients(formatRecipients(list)).valid).toEqual(list);
  });
});

describe("splitRecipientTokens", () => {
  it("parte fuera de comillas y de <...>", () => {
    expect(splitRecipientTokens('"Peña, Fabricio" <f@ej.com>; b@x.com,\n')).toEqual([
      '"Peña, Fabricio" <f@ej.com>',
      "b@x.com",
    ]);
  });
});

describe("takeRecipientTokens", () => {
  it("separa lo cerrado por un separador de lo que sigue escribiéndose", () => {
    expect(takeRecipientTokens("a@x.com; b@y.com, c")).toEqual({ tokens: ["a@x.com", "b@y.com"], rest: "c" });
    expect(takeRecipientTokens("a@x.com,")).toEqual({ tokens: ["a@x.com"], rest: "" });
    expect(takeRecipientTokens('"Peña, Fa')).toEqual({ tokens: [], rest: '"Peña, Fa' });
  });

  it("un separador dentro de <...> lo cierra: un correo no lleva comas", () => {
    expect(takeRecipientTokens("Ana <a@x.com, b")).toEqual({ tokens: ["Ana <a@x.com>"], rest: "b" });
  });

  it("los tokens cerrados quedan balanceados", () => {
    expect(takeRecipientTokens('Ana <a@x.com>; "Peña, F" <f@x.com>,').tokens).toEqual([
      "Ana <a@x.com>",
      '"Peña, F" <f@x.com>',
    ]);
  });
});

describe("balanceRecipientToken", () => {
  it("cierra un < sin >", () => {
    expect(balanceRecipientToken("Ana <a@x.com")).toBe("Ana <a@x.com>");
  });

  it("saca la comilla que no se cerró", () => {
    expect(balanceRecipientToken('"hola')).toBe("hola");
    expect(balanceRecipientToken('Ana "x <a@x.com>')).toBe("Ana x <a@x.com>");
  });

  it("deja igual lo que ya está balanceado", () => {
    for (const t of ["a@x.com", "Ana <a@x.com>", '"Peña, F" <f@x.com>', '"Ana \\"x" <a@x.com>']) {
      expect(balanceRecipientToken(t)).toBe(t);
    }
  });

  it("después de balancear, el texto ya no se come lo que sigue", () => {
    for (const t of ['"hola', "Ana <a@x.com", '"a \\" b', 'x "y <z']) {
      expect(splitRecipientTokens(`${balanceRecipientToken(t)}, f`)).toHaveLength(2);
    }
  });
});

describe("comillas escapadas", () => {
  it("un nombre con una comilla suelta va y vuelve", () => {
    const list = [{ name: 'Ana "la', email: "a@x.com" }, { name: "", email: "b@x.com" }];
    expect(splitRecipientTokens(formatRecipients(list))).toHaveLength(2);
    expect(parseRecipients(formatRecipients(list)).valid).toEqual(list);
  });
});

describe("moveRecipient", () => {
  it("saca el destinatario de un campo y lo suma al final del otro", () => {
    expect(moveRecipient("Ana <a@x.com>, b@x.com", 0, "Ana <a@x.com>", "c@x.com")).toEqual({
      from: "b@x.com",
      to: "c@x.com, Ana <a@x.com>",
    });
  });

  it("respeta los nombres con coma y llega a un campo vacío", () => {
    expect(moveRecipient('a@x.com, "Peña, F" <f@x.com>', 1, '"Peña, F" <f@x.com>', "")).toEqual({
      from: "a@x.com",
      to: '"Peña, F" <f@x.com>',
    });
  });

  it("si el destino ya tiene esa dirección no la repite", () => {
    expect(moveRecipient("a@x.com, b@x.com", 1, "b@x.com", "Beto <B@X.com>")).toEqual({
      from: "a@x.com",
      to: "Beto <B@X.com>",
    });
  });

  it("si el campo cambió y el chip ya no está ahí no hace nada", () => {
    expect(moveRecipient("a@x.com", 1, "b@x.com", "")).toBeNull();
    expect(moveRecipient("z@x.com, b@x.com", 0, "b@x.com", "")).toBeNull();
  });

  it("también mueve una dirección inválida, para corregirla en el otro campo", () => {
    expect(moveRecipient("sin-arroba", 0, "sin-arroba", "a@x.com")).toEqual({
      from: "",
      to: "a@x.com, sin-arroba",
    });
  });
});

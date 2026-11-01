import { describe, expect, it } from "vitest";
import { formatRecipients, parseRecipients, splitRecipientTokens, takeRecipientTokens } from "./recipients";

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
});

import { describe, expect, it } from "vitest";
import { printDocument } from "./print";

const msg = {
  from: { name: "Ana <jefa>", email: "ana@x.com" },
  to: [{ name: "", email: "yo@x.com" }],
  cc: null,
  subject: "Hola & chau",
  date: "2026-11-13T13:00:00Z",
  html: '<p>Cuerpo</p><img data-remote-src="https://t.com/p.gif">',
  text: "Cuerpo",
};

describe("printDocument", () => {
  it("lleva el encabezado escapado y el cuerpo, con las imágenes remotas bloqueadas", () => {
    const doc = printDocument(msg, false);
    expect(doc).toContain("Hola &amp; chau");
    expect(doc).toContain("Ana &lt;jefa&gt; &lt;ana@x.com&gt;");
    expect(doc).toContain("yo@x.com");
    expect(doc).toContain("<p>Cuerpo</p>");
    expect(doc).not.toMatch(/\ssrc="https:\/\/t\.com/);
    expect(doc).toContain("Content-Security-Policy");
  });

  it("sin HTML imprime el texto plano", () => {
    const doc = printDocument({ ...msg, html: "", text: "línea 1\n<b>2</b>" }, false);
    expect(doc).toContain("<pre");
    expect(doc).toContain("&lt;b&gt;2&lt;/b&gt;");
  });

  it("con imágenes permitidas las muestra", () => {
    expect(printDocument(msg, true)).toMatch(/\ssrc="https:\/\/t\.com/);
  });
});

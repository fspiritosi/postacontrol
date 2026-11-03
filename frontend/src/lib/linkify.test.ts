import { describe, expect, it } from "vitest";
import { linkify } from "./linkify";

describe("linkify", () => {
  it("saca el punto final de una URL", () => {
    expect(linkify("ver https://x.com/a.")).toEqual([
      { kind: "text", text: "ver " },
      { kind: "link", text: "https://x.com/a", href: "https://x.com/a" },
      { kind: "text", text: "." },
    ]);
  });

  it("www. entre paréntesis", () => {
    expect(linkify("(www.x.com)")).toEqual([
      { kind: "text", text: "(" },
      { kind: "link", text: "www.x.com", href: "https://www.x.com" },
      { kind: "text", text: ")" },
    ]);
  });

  it("conserva el ) si hay un ( sin cerrar", () => {
    expect(linkify("https://es.wikipedia.org/wiki/A_(b)")).toEqual([
      {
        kind: "link",
        text: "https://es.wikipedia.org/wiki/A_(b)",
        href: "https://es.wikipedia.org/wiki/A_(b)",
      },
    ]);
  });

  it("detecta direcciones de correo", () => {
    expect(linkify("escribí a ana@x.com, gracias")).toEqual([
      { kind: "text", text: "escribí a " },
      { kind: "link", text: "ana@x.com", href: "mailto:ana@x.com" },
      { kind: "text", text: ", gracias" },
    ]);
  });

  it("un correo dentro de una URL no se detecta aparte", () => {
    expect(linkify("https://x.com/?u=ana@x.com")).toEqual([
      { kind: "link", text: "https://x.com/?u=ana@x.com", href: "https://x.com/?u=ana@x.com" },
    ]);
  });

  it("sin enlaces da una sola parte de texto", () => {
    expect(linkify("hola mundo")).toEqual([{ kind: "text", text: "hola mundo" }]);
  });
});

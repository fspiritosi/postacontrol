import { describe, expect, it } from "vitest";
import { buildSrcDoc } from "./srcdoc";

const html = '<p>Hola</p><img data-remote-src="https://t.example/p.gif">';

describe("buildSrcDoc", () => {
  it("bloquea remotas por defecto", () => {
    const out = buildSrcDoc(html, { allowRemote: false });
    expect(out).toContain("img-src data:;");
    expect(out).not.toMatch(/\ssrc="https:/); // data-remote-src sigue inerte
  });

  it("permite remotas a pedido", () => {
    const out = buildSrcDoc(html, { allowRemote: true });
    expect(out).toContain("img-src data: https: http:");
    expect(out).toContain('src="https://t.example/p.gif"');
    expect(out).not.toContain("data-remote-src");
  });

  it("no introduce scripts", () => {
    const out = buildSrcDoc("<p>x</p><script>alert(1)</script>", { allowRemote: false });
    expect(out).toContain("default-src 'none'");
    expect(out).not.toContain("allow-scripts");
    expect(out).toContain("<base target=\"_blank\">");
  });
});

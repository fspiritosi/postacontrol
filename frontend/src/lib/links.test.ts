import { describe, expect, it } from "vitest";
import { toLinkHref } from "./links";

describe("toLinkHref", () => {
  it("agrega https:// si no hay esquema", () => {
    expect(toLinkHref(" ejemplo.com/a ")).toBe("https://ejemplo.com/a");
  });
  it("acepta http, https y mailto", () => {
    expect(toLinkHref("http://x.y")).toBe("http://x.y");
    expect(toLinkHref("HTTPS://x.y")).toBe("HTTPS://x.y");
    expect(toLinkHref("mailto:a@b.c")).toBe("mailto:a@b.c");
  });
  it("rechaza otros esquemas", () => {
    expect(toLinkHref("javascript:alert(1)")).toBeNull();
    expect(toLinkHref("ftp://x.y")).toBeNull();
    expect(toLinkHref("data:text/html,x")).toBeNull();
  });
});

import { describe, expect, it } from "vitest";
import type { ComposerInit } from "@/features/compose/ComposerContext";
import { composeWindowId, parseInit, serializeInit } from "./composeWindow";

const init: ComposerInit = {
  accountId: 3, to: [{ name: "Ana", email: "a@x.com" }], cc: [], bcc: [], subject: "Hola", bodyHtml: "<p>x</p>",
  quotedHtml: "", reply: null, attachments: [], originalAttachments: [], skipOriginalAttachments: [], signatureId: 0,
};

describe("composeWindowId", () => {
  it("lee el id del hash", () => expect(composeWindowId("#/compose/7")).toBe(7));
  it.each(["", "#/compose/", "#/compose/x", "#/compose/0", "#/compose/-3", "#/compose/1/2"])("%j da null", (h) =>
    expect(composeWindowId(h)).toBeNull());
});

describe("serializeInit / parseInit", () => {
  it("va y vuelve", () => expect(parseInit(serializeInit(init))).toEqual(init));
  it("rechaza datos incompletos", () => {
    expect(() => parseInit("{}")).toThrow();
    expect(() => parseInit(JSON.stringify({ ...init, to: null }))).toThrow();
  });
});

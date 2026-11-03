import { describe, expect, it } from "vitest";
import { readingPane } from "./settings";

const base = { unifiedInbox: false, disableNotifications: false, disableSpellcheck: false, threadedList: false, background: "" };

describe("readingPane", () => {
  it("sin configuración o con valor vacío, abajo", () => {
    expect(readingPane(undefined)).toBe("bottom");
    expect(readingPane({ ...base, readingPane: "" })).toBe("bottom");
  });

  it("respeta un valor conocido", () => {
    expect(readingPane({ ...base, readingPane: "right" })).toBe("right");
    expect(readingPane({ ...base, readingPane: "top" })).toBe("top");
  });
});

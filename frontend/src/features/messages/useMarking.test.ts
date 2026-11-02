import { describe, expect, it, vi } from "vitest";
import type { ShellState } from "../layout/MailShell";
import { finishFlat } from "./useMarking";

const mk = (messageId: number | null) =>
  ({
    messageId,
    setMarked: vi.fn(),
    selectMessage: vi.fn(),
  }) as unknown as ShellState;

describe("finishFlat", () => {
  it("marcar no leído cierra el lector si el abierto estaba entre los hechos", () => {
    const s = mk(1);
    finishFlat(s, { ok: [1, 2], failed: [], removed: false, unread: true });
    expect(s.selectMessage).toHaveBeenCalledWith(null);
    expect(s.setMarked).toHaveBeenCalledWith({ keys: [], anchor: null });
  });

  it("marcar leído no cierra el lector", () => {
    const s = mk(1);
    finishFlat(s, { ok: [1, 2], failed: [], removed: false, unread: false });
    expect(s.selectMessage).not.toHaveBeenCalled();
  });
});

import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../bindings/postacontrol/app/mailservice", () => ({
  GetThread: vi.fn(),
}));

import * as svc from "../../bindings/postacontrol/app/mailservice";
import { runPerAccount, threadTargets, type Target } from "./bulk";

const t = (
  id: number,
  accountId: number,
  folderId = accountId * 10,
): Target => ({
  id,
  accountId,
  folderId,
  seen: true,
  flagged: false,
});

describe("runPerAccount", () => {
  it("llama una vez por cuenta con los ids de cada una", async () => {
    const fn = vi.fn(() => Promise.resolve());
    const res = await runPerAccount([t(1, 7), t(2, 8), t(3, 7)], fn);
    expect(fn).toHaveBeenCalledTimes(2);
    expect(fn).toHaveBeenCalledWith(7, [1, 3]);
    expect(fn).toHaveBeenCalledWith(8, [2]);
    expect(res.failed).toEqual([]);
  });

  it("si la segunda falla, devuelve sus ids y la primera igual se hizo", async () => {
    const boom = new Error("boom");
    const fn = vi.fn((accountId: number) =>
      accountId === 8 ? Promise.reject(boom) : Promise.resolve(),
    );
    const res = await runPerAccount([t(1, 7), t(2, 8), t(3, 8)], fn);
    expect(fn).toHaveBeenCalledTimes(2);
    expect(res.failed).toEqual([{ accountId: 8, ids: [2, 3], error: boom }]);
  });
});

describe("threadTargets", () => {
  beforeEach(() => vi.clearAllMocks());

  it("descarta los mensajes de otra carpeta", async () => {
    vi.mocked(svc.GetThread).mockResolvedValue([
      { id: 1, accountId: 7, folderId: 70, seen: false, flagged: true },
      { id: 2, accountId: 7, folderId: 71, seen: true, flagged: false },
    ] as never);
    const res = await threadTargets(70, ["a"]);
    expect(res).toEqual([
      { id: 1, accountId: 7, folderId: 70, seen: false, flagged: true },
    ]);
    expect(svc.GetThread).toHaveBeenCalledWith(70, "a");
  });
});

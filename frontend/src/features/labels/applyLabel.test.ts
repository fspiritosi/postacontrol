import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  SetLabel: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Target } from "@/lib/bulk";
import { applyLabel, labelState, toggleTargets } from "./applyLabel";

const t = (id: number, accountId: number, labels: number[]): Target => ({
  id,
  accountId,
  folderId: accountId * 10,
  seen: true,
  flagged: false,
  labels,
});

describe("labelState / toggleTargets", () => {
  it("algunos, todos o ninguno", () => {
    expect(labelState([t(1, 7, [1]), t(2, 7, [])], 1)).toBe("some");
    expect(labelState([t(1, 7, [1]), t(2, 7, [1, 2])], 1)).toBe("all");
    expect(labelState([t(1, 7, [2]), t(2, 7, [])], 1)).toBe("none");
  });

  it("se pone con algunos o ninguno y se saca solo si la tienen todos", () => {
    expect(toggleTargets([t(1, 7, [1]), t(2, 7, [])], 1)).toEqual({ on: true });
    expect(toggleTargets([t(1, 7, [])], 1)).toEqual({ on: true });
    expect(toggleTargets([t(1, 7, [1]), t(2, 7, [1])], 1)).toEqual({ on: false });
  });
});

describe("applyLabel", () => {
  beforeEach(() => vi.clearAllMocks());

  it("llama SetLabel una vez por cuenta", async () => {
    await applyLabel(new QueryClient(), [t(1, 7, []), t(2, 8, []), t(3, 7, [])], 5);
    expect(svc.SetLabel).toHaveBeenCalledTimes(2);
    expect(svc.SetLabel).toHaveBeenCalledWith([1, 3], 5, true);
    expect(svc.SetLabel).toHaveBeenCalledWith([2], 5, true);
  });

  it("si la tienen todos la saca", async () => {
    await applyLabel(new QueryClient(), [t(1, 7, [5])], 5);
    expect(svc.SetLabel).toHaveBeenCalledWith([1], 5, false);
  });

  it("avisa de cada cuenta que falla y hace las demás", async () => {
    vi.mocked(svc.SetLabel).mockRejectedValueOnce(new Error("boom"));
    const res = await applyLabel(new QueryClient(), [t(1, 7, []), t(2, 8, [])], 5);
    expect(res.failed).toHaveLength(1);
    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(svc.SetLabel).toHaveBeenCalledTimes(2);
  });
});

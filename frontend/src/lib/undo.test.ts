import { QueryClient } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../bindings/postacontrol/app/mailservice", () => ({
  UndoMove: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import { undoLast, undoableToast } from "./undo";

type Action = { label: string; onClick: () => void };
const lastAction = () => {
  const calls = vi.mocked(toast.success).mock.calls;
  return (calls[calls.length - 1]?.[1] as { action?: Action } | undefined)?.action;
};

describe("undoableToast", () => {
  let qc: QueryClient;
  beforeEach(() => {
    vi.clearAllMocks();
    qc = new QueryClient();
    vi.spyOn(qc, "invalidateQueries");
  });

  it("ofrece Deshacer, que devuelve cada lote y refresca", async () => {
    undoableToast(qc, "Archivado", [4, 0, 5]);
    const action = lastAction();
    expect(vi.mocked(toast.success).mock.calls[0][0]).toBe("Archivado");
    expect(action?.label).toBe("Deshacer");
    action!.onClick();
    await vi.waitFor(() => expect(svc.UndoMove).toHaveBeenCalledTimes(2));
    expect(svc.UndoMove).toHaveBeenCalledWith(4);
    expect(svc.UndoMove).toHaveBeenCalledWith(5);
    await vi.waitFor(() => expect(qc.invalidateQueries).toHaveBeenCalled());
    expect(toast.success).toHaveBeenLastCalledWith("Deshecho");
  });

  it("sin nada que deshacer es un aviso común", () => {
    undoableToast(qc, "Movido a la Papelera", [0]);
    expect(toast.success).toHaveBeenCalledWith("Movido a la Papelera");
    expect(lastAction()).toBeUndefined();
  });

  it("z deshace lo último una sola vez", async () => {
    undoableToast(qc, "a", [1]);
    undoableToast(qc, "b", [2]);
    expect(undoLast(qc)).toBe(true);
    await vi.waitFor(() => expect(svc.UndoMove).toHaveBeenCalledWith(2));
    expect(svc.UndoMove).not.toHaveBeenCalledWith(1);
    expect(undoLast(qc)).toBe(false);
  });

  it("si falla lo avisa", async () => {
    vi.mocked(svc.UndoMove).mockRejectedValueOnce(new Error("Ya no se puede deshacer."));
    undoableToast(qc, "a", [9]);
    lastAction()!.onClick();
    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledWith("Ya no se puede deshacer."));
  });
});

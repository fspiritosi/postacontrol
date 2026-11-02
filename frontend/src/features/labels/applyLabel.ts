import type { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import {
  invalidateMailQueries,
  runPerAccount,
  type AccountFailure,
  type Target,
} from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";

// Cuántos de los correos tienen la etiqueta: todos, algunos o ninguno.
export function labelState(
  targets: Target[],
  labelId: number,
): "all" | "some" | "none" {
  const n = targets.filter((t) => t.labels.includes(labelId)).length;
  if (n === 0) return "none";
  return n === targets.length ? "all" : "some";
}

// Con estado mixto se pone en todos; solo si ya la tienen todos se saca.
export function toggleTargets(
  targets: Target[],
  labelId: number,
): { on: boolean } {
  return { on: labelState(targets, labelId) !== "all" };
}

// Pone o saca la etiqueta (según `toggleTargets`) en los correos, una llamada
// por cuenta. Cada cuenta que falla avisa con su toast; las demás igual se hacen.
export async function applyLabel(
  queryClient: QueryClient,
  targets: Target[],
  labelId: number,
): Promise<{ failed: AccountFailure[] }> {
  const { on } = toggleTargets(targets, labelId);
  const res = await runPerAccount(targets, (_account, ids) =>
    svc.SetLabel(ids, labelId, on),
  );
  for (const f of res.failed) toast.error(errorMessage(f.error));
  invalidateMailQueries(queryClient);
  return res;
}

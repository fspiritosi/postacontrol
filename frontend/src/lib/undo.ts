import type { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import { invalidateMailQueries } from "./bulk";
import { errorMessage } from "./errors";

// Mover, archivar y mandar a la Papelera devuelven un id con qué deshacerlo
// (uno por cuenta; 0 = nada que deshacer). El aviso ofrece "Deshacer" y la
// tecla z deshace lo último.

// Cuánto queda a la vista el aviso con Deshacer.
const UNDO_TOAST_MS = 8000;

let last: number[] = [];

async function runUndo(queryClient: QueryClient, ids: number[]) {
  if (last === ids) last = [];
  try {
    for (const id of ids) await svc.UndoMove(id);
    toast.success("Deshecho");
  } catch (e) {
    toast.error(errorMessage(e));
  } finally {
    invalidateMailQueries(queryClient);
  }
}

// Avisa lo que se hizo y, si se puede deshacer, ofrece hacerlo.
export function undoableToast(queryClient: QueryClient, message: string, undoIds: number[]) {
  const ids = undoIds.filter((id) => id > 0);
  if (ids.length === 0) {
    toast.success(message);
    return;
  }
  last = ids;
  toast.success(message, {
    duration: UNDO_TOAST_MS,
    action: { label: "Deshacer", onClick: () => void runUndo(queryClient, ids) },
  });
}

// Deshace lo último que se ofreció deshacer. false si no hay nada.
export function undoLast(queryClient: QueryClient): boolean {
  if (last.length === 0) return false;
  void runUndo(queryClient, last);
  return true;
}

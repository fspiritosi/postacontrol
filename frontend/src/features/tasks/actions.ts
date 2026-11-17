import type { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "@/lib/errors";

// Marca una tarea local como hecha o pendiente y recarga las listas.
export async function setTaskDone(qc: QueryClient, id: number, done: boolean): Promise<boolean> {
  try {
    await svc.SetLocalTaskDone(id, done);
    void qc.invalidateQueries({ queryKey: ["local-tasks"] });
    return true;
  } catch (e) {
    toast.error(errorMessage(e));
    return false;
  }
}

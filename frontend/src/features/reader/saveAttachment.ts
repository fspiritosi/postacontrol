import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { errorMessage } from "@/lib/errors";

// saveAttachment abre "Guardar como" para el adjunto y avisa cómo salió.
export async function saveAttachment(messageId: number, index: number) {
  try {
    // saved=false es cancelar el diálogo: no es un error.
    if (await svc.SaveAttachment(messageId, index)) toast.success("Adjunto guardado");
  } catch (e) {
    toast.error(errorMessage(e));
  }
}

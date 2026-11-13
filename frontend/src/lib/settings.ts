import type { QueryClient } from "@tanstack/react-query";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import type { Settings } from "../../bindings/postacontrol/core/model/models";
import { useSettings } from "./queries";

export type ReadingPane = "right" | "bottom" | "top";

// Configuración vieja o vacía: el panel va abajo.
export function readingPane(s: Settings | undefined): ReadingPane {
  const v = s?.readingPane;
  return v === "right" || v === "top" ? v : "bottom";
}

export function useReadingPane(): ReadingPane {
  return readingPane(useSettings().data);
}

// Guarda la configuración entera (parte de lo actual y pisa con el parche) para
// que un campo nuevo no se pierda por un objeto armado a mano.
export async function saveSettings(
  queryClient: QueryClient,
  current: Settings | undefined,
  patch: Partial<Settings>,
): Promise<void> {
  await svc.SetSettings({
    unifiedInbox: false,
    disableNotifications: false,
    disableSpellcheck: false,
    threadedList: false,
    readingPane: "",
    background: "",
    disableUndoSend: false,
    autoBackupDir: "",
    ...current,
    ...patch,
  });
  void queryClient.invalidateQueries({ queryKey: ["settings"] });
}

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { keyTargets, shortcutKey } from "@/lib/keys";
import { useLabels } from "@/lib/queries";
import type { ShellState } from "../layout/MailShell";
import { applyLabel } from "./applyLabel";

// Teclas 1 a 9: ponen o sacan la etiqueta de esa posición en lo marcado, y si
// no hay nada marcado, en el correo o la conversación abiertos.
export function useLabelKeys(state: ShellState) {
  const queryClient = useQueryClient();
  const labels = useLabels().data;
  // El oyente se registra una vez y lee siempre lo último.
  const latest = useRef({ state, labels });
  useEffect(() => {
    latest.current = { state, labels };
  });

  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.shiftKey || e.repeat || !/^[1-9]$/.test(e.key) || !shortcutKey(e)) return;
      const { state: s, labels: ls } = latest.current;
      const label = ls?.[Number(e.key) - 1];
      if (!label) return;
      void (async () => {
        try {
          const { targets } = await keyTargets(queryClient, s, { labels: true });
          if (targets.length > 0) await applyLabel(queryClient, targets, label.id);
        } catch (err) {
          toast.error(errorMessage(err));
        }
      })();
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [queryClient]);
}

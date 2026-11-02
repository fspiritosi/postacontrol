import { CheckIcon, MinusIcon } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import {
  ContextMenuItem,
  ContextMenuSeparator,
} from "@/components/ui/context-menu";
import {
  DropdownMenuItem,
  DropdownMenuSeparator,
} from "@/components/ui/dropdown-menu";
import type { Target } from "@/lib/bulk";
import { requestSettings } from "@/lib/events";
import { useLabels } from "@/lib/queries";
import { applyLabel, labelState } from "./applyLabel";

// Las etiquetas como ítems de un menú: punto de color, marca si la tienen
// todos los correos (o raya si solo algunos) y la tecla 1–9. Sirve para el
// DropdownMenu del lector y para el ContextMenu de la fila.
export function LabelMenuItems({
  kind,
  targets,
}: {
  kind: "dropdown" | "context";
  targets: Target[];
}) {
  const queryClient = useQueryClient();
  const labels = useLabels().data ?? [];
  const Item = kind === "dropdown" ? DropdownMenuItem : ContextMenuItem;
  const Separator =
    kind === "dropdown" ? DropdownMenuSeparator : ContextMenuSeparator;
  return (
    <>
      {labels.map((l, i) => {
        const state = labelState(targets, l.id);
        return (
          <Item
            key={l.id}
            onSelect={() => void applyLabel(queryClient, targets, l.id)}
          >
            <span
              className="size-2.5 shrink-0 rounded-full"
              style={{ backgroundColor: l.color }}
              aria-hidden
            />
            <span className="flex-1 truncate">{l.name}</span>
            {state === "all" && (
              <CheckIcon className="size-3.5" data-testid="label-all" />
            )}
            {state === "some" && (
              <MinusIcon className="size-3.5" data-testid="label-some" />
            )}
            {i < 9 && (
              <span
                className="w-3 text-right text-xs text-muted-foreground"
                aria-hidden
              >
                {i + 1}
              </span>
            )}
          </Item>
        );
      })}
      {labels.length > 0 && <Separator />}
      <Item onSelect={() => requestSettings("labels")}>
        Administrar etiquetas…
      </Item>
    </>
  );
}

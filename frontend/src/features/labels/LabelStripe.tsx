import type { Label } from "../../../bindings/postacontrol/core/model/models";
import { labelChipStyle, orderedLabels } from "@/lib/labels";
import { useLabels } from "@/lib/queries";

// Las etiquetas conocidas de `ids`, en el orden de Configuración. Un id sin
// definición se ignora.
function useRowLabels(ids: number[] | null): Label[] {
  const labels = useLabels().data ?? [];
  return orderedLabels(ids ?? [], labels);
}

// Línea de 4 px en el borde izquierdo de la fila, partida en tramos iguales,
// uno por etiqueta. No captura clics: la casilla de selección queda libre.
export function LabelLine({ ids }: { ids: number[] | null }) {
  const labels = useRowLabels(ids);
  if (labels.length === 0) return null;
  return (
    <span
      className="pointer-events-none absolute inset-y-0 left-0 z-[1] flex w-1 flex-col"
      data-testid="label-line"
      aria-hidden
    >
      {labels.map((l) => (
        <span
          key={l.id}
          className="flex-1"
          data-testid="label-segment"
          style={{ backgroundColor: l.color }}
        />
      ))}
    </span>
  );
}

// Pastillas con el nombre; pasado `max` queda un "+N".
export function LabelChips({
  ids,
  max,
}: {
  ids: number[] | null;
  max: number;
}) {
  const labels = useRowLabels(ids);
  if (labels.length === 0) return null;
  return (
    <>
      {labels.slice(0, max).map((l) => (
        <span
          key={l.id}
          className="max-w-24 shrink-0 truncate rounded border px-1.5 text-xs"
          style={labelChipStyle(l.color)}
        >
          {l.name}
        </span>
      ))}
      {labels.length > max && (
        <span className="shrink-0 text-xs text-muted-foreground">
          +{labels.length - max}
        </span>
      )}
    </>
  );
}

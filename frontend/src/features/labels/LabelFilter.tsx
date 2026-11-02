import { CheckIcon, ChevronDownIcon, XIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { requestSettings } from "@/lib/events";
import { useLabels } from "@/lib/queries";

// Botón "Etiqueta ▾" del encabezado de la lista. Con una elegida muestra su
// punto de color y su nombre, y una ✕ para quitar el filtro.
export function LabelFilter({
  value,
  onChange,
}: {
  value: number | null;
  onChange: (id: number | null) => void;
}) {
  const labels = useLabels().data ?? [];
  const current = labels.find((l) => l.id === value);
  return (
    <div className="flex items-center gap-0.5">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className="h-7 gap-1.5 px-2 text-xs"
            aria-label="Filtrar por etiqueta"
          >
            {current ? (
              <>
                <span
                  className="size-2.5 shrink-0 rounded-full"
                  style={{ backgroundColor: current.color }}
                  aria-hidden
                />
                <span className="max-w-32 truncate">{current.name}</span>
              </>
            ) : (
              <>
                Etiqueta
                <ChevronDownIcon className="size-3.5" />
              </>
            )}
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {labels.length === 0 && (
            <p className="px-2 py-1.5 text-sm text-muted-foreground">
              Todavía no tenés etiquetas.
            </p>
          )}
          {labels.map((l) => (
            <DropdownMenuItem key={l.id} onSelect={() => onChange(l.id)}>
              <span
                className="size-2.5 shrink-0 rounded-full"
                style={{ backgroundColor: l.color }}
                aria-hidden
              />
              <span className="flex-1 truncate">{l.name}</span>
              {l.id === value && (
                <CheckIcon className="size-3.5" data-testid="label-all" />
              )}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem onSelect={() => requestSettings("labels")}>
            Administrar etiquetas…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {current && (
        <Button
          variant="ghost"
          size="icon-sm"
          className="size-7"
          aria-label="Quitar filtro de etiqueta"
          onClick={() => onChange(null)}
        >
          <XIcon className="size-3.5" />
        </Button>
      )}
    </div>
  );
}

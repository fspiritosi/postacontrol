import { XIcon } from "lucide-react";
import { Button } from "@/components/ui/button";

type Props = {
  count: number;
  // Conversaciones en lugar de correos sueltos.
  threads?: boolean;
  onClear: () => void;
};

// Barra sobre la lista cuando hay varios marcados.
export function SelectionBar({ count, threads, onClear }: Props) {
  return (
    <div className="flex items-center gap-2 border-b bg-primary/10 px-3 py-1.5">
      <span className="min-w-0 flex-1 truncate text-sm font-medium">
        {threads ? `${count} conversaciones seleccionadas` : `${count} seleccionados`}
      </span>
      <Button variant="ghost" size="icon-sm" aria-label="Quitar selección" onClick={onClear}>
        <XIcon />
      </Button>
    </div>
  );
}

import { SlidersHorizontalIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { activeFilterCount, joinQuery, NO_FILTERS, splitQuery, type SearchFilters } from "@/lib/searchQuery";
import { cn } from "@/lib/utils";

// Panel para armar los filtros de la búsqueda sin acordarse de la sintaxis.
// Lee los que ya trae el texto y devuelve el texto nuevo.
export function SearchFiltersButton({ text, onApply }: { text: string; onApply: (text: string) => void }) {
  const [open, setOpen] = useState(false);
  const { free, filters: current } = splitQuery(text);
  const [draft, setDraft] = useState<SearchFilters>(current);
  const count = activeFilterCount(current);

  const set = <K extends keyof SearchFilters>(k: K, v: SearchFilters[K]) => setDraft((d) => ({ ...d, [k]: v }));
  const apply = (f: SearchFilters) => {
    onApply(joinQuery(free, f));
    setOpen(false);
  };

  const field = (id: keyof SearchFilters, label: string, type = "text") => (
    <div className="grid grid-cols-[4rem_1fr] items-center gap-2">
      <Label htmlFor={`search-${id}`} className="text-muted-foreground">
        {label}
      </Label>
      <Input
        id={`search-${id}`}
        type={type}
        value={draft[id] as string}
        onChange={(e) => set(id, e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") apply(draft);
        }}
        className="h-8"
      />
    </div>
  );
  const check = (id: "unread" | "flagged" | "attachment", label: string) => (
    <label className="flex items-center gap-2 text-sm">
      <input type="checkbox" checked={draft[id]} onChange={(e) => set(id, e.target.checked)} className="size-4" />
      {label}
    </label>
  );

  return (
    <Popover
      open={open}
      onOpenChange={(o) => {
        if (o) setDraft(current);
        setOpen(o);
      }}
    >
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label="Filtros de búsqueda"
          title="Filtros de búsqueda"
          className={cn(
            "inline-flex h-5 items-center gap-0.5 rounded px-1 text-muted-foreground hover:text-foreground",
            count > 0 && "text-primary",
          )}
        >
          <SlidersHorizontalIcon className="size-3.5" />
          {count > 0 && <span className="text-xs font-medium">{count}</span>}
        </button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-80 space-y-3">
        <div className="space-y-2">
          {field("from", "De")}
          {field("to", "Para")}
          {field("subject", "Asunto")}
          {field("since", "Desde", "date")}
          {field("until", "Hasta", "date")}
        </div>
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {check("unread", "No leídos")}
          {check("flagged", "Destacados")}
          {check("attachment", "Con adjuntos")}
        </div>
        <p className="text-xs text-muted-foreground">
          También se pueden escribir: de:ana, para:, asunto:, tiene:adjunto, es:no-leído, es:destacado, desde:2026-10-01,
          hasta:31/10/2026.
        </p>
        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" size="sm" onClick={() => apply(NO_FILTERS)}>
            Quitar filtros
          </Button>
          <Button type="button" size="sm" onClick={() => apply(draft)}>
            Buscar
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}

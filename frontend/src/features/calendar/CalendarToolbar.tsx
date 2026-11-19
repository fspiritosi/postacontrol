import { ChevronLeftIcon, ChevronRightIcon, PlusIcon } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { CalendarViewId } from "@/lib/calendar";

const VIEW_LABELS: { id: CalendarViewId; label: string }[] = [
  { id: "timeGridDay", label: "Día" },
  { id: "workWeek", label: "Semana laboral" },
  { id: "timeGridWeek", label: "Semana" },
  { id: "dayGridMonth", label: "Mes" },
  { id: "listWeek", label: "Agenda" },
];

function hhmm(iso: string): string {
  const d = new Date(iso);
  // Una fecha vacía llega como el año 1: no hay hora que mostrar.
  if (!iso || d.getFullYear() < 2000) return "";
  return d.toLocaleTimeString("es-AR", { hour: "2-digit", minute: "2-digit" });
}

export function CalendarToolbar({
  title,
  view,
  onView,
  onToday,
  onPrev,
  onNext,
  onNew,
  stale,
  fetchedAt,
}: {
  title: string;
  view: CalendarViewId;
  onView: (v: CalendarViewId) => void;
  onToday: () => void;
  onPrev: () => void;
  onNext: () => void;
  onNew: () => void;
  stale: boolean;
  fetchedAt: string;
}) {
  const at = hhmm(fetchedAt);
  return (
    <div className="flex flex-wrap items-center gap-2 pb-3">
      <Button variant="outline" size="sm" onClick={onToday}>
        Hoy
      </Button>
      <Button variant="ghost" size="icon-sm" aria-label="Anterior" onClick={onPrev}>
        <ChevronLeftIcon />
      </Button>
      <Button variant="ghost" size="icon-sm" aria-label="Siguiente" onClick={onNext}>
        <ChevronRightIcon />
      </Button>
      <h2 className="min-w-0 truncate text-base font-semibold capitalize">{title}</h2>
      {stale && (
        <span className="text-xs text-muted-foreground">
          Taskapp sin conexión{at ? ` · actualizado ${at}` : ""}
        </span>
      )}
      <div className="ml-auto flex items-center gap-2">
        <div role="group" aria-label="Vista" className="flex rounded-md border border-border p-0.5">
          {VIEW_LABELS.map((v) => (
            <button
              key={v.id}
              type="button"
              aria-pressed={view === v.id}
              onClick={() => onView(v.id)}
              className={cn(
                "rounded-sm px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:text-foreground",
                view === v.id && "bg-muted text-foreground",
              )}
            >
              {v.label}
            </button>
          ))}
        </div>
        <Button size="sm" onClick={onNew}>
          <PlusIcon /> Nuevo
        </Button>
      </div>
    </div>
  );
}

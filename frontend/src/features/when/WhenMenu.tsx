import { useState, type ReactNode } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { fromLocalInput, toLocalInput, whenPresets } from "@/lib/when";

type Props = {
  // El botón que abre el menú (va como asChild del disparador).
  trigger: ReactNode;
  // Título del menú y del diálogo: "Programar envío", "Posponer hasta…".
  title: string;
  onPick: (at: Date) => void;
  align?: "start" | "end";
};

// Elegir una hora: atajos (esta tarde, mañana, el lunes) o una fecha y hora a mano.
export function WhenMenu({ trigger, title, onPick, align = "end" }: Props) {
  const [custom, setCustom] = useState<string | null>(null);
  // La hora de referencia se toma al abrir el menú, no en cada render.
  const [now, setNow] = useState<Date>(() => new Date());
  const presets = whenPresets(now);
  const chosen = custom ? fromLocalInput(custom) : null;
  const valid = chosen != null && chosen.getTime() > now.getTime();

  return (
    <>
      <DropdownMenu onOpenChange={(o) => o && setNow(new Date())}>
        <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
        <DropdownMenuContent align={align}>
          <DropdownMenuLabel>{title}</DropdownMenuLabel>
          {presets.map((p) => (
            <DropdownMenuItem key={p.label} onSelect={() => onPick(p.at)}>
              {p.label}
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onSelect={() => {
              const t = new Date();
              setNow(t);
              setCustom(toLocalInput(new Date(t.getTime() + 60 * 60 * 1000)));
            }}
          >
            Elegir fecha y hora…
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <Dialog open={custom !== null} onOpenChange={(o) => !o && setCustom(null)}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>Elegí el día y la hora.</DialogDescription>
          </DialogHeader>
          <Input
            type="datetime-local"
            aria-label="Fecha y hora"
            value={custom ?? ""}
            min={toLocalInput(now)}
            onChange={(e) => setCustom(e.target.value)}
          />
          {custom && !valid && <p className="text-xs text-destructive">Elegí una hora que todavía no pasó.</p>}
          <DialogFooter>
            <Button variant="outline" onClick={() => setCustom(null)}>
              Cancelar
            </Button>
            <Button
              disabled={!valid}
              onClick={() => {
                setCustom(null);
                if (chosen) onPick(chosen);
              }}
            >
              Listo
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

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
import { dayOptions, firstPick, whenPresets } from "@/lib/when";

// Los ítems de un menú para elegir una hora: los atajos (esta tarde, mañana,
// el lunes) y "Elegir fecha y hora…", que abre WhenDialog. now es la hora de
// referencia, tomada al abrir el menú (no al pintar).
export function WhenItems({ now, onPick, onCustom }: { now: Date; onPick: (at: Date) => void; onCustom: () => void }) {
  return (
    <>
      {whenPresets(now).map((p) => (
        <DropdownMenuItem key={p.label} onSelect={() => onPick(p.at)}>
          {p.label}
        </DropdownMenuItem>
      ))}
      <DropdownMenuSeparator />
      {/* El diálogo se abre recién cuando el menú terminó de cerrarse: si no,
          el menú le devuelve el foco a su botón y el diálogo queda sin
          responder al teclado ni al mouse. */}
      <DropdownMenuItem onSelect={() => setTimeout(onCustom, 0)}>Elegir fecha y hora…</DropdownMenuItem>
    </>
  );
}

// Para elegir el día y la hora a mano. openedAt es la hora de referencia con
// que se abrió (null = cerrado); propone una hora más tarde.
export function WhenDialog({
  title,
  openedAt,
  onPick,
  onClose,
}: {
  title: string;
  openedAt: Date | null;
  onPick: (at: Date) => void;
  onClose: () => void;
}) {
  return (
    <Dialog open={openedAt !== null} onOpenChange={(o) => !o && onClose()}>
      {openedAt && <WhenDialogBody key={openedAt.getTime()} title={title} openedAt={openedAt} onPick={onPick} onClose={onClose} />}
    </Dialog>
  );
}

const pad = (n: number) => String(n).padStart(2, "0");
const HOURS = Array.from({ length: 24 }, (_, h) => h);
const MINUTES = Array.from({ length: 12 }, (_, i) => i * 5);
const selectClass =
  "h-9 rounded-md border border-input bg-transparent px-2 text-sm shadow-xs outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/50 dark:bg-input/30";

// Desplegables comunes en vez de <input type="datetime-local">: el de
// WebKitGTK no siempre deja editar la fecha.
function WhenDialogBody({
  title,
  openedAt,
  onPick,
  onClose,
}: {
  title: string;
  openedAt: Date;
  onPick: (at: Date) => void;
  onClose: () => void;
}) {
  const days = dayOptions(openedAt);
  const [pick, setPick] = useState(() => firstPick(openedAt));
  const day = days[pick.dayIndex].date;
  const chosen = new Date(day.getFullYear(), day.getMonth(), day.getDate(), pick.hour, pick.minute);
  const valid = chosen.getTime() > openedAt.getTime();
  return (
    <DialogContent className="sm:max-w-sm">
      <DialogHeader>
        <DialogTitle>{title}</DialogTitle>
        <DialogDescription>Elegí el día y la hora.</DialogDescription>
      </DialogHeader>
      <div className="flex flex-wrap items-center gap-2">
        <select
          aria-label="Día"
          className={selectClass}
          value={pick.dayIndex}
          onChange={(e) => setPick((p) => ({ ...p, dayIndex: Number(e.target.value) }))}
        >
          {days.map((d, i) => (
            <option key={i} value={i}>
              {d.label}
            </option>
          ))}
        </select>
        <span className="text-sm text-muted-foreground">a las</span>
        <select
          aria-label="Hora"
          className={selectClass}
          value={pick.hour}
          onChange={(e) => setPick((p) => ({ ...p, hour: Number(e.target.value) }))}
        >
          {HOURS.map((h) => (
            <option key={h} value={h}>
              {pad(h)}
            </option>
          ))}
        </select>
        <span>:</span>
        <select
          aria-label="Minutos"
          className={selectClass}
          value={pick.minute}
          onChange={(e) => setPick((p) => ({ ...p, minute: Number(e.target.value) }))}
        >
          {MINUTES.map((m) => (
            <option key={m} value={m}>
              {pad(m)}
            </option>
          ))}
        </select>
      </div>
      {!valid && <p className="text-xs text-destructive">Elegí una hora que todavía no pasó.</p>}
      <DialogFooter>
        <Button variant="outline" onClick={onClose}>
          Cancelar
        </Button>
        <Button
          disabled={!valid}
          onClick={() => {
            onClose();
            onPick(chosen);
          }}
        >
          Listo
        </Button>
      </DialogFooter>
    </DialogContent>
  );
}

type Props = {
  // El botón que abre el menú (va como asChild del disparador).
  trigger: ReactNode;
  // Título del menú y del diálogo: "Programar envío", "Posponer hasta…".
  title: string;
  onPick: (at: Date) => void;
  align?: "start" | "end";
};

// Un botón con su menú para elegir una hora.
export function WhenMenu({ trigger, title, onPick, align = "end" }: Props) {
  const [now, setNow] = useState<Date>(() => new Date());
  const [custom, setCustom] = useState<Date | null>(null);
  return (
    <>
      <DropdownMenu onOpenChange={(o) => o && setNow(new Date())}>
        <DropdownMenuTrigger asChild>{trigger}</DropdownMenuTrigger>
        <DropdownMenuContent align={align}>
          <DropdownMenuLabel>{title}</DropdownMenuLabel>
          <WhenItems now={now} onPick={onPick} onCustom={() => setCustom(new Date())} />
        </DropdownMenuContent>
      </DropdownMenu>
      <WhenDialog title={title} openedAt={custom} onPick={onPick} onClose={() => setCustom(null)} />
    </>
  );
}

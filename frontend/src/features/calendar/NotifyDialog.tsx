import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

function plural(n: number, one: string, many: string) {
  return `${n} ${n === 1 ? one : many}`;
}

// Pregunta si se les avisa a los invitados. null = cancelar.
export function NotifyDialog({
  open,
  added,
  removed,
  onPick,
}: {
  open: boolean;
  added: number;
  removed: number;
  onPick: (notify: boolean | null) => void;
}) {
  const detail = [
    added > 0 ? `se suma${added === 1 ? "" : "n"} ${plural(added, "invitado", "invitados")}` : "",
    removed > 0 ? `se quita${removed === 1 ? "" : "n"} ${plural(removed, "invitado", "invitados")}` : "",
  ].filter(Boolean);
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onPick(null)}>
      <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>¿Avisar a los invitados?</DialogTitle>
          <DialogDescription>
            {detail.length > 0 ? `${detail.join(" y ")}. ` : ""}Les llega un correo con el cambio.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onPick(null)}>
            Cancelar
          </Button>
          <Button variant="secondary" onClick={() => onPick(false)}>
            No avisar
          </Button>
          <Button onClick={() => onPick(true)}>Avisar</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

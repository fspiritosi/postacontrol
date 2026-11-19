import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export type Scope = "this" | "following";

// Pregunta a qué parte de una serie se aplica el cambio. null = cancelar.
// Con allowThis en false no ofrece "Solo este" (un cambio de repetición solo
// vale para la serie).
export function ScopeDialog({
  open,
  allowThis = true,
  onPick,
}: {
  open: boolean;
  allowThis?: boolean;
  onPick: (s: Scope | null) => void;
}) {
  return (
    <Dialog open={open} onOpenChange={(o) => !o && onPick(null)}>
      <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>Evento repetido</DialogTitle>
          <DialogDescription>¿A cuáles se aplica el cambio?</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onPick(null)}>
            Cancelar
          </Button>
          {allowThis && (
            <Button variant="secondary" onClick={() => onPick("this")}>
              Solo este
            </Button>
          )}
          <Button onClick={() => onPick("following")}>Este y los siguientes</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";

export type Confirm = { title: string; text: string; action: string };

// Pide confirmar una acción. true = seguir, false = cancelar.
export function ConfirmDialog({ ask, onPick }: { ask: Confirm | null; onPick: (ok: boolean) => void }) {
  return (
    <Dialog open={ask != null} onOpenChange={(o) => !o && onPick(false)}>
      <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-sm">
        <DialogHeader>
          <DialogTitle>{ask?.title}</DialogTitle>
          <DialogDescription>{ask?.text}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={() => onPick(false)}>
            Cancelar
          </Button>
          <Button variant="destructive" onClick={() => onPick(true)}>
            {ask?.action}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

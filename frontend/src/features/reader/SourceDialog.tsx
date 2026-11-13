import { useQuery } from "@tanstack/react-query";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { errorMessage } from "@/lib/errors";

// El correo tal como está en el servidor: encabezados y partes MIME.
export function SourceDialog({ messageId, onClose }: { messageId: number; onClose: () => void }) {
  const source = useQuery({ queryKey: ["source", messageId], queryFn: () => svc.MessageSource(messageId), retry: false });

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(source.data?.text ?? "");
      toast.success("Copiado.");
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };
  const save = async () => {
    try {
      if (await svc.SaveMessageSource(messageId)) toast.success("Guardado.");
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="grid max-h-[85vh] grid-rows-[auto_minmax(0,1fr)_auto] sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>Código fuente</DialogTitle>
          <DialogDescription>
            {source.data?.truncated
              ? "Es largo: se muestra el principio. Guardalo como .eml para tenerlo entero."
              : "El correo tal como está en el servidor."}
          </DialogDescription>
        </DialogHeader>
        <pre
          aria-label="Código fuente del correo"
          className="min-h-40 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap break-all"
        >
          {source.isPending ? "Cargando…" : source.isError ? errorMessage(source.error) : source.data.text}
        </pre>
        <DialogFooter>
          <Button variant="outline" disabled={!source.data} onClick={() => void copy()}>
            Copiar
          </Button>
          <Button variant="outline" onClick={() => void save()}>
            Guardar como .eml
          </Button>
          <Button onClick={onClose}>Cerrar</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Template } from "../../../bindings/postacontrol/core/model/models";
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
import { useTemplates } from "@/lib/queries";
import { TemplateDialog } from "./TemplateDialog";

// Plantillas y respuestas rápidas del redactor.
export function TemplatesSection() {
  const templates = useTemplates().data ?? [];
  // undefined = diálogo cerrado; null = plantilla nueva.
  const [editing, setEditing] = useState<Template | null | undefined>(undefined);
  const [deleting, setDeleting] = useState<Template | null>(null);

  const remove = async (t: Template) => {
    setDeleting(null);
    try {
      await svc.DeleteTemplate(t.id);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <section aria-labelledby="templates-title" className="space-y-3">
      <div className="flex items-center justify-between gap-4">
        <div className="space-y-1">
          <h3 id="templates-title" className="text-sm font-medium">
            Plantillas
          </h3>
          <p className="text-sm text-muted-foreground">Textos que repetís seguido. En el redactor, escribí «/» y su nombre.</p>
        </div>
        <Button size="sm" variant="outline" onClick={() => setEditing(null)}>
          Nueva plantilla
        </Button>
      </div>
      {templates.length === 0 ? (
        <p className="text-sm text-muted-foreground">Todavía no creaste ninguna plantilla.</p>
      ) : (
        <ul className="space-y-1">
          {templates.map((t) => (
            <li key={t.id} className="flex items-center justify-between gap-2 text-sm">
              <span className="truncate">
                {t.name}
                {t.subject && <span className="text-muted-foreground"> · {t.subject}</span>}
              </span>
              <span className="flex shrink-0 gap-1">
                <Button size="sm" variant="ghost" aria-label={`Editar ${t.name}`} onClick={() => setEditing(t)}>
                  Editar
                </Button>
                <Button size="sm" variant="ghost" aria-label={`Eliminar ${t.name}`} onClick={() => setDeleting(t)}>
                  Eliminar
                </Button>
              </span>
            </li>
          ))}
        </ul>
      )}
      {editing !== undefined && <TemplateDialog template={editing} onClose={() => setEditing(undefined)} />}
      <Dialog open={deleting != null} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Eliminar la plantilla «{deleting?.name}»?</DialogTitle>
            <DialogDescription>Los correos que ya la usaron no cambian.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleting(null)}>
              Cancelar
            </Button>
            <Button variant="destructive" onClick={() => deleting && void remove(deleting)}>
              Eliminar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

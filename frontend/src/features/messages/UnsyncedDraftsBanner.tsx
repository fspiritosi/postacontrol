import { CloudOffIcon } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import { usePendingDrafts } from "@/lib/queries";
import { useComposer } from "../compose/ComposerContext";

function pendingText(n: number): string {
  return n === 1
    ? "1 borrador todavía no se subió al servidor."
    : `${n} borradores todavía no se subieron al servidor.`;
}

// Aviso arriba de la lista de Borradores: los borradores que solo están en esta
// compu (sin conexión, sin carpeta Borradores, contraseña rechazada…). Con
// accountId solo los de esa cuenta; con null (vista unificada), todos.
export function UnsyncedDraftsBanner({
  accountId,
}: {
  accountId: number | null;
}) {
  const pending = usePendingDrafts();
  const { openComposeDraft } = useComposer();
  const drafts = (pending.data ?? []).filter(
    (d) => accountId == null || d.request.accountId === accountId,
  );
  if (drafts.length === 0) return null;

  const open = async (d: (typeof drafts)[number]) => {
    try {
      await openComposeDraft(d);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <section
      aria-label="Borradores sin subir"
      className="space-y-1.5 border-b bg-muted/50 px-4 py-2 text-xs"
    >
      <p className="flex items-center gap-1.5 font-medium">
        <CloudOffIcon className="size-3.5 shrink-0" aria-hidden />
        {pendingText(drafts.length)}
      </p>
      <ul className="space-y-1">
        {drafts.map((d) => (
          <li key={d.id} className="flex items-center justify-between gap-2">
            <span
              className="min-w-0 truncate"
              title={d.uploadError || undefined}
            >
              {d.request.subject || "(Sin asunto)"}
            </span>
            <Button variant="outline" size="xs" onClick={() => void open(d)}>
              Abrir
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}

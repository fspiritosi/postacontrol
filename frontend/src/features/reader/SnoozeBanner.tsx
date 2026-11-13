import { useQuery, useQueryClient } from "@tanstack/react-query";
import { AlarmClockIcon } from "lucide-react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
import { invalidateMailQueries } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { formatWhen } from "@/lib/when";

// En la carpeta Pospuestos: cuándo vuelve el correo, y para devolverlo ya.
export function SnoozeBanner({ messageId, onReturned }: { messageId: number; onReturned: () => void }) {
  const queryClient = useQueryClient();
  const snooze = useQuery({ queryKey: ["snooze", messageId], queryFn: () => svc.SnoozeFor(messageId) });
  const z = snooze.data;
  if (!z) return null;

  const returnNow = async () => {
    try {
      await svc.Unsnooze(z.id);
      onReturned();
      invalidateMailQueries(queryClient);
      toast.success("Volvió a su carpeta.");
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <div className="mx-6 mt-4 flex items-center justify-between gap-3 rounded-md border bg-muted/50 px-3 py-2 text-xs">
      <span className="flex items-center gap-1.5">
        <AlarmClockIcon className="size-3.5" />
        Pospuesto: vuelve {formatWhen(new Date(z.until * 1000))}.
      </span>
      <Button size="sm" variant="outline" onClick={() => void returnNow()}>
        Devolver ahora
      </Button>
    </div>
  );
}

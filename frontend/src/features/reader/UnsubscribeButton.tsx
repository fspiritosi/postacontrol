import { useQuery } from "@tanstack/react-query";
import { MailMinusIcon } from "lucide-react";
import { useState } from "react";
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

// "Desuscribirme" en un newsletter: aparece si el correo trae cómo darse de
// baja (encabezados List-Unsubscribe). Siempre pide confirmación.
export function UnsubscribeButton({ messageId, sender }: { messageId: number; sender: string }) {
  // Mirarlo va al servidor la primera vez; sin conexión simplemente no aparece.
  const info = useQuery({
    queryKey: ["unsubscribe", messageId],
    queryFn: () => svc.UnsubscribeInfo(messageId),
    retry: false,
    staleTime: Infinity,
  });
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const u = info.data;
  if (!u?.available) return null;
  const webOnly = !u.oneClick && !u.mailto;

  const unsubscribe = async () => {
    setBusy(true);
    try {
      const res = await svc.Unsubscribe(messageId);
      setOpen(false);
      if (res.method === "post") toast.success(`Te diste de baja de ${sender}.`);
      else if (res.method === "mail") toast.success(`Mandamos el pedido de baja a ${sender}.`);
      else {
        await svc.OpenExternal(res.url);
        toast.info("Terminá la baja en la página que se abrió.");
      }
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <Button variant="ghost" size="sm" className="text-muted-foreground" onClick={() => setOpen(true)}>
        <MailMinusIcon />
        Desuscribirme
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Desuscribirme</DialogTitle>
            <DialogDescription>
              {webOnly
                ? `Para dejar de recibir correos de ${sender} se abre la página del remitente en el navegador.`
                : `¿Dejar de recibir correos de ${sender}? Se le avisa al remitente que no querés recibir más.`}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Cancelar
            </Button>
            <Button disabled={busy} onClick={() => void unsubscribe()}>
              {webOnly ? "Abrir la página" : "Darme de baja"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

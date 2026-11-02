import { MailsIcon } from "lucide-react";
import { ReaderBackdrop } from "./ReaderBackdrop";

// Lo que se ve en el lector cuando hay varios marcados en lugar de un correo.
export function MarkedPane({ count, threads }: { count: number; threads?: boolean }) {
  return (
    <ReaderBackdrop>
      <div className="flex flex-col items-center gap-2">
        <MailsIcon className="size-8 text-muted-foreground" aria-hidden />
        <p className="text-sm font-medium">
          {threads ? `${count} conversaciones seleccionadas` : `${count} correos seleccionados`}
        </p>
        <p className="text-sm text-muted-foreground">Arrastralos a una carpeta o usá la barra de arriba.</p>
      </div>
    </ReaderBackdrop>
  );
}

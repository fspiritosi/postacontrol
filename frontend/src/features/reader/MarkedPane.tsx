import { MailsIcon } from "lucide-react";

// Lo que se ve en el lector cuando hay varios marcados en lugar de un correo.
export function MarkedPane({ count, threads }: { count: number; threads?: boolean }) {
  return (
    <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
      <MailsIcon className="size-8 text-muted-foreground" aria-hidden />
      <p className="text-sm font-medium">
        {threads ? `${count} conversaciones seleccionadas` : `${count} correos seleccionados`}
      </p>
      <p className="text-sm text-muted-foreground">Arrastralos a una carpeta o usá la barra de arriba.</p>
    </div>
  );
}

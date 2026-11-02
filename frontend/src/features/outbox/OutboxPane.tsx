import { ReaderBackdrop } from "../reader/ReaderBackdrop";

// Lo que se ve en el lector de la Bandeja de salida sin un correo abierto.
export function OutboxPane() {
  return (
    <ReaderBackdrop>
      <p className="text-sm text-muted-foreground">
        Abrí un correo de la Bandeja de salida para revisarlo, o reintentá el envío.
      </p>
    </ReaderBackdrop>
  );
}

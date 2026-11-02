import type { ReactNode } from "react";
import { useBackground } from "@/lib/queries";

// Centra lo que se ve cuando no hay nada abierto; si hay una imagen de fondo
// elegida, la pinta detrás y deja el contenido sobre un recuadro legible.
export function ReaderBackdrop({ children }: { children: ReactNode }) {
  const background = useBackground();
  const image = background.data;
  if (!image) {
    return <div className="flex h-full items-center justify-center p-6 text-center">{children}</div>;
  }
  return (
    <div
      data-testid="reader-backdrop"
      className="flex h-full items-center justify-center p-6 text-center"
      style={{ backgroundImage: `url("${image}")`, backgroundSize: "cover", backgroundPosition: "center" }}
    >
      <div className="rounded-lg bg-background/80 px-4 py-3 backdrop-blur-sm">{children}</div>
    </div>
  );
}

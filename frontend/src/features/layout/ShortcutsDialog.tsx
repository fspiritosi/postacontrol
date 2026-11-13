import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  {
    title: "Moverse",
    keys: [
      [["j"], "Correo siguiente"],
      [["k"], "Correo anterior"],
      [["/"], "Buscar"],
    ],
  },
  {
    title: "Escribir",
    keys: [
      [["n"], "Redactar"],
      [["r"], "Responder"],
      [["a"], "Responder a todos"],
      [["f"], "Reenviar"],
    ],
  },
  {
    title: "Ordenar",
    keys: [
      [["e"], "Archivar"],
      [["Supr", "#"], "Mandar a la Papelera"],
      [["s"], "Destacar o quitar destacado"],
      [["u"], "Marcar como no leído"],
      [["z"], "Deshacer lo último que se movió"],
      [["1", "…", "9"], "Poner o sacar la etiqueta de esa posición"],
    ],
  },
  {
    title: "Seleccionar",
    keys: [
      [["Ctrl", "A"], "Marcar todos los de la lista"],
      [["Esc"], "Soltar lo marcado"],
    ],
  },
];

// La lista de atajos de teclado (tecla "?").
export function ShortcutsDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Atajos de teclado</DialogTitle>
          <DialogDescription>
            Actúan sobre lo marcado y, si no hay nada marcado, sobre el correo o la conversación abiertos.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4 sm:grid-cols-2">
          {GROUPS.map((g) => (
            <section key={g.title} aria-label={g.title} className="space-y-1.5">
              <h3 className="text-xs font-medium text-muted-foreground uppercase">{g.title}</h3>
              <dl className="space-y-1 text-sm">
                {g.keys.map(([keys, what]) => (
                  <div key={what} className="flex items-center justify-between gap-3">
                    <dt className="text-foreground">{what}</dt>
                    <dd className="flex shrink-0 gap-1">
                      {keys.map((k) =>
                        k === "…" ? (
                          <span key={k} className="text-muted-foreground">
                            …
                          </span>
                        ) : (
                          <kbd
                            key={k}
                            className="min-w-6 rounded border bg-muted px-1.5 py-0.5 text-center font-mono text-xs"
                          >
                            {k}
                          </kbd>
                        ),
                      )}
                    </dd>
                  </div>
                ))}
              </dl>
            </section>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}

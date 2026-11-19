import { useCallback, useRef, useState, type ReactNode } from "react";
import { NotifyDialog } from "./NotifyDialog";
import { ScopeDialog, type Scope } from "./ScopeDialog";

type Ask = { kind: "scope" } | { kind: "notify"; added: number; removed: number };

// Deja pedir el alcance y el aviso como promesas: el flujo de guardar o borrar
// se lee de corrido. Cada una resuelve null si se cancela. `dialogs` se dibuja
// una vez en el componente que lo usa.
export function useChoice() {
  const [ask, setAsk] = useState<Ask | null>(null);
  const resolver = useRef<((v: never) => void) | null>(null);

  const open = useCallback(<T,>(a: Ask) => new Promise<T>((resolve) => {
    resolver.current = resolve as (v: never) => void;
    setAsk(a);
  }), []);
  const askScope = useCallback(() => open<Scope | null>({ kind: "scope" }), [open]);
  const askNotify = useCallback((added: number, removed: number) => open<boolean | null>({ kind: "notify", added, removed }), [open]);

  const answer = (v: unknown) => {
    const r = resolver.current;
    resolver.current = null;
    setAsk(null);
    r?.(v as never);
  };

  const dialogs: ReactNode = (
    <>
      <ScopeDialog open={ask?.kind === "scope"} onPick={answer} />
      <NotifyDialog
        open={ask?.kind === "notify"}
        added={ask?.kind === "notify" ? ask.added : 0}
        removed={ask?.kind === "notify" ? ask.removed : 0}
        onPick={answer}
      />
    </>
  );
  return { askScope, askNotify, dialogs };
}

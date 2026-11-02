import { useCallback, useState } from "react";

// Subcarpetas plegadas de una cuenta, por nombre completo de la carpeta padre.
// Al renombrar o mover una carpeta cambia su nombre y vuelve a verse abierta.
const key = (accountId: number) => `pc:folders-collapsed:${accountId}`;

function load(accountId: number): Set<string> {
  try {
    const raw = localStorage.getItem(key(accountId));
    if (raw) {
      const v: unknown = JSON.parse(raw);
      if (Array.isArray(v)) return new Set(v.filter((x): x is string => typeof x === "string"));
    }
  } catch {
    // sin almacenamiento o JSON inválido: todo abierto
  }
  return new Set();
}

function save(accountId: number, names: Set<string>) {
  try {
    localStorage.setItem(key(accountId), JSON.stringify([...names]));
  } catch {
    // ignorar: recordar lo plegado es opcional
  }
}

export function useCollapsedFolders(accountId: number) {
  const [collapsed, setCollapsedState] = useState(() => load(accountId));
  const setCollapsed = useCallback(
    (name: string, value: boolean) => {
      setCollapsedState((prev) => {
        if (prev.has(name) === value) return prev;
        const next = new Set(prev);
        if (value) next.add(name);
        else next.delete(name);
        save(accountId, next);
        return next;
      });
    },
    [accountId],
  );
  return { collapsed, setCollapsed };
}

import type { QueryClient } from "@tanstack/react-query";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import type {
  Folder,
  MessageSummary,
} from "../../bindings/postacontrol/core/model/models";

// Lo que cambia después de mover, borrar o marcar correos.
export function invalidateMailQueries(queryClient: QueryClient) {
  void queryClient.invalidateQueries({ queryKey: ["messages"] });
  void queryClient.invalidateQueries({ queryKey: ["threads"] });
  void queryClient.invalidateQueries({ queryKey: ["thread"] });
  void queryClient.invalidateQueries({ queryKey: ["folders"] });
  // Lo movido, borrado o marcado puede estar en los resultados de la búsqueda.
  void queryClient.invalidateQueries({ queryKey: ["search"] });
  // Sin refetch: abrir un correo lo marca leído en el servidor, así que el
  // detalle solo queda viejo y se vuelve a pedir cuando se lo abra.
  void queryClient.invalidateQueries({
    queryKey: ["message"],
    refetchType: "none",
  });
}

// Un correo sobre el que se puede actuar, con lo justo para decidir qué ofrecer.
export type Target = {
  id: number;
  accountId: number;
  folderId: number;
  seen: boolean;
  flagged: boolean;
};

export type AccountFailure = {
  accountId: number;
  ids: number[];
  error: unknown;
};

// El núcleo rechaza ids de más de una cuenta: se agrupa por cuenta y se llama
// una vez por cada una, en secuencia. Nunca rechaza: lo que falla vuelve en
// `failed` y las demás cuentas igual se hacen.
export async function runPerAccount(
  targets: Target[],
  fn: (accountId: number, ids: number[]) => Promise<void>,
): Promise<{ failed: AccountFailure[] }> {
  const byAccount = new Map<number, number[]>();
  for (const t of targets)
    byAccount.set(t.accountId, [...(byAccount.get(t.accountId) ?? []), t.id]);
  const failed: AccountFailure[] = [];
  for (const [accountId, ids] of byAccount) {
    try {
      await fn(accountId, ids);
    } catch (error) {
      failed.push({ accountId, ids, error });
    }
  }
  return { failed };
}

// Los mensajes de las conversaciones marcadas. GetThread trae también las
// respuestas que viven en Enviados: solo se toca lo de la carpeta que se mira.
export async function threadTargets(
  folderId: number,
  roots: string[],
): Promise<Target[]> {
  const threads = await Promise.all(
    roots.map((r) => svc.GetThread(folderId, r)),
  );
  return threads
    .flatMap((msgs) => msgs ?? [])
    .filter((m) => m.folderId === folderId)
    .map((m) => ({
      id: m.id,
      accountId: m.accountId,
      folderId: m.folderId,
      seen: m.seen,
      flagged: m.flagged,
    }));
}

// Mueve los correos a una carpeta (la barra y, después, soltar sobre una carpeta).
export function moveMarked(targets: Target[], folder: Folder) {
  return runPerAccount(targets, (_account, ids) =>
    svc.MoveMessages(ids, folder.id),
  );
}

// Los marcados que siguen cargados en la lista. Un id marcado que ya no está
// (se movió, se borró o quedó fuera del filtro) se salta: nunca se actúa sobre
// lo que no se ve.
export function loadedTargets(
  items: MessageSummary[],
  keys: number[],
): Target[] {
  const marked = new Set(keys);
  return items
    .filter((m) => marked.has(m.id))
    .map((m) => ({
      id: m.id,
      accountId: m.accountId,
      folderId: m.folderId,
      seen: m.seen,
      flagged: m.flagged,
    }));
}

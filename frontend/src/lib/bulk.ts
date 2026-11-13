import type { QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import type {
  Folder,
  MessageSummary,
} from "../../bindings/postacontrol/core/model/models";
import { errorMessage } from "./errors";
import { folderLabel } from "./folders";
import { undoableToast } from "./undo";

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
  // Ids de las etiquetas que ya tiene.
  labels: number[];
};

export type AccountFailure = {
  accountId: number;
  ids: number[];
  error: unknown;
};

// El núcleo rechaza ids de más de una cuenta: se agrupa por cuenta y se llama
// una vez por cada una, en secuencia. Nunca rechaza: lo que falla vuelve en
// `failed` y las demás cuentas igual se hacen.
export async function runPerAccount<R = unknown>(
  targets: Target[],
  fn: (accountId: number, ids: number[]) => Promise<R>,
): Promise<{ failed: AccountFailure[]; results: R[] }> {
  const byAccount = new Map<number, number[]>();
  for (const t of targets)
    byAccount.set(t.accountId, [...(byAccount.get(t.accountId) ?? []), t.id]);
  const failed: AccountFailure[] = [];
  const results: R[] = [];
  for (const [accountId, ids] of byAccount) {
    try {
      results.push(await fn(accountId, ids));
    } catch (error) {
      failed.push({ accountId, ids, error });
    }
  }
  return { failed, results };
}

// Los mensajes de las conversaciones marcadas. GetThread trae también las
// respuestas que viven en Enviados: solo se toca lo de la carpeta que se mira,
// salvo con allFolders (ver threadLabelTargets).
export async function threadTargets(
  folderId: number,
  roots: string[],
  opts: { allFolders?: boolean } = {},
): Promise<Target[]> {
  const threads = await Promise.all(
    roots.map((r) => svc.GetThread(folderId, r)),
  );
  return threads
    .flatMap((msgs) => msgs ?? [])
    .filter((m) => opts.allFolders || m.folderId === folderId)
    .map((m) => ({
      id: m.id,
      accountId: m.accountId,
      folderId: m.folderId,
      seen: m.seen,
      flagged: m.flagged,
      labels: m.labels ?? [],
    }));
}

// Para poner o sacar etiquetas: todos los mensajes de las conversaciones,
// también los de Enviados, porque la fila muestra las etiquetas de todos. Son
// de una sola cuenta y SetLabel acepta varias carpetas.
export function threadLabelTargets(
  folderId: number,
  roots: string[],
): Promise<Target[]> {
  return threadTargets(folderId, roots, { allFolders: true });
}

// Mueve los correos a una carpeta (la barra y, después, soltar sobre una carpeta).
export function moveMarked(targets: Target[], folder: Folder) {
  return runPerAccount(targets, (_account, ids) =>
    svc.MoveMessages(ids, folder.id),
  );
}

// Mueve y avisa, igual desde "Mover a…" que al soltar sobre una carpeta: un
// toast por cuenta que falló, otro con lo que se movió, y refresca las
// consultas. Devuelve qué ids salieron bien y cuáles no.
export async function moveAndReport(
  targets: Target[],
  folder: Folder,
  nameOf: (accountId: number) => string,
  queryClient: QueryClient,
): Promise<{ ok: number[]; failed: number[] }> {
  const { failed, results } = await moveMarked(targets, folder);
  const failedIds = new Set(failed.flatMap((f) => f.ids));
  const done = targets.filter((t) => !failedIds.has(t.id));
  for (const f of failed)
    toast.error(
      `No se pudo en ${nameOf(f.accountId)}: ${errorMessage(f.error)}`,
    );
  if (done.length > 0)
    undoableToast(queryClient, `${done.length} movidos a ${folderLabel(folder)}`, results);
  invalidateMailQueries(queryClient);
  return { ok: done.map((t) => t.id), failed: [...failedIds] };
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
      labels: m.labels ?? [],
    }));
}

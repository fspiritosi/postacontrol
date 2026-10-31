import { useInfiniteQuery, useQueries, useQuery, type UseQueryResult } from "@tanstack/react-query";
import * as svc from "../../bindings/postacontrol/app/mailservice";
import type { Account, Folder, UnifiedView } from "../../bindings/postacontrol/core/model/models";
import type { View } from "./view";

export const PAGE_SIZE = 100;

// Los listados generados llegan como `T[] | null` (R24): se normalizan acá.
export function useAccounts() {
  return useQuery({
    queryKey: ["accounts"],
    queryFn: async () => (await svc.ListAccounts()) ?? [],
  });
}

export function useFolders(accountId: number | null) {
  return useQuery({
    queryKey: ["folders", accountId],
    queryFn: async () => (await svc.ListFolders(accountId!)) ?? [],
    enabled: accountId != null,
  });
}

// Junta las carpetas de todas las cuentas. combine va fuera del hook para que
// su referencia sea estable y el resultado no cambie en cada render.
function combineFolders(results: UseQueryResult<Folder[]>[]) {
  return {
    folders: results.flatMap((r) => r.data ?? []),
    pending: results.some((r) => r.isPending),
  };
}

export function useAllFolders(accounts: Account[]): { folders: Folder[]; pending: boolean } {
  return useQueries({
    queries: accounts.map((a) => ({
      queryKey: ["folders", a.id],
      queryFn: async () => (await svc.ListFolders(a.id)) ?? [],
    })),
    combine: combineFolders,
  });
}

export function useSettings() {
  return useQuery({
    queryKey: ["settings"],
    queryFn: () => svc.GetSettings(),
  });
}

function messagesKey(view: View | null, unreadOnly: boolean) {
  if (view?.kind === "unified") return ["messages", "unified", view.role, unreadOnly];
  return ["messages", view?.kind === "folder" ? view.folderId : null, unreadOnly];
}

export function useMessages(view: View | null, unreadOnly: boolean) {
  return useInfiniteQuery({
    queryKey: messagesKey(view, unreadOnly),
    initialPageParam: 0,
    queryFn: async ({ pageParam }) => {
      const q = { unreadOnly, offset: pageParam, limit: PAGE_SIZE };
      if (view?.kind === "unified") {
        return (await svc.ListUnified(view.role as UnifiedView, q)) ?? [];
      }
      if (view?.kind === "folder") return (await svc.ListMessages(view.folderId, q)) ?? [];
      return [];
    },
    getNextPageParam: (last, all) =>
      last.length < PAGE_SIZE ? undefined : all.length * PAGE_SIZE,
    enabled: view?.kind === "folder" || view?.kind === "unified",
  });
}

export function useMessage(id: number | null) {
  return useQuery({
    queryKey: ["message", id],
    queryFn: () => svc.GetMessage(id!),
    enabled: id != null,
  });
}

export function useDrafts() {
  return useQuery({
    queryKey: ["drafts"],
    queryFn: async () => (await svc.ListLocalDrafts()) ?? [],
  });
}

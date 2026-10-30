import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import * as svc from "../../bindings/postacontrol/app/mailservice";

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

export function useMessages(folderId: number | null, unreadOnly: boolean) {
  return useInfiniteQuery({
    queryKey: ["messages", folderId, unreadOnly],
    initialPageParam: 0,
    queryFn: async ({ pageParam }) =>
      (await svc.ListMessages(folderId!, {
        unreadOnly,
        offset: pageParam,
        limit: PAGE_SIZE,
      })) ?? [],
    getNextPageParam: (last, all) =>
      last.length < PAGE_SIZE ? undefined : all.length * PAGE_SIZE,
    enabled: folderId != null,
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

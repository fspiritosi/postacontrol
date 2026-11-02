import {
  keepPreviousData,
  useInfiniteQuery,
  useQueries,
  useQuery,
  type UseQueryResult,
} from "@tanstack/react-query";
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

export function useSignatures() {
  return useQuery({
    queryKey: ["signatures"],
    queryFn: async () => (await svc.ListSignatures()) ?? [],
  });
}

export function useContacts(query: string) {
  return useQuery({
    queryKey: ["contacts", query],
    queryFn: async () => (await svc.ListContacts(query)) ?? [],
    // Mientras busca queda la lista anterior: no se vacía con cada tecla.
    placeholderData: keepPreviousData,
  });
}

// id null o 0 (ficha nueva): sin consulta.
export function useContact(id: number | null) {
  return useQuery({
    queryKey: ["contact", id],
    queryFn: () => svc.GetContact(id!),
    enabled: id != null && id !== 0,
  });
}

export function useContactForEmail(email: string) {
  return useQuery({
    queryKey: ["contact-for", email.toLowerCase()],
    queryFn: () => svc.ContactForEmail(email),
    enabled: email !== "",
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

// Data URL del fondo del panel de lectura (sin datos si no hay). La clave es
// el archivo guardado en la configuración, que cambia de nombre con cada
// fondo nuevo: la imagen (hasta 20 MB) se lee una vez por fondo, no cada vez
// que cambia otra opción.
export function useBackground() {
  const file = useSettings().data?.background ?? "";
  return useQuery({
    queryKey: ["background", file],
    queryFn: () => svc.BackgroundImage(),
    enabled: file !== "",
    staleTime: Infinity,
  });
}

function messagesKey(view: View | null, unreadOnly: boolean) {
  if (view?.kind === "unified") return ["messages", "unified", view.role, unreadOnly];
  return ["messages", view?.kind === "folder" ? view.folderId : null, unreadOnly];
}

export function useMessages(view: View | null, unreadOnly: boolean, enabled = true) {
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
    enabled: enabled && (view?.kind === "folder" || view?.kind === "unified"),
  });
}

// Hilos de una carpeta, del más reciente al más viejo.
export function useThreads(folderId: number, unreadOnly: boolean) {
  return useInfiniteQuery({
    queryKey: ["threads", folderId, unreadOnly],
    initialPageParam: 0,
    queryFn: async ({ pageParam }) =>
      (await svc.ListThreads(folderId, { unreadOnly, offset: pageParam, limit: PAGE_SIZE })) ?? [],
    getNextPageParam: (last, all) =>
      last.length < PAGE_SIZE ? undefined : all.length * PAGE_SIZE,
  });
}

// Mensajes de un hilo (también los de Enviados), del más viejo al más nuevo.
export function useThread(folderId: number | null, root: string | null) {
  return useQuery({
    queryKey: ["thread", folderId, root],
    queryFn: async () => (await svc.GetThread(folderId!, root!)) ?? [],
    enabled: folderId != null && root != null,
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

// Borradores autoguardados que todavía no se subieron a la carpeta Borradores
// del servidor (de todas las cuentas).
export function usePendingDrafts() {
  return useQuery({
    queryKey: ["pending-drafts"],
    queryFn: async () => (await svc.ListPendingDrafts()) ?? [],
  });
}

// Búsqueda en el índice local; folderId 0 = todas las carpetas de todas las
// cuentas. `pending` (mensajes sin el cuerpo indexado) se toma de la primera
// página, que es la que corresponde a todo el alcance.
export function useSearch(text: string, folderId: number) {
  return useInfiniteQuery({
    queryKey: ["search", text, folderId],
    initialPageParam: 0,
    queryFn: async ({ pageParam }) => {
      const r = await svc.Search({ text, folderId, labelId: 0, limit: PAGE_SIZE, offset: pageParam });
      return { messages: r.messages ?? [], pending: r.pending };
    },
    getNextPageParam: (last, all) =>
      last.messages.length < PAGE_SIZE ? undefined : all.length * PAGE_SIZE,
    enabled: text.trim() !== "",
  });
}

// Direcciones sugeridas al escribir un destinatario; exclude son las que ya
// están en el campo. Mientras llega la respuesta se sigue viendo la anterior.
export function useSuggestions(query: string, exclude: string[]) {
  return useQuery({
    queryKey: ["suggest", query, exclude],
    queryFn: async () => (await svc.SuggestRecipients(query, exclude)) ?? [],
    enabled: query.trim() !== "",
    placeholderData: keepPreviousData,
  });
}

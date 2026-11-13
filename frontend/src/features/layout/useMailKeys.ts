import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Folder } from "../../../bindings/postacontrol/core/model/models";
import { invalidateMailQueries, runPerAccount, type Target } from "@/lib/bulk";
import { errorMessage } from "@/lib/errors";
import { keyTargets, latestOfThread, shortcutKey } from "@/lib/keys";
import { useComposer } from "../compose/ComposerContext";
import type { ShellState } from "./MailShell";

// Una fila de la lista que se ve (correos sueltos, búsqueda o conversaciones),
// en el orden de la pantalla. MessageRow y ThreadList las marcan con data-row-*.
type Row = { el: HTMLElement; id: number | null; root: string | null };

function visibleRows(): Row[] {
  return [...document.querySelectorAll<HTMLElement>("[data-row-id], [data-row-root]")].map((el) => ({
    el,
    id: el.dataset.rowId != null ? Number(el.dataset.rowId) : null,
    root: el.dataset.rowRoot ?? null,
  }));
}

function openIndex(rows: Row[], s: ShellState): number {
  return rows.findIndex((r) => (r.id != null ? r.id === s.messageId : r.root === s.threadRoot));
}

function openRow(s: ShellState, row: Row | undefined) {
  if (!row) return;
  if (row.id != null) s.selectMessage(row.id);
  else if (row.root != null) s.selectThread(row.root);
  row.el.scrollIntoView?.({ block: "nearest" });
}

// Lo que se abre después de sacar de la lista el abierto: el siguiente, o el
// anterior si era el último.
function rowAfterRemoving(s: ShellState): Row | undefined {
  const rows = visibleRows();
  const i = openIndex(rows, s);
  if (i < 0) return undefined;
  return rows[i + 1] ?? rows[i - 1];
}

const plural = (n: number, one: string, many: string) => (n === 1 ? one : `${n} ${many}`);

// Atajos de teclado de la lista y el lector, como los de Gmail:
//
//   j / k       siguiente / anterior
//   r a f       responder, responder a todos, reenviar
//   e           archivar
//   Supr / #    a la Papelera
//   s           destacar o quitar destacado
//   u           marcar como no leído
//   n           redactar
//   /           buscar
//   ?           esta ayuda
//
// Actúan sobre lo marcado y, si no hay nada marcado, sobre lo abierto. No
// valen escribiendo, con un diálogo o un menú abierto, ni con Ctrl/Alt.
export function useMailKeys(state: ShellState, composeAccountId: number | undefined) {
  const queryClient = useQueryClient();
  const composer = useComposer();
  const [helpOpen, setHelpOpen] = useState(false);
  // El oyente se registra una vez y lee siempre lo último.
  const latest = useRef({ state, composeAccountId, composer });
  useEffect(() => {
    latest.current = { state, composeAccountId, composer };
  });

  useEffect(() => {
    const folderRole = async (t: Target) => {
      const folders = await queryClient.fetchQuery({
        queryKey: ["folders", t.accountId],
        queryFn: async (): Promise<Folder[]> => (await svc.ListFolders(t.accountId)) ?? [],
      });
      return folders.find((f) => f.id === t.folderId)?.role ?? "";
    };

    // Corre fn por cuenta sobre los destinos, avisa lo que falló y, si se
    // sacaron de la lista, abre el siguiente (o suelta lo marcado).
    const act = async (
      s: ShellState,
      fn: (ids: number[]) => Promise<void>,
      opts: {
        removes: boolean;
        check?: (targets: Target[]) => Promise<boolean>;
        done?: (n: number) => void;
        after?: (fromOpen: boolean) => void;
      },
    ) => {
      try {
        const { targets, fromOpen } = await keyTargets(queryClient, s);
        if (targets.length === 0) return;
        if (opts.check && !(await opts.check(targets))) return;
        const next = opts.removes && fromOpen ? rowAfterRemoving(s) : undefined;
        const { failed } = await runPerAccount(targets, (_a, ids) => fn(ids));
        for (const f of failed) toast.error(errorMessage(f.error));
        const ok = targets.length - failed.reduce((n, f) => n + f.ids.length, 0);
        invalidateMailQueries(queryClient);
        if (ok > 0) opts.done?.(ok);
        if (opts.removes) {
          if (!fromOpen) s.clearMarks();
          else if (ok > 0) {
            if (next) openRow(s, next);
            else if (s.messageId != null) s.selectMessage(null);
            else s.selectThread(null);
          }
        }
        opts.after?.(fromOpen);
      } catch (e) {
        toast.error(errorMessage(e));
      }
    };

    const onKeyDown = (e: KeyboardEvent) => {
      if (!shortcutKey(e)) return;
      const { state: s, composeAccountId: accountId, composer: c } = latest.current;
      const key = e.key;
      const mailView = s.view?.kind === "folder" || s.view?.kind === "unified";

      if (key === "?") {
        e.preventDefault();
        setHelpOpen(true);
        return;
      }
      if (key === "/") {
        const search = document.querySelector<HTMLInputElement>('input[aria-label="Buscar correo"]');
        if (search) {
          e.preventDefault();
          search.focus();
        }
        return;
      }
      if (key === "n" && !e.shiftKey) {
        if (accountId != null) {
          e.preventDefault();
          c.openNew(accountId);
        }
        return;
      }
      if (!mailView) return;

      if ((key === "j" || key === "k") && !e.shiftKey) {
        const rows = visibleRows();
        if (rows.length === 0) return;
        e.preventDefault();
        const i = openIndex(rows, s);
        const target = i < 0 ? (key === "j" ? 0 : rows.length - 1) : i + (key === "j" ? 1 : -1);
        openRow(s, rows[target]);
        return;
      }
      if (e.repeat) return;

      const replyMode = { r: "reply", a: "replyAll", f: "forward" }[key] as
        | "reply"
        | "replyAll"
        | "forward"
        | undefined;
      if (replyMode && !e.shiftKey) {
        const id = s.messageId ?? (s.threadRoot != null ? latestOfThread(queryClient, s.threadRoot) : null);
        if (id == null) return;
        e.preventDefault();
        void c.openFrom(id, replyMode).catch((err: unknown) => toast.error(errorMessage(err)));
        return;
      }

      if (key === "e" && !e.shiftKey) {
        e.preventDefault();
        void act(s, (ids) => svc.ArchiveMessages(ids), {
          removes: true,
          done: (n) => toast.success(plural(n, "Archivado", "archivados")),
        });
      } else if (key === "Delete" || key === "#") {
        e.preventDefault();
        void act(s, (ids) => svc.DeleteMessages(ids), {
          removes: true,
          // En la Papelera es definitivo: eso se hace con el botón, que confirma.
          check: async (targets) => {
            const roles = await Promise.all(targets.map(folderRole));
            if (!roles.includes("trash")) return true;
            toast.warning("En la Papelera se elimina definitivamente: usá el botón Eliminar, que pide confirmación.");
            return false;
          },
          done: (n) => toast.success(plural(n, "Movido a la Papelera", "movidos a la Papelera")),
        });
      } else if (key === "s" && !e.shiftKey) {
        e.preventDefault();
        void (async () => {
          const { targets } = await keyTargets(queryClient, s).catch(() => ({ targets: [] as Target[] }));
          const flag = !targets.every((t) => t.flagged);
          await act(s, (ids) => svc.SetFlagged(ids, flag), { removes: false });
          void queryClient.invalidateQueries({ queryKey: ["message"] });
        })();
      } else if (key === "u" && !e.shiftKey) {
        e.preventDefault();
        // Como "Marcar como no leído": se cierra el lector, que si no lo
        // volvería a marcar leído.
        void act(s, (ids) => svc.SetSeen(ids, false), {
          removes: false,
          after: (fromOpen) => {
            if (!fromOpen) return;
            if (s.messageId != null) s.selectMessage(null);
            else s.selectThread(null);
          },
        });
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [queryClient]);

  return { helpOpen, setHelpOpen };
}

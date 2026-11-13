import { MailIcon, PencilIcon } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import type { Account, Folder } from "../../../bindings/postacontrol/core/model/models";
import { onOpenMessage, syncFolder, syncUnified } from "@/lib/events";
import { useAllFolders, useSettings } from "@/lib/queries";
import { NO_MARKS, type Marks } from "@/lib/selection";
import { useReadingPane, type ReadingPane } from "@/lib/settings";
import { defaultView, loadView, saveView, viewAccount, viewIsValid, type View } from "@/lib/view";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { useComposer } from "../compose/ComposerContext";
import { useLabelKeys } from "../labels/useLabelKeys";
import { ShortcutsDialog } from "./ShortcutsDialog";
import { useMailKeys } from "./useMailKeys";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";

type Sizes = Record<string, number>;
type Layouts = { outer: Sizes; inner: Sizes };

// Los tamaños se guardan por posición del panel de lectura: la clave vieja
// (pc:layout, de tres paneles en fila) ya no sirve y se ignora.
const layoutKey = (pane: ReadingPane) => `pc:layout:${pane}`;

function defaultLayouts(pane: ReadingPane): Layouts {
  return {
    outer: { sidebar: 18, main: 82 },
    inner: pane === "right" ? { list: 36, reader: 64 } : { list: 40, reader: 60 },
  };
}

const hasKeys = (l: unknown, keys: string[]): l is Sizes =>
  typeof l === "object" && l != null && keys.every((k) => typeof (l as Sizes)[k] === "number");

function loadLayouts(pane: ReadingPane): Layouts {
  try {
    const raw = localStorage.getItem(layoutKey(pane));
    if (raw) {
      const l = JSON.parse(raw) as Partial<Layouts>;
      if (hasKeys(l.outer, ["sidebar", "main"]) && hasKeys(l.inner, ["list", "reader"])) {
        return { outer: l.outer, inner: l.inner };
      }
    }
  } catch {
    // sin almacenamiento o JSON inválido: se usan los tamaños por defecto
  }
  return defaultLayouts(pane);
}

// Guarda solo uno de los dos grupos y deja el otro como estaba.
function saveLayout(pane: ReadingPane, group: keyof Layouts, sizes: Sizes) {
  try {
    const current = loadLayouts(pane);
    localStorage.setItem(layoutKey(pane), JSON.stringify({ ...current, [group]: sizes }));
  } catch {
    // ignorar: guardar el layout es opcional
  }
}

export type SearchScope = "all" | "folder";
export type SearchState = { text: string; scope: SearchScope };

export type ShellState = {
  view: View | null;
  messageId: number | null;
  // Cambia la vista (que se guarda y se sincroniza) y limpia el mensaje y el hilo abiertos.
  selectView: (v: View) => void;
  // Abre un mensaje de la lista plana: cierra el hilo abierto.
  selectMessage: (id: number | null) => void;
  // Búsqueda en curso (null = lista normal). Cambiar de vista la limpia;
  // cambiarla cierra el hilo abierto.
  search: SearchState | null;
  setSearch: (s: SearchState | null) => void;
  // Hilo abierto en el lector (lista en conversaciones). Abrir uno limpia el mensaje.
  threadRoot: string | null;
  selectThread: (root: string | null) => void;
  // Ficha abierta en la libreta (0 = ficha nueva en edición).
  contactId: number | null;
  selectContact: (id: number | null) => void;
  // Pasa a la libreta con esa ficha abierta.
  openContact: (id: number) => void;
  // Correos marcados para actuar sobre varios (listas planas y búsqueda) y
  // conversaciones marcadas (por raíz). Abrir algo, cambiar de vista o buscar
  // los suelta: si no, una acción tocaría correos que ya no se ven.
  marked: Marks<number>;
  markedThreads: Marks<string>;
  setMarked: (m: Marks<number>) => void;
  setMarkedThreads: (m: Marks<string>) => void;
  clearMarks: () => void;
  // Etiqueta por la que se filtra la lista (null = sin filtro). Cambiar de
  // vista la limpia; cambiarla suelta lo marcado, que puede quedar oculto.
  labelFilter: number | null;
  setLabelFilter: (id: number | null) => void;
};

type Props = {
  accounts: Account[];
  // Slots que llenan las Tasks 16 y 17.
  sidebar?: (s: ShellState) => ReactNode;
  list?: (s: ShellState) => ReactNode;
  reader?: (s: ShellState) => ReactNode;
  // Componentes sin lugar en pantalla que necesitan el estado (p. ej. avisos).
  extras?: (s: ShellState) => ReactNode;
};

function Placeholder({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full items-center justify-center p-6 text-sm text-muted-foreground">
      {children}
    </div>
  );
}

// Pide sincronizar lo que muestra la vista, sin esperar: la lista se
// refresca por eventos cuando termina.
function syncView(v: View, folders: Folder[]) {
  if (v.kind === "folder") {
    const f = folders.find((x) => x.id === v.folderId);
    if (f) void syncFolder(f.accountId, f.id);
  } else if (v.kind === "unified") {
    void syncUnified(v.role);
  }
  // outbox y contacts no tienen nada que traer del servidor.
}

export function MailShell({ accounts, sidebar, list, reader, extras }: Props) {
  const [chosen, setChosen] = useState<View | null>(loadView);
  const [messageId, setMessageId] = useState<number | null>(null);
  const [search, setSearchState] = useState<SearchState | null>(null);
  const [threadRoot, setThreadRoot] = useState<string | null>(null);
  const [contactId, setContactId] = useState<number | null>(null);
  const [marked, setMarked] = useState<Marks<number>>(NO_MARKS);
  const [markedThreads, setMarkedThreads] = useState<Marks<string>>(NO_MARKS);
  const [labelFilter, setLabelFilterState] = useState<number | null>(null);
  const { folders, pending } = useAllFolders(accounts);
  const settings = useSettings();
  const pane = useReadingPane();
  // Los tamaños arrancan según la posición; al cambiarla se leen los de la nueva.
  const layout = useMemo(() => loadLayouts(pane), [pane]);
  const composer = useComposer();
  // La última vista elegida que ya se guardó y se mandó a sincronizar.
  const syncedRef = useRef<View | null>(null);

  const unifiedEnabled = settings.data?.unifiedInbox ?? false;
  // Mientras cargan las carpetas o la configuración no se puede saber si la
  // vista sigue existiendo: se la deja como está.
  const ready = !pending && !settings.isPending;
  const valid = chosen != null && viewIsValid(chosen, folders, unifiedEnabled);

  // Si la vista dejó de valer (se quitó la cuenta, se apagó la unificada) se
  // pasa a la de por defecto ya en este render, para no mostrar una vista que
  // no existe. Deja de cumplirse apenas se aplica: la de por defecto es válida.
  if (ready && !valid) {
    const d = defaultView(folders);
    if (d) {
      setChosen(d);
      setMessageId(null);
      setSearchState(null);
      setThreadRoot(null);
      setContactId(null);
      setMarked(NO_MARKS);
      setMarkedThreads(NO_MARKS);
      setLabelFilterState(null);
    }
  }

  // Cada vista elegida (un objeto nuevo por elección, así volver a hacer clic
  // en la misma carpeta la sincroniza otra vez) se guarda y se sincroniza sin
  // esperar: la lista se refresca por eventos cuando termina.
  useEffect(() => {
    if (!ready || !valid || chosen == null || syncedRef.current === chosen) return;
    syncedRef.current = chosen;
    saveView(chosen);
    syncView(chosen, folders);
  }, [ready, valid, chosen, folders]);

  const clearMarks = useCallback(() => {
    setMarked(NO_MARKS);
    setMarkedThreads(NO_MARKS);
  }, []);

  const selectView = useCallback((v: View) => {
    clearMarks();
    setChosen(v);
    setMessageId(null);
    setSearchState(null);
    setThreadRoot(null);
    setContactId(null);
    setLabelFilterState(null);
  }, [clearMarks]);

  const setLabelFilter = useCallback((id: number | null) => {
    clearMarks();
    setLabelFilterState(id);
  }, [clearMarks]);

  const openContact = useCallback((id: number) => {
    clearMarks();
    setChosen({ kind: "contacts" });
    setLabelFilterState(null);
    setMessageId(null);
    setSearchState(null);
    setThreadRoot(null);
    setContactId(id);
  }, [clearMarks]);

  const selectMessage = useCallback((id: number | null) => {
    if (id != null) clearMarks();
    setMessageId(id);
    setThreadRoot(null);
  }, [clearMarks]);

  const selectThread = useCallback((root: string | null) => {
    if (root != null) clearMarks();
    setThreadRoot(root);
    setMessageId(null);
  }, [clearMarks]);

  const setSearch = useCallback((s: SearchState | null) => {
    clearMarks();
    setSearchState(s);
    setThreadRoot(null);
  }, [clearMarks]);

  // Abrir desde una notificación: primero la carpeta (que limpia el mensaje) y
  // después el mensaje. Ambos set* se agrupan en un solo render y el último gana.
  useEffect(
    () =>
      onOpenMessage(({ folderId, messageId: id }) => {
        selectView({ kind: "folder", folderId });
        if (id > 0) setMessageId(id);
      }),
    [selectView],
  );

  const vertical = pane !== "right";
  const view = !ready || valid ? chosen : null;
  const composeAccount = viewAccount(view, folders, accounts) ?? accounts[0];

  const state: ShellState = {
    view,
    messageId,
    selectView,
    selectMessage,
    search,
    setSearch,
    threadRoot,
    selectThread,
    contactId,
    selectContact: setContactId,
    openContact,
    marked,
    markedThreads,
    setMarked,
    setMarkedThreads,
    clearMarks,
    labelFilter,
    setLabelFilter,
  };

  useLabelKeys(state);
  const keys = useMailKeys(state, composeAccount?.id);

  // Los dos paneles del grupo interior se arman una vez; solo cambia el orden.
  const listPanel = (
    <ResizablePanel id="list" defaultSize={`${layout.inner.list}%`} minSize={vertical ? "20%" : "22%"}>
      {list ? list(state) : <Placeholder>Elegí una carpeta</Placeholder>}
    </ResizablePanel>
  );
  const readerPanel = (
    <ResizablePanel id="reader" defaultSize={`${layout.inner.reader}%`} minSize={vertical ? "20%" : "30%"}>
      {reader ? (
        reader(state)
      ) : (
        <Placeholder>
          <MailIcon className="mr-2 size-4" /> Elegí un correo para leerlo
        </Placeholder>
      )}
    </ResizablePanel>
  );

  return (
    <div className="h-screen bg-background text-foreground">
      {extras?.(state)}
      <ShortcutsDialog open={keys.helpOpen} onOpenChange={keys.setHelpOpen} />
      <ResizablePanelGroup
        orientation="horizontal"
        defaultLayout={layout.outer}
        onLayoutChanged={(sizes) => saveLayout(pane, "outer", sizes)}
      >
        <ResizablePanel id="sidebar" defaultSize={`${layout.outer.sidebar}%`} minSize="14%" className="bg-sidebar">
          <div className="flex h-full flex-col">
            <div className="p-2">
              <Button className="w-full" onClick={() => composer.openNew(composeAccount.id)}>
                <PencilIcon />
                Redactar
              </Button>
            </div>
            <div className="min-h-0 flex-1">
              {sidebar ? (
                sidebar(state)
              ) : (
                <div className="flex items-center gap-2.5 border-b p-3">
                  <Avatar className="size-8">
                    <AvatarFallback>{composeAccount.email.slice(0, 1).toUpperCase()}</AvatarFallback>
                  </Avatar>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{composeAccount.displayName || composeAccount.email}</p>
                    <p className="truncate text-xs text-muted-foreground">{composeAccount.email}</p>
                  </div>
                </div>
              )}
            </div>
          </div>
        </ResizablePanel>
        <ResizableHandle />
        <ResizablePanel id="main" defaultSize={`${layout.outer.main}%`} minSize="40%">
          {/* key = posición: al cambiarla el grupo se vuelve a montar con sus tamaños. */}
          <ResizablePanelGroup
            key={pane}
            orientation={vertical ? "vertical" : "horizontal"}
            defaultLayout={layout.inner}
            onLayoutChanged={(sizes) => saveLayout(pane, "inner", sizes)}
          >
            {pane === "top" ? (
              <>
                {readerPanel}
                <ResizableHandle />
                {listPanel}
              </>
            ) : (
              <>
                {listPanel}
                <ResizableHandle />
                {readerPanel}
              </>
            )}
          </ResizablePanelGroup>
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
}

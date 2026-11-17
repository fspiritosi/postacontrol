import { MailIcon, PanelLeftCloseIcon, PanelLeftOpenIcon, PencilIcon } from "lucide-react";
import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { usePanelRef } from "react-resizable-panels";
import type { Account, Folder } from "../../../bindings/postacontrol/core/model/models";
import { onOpenMessage, onOpenTask, syncFolder, syncUnified } from "@/lib/events";
import { useAllFolders, useSettings } from "@/lib/queries";
import { NO_MARKS, type Marks } from "@/lib/selection";
import { useReadingPane, type ReadingPane } from "@/lib/settings";
import { defaultView, loadView, saveView, viewAccount, viewIsValid, type View } from "@/lib/view";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { TooltipProvider } from "@/components/ui/tooltip";
import { RailButton } from "./RailButton";
import { useComposer } from "../compose/ComposerContext";
import { useLabelKeys } from "../labels/useLabelKeys";
import { ShortcutsDialog } from "./ShortcutsDialog";
import { CreateTaskDialog } from "../tasks/CreateTaskDialog";
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

// Mientras el sidebar está plegado no se guarda el layout exterior: así, al
// desplegarlo (también después de reiniciar) vuelve al ancho que tenía.
export function saveOuterLayout(pane: ReadingPane, sizes: Sizes, collapsed: boolean) {
  if (!collapsed) saveLayout(pane, "outer", sizes);
}

// Qué hacer cuando el panel del sidebar cambia de tamaño. La primera medición
// real (sin tamaño previo o desde 0 px, como al mostrar una ventana que arrancó
// oculta) puede venir de un layout rearmado desplegado: ahí manda lo guardado y
// se vuelve a aplicar. Después manda el panel (arrastrar el borde, teclado).
export function sidebarResizeAction(
  prevPixels: number | undefined,
  panelCollapsed: boolean,
  wantedCollapsed: boolean,
): "reapply" | "adopt" | "none" {
  if (panelCollapsed === wantedCollapsed) return "none";
  return prevPixels == null || prevPixels === 0 ? "reapply" : "adopt";
}

// Ancho al desplegar: el último guardado (nunca se guarda plegado), no el
// mínimo ni el que recuerde el panel.
export function expandedSidebarSize(pane: ReadingPane): string {
  return `${loadLayouts(pane).outer.sidebar}%`;
}

const SIDEBAR_COLLAPSED_KEY = "pc:sidebar-collapsed";

function loadSidebarCollapsed(): boolean {
  try {
    return localStorage.getItem(SIDEBAR_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

function saveSidebarCollapsed(collapsed: boolean) {
  try {
    localStorage.setItem(SIDEBAR_COLLAPSED_KEY, collapsed ? "1" : "0");
  } catch {
    // ignorar: recordar el sidebar plegado es opcional
  }
}

// Si el sidebar está plegado a la columna de íconos y cómo cambiarlo.
export type SidebarMode = { collapsed: boolean; toggle: () => void };

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
  // Tarea abierta en la vista de Tareas ("l:<id>" local, "t:<id>" de Taskapp).
  taskKey: string | null;
  selectTask: (key: string | null) => void;
  // Pasa a Tareas con esa tarea local abierta.
  openTask: (localId: number) => void;
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
  sidebar?: (s: ShellState, mode: SidebarMode) => ReactNode;
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
  // outbox, contacts y tasks no tienen nada que traer del servidor.
}

export function MailShell({ accounts, sidebar, list, reader, extras }: Props) {
  const [chosen, setChosen] = useState<View | null>(loadView);
  const [messageId, setMessageId] = useState<number | null>(null);
  const [search, setSearchState] = useState<SearchState | null>(null);
  const [threadRoot, setThreadRoot] = useState<string | null>(null);
  const [contactId, setContactId] = useState<number | null>(null);
  const [taskKey, setTaskKey] = useState<string | null>(null);
  const [marked, setMarked] = useState<Marks<number>>(NO_MARKS);
  const [markedThreads, setMarkedThreads] = useState<Marks<string>>(NO_MARKS);
  const [labelFilter, setLabelFilterState] = useState<number | null>(null);
  const { folders, pending } = useAllFolders(accounts);
  const settings = useSettings();
  const pane = useReadingPane();
  // Los tamaños arrancan según la posición; al cambiarla se leen los de la nueva.
  const layout = useMemo(() => loadLayouts(pane), [pane]);
  const composer = useComposer();
  // El flag guardado manda: el tamaño en porcentaje no alcanza para saber si
  // estaba plegado (cambia con el ancho de la ventana).
  const [collapsed, setCollapsedState] = useState(loadSidebarCollapsed);
  const collapsedRef = useRef(collapsed);
  const sidebarRef = usePanelRef();
  const setCollapsed = useCallback((c: boolean) => {
    saveSidebarCollapsed(c);
    setCollapsedState(c);
  }, []);
  const toggleSidebar = useCallback(() => {
    setCollapsedState((c) => {
      saveSidebarCollapsed(!c);
      return !c;
    });
  }, []);
  // El panel sigue al estado: al tocar el botón y al montar (si quedó plegado).
  // Si ya está como corresponde (se arrastró el borde) no hace nada.
  const applyToPanel = useCallback(
    (c: boolean) => {
      const panel = sidebarRef.current;
      if (!panel) return;
      if (c && !panel.isCollapsed()) panel.collapse();
      else if (!c && panel.isCollapsed()) panel.resize(expandedSidebarSize(pane));
    },
    [sidebarRef, pane],
  );
  useLayoutEffect(() => {
    collapsedRef.current = collapsed;
    applyToPanel(collapsed);
  }, [collapsed, applyToPanel]);
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
      setTaskKey(null);
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
    setTaskKey(null);
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

  const openTask = useCallback((localId: number) => {
    clearMarks();
    setChosen({ kind: "tasks" });
    setLabelFilterState(null);
    setMessageId(null);
    setSearchState(null);
    setThreadRoot(null);
    setTaskKey(`l:${localId}`);
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

  // Abrir desde el aviso de una tarea.
  useEffect(() => onOpenTask(({ taskId }) => openTask(taskId)), [openTask]);

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
    taskKey,
    selectTask: setTaskKey,
    openTask,
    marked,
    markedThreads,
    setMarked,
    setMarkedThreads,
    clearMarks,
    labelFilter,
    setLabelFilter,
  };

  useLabelKeys(state);
  const [taskFor, setTaskFor] = useState<number | null>(null);
  const keys = useMailKeys(state, composeAccount?.id, setTaskFor);

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
    <TooltipProvider delayDuration={300}>
      <div className="h-screen bg-background text-foreground">
        {extras?.(state)}
        <CreateTaskDialog messageId={taskFor} open={taskFor != null} onOpenChange={(o) => !o && setTaskFor(null)} />
        <ShortcutsDialog open={keys.helpOpen} onOpenChange={keys.setHelpOpen} />
        <ResizablePanelGroup
          orientation="horizontal"
          defaultLayout={layout.outer}
          // Se pregunta al panel: al plegar con el teclado este aviso llega antes que onResize.
          onLayoutChanged={(sizes) =>
            saveOuterLayout(pane, sizes, sidebarRef.current?.isCollapsed() ?? collapsedRef.current)
          }
        >
          <ResizablePanel
            id="sidebar"
            defaultSize={`${layout.outer.sidebar}%`}
            minSize="14%"
            collapsible
            collapsedSize="52px"
            panelRef={sidebarRef}
            // Plegarlo o desplegarlo arrastrando el borde también cambia el modo.
            onResize={(_size, _id, prev) => {
              const panel = sidebarRef.current;
              if (!panel) return;
              const c = panel.isCollapsed();
              const action = sidebarResizeAction(prev?.inPixels, c, collapsedRef.current);
              if (action === "reapply") applyToPanel(collapsedRef.current);
              else if (action === "adopt") {
                collapsedRef.current = c;
                setCollapsed(c);
              }
            }}
            className="bg-sidebar"
          >
            <div className="flex h-full flex-col">
              {collapsed ? (
                <div className="flex flex-col items-center gap-1 p-2">
                  <RailButton label="Redactar" onClick={() => composer.openNew(composeAccount.id)}>
                    <PencilIcon />
                  </RailButton>
                  <RailButton label="Expandir sidebar" onClick={toggleSidebar}>
                    <PanelLeftOpenIcon />
                  </RailButton>
                </div>
              ) : (
                <div className="flex gap-1 p-2">
                  <Button className="flex-1" onClick={() => composer.openNew(composeAccount.id)}>
                    <PencilIcon />
                    Redactar
                  </Button>
                  <Button variant="ghost" size="icon" aria-label="Plegar sidebar" title="Plegar sidebar" onClick={toggleSidebar}>
                    <PanelLeftCloseIcon />
                  </Button>
                </div>
              )}
              <div className="min-h-0 flex-1">
                {sidebar ? (
                  sidebar(state, { collapsed, toggle: toggleSidebar })
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
    </TooltipProvider>
  );
}

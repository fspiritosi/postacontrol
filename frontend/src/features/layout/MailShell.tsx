import { MailIcon, PencilIcon } from "lucide-react";
import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import type { Account, Folder } from "../../../bindings/postacontrol/core/model/models";
import { syncFolder, syncUnified } from "@/lib/events";
import { useAllFolders, useSettings } from "@/lib/queries";
import { defaultView, loadView, saveView, viewAccount, viewIsValid, type View } from "@/lib/view";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { useComposer } from "../compose/ComposerContext";
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable";

const LAYOUT_KEY = "pc:layout";
const DEFAULT_LAYOUT = { sidebar: 18, list: 30, reader: 52 };

function loadLayout(): Record<string, number> {
  try {
    const raw = localStorage.getItem(LAYOUT_KEY);
    if (raw) {
      const l = JSON.parse(raw) as Record<string, number>;
      if (["sidebar", "list", "reader"].every((k) => typeof l[k] === "number")) return l;
    }
  } catch {
    // sin almacenamiento o JSON inválido: se usan los tamaños por defecto
  }
  return DEFAULT_LAYOUT;
}

function saveLayout(layout: Record<string, number>) {
  try {
    localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
  } catch {
    // ignorar: guardar el layout es opcional
  }
}

export type ShellState = {
  view: View | null;
  messageId: number | null;
  // Cambia la vista (que se guarda y se sincroniza) y limpia el mensaje abierto.
  selectView: (v: View) => void;
  selectMessage: (id: number | null) => void;
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
}

export function MailShell({ accounts, sidebar, list, reader, extras }: Props) {
  const [chosen, setChosen] = useState<View | null>(loadView);
  const [messageId, setMessageId] = useState<number | null>(null);
  const { folders, pending } = useAllFolders(accounts);
  const settings = useSettings();
  const [layout] = useState(loadLayout);
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

  const selectView = useCallback((v: View) => {
    setChosen(v);
    setMessageId(null);
  }, []);

  const view = !ready || valid ? chosen : null;
  const composeAccount = viewAccount(view, folders, accounts) ?? accounts[0];

  const state: ShellState = {
    view,
    messageId,
    selectView,
    selectMessage: setMessageId,
  };

  return (
    <div className="h-screen bg-background text-foreground">
      {extras?.(state)}
      <ResizablePanelGroup
        orientation="horizontal"
        defaultLayout={layout}
        onLayoutChanged={saveLayout}
      >
        <ResizablePanel id="sidebar" defaultSize={`${layout.sidebar}%`} minSize="14%" className="bg-sidebar">
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
        <ResizablePanel id="list" defaultSize={`${layout.list}%`} minSize="22%">
          {list ? list(state) : <Placeholder>Elegí una carpeta</Placeholder>}
        </ResizablePanel>
        <ResizableHandle />
        <ResizablePanel id="reader" defaultSize={`${layout.reader}%`} minSize="30%">
          {reader ? (
            reader(state)
          ) : (
            <Placeholder>
              <MailIcon className="mr-2 size-4" /> Elegí un correo para leerlo
            </Placeholder>
          )}
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
}

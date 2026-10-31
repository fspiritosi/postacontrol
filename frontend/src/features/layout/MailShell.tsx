import { MailIcon, PencilIcon } from "lucide-react";
import { useState, type ReactNode } from "react";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
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
  accountId: number;
  folderId: number | null;
  messageId: number | null;
  // La Bandeja de salida está abierta en lugar de la carpeta folderId.
  outbox: boolean;
  selectFolder: (id: number | null) => void;
  selectMessage: (id: number | null) => void;
  openOutbox: () => void;
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

export function MailShell({ accounts, sidebar, list, reader, extras }: Props) {
  const [accountId] = useState<number>(accounts[0].id);
  const [folderId, setFolderId] = useState<number | null>(null);
  const [messageId, setMessageId] = useState<number | null>(null);
  const [outbox, setOutbox] = useState(false);
  const account = accounts.find((a) => a.id === accountId) ?? accounts[0];
  const [layout] = useState(loadLayout);
  const composer = useComposer();

  const state: ShellState = {
    accountId: account.id,
    folderId,
    messageId,
    outbox,
    selectFolder: (id) => {
      setOutbox(false);
      setFolderId(id);
      setMessageId(null);
    },
    selectMessage: setMessageId,
    openOutbox: () => {
      setOutbox(true);
      setMessageId(null);
    },
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
              <Button className="w-full" onClick={() => composer.openNew(account.id)}>
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
                    <AvatarFallback>{account.email.slice(0, 1).toUpperCase()}</AvatarFallback>
                  </Avatar>
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{account.displayName || account.email}</p>
                    <p className="truncate text-xs text-muted-foreground">{account.email}</p>
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

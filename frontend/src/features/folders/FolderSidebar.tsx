import { ClockArrowUpIcon, InboxIcon, PlusIcon, SettingsIcon, StarIcon, UsersIcon } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { onPasswordRequest, onSettingsRequest } from "@/lib/events";
import { useAllFolders, useDrafts, useSettings } from "@/lib/queries";
import { cn } from "@/lib/utils";
import { UNIFIED_LABEL, type UnifiedRole } from "@/lib/view";
import { AccountSetupDialog } from "../accounts/AccountSetupDialog";
import { PasswordDialog } from "../accounts/PasswordDialog";
import type { ShellState } from "../layout/MailShell";
import { SettingsDialog } from "../settings/SettingsDialog";
import { AccountSection, ROLE_ICON } from "./AccountSection";

const COLLAPSED_KEY = "pc:collapsed";
const EXPANDED_KEY = "pc:expanded";

function loadIds(key: string): number[] {
  try {
    const raw = localStorage.getItem(key);
    if (raw) {
      const v: unknown = JSON.parse(raw);
      if (Array.isArray(v)) return v.filter((x): x is number => typeof x === "number");
    }
  } catch {
    // sin almacenamiento o JSON inválido: sin estado guardado
  }
  return [];
}

function saveIds(key: string, ids: number[]) {
  try {
    localStorage.setItem(key, JSON.stringify(ids));
  } catch {
    // ignorar: recordar las secciones es opcional
  }
}

const UNIFIED_ROLES: UnifiedRole[] = ["inbox", "flagged", "sent", "drafts", "trash"];

// Solo aparece si hay correos pendientes; en rojo si alguno falló.
function OutboxRow({ state }: { state: ShellState }) {
  const drafts = useDrafts();
  const list = drafts.data ?? [];
  if (list.length === 0) return null;
  const failed = list.some((d) => d.status === "failed");
  const active = state.view?.kind === "outbox";
  return (
    <button
      type="button"
      onClick={() => state.selectView({ kind: "outbox" })}
      aria-current={active ? "true" : undefined}
      className={cn(
        "flex w-full items-center gap-2 rounded-md py-1.5 pr-2 pl-2 text-left text-sm transition-colors hover:bg-sidebar-accent",
        active && "bg-sidebar-accent font-medium",
      )}
    >
      <span className="text-muted-foreground [&>svg]:size-4">
        <ClockArrowUpIcon />
      </span>
      <span className="min-w-0 flex-1 truncate">Bandeja de salida</span>
      <span
        className={cn(
          "text-xs font-medium tabular-nums",
          failed ? "text-destructive" : "text-muted-foreground",
        )}
      >
        {list.length}
      </span>
    </button>
  );
}


// La libreta de contactos: siempre visible, debajo de las cuentas.
function ContactsRow({ state }: { state: ShellState }) {
  const active = state.view?.kind === "contacts";
  return (
    <button
      type="button"
      onClick={() => state.selectView({ kind: "contacts" })}
      aria-current={active ? "true" : undefined}
      className={cn(
        "flex w-full items-center gap-2 rounded-md py-1.5 pr-2 pl-2 text-left text-sm transition-colors hover:bg-sidebar-accent",
        active && "bg-sidebar-accent font-medium",
      )}
    >
      <span className="text-muted-foreground [&>svg]:size-4">
        <UsersIcon />
      </span>
      <span className="min-w-0 flex-1 truncate">Contactos</span>
    </button>
  );
}

export function FolderSidebar({
  accounts,
  state,
}: {
  accounts: Account[];
  state: ShellState;
}) {
  const { folders } = useAllFolders(accounts);
  const settings = useSettings();
  const unified = settings.data?.unifiedInbox ?? false;
  const [collapsedIds, setCollapsedIds] = useState(() => loadIds(COLLAPSED_KEY));
  const [expandedIds, setExpandedIds] = useState(() => loadIds(EXPANDED_KEY));
  const [addOpen, setAddOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsSection, setSettingsSection] = useState<"labels" | undefined>(undefined);
  useEffect(
    () =>
      onSettingsRequest((section) => {
        setSettingsSection(section);
        setSettingsOpen(true);
      }),
    [],
  );
  // Dueño único del diálogo de contraseña: lo abren el menú y el toast de error.
  const [passwordAccountId, setPasswordAccountId] = useState<number | null>(null);
  useEffect(() => onPasswordRequest(setPasswordAccountId), []);
  const passwordAccount = accounts.find((a) => a.id === passwordAccountId) ?? null;

  // El estado guardado manda; sin él, con la unificada activa arrancan plegadas.
  const isOpen = (id: number) => {
    if (collapsedIds.includes(id)) return false;
    if (expandedIds.includes(id)) return true;
    return !unified;
  };
  const setOpen = (id: number, open: boolean) => {
    const collapsed = collapsedIds.filter((x) => x !== id).concat(open ? [] : [id]);
    const expanded = expandedIds.filter((x) => x !== id).concat(open ? [id] : []);
    setCollapsedIds(collapsed);
    setExpandedIds(expanded);
    saveIds(COLLAPSED_KEY, collapsed);
    saveIds(EXPANDED_KEY, expanded);
  };

  const unifiedRows = UNIFIED_ROLES.filter(
    (r) => r === "flagged" || folders.some((f) => f.role === r),
  );
  const inboxUnread = folders.filter((f) => f.role === "inbox").reduce((n, f) => n + f.unread, 0);
  const unifiedIcon = (r: UnifiedRole): ReactNode =>
    r === "flagged" ? <StarIcon /> : (ROLE_ICON[r] ?? <InboxIcon />);

  return (
    <div className="flex h-full flex-col">
      <nav className="flex-1 space-y-0.5 overflow-y-auto p-2" aria-label="Carpetas">
        <OutboxRow state={state} />
        {unified && (
          <div className="mb-1 space-y-0.5">
            <p className="px-2 pt-1 text-xs font-medium text-muted-foreground">Todas las cuentas</p>
            {unifiedRows.map((r) => {
              const active = state.view?.kind === "unified" && state.view.role === r;
              return (
                <button
                  key={r}
                  type="button"
                  onClick={() => state.selectView({ kind: "unified", role: r })}
                  aria-current={active ? "true" : undefined}
                  className={cn(
                    "flex w-full items-center gap-2 rounded-md py-1.5 pr-2 pl-2 text-left text-sm transition-colors hover:bg-sidebar-accent",
                    active && "bg-sidebar-accent font-medium",
                  )}
                >
                  <span className="text-muted-foreground [&>svg]:size-4">{unifiedIcon(r)}</span>
                  <span className="min-w-0 flex-1 truncate">{UNIFIED_LABEL[r]}</span>
                  {r === "inbox" && inboxUnread > 0 && (
                    <span className="text-xs font-medium tabular-nums text-muted-foreground">
                      {inboxUnread}
                    </span>
                  )}
                </button>
              );
            })}
          </div>
        )}
        {accounts.map((a) => (
          <AccountSection
            key={a.id}
            account={a}
            state={state}
            open={isOpen(a.id)}
            onOpenChange={(o) => setOpen(a.id, o)}
            onChangePassword={(acc) => setPasswordAccountId(acc.id)}
          />
        ))}
        <ContactsRow state={state} />
      </nav>
      <div className="space-y-0.5 border-t p-2">
        <Button variant="ghost" className="w-full justify-start" onClick={() => setAddOpen(true)}>
          <PlusIcon /> Agregar cuenta
        </Button>
        <Button
          variant="ghost"
          className="w-full justify-start"
          onClick={() => {
            setSettingsSection(undefined);
            setSettingsOpen(true);
          }}
        >
          <SettingsIcon /> Configuración
        </Button>
      </div>
      <AccountSetupDialog open={addOpen} onOpenChange={setAddOpen} />
      <PasswordDialog
        key={passwordAccountId ?? "none"}
        account={passwordAccount}
        open={passwordAccount !== null}
        onOpenChange={(o) => !o && setPasswordAccountId(null)}
      />
      <SettingsDialog open={settingsOpen} onOpenChange={setSettingsOpen} focusSection={settingsSection} />
    </div>
  );
}

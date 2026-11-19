import { CalendarIcon, CheckSquareIcon, ClockArrowUpIcon, InboxIcon, SettingsIcon, StarIcon, UsersIcon } from "lucide-react";
import { useEffect, useRef, useState, type DragEvent, type ReactNode } from "react";
import type { Account } from "../../../bindings/postacontrol/core/model/models";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { dragAccount, hasMessageDrag } from "@/lib/dnd";
import { useDrafts } from "@/lib/queries";
import { cn } from "@/lib/utils";
import { UNIFIED_LABEL, type UnifiedRole, type View } from "@/lib/view";
import type { ShellState } from "../layout/MailShell";
import { RailButton } from "../layout/RailButton";
import { EXPAND_DELAY_MS, FolderTreeRows, ROLE_ICON, useFolderTree } from "./FolderTree";

const unifiedIcon = (r: UnifiedRole): ReactNode =>
  r === "flagged" ? <StarIcon /> : (ROLE_ICON[r] ?? <InboxIcon />);

// La burbuja de una cuenta en la columna plegada: su color, la inicial y los
// no leídos de la Entrada. Abre un popover con su árbol de carpetas, que
// también se abre quedándose encima mientras se arrastran correos de la cuenta.
function RailAccount({ account, state }: { account: Account; state: ShellState }) {
  const [open, setOpen] = useState(false);
  const { tree, list, actions, dialogs } = useFolderTree(account, state);
  const name = account.displayName || account.email;
  const unread = list.filter((f) => f.role === "inbox").reduce((n, f) => n + f.unread, 0);
  const v = state.view;
  const active = v?.kind === "folder" && list.some((f) => f.id === v.folderId);

  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const disarm = () => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  };
  useEffect(() => {
    const t = timer;
    return () => {
      if (t.current) clearTimeout(t.current);
    };
  }, []);
  const onDragOver = (e: DragEvent) => {
    if (open || timer.current) return;
    const dt = e.dataTransfer;
    if (!hasMessageDrag(dt) || dragAccount(dt) !== account.id) return;
    timer.current = setTimeout(() => {
      timer.current = null;
      setOpen(true);
    }, EXPAND_DELAY_MS);
  };

  // Elegir una carpeta del popover también lo cierra.
  const inPopover: ShellState = {
    ...state,
    selectView: (view: View) => {
      state.selectView(view);
      setOpen(false);
    },
  };

  const label = `${name} · ${account.email}`;
  return (
    <>
      <Popover open={open} onOpenChange={setOpen} modal={false}>
        <Tooltip>
          <TooltipTrigger asChild>
            <PopoverTrigger asChild>
              <button
                type="button"
                aria-label={label}
                aria-current={active ? "true" : undefined}
                onDragOver={onDragOver}
                onDragLeave={disarm}
                onDrop={disarm}
                className={cn(
                  "relative flex size-8 items-center justify-center rounded-full text-sm font-semibold text-white ring-offset-2 ring-offset-sidebar",
                  active && "ring-2 ring-ring",
                )}
                style={{ backgroundColor: account.color || "var(--muted-foreground)" }}
              >
                {name.slice(0, 1).toUpperCase()}
                {unread > 0 && (
                  <span className="absolute -top-1 -right-1.5 min-w-4 rounded-full bg-primary px-1 text-[10px] leading-4 font-medium text-primary-foreground tabular-nums">
                    {unread > 99 ? "99+" : unread}
                  </span>
                )}
              </button>
            </PopoverTrigger>
          </TooltipTrigger>
          <TooltipContent side="right">{label}</TooltipContent>
        </Tooltip>
        <PopoverContent
          side="right"
          align="start"
          className="max-h-[70vh] w-64 gap-1 overflow-y-auto p-2"
          // Al soltar correos en una carpeta, el popover ya cumplió.
          onDrop={() => setOpen(false)}
        >
          <p className="truncate px-2 pt-1 pb-1 text-xs font-medium text-muted-foreground">{name}</p>
          <div className="space-y-0.5">
            <FolderTreeRows tree={tree} state={inPopover} accountName={name} actions={actions} />
          </div>
        </PopoverContent>
      </Popover>
      {/* Afuera del popover: siguen abiertos aunque él se cierre. */}
      {dialogs}
    </>
  );
}

function RailOutbox({ state }: { state: ShellState }) {
  const drafts = useDrafts();
  const list = drafts.data ?? [];
  if (list.length === 0) return null;
  const failed = list.some((d) => d.status === "failed");
  return (
    <RailButton
      label={`Bandeja de salida (${list.length})`}
      active={state.view?.kind === "outbox"}
      onClick={() => state.selectView({ kind: "outbox" })}
    >
      <ClockArrowUpIcon className={failed ? "text-destructive" : undefined} />
    </RailButton>
  );
}

// La columna de íconos del sidebar plegado.
export function SidebarRail({
  accounts,
  state,
  unifiedRows,
  onOpenSettings,
}: {
  accounts: Account[];
  state: ShellState;
  // Las vistas de "Todas las cuentas"; vacío si la bandeja unificada está apagada.
  unifiedRows: UnifiedRole[];
  onOpenSettings: () => void;
}) {
  const v = state.view;
  return (
    <div className="flex h-full flex-col items-center">
      <nav className="flex min-h-0 flex-1 flex-col items-center gap-1 overflow-y-auto px-2 py-1" aria-label="Carpetas">
        {unifiedRows.length > 0 && (
          <>
            {unifiedRows.map((r) => (
              <RailButton
                key={r}
                label={`Todas las cuentas · ${UNIFIED_LABEL[r]}`}
                active={v?.kind === "unified" && v.role === r}
                onClick={() => state.selectView({ kind: "unified", role: r })}
              >
                {unifiedIcon(r)}
              </RailButton>
            ))}
            <Separator className="my-1 w-6" />
          </>
        )}
        <div className="flex flex-col items-center gap-2 py-1">
          {accounts.map((a) => (
            <RailAccount key={a.id} account={a} state={state} />
          ))}
        </div>
        <Separator className="my-1 w-6" />
        <RailOutbox state={state} />
        <RailButton
          label="Contactos"
          active={v?.kind === "contacts"}
          onClick={() => state.selectView({ kind: "contacts" })}
        >
          <UsersIcon />
        </RailButton>
        <RailButton
          label="Tareas"
          active={v?.kind === "tasks"}
          onClick={() => state.selectView({ kind: "tasks" })}
        >
          <CheckSquareIcon />
        </RailButton>
        <RailButton
          label="Calendario"
          active={v?.kind === "calendar"}
          onClick={() => state.selectView({ kind: "calendar" })}
        >
          <CalendarIcon />
        </RailButton>
      </nav>
      <div className="border-t p-2">
        <RailButton label="Configuración" onClick={onOpenSettings}>
          <SettingsIcon />
        </RailButton>
      </div>
    </div>
  );
}

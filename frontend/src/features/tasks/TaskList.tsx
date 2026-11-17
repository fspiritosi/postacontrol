import { useQueryClient } from "@tanstack/react-query";
import { CheckCircle2Icon, CircleIcon, PlusIcon } from "lucide-react";
import { useMemo, useState } from "react";
import type { LocalTask } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { errorMessage } from "@/lib/errors";
import { requestSettings } from "@/lib/events";
import { useLocalTasks, useTaskappStatus, useTaskappTasks } from "@/lib/queries";
import { groupByDue, toItems, type TaskItem, type TaskSource } from "@/lib/tasks";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";
import { LocalTaskDialog } from "./LocalTaskDialog";
import { setTaskDone } from "./actions";

const SOURCE_KEY = "pc:tasks-source";
const SOURCES: { value: TaskSource; label: string }[] = [
  { value: "all", label: "Todas" },
  { value: "taskapp", label: "Taskapp" },
  { value: "local", label: "Locales" },
];

function loadSource(): TaskSource {
  try {
    const v = localStorage.getItem(SOURCE_KEY);
    if (v === "all" || v === "taskapp" || v === "local") return v;
  } catch {
    // sin almacenamiento: se muestran todas
  }
  return "all";
}

function saveSource(s: TaskSource) {
  try {
    localStorage.setItem(SOURCE_KEY, s);
  } catch {
    // ignorar: recordar el filtro es opcional
  }
}

// "hace 10 min" para la última vez que se pudo leer Taskapp.
function ago(iso: string, now: Date): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return "";
  const min = Math.max(0, Math.round((now.getTime() - t) / 60_000));
  if (min < 1) return "hace un momento";
  if (min < 60) return `hace ${min} min`;
  if (min < 24 * 60) return `hace ${Math.round(min / 60)} h`;
  return `hace ${Math.round(min / (24 * 60))} d`;
}

export function TaskList({ state }: { state: ShellState }) {
  const [source, setSourceState] = useState<TaskSource>(loadSource);
  const [showDone, setShowDone] = useState(false);
  const [newOpen, setNewOpen] = useState(false);
  const status = useTaskappStatus();
  const connected = status.data?.connected ?? false;
  const local = useLocalTasks(showDone);
  // Sin conexión (o sin saberlo todavía) no se piden las remotas.
  const remote = useTaskappTasks(connected && source !== "local");

  const setSource = (s: TaskSource) => {
    setSourceState(s);
    saveSource(s);
  };

  const all = useMemo(() => local.data ?? [], [local.data]);
  const remoteData = remote.data;
  const groups = useMemo(() => {
    const pending = all.filter((t) => !t.doneAt);
    const items = toItems(pending, connected ? (remoteData?.tasks ?? []) : [], source);
    return groupByDue(items, new Date());
  }, [all, remoteData, connected, source]);
  const done = useMemo(
    () =>
      showDone && source !== "taskapp"
        ? all.filter((t) => t.doneAt).sort((a, b) => (b.doneAt ?? "").localeCompare(a.doneAt ?? ""))
        : [],
    [all, showDone, source],
  );

  const showRemoteNotices = source !== "local";
  const notConnected = status.isSuccess && !connected && showRemoteNotices;
  const stale = connected && showRemoteNotices && remoteData?.stale;
  const remoteError =
    connected && showRemoteNotices
      ? remote.isError
        ? errorMessage(remote.error)
        : remoteData && !remoteData.stale && remoteData.error
          ? remoteData.error
          : ""
      : "";
  const empty = groups.length === 0 && done.length === 0;
  const loading = local.isPending;

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b p-2">
        <Button variant="outline" onClick={() => setNewOpen(true)}>
          <PlusIcon />
          Nueva
        </Button>
        <Select value={source} onValueChange={(v) => setSource(v as TaskSource)}>
          <SelectTrigger className="ml-auto w-32" aria-label="Origen de las tareas">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {SOURCES.map((s) => (
              <SelectItem key={s.value} value={s.value}>
                {s.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {notConnected && (
          <button
            type="button"
            onClick={() => requestSettings("taskapp")}
            className="mb-2 w-full rounded-md bg-muted px-2 py-1.5 text-left text-xs text-muted-foreground hover:bg-accent"
          >
            Conectá Taskapp para ver tus tareas
          </button>
        )}
        {stale && (
          <p className="mb-2 rounded-md bg-muted px-2 py-1.5 text-xs text-muted-foreground">
            {`Taskapp sin conexión · actualizado ${ago(remoteData?.fetchedAt ?? "", new Date())}`.trim()}
          </p>
        )}
        {remoteError && (
          <p role="alert" className="mb-2 rounded-md bg-destructive/10 px-2 py-1.5 text-xs text-destructive">
            {remoteError}
          </p>
        )}
        {local.isError && (
          <p role="alert" className="p-4 text-center text-sm text-destructive">
            {errorMessage(local.error)}
          </p>
        )}
        {!loading && !local.isError && empty && (
          <p className="p-4 text-center text-sm text-muted-foreground">No tenés tareas pendientes.</p>
        )}
        {groups.map((g) => (
          <section key={g.id} className="mb-3">
            <h3 className="px-2 pb-1 text-xs font-medium text-muted-foreground">{g.label}</h3>
            <ul className="space-y-0.5">
              {g.items.map((item) => (
                <TaskRow key={item.key} item={item} active={state.taskKey === item.key} state={state} />
              ))}
            </ul>
          </section>
        ))}
        {done.length > 0 && (
          <section className="mb-3">
            <h3 className="px-2 pb-1 text-xs font-medium text-muted-foreground">Hechas</h3>
            <ul className="space-y-0.5">
              {done.map((t) => (
                <LocalRow key={t.id} task={t} done active={state.taskKey === `l:${t.id}`} state={state} />
              ))}
            </ul>
          </section>
        )}
      </div>
      <div className="flex items-center gap-2 border-t p-2">
        <Switch id="tasks-show-done" checked={showDone} onCheckedChange={setShowDone} />
        <Label htmlFor="tasks-show-done" className="text-xs font-normal">
          Mostrar hechas
        </Label>
      </div>
      <LocalTaskDialog
        open={newOpen}
        onOpenChange={setNewOpen}
        task={null}
        onSaved={(saved) => state.selectTask(`l:${saved.id}`)}
      />
    </div>
  );
}

function TaskRow({ item, active, state }: { item: TaskItem; active: boolean; state: ShellState }) {
  if (item.source === "local") return <LocalRow task={item.task} active={active} state={state} />;
  const t = item.task;
  return (
    <li>
      <button
        type="button"
        onClick={() => state.selectTask(item.key)}
        aria-current={active ? "true" : undefined}
        className={cn(
          "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent",
          active && "bg-accent font-medium",
        )}
      >
        <span className="size-4 shrink-0" aria-hidden />
        <span className="min-w-0 flex-1 truncate">{t.title}</span>
        {t.statusName && (
          <span className="max-w-24 shrink-0 truncate text-xs text-muted-foreground">{t.statusName}</span>
        )}
        <span className="shrink-0 text-xs text-muted-foreground">
          {t.ticket ? `Ticket #${t.id}` : `#${t.id}`}
        </span>
        <span className="flex max-w-28 shrink-0 items-center gap-1 text-xs text-muted-foreground">
          <span
            className="size-2 shrink-0 rounded-full"
            style={{ backgroundColor: t.projectColor || undefined }}
            aria-hidden
          />
          <span className="truncate">{t.projectName}</span>
        </span>
      </button>
    </li>
  );
}

function LocalRow({
  task,
  active,
  state,
  done = false,
}: {
  task: LocalTask;
  active: boolean;
  state: ShellState;
  done?: boolean;
}) {
  const qc = useQueryClient();
  return (
    <li
      className={cn(
        "flex items-center gap-1 rounded-md pl-1 transition-colors hover:bg-accent",
        active && "bg-accent font-medium",
      )}
    >
      <button
        type="button"
        aria-label={`${done ? "Marcar como pendiente" : "Marcar como hecha"}: ${task.title}`}
        className="shrink-0 rounded-full p-1 text-muted-foreground hover:text-foreground"
        onClick={() => void setTaskDone(qc, task.id, !done)}
      >
        {done ? <CheckCircle2Icon className="size-4" /> : <CircleIcon className="size-4" />}
      </button>
      <button
        type="button"
        onClick={() => state.selectTask(`l:${task.id}`)}
        aria-current={active ? "true" : undefined}
        className="flex min-w-0 flex-1 items-center gap-2 py-1.5 pr-2 text-left text-sm"
      >
        <span className={cn("min-w-0 flex-1 truncate", done && "text-muted-foreground line-through")}>
          {task.title}
        </span>
        {task.dueTime && <span className="shrink-0 text-xs text-muted-foreground">{task.dueTime}</span>}
        <span className="shrink-0 text-xs text-muted-foreground">Local</span>
      </button>
    </li>
  );
}

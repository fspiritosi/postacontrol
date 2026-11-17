import { useQueryClient } from "@tanstack/react-query";
import { CheckIcon, ExternalLinkIcon, MailIcon, PencilIcon, RotateCcwIcon, Trash2Icon } from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { LocalTask, TaskappTask } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { errorMessage } from "@/lib/errors";
import { requestOpenMessage } from "@/lib/events";
import { useLocalTasks, useSettings, useTaskappTasks } from "@/lib/queries";
import { REMINDER_OPTIONS } from "@/lib/tasks";
import type { ShellState } from "../layout/MailShell";
import { ReaderBackdrop } from "../reader/ReaderBackdrop";
import { SafeHtmlFrame } from "../reader/SafeHtmlFrame";
import { LocalTaskDialog } from "./LocalTaskDialog";
import { setTaskDone } from "./actions";

const DEFAULT_MINUTES = 15;

// "AAAA-MM-DD" en hora local; new Date(texto) lo leería como UTC.
function formatDay(s: string): string {
  const m = /^(\d{4})-(\d{2})-(\d{2})$/.exec(s);
  if (!m) return "";
  return new Date(+m[1], +m[2] - 1, +m[3]).toLocaleDateString("es-AR", {
    weekday: "long", day: "numeric", month: "long", year: "numeric",
  });
}

function reminderLabel(minutes: number): string {
  return REMINDER_OPTIONS.find((o) => o.minutes === minutes)?.label ?? `${minutes} min antes`;
}

const PRIORITY: Record<string, string> = { low: "Baja", medium: "Media", high: "Alta", urgent: "Urgente" };

export function TaskPanel({ state }: { state: ShellState }) {
  const key = state.taskKey;
  const isRemote = key?.startsWith("t:") ?? false;
  const local = useLocalTasks(true);
  const remote = useTaskappTasks(isRemote);

  const empty = (
    <ReaderBackdrop>
      <p className="text-sm text-muted-foreground">Elegí una tarea para ver el detalle.</p>
    </ReaderBackdrop>
  );
  if (key == null) return empty;
  const id = Number(key.slice(2));
  if (isRemote) {
    const t = remote.data?.tasks?.find((x) => x.id === id);
    return t ? <RemoteDetail task={t} /> : empty;
  }
  const t = local.data?.find((x) => x.id === id);
  return t ? <LocalDetail key={t.id} task={t} state={state} /> : empty;
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div>
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      <dd className="select-text">{children}</dd>
    </div>
  );
}

function RemoteDetail({ task }: { task: TaskappTask }) {
  const open = () => svc.OpenTaskInTaskapp(task.url).catch((e) => toast.error(errorMessage(e)));
  return (
    <div className="h-full overflow-y-auto p-6">
      <div className="mx-auto max-w-xl space-y-5">
        <h2 className="text-xl font-semibold select-text">{task.title}</h2>
        <Button onClick={() => void open()}>
          <ExternalLinkIcon />
          Abrir en Taskapp
        </Button>
        <dl className="space-y-4 text-sm">
          <Row label="Proyecto">
            <span className="inline-flex items-center gap-1.5">
              <span
                className="size-2 rounded-full"
                style={{ backgroundColor: task.projectColor || undefined }}
                aria-hidden
              />
              {task.projectName}
            </span>
          </Row>
          {task.statusName && <Row label="Estado">{task.statusName}</Row>}
          {task.priority && <Row label="Prioridad">{PRIORITY[task.priority] ?? task.priority}</Row>}
          {task.dueDate && <Row label="Vencimiento">{formatDay(task.dueDate) || task.dueDate}</Row>}
        </dl>
        {task.description && (
          <div>
            <h3 className="text-xs font-medium text-muted-foreground">Descripción</h3>
            <SafeHtmlFrame html={task.description} text="" allowRemote={false} accountId={0} bare />
          </div>
        )}
      </div>
    </div>
  );
}

function LocalDetail({ task, state }: { task: LocalTask; state: ShellState }) {
  const qc = useQueryClient();
  const settings = useSettings();
  const [editOpen, setEditOpen] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);
  const done = task.doneAt != null;
  const hasTime = task.dueDate !== "" && task.dueTime !== "";
  const minutes = task.remindBefore ?? settings.data?.taskReminderMinutes ?? DEFAULT_MINUTES;

  const openSource = async () => {
    try {
      const m = await svc.FindMessageByHeader(task.sourceMessageId);
      // El mismo camino que al tocar una notificación: carpeta y correo.
      requestOpenMessage({ folderId: m.folderId, messageId: m.id });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const remove = async () => {
    setConfirmOpen(false);
    try {
      await svc.DeleteLocalTask(task.id);
      void qc.invalidateQueries({ queryKey: ["local-tasks"] });
      state.selectTask(null);
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  return (
    <div className="h-full overflow-y-auto p-6">
      <div className="mx-auto max-w-xl space-y-5">
        <h2 className="text-xl font-semibold select-text">{task.title}</h2>
        <div className="flex gap-2">
          <Button variant={done ? "outline" : "default"} onClick={() => void setTaskDone(qc, task.id, !done)}>
            {done ? <RotateCcwIcon /> : <CheckIcon />}
            {done ? "Reabrir" : "Hecha"}
          </Button>
          <Button variant="outline" onClick={() => setEditOpen(true)}>
            <PencilIcon />
            Editar
          </Button>
          <Button variant="outline" onClick={() => setConfirmOpen(true)}>
            <Trash2Icon />
            Borrar
          </Button>
        </div>
        <dl className="space-y-4 text-sm">
          {task.dueDate && (
            <Row label="Vence">
              {`${formatDay(task.dueDate) || task.dueDate}${task.dueTime ? ` · ${task.dueTime}` : ""}`}
            </Row>
          )}
          {hasTime && <Row label="Aviso">{reminderLabel(minutes)}</Row>}
          {task.notes && (
            <div>
              <dt className="text-xs font-medium text-muted-foreground">Notas</dt>
              <dd className="whitespace-pre-wrap select-text">{task.notes}</dd>
            </div>
          )}
          {task.sourceMessageId && (
            <div>
              <dt className="text-xs font-medium text-muted-foreground">Origen</dt>
              <dd>
                <button
                  type="button"
                  onClick={() => void openSource()}
                  className="inline-flex items-center gap-1.5 text-primary underline underline-offset-2"
                >
                  <MailIcon className="size-3.5" />
                  Correo de origen
                  {task.sourceSubject && <span className="text-muted-foreground no-underline">{`· ${task.sourceSubject}`}</span>}
                </button>
              </dd>
            </div>
          )}
        </dl>
      </div>
      <LocalTaskDialog open={editOpen} onOpenChange={setEditOpen} task={task} />
      <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{`¿Borrar «${task.title}»?`}</DialogTitle>
            <DialogDescription>La tarea se borra de PostaControl. El correo no se toca.</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmOpen(false)}>
              Cancelar
            </Button>
            <Button variant="destructive" onClick={() => void remove()}>
              Borrar tarea
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { LocalTask } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { errorMessage } from "@/lib/errors";
import { useSettings } from "@/lib/queries";
import { REMINDER_OPTIONS } from "@/lib/tasks";

const DEFAULT_REMINDER = "default";
// Si Configuración no fija un valor, el aviso es a los 15 minutos.
const FALLBACK_MINUTES = 15;

const NEW_TASK: LocalTask = {
  id: 0, title: "", notes: "", dueDate: "", dueTime: "", remindBefore: null,
  sourceMessageId: "", sourceSubject: "", doneAt: null, createdAt: "",
};

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // null = tarea nueva.
  task: LocalTask | null;
  onSaved?: (saved: LocalTask) => void;
};

export function LocalTaskDialog({ open, onOpenChange, task, onSaved }: Props) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {/* El formulario se monta de nuevo en cada apertura: parte siempre de la tarea. */}
      <DialogContent>
        <TaskForm key={task?.id ?? 0} task={task ?? NEW_TASK} onOpenChange={onOpenChange} onSaved={onSaved} />
      </DialogContent>
    </Dialog>
  );
}

function TaskForm({
  task,
  onOpenChange,
  onSaved,
}: {
  task: LocalTask;
  onOpenChange: (open: boolean) => void;
  onSaved?: (saved: LocalTask) => void;
}) {
  const qc = useQueryClient();
  const settings = useSettings();
  const [title, setTitle] = useState(task.title);
  const [notes, setNotes] = useState(task.notes);
  const [dueDate, setDueDate] = useState(task.dueDate);
  const [dueTime, setDueTime] = useState(task.dueTime);
  const [reminder, setReminder] = useState(
    task.remindBefore == null ? DEFAULT_REMINDER : String(task.remindBefore),
  );
  const [saving, setSaving] = useState(false);

  const defaultMinutes = settings.data?.taskReminderMinutes ?? FALLBACK_MINUTES;
  const hasDate = dueDate !== "";
  const hasTime = hasDate && dueTime !== "";

  const save = async () => {
    setSaving(true);
    try {
      const saved = await svc.SaveLocalTask({
        ...task,
        title: title.trim(),
        notes,
        dueDate,
        dueTime: hasDate ? dueTime : "",
        remindBefore: hasTime && reminder !== DEFAULT_REMINDER ? Number(reminder) : null,
      });
      void qc.invalidateQueries({ queryKey: ["local-tasks"] });
      onSaved?.(saved);
      onOpenChange(false);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        void save();
      }}
    >
      <DialogHeader>
        <DialogTitle>{task.id === 0 ? "Nueva tarea" : "Editar tarea"}</DialogTitle>
        <DialogDescription>Una tarea propia de PostaControl, con aviso en el escritorio.</DialogDescription>
      </DialogHeader>
      <div className="space-y-2">
        <Label htmlFor="task-title">Título</Label>
        <Input id="task-title" value={title} onChange={(e) => setTitle(e.target.value)} autoFocus />
      </div>
      <div className="space-y-2">
        <Label htmlFor="task-notes">Notas</Label>
        <Textarea id="task-notes" value={notes} onChange={(e) => setNotes(e.target.value)} />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label htmlFor="task-date">Fecha</Label>
          <Input id="task-date" type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} />
        </div>
        <div className="space-y-2">
          <Label htmlFor="task-time">Hora</Label>
          <Input
            id="task-time"
            type="time"
            value={hasDate ? dueTime : ""}
            disabled={!hasDate}
            onChange={(e) => setDueTime(e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-2">
        <Label htmlFor="task-reminder">Aviso</Label>
        <Select value={reminder} onValueChange={setReminder} disabled={!hasTime}>
          <SelectTrigger id="task-reminder" className="w-full" aria-label="Aviso">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={DEFAULT_REMINDER}>{`Predeterminado (${defaultMinutes} min)`}</SelectItem>
            {REMINDER_OPTIONS.map((o) => (
              <SelectItem key={o.minutes} value={String(o.minutes)}>
                {o.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Cancelar
        </Button>
        <Button type="submit" disabled={saving}>
          Guardar
        </Button>
      </DialogFooter>
    </form>
  );
}

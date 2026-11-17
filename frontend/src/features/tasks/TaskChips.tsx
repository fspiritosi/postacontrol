import { useQueryClient } from "@tanstack/react-query";
import { AlertTriangleIcon, ListPlusIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { TaskLink } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { errorMessage } from "@/lib/errors";
import { useTaskLinks } from "@/lib/queries";
import { CreateTaskDialog, retryAttachment, showTaskError } from "./CreateTaskDialog";

const label = (l: TaskLink) => `${l.ticket ? "Ticket" : "Tarea"} #${l.taskId}`;

// Las tareas de Taskapp del hilo de este correo, y sumar este correo a ellas.
export function TaskChips({ messageId }: { messageId: number }) {
  const qc = useQueryClient();
  const links = useTaskLinks(messageId).data ?? [];
  const [creating, setCreating] = useState(false);
  if (links.length === 0) return null;

  const add = async (l: TaskLink) => {
    try {
      await svc.AddMessageToTask(l.id, messageId);
      toast.success(`Se sumó el correo a ${label(l).toLowerCase()}.`);
      void qc.invalidateQueries({ queryKey: ["task-links"] });
    } catch (e) {
      showTaskError(e);
    }
  };
  const open = (l: TaskLink) => svc.OpenTaskInTaskapp(l.url).catch((e) => toast.error(errorMessage(e)));

  const pending = links.filter((l) => !l.alreadyAdded);
  const completed = pending.filter((l) => l.completed);
  const addable = pending.filter((l) => !l.completed);
  const single = links.length === 1;

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      {links.map((l) => {
        const text = l.projectName ? `${label(l)} · ${l.projectName}` : label(l);
        return (
          <span key={l.id} className="inline-flex items-center rounded border text-xs">
            <button
              type="button"
              className="rounded-l px-2 py-0.5 hover:bg-muted"
              onClick={() => void open(l)}
            >
              {text}
            </button>
            {l.attachPending && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    type="button"
                    aria-label="Adjunto pendiente"
                    title="No se pudo adjuntar el correo"
                    className="rounded-r px-1.5 py-0.5 text-amber-600 hover:bg-muted"
                  >
                    <AlertTriangleIcon className="size-3.5" />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start">
                  <DropdownMenuItem onSelect={() => void retryAttachment(qc, l.id)}>Reintentar adjunto</DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </span>
        );
      })}

      {single && pending.length === 1 && (
        <span title={pending[0].completed ? `La tarea #${pending[0].taskId} está completada` : undefined}>
          <Button
            size="xs"
            variant="outline"
            disabled={pending[0].completed}
            onClick={() => void add(pending[0])}
          >
            {`Sumar a la tarea #${pending[0].taskId}`}
          </Button>
        </span>
      )}
      {!single && addable.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="xs" variant="outline">
              Sumar a…
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            {addable.map((l) => (
              <DropdownMenuItem key={l.id} onSelect={() => void add(l)}>
                {l.projectName ? `${label(l)} · ${l.projectName}` : label(l)}
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {completed.length > 0 && (
        <Button size="xs" variant="ghost" onClick={() => setCreating(true)}>
          <ListPlusIcon /> Crear tarea nueva
        </Button>
      )}
      <CreateTaskDialog messageId={messageId} open={creating} onOpenChange={setCreating} />
    </div>
  );
}

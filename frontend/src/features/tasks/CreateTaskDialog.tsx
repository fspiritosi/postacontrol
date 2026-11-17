import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { TaskLink, TaskappDraft, TaskappProject } from "../../../bindings/postacontrol/core/model/models";
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
import { errorMessage, isOrphanError } from "@/lib/errors";
import { requestSettings } from "@/lib/events";
import { useTaskappStatus } from "@/lib/queries";
import { LocalTaskDialog } from "./LocalTaskDialog";

type Dest = "task" | "ticket" | "local";

const PRIORITIES = [
  ["none", "Ninguna"],
  ["low", "Baja"],
  ["medium", "Media"],
  ["high", "Alta"],
  ["urgent", "Urgente"],
] as const;

// Un error que dice que la tarea quedó creada a medias: el aviso no se va solo.
export function showTaskError(e: unknown) {
  const msg = errorMessage(e);
  if (isOrphanError(e)) toast.error(msg, { duration: Infinity });
  else toast.error(msg);
}

const openLink = (url: string) => svc.OpenTaskInTaskapp(url).catch((e) => toast.error(errorMessage(e)));

// Reintenta adjuntar el .eml de un vínculo y refresca los chips.
export async function retryAttachment(qc: ReturnType<typeof useQueryClient>, linkId: number) {
  try {
    await svc.RetryTaskLinkAttachment(linkId);
    toast.success("Se adjuntó el correo a la tarea.");
    void qc.invalidateQueries({ queryKey: ["task-links"] });
  } catch (e) {
    showTaskError(e);
  }
}

type Props = {
  messageId: number | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// Crear una tarea o ticket de Taskapp, o una tarea local, a partir de un correo.
export function CreateTaskDialog({ messageId, open, onOpenChange }: Props) {
  if (messageId == null) return null;
  // Se monta de nuevo en cada apertura: parte siempre del borrador.
  return open ? <Flow key={messageId} messageId={messageId} onOpenChange={onOpenChange} /> : null;
}

function Flow({ messageId, onOpenChange }: { messageId: number; onOpenChange: (o: boolean) => void }) {
  const status = useTaskappStatus();
  const [chosen, setChosen] = useState<Dest | null>(null);
  const connected = status.data?.connected === true;
  const dest = chosen ?? (connected ? "task" : null);

  const local = useQuery({
    queryKey: ["local-task-draft", messageId],
    queryFn: () => svc.CreateLocalTaskFrom(messageId),
    enabled: dest === "local",
    gcTime: 0,
    staleTime: Infinity,
  });

  if (dest === "local") {
    // El formulario local necesita la tarea ya cargada: arranca de ella.
    if (!local.data) return null;
    return <LocalTaskDialog open onOpenChange={onOpenChange} task={local.data} />;
  }
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Crear tarea</DialogTitle>
          <DialogDescription>Elegí dónde crear la tarea de este correo.</DialogDescription>
        </DialogHeader>
        {status.isPending ? (
          <div className="h-24" aria-busy />
        ) : (
          <>
            <fieldset className="space-y-1.5">
              <legend className="mb-1.5 text-sm font-medium">Destino</legend>
              {connected && <Radio value="task" label="Tarea en Taskapp" current={dest} onPick={setChosen} />}
              {connected && <Radio value="ticket" label="Ticket en Taskapp" current={dest} onPick={setChosen} />}
              <Radio value="local" label="Tarea local" current={dest} onPick={setChosen} />
              {!connected && (
                <Button type="button" variant="link" size="sm" className="px-0" onClick={() => requestSettings("taskapp")}>
                  Conectar Taskapp
                </Button>
              )}
            </fieldset>
            {connected && dest && (
              <TaskappForm messageId={messageId} ticket={dest === "ticket"} onOpenChange={onOpenChange} />
            )}
          </>
        )}
      </DialogContent>
    </Dialog>
  );
}

function Radio({
  value,
  label,
  current,
  onPick,
}: {
  value: Dest;
  label: string;
  current: Dest | null;
  onPick: (d: Dest) => void;
}) {
  return (
    <label className="flex items-center gap-2 text-sm">
      <input type="radio" name="task-dest" checked={current === value} onChange={() => onPick(value)} />
      {label}
    </label>
  );
}

function TaskappForm({
  messageId,
  ticket,
  onOpenChange,
}: {
  messageId: number;
  ticket: boolean;
  onOpenChange: (o: boolean) => void;
}) {
  const draft = useQuery({
    queryKey: ["taskapp-draft", messageId],
    queryFn: () => svc.TaskappDraftFor(messageId),
    gcTime: 0,
    staleTime: Infinity,
  });
  const projects = useQuery({
    queryKey: ["taskapp-projects"],
    queryFn: async () => (await svc.TaskappProjects()) ?? [],
  });
  if (draft.isError) return <p className="text-sm text-destructive">{errorMessage(draft.error)}</p>;
  if (!draft.data) return <div className="h-24" aria-busy />;
  return (
    <Fields
      messageId={messageId}
      ticket={ticket}
      draft={draft.data}
      projects={projects.data ?? []}
      onOpenChange={onOpenChange}
    />
  );
}

function Fields({
  messageId,
  ticket,
  draft,
  projects,
  onOpenChange,
}: {
  messageId: number;
  ticket: boolean;
  draft: TaskappDraft;
  projects: TaskappProject[];
  onOpenChange: (o: boolean) => void;
}) {
  const qc = useQueryClient();
  const [projectId, setProjectId] = useState(draft.projectId);
  const [title, setTitle] = useState(draft.title);
  const [html, setHtml] = useState(draft.descriptionHtml);
  const [priority, setPriority] = useState("none");
  const [dueDate, setDueDate] = useState("");

  const create = useMutation({
    mutationFn: () =>
      svc.CreateTaskappItem({
        messageId,
        ticket,
        projectId,
        title: title.trim(),
        descriptionHtml: html,
        priority,
        dueDate: ticket ? "" : dueDate,
      }),
    onSuccess: (link) => {
      void qc.invalidateQueries({ queryKey: ["task-links"] });
      onOpenChange(false);
      announce(qc, link);
    },
    onError: (e) => {
      showTaskError(e);
      // La tarea quedó creada en Taskapp: el diálogo no puede dejar crearla de nuevo.
      if (isOrphanError(e)) {
        void qc.invalidateQueries({ queryKey: ["task-links"] });
        onOpenChange(false);
      }
    },
  });

  const needsProject = !ticket && projectId === 0;
  const canCreate = !create.isPending && !needsProject && title.trim() !== "";

  return (
    <form
      className="space-y-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (canCreate) create.mutate();
      }}
    >
      <div className="space-y-2">
        <Label htmlFor="ct-project">Proyecto</Label>
        <Select
          value={projectId === 0 ? "" : String(projectId)}
          onValueChange={(v) => setProjectId(Number(v))}
        >
          <SelectTrigger id="ct-project" className="w-full" aria-label="Proyecto">
            <SelectValue placeholder="Elegí un proyecto" />
          </SelectTrigger>
          <SelectContent>
            {projects.map((p) => (
              <SelectItem key={p.id} value={String(p.id)}>
                <span className="inline-block size-2 rounded-full" style={{ backgroundColor: p.color }} aria-hidden />
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-2">
        <Label htmlFor="ct-title">Título</Label>
        <Input id="ct-title" value={title} onChange={(e) => setTitle(e.target.value)} />
      </div>
      <div className="space-y-2">
        <Label htmlFor="ct-desc">Descripción (HTML)</Label>
        <Textarea id="ct-desc" className="max-h-40 font-mono text-xs" value={html} onChange={(e) => setHtml(e.target.value)} />
      </div>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-2">
          <Label htmlFor="ct-priority">Prioridad</Label>
          <Select value={priority} onValueChange={setPriority}>
            <SelectTrigger id="ct-priority" className="w-full" aria-label="Prioridad">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {PRIORITIES.map(([v, l]) => (
                <SelectItem key={v} value={v}>
                  {l}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {!ticket && (
          <div className="space-y-2">
            <Label htmlFor="ct-due">Vence</Label>
            <Input id="ct-due" type="date" value={dueDate} onChange={(e) => setDueDate(e.target.value)} />
          </div>
        )}
      </div>
      <p className="text-xs text-muted-foreground">
        {draft.oversize
          ? "El correo pesa más de 20 MB: se adjunta sin sus archivos y la descripción los lista."
          : "📎 Se adjunta el correo original (.eml)"}
      </p>
      {needsProject && <p className="text-xs text-muted-foreground">Elegí un proyecto para la tarea.</p>}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
          Cancelar
        </Button>
        <Button type="submit" disabled={!canCreate}>
          Crear
        </Button>
      </DialogFooter>
    </form>
  );
}

function announce(qc: ReturnType<typeof useQueryClient>, link: TaskLink) {
  if (link.attachPending) {
    const what = link.ticket ? "El ticket" : "La tarea";
    toast.warning(`${what} #${link.taskId} se creó, pero no se pudo adjuntar el correo.`, {
      action: { label: "Reintentar", onClick: () => void retryAttachment(qc, link.id) },
    });
    return;
  }
  const where = link.projectName ? ` en ${link.projectName}` : " sin proyecto";
  const msg = link.ticket
    ? `Ticket #${link.taskId} creado${where}`
    : `Tarea #${link.taskId} creada${where}`;
  toast.success(msg, { action: { label: "Abrir", onClick: () => void openLink(link.url) } });
}

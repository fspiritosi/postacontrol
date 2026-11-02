import { useQueryClient } from "@tanstack/react-query";
import { ChevronDownIcon, ChevronUpIcon, Trash2Icon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Label } from "../../../bindings/postacontrol/core/model/models";
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
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { errorMessage } from "@/lib/errors";
import { LABEL_COLORS, nextColor } from "@/lib/labels";
import { useLabels } from "@/lib/queries";

function usageText(n: number): string {
  if (n === 0) return "Ningún correo la tiene todavía. Los correos no se borran.";
  return `Se saca de ${n === 1 ? "1 correo" : `${n} correos`}. Los correos no se borran.`;
}

type RowProps = {
  label: Label;
  index: number;
  count: number;
  onUpdate: (l: Label) => Promise<void>;
  onMove: (index: number, delta: -1 | 1) => void;
  onDelete: (l: Label) => void;
};

function LabelRow({ label, index, count, onUpdate, onMove, onDelete }: RowProps) {
  const [name, setName] = useState(label.name);
  const [colorOpen, setColorOpen] = useState(false);
  // El nombre del servidor manda cuando cambia (renombre confirmado o evento).
  const [seen, setSeen] = useState(label.name);
  if (seen !== label.name) {
    setSeen(label.name);
    setName(label.name);
  }

  // Escape cancela: el blur que sigue no debe confirmar el texto viejo del cierre.
  const cancelled = useRef(false);

  const commit = async () => {
    if (cancelled.current) {
      cancelled.current = false;
      return;
    }
    if (name === label.name) return;
    try {
      await onUpdate({ ...label, name });
    } catch {
      setName(label.name);
    }
  };

  return (
    <li className="flex items-center gap-2 text-sm">
      <span className="w-4 shrink-0 text-center text-xs text-muted-foreground tabular-nums">
        {index < 9 ? index + 1 : ""}
      </span>
      <Popover open={colorOpen} onOpenChange={setColorOpen}>
        <PopoverTrigger asChild>
          <button
            type="button"
            aria-label={`Color de ${label.name}`}
            className="size-5 shrink-0 rounded-full border border-border"
            style={{ backgroundColor: label.color }}
          />
        </PopoverTrigger>
        <PopoverContent className="grid w-auto grid-cols-6 gap-2 p-3">
          {LABEL_COLORS.map((c) => (
            <button
              key={c}
              type="button"
              aria-label={c}
              aria-pressed={c === label.color}
              className="size-6 rounded-full border border-border outline-offset-2 aria-pressed:outline-2 aria-pressed:outline-ring"
              style={{ backgroundColor: c }}
              onClick={() => {
                setColorOpen(false);
                if (c !== label.color) void onUpdate({ ...label, color: c }).catch(() => {});
              }}
            />
          ))}
        </PopoverContent>
      </Popover>
      <Input
        aria-label={`Nombre de ${label.name}`}
        value={name}
        onChange={(e) => setName(e.target.value)}
        onBlur={() => void commit()}
        onKeyDown={(e) => {
          if (e.key === "Enter") e.currentTarget.blur();
          else if (e.key === "Escape") {
            cancelled.current = true;
            setName(label.name);
            e.currentTarget.blur();
          }
        }}
        className="h-8 min-w-0 flex-1"
      />
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={`Subir ${label.name}`}
        disabled={index === 0}
        onClick={() => onMove(index, -1)}
      >
        <ChevronUpIcon />
      </Button>
      <Button
        size="icon-sm"
        variant="ghost"
        aria-label={`Bajar ${label.name}`}
        disabled={index === count - 1}
        onClick={() => onMove(index, 1)}
      >
        <ChevronDownIcon />
      </Button>
      <Button size="icon-sm" variant="ghost" aria-label={`Borrar ${label.name}`} onClick={() => onDelete(label)}>
        <Trash2Icon />
      </Button>
    </li>
  );
}

export function LabelsSection({ focus }: { focus?: boolean }) {
  const queryClient = useQueryClient();
  const labels = useLabels().data ?? [];
  const sectionRef = useRef<HTMLElement>(null);
  // null = sin fila de alta; string = nombre que se está tipeando.
  const [draft, setDraft] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<{ label: Label; usage: number } | null>(null);

  useEffect(() => {
    if (focus) sectionRef.current?.scrollIntoView({ block: "start" });
  }, [focus]);

  const refresh = () => queryClient.invalidateQueries({ queryKey: ["labels"] });

  // Avisa el error del núcleo y lo vuelve a lanzar para que la fila restaure su valor.
  const update = async (l: Label) => {
    try {
      await svc.UpdateLabel(l);
    } catch (e) {
      toast.error(errorMessage(e));
      throw e;
    } finally {
      void refresh();
    }
  };

  const create = async () => {
    const name = (draft ?? "").trim();
    setDraft(null);
    if (!name) return;
    try {
      await svc.CreateLabel(name, nextColor(labels.map((l) => l.color)));
      void refresh();
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const move = async (index: number, delta: -1 | 1) => {
    const ids = labels.map((l) => l.id);
    [ids[index], ids[index + delta]] = [ids[index + delta], ids[index]];
    try {
      await svc.ReorderLabels(ids);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void refresh();
    }
  };

  const askDelete = async (label: Label) => {
    try {
      setDeleting({ label, usage: await svc.LabelUsage(label.id) });
    } catch (e) {
      toast.error(errorMessage(e));
    }
  };

  const remove = async (label: Label) => {
    setDeleting(null);
    try {
      await svc.DeleteLabel(label.id);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      void refresh();
    }
  };

  return (
    <section id="settings-labels" ref={sectionRef} aria-labelledby="labels-title" className="space-y-3">
      <div className="flex items-center justify-between gap-4">
        <h3 id="labels-title" className="text-sm font-medium">
          Etiquetas
        </h3>
        <Button size="sm" variant="outline" onClick={() => setDraft("")}>
          Nueva etiqueta
        </Button>
      </div>
      {labels.length === 0 && draft === null ? (
        <p className="text-sm text-muted-foreground">Todavía no tenés etiquetas.</p>
      ) : (
        <ul className="space-y-1">
          {labels.map((l, i) => (
            <LabelRow
              key={l.id}
              label={l}
              index={i}
              count={labels.length}
              onUpdate={update}
              onMove={(idx, d) => void move(idx, d)}
              onDelete={(x) => void askDelete(x)}
            />
          ))}
          {draft !== null && (
            <li className="flex items-center gap-2 pl-6">
              <Input
                autoFocus
                aria-label="Nombre de la etiqueta nueva"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                onBlur={() => void create()}
                onKeyDown={(e) => {
                  if (e.key === "Enter") e.currentTarget.blur();
                  else if (e.key === "Escape") setDraft(null);
                }}
                className="h-8 flex-1"
              />
            </li>
          )}
        </ul>
      )}
      <Dialog open={deleting != null} onOpenChange={(o) => !o && setDeleting(null)}>
        <DialogContent className="sm:max-w-sm" showCloseButton={false}>
          <DialogHeader>
            <DialogTitle>¿Borrar la etiqueta «{deleting?.label.name}»?</DialogTitle>
            <DialogDescription>{usageText(deleting?.usage ?? 0)}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setDeleting(null)}>
              Cancelar
            </Button>
            <Button variant="destructive" onClick={() => deleting && void remove(deleting.label)}>
              Borrar
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </section>
  );
}

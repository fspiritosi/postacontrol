import { useQuery, useQueryClient } from "@tanstack/react-query";
import { DownloadIcon, FolderClockIcon, UploadIcon } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { Button } from "@/components/ui/button";
import { errorMessage } from "@/lib/errors";
import type { BackupImport } from "../../../bindings/postacontrol/core/model/models";
import { PageHeader } from "./SettingsPages";

const KINDS = [
  ["contacts", "contacto", "contactos"],
  ["signatures", "firma", "firmas"],
  ["labels", "etiqueta", "etiquetas"],
  ["rules", "regla", "reglas"],
  ["imageSenders", "remitente con imágenes permitidas", "remitentes con imágenes permitidas"],
] as const;

function count(n: number, one: string, many: string) {
  return `${n} ${n === 1 ? one : many}`;
}

// "a", "a y b", "a, b y c".
function joinList(parts: string[]) {
  return parts.length <= 1 ? parts.join("") : `${parts.slice(0, -1).join(", ")} y ${parts[parts.length - 1]}`;
}

function listOf(r: BackupImport, suffix: "Added" | "Skipped" | "Failed") {
  return KINDS.flatMap(([key, one, many]) => {
    const n = (r as unknown as Record<string, number | undefined>)[`${key}${suffix}`] ?? 0;
    return n > 0 ? [count(n, one, many)] : [];
  });
}

// El resumen de una importación, una oración por renglón.
export function describeImport(r: BackupImport): string[] {
  const added = listOf(r, "Added");
  const skipped = listOf(r, "Skipped");
  const failed = listOf(r, "Failed");
  const lines = [added.length > 0 ? `Se sumaron: ${joinList(added)}.` : "No había nada nuevo para sumar."];
  if (skipped.length > 0) lines.push(`Ya estaban y no se tocaron: ${joinList(skipped)}.`);
  if (failed.length > 0) {
    lines.push(
      `No se pudieron restaurar: ${joinList(failed)}. Falta la cuenta, la carpeta o la etiqueta de la que dependen, o ya se llegó al máximo.`,
    );
  }
  return lines;
}

// Exportar e importar lo que vive solo en esta PC.
export function BackupSection() {
  const [busy, setBusy] = useState(false);
  const [summary, setSummary] = useState<string[] | null>(null);

  const run = async (fn: () => Promise<void>) => {
    setBusy(true);
    try {
      await fn();
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const exportBackup = () =>
    run(async () => {
      if (await svc.ExportBackup()) toast.success("Respaldo guardado.");
    });

  const importBackup = () =>
    run(async () => {
      const res = await svc.ImportBackup();
      if (res) setSummary(describeImport(res));
    });

  return (
    <section aria-labelledby="page-backup" className="space-y-4">
      <PageHeader
        id="page-backup"
        title="Respaldo"
        description="Tus contactos, firmas, etiquetas y reglas viven solo en esta PC. Guardá una copia para no perderlos si cambiás de computadora."
      />
      <p className="text-sm text-muted-foreground">
        El correo no va en el respaldo porque está en el servidor, y las contraseñas tampoco. Al importar, se suma lo
        que falta y no se pisa ni se borra nada de lo que ya tenés. Las reglas se asocian a tus cuentas por la
        dirección de correo, así que conviene agregar las cuentas antes de importar.
      </p>
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" disabled={busy} onClick={() => void exportBackup()}>
          <DownloadIcon /> Exportar respaldo…
        </Button>
        <Button type="button" variant="outline" disabled={busy} onClick={() => void importBackup()}>
          <UploadIcon /> Importar respaldo…
        </Button>
      </div>
      <AutoBackup />
      {summary && (
        <div role="status" className="space-y-1 rounded-md border bg-muted/40 p-3 text-sm">
          {summary.map((line) => (
            <p key={line}>{line}</p>
          ))}
        </div>
      )}
    </section>
  );
}

// "2026-11-10" → "10/11/2026".
function shortDate(iso: string) {
  const [y, m, d] = iso.split("-");
  return `${d}/${m}/${y}`;
}

// La copia semanal en una carpeta (Drive, Nextcloud, un pendrive...).
function AutoBackup() {
  const queryClient = useQueryClient();
  const info = useQuery({ queryKey: ["autoBackup"], queryFn: () => svc.AutoBackupInfo() });
  const [busy, setBusy] = useState(false);
  const dir = info.data?.dir ?? "";

  const run = async (fn: () => Promise<unknown>, ok?: string) => {
    setBusy(true);
    try {
      await fn();
      if (ok) toast.success(ok);
    } catch (e) {
      toast.error(errorMessage(e));
    } finally {
      setBusy(false);
      void queryClient.invalidateQueries({ queryKey: ["autoBackup"] });
    }
  };

  return (
    <div className="space-y-2 border-t pt-4">
      <div className="space-y-1">
        <h3 className="text-sm font-medium">Respaldo automático</h3>
        <p className="text-sm text-muted-foreground">
          Una vez por semana se guarda una copia en la carpeta que elijas y se conservan las últimas 8. Conviene que
          sea una carpeta que se sincronice fuera de esta PC (Drive, Nextcloud) o un pendrive.
        </p>
      </div>
      {info.isPending ? null : dir ? (
        <div className="space-y-2">
          <div className="flex items-center gap-2 text-sm">
            <FolderClockIcon className="size-4 shrink-0 text-muted-foreground" />
            <span className="truncate font-mono text-xs">{dir}</span>
          </div>
          <p className="text-sm text-muted-foreground">
            {info.data?.lastDate ? `Última copia: ${shortDate(info.data.lastDate)}.` : "Todavía no hay copias en esa carpeta."}
          </p>
          <div className="flex flex-wrap gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={busy}
              onClick={() => void run(() => svc.RunAutoBackupNow(), "Copia guardada.")}
            >
              Hacer una copia ahora
            </Button>
            <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void run(() => svc.PickAutoBackupDir())}>
              Cambiar carpeta…
            </Button>
            <Button type="button" variant="ghost" size="sm" disabled={busy} onClick={() => void run(() => svc.ClearAutoBackupDir())}>
              Apagar
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex items-center gap-3">
          <p className="text-sm text-muted-foreground">Apagado.</p>
          <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => void run(() => svc.PickAutoBackupDir())}>
            Elegir carpeta…
          </Button>
        </div>
      )}
    </div>
  );
}

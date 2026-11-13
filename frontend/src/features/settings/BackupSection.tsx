import { DownloadIcon, UploadIcon } from "lucide-react";
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

import { useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { ICSCalendar } from "../../../bindings/postacontrol/core/model/models";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { errorMessage } from "@/lib/errors";
import { REMINDER_OPTIONS } from "@/lib/tasks";
import { PageHeader, SettingRow, useSaveSetting } from "./SettingsPages";

// Los calendarios que Google suma solos (feriados, cumpleaños) empiezan sin marcar.
const SKIPPED_BY_DEFAULT = /^\s*(feriados|holidays|cumpleaños)/i;

export function CalendarSection() {
  const { settings, save } = useSaveSetting();
  const [calendars, setCalendars] = useState<ICSCalendar[]>([]);
  const [checked, setChecked] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [result, setResult] = useState("");
  const [error, setError] = useState("");

  const pick = async () => {
    setBusy(true);
    setResult("");
    setError("");
    try {
      const paths = await svc.PickICSFiles();
      if (!paths?.length) return;
      const found = (await svc.PreviewICSImport(paths)) ?? [];
      setCalendars(found);
      setChecked(new Set(found.filter((c) => !SKIPPED_BY_DEFAULT.test(c.name)).map((c) => c.id)));
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  const toggle = (id: string, on: boolean) =>
    setChecked((prev) => {
      const next = new Set(prev);
      if (on) next.add(id);
      else next.delete(id);
      return next;
    });

  const run = async () => {
    setBusy(true);
    setResult("");
    setError("");
    try {
      const ids = calendars.filter((c) => checked.has(c.id)).map((c) => c.id);
      const r = await svc.ImportICS(ids);
      setResult(`Se importaron ${r.added} eventos (${r.updated} actualizados, ${r.skipped} ya estaban)`);
      setCalendars([]);
      setChecked(new Set());
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <section aria-labelledby="page-calendar" className="space-y-4">
      <PageHeader
        id="page-calendar"
        title="Calendario"
        description="Avisos de eventos e importación desde Google Calendar."
      />
      <div className="divide-y">
        <SettingRow
          htmlFor="event-reminder"
          label="Aviso de eventos por defecto"
          description="Cuánto antes del comienzo avisa un evento nuevo."
        >
          <Select
            value={String(settings.data?.eventReminderMinutes ?? 15)}
            disabled={settings.isPending}
            onValueChange={(v) => void save({ eventReminderMinutes: Number(v) })}
          >
            <SelectTrigger id="event-reminder" className="w-44">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {REMINDER_OPTIONS.map((o) => (
                <SelectItem key={o.minutes} value={String(o.minutes)}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </SettingRow>
      </div>
      <div className="space-y-3">
        <h3 className="text-sm font-medium">Importar .ics</h3>
        <p className="text-sm text-muted-foreground">
          En Google Calendar: Configuración → Importar y exportar → Exportar. Elegí el .zip que baja.
        </p>
        <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => void pick()}>
          Importar .ics
        </Button>
        {calendars.length > 0 ? (
          <div className="space-y-3">
            <ul className="space-y-2">
              {calendars.map((c) => (
                <li key={c.id} className="flex items-center gap-2 text-sm">
                  <input
                    id={`ics-${c.id}`}
                    type="checkbox"
                    className="size-4"
                    checked={checked.has(c.id)}
                    disabled={busy}
                    onChange={(e) => toggle(c.id, e.target.checked)}
                  />
                  <label htmlFor={`ics-${c.id}`}>{c.name}</label>
                  <span className="text-muted-foreground">{c.events} eventos</span>
                </li>
              ))}
            </ul>
            <Button type="button" size="sm" disabled={busy || checked.size === 0} onClick={() => void run()}>
              Importar {checked.size} calendarios
            </Button>
          </div>
        ) : null}
        {result ? <p role="status" className="text-sm">{result}</p> : null}
        {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
      </div>
    </section>
  );
}

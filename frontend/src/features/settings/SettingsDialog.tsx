import {
  ArchiveRestoreIcon,
  FilterIcon,
  type LucideIcon,
  MailOpenIcon,
  PenLineIcon,
  SlidersHorizontalIcon,
  TagIcon,
  UsersIcon,
} from "lucide-react";
import { useRef, useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import type { RuleDraft, SettingsSection } from "@/lib/events";
import { cn } from "@/lib/utils";
import { AccountsSection } from "./AccountsSection";
import { BackupSection } from "./BackupSection";
import { LabelsSection } from "./LabelsSection";
import { RulesSection } from "./RulesSection";
import { ComposePage, GeneralPage, ReadingPage } from "./SettingsPages";

const SECTIONS: { id: SettingsSection; label: string; icon: LucideIcon }[] = [
  { id: "general", label: "General", icon: SlidersHorizontalIcon },
  { id: "reading", label: "Lectura", icon: MailOpenIcon },
  { id: "compose", label: "Redacción", icon: PenLineIcon },
  { id: "accounts", label: "Cuentas", icon: UsersIcon },
  { id: "labels", label: "Etiquetas", icon: TagIcon },
  { id: "rules", label: "Reglas", icon: FilterIcon },
  { id: "backup", label: "Respaldo", icon: ArchiveRestoreIcon },
];

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // Sección a la que ir al abrir. Sin ella vuelve a la última que se vio.
  focusSection?: SettingsSection;
  // Regla nueva ya completada (desde un correo): abre su editor en Reglas.
  ruleDraft?: RuleDraft | null;
};

export function SettingsDialog({ open, onOpenChange, focusSection, ruleDraft }: Props) {
  const [section, setSection] = useState<SettingsSection>(focusSection ?? "general");
  // Al abrir con una sección pedida, se va a esa (ajuste de estado en el render).
  const [wasOpen, setWasOpen] = useState(open);
  if (open !== wasOpen) {
    setWasOpen(open);
    if (open && focusSection) setSection(focusSection);
  }
  // La regla pedida desde un correo se usa una sola vez: al cerrar su editor
  // se descarta, y volver a Reglas no lo reabre.
  const [draft, setDraft] = useState(ruleDraft ?? null);
  const [seenDraft, setSeenDraft] = useState(ruleDraft);
  if (ruleDraft !== seenDraft) {
    setSeenDraft(ruleDraft);
    setDraft(ruleDraft ?? null);
    if (ruleDraft) setSection("rules");
  }

  const navRef = useRef<HTMLElement>(null);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="flex h-[min(44rem,85vh)] gap-0 overflow-hidden p-0 sm:max-w-4xl"
        // Al abrir, el foco va a la sección activa y no a la primera de la lista.
        onOpenAutoFocus={(e) => {
          const current = navRef.current?.querySelector<HTMLElement>("[aria-current=page]");
          if (current) {
            e.preventDefault();
            current.focus();
          }
        }}
        // En un campo que usa Escape para cancelar (las etiquetas), Escape no cierra.
        onEscapeKeyDown={(e) => {
          if ((e.target as Element | null)?.closest?.("[data-escape-local]"))
            e.preventDefault();
        }}
      >
        <nav
          ref={navRef}
          aria-label="Secciones de Configuración"
          className="flex w-48 shrink-0 flex-col gap-0.5 border-r bg-muted/40 p-3"
        >
          <DialogTitle className="px-2 pt-1 pb-3 text-base">Configuración</DialogTitle>
          <DialogDescription className="sr-only">Ajustes de la aplicación.</DialogDescription>
          {SECTIONS.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              aria-current={section === id ? "page" : undefined}
              onClick={() => setSection(id)}
              className={cn(
                "flex items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none",
                section === id && "bg-accent font-medium text-foreground",
              )}
            >
              <Icon className="size-4 shrink-0" aria-hidden />
              {label}
            </button>
          ))}
        </nav>
        <div className="min-w-0 flex-1 overflow-y-auto p-6 pr-12">
          {section === "general" ? <GeneralPage /> : null}
          {section === "reading" ? <ReadingPage /> : null}
          {section === "compose" ? <ComposePage /> : null}
          {section === "accounts" ? <AccountsSection /> : null}
          {section === "labels" ? <LabelsSection /> : null}
          {section === "rules" ? <RulesSection draft={draft} onDraftDone={() => setDraft(null)} /> : null}
          {section === "backup" ? <BackupSection /> : null}
        </div>
      </DialogContent>
    </Dialog>
  );
}

import { useQueryClient } from "@tanstack/react-query";
import { PencilIcon, PlusIcon, SendIcon, Trash2Icon, XIcon } from "lucide-react";
import { useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Contact } from "../../../bindings/postacontrol/core/model/models";
import { Alert, AlertDescription } from "@/components/ui/alert";
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
import { Textarea } from "@/components/ui/textarea";
import { contactDisplayName } from "@/lib/contacts";
import { errorMessage } from "@/lib/errors";
import { useAccounts, useContact } from "@/lib/queries";
import type { ShellState } from "../layout/MailShell";
import { useComposer } from "../compose/ComposerContext";
import { ReaderBackdrop } from "../reader/ReaderBackdrop";

const NEW_CONTACT: Contact = { id: 0, name: "", emails: [], phones: [], company: "", notes: "" };

export function ContactPanel({ state }: { state: ShellState }) {
  const id = state.contactId;
  const contact = useContact(id);
  if (id == null) {
    return (
      <ReaderBackdrop>
        <p className="text-sm text-muted-foreground">Elegí un contacto o creá uno nuevo.</p>
      </ReaderBackdrop>
    );
  }
  if (id === 0) return <ContactCard key="new" contact={NEW_CONTACT} state={state} />;
  if (contact.isError) {
    return (
      <div className="p-4">
        <Alert variant="destructive">
          <AlertDescription>{errorMessage(contact.error)}</AlertDescription>
        </Alert>
      </div>
    );
  }
  if (!contact.data) return null;
  return <ContactCard key={id} contact={contact.data} state={state} />;
}

// Ficha y formulario son lo mismo: `editing` alterna entre leer y editar.
function ContactCard({ contact, state }: { contact: Contact; state: ShellState }) {
  const isNew = contact.id === 0;
  const qc = useQueryClient();
  const composer = useComposer();
  const accounts = useAccounts();
  const [editing, setEditing] = useState(isNew);
  const [draft, setDraft] = useState(contact);
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const [confirmOpen, setConfirmOpen] = useState(false);

  const emails = contact.emails ?? [];
  const phones = contact.phones ?? [];
  const dEmails = draft.emails ?? [];
  const dPhones = draft.phones ?? [];
  // Una ficha nueva arranca con un campo de correo vacío para escribir.
  const emailFields = dEmails.length > 0 ? dEmails : [""];
  const phoneFields = dPhones;

  const startEditing = () => {
    setDraft(contact);
    setError("");
    setEditing(true);
  };

  const cancel = () => {
    if (isNew) state.selectContact(null);
    else setEditing(false);
  };

  const save = async () => {
    setSaving(true);
    setError("");
    const clean = (l: string[]) => l.map((s) => s.trim()).filter((s) => s !== "");
    try {
      const saved = await svc.SaveContact({
        ...draft,
        emails: clean(emailFields),
        phones: clean(phoneFields),
      });
      qc.setQueryData(["contact", saved.id], saved);
      if (isNew) state.selectContact(saved.id);
      else setEditing(false);
    } catch (e) {
      setError(errorMessage(e));
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    setConfirmOpen(false);
    try {
      await svc.DeleteContact(contact.id);
      state.selectContact(null);
    } catch (e) {
      setError(errorMessage(e));
    }
  };

  const setList = (key: "emails" | "phones", list: string[]) => setDraft({ ...draft, [key]: list });
  const setAt = (list: string[], i: number, v: string) => list.map((x, j) => (j === i ? v : x));

  const errorAlert = error && (
    <Alert variant="destructive">
      <AlertDescription>{error}</AlertDescription>
    </Alert>
  );

  if (!editing) {
    return (
      <div className="h-full overflow-y-auto p-6">
        <div className="mx-auto max-w-xl space-y-5">
          <h2 className="text-xl font-semibold">{contactDisplayName(contact)}</h2>
          <div className="flex gap-2">
            {emails.length > 0 && (
              <Button
                onClick={() =>
                  composer.openNew((accounts.data ?? [])[0]?.id ?? 0, [
                    { name: contact.name, email: emails[0] },
                  ])
                }
                disabled={!accounts.data?.length}
              >
                <SendIcon />
                Escribirle
              </Button>
            )}
            <Button variant="outline" onClick={startEditing}>
              <PencilIcon />
              Editar
            </Button>
            <Button variant="outline" onClick={() => setConfirmOpen(true)}>
              <Trash2Icon />
              Eliminar
            </Button>
          </div>
          {errorAlert}
          <dl className="space-y-4 text-sm">
            <Field label="Correos" values={emails} />
            <Field label="Teléfonos" values={phones} />
            <Field label="Empresa" values={contact.company ? [contact.company] : []} />
            <Field label="Notas" values={contact.notes ? [contact.notes] : []} pre />
          </dl>
        </div>
        <Dialog open={confirmOpen} onOpenChange={setConfirmOpen}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{`¿Eliminar a «${contactDisplayName(contact)}»?`}</DialogTitle>
              <DialogDescription>Se borra de la libreta. Los correos no se tocan.</DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <Button variant="outline" onClick={() => setConfirmOpen(false)}>
                Cancelar
              </Button>
              <Button variant="destructive" onClick={() => void remove()}>
                Eliminar
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto p-6">
      <form
        className="mx-auto max-w-xl space-y-5"
        onSubmit={(e) => {
          e.preventDefault();
          void save();
        }}
      >
        <div className="space-y-2">
          <Label htmlFor="contact-name">Nombre</Label>
          <Input id="contact-name" value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} />
        </div>
        <div className="space-y-2">
          <Label asChild>
            <span>Correos</span>
          </Label>
          {emailFields.map((v, i) => (
            <div key={i} className="flex gap-2">
              <Input
                aria-label={`Correo ${i + 1}`}
                value={v}
                onChange={(e) => setList("emails", setAt(emailFields, i, e.target.value))}
              />
              {emailFields.length > 1 && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={`Quitar correo ${i + 1}`}
                  onClick={() => setList("emails", emailFields.filter((_, j) => j !== i))}
                >
                  <XIcon />
                </Button>
              )}
            </div>
          ))}
          <Button type="button" variant="outline" size="sm" onClick={() => setList("emails", [...emailFields, ""])}>
            <PlusIcon />
            Agregar correo
          </Button>
        </div>
        <div className="space-y-2">
          <Label asChild>
            <span>Teléfonos</span>
          </Label>
          {phoneFields.map((v, i) => (
            <div key={i} className="flex gap-2">
              <Input
                aria-label={`Teléfono ${i + 1}`}
                value={v}
                onChange={(e) => setList("phones", setAt(phoneFields, i, e.target.value))}
              />
              <Button
                type="button"
                variant="ghost"
                size="icon"
                aria-label={`Quitar teléfono ${i + 1}`}
                onClick={() => setList("phones", phoneFields.filter((_, j) => j !== i))}
              >
                <XIcon />
              </Button>
            </div>
          ))}
          <Button type="button" variant="outline" size="sm" onClick={() => setList("phones", [...phoneFields, ""])}>
            <PlusIcon />
            Agregar teléfono
          </Button>
        </div>
        <div className="space-y-2">
          <Label htmlFor="contact-company">Empresa</Label>
          <Input
            id="contact-company"
            value={draft.company}
            onChange={(e) => setDraft({ ...draft, company: e.target.value })}
          />
        </div>
        <div className="space-y-2">
          <Label htmlFor="contact-notes">Notas</Label>
          <Textarea
            id="contact-notes"
            value={draft.notes}
            onChange={(e) => setDraft({ ...draft, notes: e.target.value })}
          />
        </div>
        {errorAlert}
        <div className="flex gap-2">
          <Button type="submit" disabled={saving}>
            Guardar
          </Button>
          <Button type="button" variant="outline" onClick={cancel}>
            Cancelar
          </Button>
        </div>
      </form>
    </div>
  );
}

function Field({ label, values, pre }: { label: string; values: string[]; pre?: boolean }) {
  if (values.length === 0) return null;
  return (
    <div>
      <dt className="text-xs font-medium text-muted-foreground">{label}</dt>
      {values.map((v, i) => (
        <dd key={i} className={pre ? "whitespace-pre-wrap select-text" : "select-text"}>
          {v}
        </dd>
      ))}
    </div>
  );
}

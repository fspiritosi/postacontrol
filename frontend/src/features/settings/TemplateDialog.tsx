import { useRef, useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Template } from "../../../bindings/postacontrol/core/model/models";
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
import { errorMessage } from "@/lib/errors";
import { RichEditor } from "../compose/RichEditor";

type Props = {
  // null = plantilla nueva.
  template: Template | null;
  onClose: () => void;
};

// Se monta solo mientras está abierto, así el estado arranca limpio cada vez.
export function TemplateDialog({ template, onClose }: Props) {
  const [name, setName] = useState(template?.name ?? "");
  const [subject, setSubject] = useState(template?.subject ?? "");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const htmlRef = useRef<() => string>(() => template?.html ?? "");

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      await svc.SaveTemplate({ id: template?.id ?? 0, name, subject, html: htmlRef.current() });
      onClose();
    } catch (e) {
      setError(errorMessage(e));
      setSaving(false);
    }
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[85vh] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{template ? "Editar plantilla" : "Nueva plantilla"}</DialogTitle>
          <DialogDescription>
            En el redactor se inserta escribiendo «/» y su nombre. Si el correo no tiene asunto, toma el de la plantilla.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-2">
            <Label htmlFor="template-name">Nombre</Label>
            <Input id="template-name" value={name} onChange={(e) => setName(e.target.value)} />
          </div>
          <div className="space-y-2">
            <Label htmlFor="template-subject">Asunto (opcional)</Label>
            <Input id="template-subject" value={subject} onChange={(e) => setSubject(e.target.value)} />
          </div>
        </div>
        <RichEditor
          content={template?.html ?? ""}
          onError={setError}
          onPickImage={(editor) => {
            void svc
              .PickImage()
              .then(({ dataUrl, filename }) => {
                if (dataUrl) editor.chain().focus().setImage({ src: dataUrl, alt: filename }).run();
              })
              .catch((e) => setError(errorMessage(e)));
          }}
          onChange={(getHTML) => {
            htmlRef.current = getHTML;
          }}
        />
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancelar
          </Button>
          <Button onClick={() => void save()} disabled={saving}>
            Guardar
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

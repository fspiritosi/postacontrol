import { useRef, useState } from "react";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Signature } from "../../../bindings/postacontrol/core/model/models";
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
  // null = firma nueva.
  signature: Signature | null;
  onClose: () => void;
};

// Se monta solo mientras está abierto, así el estado arranca limpio cada vez.
export function SignatureDialog({ signature, onClose }: Props) {
  const [name, setName] = useState(signature?.name ?? "");
  const [error, setError] = useState("");
  const [saving, setSaving] = useState(false);
  const htmlRef = useRef<() => string>(() => signature?.html ?? "");

  const save = async () => {
    setSaving(true);
    setError("");
    try {
      await svc.SaveSignature({ id: signature?.id ?? 0, name, html: htmlRef.current() });
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
          <DialogTitle>{signature ? "Editar firma" : "Nueva firma"}</DialogTitle>
          <DialogDescription className="sr-only">Nombre y contenido de la firma.</DialogDescription>
        </DialogHeader>
        <div className="space-y-2">
          <Label htmlFor="signature-name">Nombre</Label>
          <Input id="signature-name" value={name} onChange={(e) => setName(e.target.value)} />
        </div>
        <RichEditor
          content={signature?.html ?? ""}
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

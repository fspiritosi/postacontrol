import type { Signature } from "../../../bindings/postacontrol/core/model/models";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";

type Props = {
  value: number; // 0: sin firma
  signatures: Signature[];
  onChange: (id: number) => void;
};

// Elige la firma del correo. La firma no entra en el cuerpo editable: el
// núcleo la arma al enviar a partir del id.
export function SignaturePicker({ value, signatures, onChange }: Props) {
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor="compose-signature" className="w-14 shrink-0 text-muted-foreground">
        Firma
      </Label>
      <Select value={String(value)} onValueChange={(v) => onChange(Number(v))}>
        <SelectTrigger id="compose-signature" className="w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="0">Sin firma</SelectItem>
          {signatures.map((s) => (
            <SelectItem key={s.id} value={String(s.id)}>
              {s.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

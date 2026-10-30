import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { parseRecipients } from "@/lib/recipients";

type Props = {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
};

// Campo de texto libre: se parsea al enviar. Marca las direcciones inválidas.
export function RecipientInput({ id, label, value, onChange }: Props) {
  const { invalid } = parseRecipients(value);
  return (
    <div className="flex items-center gap-2">
      <Label htmlFor={id} className="w-14 shrink-0 text-muted-foreground">
        {label}
      </Label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={invalid.length > 0}
        autoComplete="off"
        spellCheck={false}
      />
    </div>
  );
}

import { useQueryClient } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import { LABEL_COLORS } from "@/lib/labels";
import { useSignatures } from "@/lib/queries";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Account, Security } from "../../../bindings/postacontrol/core/model/models";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ServerFields, validServer, type Server } from "./AccountSetupDialog";

type Props = {
  account: Account | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const toServer = (s: Account["imap"]): Server => ({
  host: s.host,
  port: String(s.port),
  security: s.security,
});

const sameServer = (a: Server, b: Server) =>
  a.host.trim().toLowerCase() === b.host.trim().toLowerCase() &&
  a.port.trim() === b.port.trim() &&
  a.security === b.security;

export function AccountEditDialog({ account, open, onOpenChange }: Props) {
  // El formulario se monta solo con una cuenta, así el estado arranca con sus datos.
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      {account && <EditForm key={account.id} account={account} onOpenChange={onOpenChange} />}
    </Dialog>
  );
}

function EditForm({
  account,
  onOpenChange,
}: {
  account: Account;
  onOpenChange: (open: boolean) => void;
}) {
  const queryClient = useQueryClient();
  const signatures = useSignatures();
  const [displayName, setDisplayName] = useState(account.displayName);
  const [email, setEmail] = useState(account.email);
  const [username, setUsername] = useState(account.username);
  const [password, setPassword] = useState("");
  const [imap, setImap] = useState<Server>(toServer(account.imap));
  const [smtp, setSmtp] = useState<Server>(toServer(account.smtp));
  const [color, setColor] = useState(account.color);
  const [signatureId, setSignatureId] = useState(account.defaultSignatureId);
  const [saveSentCopy, setSaveSentCopy] = useState(account.saveSentCopy);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Un color fuera de la paleta (de una versión anterior) va primero y queda elegido.
  const colors =
    account.color && !LABEL_COLORS.includes(account.color)
      ? [account.color, ...LABEL_COLORS]
      : LABEL_COLORS;

  // Misma regla que el núcleo: cambia el buzón si cambia el host IMAP
  // (sin distinguir mayúsculas) o el usuario efectivo (vacío = el correo).
  const effectiveUser = (user: string, mail: string) => (user.trim() || mail.trim()).toLowerCase();
  const mailboxChanged =
    imap.host.trim().toLowerCase() !== account.imap.host.trim().toLowerCase() ||
    effectiveUser(username, email) !== effectiveUser(account.username, account.email);

  // Cambia algo de conexión: se va a probar contra el servidor.
  const connectionChanged =
    password !== "" ||
    email.trim() !== account.email ||
    username.trim() !== account.username ||
    !sameServer(imap, toServer(account.imap)) ||
    !sameServer(smtp, toServer(account.smtp));

  const valid = validServer(imap) && validServer(smtp) && email.trim() !== "";

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setSaving(true);
    setError(null);
    try {
      await svc.UpdateAccount(account.id, {
        id: account.id,
        email: email.trim(),
        displayName: displayName.trim(),
        username: username.trim(),
        color,
        imap: { host: imap.host.trim(), port: Number(imap.port), security: imap.security as Security },
        smtp: { host: smtp.host.trim(), port: Number(smtp.port), security: smtp.security as Security },
        saveSentCopy,
        defaultSignatureId: signatureId,
        password,
      });
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      void queryClient.invalidateQueries({ queryKey: ["folders", account.id] });
      toast.success("Cuenta guardada.");
      setSaving(false);
      onOpenChange(false);
    } catch (err) {
      setError(errorMessage(err));
      setSaving(false);
    }
  }

  return (
    <DialogContent
      className="max-h-[90vh] overflow-y-auto sm:max-w-xl"
      onInteractOutside={(e) => saving && e.preventDefault()}
      onEscapeKeyDown={(e) => saving && e.preventDefault()}
    >
      <DialogHeader>
        <DialogTitle>Editar cuenta</DialogTitle>
        <DialogDescription>{account.email}</DialogDescription>
      </DialogHeader>
      <form onSubmit={save} className="space-y-4">
        <div className="grid grid-cols-2 gap-2">
          <div className="space-y-1.5">
            <Label htmlFor="edit-name">Nombre para mostrar</Label>
            <Input id="edit-name" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-email">Dirección</Label>
            <Input
              id="edit-email"
              type="email"
              spellCheck={false}
              value={email}
              onChange={(e) => setEmail(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-user">Usuario</Label>
            <Input
              id="edit-user"
              spellCheck={false}
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="edit-pass">Contraseña</Label>
            <Input
              id="edit-pass"
              type="password"
              spellCheck={false}
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
            <p className="text-xs text-muted-foreground">Dejala vacía para conservar la actual.</p>
          </div>
        </div>
        <ServerFields label="IMAP" prefix="imap" value={imap} onChange={setImap} />
        <ServerFields label="SMTP" prefix="smtp" value={smtp} onChange={setSmtp} />
        {mailboxChanged && (
          <Alert>
            <AlertDescription>
              Cambiaste el servidor o el usuario: se van a volver a bajar los correos de esta cuenta. En
              el servidor no se borra nada.
            </AlertDescription>
          </Alert>
        )}
        <div className="space-y-1.5">
          <Label id="edit-color-label">Color</Label>
          <div role="group" aria-labelledby="edit-color-label" className="flex flex-wrap gap-2">
            {colors.map((c) => (
              <button
                key={c}
                type="button"
                aria-label={c}
                aria-pressed={color === c}
                onClick={() => setColor(c)}
                style={{ backgroundColor: c }}
                className="size-6 rounded-full outline-offset-2 aria-pressed:outline-2 aria-pressed:outline-foreground focus-visible:outline-2 focus-visible:outline-ring"
              />
            ))}
          </div>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="edit-sig">Firma predeterminada</Label>
          <Select
            value={String(signatureId)}
            onValueChange={(v) => setSignatureId(Number(v))}
          >
            <SelectTrigger id="edit-sig" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="0">Sin firma</SelectItem>
              {(signatures.data ?? []).map((s) => (
                <SelectItem key={s.id} value={String(s.id)}>
                  {s.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-center gap-2">
          <Switch id="edit-sent" checked={saveSentCopy} onCheckedChange={setSaveSentCopy} />
          <Label htmlFor="edit-sent">Guardar copia en Enviados</Label>
        </div>
        {error && (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        )}
        <DialogFooter>
          <Button type="submit" disabled={saving || !valid}>
            {saving ? (
              <>
                <Loader2Icon className="animate-spin" /> {connectionChanged ? "Probando…" : "Guardando…"}
              </>
            ) : (
              "Guardar"
            )}
          </Button>
        </DialogFooter>
      </form>
    </DialogContent>
  );
}

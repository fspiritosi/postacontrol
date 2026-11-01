import { useQueryClient } from "@tanstack/react-query";
import { Loader2Icon } from "lucide-react";
import { useRef, useState, type FormEvent } from "react";
import { toast } from "sonner";
import { errorMessage } from "@/lib/errors";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { Settings } from "../../../bindings/postacontrol/core/autoconfig/models";
import type { Security } from "../../../bindings/postacontrol/core/model/models";
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

type Props = { open: boolean; onOpenChange: (open: boolean) => void };

type Server = { host: string; port: string; security: string };
type Step = "credentials" | "detecting" | "servers";

const emptyServer = (security: string): Server => ({ host: "", port: "", security });

const fromSettings = (s: Settings["imap"]): Server => ({
  host: s.host,
  port: String(s.port),
  security: s.security,
});

const validPort = (p: string) => {
  const n = Number(p);
  return p.trim() !== "" && Number.isInteger(n) && n >= 1 && n <= 65535;
};
const validServer = (s: Server) => s.host.trim() !== "" && validPort(s.port);

// Los puertos estándar fijan la seguridad: 993/465 son TLS directo y
// 143/587 usan STARTTLS. Con otro puerto se deja lo elegido.
const PORT_SECURITY: Record<string, string> = { "993": "tls", "465": "tls", "143": "starttls", "587": "starttls" };

function ServerFields({
  label,
  prefix,
  value,
  onChange,
}: {
  label: string;
  prefix: string;
  value: Server;
  onChange: (v: Server) => void;
}) {
  return (
    <fieldset className="grid grid-cols-[1fr_5.5rem_8rem] gap-2">
      <legend className="mb-1 text-sm font-medium">{label}</legend>
      <div className="space-y-1.5">
        <Label htmlFor={`${prefix}-host`}>Servidor {label}</Label>
        <Input
          id={`${prefix}-host`}
          value={value.host}
          onChange={(e) => onChange({ ...value, host: e.target.value })}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${prefix}-port`}>Puerto {label}</Label>
        <Input
          id={`${prefix}-port`}
          type="number"
          value={value.port}
          onChange={(e) => {
            const port = e.target.value;
            onChange({ ...value, port, security: PORT_SECURITY[port.trim()] ?? value.security });
          }}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`${prefix}-sec`}>Seguridad {label}</Label>
        <Select value={value.security} onValueChange={(security) => onChange({ ...value, security })}>
          <SelectTrigger id={`${prefix}-sec`} className="w-full">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="tls">TLS</SelectItem>
            <SelectItem value="starttls">STARTTLS</SelectItem>
          </SelectContent>
        </Select>
      </div>
    </fieldset>
  );
}

export function AccountSetupDialog({ open, onOpenChange }: Props) {
  const queryClient = useQueryClient();
  const [step, setStep] = useState<Step>("credentials");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [detected, setDetected] = useState(false);
  const [imap, setImap] = useState<Server>(emptyServer("tls"));
  const [smtp, setSmtp] = useState<Server>(emptyServer("starttls"));
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const requestId = useRef(0);

  function reset() {
    requestId.current++;
    setStep("credentials");
    setEmail("");
    setPassword("");
    setDetected(false);
    setImap(emptyServer("tls"));
    setSmtp(emptyServer("starttls"));
    setUsername("");
    setDisplayName("");
    setError(null);
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      if (saving || step === "detecting") return;
      reset();
    }
    onOpenChange(next);
  }

  async function detect(e: FormEvent) {
    e.preventDefault();
    const id = ++requestId.current;
    setStep("detecting");
    setError(null);
    let found: Settings | null;
    try {
      found = await svc.DetectSettings(email.trim());
    } catch {
      found = null;
    }
    if (id !== requestId.current) return; // resultado viejo: el diálogo se reseteó
    if (found) {
      setImap(fromSettings(found.imap));
      setSmtp(fromSettings(found.smtp));
      setUsername(found.username || email.trim());
    } else {
      setImap(emptyServer("tls"));
      setSmtp(emptyServer("starttls"));
      setUsername(email.trim());
    }
    setDetected(found !== null);
    setStep("servers");
  }

  const valid = validServer(imap) && validServer(smtp);

  async function save(e: FormEvent) {
    e.preventDefault();
    if (!valid) return;
    setSaving(true);
    setError(null);
    try {
      await svc.AddAccount({
        id: 0,
        email: email.trim(),
        displayName: displayName.trim(),
        username: username.trim(),
        color: "",
        imap: { host: imap.host.trim(), port: Number(imap.port), security: imap.security as Security },
        smtp: { host: smtp.host.trim(), port: Number(smtp.port), security: smtp.security as Security },
        saveSentCopy: true,
        defaultSignatureId: 0,
        password,
      });
      void queryClient.invalidateQueries({ queryKey: ["accounts"] });
      toast.success("Cuenta agregada. Sincronizando…");
      setSaving(false);
      handleOpenChange(false);
    } catch (err) {
      setError(errorMessage(err));
      setSaving(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>Agregar cuenta</DialogTitle>
          <DialogDescription>
            {step === "servers"
              ? "Revisá los servidores y probá la conexión."
              : "Ingresá tu correo y contraseña."}
          </DialogDescription>
        </DialogHeader>

        {step !== "servers" ? (
          <form onSubmit={detect} className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="acc-email">Correo electrónico</Label>
              <Input
                id="acc-email"
                type="email"
                autoComplete="off"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                disabled={step === "detecting"}
                required
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="acc-pass">Contraseña</Label>
              <Input
                id="acc-pass"
                type="password"
                autoComplete="off"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                disabled={step === "detecting"}
                required
              />
            </div>
            <DialogFooter>
              <Button type="submit" disabled={step === "detecting"}>
                {step === "detecting" ? (
                  <>
                    <Loader2Icon className="animate-spin" /> Buscando la configuración…
                  </>
                ) : (
                  "Continuar"
                )}
              </Button>
            </DialogFooter>
          </form>
        ) : (
          <form onSubmit={save} className="space-y-4">
            <Alert>
              <AlertDescription>
                {detected
                  ? "Encontramos la configuración"
                  : "No pudimos detectar los servidores; cargalos a mano"}
              </AlertDescription>
            </Alert>
            <ServerFields label="IMAP" prefix="imap" value={imap} onChange={setImap} />
            <ServerFields label="SMTP" prefix="smtp" value={smtp} onChange={setSmtp} />
            <div className="grid grid-cols-2 gap-2">
              <div className="space-y-1.5">
                <Label htmlFor="acc-user">Usuario</Label>
                <Input id="acc-user" value={username} onChange={(e) => setUsername(e.target.value)} />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="acc-name">Nombre visible</Label>
                <Input id="acc-name" value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
              </div>
            </div>
            {error && (
              <Alert variant="destructive">
                <AlertDescription>{error}</AlertDescription>
              </Alert>
            )}
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setStep("credentials")} disabled={saving}>
                Atrás
              </Button>
              <Button type="submit" disabled={saving || !valid}>
                {saving ? (
                  <>
                    <Loader2Icon className="animate-spin" /> Probando conexión…
                  </>
                ) : (
                  "Probar y guardar"
                )}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}

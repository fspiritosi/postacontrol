import { MailIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { AccountSetupDialog } from "./AccountSetupDialog";

export function Welcome() {
  const [open, setOpen] = useState(false);
  return (
    <main className="flex h-screen flex-col items-center justify-center gap-6 bg-background p-6 text-center">
      <div className="flex size-14 items-center justify-center rounded-2xl border bg-card shadow-sm">
        <MailIcon className="size-7 text-foreground" />
      </div>
      <div className="space-y-2">
        <h1 className="text-2xl font-semibold tracking-tight">Bienvenido a PostaControl</h1>
        <p className="text-sm text-muted-foreground">
          Configurá tu primera cuenta de correo para empezar.
        </p>
      </div>
      <Button size="lg" onClick={() => setOpen(true)}>
        Agregar cuenta
      </Button>
      <AccountSetupDialog open={open} onOpenChange={setOpen} />
    </main>
  );
}

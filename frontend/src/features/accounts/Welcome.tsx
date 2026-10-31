import { useState } from "react";
import { Button } from "@/components/ui/button";
import { AccountSetupDialog } from "./AccountSetupDialog";

export function Welcome() {
  const [open, setOpen] = useState(false);
  return (
    <main className="flex h-screen flex-col items-center justify-center gap-6 bg-background p-6 text-center">
      <img src="/logo.png" alt="PostaControl" width={64} height={64} className="size-16" />
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

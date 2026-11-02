import { PlusIcon } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { contactDisplayName } from "@/lib/contacts";
import { useContacts } from "@/lib/queries";
import { cn } from "@/lib/utils";
import type { ShellState } from "../layout/MailShell";

// Lista de la libreta con buscador; la ficha del elegido va en el panel derecho.
export function ContactList({ state }: { state: ShellState }) {
  const [query, setQuery] = useState("");
  const contacts = useContacts(query);
  const list = contacts.data;

  return (
    <div className="flex h-full flex-col">
      <div className="flex items-center gap-2 border-b p-2">
        <Input
          type="search"
          placeholder="Buscar contactos"
          aria-label="Buscar contactos"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <Button variant="outline" onClick={() => state.selectContact(0)}>
          <PlusIcon />
          Nuevo contacto
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {/* La lista anterior vacía no dice nada de la búsqueda nueva. */}
        {list?.length === 0 && !contacts.isPlaceholderData && (
          <p className="p-4 text-center text-sm text-muted-foreground">
            {query === "" ? "Todavía no tenés contactos." : `Ningún contacto coincide con «${query}».`}
          </p>
        )}
        <ul className="space-y-0.5">
          {list?.map((c) => {
            const emails = c.emails ?? [];
            const active = state.contactId === c.id;
            return (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => state.selectContact(c.id)}
                  aria-current={active ? "true" : undefined}
                  className={cn(
                    "flex w-full flex-col rounded-md px-2 py-1.5 text-left text-sm transition-colors hover:bg-accent",
                    active && "bg-accent font-medium",
                  )}
                >
                  <span className="truncate">{contactDisplayName(c)}</span>
                  {c.name && emails[0] && (
                    <span className="truncate text-xs font-normal text-muted-foreground">{emails[0]}</span>
                  )}
                </button>
              </li>
            );
          })}
        </ul>
      </div>
    </div>
  );
}

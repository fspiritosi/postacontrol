import { XIcon } from "lucide-react";
import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ClipboardEvent,
  type DragEvent,
  type KeyboardEvent,
} from "react";
import { ContextMenu, ContextMenuContent, ContextMenuItem, ContextMenuTrigger } from "@/components/ui/context-menu";
import { Label } from "@/components/ui/label";
import { useSuggestions } from "@/lib/queries";
import {
  balanceRecipientToken,
  formatRecipients,
  parseRecipients,
  splitRecipientTokens,
  takeRecipientTokens,
} from "@/lib/recipients";
import { cn } from "@/lib/utils";
import type { Address, Suggestion } from "../../../bindings/postacontrol/core/model/models";

// Tipo propio del arrastre de un chip: un campo de texto (Asunto, el cuerpo) no
// lo acepta, así que la dirección no se pega ahí como texto.
export const RECIPIENT_DRAG_TYPE = "application/x-postacontrol-recipient";

// Arrastrar chips entre Para, CC y CCO. El redactor sabe qué chip está en vuelo
// y mueve la dirección de un campo al otro.
export type RecipientDrag = {
  // Hay un chip de otro campo en vuelo: este campo lo recibe.
  accepting: boolean;
  start: (index: number, token: string) => void;
  end: () => void;
  drop: () => void;
  // Los otros campos, para "Mover a…" desde el clic derecho (sin arrastrar).
  targets: { label: string; move: (index: number, token: string) => void }[];
};

type Props = {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
  // Avisa cuándo abre la lista de sugerencias (con una función para cerrarla) y
  // cuándo cierra, como onSlashMenuChange: dentro del diálogo, el Esc lo ve Radix primero.
  onSuggestionsChange?: (close: (() => void) | null) => void;
  drag?: RecipientDrag;
};

const NO_SUGGESTIONS: Suggestion[] = [];

// El correo, en minúsculas, si el texto es una sola dirección válida.
function singleEmail(text: string): string | null {
  const { valid, invalid } = parseRecipients(text);
  return valid.length === 1 && invalid.length === 0 ? valid[0].email.toLowerCase() : null;
}

// Destinatarios como chips. El valor sigue siendo el texto completo (se parsea
// al enviar): los chips son sus tokens y lo que se está escribiendo va al final.
export function RecipientInput({ id, label, value, onChange, onSuggestionsChange, drag }: Props) {
  const [pending, setPending] = useState("");
  // El chip en vuelo está sobre este campo. Al terminar el arrastre se
  // olvida, aunque no haya llegado el dragleave.
  const [over, setOver] = useState(false);
  const accepting = drag?.accepting ?? false;
  if (over && !accepting) setOver(false);
  // Texto para el que se cerró la lista con Esc: al seguir escribiendo vuelve a abrir.
  const [dismissed, setDismissed] = useState<string | null>(null);
  // La opción elegida con las flechas (o el mouse) va con la lista y el texto a
  // los que corresponde: si cambia cualquiera de los dos, deja de valer.
  const [active, setActive] = useState<{
    list: Suggestion[];
    text: string;
    index: number;
    via: "keys" | "mouse";
  } | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const tokens = splitRecipientTokens(value);
  const typed = pending.trim();
  // Balanceados por si el valor llegó con algo abierto: si no, lo que se escriba se le pegaría.
  const chips = (typed !== "" && tokens[tokens.length - 1] === typed ? tokens.slice(0, -1) : tokens).map(
    balanceRecipientToken,
  );
  const parsed = chips.map((token) => ({ token, addr: parseRecipients(token).valid[0] as Address | undefined }));
  const exclude = parsed.flatMap((c) => (c.addr ? [c.addr.email] : []));

  const suggestions = useSuggestions(typed, exclude);
  // Sin texto no hay lista, aunque keepPreviousData conserve la última.
  const items = (typed !== "" && suggestions.data) || NO_SUGGESTIONS;
  const open = items.length > 0 && dismissed !== pending;
  // Sin elegir nada, Enter toma lo escrito (y no se marca ninguna) si la lista
  // es de una búsqueda anterior o si ya es una dirección entera y la primera es
  // otra: no cambia "ana@x.com" por "mariana@x.com".
  const typedEmail = singleEmail(typed);
  const keepTyped =
    suggestions.isPlaceholderData || (typedEmail !== null && items[0]?.email.toLowerCase() !== typedEmail);
  // Pasar el mouse no alcanza para cambiar lo escrito por otra persona: en ese
  // caso solo vale lo elegido con las flechas (o un clic, que elige directo).
  const current = active?.list === items && active.text === typed ? active : null;
  const chosen = current && !(keepTyped && current.via === "mouse") ? current.index : null;
  const index = chosen ?? (keepTyped ? -1 : 0);

  const notifySuggestions = useEffectEvent((close: (() => void) | null) => onSuggestionsChange?.(close));
  useEffect(() => {
    if (!open) return;
    notifySuggestions(() => setDismissed(pending));
    return () => notifySuggestions(null);
  }, [open, pending]);

  const emit = (nextChips: string[], nextPending: string) => {
    setPending(nextPending);
    onChange([...nextChips, nextPending].filter(Boolean).join(", "));
  };

  // Lo cerrado por un separador pasa a chip; lo que sigue queda en el campo.
  const changeText = (text: string) => {
    const { tokens: done, rest } = takeRecipientTokens(text);
    emit([...chips, ...done], rest);
  };

  const commit = () => {
    if (typed !== "") emit([...chips, balanceRecipientToken(typed)], "");
  };

  const pick = (s: Suggestion) => emit([...chips, formatRecipients([{ name: s.name, email: s.email }])], "");

  const remove = (i: number) => emit(chips.filter((_, j) => j !== i), pending);

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (open && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
      e.preventDefault();
      const step = e.key === "ArrowDown" ? 1 : -1;
      const next = index < 0 ? (step > 0 ? 0 : items.length - 1) : (index + step + items.length) % items.length;
      setActive({ list: items, text: typed, index: next, via: "keys" });
    } else if (e.key === "Enter" || (e.key === "Tab" && !e.shiftKey)) {
      // Sin nada escrito, Tab deja pasar el foco.
      if (open && index >= 0) {
        e.preventDefault();
        pick(items[index]);
      } else if (typed !== "") {
        e.preventDefault();
        commit();
      }
    } else if (e.key === "Escape" && open) {
      e.preventDefault();
      setDismissed(pending);
    } else if (e.key === "Backspace" && pending === "" && chips.length > 0) {
      e.preventDefault();
      remove(chips.length - 1);
    }
  };

  // Un input de una línea se come los saltos de línea: una lista pegada así se separa acá.
  const onPaste = (e: ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData("text");
    if (!/[\r\n]/.test(text)) return;
    e.preventDefault();
    const el = e.currentTarget;
    const start = el.selectionStart ?? pending.length;
    const end = el.selectionEnd ?? start;
    changeText(pending.slice(0, start) + text + pending.slice(end));
  };

  // Solo cuenta un chip de otro campo: ni archivos, ni texto, ni uno de acá mismo.
  const acceptsDrag = (e: DragEvent) => accepting && e.dataTransfer.types.includes(RECIPIENT_DRAG_TYPE);

  const onDragOver = (e: DragEvent<HTMLDivElement>) => {
    if (!acceptsDrag(e)) return;
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setOver(true);
  };

  const onDragLeave = (e: DragEvent<HTMLDivElement>) => {
    if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOver(false);
  };

  const onDrop = (e: DragEvent<HTMLDivElement>) => {
    if (!acceptsDrag(e)) return;
    e.preventDefault();
    setOver(false);
    drag?.drop();
  };

  const listId = `${id}-suggestions`;
  const optionId = (i: number) => `${id}-suggestion-${i}`;

  return (
    <div className="flex items-start gap-2">
      <Label htmlFor={id} className="h-9 w-14 shrink-0 text-muted-foreground">
        {label}
      </Label>
      <div className="relative min-w-0 flex-1">
        <div
          className={cn(
            "flex min-h-9 w-full flex-wrap items-center gap-1 rounded-md border border-input bg-transparent px-1.5 py-1 shadow-xs transition-[color,box-shadow] focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 dark:bg-input/30",
            accepting && "border-dashed border-primary/60",
            over && "border-solid border-primary bg-primary/5 ring-3 ring-ring/50",
          )}
          onDragEnter={onDragOver}
          onDragOver={onDragOver}
          onDragLeave={onDragLeave}
          onDrop={onDrop}
          // Un click en el espacio libre entre chips lleva al campo.
          onMouseDown={(e) => {
            if (e.target !== e.currentTarget) return;
            e.preventDefault();
            inputRef.current?.focus();
          }}
        >
          {parsed.map(({ token, addr }, i) => (
            <ContextMenu key={`${i}:${token}`}>
              <ContextMenuTrigger asChild>
                <span
                  title={token}
                  aria-invalid={!addr || undefined}
                  draggable={drag !== undefined}
                  onDragStart={(e) => {
                    if (!drag) return;
                    e.dataTransfer.setData(RECIPIENT_DRAG_TYPE, token);
                    e.dataTransfer.effectAllowed = "move";
                    drag.start(i, token);
                  }}
                  onDragEnd={() => drag?.end()}
                  className={cn(
                    "inline-flex max-w-full items-center gap-0.5 rounded-sm bg-secondary py-0.5 pr-0.5 pl-2 text-sm text-secondary-foreground",
                    !addr && "bg-destructive/10 text-destructive",
                    drag && "cursor-grab active:cursor-grabbing",
                  )}
                >
                  <span className="truncate">{addr ? addr.name || addr.email : token}</span>
                  <button
                    type="button"
                    // Desde el teclado se quitan con Backspace; así Tab no pasa por cada chip.
                    tabIndex={-1}
                    aria-label={`Quitar ${token}`}
                    // mousedown (y no click) para no sacarle el foco al campo.
                    onMouseDown={(e) => e.preventDefault()}
                    onClick={() => remove(i)}
                    className="rounded-sm p-0.5 opacity-60 hover:bg-foreground/10 hover:opacity-100"
                  >
                    <XIcon className="size-3" />
                  </button>
                </span>
              </ContextMenuTrigger>
              {drag && drag.targets.length > 0 && (
                <ContextMenuContent>
                  {drag.targets.map((t) => (
                    <ContextMenuItem key={t.label} onSelect={() => t.move(i, token)}>
                      Mover a {t.label}
                    </ContextMenuItem>
                  ))}
                </ContextMenuContent>
              )}
            </ContextMenu>
          ))}
          <input
            ref={inputRef}
            id={id}
            value={pending}
            onChange={(e) => changeText(e.target.value)}
            onKeyDown={onKeyDown}
            onPaste={onPaste}
            onBlur={commit}
            role="combobox"
            aria-expanded={open}
            aria-controls={open ? listId : undefined}
            aria-autocomplete="list"
            aria-activedescendant={open && index >= 0 ? optionId(index) : undefined}
            autoComplete="off"
            spellCheck={false}
            className="h-7 min-w-24 flex-1 bg-transparent px-1 text-base outline-none md:text-sm"
          />
        </div>
        {open && (
          <div
            id={listId}
            role="listbox"
            aria-label="Sugerencias"
            className="absolute inset-x-0 top-full z-50 mt-1 rounded-md bg-popover p-1 text-sm text-popover-foreground shadow-md ring-1 ring-foreground/10"
          >
            {items.map((s, i) => (
              <div
                key={s.email}
                id={optionId(i)}
                role="option"
                aria-selected={i === index}
                // mousedown (y no click) para no sacarle el foco al campo.
                onMouseDown={(e) => {
                  e.preventDefault();
                  pick(s);
                }}
                onMouseEnter={() => setActive({ list: items, text: typed, index: i, via: "mouse" })}
                className={cn(
                  "flex cursor-pointer gap-2 rounded-sm px-2 py-1.5 hover:bg-accent/60",
                  i === index && "bg-accent text-accent-foreground",
                )}
              >
                {s.name && <span className="truncate font-medium">{s.name}</span>}
                <span className="truncate text-muted-foreground">{s.email}</span>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}

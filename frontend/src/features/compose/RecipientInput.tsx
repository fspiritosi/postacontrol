import { XIcon } from "lucide-react";
import {
  useEffect,
  useEffectEvent,
  useRef,
  useState,
  type ClipboardEvent,
  type KeyboardEvent,
} from "react";
import { Label } from "@/components/ui/label";
import { useSuggestions } from "@/lib/queries";
import {
  formatRecipients,
  parseRecipients,
  splitRecipientTokens,
  takeRecipientTokens,
} from "@/lib/recipients";
import { cn } from "@/lib/utils";
import type { Address, Suggestion } from "../../../bindings/postacontrol/core/model/models";

type Props = {
  id: string;
  label: string;
  value: string;
  onChange: (v: string) => void;
  // Avisa cuándo abre la lista de sugerencias (con una función para cerrarla) y
  // cuándo cierra, como onSlashMenuChange: dentro del diálogo, el Esc lo ve Radix primero.
  onSuggestionsChange?: (close: (() => void) | null) => void;
};

const NO_SUGGESTIONS: Suggestion[] = [];

// Destinatarios como chips. El valor sigue siendo el texto completo (se parsea
// al enviar): los chips son sus tokens y lo que se está escribiendo va al final.
export function RecipientInput({ id, label, value, onChange, onSuggestionsChange }: Props) {
  const [pending, setPending] = useState("");
  // Texto para el que se cerró la lista con Esc: al seguir escribiendo vuelve a abrir.
  const [dismissed, setDismissed] = useState<string | null>(null);
  // La opción activa va con la lista a la que corresponde: con resultados nuevos vuelve a la primera.
  const [active, setActive] = useState<{ list: Suggestion[]; index: number } | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const tokens = splitRecipientTokens(value);
  const typed = pending.trim();
  const chips = typed !== "" && tokens[tokens.length - 1] === typed ? tokens.slice(0, -1) : tokens;
  const parsed = chips.map((token) => ({ token, addr: parseRecipients(token).valid[0] as Address | undefined }));
  const exclude = parsed.flatMap((c) => (c.addr ? [c.addr.email] : []));

  const suggestions = useSuggestions(typed, exclude);
  // Sin texto no hay lista, aunque keepPreviousData conserve la última.
  const items = (typed !== "" && suggestions.data) || NO_SUGGESTIONS;
  const open = items.length > 0 && dismissed !== pending;
  const index = active?.list === items ? active.index : 0;

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
    if (typed !== "") emit([...chips, typed], "");
  };

  const pick = (s: Suggestion) => emit([...chips, formatRecipients([{ name: s.name, email: s.email }])], "");

  const remove = (i: number) => emit(chips.filter((_, j) => j !== i), pending);

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (open && (e.key === "ArrowDown" || e.key === "ArrowUp")) {
      e.preventDefault();
      const step = e.key === "ArrowDown" ? 1 : -1;
      setActive({ list: items, index: (index + step + items.length) % items.length });
    } else if (e.key === "Enter" || (e.key === "Tab" && !e.shiftKey)) {
      // Sin nada escrito, Tab deja pasar el foco.
      if (open) {
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

  const listId = `${id}-suggestions`;
  const optionId = (i: number) => `${id}-suggestion-${i}`;

  return (
    <div className="flex items-start gap-2">
      <Label htmlFor={id} className="h-9 w-14 shrink-0 text-muted-foreground">
        {label}
      </Label>
      <div className="relative min-w-0 flex-1">
        <div
          className="flex min-h-9 w-full flex-wrap items-center gap-1 rounded-md border border-input bg-transparent px-1.5 py-1 shadow-xs transition-[color,box-shadow] focus-within:border-ring focus-within:ring-3 focus-within:ring-ring/50 dark:bg-input/30"
          // Un click en el espacio libre entre chips lleva al campo.
          onMouseDown={(e) => {
            if (e.target !== e.currentTarget) return;
            e.preventDefault();
            inputRef.current?.focus();
          }}
        >
          {parsed.map(({ token, addr }, i) => (
            <span
              key={`${i}:${token}`}
              title={token}
              aria-invalid={!addr || undefined}
              className={cn(
                "inline-flex max-w-full items-center gap-0.5 rounded-sm bg-secondary py-0.5 pr-0.5 pl-2 text-sm text-secondary-foreground",
                !addr && "bg-destructive/10 text-destructive",
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
            aria-activedescendant={open ? optionId(index) : undefined}
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
                onMouseEnter={() => setActive({ list: items, index: i })}
                className={cn(
                  "flex cursor-pointer gap-2 rounded-sm px-2 py-1.5",
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

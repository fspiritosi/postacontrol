import { SearchIcon, XIcon } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Input } from "@/components/ui/input";

export const SEARCH_DELAY_MS = 300;

type Props = {
  // El texto que se está buscando (null = sin búsqueda).
  active: string | null;
  onSearch: (text: string | null) => void;
};

export function SearchBox({ active, onSearch }: Props) {
  const [text, setText] = useState(active ?? "");
  const onSearchRef = useRef(onSearch);
  useEffect(() => {
    onSearchRef.current = onSearch;
  });

  // Si la búsqueda se limpió desde afuera (cambio de vista), se vacía el campo.
  const [prevActive, setPrevActive] = useState(active);
  if (prevActive !== active) {
    setPrevActive(active);
    if (active == null) setText("");
  }

  // Busca 300 ms después de dejar de escribir.
  useEffect(() => {
    const t = text.trim();
    if (t === (active ?? "")) return;
    if (t === "") {
      onSearchRef.current(null);
      return;
    }
    const id = setTimeout(() => onSearchRef.current(t), SEARCH_DELAY_MS);
    return () => clearTimeout(id);
  }, [text, active]);

  const clear = () => {
    setText("");
    onSearch(null);
  };

  return (
    <div className="relative">
      <SearchIcon
        className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
        aria-hidden
      />
      <Input
        type="search"
        aria-label="Buscar correo"
        placeholder="Buscar correo"
        value={text}
        onChange={(e) => setText(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            clear();
          } else if (e.key === "Enter") {
            const t = text.trim();
            if (t !== "") onSearch(t);
          }
        }}
        className="h-8 pl-8 pr-8 [&::-webkit-search-cancel-button]:appearance-none"
      />
      {text !== "" && (
        <button
          type="button"
          aria-label="Limpiar búsqueda"
          onClick={clear}
          className="absolute right-1.5 top-1/2 inline-flex size-5 -translate-y-1/2 items-center justify-center rounded text-muted-foreground hover:text-foreground"
        >
          <XIcon className="size-3.5" />
        </button>
      )}
    </div>
  );
}

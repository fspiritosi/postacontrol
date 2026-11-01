import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  SuggestRecipients: vi.fn(),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { parseRecipients } from "@/lib/recipients";
import { RecipientInput } from "./RecipientInput";

// Radix y ProseMirror piden APIs que jsdom no trae (las mismas que en SignaturesSection.test).
beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
  const noRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] }) as unknown as DOMRectList;
  Range.prototype.getClientRects = noRects;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
});

const suggestions = [
  { name: "Juan Pérez", email: "juan@x.com", contactId: 1 },
  { name: "", email: "juana@y.com", contactId: 0 },
];

// El campo es controlado: el arnés guarda el valor como lo haría el redactor.
function Harness({ initial, onChange }: { initial: string; onChange: (v: string) => void }) {
  const [value, setValue] = useState(initial);
  return (
    <RecipientInput
      id="to"
      label="Para"
      value={value}
      onChange={(v) => {
        onChange(v);
        setValue(v);
      }}
    />
  );
}

function setup(initial = "") {
  const onChange = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <Harness initial={initial} onChange={onChange} />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), onChange, input: screen.getByLabelText("Para") };
}

// Contesta solo a esa búsqueda: la lista abre recién con su resultado, no con
// el de una anterior (que keepPreviousData mostraría mientras tanto).
const suggestFor = (query: string, list: object[]) =>
  vi.mocked(svc.SuggestRecipients).mockImplementation((async (q: string) => (q === query ? list : [])) as never);

const chipButtons = () => screen.queryAllByRole("button", { name: /^Quitar / });
const lastValue = (fn: ReturnType<typeof vi.fn>) => fn.mock.calls[fn.mock.calls.length - 1]?.[0];

describe("RecipientInput", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.SuggestRecipients).mockResolvedValue([]);
  });

  it("un valor inicial se muestra entero como chips", () => {
    const { input } = setup("Ana <a@x.com>, b@y.com");
    expect(chipButtons()).toHaveLength(2);
    expect(screen.getByTitle("Ana <a@x.com>")).toHaveTextContent("Ana");
    expect(screen.getByTitle("b@y.com")).toHaveTextContent("b@y.com");
    expect(input).toHaveValue("");
  });

  it("escribir y poner coma crea un chip", async () => {
    const { user, onChange, input } = setup();
    await user.type(input, "c@z.com,");
    expect(lastValue(onChange)).toBe("c@z.com");
    expect(screen.getByTitle("c@z.com")).toBeInTheDocument();
    expect(chipButtons()).toHaveLength(1);
    expect(input).toHaveValue("");
  });

  it("pegar varias direcciones las convierte en chips", async () => {
    const { user, onChange, input } = setup();
    await user.click(input);
    await user.paste("a@x.com; b@y.com, c@z.com");
    expect(chipButtons()).toHaveLength(2);
    // Lo último, sin separador detrás, sigue en el campo hasta salir.
    expect(input).toHaveValue("c@z.com");
    await user.tab();
    expect(chipButtons()).toHaveLength(3);
    expect(parseRecipients(lastValue(onChange)).valid.map((a) => a.email)).toEqual([
      "a@x.com",
      "b@y.com",
      "c@z.com",
    ]);
  });

  it("pegar una lista con saltos de línea también la separa", async () => {
    const { user, input } = setup();
    await user.click(input);
    await user.paste("a@x.com\nb@y.com\n");
    expect(chipButtons()).toHaveLength(2);
    expect(input).toHaveValue("");
  });

  it("Backspace en vacío quita el último chip", async () => {
    const { user, onChange, input } = setup("a@x.com, b@y.com");
    await user.click(input);
    await user.keyboard("{Backspace}");
    expect(lastValue(onChange)).toBe("a@x.com");
    expect(chipButtons()).toHaveLength(1);
    expect(screen.queryByTitle("b@y.com")).toBeNull();
  });

  it("la × quita ese chip", async () => {
    const { user, onChange } = setup("a@x.com, b@y.com, c@z.com");
    await user.click(screen.getByRole("button", { name: "Quitar b@y.com" }));
    expect(lastValue(onChange)).toBe("a@x.com, c@z.com");
    expect(chipButtons()).toHaveLength(2);
  });

  it("un chip inválido queda marcado", async () => {
    const { user, input } = setup();
    await user.type(input, "hola");
    await user.tab();
    expect(screen.getByTitle("hola")).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveValue("");
  });

  it("un chip válido no queda marcado", () => {
    setup("a@x.com");
    expect(screen.getByTitle("a@x.com")).not.toHaveAttribute("aria-invalid", "true");
  });

  it("un < sin cerrar se cierra al confirmar y no se come lo que sigue", async () => {
    const { user, onChange, input } = setup();
    await user.type(input, "Ana <a@x.com");
    await user.tab();
    expect(lastValue(onChange)).toBe("Ana <a@x.com>");
    await user.type(input, "f");
    expect(lastValue(onChange)).toBe("Ana <a@x.com>, f");
    await user.type(input, "o");
    expect(lastValue(onChange)).toBe("Ana <a@x.com>, fo");
    expect(chipButtons()).toHaveLength(1);
    expect(screen.getByTitle("Ana <a@x.com>")).toHaveTextContent("Ana");
    expect(input).toHaveValue("fo");
  });

  it("una coma dentro de <...> separa igual", async () => {
    const { user, onChange, input } = setup();
    await user.type(input, "Ana <a@x.com,");
    expect(lastValue(onChange)).toBe("Ana <a@x.com>");
    expect(input).toHaveValue("");
    await user.type(input, "b@y.com,");
    expect(lastValue(onChange)).toBe("Ana <a@x.com>, b@y.com");
    expect(chipButtons()).toHaveLength(2);
    expect(screen.getByTitle("Ana <a@x.com>")).not.toHaveAttribute("aria-invalid", "true");
  });

  it("una comilla sin cerrar no se come las direcciones que siguen", async () => {
    const { user, onChange, input } = setup();
    await user.type(input, '"hola');
    await user.tab();
    await user.type(input, "b@y.com,");
    expect(lastValue(onChange)).toBe("hola, b@y.com");
    expect(chipButtons()).toHaveLength(2);
    expect(screen.getByTitle("hola")).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByTitle("b@y.com")).not.toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveValue("");
  });

  it("un valor inicial con un < sin cerrar no duplica lo que se escribe", async () => {
    const { user, onChange, input } = setup("Ana <a@x.com");
    await user.type(input, "fo");
    expect(lastValue(onChange)).toBe("Ana <a@x.com>, fo");
    expect(chipButtons()).toHaveLength(1);
  });

  it("las sugerencias aparecen y Enter elige la activa", async () => {
    suggestFor("jua", suggestions);
    const { user, onChange, input } = setup();
    await user.type(input, "jua");
    const list = await screen.findByRole("listbox");
    const options = screen.getAllByRole("option");
    expect(list).toBeInTheDocument();
    expect(options).toHaveLength(2);
    expect(options[0]).toHaveTextContent("Juan Pérez");
    expect(options[0]).toHaveTextContent("juan@x.com");
    expect(options[0]).toHaveAttribute("aria-selected", "true");
    expect(input).toHaveAttribute("aria-activedescendant", options[0].id);
    await user.keyboard("{ArrowDown}");
    expect(options[1]).toHaveAttribute("aria-selected", "true");
    expect(input).toHaveAttribute("aria-activedescendant", options[1].id);
    await user.keyboard("{Enter}");
    expect(lastValue(onChange)).toBe("juana@y.com");
    expect(screen.getByTitle("juana@y.com")).toBeInTheDocument();
    expect(input).toHaveValue("");
    expect(input).toHaveFocus();
    expect(screen.queryByRole("listbox")).toBeNull();
  });

  it("las flechas recorren la lista en círculo", async () => {
    vi.mocked(svc.SuggestRecipients).mockResolvedValue(suggestions as never);
    const { user, input } = setup();
    await user.type(input, "jua");
    await screen.findByRole("listbox");
    await user.keyboard("{ArrowUp}");
    expect(screen.getAllByRole("option")[1]).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{ArrowDown}");
    expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
  });

  it("Tab con la lista abierta elige la activa y deja el foco en el campo", async () => {
    suggestFor("jua", suggestions);
    const { user, onChange, input } = setup();
    await user.type(input, "jua");
    await screen.findByRole("listbox");
    await user.tab();
    expect(lastValue(onChange)).toBe("Juan Pérez <juan@x.com>");
    expect(input).toHaveFocus();
  });

  describe("con una dirección completa escrita", () => {
    const mariana = { name: "Mariana", email: "mariana@x.com", contactId: 3 };
    const otra = { name: "", email: "ana.b@x.com", contactId: 0 };
    const answer = (list: object[]) => suggestFor("ana@x.com", list);

    it("Enter deja lo escrito si la primera sugerencia es otra dirección", async () => {
      answer([mariana, otra]);
      const { user, onChange, input } = setup();
      await user.type(input, "ana@x.com");
      await screen.findByRole("listbox");
      // Sin opción marcada: Enter toma lo escrito y la lista no dice otra cosa.
      expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "false");
      expect(input).not.toHaveAttribute("aria-activedescendant");
      await user.keyboard("{Enter}");
      expect(lastValue(onChange)).toBe("ana@x.com");
      expect(input).toHaveValue("");
    });

    it("Tab también deja lo escrito", async () => {
      answer([mariana, otra]);
      const { user, onChange, input } = setup();
      await user.type(input, "ana@x.com");
      await screen.findByRole("listbox");
      await user.tab();
      expect(lastValue(onChange)).toBe("ana@x.com");
      expect(input).toHaveFocus();
    });

    it("con las flechas sí se elige otra", async () => {
      answer([mariana, otra]);
      const { user, onChange, input } = setup();
      await user.type(input, "ana@x.com");
      await screen.findByRole("listbox");
      await user.keyboard("{ArrowDown}");
      expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
      await user.keyboard("{ArrowDown}{ArrowUp}{Enter}");
      expect(lastValue(onChange)).toBe("Mariana <mariana@x.com>");
    });

    it("si la primera es la misma dirección, Enter la elige con su nombre", async () => {
      answer([{ name: "Ana", email: "ANA@x.com", contactId: 4 }, mariana]);
      const { user, onChange, input } = setup();
      await user.type(input, "ana@x.com");
      await screen.findByRole("listbox");
      expect(screen.getAllByRole("option")[0]).toHaveAttribute("aria-selected", "true");
      await user.keyboard("{Enter}");
      expect(lastValue(onChange)).toBe("Ana <ANA@x.com>");
    });
  });

  it("con la lista de una búsqueda anterior, Enter deja lo escrito", async () => {
    // La primera búsqueda contesta; las siguientes quedan colgadas, así la lista
    // que se ve es la anterior (keepPreviousData).
    vi.mocked(svc.SuggestRecipients)
      .mockResolvedValueOnce([
        { name: "Mariana", email: "mariana@x.com", contactId: 3 },
        { name: "Marta", email: "marta@x.com", contactId: 5 },
      ] as never)
      .mockReturnValue(new Promise(() => {}) as never);
    const { user, onChange, input } = setup();
    await user.type(input, "m");
    await screen.findByRole("listbox");
    // Lo marcado antes de seguir escribiendo no cuenta.
    await user.keyboard("{ArrowDown}{ArrowDown}");
    await user.type(input, "a");
    expect(screen.getByRole("listbox")).toBeInTheDocument();
    await user.keyboard("{Enter}");
    expect(lastValue(onChange)).toBe("ma");
  });

  it("con la lista anterior, lo elegido con las flechas sí vale", async () => {
    vi.mocked(svc.SuggestRecipients)
      .mockResolvedValueOnce([{ name: "Mariana", email: "mariana@x.com", contactId: 3 }] as never)
      .mockReturnValue(new Promise(() => {}) as never);
    const { user, onChange, input } = setup();
    await user.type(input, "m");
    await screen.findByRole("listbox");
    await user.type(input, "a");
    await user.keyboard("{ArrowDown}{Enter}");
    expect(lastValue(onChange)).toBe("Mariana <mariana@x.com>");
  });

  it("elegir una sugerencia con coma en el nombre deja un chip válido", async () => {
    vi.mocked(svc.SuggestRecipients).mockResolvedValue([
      { name: "Peña, Fabricio", email: "f@x.com", contactId: 2 },
    ] as never);
    const { user, onChange, input } = setup();
    await user.type(input, "pe");
    await user.click(await screen.findByRole("option", { name: /Peña, Fabricio/ }));
    const value = lastValue(onChange);
    expect(value).toBe('"Peña, Fabricio" <f@x.com>');
    expect(parseRecipients(value)).toEqual({
      valid: [{ name: "Peña, Fabricio", email: "f@x.com" }],
      invalid: [],
    });
    expect(chipButtons()).toHaveLength(1);
    expect(screen.getByTitle('"Peña, Fabricio" <f@x.com>')).not.toHaveAttribute("aria-invalid", "true");
  });

  it("Esc cierra la lista", async () => {
    vi.mocked(svc.SuggestRecipients).mockResolvedValue(suggestions as never);
    const { user, input } = setup();
    await user.type(input, "jua");
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("listbox")).toBeNull();
    expect(input).toHaveValue("jua");
    expect(chipButtons()).toHaveLength(0);
  });

  it("no sugiere lo que ya está en el campo", async () => {
    const { user, input } = setup("juan@x.com");
    await user.type(input, "jua");
    await waitFor(() => expect(svc.SuggestRecipients).toHaveBeenCalled());
    const calls = vi.mocked(svc.SuggestRecipients).mock.calls;
    const [query, exclude] = calls[calls.length - 1];
    expect(query).toBe("jua");
    expect(exclude).toContain("juan@x.com");
  });
});

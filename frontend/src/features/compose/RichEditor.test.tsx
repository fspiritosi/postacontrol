import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/components/ui/dialog";
import { RichEditor } from "./RichEditor";

// jsdom no calcula layout: ProseMirror pide rectángulos al mover la selección.
beforeAll(() => {
  // Radix Select pide APIs de puntero que jsdom no trae.
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
  const noRects = () => ({ length: 0, item: () => null, [Symbol.iterator]: [][Symbol.iterator] }) as unknown as DOMRectList;
  Range.prototype.getClientRects = noRects;
  Range.prototype.getBoundingClientRect = () => new DOMRect();
});

async function setup(content: string) {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<RichEditor content={content} onChange={onChange} />);
  const body = await screen.findByLabelText("Cuerpo del mensaje");
  // Sin click: ProseMirror usa elementFromPoint en mousedown y jsdom no lo tiene.
  act(() => body.focus());
  await user.keyboard("{Control>}a{/Control}"); // selecciona todo el texto
  return { user, onChange };
}

// La barra fija; la de formato flotante repite algunos botones.
const toolbar = () => within(screen.getByRole("toolbar", { name: "Herramientas del redactor" }));

const lastHTML = (onChange: ReturnType<typeof vi.fn>) => {
  const calls = onChange.mock.calls;
  return calls.length ? (calls[calls.length - 1][0] as () => string)() : undefined;
};

describe("RichEditor: enlaces", () => {
  it("aplica un enlace desde el popover y agrega https:// si falta", async () => {
    const { user, onChange } = await setup("<p>hola</p>");
    await user.click(toolbar().getByRole("button", { name: "Enlace" }));
    const input = await screen.findByLabelText("URL del enlace");
    await user.type(input, "ejemplo.com");
    await user.click(screen.getByRole("button", { name: "Aplicar" }));
    await waitFor(() => expect(lastHTML(onChange)).toContain('href="https://ejemplo.com"'));
    expect(screen.queryByLabelText("URL del enlace")).toBeNull();
  });

  it("rechaza esquemas no permitidos sin tocar el texto", async () => {
    const { user, onChange } = await setup("<p>hola</p>");
    await user.click(toolbar().getByRole("button", { name: "Enlace" }));
    await user.type(await screen.findByLabelText("URL del enlace"), "javascript:alert(1)");
    await user.click(screen.getByRole("button", { name: "Aplicar" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Solo se permiten enlaces http, https o mailto.",
    );
    expect(lastHTML(onChange) ?? "").not.toContain("<a");
  });

  it("muestra el enlace actual y lo quita", async () => {
    const { user, onChange } = await setup('<p><a href="https://x.y">hola</a></p>');
    await user.click(toolbar().getByRole("button", { name: "Enlace" }));
    expect(await screen.findByLabelText("URL del enlace")).toHaveValue("https://x.y");
    await user.click(screen.getByRole("button", { name: "Quitar enlace" }));
    await waitFor(() => expect(lastHTML(onChange)).toBe("<p>hola</p>"));
  });
});

// Escribe en el redactor vacío, sin seleccionar nada (setup() selecciona todo).
async function setupEmpty(props: { onPickImage?: () => void } = {}) {
  const user = userEvent.setup();
  const onChange = vi.fn();
  render(<RichEditor content="" onChange={onChange} {...props} />);
  const body = await screen.findByLabelText("Cuerpo del mensaje");
  act(() => body.focus());
  return { user, onChange, body };
}

describe("RichEditor: menú /", () => {
  it("escribir / abre el menú con los ítems", async () => {
    const { user } = await setupEmpty();
    await user.keyboard("/");
    expect(await screen.findByRole("listbox", { name: "Insertar" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Título 1" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Tabla" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: "Enlace" })).toBeInTheDocument();
    // Sin onPickImage no se ofrece "Imagen".
    expect(screen.queryByRole("option", { name: "Imagen" })).toBeNull();
  });

  it("ofrece Imagen cuando hay onPickImage y la ejecuta", async () => {
    const onPickImage = vi.fn();
    const { user } = await setupEmpty({ onPickImage });
    await user.keyboard("/imag");
    await screen.findByRole("option", { name: "Imagen" });
    await user.keyboard("{Enter}");
    expect(onPickImage).toHaveBeenCalledTimes(1);
  });

  it("/tab filtra a Tabla y Enter inserta una tabla", async () => {
    const { user, onChange } = await setupEmpty();
    await user.keyboard("/tab");
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(1));
    expect(screen.getByRole("option", { name: "Tabla" })).toBeInTheDocument();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(lastHTML(onChange)).toContain("<table"));
    expect(lastHTML(onChange)).toContain("<th");
    expect(lastHTML(onChange)).not.toMatch(/\/tab(?!le)/);
  });

  it("Esc cierra el menú y deja el texto", async () => {
    const { user, onChange } = await setupEmpty();
    await user.keyboard("/ta");
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
    expect(lastHTML(onChange)).toContain("/ta");
  });

  it("y/o no abre el menú, pero un espacio antes de la barra sí", async () => {
    const { user } = await setupEmpty();
    await user.keyboard("y/o");
    expect(screen.queryByRole("listbox")).toBeNull();
    await user.keyboard(" /");
    expect(await screen.findByRole("listbox")).toBeInTheDocument();
  });

  it("a / b + Enter deja el texto y hace un renglón nuevo", async () => {
    const { user, onChange } = await setupEmpty();
    await user.keyboard("a / b");
    expect(screen.queryByRole("listbox")).toBeNull();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(lastHTML(onChange)).toBe("<p>a / b</p><p></p>"));
  });

  it("/ + espacio cierra el menú", async () => {
    const { user } = await setupEmpty();
    await user.keyboard("/");
    await screen.findByRole("listbox");
    await user.keyboard(" ");
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  });

  it("una consulta con espacios sin coincidencias cierra el menú", async () => {
    const { user } = await setupEmpty();
    await user.keyboard("/hola mundo");
    await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  });

  it("dentro de un diálogo el menú se monta en el diálogo", async () => {
    const user = userEvent.setup();
    render(
      <Dialog open>
        <DialogContent>
          <DialogTitle>t</DialogTitle>
          <DialogDescription>d</DialogDescription>
          <RichEditor content="" onChange={() => {}} />
        </DialogContent>
      </Dialog>,
    );
    act(() => screen.getByLabelText("Cuerpo del mensaje").focus());
    await user.keyboard("/");
    const list = await screen.findByRole("listbox", { name: "Insertar" });
    expect(screen.getByRole("dialog")).toContainElement(list);
  });

  it("sin coincidencias muestra Sin resultados", async () => {
    const { user } = await setupEmpty();
    await user.keyboard("/zzz");
    expect(await screen.findByText("Sin resultados")).toBeInTheDocument();
  });

  it("las flechas mueven la selección de forma circular", async () => {
    const { user } = await setupEmpty();
    await user.keyboard("/");
    await screen.findByRole("listbox");
    expect(screen.getByRole("option", { name: "Título 1" })).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{ArrowUp}");
    expect(screen.getByRole("option", { name: "Enlace" })).toHaveAttribute("aria-selected", "true");
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: "Título 1" })).toHaveAttribute("aria-selected", "true");
  });

  it("/titulo 1 + Enter convierte el renglón en h1", async () => {
    const { user, onChange } = await setupEmpty();
    await user.keyboard("/titulo 1");
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(1));
    await user.keyboard("{Enter}");
    await waitFor(() => expect(lastHTML(onChange)).toContain("<h1"));
    expect(lastHTML(onChange)).not.toContain("/titulo");
  });
});

describe("RichEditor: enlace desde el menú y la barra", () => {
  it("/enlace + Enter + URL + Aplicar inserta el enlace como texto", async () => {
    const { user, onChange } = await setupEmpty();
    await user.keyboard("/enlace");
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(1));
    await user.keyboard("{Enter}");
    await user.type(await screen.findByLabelText("URL del enlace"), "ejemplo.com");
    await user.click(screen.getByRole("button", { name: "Aplicar" }));
    await waitFor(() => expect(lastHTML(onChange)).toMatch(/href="https:\/\/ejemplo\.com"[^>]*>https:\/\/ejemplo\.com<\/a>/));
  });

  it("al abrir desde la barra de formato carga el enlace actual y limpia errores", async () => {
    const { user } = await setup('<p><a href="https://x.y">hola</a></p>');
    // Primero se deja un error y una URL vieja en el popover.
    await user.click(toolbar().getByRole("button", { name: "Enlace" }));
    const input = await screen.findByLabelText("URL del enlace");
    await user.clear(input);
    await user.type(input, "javascript:x");
    await user.click(screen.getByRole("button", { name: "Aplicar" }));
    await screen.findByRole("alert");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByLabelText("URL del enlace")).toBeNull());
    // Se reabre desde la barra flotante.
    act(() => screen.getByLabelText("Cuerpo del mensaje").focus());
    await user.keyboard("{Control>}a{/Control}");
    const bubble = await screen.findByRole("toolbar", { name: "Formato del texto" });
    await user.click(within(bubble).getByRole("button", { name: "Enlace" }));
    expect(await screen.findByLabelText("URL del enlace")).toHaveValue("https://x.y");
    expect(screen.queryByRole("alert")).toBeNull();
  });
});

describe("RichEditor: tablas", () => {
  // insertTable deja el cursor en la primera celda.
  async function conTabla() {
    const ctx = await setupEmpty();
    await ctx.user.keyboard("/tab");
    await waitFor(() => expect(screen.getAllByRole("option")).toHaveLength(1));
    await ctx.user.keyboard("{Enter}");
    return ctx;
  }

  it("con el cursor en una tabla aparecen Agregar fila y Eliminar tabla", async () => {
    await conTabla();
    expect(await screen.findByRole("button", { name: "Agregar fila" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Agregar columna" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Eliminar fila" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Eliminar columna" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Eliminar tabla" })).toBeInTheDocument();
  });

  it("fuera de una tabla no aparecen las herramientas de tabla", async () => {
    await setup("<p>hola</p>");
    expect(screen.queryByRole("button", { name: "Agregar fila" })).toBeNull();
  });

  it("Agregar fila suma un tr y Eliminar tabla la quita", async () => {
    const { user, onChange } = await conTabla();
    await waitFor(() => expect((lastHTML(onChange) ?? "").match(/<tr/g)).toHaveLength(3));
    await user.click(await screen.findByRole("button", { name: "Agregar fila" }));
    await waitFor(() => expect((lastHTML(onChange) ?? "").match(/<tr/g)).toHaveLength(4));
    await user.click(screen.getByRole("button", { name: "Eliminar tabla" }));
    await waitFor(() => expect(lastHTML(onChange)).not.toContain("<table"));
  });
});

describe("RichEditor: barra de formato", () => {
  it("aparece con texto seleccionado y aplica negrita", async () => {
    const { user, onChange } = await setup("<p>hola</p>");
    const bubble = await screen.findByRole("toolbar", { name: "Formato del texto" });
    for (const name of ["Negrita", "Cursiva", "Subrayado", "Tachado", "Enlace", "Quitar formato"]) {
      expect(within(bubble).getByRole("button", { name })).toBeInTheDocument();
    }
    await user.click(within(bubble).getByRole("button", { name: "Subrayado" }));
    await waitFor(() => expect(lastHTML(onChange)).toContain("<u>"));
  });

  it("Quitar formato limpia las marcas", async () => {
    const { user, onChange } = await setup("<p><strong>hola</strong></p>");
    const bubble = await screen.findByRole("toolbar", { name: "Formato del texto" });
    await user.click(within(bubble).getByRole("button", { name: "Quitar formato" }));
    await waitFor(() => expect(lastHTML(onChange)).toBe("<p>hola</p>"));
  });
});

describe("RichEditor: imágenes", () => {
  const PNG = "data:image/png;base64,iVBORw0KGgo=";

  it("pegar HTML con <img src=https://…> no agrega la imagen", async () => {
    const { onChange, body } = await setupEmpty();
    // jsdom no tiene DataTransfer: alcanza con lo que ProseMirror lee.
    const clip: Record<string, string> = { "text/html": '<p>hola</p><img src="https://x.test/a.png">', "text/plain": "hola" };
    const data = { getData: (t: string) => clip[t] ?? "", types: Object.keys(clip), files: [], items: [] };
    act(() => {
      const ev = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(ev, "clipboardData", { value: data });
      body.dispatchEvent(ev);
    });
    await waitFor(() => expect(lastHTML(onChange)).toContain("hola"));
    expect(lastHTML(onChange)).not.toContain("<img");
  });

  // jsdom no tiene DataTransfer ni FileReader sobre blobs raros: alcanza con lo que lee FileHandler.
  const pasteFiles = (body: HTMLElement, html: string) => {
    const file = new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47])], "captura.png", { type: "image/png" });
    const clip: Record<string, string> = { "text/html": html, "text/plain": "x" };
    const data = { getData: (t: string) => clip[t] ?? "", types: Object.keys(clip), files: [file], items: [] };
    act(() => {
      const ev = new Event("paste", { bubbles: true, cancelable: true });
      Object.defineProperty(ev, "clipboardData", { value: data });
      body.dispatchEvent(ev);
    });
  };

  it("pegar HTML de una planilla con su captura PNG no inserta una imagen aparte", async () => {
    const { onChange, body } = await setupEmpty();
    pasteFiles(body, "<table><tbody><tr><td>uno</td></tr></tbody></table>");
    await waitFor(() => expect(lastHTML(onChange)).toContain("<table"));
    await new Promise((r) => setTimeout(r, 50)); // FileReader es asíncrono
    expect(lastHTML(onChange)).not.toContain("<img");
  });

  it("pegar una captura sin HTML la inserta como imagen", async () => {
    const { onChange, body } = await setupEmpty();
    pasteFiles(body, "");
    await waitFor(() => expect(lastHTML(onChange)).toContain('<img src="data:image/png'));
  });

  it("el contenido con <img src=data:image/png…> se conserva", async () => {
    const onChange = vi.fn();
    render(<RichEditor content={`<p>a</p><img src="${PNG}">`} onChange={onChange} />);
    const body = await screen.findByLabelText("Cuerpo del mensaje");
    expect(body.querySelector("img")?.getAttribute("src")).toBe(PNG);
  });

  it("el botón Imagen llama a onPickImage", async () => {
    const onPickImage = vi.fn();
    const { user } = await setupEmpty({ onPickImage });
    await user.click(toolbar().getByRole("button", { name: "Imagen" }));
    expect(onPickImage).toHaveBeenCalledTimes(1);
  });
});

describe("RichEditor: tamaño del texto", () => {
  it("Grande pone font-size 18px a la selección y Normal lo quita", async () => {
    const { user, onChange } = await setup("<p>hola</p>");
    const combo = toolbar().getByRole("combobox", { name: "Tamaño del texto" });
    expect(combo).toHaveTextContent("Normal");
    await user.click(combo);
    await user.click(await screen.findByRole("option", { name: "Grande" }));
    await waitFor(() => expect(lastHTML(onChange)).toBe('<p><span style="font-size: 18px;">hola</span></p>'));
    await waitFor(() => expect(combo).toHaveTextContent("Grande"));
    await user.click(combo);
    await user.click(await screen.findByRole("option", { name: "Normal" }));
    await waitFor(() => expect(lastHTML(onChange)).toBe("<p>hola</p>"));
  });

  it("el contenido con font-size se conserva", async () => {
    const { onChange } = await setup('<p><span style="font-size: 12px">hola</span></p>');
    await waitFor(() => expect(toolbar().getByRole("combobox", { name: "Tamaño del texto" })).toHaveTextContent("Chico"));
    expect(onChange).not.toHaveBeenCalled();
  });
});

import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { RichEditor } from "./RichEditor";

// jsdom no calcula layout: ProseMirror pide rectángulos al mover la selección.
beforeAll(() => {
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

const lastHTML = (onChange: ReturnType<typeof vi.fn>) => {
  const calls = onChange.mock.calls;
  return calls.length ? (calls[calls.length - 1][0] as string) : undefined;
};

describe("RichEditor: enlaces", () => {
  it("aplica un enlace desde el popover y agrega https:// si falta", async () => {
    const { user, onChange } = await setup("<p>hola</p>");
    await user.click(screen.getByRole("button", { name: "Enlace" }));
    const input = await screen.findByLabelText("URL del enlace");
    await user.type(input, "ejemplo.com");
    await user.click(screen.getByRole("button", { name: "Aplicar" }));
    await waitFor(() => expect(lastHTML(onChange)).toContain('href="https://ejemplo.com"'));
    expect(screen.queryByLabelText("URL del enlace")).toBeNull();
  });

  it("rechaza esquemas no permitidos sin tocar el texto", async () => {
    const { user, onChange } = await setup("<p>hola</p>");
    await user.click(screen.getByRole("button", { name: "Enlace" }));
    await user.type(await screen.findByLabelText("URL del enlace"), "javascript:alert(1)");
    await user.click(screen.getByRole("button", { name: "Aplicar" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Solo se permiten enlaces http, https o mailto.",
    );
    expect(lastHTML(onChange) ?? "").not.toContain("<a");
  });

  it("muestra el enlace actual y lo quita", async () => {
    const { user, onChange } = await setup('<p><a href="https://x.y">hola</a></p>');
    await user.click(screen.getByRole("button", { name: "Enlace" }));
    expect(await screen.findByLabelText("URL del enlace")).toHaveValue("https://x.y");
    await user.click(screen.getByRole("button", { name: "Quitar enlace" }));
    await waitFor(() => expect(lastHTML(onChange)).toBe("<p>hola</p>"));
  });
});

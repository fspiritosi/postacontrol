import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { SearchBox } from "./SearchBox";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
});

function setup(active: string | null = null) {
  const onSearch = vi.fn();
  render(<SearchBox active={active} onSearch={onSearch} />);
  return { onSearch, user: userEvent.setup() };
}

describe("SearchBox: filtros", () => {
  it("arma la búsqueda con los filtros elegidos y busca enseguida", async () => {
    const { onSearch, user } = setup();
    await user.type(screen.getByLabelText("Buscar correo"), "presupuesto");
    await user.click(screen.getByRole("button", { name: "Filtros de búsqueda" }));
    await user.type(screen.getByLabelText("De"), "Ana López");
    await user.click(screen.getByLabelText("No leídos"));
    await user.click(screen.getByLabelText("Con adjuntos"));
    await user.click(screen.getByRole("button", { name: "Buscar" }));
    expect(onSearch).toHaveBeenLastCalledWith('presupuesto de:"Ana López" tiene:adjunto es:no-leído');
    expect(screen.getByLabelText("Buscar correo")).toHaveValue('presupuesto de:"Ana López" tiene:adjunto es:no-leído');
  });

  it("muestra los filtros que ya tiene la búsqueda y cuántos son", async () => {
    const { user } = setup("hola de:beto es:destacado");
    const btn = screen.getByRole("button", { name: "Filtros de búsqueda" });
    expect(btn).toHaveTextContent("2");
    await user.click(btn);
    expect(screen.getByLabelText("De")).toHaveValue("beto");
    expect(screen.getByLabelText("Destacados")).toBeChecked();
  });

  it("Quitar filtros deja solo el texto", async () => {
    const { onSearch, user } = setup("hola de:beto es:destacado");
    await user.click(screen.getByRole("button", { name: "Filtros de búsqueda" }));
    await user.click(screen.getByRole("button", { name: "Quitar filtros" }));
    expect(onSearch).toHaveBeenLastCalledWith("hola");
  });
});

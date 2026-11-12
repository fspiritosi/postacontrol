import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { PasswordInput } from "./password-input";

function setup(props: { disabled?: boolean } = {}) {
  render(
    <>
      <label htmlFor="p">Contraseña</label>
      <PasswordInput id="p" defaultValue="secreta" {...props} />
    </>,
  );
  return { user: userEvent.setup(), input: screen.getByLabelText("Contraseña") };
}

describe("PasswordInput", () => {
  it("arranca oculta", () => {
    const { input } = setup();
    expect(input).toHaveAttribute("type", "password");
    expect(screen.getByRole("button", { name: "Mostrar contraseña" })).toHaveAttribute("aria-pressed", "false");
  });

  it("el ojo la muestra y la vuelve a ocultar, sin perder lo escrito", async () => {
    const { user, input } = setup();
    await user.click(screen.getByRole("button", { name: "Mostrar contraseña" }));
    expect(input).toHaveAttribute("type", "text");
    expect(input).toHaveValue("secreta");
    const hide = screen.getByRole("button", { name: "Ocultar contraseña" });
    expect(hide).toHaveAttribute("aria-pressed", "true");
    await user.click(hide);
    expect(input).toHaveAttribute("type", "password");
  });

  it("el ojo no envía el formulario", async () => {
    let submitted = false;
    render(
      <form onSubmit={(e) => { e.preventDefault(); submitted = true; }}>
        <PasswordInput aria-label="clave" />
      </form>,
    );
    await userEvent.setup().click(screen.getByRole("button", { name: "Mostrar contraseña" }));
    expect(submitted).toBe(false);
  });

  it("deshabilitado, el ojo también", () => {
    setup({ disabled: true });
    expect(screen.getByRole("button", { name: "Mostrar contraseña" })).toBeDisabled();
  });
});

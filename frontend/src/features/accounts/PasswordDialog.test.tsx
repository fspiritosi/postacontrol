import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { PasswordDialog } from "./PasswordDialog";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({ UpdatePassword: vi.fn() }));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { toast } from "sonner";

const account = { id: 7, email: "yo@ejemplo.com" } as never;

function setup() {
  const onOpenChange = vi.fn();
  render(<PasswordDialog account={account} open onOpenChange={onOpenChange} />);
  return { user: userEvent.setup(), onOpenChange };
}

describe("PasswordDialog", () => {
  beforeEach(() => vi.clearAllMocks());

  it("guarda y cierra", async () => {
    vi.mocked(svc.UpdatePassword).mockResolvedValue(undefined as never);
    const { user, onOpenChange } = setup();
    expect(screen.getByText("yo@ejemplo.com")).toBeInTheDocument();
    await user.type(screen.getByLabelText("Contraseña"), "nueva");
    await user.click(screen.getByRole("button", { name: "Probar y guardar" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(svc.UpdatePassword).toHaveBeenCalledWith(7, "nueva");
    expect(toast.success).toHaveBeenCalledWith("Contraseña actualizada");
  });

  it("muestra el error sin cerrar", async () => {
    vi.mocked(svc.UpdatePassword).mockRejectedValue(
      new Error("El servidor rechazó el usuario o la contraseña."),
    );
    const { user, onOpenChange } = setup();
    await user.type(screen.getByLabelText("Contraseña"), "mala");
    await user.click(screen.getByRole("button", { name: "Probar y guardar" }));
    expect(
      await screen.findByText("El servidor rechazó el usuario o la contraseña."),
    ).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(toast.success).not.toHaveBeenCalled();
  });
});

import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { Button } from "@/components/ui/button";
import { WhenMenu } from "./WhenMenu";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
});

describe("WhenMenu: elegir fecha y hora", () => {
  it("se puede cambiar el día, la hora y los minutos", async () => {
    const onPick = vi.fn();
    const user = userEvent.setup();
    render(<WhenMenu title="Programar envío" onPick={onPick} trigger={<Button>Programar</Button>} />);
    await user.click(screen.getByRole("button", { name: "Programar" }));
    await user.click(await screen.findByRole("menuitem", { name: "Elegir fecha y hora…" }));
    const day = await screen.findByLabelText("Día");
    await user.selectOptions(day, "2");
    await user.selectOptions(screen.getByLabelText("Hora"), "9");
    await user.selectOptions(screen.getByLabelText("Minutos"), "45");
    await user.click(screen.getByRole("button", { name: "Listo" }));

    const want = new Date();
    want.setDate(want.getDate() + 2);
    want.setHours(9, 45, 0, 0);
    expect(onPick).toHaveBeenCalledWith(want);
    await waitFor(() => expect(screen.queryByLabelText("Día")).toBeNull());
  });

  it("una hora que ya pasó no se puede elegir", async () => {
    const user = userEvent.setup();
    render(<WhenMenu title="Programar envío" onPick={vi.fn()} trigger={<Button>Programar</Button>} />);
    await user.click(screen.getByRole("button", { name: "Programar" }));
    await user.click(await screen.findByRole("menuitem", { name: "Elegir fecha y hora…" }));
    await user.selectOptions(await screen.findByLabelText("Día"), "0");
    await user.selectOptions(screen.getByLabelText("Hora"), "0");
    await user.selectOptions(screen.getByLabelText("Minutos"), "0");
    expect(screen.getByText("Elegí una hora que todavía no pasó.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Listo" })).toBeDisabled();
  });
});

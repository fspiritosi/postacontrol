import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  SaveLocalTask: vi.fn(),
  GetSettings: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { localTask, SETTINGS } from "./fixtures";
import { LocalTaskDialog } from "./LocalTaskDialog";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.scrollIntoView = () => {};
});

function setup(task: ReturnType<typeof localTask> | null = null) {
  const user = userEvent.setup();
  const onSaved = vi.fn();
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocalTaskDialog open onOpenChange={onOpenChange} task={task as never} onSaved={onSaved} />
    </QueryClientProvider>,
  );
  return { user, onSaved, onOpenChange };
}

describe("LocalTaskDialog", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.GetSettings).mockResolvedValue({ ...SETTINGS, taskReminderMinutes: 30 } as never);
  });

  it("la hora está deshabilitada sin fecha y el aviso sin hora", async () => {
    setup();
    const time = screen.getByLabelText("Hora");
    expect(time).toBeDisabled();
    expect(screen.getByRole("combobox", { name: "Aviso" })).toBeDisabled();
    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: "2026-11-20" } });
    expect(time).toBeEnabled();
    expect(screen.getByRole("combobox", { name: "Aviso" })).toBeDisabled();
    fireEvent.change(time, { target: { value: "10:00" } });
    expect(screen.getByRole("combobox", { name: "Aviso" })).toBeEnabled();
  });

  it("muestra el aviso predeterminado con los minutos de Configuración", async () => {
    setup();
    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: "2026-11-20" } });
    fireEvent.change(screen.getByLabelText("Hora"), { target: { value: "10:00" } });
    expect(await screen.findByText("Predeterminado (30 min)")).toBeInTheDocument();
  });

  it("al guardar con Predeterminado manda remindBefore null", async () => {
    vi.mocked(svc.SaveLocalTask).mockResolvedValue(localTask({ id: 7 }) as never);
    const { user, onSaved } = setup();
    await user.type(screen.getByLabelText("Título"), "Pagar la luz");
    fireEvent.change(screen.getByLabelText("Fecha"), { target: { value: "2026-11-20" } });
    fireEvent.change(screen.getByLabelText("Hora"), { target: { value: "10:00" } });
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(svc.SaveLocalTask).toHaveBeenCalled());
    expect(vi.mocked(svc.SaveLocalTask).mock.calls[0][0]).toMatchObject({
      id: 0, title: "Pagar la luz", dueDate: "2026-11-20", dueTime: "10:00", remindBefore: null,
    });
    await waitFor(() => expect(onSaved).toHaveBeenCalled());
  });

  it("muestra el error del núcleo", async () => {
    vi.mocked(svc.SaveLocalTask).mockRejectedValue(new Error("título vacío"));
    const { user } = setup();
    await user.type(screen.getByLabelText("Título"), "x");
    await user.click(screen.getByRole("button", { name: "Guardar" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });
});

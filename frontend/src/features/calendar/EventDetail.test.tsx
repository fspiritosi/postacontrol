import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetEvent: vi.fn(),
  RespondInvite: vi.fn(),
  NotifyTaskappChange: vi.fn(),
  DismissTaskappChange: vi.fn(),
  RemoveTaskappExternals: vi.fn(),
  RemoveCancelledEvent: vi.fn(),
  OpenTaskInTaskapp: vi.fn(),
  PreviewEventDelete: vi.fn(),
  DeleteEvent: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), warning: vi.fn(), success: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { calItem, calShellState } from "./fixtures";
import { EventDetail } from "./EventDetail";

function setup(item: ReturnType<typeof calItem>) {
  const user = userEvent.setup();
  const state = calShellState();
  const onEdit = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <EventDetail item={item} state={state} onEdit={onEdit} />
    </QueryClientProvider>,
  );
  return { user, state, onEdit };
}

describe("EventDetail", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.GetEvent).mockResolvedValue({
      id: 1, organizer: { name: "Ana", email: "ana@x.com" }, notes: "", description: "",
      attendees: [{ email: "b@x.com", name: "Beto", partstat: "ACCEPTED" }],
    } as never);
  });

  it("en una invitación, Aceptar responde y muestra el aviso", async () => {
    vi.mocked(svc.RespondInvite).mockResolvedValue({ warning: "No se pudo avisar" });
    const { user } = setup(calItem({ source: "invite", eventId: 7, partstat: "NEEDS-ACTION" }));
    await user.click(screen.getByRole("button", { name: "Aceptar" }));
    await waitFor(() => expect(svc.RespondInvite).toHaveBeenCalledWith(7, "ACCEPTED"));
    await waitFor(() => expect(toast.warning).toHaveBeenCalledWith("No se pudo avisar"));
    await user.click(screen.getByRole("button", { name: "Tal vez" }));
    expect(svc.RespondInvite).toHaveBeenLastCalledWith(7, "TENTATIVE");
  });

  it("una reunión de Taskapp que cambió pregunta si avisar", async () => {
    vi.mocked(svc.NotifyTaskappChange).mockResolvedValue();
    vi.mocked(svc.DismissTaskappChange).mockResolvedValue();
    const { user } = setup(calItem({ source: "taskapp", eventId: 9, taskappChange: "changed", url: "https://t/1" }));
    expect(screen.getByText(/Cambió en Taskapp/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Avisar" }));
    expect(svc.NotifyTaskappChange).toHaveBeenCalledWith(9);
    await user.click(screen.getByRole("button", { name: "Ahora no" }));
    expect(svc.DismissTaskappChange).toHaveBeenCalledWith(9);
  });

  it("en una tarea, Abrir tarea la abre", async () => {
    const { user, state } = setup(calItem({ source: "task", taskId: 4, eventId: 0 }));
    await user.click(screen.getByRole("button", { name: "Abrir tarea" }));
    expect(state.openTask).toHaveBeenCalledWith(4);
  });

  it("borrar un evento repetido pide el alcance y luego el aviso", async () => {
    vi.mocked(svc.PreviewEventDelete).mockResolvedValue({ notifyNeeded: true, added: 0, removed: 1 });
    vi.mocked(svc.DeleteEvent).mockResolvedValue();
    const { user } = setup(calItem({ recurring: true, attendees: 1 }));
    await user.click(screen.getByRole("button", { name: "Borrar" }));
    await user.click(await screen.findByRole("button", { name: "Solo este" }));
    await user.click(await screen.findByRole("button", { name: "Avisar" }));
    await waitFor(() =>
      expect(svc.DeleteEvent).toHaveBeenCalledWith({
        id: 1, scope: "this", occurrenceAt: "2026-11-20T13:00:00Z", notify: true,
      }),
    );
  });

  it("si el borrado quedó hecho y falló el correo, avisa y suelta la selección", async () => {
    vi.mocked(svc.PreviewEventDelete).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    vi.mocked(svc.DeleteEvent).mockRejectedValue(new Error("El evento se borró, pero no pudimos mandar las cancelaciones: x"));
    const { user, state } = setup(calItem());
    await user.click(screen.getByRole("button", { name: "Borrar" }));
    await waitFor(() => expect(state.selectEvent).toHaveBeenCalledWith(null));
    expect(toast.warning).toHaveBeenCalledWith(expect.stringContaining("pero no pudimos mandar"));
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("local: muestra los invitados con su estado y Editar llama al callback", async () => {
    const { user, onEdit } = setup(
      calItem({ attendees: 1, confirmed: 1, people: [{ email: "b@x.com", name: "Beto", partstat: "ACCEPTED" }] }),
    );
    expect(await screen.findByText("Beto")).toBeInTheDocument();
    expect(screen.getByText("confirmó")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Editar" }));
    expect(onEdit).toHaveBeenCalled();
  });

  it("una invitación lista a los invitados con su estado", () => {
    setup(
      calItem({
        source: "invite", attendees: 2, confirmed: 0,
        people: [
          { email: "c@x.com", name: "", partstat: "DECLINED" },
          { email: "d@x.com", name: "Dora", partstat: "TENTATIVE" },
        ],
      }),
    );
    expect(screen.getByText("c@x.com")).toBeInTheDocument();
    expect(screen.getByText("rechazó")).toBeInTheDocument();
    expect(screen.getByText("Dora")).toBeInTheDocument();
    expect(screen.getByText("tal vez")).toBeInTheDocument();
  });

  it("una reunión de Taskapp muestra a los asistentes y aclara que el conteo es de externos", () => {
    setup(
      calItem({
        source: "taskapp", eventId: 9, attendees: 1, confirmed: 0,
        people: [
          { email: "ana@cc.com", name: "Ana", partstat: "ACCEPTED" },
          { email: "bea@cliente.com", name: "Bea", partstat: "NEEDS-ACTION" },
        ],
      }),
    );
    expect(screen.getByText("Ana")).toBeInTheDocument();
    expect(screen.getByText("confirmó")).toBeInTheDocument();
    expect(screen.getByText("Bea")).toBeInTheDocument();
    expect(screen.getByText("sin responder")).toBeInTheDocument();
    expect(screen.getByText("Invitados externos: 1 · 0 confirmaron")).toBeInTheDocument();
    expect(svc.GetEvent).not.toHaveBeenCalled();
  });

  it("Quitar invitados externos pide confirmación antes de mandar la cancelación", async () => {
    vi.mocked(svc.RemoveTaskappExternals).mockResolvedValue({ warning: "La reunión sigue en Taskapp" });
    const { user } = setup(calItem({ source: "taskapp", eventId: 9, attendees: 1 }));
    await user.click(screen.getByRole("button", { name: "Quitar invitados externos" }));
    expect(
      await screen.findByText("Se les va a mandar una cancelación a los invitados externos. La reunión sigue en Taskapp."),
    ).toBeInTheDocument();
    expect(svc.RemoveTaskappExternals).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    expect(svc.RemoveTaskappExternals).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Quitar invitados externos" }));
    await user.click(await screen.findByRole("button", { name: "Quitar" }));
    await waitFor(() => expect(svc.RemoveTaskappExternals).toHaveBeenCalledWith(9));
  });
});

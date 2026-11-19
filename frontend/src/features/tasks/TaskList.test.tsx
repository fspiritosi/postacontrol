import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLocalTasks: vi.fn(),
  ListTaskappTasks: vi.fn(),
  TaskappStatus: vi.fn(),
  SetLocalTaskDone: vi.fn(),
  GetSettings: vi.fn(),
  SaveLocalTask: vi.fn(),
}));
const requestSettings = vi.fn();
vi.mock("@/lib/events", async (orig) => ({ ...(await orig<object>()), requestSettings: (s?: string) => requestSettings(s) }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { localTask, remoteTask, SETTINGS, taskShellState } from "./fixtures";
import { TaskList } from "./TaskList";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.scrollIntoView = () => {};
});

function ymd(d: Date) {
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}
const today = ymd(new Date());

function setup(state = taskShellState()) {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <TaskList state={state} />
    </QueryClientProvider>,
  );
  return { user, state };
}

describe("TaskList", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.mocked(svc.GetSettings).mockResolvedValue(SETTINGS as never);
    vi.mocked(svc.TaskappStatus).mockResolvedValue({ connected: true, userId: 0, userName: "Ana", userEmail: "a@b.c" });
    vi.mocked(svc.ListLocalTasks).mockResolvedValue([localTask({ dueDate: today })] as never);
    vi.mocked(svc.ListTaskappTasks).mockResolvedValue({
      tasks: [remoteTask({ dueDate: today })], fetchedAt: new Date().toISOString(), stale: false, error: "",
    } as never);
  });

  it("muestra los grupos y el origen de cada tarea", async () => {
    setup();
    expect(await screen.findByText("Hoy")).toBeInTheDocument();
    expect(screen.getByText("Llamar al banco")).toBeInTheDocument();
    expect(screen.getByText("Local")).toBeInTheDocument();
    expect(await screen.findByText("Arreglar login")).toBeInTheDocument();
    expect(screen.getByText("Web")).toBeInTheDocument();
    expect(screen.getByText("Ticket #5")).toBeInTheDocument();
    expect(screen.getByText("En curso")).toBeInTheDocument();
  });

  it("cada fila de Taskapp muestra su estado y distingue tarea de ticket", async () => {
    vi.mocked(svc.ListTaskappTasks).mockResolvedValue({
      tasks: [
        remoteTask({ dueDate: today }),
        remoteTask({ id: 9, ticket: false, title: "Revisar factura", statusName: "Pendiente", dueDate: today }),
      ],
      fetchedAt: new Date().toISOString(), stale: false, error: "",
    } as never);
    setup();
    const tarea = (await screen.findByText("Revisar factura")).closest("button")!;
    expect(tarea).toHaveTextContent("#9");
    expect(tarea).not.toHaveTextContent("Ticket");
    expect(tarea).toHaveTextContent("Pendiente");
    const ticket = screen.getByText("Arreglar login").closest("button")!;
    expect(ticket).toHaveTextContent("Ticket #5");
    expect(ticket).toHaveTextContent("En curso");
  });

  it("avisa cuando la lista de Taskapp viene de la caché", async () => {
    vi.mocked(svc.ListTaskappTasks).mockResolvedValue({
      tasks: [remoteTask()], fetchedAt: new Date(Date.now() - 10 * 60_000).toISOString(), stale: true, error: "sin red",
    } as never);
    setup();
    expect(await screen.findByText(/Taskapp sin conexión · actualizado hace 10 min/)).toBeInTheDocument();
  });

  it("sin conectar a Taskapp ofrece conectar y no pide las remotas", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue({ connected: false, userId: 0, userName: "", userEmail: "" });
    const { user } = setup();
    const link = await screen.findByRole("button", { name: /Conectá Taskapp para ver tus tareas/ });
    await screen.findByText("Llamar al banco");
    expect(svc.ListTaskappTasks).not.toHaveBeenCalled();
    await user.click(link);
    expect(requestSettings).toHaveBeenCalledWith("taskapp");
  });

  it("marcar el círculo de una local llama a SetLocalTaskDone", async () => {
    vi.mocked(svc.SetLocalTaskDone).mockResolvedValue();
    const { user } = setup();
    await user.click(await screen.findByRole("button", { name: "Marcar como hecha: Llamar al banco" }));
    expect(svc.SetLocalTaskDone).toHaveBeenCalledWith(1, true);
  });

  it("Mostrar hechas pide las hechas y las lista aparte", async () => {
    vi.mocked(svc.ListLocalTasks).mockImplementation((inc: boolean) =>
      Promise.resolve(inc
        ? [localTask(), localTask({ id: 2, title: "Vieja", doneAt: "2026-11-01T10:00:00Z" })]
        : [localTask()]) as never);
    const { user } = setup();
    await screen.findByText("Llamar al banco");
    expect(screen.queryByText("Hechas")).not.toBeInTheDocument();
    await user.click(screen.getByRole("switch", { name: "Mostrar hechas" }));
    expect(await screen.findByText("Hechas")).toBeInTheDocument();
    expect(svc.ListLocalTasks).toHaveBeenCalledWith(true);
    await user.click(screen.getByRole("button", { name: "Marcar como pendiente: Vieja" }));
    expect(svc.SetLocalTaskDone).toHaveBeenCalledWith(2, false);
  });

  it("sin tareas dice que no hay pendientes", async () => {
    vi.mocked(svc.ListLocalTasks).mockResolvedValue([]);
    vi.mocked(svc.ListTaskappTasks).mockResolvedValue({ tasks: [], fetchedAt: "", stale: false, error: "" } as never);
    setup();
    expect(await screen.findByText("No tenés tareas pendientes.")).toBeInTheDocument();
  });

  it("elegir una fila la abre", async () => {
    const { user, state } = setup();
    await user.click(await screen.findByText("Llamar al banco"));
    expect(state.selectTask).toHaveBeenCalledWith("l:1");
  });

  it("el filtro de origen se recuerda y oculta las de Taskapp", async () => {
    localStorage.setItem("pc:tasks-source", "local");
    setup();
    await screen.findByText("Llamar al banco");
    await waitFor(() => expect(svc.TaskappStatus).toHaveBeenCalled());
    expect(screen.queryByText("Arreglar login")).not.toBeInTheDocument();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListLocalTasks: vi.fn(),
  ListTaskappTasks: vi.fn(),
  TaskappStatus: vi.fn(),
  SetLocalTaskDone: vi.fn(),
  DeleteLocalTask: vi.fn(),
  SaveLocalTask: vi.fn(),
  GetSettings: vi.fn(),
  OpenTaskInTaskapp: vi.fn(),
  FindMessageByHeader: vi.fn(),
}));
const requestOpenMessage = vi.fn();
vi.mock("@/lib/events", async (orig) => ({ ...(await orig<object>()), requestOpenMessage: (r: unknown) => requestOpenMessage(r) }));
vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { localTask, remoteTask, SETTINGS, taskShellState } from "./fixtures";
import { TaskPanel } from "./TaskPanel";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.scrollIntoView = () => {};
});

function setup(key: string | null) {
  const user = userEvent.setup();
  const state = taskShellState(key);
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <TaskPanel state={state} />
    </QueryClientProvider>,
  );
  return { user, state };
}

describe("TaskPanel", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.GetSettings).mockResolvedValue(SETTINGS as never);
    vi.mocked(svc.OpenTaskInTaskapp).mockResolvedValue();
    vi.mocked(svc.TaskappStatus).mockResolvedValue({ connected: true, userId: 0, userName: "", userEmail: "" });
    vi.mocked(svc.ListTaskappTasks).mockResolvedValue({
      tasks: [remoteTask()], fetchedAt: "", stale: false, error: "",
    } as never);
    vi.mocked(svc.ListLocalTasks).mockResolvedValue([
      localTask({ dueDate: "2026-11-20", dueTime: "09:30", notes: "Pedir turno", sourceMessageId: "<m@x>", sourceSubject: "Turno" }),
    ] as never);
  });

  it("una de Taskapp muestra Abrir en Taskapp y no Editar", async () => {
    const { user } = setup("t:5");
    expect(await screen.findByText("Arreglar login")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Editar" })).not.toBeInTheDocument();
    expect(screen.getByText("En curso")).toBeInTheDocument();
    expect(screen.getByText("Web")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Abrir en Taskapp/ }));
    expect(svc.OpenTaskInTaskapp).toHaveBeenCalledWith("https://taskapp.test/t/5");
  });

  it("una local muestra notas y botones Hecha, Editar y Borrar", async () => {
    const { user } = setup("l:1");
    expect(await screen.findByText("Pedir turno")).toBeInTheDocument();
    expect(screen.getByText(/09:30/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Editar" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hecha" }));
    expect(svc.SetLocalTaskDone).toHaveBeenCalledWith(1, true);
  });

  it("Borrar pide confirmación", async () => {
    vi.mocked(svc.DeleteLocalTask).mockResolvedValue();
    const { user, state } = setup("l:1");
    await user.click(await screen.findByRole("button", { name: "Borrar" }));
    expect(svc.DeleteLocalTask).not.toHaveBeenCalled();
    await user.click(await screen.findByRole("button", { name: "Borrar tarea" }));
    await waitFor(() => expect(svc.DeleteLocalTask).toHaveBeenCalledWith(1));
    expect(state.selectTask).toHaveBeenCalledWith(null);
  });

  it("Correo de origen abre el correo en su carpeta", async () => {
    vi.mocked(svc.FindMessageByHeader).mockResolvedValue({ id: 9, folderId: 4 } as never);
    const { user } = setup("l:1");
    await user.click(await screen.findByRole("button", { name: /Correo de origen/ }));
    expect(svc.FindMessageByHeader).toHaveBeenCalledWith("<m@x>");
    await waitFor(() => expect(requestOpenMessage).toHaveBeenCalledWith({ folderId: 4, messageId: 9 }));
  });

  it("si el correo no está muestra el error del núcleo", async () => {
    vi.mocked(svc.FindMessageByHeader).mockRejectedValue(new Error("ese correo ya no está"));
    const { user } = setup("l:1");
    await user.click(await screen.findByRole("button", { name: /Correo de origen/ }));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });
});

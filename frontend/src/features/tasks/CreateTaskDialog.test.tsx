import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  TaskappStatus: vi.fn(),
  TaskappDraftFor: vi.fn(),
  TaskappProjects: vi.fn(),
  CreateTaskappItem: vi.fn(),
  RetryTaskLinkAttachment: vi.fn(),
  CreateLocalTaskFrom: vi.fn(),
  OpenTaskInTaskapp: vi.fn(),
  GetSettings: vi.fn(),
  SaveLocalTask: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { SETTINGS } from "./fixtures";
import { CreateTaskDialog } from "./CreateTaskDialog";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.scrollIntoView = () => {};
});

const draft = (over = {}) => ({ messageId: 9, title: "Re: Presupuesto", descriptionHtml: "<p>Hola</p>", projectId: 2, oversize: false, ...over });
const link = (over = {}) => ({
  id: 1, threadRoot: "r", taskId: 123, ticket: false, projectName: "Sinergia", title: "Re: Presupuesto",
  attachPending: false, createdAt: "", url: "https://t/123", alreadyAdded: true, completed: false, stateKnown: true, ...over,
});

function setup(connected = true, draftOver = {}) {
  vi.mocked(svc.TaskappStatus).mockResolvedValue({ connected, userName: "", userEmail: "" });
  vi.mocked(svc.TaskappDraftFor).mockResolvedValue(draft(draftOver));
  vi.mocked(svc.TaskappProjects).mockResolvedValue([
    { id: 1, workspaceId: 1, name: "Web", color: "#f00" },
    { id: 2, workspaceId: 1, name: "Sinergia", color: "#0f0" },
  ]);
  vi.mocked(svc.GetSettings).mockResolvedValue(SETTINGS as never);
  vi.mocked(svc.CreateLocalTaskFrom).mockResolvedValue({
    id: 0, title: "Local desde correo", notes: "", dueDate: "", dueTime: "", remindBefore: null,
    sourceMessageId: "m", sourceSubject: "", doneAt: null, createdAt: "",
  });
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const spy = vi.spyOn(qc, "invalidateQueries");
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <CreateTaskDialog messageId={9} open onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), spy, onOpenChange };
}

describe("CreateTaskDialog", () => {
  beforeEach(() => vi.clearAllMocks());

  it("precarga el título y el proyecto del borrador", async () => {
    setup();
    expect(await screen.findByDisplayValue("Re: Presupuesto")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Proyecto" })).toHaveTextContent("Sinergia");
    expect(screen.getByLabelText("Descripción (HTML)")).toHaveValue("<p>Hola</p>");
    expect(screen.getByRole("button", { name: "Crear" })).toBeEnabled();
  });

  it("Crear está deshabilitado con Tarea sin proyecto y habilitado con Ticket", async () => {
    const { user } = setup(true, { projectId: 0 });
    await screen.findByDisplayValue("Re: Presupuesto");
    expect(screen.getByRole("button", { name: "Crear" })).toBeDisabled();
    expect(screen.getByText("Elegí un proyecto para la tarea.")).toBeInTheDocument();
    expect(screen.getByLabelText("Vence")).toBeInTheDocument();
    await user.click(screen.getByLabelText("Ticket en Taskapp"));
    expect(screen.getByRole("button", { name: "Crear" })).toBeEnabled();
    expect(screen.queryByLabelText("Vence")).not.toBeInTheDocument();
  });

  it("crea la tarea, avisa y refresca los vínculos", async () => {
    vi.mocked(svc.CreateTaskappItem).mockResolvedValue(link());
    const { user, spy, onOpenChange } = setup();
    await screen.findByDisplayValue("Re: Presupuesto");
    await user.click(screen.getByRole("button", { name: "Crear" }));
    await waitFor(() => expect(svc.CreateTaskappItem).toHaveBeenCalledWith(
      expect.objectContaining({ messageId: 9, ticket: false, projectId: 2, title: "Re: Presupuesto", priority: "none" }),
    ));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Tarea #123 creada en Sinergia", expect.anything()));
    expect(spy).toHaveBeenCalledWith({ queryKey: ["task-links"] });
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("attachPending muestra el toast de reintento", async () => {
    vi.mocked(svc.CreateTaskappItem).mockResolvedValue(link({ attachPending: true }));
    vi.mocked(svc.RetryTaskLinkAttachment).mockResolvedValue(link());
    const { user } = setup();
    await screen.findByDisplayValue("Re: Presupuesto");
    await user.click(screen.getByRole("button", { name: "Crear" }));
    await waitFor(() => expect(toast.warning).toHaveBeenCalled());
    const [msg, opts] = vi.mocked(toast.warning).mock.calls[0] as unknown as [string, { action: { label: string; onClick: () => void } }];
    expect(msg).toBe("La tarea #123 se creó, pero no se pudo adjuntar el correo.");
    expect(opts.action.label).toBe("Reintentar");
    opts.action.onClick();
    expect(svc.RetryTaskLinkAttachment).toHaveBeenCalledWith(1);
  });

  it("un error de huérfana queda fijo en pantalla y cierra el diálogo", async () => {
    const msg = "La tarea #5 se creó en Taskapp, pero no se pudo guardar el vínculo en esta PC. Abrila desde Taskapp; no la vuelvas a crear.";
    vi.mocked(svc.CreateTaskappItem).mockRejectedValue(new Error(msg));
    const { user, onOpenChange, spy } = setup();
    await screen.findByDisplayValue("Re: Presupuesto");
    await user.click(screen.getByRole("button", { name: "Crear" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith(msg, { duration: Infinity }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(spy).toHaveBeenCalledWith({ queryKey: ["task-links"] });
  });

  it("sin conexión solo ofrece Tarea local", async () => {
    const { user } = setup(false);
    expect(await screen.findByLabelText("Tarea local")).toBeInTheDocument();
    expect(screen.queryByLabelText("Tarea en Taskapp")).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Ticket en Taskapp")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Conectar Taskapp" })).toBeInTheDocument();
    await user.click(screen.getByLabelText("Tarea local"));
    expect(await screen.findByDisplayValue("Local desde correo")).toBeInTheDocument();
    expect(svc.CreateLocalTaskFrom).toHaveBeenCalledWith(9);
  });
});

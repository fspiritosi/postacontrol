import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  TaskLinksForMessage: vi.fn(),
  AddMessageToTask: vi.fn(),
  RetryTaskLinkAttachment: vi.fn(),
  OpenTaskInTaskapp: vi.fn(),
  TaskappStatus: vi.fn(),
  TaskappDraftFor: vi.fn(),
  TaskappProjects: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
const requestSettings = vi.fn();
vi.mock("@/lib/events", async (orig) => ({ ...(await orig<object>()), requestSettings: (s?: string) => requestSettings(s) }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { TaskChips } from "./TaskChips";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.scrollIntoView = () => {};
});

const link = (over = {}) => ({
  id: 1, threadRoot: "r", taskId: 123, ticket: false, projectName: "Sinergia", title: "T",
  attachPending: false, createdAt: "", url: "https://t/123", alreadyAdded: false, completed: false, stateKnown: true, ...over,
});

function setup(links: unknown[], connected = true) {
  vi.mocked(svc.TaskLinksForMessage).mockResolvedValue(links as never);
  vi.mocked(svc.OpenTaskInTaskapp).mockResolvedValue(undefined);
  vi.mocked(svc.TaskappStatus).mockResolvedValue({ connected, userName: "", userEmail: "" });
  vi.mocked(svc.TaskappDraftFor).mockResolvedValue({ messageId: 9, title: "Nuevo", descriptionHtml: "", projectId: 0, oversize: false });
  vi.mocked(svc.TaskappProjects).mockResolvedValue([]);
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const spy = vi.spyOn(qc, "invalidateQueries");
  render(
    <QueryClientProvider client={qc}>
      <TaskChips messageId={9} />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), spy };
}

describe("TaskChips", () => {
  beforeEach(() => vi.clearAllMocks());

  it("muestra el chip y Sumar, y el chip abre la tarea", async () => {
    const { user } = setup([link()]);
    const chip = await screen.findByRole("button", { name: "Tarea #123 · Sinergia" });
    expect(screen.getByRole("button", { name: "Sumar a la tarea #123" })).toBeEnabled();
    await user.click(chip);
    expect(svc.OpenTaskInTaskapp).toHaveBeenCalledWith("https://t/123");
  });

  it("sin vínculos no muestra nada", async () => {
    setup([]);
    await waitFor(() => expect(svc.TaskLinksForMessage).toHaveBeenCalled());
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("con la tarea completada, Sumar queda deshabilitado y ofrece Crear tarea nueva", async () => {
    const { user } = setup([link({ completed: true })]);
    expect(await screen.findByRole("button", { name: "Sumar a la tarea #123" })).toBeDisabled();
    expect(screen.getByTitle("La tarea #123 está completada")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Crear tarea nueva" }));
    expect(await screen.findByDisplayValue("Nuevo")).toBeInTheDocument();
  });

  it("si ya se sumó no ofrece Sumar", async () => {
    setup([link({ alreadyAdded: true })]);
    await screen.findByRole("button", { name: "Tarea #123 · Sinergia" });
    expect(screen.queryByRole("button", { name: /Sumar/ })).not.toBeInTheDocument();
  });

  it("después de sumar se invalida task-links", async () => {
    vi.mocked(svc.AddMessageToTask).mockResolvedValue(undefined);
    const { user, spy } = setup([link()]);
    await user.click(await screen.findByRole("button", { name: "Sumar a la tarea #123" }));
    await waitFor(() => expect(svc.AddMessageToTask).toHaveBeenCalledWith(1, 9));
    await waitFor(() => expect(spy).toHaveBeenCalledWith({ queryKey: ["task-links"] }));
  });

  it("con varios vínculos Sumar a… abre un menú para elegir", async () => {
    vi.mocked(svc.AddMessageToTask).mockResolvedValue(undefined);
    const { user } = setup([link(), link({ id: 2, taskId: 7, ticket: true, projectName: "" })]);
    await user.click(await screen.findByRole("button", { name: "Sumar a…" }));
    await user.click(await screen.findByRole("menuitem", { name: /Ticket #7/ }));
    await waitFor(() => expect(svc.AddMessageToTask).toHaveBeenCalledWith(2, 9));
  });

  it("con adjunto pendiente ofrece Reintentar adjunto", async () => {
    vi.mocked(svc.RetryTaskLinkAttachment).mockResolvedValue(link() as never);
    const { user } = setup([link({ attachPending: true, alreadyAdded: true })]);
    await user.click(await screen.findByRole("button", { name: "Adjunto pendiente" }));
    await user.click(await screen.findByRole("menuitem", { name: "Reintentar adjunto" }));
    expect(svc.RetryTaskLinkAttachment).toHaveBeenCalledWith(1);
  });

  it("sin Taskapp conectada no ofrece Sumar ni Reintentar, y lleva a conectar", async () => {
    const { user } = setup([link({ attachPending: true })], false);
    const chip = await screen.findByRole("button", { name: "Tarea #123 · Sinergia" });
    const conectar = await screen.findByRole("button", { name: "Conectá Taskapp" });
    expect(screen.getByRole("button", { name: "Sumar a la tarea #123" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Adjunto pendiente" })).toBeDisabled();
    await user.click(conectar);
    expect(requestSettings).toHaveBeenCalledWith("taskapp");
    await user.click(chip);
    expect(svc.OpenTaskInTaskapp).toHaveBeenCalledWith("https://t/123");
    expect(svc.AddMessageToTask).not.toHaveBeenCalled();
    expect(svc.RetryTaskLinkAttachment).not.toHaveBeenCalled();
  });

  it("sin Taskapp conectada, con varios vínculos Sumar a… queda deshabilitado", async () => {
    setup([link(), link({ id: 2, taskId: 7, ticket: true, projectName: "" })], false);
    await screen.findByRole("button", { name: "Conectá Taskapp" });
    expect(screen.getByRole("button", { name: "Sumar a…" })).toBeDisabled();
  });

  it("conectada no muestra el enlace para conectar", async () => {
    setup([link()]);
    await screen.findByRole("button", { name: "Tarea #123 · Sinergia" });
    await waitFor(() => expect(svc.TaskappStatus).toHaveBeenCalled());
    expect(screen.queryByRole("button", { name: "Conectá Taskapp" })).not.toBeInTheDocument();
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetEvent: vi.fn(),
  SaveEvent: vi.fn(),
  PreviewEventSave: vi.fn(),
  DeleteEvent: vi.fn(),
  PreviewEventDelete: vi.fn(),
  CreateTaskappMeeting: vi.fn(),
  TaskappProjects: vi.fn(),
  TaskappStatus: vi.fn(),
  TaskappWorkspaces: vi.fn(),
  TaskappMembers: vi.fn(),
  ListAccounts: vi.fn(),
  SuggestRecipients: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), warning: vi.fn(), success: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { EventDialog, type EventInitial } from "./EventDialog";

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.scrollIntoView = () => {};
});

const EVENT = {
  id: 1, uid: "u", kind: "local", title: "Reunión", description: "", notes: "", location: "", allDay: false,
  start: "2026-11-20T13:00:00Z", end: "2026-11-20T14:00:00Z", tzid: "", rrule: "", remindBefore: null, accountId: 2,
  organizer: { name: "", email: "" }, sequence: 0, status: "confirmed", myPartstat: "", myEmail: "",
  aliasReply: false, needsReview: false, taskappMeetingId: 0, taskappChange: "",
  attendees: [{ email: "b@x.com", name: "Beto", partstat: "NEEDS-ACTION" }], exceptions: [],
};

const INITIAL: EventInitial = {
  start: new Date("2026-11-20T13:00:00Z"), end: new Date("2026-11-20T14:00:00Z"), allDay: false,
};

type Setup = {
  connected?: boolean;
  workspaces?: { id: number; name: string }[];
  eventId?: number | null;
  event?: Record<string, unknown>;
  initial?: Partial<EventInitial>;
};

function setup({ connected = false, workspaces = [{ id: 1, name: "Sinergia" }], eventId = null, event = {}, initial = {} }: Setup = {}) {
  vi.mocked(svc.TaskappStatus).mockResolvedValue({ connected, userId: 7, userName: "Yo", userEmail: "yo@x.com" });
  vi.mocked(svc.TaskappWorkspaces).mockResolvedValue(workspaces);
  vi.mocked(svc.TaskappProjects).mockResolvedValue([{ id: 5, workspaceId: 1, name: "Web", color: "#f00" }]);
  vi.mocked(svc.TaskappMembers).mockResolvedValue([
    { id: 7, name: "Yo", email: "yo@x.com" },
    { id: 8, name: "Ana", email: "ana@x.com" },
  ]);
  vi.mocked(svc.ListAccounts).mockResolvedValue([
    { id: 3, email: "tres@x.com" },
    { id: 2, email: "dos@x.com" },
  ] as never);
  vi.mocked(svc.SuggestRecipients).mockResolvedValue([]);
  vi.mocked(svc.GetEvent).mockResolvedValue({ ...EVENT, ...event } as never);
  vi.mocked(svc.SaveEvent).mockResolvedValue({} as never);
  vi.mocked(svc.CreateTaskappMeeting).mockResolvedValue({} as never);
  const onOpenChange = vi.fn();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <EventDialog open onOpenChange={onOpenChange} initial={{ ...INITIAL, ...initial }} eventId={eventId} />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), onOpenChange };
}

const save = () => screen.getByRole("button", { name: "Guardar" });

describe("EventDialog", () => {
  beforeEach(() => vi.clearAllMocks());

  it("sin Taskapp no aparece el destino", async () => {
    setup();
    await screen.findByLabelText("Título");
    expect(screen.queryByText("Destino")).not.toBeInTheDocument();
    expect(screen.queryByText("Reunión en Taskapp")).not.toBeInTheDocument();
  });

  it("con Taskapp, el destino aparece al crear pero no al editar", async () => {
    setup({ connected: true });
    expect(await screen.findByText("Reunión en Taskapp")).toBeInTheDocument();
    expect(screen.getByText("Calendario local")).toBeInTheDocument();
  });

  it("al editar no ofrece el destino aunque Taskapp esté conectado", async () => {
    setup({ connected: true, eventId: 1 });
    await screen.findByDisplayValue("Reunión");
    await waitFor(() => expect(svc.TaskappStatus).toHaveBeenCalled());
    expect(screen.queryByText("Reunión en Taskapp")).not.toBeInTheDocument();
  });

  it("con el título vacío, Guardar está deshabilitado", async () => {
    const { user } = setup();
    const title = await screen.findByLabelText("Título");
    expect(save()).toBeDisabled();
    await user.type(title, "Almuerzo");
    expect(save()).toBeEnabled();
  });

  it("con invitados aparece Enviar desde, con la primera cuenta por defecto", async () => {
    const { user } = setup();
    await screen.findByLabelText("Título");
    expect(screen.queryByText("Enviar desde")).not.toBeInTheDocument();
    await user.type(screen.getByLabelText("Invitados"), "ana@x.com,");
    expect(await screen.findByRole("combobox", { name: "Enviar desde" })).toHaveTextContent("dos@x.com");
  });

  it("editar solo las notas no pregunta y guarda con notify false", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    const { user, onOpenChange } = setup({ eventId: 1 });
    const notes = await screen.findByLabelText("Notas privadas");
    await user.type(notes, "algo");
    await user.click(save());
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    expect(screen.queryByText("¿Avisar a los invitados?")).not.toBeInTheDocument();
    const req = vi.mocked(svc.SaveEvent).mock.calls[0][0];
    expect(req).toMatchObject({ scope: "", notify: false });
    expect(req.event).toMatchObject({ id: 1, notes: "algo" });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("editar la hora con invitados pregunta y Avisar manda notify true", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: true, added: 0, removed: 0 });
    const { user } = setup({ eventId: 1 });
    const time = await screen.findByLabelText("Hora de fin");
    fireEvent.change(time, { target: { value: "20:30" } });
    await user.click(save());
    await user.click(await screen.findByRole("button", { name: "Avisar" }));
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    expect(vi.mocked(svc.SaveEvent).mock.calls[0][0]).toMatchObject({ notify: true });
  });

  it("cancelar la pregunta deja el diálogo abierto y no guarda", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: true, added: 0, removed: 0 });
    const { user, onOpenChange } = setup({ eventId: 1 });
    await screen.findByLabelText("Título");
    await user.click(save());
    await screen.findByText("¿Avisar a los invitados?");
    await user.click(screen.getByRole("button", { name: "Cancelar" }));
    await waitFor(() => expect(screen.queryByText("¿Avisar a los invitados?")).not.toBeInTheDocument());
    expect(svc.SaveEvent).not.toHaveBeenCalled();
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(screen.getByLabelText("Título")).toBeInTheDocument();
  });

  it("un repetido pide el alcance y manda following con la ocurrencia original", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    const { user } = setup({
      eventId: 1,
      event: { rrule: "FREQ=WEEKLY;BYDAY=FR" },
      initial: { recurring: true, occurrenceAt: "2026-11-27T13:00:00Z", start: new Date("2026-11-27T13:00:00Z"), end: new Date("2026-11-27T14:00:00Z") },
    });
    const time = await screen.findByLabelText("Hora de fin");
    fireEvent.change(time, { target: { value: "23:00" } });
    await user.click(save());
    await user.click(await screen.findByRole("button", { name: "Este y los siguientes" }));
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    const req = vi.mocked(svc.SaveEvent).mock.calls[0][0];
    expect(req).toMatchObject({ scope: "following", occurrenceAt: "2026-11-27T13:00:00Z" });
    expect(req.event.start).toBe(new Date("2026-11-27T13:00:00Z").toISOString());
    expect(req.event.rrule).toBe("FREQ=WEEKLY;BYDAY=FR");
  });

  it("un alta con invitados manda sin preguntar", async () => {
    const { user } = setup();
    await user.type(await screen.findByLabelText("Título"), "Cena");
    await user.type(screen.getByLabelText("Invitados"), "ana@x.com,");
    await user.click(save());
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    expect(svc.PreviewEventSave).not.toHaveBeenCalled();
    const req = vi.mocked(svc.SaveEvent).mock.calls[0][0];
    expect(req.event).toMatchObject({ id: 0, title: "Cena", accountId: 2 });
    expect(req.event.attendees?.[0]).toMatchObject({ email: "ana@x.com" });
  });

  it("un día completo se manda como medianoche UTC con el fin exclusivo", async () => {
    const { user } = setup({
      initial: { allDay: true, start: new Date(2026, 10, 20), end: new Date(2026, 10, 21) },
    });
    await user.type(await screen.findByLabelText("Título"), "Feriado");
    await user.click(save());
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    const ev = vi.mocked(svc.SaveEvent).mock.calls[0][0].event;
    expect(ev).toMatchObject({ allDay: true, start: "2026-11-20T00:00:00.000Z", end: "2026-11-21T00:00:00.000Z" });
  });

  it("borrar un repetido pasa por el alcance y el aviso", async () => {
    vi.mocked(svc.PreviewEventDelete).mockResolvedValue({ notifyNeeded: true, added: 0, removed: 0 });
    const { user, onOpenChange } = setup({
      eventId: 1,
      event: { rrule: "FREQ=DAILY" },
      initial: { recurring: true, occurrenceAt: "2026-11-21T13:00:00Z" },
    });
    await screen.findByLabelText("Título");
    await user.click(screen.getByRole("button", { name: "Borrar" }));
    await user.click(await screen.findByRole("button", { name: "Solo este" }));
    await user.click(await screen.findByRole("button", { name: "No avisar" }));
    await waitFor(() => expect(svc.DeleteEvent).toHaveBeenCalledTimes(1));
    expect(vi.mocked(svc.DeleteEvent).mock.calls[0][0]).toEqual({
      id: 1, scope: "this", occurrenceAt: "2026-11-21T13:00:00Z", notify: false,
    });
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
  });

  it("tocar la repetición no ofrece Solo este", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    const { user } = setup({
      eventId: 1,
      event: { rrule: "FREQ=DAILY" },
      initial: { recurring: true, occurrenceAt: "2026-11-20T13:00:00Z" },
    });
    await user.click(await screen.findByRole("combobox", { name: "Repetición" }));
    await user.click(await screen.findByRole("option", { name: "Todos los meses" }));
    await user.click(save());
    await screen.findByRole("button", { name: "Este y los siguientes" });
    expect(screen.queryByRole("button", { name: "Solo este" })).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Este y los siguientes" }));
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event.rrule).toBe("FREQ=MONTHLY");
  });

  it("sin tocar la repetición manda la regla tal cual, aunque la arme el formulario", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    const rrule = "FREQ=WEEKLY;BYDAY=MO;UNTIL=20261220T120000Z";
    const { user } = setup({ eventId: 1, event: { rrule }, initial: { recurring: true, occurrenceAt: "2026-11-20T13:00:00Z" } });
    await screen.findByLabelText("Título");
    await user.click(save());
    await user.click(await screen.findByRole("button", { name: "Solo este" }));
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event.rrule).toBe(rrule);
  });

  it("si el borrado quedó hecho y falló el correo, avisa y cierra", async () => {
    vi.mocked(svc.PreviewEventDelete).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    const { user, onOpenChange } = setup({ eventId: 1 });
    vi.mocked(svc.DeleteEvent).mockRejectedValue(new Error("El evento se borró, pero no pudimos mandar las cancelaciones: x"));
    await screen.findByLabelText("Título");
    await user.click(screen.getByRole("button", { name: "Borrar" }));
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(toast.warning).toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("si el guardado quedó hecho y falló el correo, avisa y cierra", async () => {
    const { user, onOpenChange } = setup();
    vi.mocked(svc.SaveEvent).mockRejectedValue(new Error("El evento se guardó, pero no pudimos mandar las invitaciones: x"));
    await user.type(await screen.findByLabelText("Título"), "Cena");
    await user.click(save());
    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    expect(toast.warning).toHaveBeenCalled();
  });

  it("un evento con regla ajena la muestra como personalizada y la conserva", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    const { user } = setup({
      eventId: 1,
      event: { rrule: "FREQ=WEEKLY;INTERVAL=2" },
      initial: { recurring: true, occurrenceAt: "2026-11-20T13:00:00Z" },
    });
    expect(await screen.findByRole("combobox", { name: "Repetición" })).toHaveTextContent("Repetición personalizada");
    await user.click(save());
    await user.click(await screen.findByRole("button", { name: "Solo este" }));
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event.rrule).toBe("FREQ=WEEKLY;INTERVAL=2");
  });

  it("al editar una ocurrencia con textos propios muestra los de la ocurrencia", async () => {
    setup({
      eventId: 1,
      event: {
        rrule: "FREQ=WEEKLY", location: "Sala serie", description: "Desc serie",
        exceptions: [{
          recurrenceAt: "2026-11-27T13:00:00Z", cancelled: false, start: "2026-11-27T15:00:00Z",
          end: "2026-11-27T16:00:00Z", title: "Propia", location: "Sala propia", description: "Desc propia",
        }],
      },
      initial: {
        start: new Date("2026-11-27T15:00:00Z"), end: new Date("2026-11-27T16:00:00Z"),
        recurring: true, occurrenceAt: "2026-11-27T13:00:00.000Z",
      },
    });
    expect(await screen.findByLabelText("Título")).toHaveValue("Propia");
    expect(screen.getByLabelText("Lugar")).toHaveValue("Sala propia");
    expect(screen.getByLabelText("Descripción")).toHaveValue("Desc propia");
  });

  it("una ocurrencia sin excepción muestra los textos de la serie", async () => {
    setup({
      eventId: 1,
      event: { rrule: "FREQ=WEEKLY", location: "Sala serie" },
      initial: { recurring: true, occurrenceAt: "2026-11-27T13:00:00Z" },
    });
    expect(await screen.findByLabelText("Título")).toHaveValue("Reunión");
    expect(screen.getByLabelText("Lugar")).toHaveValue("Sala serie");
  });

  describe("reunión en Taskapp", () => {
    async function pickTaskapp(user: ReturnType<typeof userEvent.setup>) {
      await user.click(await screen.findByLabelText("Reunión en Taskapp"));
    }

    it("con un solo workspace y sin proyecto no muestra el selector de workspace", async () => {
      const { user } = setup({ connected: true });
      await pickTaskapp(user);
      await screen.findByRole("combobox", { name: "Proyecto" });
      expect(screen.queryByRole("combobox", { name: "Workspace" })).not.toBeInTheDocument();
    });

    it("con dos workspaces lo muestra", async () => {
      const { user } = setup({ connected: true, workspaces: [{ id: 1, name: "Sinergia" }, { id: 2, name: "Otro" }] });
      await pickTaskapp(user);
      expect(await screen.findByRole("combobox", { name: "Workspace" })).toBeInTheDocument();
    });

    it("crea la reunión con el usuario marcado, los externos y la cuenta", async () => {
      const { user, onOpenChange } = setup({ connected: true });
      await pickTaskapp(user);
      await user.type(screen.getByLabelText("Título"), "Kickoff");
      expect(await screen.findByLabelText("Yo")).toBeChecked();
      await user.click(screen.getByLabelText("Ana"));
      await user.type(screen.getByLabelText("Invitados externos"), "ext@y.com,");
      expect(await screen.findByRole("combobox", { name: "Enviar desde" })).toHaveTextContent("dos@x.com");
      await user.click(save());
      await waitFor(() => expect(svc.CreateTaskappMeeting).toHaveBeenCalledTimes(1));
      const req = vi.mocked(svc.CreateTaskappMeeting).mock.calls[0][0];
      expect(req).toMatchObject({ workspaceId: 1, projectId: 0, title: "Kickoff", allDay: false, attendeeIds: [7, 8], accountId: 2 });
      expect(req.external).toEqual([expect.objectContaining({ email: "ext@y.com" })]);
      await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
    });

    it("un error huérfano muestra el mensaje y cierra igual", async () => {
      const { user, onOpenChange } = setup({ connected: true });
      vi.mocked(svc.CreateTaskappMeeting).mockRejectedValue(new Error("La reunión se creó en Taskapp, pero no se pudo guardar; no la vuelvas a crear."));
      await pickTaskapp(user);
      await user.type(screen.getByLabelText("Título"), "Kickoff");
      await waitFor(() => expect(save()).toBeEnabled());
      await user.click(save());
      await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false));
      expect(toast.error).toHaveBeenCalledWith(expect.stringContaining("no la vuelvas a crear"), { duration: Infinity });
    });

    it("un error común muestra el mensaje y deja el diálogo abierto", async () => {
      const { user, onOpenChange } = setup({ connected: true });
      vi.mocked(svc.CreateTaskappMeeting).mockRejectedValue(new Error("Taskapp no responde"));
      await pickTaskapp(user);
      await user.type(screen.getByLabelText("Título"), "Kickoff");
      await waitFor(() => expect(save()).toBeEnabled());
      await user.click(save());
      await waitFor(() => expect(toast.error).toHaveBeenCalled());
      expect(onOpenChange).not.toHaveBeenCalled();
    });
  });
});

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { forwardRef, useImperativeHandle } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

type FcProps = {
  initialView: string;
  weekends?: boolean;
  views: Record<string, unknown>;
  eventDrop: (info: unknown) => void;
  eventResize: (info: unknown) => void;
  select: (info: unknown) => void;
};
const fc = vi.hoisted(() => ({
  props: {} as FcProps,
  api: { gotoDate: vi.fn(), changeView: vi.fn(), today: vi.fn(), prev: vi.fn(), next: vi.fn(), unselect: vi.fn() },
}));
vi.mock("@fullcalendar/react", () => ({
  default: forwardRef((props: FcProps, ref) => {
    fc.props = props;
    useImperativeHandle(ref, () => ({ getApi: () => fc.api }));
    return <div data-testid="fc" data-initial={props.initialView} />;
  }),
}));
vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ListCalendar: vi.fn(),
  GetEvent: vi.fn(),
  PreviewEventSave: vi.fn(),
  SaveEvent: vi.fn(),
  TaskappStatus: vi.fn(),
  TaskappWorkspaces: vi.fn(),
  ListAccounts: vi.fn(),
  SuggestRecipients: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { error: vi.fn(), warning: vi.fn(), success: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { CalendarView } from "./CalendarView";
import { calItem, calShellState } from "./fixtures";

function setup(state = calShellState()) {
  const user = userEvent.setup();
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CalendarView state={state} />
    </QueryClientProvider>,
  );
  return { user, state };
}

const range = (over: Record<string, unknown> = {}) => ({
  items: [calItem({ attendees: 1 })], taskappStale: false, taskappFetchedAt: "", taskappError: "", ...over,
});

describe("CalendarView", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    vi.mocked(svc.ListCalendar).mockResolvedValue(range() as never);
    vi.mocked(svc.TaskappStatus).mockResolvedValue({ connected: false, userId: 0, userName: "", userEmail: "" });
    vi.mocked(svc.ListAccounts).mockResolvedValue([]);
    vi.mocked(svc.GetEvent).mockResolvedValue({
      id: 1, uid: "u", title: "Reunión", start: "2026-11-20T13:00:00Z", end: "2026-11-20T14:00:00Z",
      allDay: false, rrule: "", notes: "", description: "", location: "", remindBefore: null, accountId: 0,
      attendees: [{ email: "b@x.com", name: "Beto", partstat: "NEEDS-ACTION" }],
    } as never);
  });

  it("arranca en semana laboral y recuerda el cambio a Mes", async () => {
    const { user } = setup();
    expect(screen.getByTestId("fc")).toHaveAttribute("data-initial", "workWeek");
    expect(fc.props.weekends).toBeUndefined();
    expect(fc.props.views.workWeek).toMatchObject({ type: "timeGrid", weekends: false });
    await user.click(screen.getByRole("button", { name: "Mes" }));
    expect(fc.api.changeView).toHaveBeenCalledWith("dayGridMonth");
    expect(localStorage.getItem("pc:calendar:view")).toBe("dayGridMonth");
  });

  it("con calendarDate y eventKey va a la fecha y muestra el detalle", async () => {
    setup(calShellState({ calendarDate: "2026-11-20T13:00:00Z", eventKey: "e:1:1700000000" }));
    await waitFor(() => expect(fc.api.gotoDate).toHaveBeenCalledWith(new Date("2026-11-20T13:00:00Z")));
    expect(await screen.findByRole("heading", { name: "Reunión" })).toBeInTheDocument();
  });

  it("sin selección muestra el aviso de elegir", () => {
    setup();
    expect(screen.getByText("Elegí un evento para ver el detalle")).toBeInTheDocument();
  });

  it("con Taskapp sin conexión se ve el aviso", async () => {
    vi.mocked(svc.ListCalendar).mockResolvedValue(
      range({ taskappStale: true, taskappFetchedAt: "2026-11-19T12:30:00Z" }) as never,
    );
    setup();
    expect(await screen.findByText(/Taskapp sin conexión · actualizado/)).toBeInTheDocument();
  });

  it("elegir un rango en la grilla abre el formulario de un evento nuevo", async () => {
    setup();
    await screen.findByTestId("fc");
    act(() => fc.props.select({ start: new Date("2026-11-24T14:00:00Z"), end: new Date("2026-11-24T15:00:00Z"), allDay: false }));
    expect(await screen.findByRole("heading", { name: "Nuevo evento" })).toBeInTheDocument();
    expect(fc.api.unselect).toHaveBeenCalled();
  });

  it("Editar en el detalle abre el formulario con el evento", async () => {
    const { user } = setup(calShellState({ eventKey: "e:1:1700000000" }));
    await user.click(await screen.findByRole("button", { name: "Editar" }));
    expect(await screen.findByRole("heading", { name: "Editar evento" })).toBeInTheDocument();
    expect(await screen.findByLabelText("Título")).toHaveValue("Reunión");
  });

  type Ev = { allDay: boolean; start: Date; end: Date | null };
  const TIMED: Ev = { allDay: false, start: new Date("2026-11-21T13:00:00Z"), end: new Date("2026-11-21T14:00:00Z") };

  function drop(ev: Ev = TIMED, item = calItem({ attendees: 1 }), handler: "eventDrop" | "eventResize" = "eventDrop") {
    const revert = vi.fn();
    const info = { revert, event: { id: item.key, ...ev, extendedProps: { item } } };
    return { revert, run: () => fc.props[handler](info) };
  }

  // Mueve sin invitados: guarda directo, sin preguntar el aviso.
  async function saved(ev: Ev, item = calItem(), handler: "eventDrop" | "eventResize" = "eventDrop") {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: false, added: 0, removed: 0 });
    vi.mocked(svc.SaveEvent).mockResolvedValue({} as never);
    const { user } = setup();
    await screen.findByTestId("fc");
    const d = drop(ev, item, handler);
    d.run();
    return { ...d, user };
  }

  it("arrastrar con invitados pregunta y No avisar guarda con notify false", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: true, added: 0, removed: 0 });
    vi.mocked(svc.SaveEvent).mockResolvedValue({} as never);
    const { user } = setup();
    await screen.findByTestId("fc");
    const { run, revert } = drop();
    run();
    await user.click(await screen.findByRole("button", { name: "No avisar" }));
    await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
    const req = vi.mocked(svc.SaveEvent).mock.calls[0][0];
    expect(req).toMatchObject({ scope: "", notify: false });
    expect(req.event.start).toBe("2026-11-21T13:00:00.000Z");
    expect(req.event.end).toBe("2026-11-21T14:00:00.000Z");
    expect(revert).not.toHaveBeenCalled();
  });

  it("cancelar la pregunta revierte y no guarda", async () => {
    vi.mocked(svc.PreviewEventSave).mockResolvedValue({ notifyNeeded: true, added: 0, removed: 0 });
    const { user } = setup();
    await screen.findByTestId("fc");
    const { run, revert } = drop();
    run();
    await screen.findByText("¿Avisar a los invitados?");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(revert).toHaveBeenCalled());
    expect(svc.SaveEvent).not.toHaveBeenCalled();
  });

  describe("mover y estirar", () => {
    it("arrastrar un día completo manda medianoche UTC con el fin exclusivo", async () => {
      await saved({ allDay: true, start: new Date(2026, 10, 21), end: new Date(2026, 10, 22) }, calItem({ allDay: true }));
      await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
      expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event).toMatchObject({
        allDay: true, start: "2026-11-21T00:00:00.000Z", end: "2026-11-22T00:00:00.000Z",
      });
    });

    it("un día completo sin fin dura un día", async () => {
      await saved({ allDay: true, start: new Date(2026, 10, 21), end: null }, calItem({ allDay: true }));
      await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
      expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event).toMatchObject({
        allDay: true, start: "2026-11-21T00:00:00.000Z", end: "2026-11-22T00:00:00.000Z",
      });
    });

    it("arrastrar uno con hora a la franja de todo el día lo vuelve de día completo", async () => {
      await saved({ allDay: true, start: new Date(2026, 10, 23), end: null });
      await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
      expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event).toMatchObject({
        allDay: true, start: "2026-11-23T00:00:00.000Z", end: "2026-11-24T00:00:00.000Z",
      });
    });

    it("y de la franja de todo el día a una hora le da una hora de duración", async () => {
      await saved({ allDay: false, start: new Date("2026-11-23T15:00:00Z"), end: null }, calItem({ allDay: true }));
      await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
      expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event).toMatchObject({
        allDay: false, start: "2026-11-23T15:00:00.000Z", end: "2026-11-23T16:00:00.000Z",
      });
    });

    it("estirar manda el nuevo fin", async () => {
      await saved(
        { allDay: false, start: new Date("2026-11-20T13:00:00Z"), end: new Date("2026-11-20T15:30:00Z") },
        calItem(),
        "eventResize",
      );
      await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
      expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].event).toMatchObject({
        start: "2026-11-20T13:00:00.000Z", end: "2026-11-20T15:30:00.000Z",
      });
    });

    const series = calItem({ recurring: true, occurrenceAt: "2026-11-27T13:00:00Z" });

    it("un repetido pregunta el alcance: Solo este manda la ocurrencia original y las horas nuevas", async () => {
      const { user, revert } = await saved(TIMED, series);
      await user.click(await screen.findByRole("button", { name: "Solo este" }));
      await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
      const req = vi.mocked(svc.SaveEvent).mock.calls[0][0];
      expect(req).toMatchObject({ scope: "this", occurrenceAt: "2026-11-27T13:00:00Z", notify: false });
      expect(req.event).toMatchObject({ start: "2026-11-21T13:00:00.000Z", end: "2026-11-21T14:00:00.000Z" });
      expect(revert).not.toHaveBeenCalled();
    });

    it("Este y los siguientes manda following", async () => {
      const { user } = await saved(TIMED, series);
      await user.click(await screen.findByRole("button", { name: "Este y los siguientes" }));
      await waitFor(() => expect(svc.SaveEvent).toHaveBeenCalledTimes(1));
      expect(vi.mocked(svc.SaveEvent).mock.calls[0][0]).toMatchObject({
        scope: "following", occurrenceAt: "2026-11-27T13:00:00Z",
      });
    });

    it("cancelar el alcance revierte y no guarda", async () => {
      const { user, revert } = await saved(TIMED, series);
      await screen.findByText("Evento repetido");
      await user.keyboard("{Escape}");
      await waitFor(() => expect(revert).toHaveBeenCalled());
      expect(svc.SaveEvent).not.toHaveBeenCalled();
    });
  });
});

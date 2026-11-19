import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { forwardRef, useImperativeHandle } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

type FcProps = {
  initialView: string;
  weekends?: boolean;
  views: Record<string, unknown>;
  eventDrop: (info: unknown) => void;
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
    vi.mocked(svc.GetEvent).mockResolvedValue({
      id: 1, uid: "u", title: "Reunión", start: "2026-11-20T13:00:00Z", end: "2026-11-20T14:00:00Z",
      allDay: false, attendees: [{ email: "b@x.com", name: "Beto", partstat: "NEEDS-ACTION" }],
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

  function drop() {
    const revert = vi.fn();
    const info = {
      revert,
      event: {
        id: "e:1:1700000000", allDay: false, start: new Date("2026-11-21T13:00:00Z"), end: new Date("2026-11-21T14:00:00Z"),
        extendedProps: { item: calItem({ attendees: 1 }) },
      },
    };
    return { revert, run: () => fc.props.eventDrop(info) };
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
});

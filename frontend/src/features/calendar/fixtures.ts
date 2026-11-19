import { vi } from "vitest";
import type { CalendarItem } from "../../../bindings/postacontrol/core/model/models";
import type { ShellState } from "../layout/MailShell";
import { taskShellState } from "../tasks/fixtures";

export function calItem(over: Partial<CalendarItem> = {}): CalendarItem {
  return {
    key: "e:1:1700000000", source: "local", eventId: 1, meetingId: 0, taskId: 0, title: "Reunión", location: "",
    start: "2026-11-20T13:00:00Z", end: "2026-11-20T14:00:00Z", allDay: false, recurring: false,
    occurrenceAt: "2026-11-20T13:00:00Z", color: "", projectName: "", partstat: "", needsReview: false,
    cancelled: false, attendees: 0, confirmed: 0, taskappChange: "", url: "", ...over,
  };
}

export function calShellState(over: Partial<ShellState> = {}): ShellState {
  return { ...taskShellState(null), view: { kind: "calendar" }, selectEvent: vi.fn(), openTask: vi.fn(), ...over };
}

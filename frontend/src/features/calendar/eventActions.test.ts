import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  SaveEvent: vi.fn(async () => ({})),
  PreviewEventSave: vi.fn(async () => ({ notifyNeeded: false, added: 0, removed: 0 })),
  DeleteEvent: vi.fn(async () => {}),
  PreviewEventDelete: vi.fn(async () => ({ notifyNeeded: false, added: 0, removed: 0 })),
}));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { CalendarEvent } from "../../../bindings/postacontrol/core/model/models";
import { deleteEventFlow, saveEventFlow } from "./eventActions";

const ask = { askScope: vi.fn(), askNotify: vi.fn() };

// Go lee los time.Time con time.Parse(RFC3339): un "" rechaza el pedido entero
// ("could not parse argument #0"). Sin ocurrencia va la hora cero de Go.
const GO_ZERO = "0001-01-01T00:00:00Z";

const event = { id: 0, title: "Reunión", start: "2026-11-20T13:00:00Z", end: "2026-11-20T14:00:00Z" } as CalendarEvent;

beforeEach(() => vi.clearAllMocks());

describe("eventActions: fechas que entiende Go", () => {
  it("un alta sin ocurrencia manda la hora cero, no un texto vacío", async () => {
    await saveEventFlow(ask, event, null);
    expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].occurrenceAt).toBe(GO_ZERO);
  });

  it("una ocurrencia vacía también se manda como hora cero", async () => {
    await saveEventFlow(ask, event, { recurring: false, occurrenceAt: "" });
    expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].occurrenceAt).toBe(GO_ZERO);
  });

  it("borrar un evento que no se repite manda la hora cero", async () => {
    await deleteEventFlow(ask, 7, { recurring: false, occurrenceAt: "" });
    expect(vi.mocked(svc.PreviewEventDelete).mock.calls[0][0].occurrenceAt).toBe(GO_ZERO);
    expect(vi.mocked(svc.DeleteEvent).mock.calls[0][0].occurrenceAt).toBe(GO_ZERO);
  });

  it("una ocurrencia real se manda tal cual", async () => {
    await saveEventFlow(ask, { ...event, id: 3 }, { recurring: false, occurrenceAt: "2026-11-20T13:00:00Z" });
    expect(vi.mocked(svc.SaveEvent).mock.calls[0][0].occurrenceAt).toBe("2026-11-20T13:00:00Z");
  });
});

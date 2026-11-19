import { beforeEach, describe, expect, it } from "vitest";
import type { CalendarItem } from "../../bindings/postacontrol/core/model/models";
import { calItem } from "../features/calendar/fixtures";
import { allDayDate, DARK_TEXT, LIGHT_TEXT, loadCalendarView, saveCalendarView, toFcEvent } from "./calendar";

const item = (over: Partial<CalendarItem>) => calItem(over);

describe("toFcEvent", () => {
  it("solo los locales no cancelados se pueden mover", () => {
    expect(toFcEvent(item({ source: "local" })).editable).toBe(true);
    expect(toFcEvent(item({ source: "local", cancelled: true })).editable).toBe(false);
    for (const source of ["invite", "taskapp", "task"]) {
      expect(toFcEvent(item({ source })).editable).toBe(false);
    }
  });

  it("marca las invitaciones pendientes y las canceladas", () => {
    expect(toFcEvent(item({ source: "invite", partstat: "NEEDS-ACTION" })).classNames).toContain("ev-invite-pending");
    expect(toFcEvent(item({ source: "invite", cancelled: true })).classNames).toContain("ev-cancelled");
    expect(toFcEvent(item({ source: "task" })).classNames).toContain("ev-task");
    expect(toFcEvent(item({ source: "local" })).classNames).toContain("ev-local");
  });

  it("en Taskapp usa el color del proyecto", () => {
    expect(toFcEvent(item({ source: "taskapp", color: "#ff0000" })).backgroundColor).toBe("#ff0000");
    expect(toFcEvent(item({ source: "local", color: "#ff0000" })).backgroundColor).toBeUndefined();
  });

  it("una reunión de Taskapp sin proyecto va en color neutro, no el de los locales", () => {
    const ev = toFcEvent(item({ source: "taskapp", color: "" }));
    expect(ev.classNames).toContain("ev-taskapp");
    expect(ev.classNames).not.toContain("ev-local");
    expect(ev.backgroundColor).toBeUndefined();
  });

  it("con color de proyecto, el texto contrasta con el fondo", () => {
    const light = toFcEvent(item({ source: "taskapp", color: "#ffeb3b" }));
    expect(light.backgroundColor).toBe("#ffeb3b");
    expect(light.borderColor).toBe("#ffeb3b");
    expect(light.textColor).toBe(DARK_TEXT);
    expect(light.classNames).not.toContain("ev-taskapp");
    expect(toFcEvent(item({ source: "taskapp", color: "#1e3a8a" })).textColor).toBe(LIGHT_TEXT);
    expect(toFcEvent(item({ source: "taskapp", color: "#fff" })).textColor).toBe(DARK_TEXT);
  });

  it("un día completo va como fecha, sin correrse de día", () => {
    const ev = toFcEvent(item({ allDay: true, start: "2026-11-20T00:00:00Z", end: "2026-11-21T00:00:00Z" }));
    expect(ev).toMatchObject({ id: "e:1:1700000000", allDay: true, start: "2026-11-20", end: "2026-11-21" });
  });
});

describe("allDayDate", () => {
  it("da la fecha UTC aunque la zona local esté atrás", () => {
    process.env.TZ = "America/Argentina/Buenos_Aires";
    expect(new Date("2026-11-20T00:00:00Z").getDate()).toBe(19);
    expect(allDayDate("2026-11-20T00:00:00Z")).toBe("2026-11-20");
  });
});

describe("vista guardada", () => {
  beforeEach(() => localStorage.clear());

  it("por defecto es la semana laboral, también con basura", () => {
    expect(loadCalendarView()).toBe("workWeek");
    localStorage.setItem("pc:calendar:view", "cualquiera");
    expect(loadCalendarView()).toBe("workWeek");
  });

  it("recuerda la elegida", () => {
    saveCalendarView("dayGridMonth");
    expect(loadCalendarView()).toBe("dayGridMonth");
  });
});

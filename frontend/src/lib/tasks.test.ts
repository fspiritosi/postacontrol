import { describe, expect, it } from "vitest";
import type { LocalTask, TaskappTask } from "../../bindings/postacontrol/core/model/models";
import { groupByDue, REMINDER_OPTIONS, toItems } from "./tasks";

function local(p: Partial<LocalTask>): LocalTask {
  return {
    id: 1, title: "Tarea", notes: "", dueDate: "", dueTime: "", remindBefore: null,
    sourceMessageId: "", sourceSubject: "", doneAt: null, createdAt: "", ...p,
  };
}

function remote(p: Partial<TaskappTask>): TaskappTask {
  return {
    id: 1, ticket: false, title: "Ticket", priority: "", dueDate: "", statusName: "", projectId: 0,
    projectName: "", projectColor: "", url: "", description: "", ...p,
  };
}

// Miércoles 18/11/2026 a las 12:00 (hora local).
const now = new Date(2026, 10, 18, 12, 0);

describe("groupByDue", () => {
  it("groups by due", () => {
    const items = toItems([
      local({ id: 1, title: "vencida", dueDate: "2026-11-18", dueTime: "09:00" }),
      local({ id: 2, title: "hoy", dueDate: "2026-11-18" }),
      local({ id: 3, title: "mañana", dueDate: "2026-11-19" }),
      local({ id: 4, title: "semana", dueDate: "2026-11-22" }),
      local({ id: 5, title: "luego", dueDate: "2026-11-23" }),
      local({ id: 6, title: "sin fecha" }),
      local({ id: 7, title: "ayer", dueDate: "2026-11-17" }),
    ], [], "all");
    const groups = groupByDue(items, now);
    expect(groups.map((g) => [g.id, g.label, g.items.map((i) => i.task.title)])).toEqual([
      ["overdue", "Vencidas", ["ayer", "vencida"]],
      ["today", "Hoy", ["hoy"]],
      ["tomorrow", "Mañana", ["mañana"]],
      ["week", "Esta semana", ["semana"]],
      ["later", "Más adelante", ["luego"]],
      ["none", "Sin fecha", ["sin fecha"]],
    ]);
  });

  it("timed task of today that already passed is overdue, later today is today", () => {
    const groups = groupByDue(toItems([
      local({ id: 1, title: "pasó", dueDate: "2026-11-18", dueTime: "11:59" }),
      local({ id: 2, title: "falta", dueDate: "2026-11-18", dueTime: "12:01" }),
    ], [], "local"), now);
    expect(groups.map((g) => [g.id, g.items.map((i) => i.task.title)])).toEqual([
      ["overdue", ["pasó"]],
      ["today", ["falta"]],
    ]);
  });

  it("week ends on the Sunday of the current week (Monday start)", () => {
    const items = toItems([
      local({ id: 1, title: "domingo", dueDate: "2026-11-22" }),
      local({ id: 2, title: "lunes siguiente", dueDate: "2026-11-23" }),
    ], [], "local");
    const groups = groupByDue(items, now);
    expect(groups.map((g) => [g.id, g.items.map((i) => i.task.title)])).toEqual([
      ["week", ["domingo"]],
      ["later", ["lunes siguiente"]],
    ]);
  });

  it("on Saturday tomorrow is Sunday and the week group is empty", () => {
    const sat = new Date(2026, 10, 21, 10, 0);
    const groups = groupByDue(toItems([
      local({ id: 1, title: "domingo", dueDate: "2026-11-22" }),
      local({ id: 2, title: "lunes", dueDate: "2026-11-23" }),
    ], [], "local"), sat);
    expect(groups.map((g) => [g.id, g.items.map((i) => i.task.title)])).toEqual([
      ["tomorrow", ["domingo"]],
      ["later", ["lunes"]],
    ]);
  });

  it("on Sunday nothing goes in week", () => {
    const sun = new Date(2026, 10, 22, 10, 0);
    const groups = groupByDue(toItems([
      local({ id: 1, title: "lunes", dueDate: "2026-11-23" }),
      local({ id: 2, title: "miércoles", dueDate: "2026-11-25" }),
    ], [], "local"), sun);
    expect(groups.map((g) => g.id)).toEqual(["tomorrow", "later"]);
  });

  it("omits empty groups", () => {
    const groups = groupByDue(toItems([local({ id: 1, title: "hoy", dueDate: "2026-11-18" })], [], "local"), now);
    expect(groups.map((g) => g.id)).toEqual(["today"]);
    expect(groupByDue([], now)).toEqual([]);
  });

  it("sorts within group by due and then title", () => {
    const groups = groupByDue(toItems([
      local({ id: 1, title: "Zeta", dueDate: "2026-11-19", dueTime: "10:00" }),
      local({ id: 2, title: "Beta", dueDate: "2026-11-19" }),
      local({ id: 3, title: "Ábaco", dueDate: "2026-11-19" }),
      local({ id: 4, title: "Alfa", dueDate: "2026-11-19", dueTime: "08:00" }),
      local({ id: 5, title: "Gamma", dueDate: "2026-11-19", dueTime: "08:00" }),
    ], [], "local"), now);
    expect(groups[0].items.map((i) => i.task.title)).toEqual(["Ábaco", "Beta", "Alfa", "Gamma", "Zeta"]);
  });
});

describe("toItems", () => {
  const locals = [local({ id: 7, title: "local", dueDate: "2026-11-19" })];
  const remotes = [remote({ id: 9, title: "remota", dueDate: "2026-11-19" })];

  it("filters by source", () => {
    expect(toItems(locals, remotes, "all").map((i) => i.key)).toEqual(["l:7", "t:9"]);
    expect(toItems(locals, remotes, "local").map((i) => i.key)).toEqual(["l:7"]);
    expect(toItems(locals, remotes, "taskapp").map((i) => i.key)).toEqual(["t:9"]);
  });

  it("taskapp items are date-only and local items keep hasTime", () => {
    const items = toItems([local({ id: 1, dueDate: "2026-11-19", dueTime: "08:30" })], remotes, "all");
    expect(items[0]).toMatchObject({ source: "local", hasTime: true });
    expect(items[0].due).toEqual(new Date(2026, 10, 19, 8, 30));
    expect(items[1]).toMatchObject({ source: "taskapp", hasTime: false });
    expect(items[1].due).toEqual(new Date(2026, 10, 19));
  });
});

describe("REMINDER_OPTIONS", () => {
  it("lists the minutes and labels", () => {
    expect(REMINDER_OPTIONS).toEqual([
      { minutes: 0, label: "A la hora" },
      { minutes: 5, label: "5 minutos antes" },
      { minutes: 15, label: "15 minutos antes" },
      { minutes: 30, label: "30 minutos antes" },
      { minutes: 60, label: "1 hora antes" },
      { minutes: 1440, label: "1 día antes" },
    ]);
  });
});

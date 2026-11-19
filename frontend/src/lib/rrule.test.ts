import { beforeAll, describe, expect, it } from "vitest";
import { buildRRule, describeRRule, parseRRule, type Repeat } from "./rrule";

beforeAll(() => {
  process.env.TZ = "America/Argentina/Buenos_Aires";
});

const start = new Date(2026, 10, 18, 10, 0); // miércoles 18/11/2026, hora local

describe("buildRRule", () => {
  it("sin repetición da vacío", () => {
    expect(buildRRule({ freq: "none", days: [], until: "" }, start)).toBe("");
  });

  it("semanal con días: 0 = lunes", () => {
    expect(buildRRule({ freq: "weekly", days: [0, 2], until: "" }, start)).toBe("FREQ=WEEKLY;BYDAY=MO,WE");
  });

  it("semanal sin días usa el día del inicio", () => {
    expect(buildRRule({ freq: "weekly", days: [], until: "" }, start)).toBe("FREQ=WEEKLY;BYDAY=WE");
    expect(buildRRule({ freq: "weekly", days: [], until: "" }, new Date(2026, 10, 22))).toBe("FREQ=WEEKLY;BYDAY=SU");
  });

  it("el UNTIL va a las 23:59:59 locales pasado a UTC", () => {
    expect(buildRRule({ freq: "weekly", days: [0, 2], until: "2026-12-20" }, start)).toBe(
      "FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20261221T025959Z",
    );
  });

  it("en un día completo el UNTIL es una fecha", () => {
    expect(buildRRule({ freq: "daily", days: [], until: "2026-12-20" }, start, true)).toBe("FREQ=DAILY;UNTIL=20261220");
  });

  it("diaria, mensual y anual", () => {
    expect(buildRRule({ freq: "daily", days: [], until: "" }, start)).toBe("FREQ=DAILY");
    expect(buildRRule({ freq: "monthly", days: [], until: "" }, start)).toBe("FREQ=MONTHLY");
    expect(buildRRule({ freq: "yearly", days: [], until: "" }, start)).toBe("FREQ=YEARLY");
  });
});

describe("parseRRule", () => {
  const cases: Repeat[] = [
    { freq: "weekly", days: [0, 2], until: "" },
    { freq: "weekly", days: [6], until: "2026-12-20" },
    { freq: "daily", days: [], until: "2027-01-31" },
    { freq: "monthly", days: [], until: "" },
    { freq: "yearly", days: [], until: "" },
  ];
  it.each(cases)("vuelve a dar lo que armó buildRRule: %j", (x) => {
    expect(parseRRule(buildRRule(x, start))).toEqual(x);
  });

  it("también la forma de fecha de un día completo", () => {
    expect(parseRRule("FREQ=DAILY;UNTIL=20261220")).toEqual({ freq: "daily", days: [], until: "2026-12-20" });
  });

  it("vacío es sin repetición", () => {
    expect(parseRRule("")).toEqual({ freq: "none", days: [], until: "" });
  });

  it("una regla ajena da null", () => {
    expect(parseRRule("FREQ=WEEKLY;INTERVAL=2")).toBeNull();
    expect(parseRRule("FREQ=MONTHLY;BYMONTHDAY=1,15")).toBeNull();
    expect(parseRRule("FREQ=DAILY;COUNT=5")).toBeNull();
  });
});

describe("describeRRule", () => {
  it("describe las reglas propias", () => {
    expect(describeRRule("FREQ=WEEKLY;BYDAY=MO,WE;UNTIL=20261221T025959Z")).toBe(
      "Cada semana: lun, mié · hasta 20/12/2026",
    );
    expect(describeRRule("FREQ=DAILY")).toBe("Cada día");
    expect(describeRRule("FREQ=MONTHLY")).toBe("Cada mes");
    expect(describeRRule("FREQ=YEARLY")).toBe("Cada año");
  });

  it("una regla ajena es personalizada", () => {
    expect(describeRRule("FREQ=WEEKLY;INTERVAL=2")).toBe("Repetición personalizada");
  });
});

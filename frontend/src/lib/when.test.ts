import { describe, expect, it } from "vitest";
import { formatWhen, fromLocalInput, toLocalInput, whenPresets } from "./when";

// Viernes 13/11/2026 a las 10:30 (hora local).
const fri = new Date(2026, 10, 13, 10, 30);

describe("whenPresets", () => {
  it("de mañana temprano ofrece esta tarde, mañana y el lunes", () => {
    expect(whenPresets(fri).map((p) => [p.label, p.at.toString()])).toEqual([
      ["Esta tarde, 18:00", new Date(2026, 10, 13, 18, 0).toString()],
      ["Mañana, 8:00", new Date(2026, 10, 14, 8, 0).toString()],
      ["El lunes, 8:00", new Date(2026, 10, 16, 8, 0).toString()],
    ]);
  });

  it("de noche no ofrece esta tarde, y el domingo el lunes es mañana", () => {
    const sunNight = new Date(2026, 10, 15, 21, 0);
    expect(whenPresets(sunNight).map((p) => p.label)).toEqual(["Mañana, 8:00", "El lunes que viene, 8:00"]);
    expect(whenPresets(sunNight)[1].at).toEqual(new Date(2026, 10, 23, 8, 0));
  });
});

describe("formatWhen", () => {
  it("dice hoy, mañana o el día", () => {
    expect(formatWhen(new Date(2026, 10, 13, 18, 0), fri)).toBe("hoy a las 18:00");
    expect(formatWhen(new Date(2026, 10, 14, 8, 5), fri)).toBe("mañana a las 8:05");
    expect(formatWhen(new Date(2026, 10, 16, 8, 0), fri)).toBe("el lun 16/11 a las 8:00");
  });
});

describe("datetime-local", () => {
  it("ida y vuelta en hora local", () => {
    expect(toLocalInput(new Date(2026, 0, 5, 9, 7))).toBe("2026-01-05T09:07");
    expect(fromLocalInput("2026-01-05T09:07")).toEqual(new Date(2026, 0, 5, 9, 7));
    expect(fromLocalInput("")).toBeNull();
  });
});

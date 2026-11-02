import { describe, expect, it } from "vitest";
import { markAll, NO_MARKS, rangeMark, toggleMark, type Marks } from "./selection";

describe("toggleMark", () => {
  it("con un correo abierto, marcar otro deja los dos marcados", () => {
    const m = toggleMark<number>(NO_MARKS, 3, 1);
    expect(m).toEqual({ keys: [1, 3], anchor: 3 });
  });

  it("sin nada abierto marca solo ese", () => {
    expect(toggleMark<number>(NO_MARKS, 3, null)).toEqual({ keys: [3], anchor: 3 });
  });

  it("si el abierto es el mismo, no se repite", () => {
    expect(toggleMark<number>(NO_MARKS, 1, 1)).toEqual({ keys: [1], anchor: 1 });
  });

  it("el abierto solo se suma cuando no hay nada marcado", () => {
    const m: Marks<number> = { keys: [2, 3], anchor: 3 };
    expect(toggleMark(m, 4, 1)).toEqual({ keys: [2, 3, 4], anchor: 4 });
  });

  it("sacar uno de dos deja uno", () => {
    const m: Marks<number> = { keys: [1, 3], anchor: 3 };
    expect(toggleMark(m, 3, 1)).toEqual({ keys: [1], anchor: 3 });
  });
});

describe("rangeMark", () => {
  const order = [1, 2, 3, 4, 5];

  it("marca del ancla hasta el clic, y reemplaza lo marcado", () => {
    const m: Marks<number> = { keys: [2, 5], anchor: 2 };
    expect(rangeMark(m, 4, order, null)).toEqual({ keys: [2, 3, 4], anchor: 2 });
  });

  it("también al revés", () => {
    const m: Marks<number> = { keys: [4], anchor: 4 };
    expect(rangeMark(m, 2, order, null)).toEqual({ keys: [2, 3, 4], anchor: 4 });
  });

  it("sin ancla parte del abierto", () => {
    expect(rangeMark<number>(NO_MARKS, 4, order, 2).keys).toEqual([2, 3, 4]);
  });

  it("sin ancla ni abierto marca solo ese", () => {
    expect(rangeMark<number>(NO_MARKS, 4, order, null).keys).toEqual([4]);
  });

  it("si el ancla ya no está en la lista, marca solo ese", () => {
    const m: Marks<number> = { keys: [9], anchor: 9 };
    expect(rangeMark(m, 3, order, null).keys).toEqual([3]);
  });
});

describe("markAll", () => {
  it("marca todos, en orden", () => {
    expect(markAll([5, 6, 7]).keys).toEqual([5, 6, 7]);
  });
});

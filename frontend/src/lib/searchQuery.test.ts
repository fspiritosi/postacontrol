import { describe, expect, it } from "vitest";
import { NO_FILTERS, activeFilterCount, joinQuery, splitQuery } from "./searchQuery";

describe("splitQuery / joinQuery", () => {
  it("separa los filtros conocidos del texto libre", () => {
    const { free, filters } = splitQuery(
      'presupuesto de:ana asunto:"factura marzo" tiene:adjunto es:no-leído desde:2026-10-01 hasta:31/10/2026 nota:x',
    );
    expect(free).toBe("presupuesto nota:x");
    expect(filters).toEqual({
      ...NO_FILTERS,
      from: "ana",
      subject: "factura marzo",
      attachment: true,
      unread: true,
      since: "2026-10-01",
      until: "2026-10-31",
    });
  });

  it("también entiende la forma en inglés y sin tildes", () => {
    const { filters } = splitQuery("from:a to:b is:unread is:starred has:attachment es:noleido");
    expect(filters).toMatchObject({ from: "a", to: "b", unread: true, flagged: true, attachment: true });
  });

  it("arma el texto con comillas donde hace falta", () => {
    expect(
      joinQuery("presupuesto", { ...NO_FILTERS, from: "Ana López", unread: true, until: "2026-10-31" }),
    ).toBe('presupuesto de:"Ana López" es:no-leído hasta:2026-10-31');
    expect(joinQuery("", NO_FILTERS)).toBe("");
  });

  it("ida y vuelta da lo mismo", () => {
    const text = 'hola de:ana para:"Juan P" tiene:adjunto es:destacado desde:2026-01-01';
    const { free, filters } = splitQuery(text);
    expect(joinQuery(free, filters)).toBe(text);
  });

  it("cuenta los filtros activos", () => {
    expect(activeFilterCount(NO_FILTERS)).toBe(0);
    expect(activeFilterCount({ ...NO_FILTERS, from: "a", unread: true })).toBe(2);
  });
});

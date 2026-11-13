import { describe, expect, it } from "vitest";
import { formatInviteWhen, parseIcs } from "./ics";

const invite = [
  "BEGIN:VCALENDAR",
  "METHOD:REQUEST",
  "BEGIN:VEVENT",
  "SUMMARY:Reunión de\\, presupuesto",
  "DTSTART:20261117T130000Z",
  "DTEND:20261117T140000Z",
  "LOCATION:Oficina 3\\; piso 2\\\\ fondo",
  "ORGANIZER;CN=Ana López:mailto:ana@x.com",
  "DESCRIPTION:Traer los números.\\nGracias",
  " , saludos",
  "END:VEVENT",
  "END:VCALENDAR",
].join("\r\n");

describe("parseIcs", () => {
  it("lee el evento, con texto escapado y líneas plegadas", () => {
    const ev = parseIcs(invite)!;
    expect(ev.summary).toBe("Reunión de, presupuesto");
    expect(ev.location).toBe("Oficina 3; piso 2\\ fondo");
    expect(ev.organizer).toBe("Ana López");
    expect(ev.description).toBe("Traer los números.\nGracias, saludos");
    expect(ev.start).toEqual(new Date(Date.UTC(2026, 10, 17, 13, 0)));
    expect(ev.end).toEqual(new Date(Date.UTC(2026, 10, 17, 14, 0)));
    expect(ev.allDay).toBe(false);
    expect(ev.cancelled).toBe(false);
  });

  it("todo el día, hora local, organizador sin nombre y cancelación", () => {
    const ev = parseIcs(
      "BEGIN:VCALENDAR\nMETHOD:CANCEL\nBEGIN:VEVENT\nSUMMARY:Feriado\nDTSTART;VALUE=DATE:20261208\nORGANIZER:mailto:b@x.com\nEND:VEVENT\nEND:VCALENDAR",
    )!;
    expect(ev.allDay).toBe(true);
    expect(ev.start).toEqual(new Date(2026, 11, 8));
    expect(ev.organizer).toBe("b@x.com");
    expect(ev.cancelled).toBe(true);
    const local = parseIcs("BEGIN:VEVENT\nDTSTART;TZID=America/Argentina/Buenos_Aires:20261117T100000\nEND:VEVENT")!;
    expect(local.start).toEqual(new Date(2026, 10, 17, 10, 0));
  });

  it("sin evento no hay invitación", () => {
    expect(parseIcs("hola")).toBeNull();
    expect(parseIcs("")).toBeNull();
  });
});

describe("formatInviteWhen", () => {
  it("arma el día y el horario", () => {
    expect(formatInviteWhen({ start: new Date(2026, 10, 17, 10, 0), end: new Date(2026, 10, 17, 11, 30), allDay: false })).toBe(
      "mar 17/11/2026, 10:00 a 11:30",
    );
    expect(formatInviteWhen({ start: new Date(2026, 11, 8), end: null, allDay: true })).toBe("mar 8/12/2026, todo el día");
    expect(formatInviteWhen({ start: null, end: null, allDay: false })).toBe("");
  });
});

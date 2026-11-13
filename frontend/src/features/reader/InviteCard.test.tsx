import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  AttachmentData: vi.fn(),
  SaveAttachment: vi.fn(() => Promise.resolve(true)),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { InviteCard, isCalendar } from "./InviteCard";

const att = { index: 1, filename: "invite.ics", mimeType: "text/calendar", size: 300, part: [2] };
const ics = "BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nSUMMARY:Reunión\r\nDTSTART:20261117T100000\r\nDTEND:20261117T110000\r\nLOCATION:Oficina\r\nORGANIZER;CN=Ana:mailto:ana@x.com\r\nEND:VEVENT\r\nEND:VCALENDAR";

// Como llega del núcleo: los bytes UTF-8 del adjunto, en base64.
const utf8Base64 = (s: string) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));

function setup(text = ics) {
  vi.mocked(svc.AttachmentData).mockResolvedValue({ filename: "invite.ics", mimeType: "text/calendar", data: utf8Base64(text) });
  render(
    <QueryClientProvider client={new QueryClient()}>
      <InviteCard messageId={5} attachment={att} />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("InviteCard", () => {
  beforeEach(() => vi.clearAllMocks());

  it("muestra qué, cuándo, dónde y quién, y guarda el .ics", async () => {
    const user = setup();
    const card = await screen.findByRole("region", { name: "Invitación" });
    expect(card).toHaveTextContent("Reunión");
    expect(card).toHaveTextContent("mar 17/11/2026, 10:00 a 11:00");
    expect(card).toHaveTextContent("Oficina");
    expect(card).toHaveTextContent("Organiza Ana");
    await user.click(screen.getByRole("button", { name: "Guardar .ics" }));
    expect(svc.SaveAttachment).toHaveBeenCalledWith(5, 1);
  });

  it("si no es una invitación no muestra nada", async () => {
    setup("no es un calendario");
    await waitFor(() => expect(svc.AttachmentData).toHaveBeenCalled());
    expect(screen.queryByRole("region", { name: "Invitación" })).toBeNull();
  });

  it("reconoce los adjuntos de calendario", () => {
    expect(isCalendar({ mimeType: "text/calendar; method=REQUEST", filename: "" })).toBe(true);
    expect(isCalendar({ mimeType: "application/octet-stream", filename: "Evento.ICS" })).toBe(true);
    expect(isCalendar({ mimeType: "application/pdf", filename: "a.pdf" })).toBe(false);
  });
});

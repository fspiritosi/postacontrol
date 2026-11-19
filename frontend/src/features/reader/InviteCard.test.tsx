import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MessageInvite } from "../../../bindings/postacontrol/core/model/models";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  InviteForMessage: vi.fn(),
  RespondInvite: vi.fn(),
  RemoveCancelledEvent: vi.fn(() => Promise.resolve()),
  SaveAttachment: vi.fn(() => Promise.resolve(true)),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() } }));
const openEvent = vi.fn();
vi.mock("@/lib/events", () => ({ requestOpenEvent: (r: unknown) => openEvent(r) }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { toast } from "sonner";
import { InviteCard, isCalendar } from "./InviteCard";

const base: MessageInvite = {
  found: true,
  method: "REQUEST",
  eventId: 7,
  title: "Demo",
  location: "Oficina",
  organizer: { name: "Ana", email: "ana@x.com" },
  start: "2026-11-20T13:00:00Z",
  end: "2026-11-20T14:00:00Z",
  allDay: false,
  recurring: false,
  partstat: "NEEDS-ACTION",
  needsReview: false,
  cancelled: false,
  past: false,
  outdated: false,
  aliasReply: false,
  replyFrom: { name: "", email: "" },
  replyStatus: "",
};

const att = { index: 1, filename: "invite.ics", mimeType: "text/calendar", size: 300, part: [2] };

function setup(over: Partial<MessageInvite> = {}, props: { attachment?: typeof att; accountEmail?: string } = {}) {
  vi.mocked(svc.InviteForMessage).mockResolvedValue({ ...base, ...over });
  const view = render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <InviteCard messageId={5} {...props} />
    </QueryClientProvider>,
  );
  return { user: userEvent.setup(), ...view };
}

describe("InviteCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.RespondInvite).mockResolvedValue({ warning: "" });
  });

  it("muestra título, lugar y organizador, y los botones de respuesta", async () => {
    setup({ recurring: true });
    const card = await screen.findByRole("region", { name: "Invitación" });
    expect(card).toHaveTextContent("Demo");
    expect(card).toHaveTextContent("se repite");
    expect(card).toHaveTextContent("Oficina");
    expect(card).toHaveTextContent("Organiza Ana");
    expect(screen.getByRole("button", { name: "Aceptar" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Tal vez" })).toBeEnabled();
    expect(screen.getByRole("button", { name: "Rechazar" })).toBeEnabled();
  });

  it("Aceptar responde ACCEPTED y muestra el aviso si vuelve", async () => {
    const { user } = setup();
    vi.mocked(svc.RespondInvite).mockResolvedValue({ warning: "No se pudo avisar al organizador" });
    await user.click(await screen.findByRole("button", { name: "Aceptar" }));
    expect(svc.RespondInvite).toHaveBeenCalledWith(7, "ACCEPTED");
    await waitFor(() => expect(toast.warning).toHaveBeenCalledWith("No se pudo avisar al organizador"));
  });

  it("Tal vez y Rechazar mandan su estado; un error sale en un toast", async () => {
    const { user } = setup();
    await user.click(await screen.findByRole("button", { name: "Tal vez" }));
    expect(svc.RespondInvite).toHaveBeenCalledWith(7, "TENTATIVE");
    vi.mocked(svc.RespondInvite).mockRejectedValue(new Error("falló"));
    await user.click(screen.getByRole("button", { name: "Rechazar" }));
    expect(svc.RespondInvite).toHaveBeenCalledWith(7, "DECLINED");
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });

  it("el estado actual queda marcado", async () => {
    setup({ partstat: "TENTATIVE" });
    expect(await screen.findByRole("button", { name: "Tal vez" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "Aceptar" })).toHaveAttribute("aria-pressed", "false");
  });

  it("needsReview pide volver a responder", async () => {
    setup({ needsReview: true, partstat: "ACCEPTED" });
    expect(await screen.findByText("Cambió la hora: volvé a responder")).toBeInTheDocument();
  });

  it("aliasReply sin cuenta conocida no nombra una cuenta vacía", async () => {
    setup({ aliasReply: true });
    expect(
      await screen.findByText(
        "Ninguna de tus cuentas figura como invitada; vas a responder desde la cuenta que recibió el correo",
      ),
    ).toBeInTheDocument();
  });

  it("aliasReply avisa desde qué cuenta se responde", async () => {
    setup({ aliasReply: true }, { accountEmail: "yo@x.com" });
    expect(
      await screen.findByText("Ninguna de tus cuentas figura como invitada; vas a responder desde yo@x.com"),
    ).toBeInTheDocument();
  });

  it("outdated no muestra botones de respuesta", async () => {
    setup({ outdated: true });
    expect(await screen.findByText("Hay una versión más nueva de esta invitación")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aceptar" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Rechazar" })).toBeNull();
  });

  it("past no muestra botones de respuesta", async () => {
    setup({ past: true });
    expect(await screen.findByText("Este evento ya pasó")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aceptar" })).toBeNull();
  });

  it("Ver en el calendario abre el evento por su inicio", async () => {
    const { user } = setup();
    await user.click(await screen.findByRole("button", { name: "Ver en el calendario" }));
    expect(openEvent).toHaveBeenCalledWith({
      key: `e:7:${Math.floor(Date.parse(base.start) / 1000)}`,
      at: base.start,
    });
  });

  it("un CANCEL ofrece quitar del calendario", async () => {
    const { user } = setup({ method: "CANCEL", cancelled: true });
    expect(await screen.findByText("Cancelada")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Aceptar" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Quitar del calendario" }));
    expect(svc.RemoveCancelledEvent).toHaveBeenCalledWith(7);
  });

  it("un REPLY dice quién respondió qué", async () => {
    setup({ method: "REPLY", replyFrom: { name: "Ana", email: "ana@x.com" }, replyStatus: "TENTATIVE" });
    expect(await screen.findByText("Ana respondió tal vez a la invitación a «Demo».")).toBeInTheDocument();
  });

  it("un REPLY sin nombre usa el email; aceptó y rechazó", async () => {
    const { unmount } = setup({ method: "REPLY", replyFrom: { name: "", email: "b@x.com" }, replyStatus: "ACCEPTED" });
    expect(await screen.findByText("b@x.com aceptó la invitación a «Demo».")).toBeInTheDocument();
    unmount();
    setup({ method: "REPLY", replyFrom: { name: "Bo", email: "b@x.com" }, replyStatus: "DECLINED" });
    expect(await screen.findByText("Bo rechazó la invitación a «Demo».")).toBeInTheDocument();
  });

  it("con found:false no renderiza nada", async () => {
    setup({ found: false });
    await waitFor(() => expect(svc.InviteForMessage).toHaveBeenCalled());
    expect(screen.queryByRole("region", { name: "Invitación" })).toBeNull();
  });

  it("guarda el .ics si hay adjunto de calendario", async () => {
    const { user } = setup({}, { attachment: att });
    await user.click(await screen.findByRole("button", { name: "Guardar .ics" }));
    expect(svc.SaveAttachment).toHaveBeenCalledWith(5, 1);
  });

  it("reconoce los adjuntos de calendario", () => {
    expect(isCalendar({ mimeType: "text/calendar; method=REQUEST", filename: "" })).toBe(true);
    expect(isCalendar({ mimeType: "application/octet-stream", filename: "Evento.ICS" })).toBe(true);
    expect(isCalendar({ mimeType: "application/pdf", filename: "a.pdf" })).toBe(false);
  });
});

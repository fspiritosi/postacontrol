import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(),
  SetSettings: vi.fn(() => Promise.resolve()),
  PickICSFiles: vi.fn(),
  PreviewICSImport: vi.fn(),
  ImportICS: vi.fn(),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { CalendarSection } from "./CalendarSection";

function setup() {
  vi.mocked(svc.GetSettings).mockResolvedValue({ eventReminderMinutes: null } as never);
  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <CalendarSection />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("CalendarSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Radix Select en jsdom.
    Element.prototype.hasPointerCapture = () => false;
    Element.prototype.setPointerCapture = () => {};
    Element.prototype.releasePointerCapture = () => {};
    Element.prototype.scrollIntoView = () => {};
  });

  it("cambiar el aviso guarda eventReminderMinutes", async () => {
    const user = setup();
    await waitFor(() => expect(screen.getByRole("combobox", { name: /Aviso de eventos/ })).toBeEnabled());
    await user.click(screen.getByRole("combobox", { name: /Aviso de eventos/ }));
    await user.click(await screen.findByRole("option", { name: /30 minutos/ }));
    await waitFor(() => expect(svc.SetSettings).toHaveBeenCalled());
    expect(vi.mocked(svc.SetSettings).mock.calls[0][0]).toMatchObject({ eventReminderMinutes: 30 });
  });

  it("deja sin marcar Feriados e importa los marcados", async () => {
    vi.mocked(svc.PickICSFiles).mockResolvedValue(["/x/cal.zip"]);
    vi.mocked(svc.PreviewICSImport).mockResolvedValue([
      { id: "a", name: "Trabajo", events: 3 },
      { id: "b", name: "Feriados en Argentina", events: 20 },
      { id: "c", name: "Cumpleaños", events: 5 },
    ]);
    vi.mocked(svc.ImportICS).mockResolvedValue({ added: 3, updated: 1, skipped: 2 });
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Importar .ics" }));
    expect(await screen.findByRole("checkbox", { name: /Trabajo/ })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Feriados/ })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Cumpleaños/ })).not.toBeChecked();
    expect(screen.getByText("3 eventos")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Importar 1 calendario" }));
    expect(svc.ImportICS).toHaveBeenCalledWith(["a"]);
    expect(await screen.findByText("Se importaron 3 eventos (1 actualizados, 2 ya estaban)")).toBeInTheDocument();
  });

  it("usa plural con varios calendarios y singular con un evento", async () => {
    vi.mocked(svc.PickICSFiles).mockResolvedValue(["/x/a.ics"]);
    vi.mocked(svc.PreviewICSImport).mockResolvedValue([
      { id: "a", name: "Trabajo", events: 1 },
      { id: "b", name: "Casa", events: 2 },
    ]);
    vi.mocked(svc.ImportICS).mockResolvedValue({ added: 1, updated: 0, skipped: 0 });
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Importar .ics" }));
    await user.click(await screen.findByRole("button", { name: "Importar 2 calendarios" }));
    expect(await screen.findByText("Se importó 1 evento (0 actualizados, 0 ya estaban)")).toBeInTheDocument();
  });

  it("si el selector devuelve una lista vacía no pasa nada", async () => {
    vi.mocked(svc.PickICSFiles).mockResolvedValue([]);
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Importar .ics" }));
    await waitFor(() => expect(svc.PickICSFiles).toHaveBeenCalled());
    expect(svc.PreviewICSImport).not.toHaveBeenCalled();
    expect(screen.queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("muestra el error si falla la importación", async () => {
    vi.mocked(svc.PickICSFiles).mockResolvedValue(["/x/a.ics"]);
    vi.mocked(svc.PreviewICSImport).mockRejectedValue(new Error("archivo ilegible"));
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Importar .ics" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("archivo ilegible");
  });
});

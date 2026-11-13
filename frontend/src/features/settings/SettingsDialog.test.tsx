import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  GetSettings: vi.fn(() => Promise.resolve({ unifiedInbox: false, disableNotifications: false, disableSpellcheck: false, disableUndoSend: false, threadedList: false })),
  SetSettings: vi.fn(() => Promise.resolve()),
  GetStartAtLogin: vi.fn(() => Promise.resolve(false)),
  SetStartAtLogin: vi.fn(() => Promise.resolve()),
  ListAccounts: vi.fn(() => Promise.resolve([])),
  ListSignatures: vi.fn(() => Promise.resolve([])),
  BackgroundImage: vi.fn(() => Promise.resolve("")),
  PickBackground: vi.fn(() => Promise.resolve(true)),
  ClearBackground: vi.fn(() => Promise.resolve()),
  ListLabels: vi.fn(() => Promise.resolve([])),
  CreateLabel: vi.fn(() => Promise.resolve()),
  ListImageSenders: vi.fn(() => Promise.resolve([])),
  UntrustImageSender: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import type { SettingsSection } from "@/lib/events";
import { SettingsDialog } from "./SettingsDialog";

function renderDialog(focusSection?: SettingsSection) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <SettingsDialog open onOpenChange={() => {}} focusSection={focusSection} />
    </QueryClientProvider>,
  );
}

// Radix pide APIs de puntero que jsdom no trae.
beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
});

describe("SettingsDialog", () => {
  beforeEach(() => vi.clearAllMocks());

  it("Escape en el nombre de la etiqueta nueva la cancela sin cerrar Configuración", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SettingsDialog open onOpenChange={onOpenChange} focusSection="labels" />
      </QueryClientProvider>,
    );
    await user.click(await screen.findByRole("button", { name: "Nueva etiqueta" }));
    await user.type(screen.getByLabelText("Nombre de la etiqueta nueva"), "Banco{Escape}");
    expect(screen.queryByLabelText("Nombre de la etiqueta nueva")).toBeNull();
    expect(onOpenChange).not.toHaveBeenCalled();
    expect(svc.CreateLabel).not.toHaveBeenCalled();
    // Afuera de un campo, Escape sí cierra.
    await user.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("el interruptor guarda la bandeja unificada", async () => {
    const user = userEvent.setup();
    render(
      <QueryClientProvider client={new QueryClient()}>
        <SettingsDialog open onOpenChange={() => {}} />
      </QueryClientProvider>,
    );
    const sw = await screen.findByRole("switch", { name: "Bandeja unificada" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    await user.click(sw);
    await waitFor(() => expect(svc.SetSettings).toHaveBeenCalledWith({ unifiedInbox: true, disableNotifications: false, disableSpellcheck: false, threadedList: false, readingPane: "", background: "", disableUndoSend: false }));
  });

  it("el interruptor de notificaciones guarda disableNotifications", async () => {
    const user = userEvent.setup();
    renderDialog();
    const sw = await screen.findByRole("switch", { name: "Notificaciones de correo nuevo" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    expect(sw.getAttribute("aria-checked")).toBe("true");
    await user.click(sw);
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({ unifiedInbox: false, disableNotifications: true, disableSpellcheck: false, threadedList: false, readingPane: "", background: "", disableUndoSend: false }),
    );
  });

  it("el interruptor del corrector guarda disableSpellcheck", async () => {
    const user = userEvent.setup();
    renderDialog("compose");
    const sw = await screen.findByRole("switch", { name: "Corregir ortografía al escribir" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    expect(sw.getAttribute("aria-checked")).toBe("true");
    await user.click(sw);
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({ unifiedInbox: false, disableNotifications: false, disableSpellcheck: true, threadedList: false, readingPane: "", background: "", disableUndoSend: false }),
    );
  });

  it("Iniciar al abrir sesión llama SetStartAtLogin", async () => {
    const user = userEvent.setup();
    renderDialog();
    const sw = await screen.findByRole("switch", { name: "Iniciar al abrir sesión" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    await user.click(sw);
    await waitFor(() => expect(svc.SetStartAtLogin).toHaveBeenCalledWith(true));
  });

  it("si SetStartAtLogin falla, vuelve atrás y avisa", async () => {
    const user = userEvent.setup();
    vi.mocked(svc.SetStartAtLogin).mockRejectedValueOnce(new Error("No se pudo"));
    renderDialog();
    const sw = await screen.findByRole("switch", { name: "Iniciar al abrir sesión" });
    await waitFor(() => expect(sw.hasAttribute("disabled")).toBe(false));
    await user.click(sw);
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    await waitFor(() => expect(sw.getAttribute("aria-checked")).toBe("false"));
  });

  it("elegir Derecha guarda readingPane y conserva el resto", async () => {
    const user = userEvent.setup();
    vi.mocked(svc.GetSettings).mockResolvedValueOnce({
      unifiedInbox: true,
      background: "",
      disableNotifications: true, disableSpellcheck: false, disableUndoSend: false,
      threadedList: true,
      readingPane: "",
    });
    renderDialog("reading");
    const trigger = await screen.findByRole("combobox", { name: "Panel de lectura" });
    await waitFor(() => expect(trigger.hasAttribute("disabled")).toBe(false));
    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: "Derecha" }));
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({
        unifiedInbox: true,
        background: "",
        disableNotifications: true, disableSpellcheck: false, disableUndoSend: false,
        threadedList: true,
        readingPane: "right",
      }),
    );
  });

  it("Elegir imagen… llama a PickBackground", async () => {
    const user = userEvent.setup();
    renderDialog("reading");
    await user.click(await screen.findByRole("button", { name: "Elegir imagen…" }));
    await waitFor(() => expect(svc.PickBackground).toHaveBeenCalled());
    // Recarga la configuración, que trae el nombre del fondo nuevo.
    await waitFor(() => expect(svc.GetSettings).toHaveBeenCalledTimes(2));
  });

  it("Quitar llama a ClearBackground y solo aparece con fondo puesto", async () => {
    renderDialog("reading");
    await screen.findByRole("button", { name: "Elegir imagen…" });
    expect(screen.queryByRole("button", { name: "Quitar" })).toBeNull();
  });

  it("con fondo muestra la miniatura y Quitar", async () => {
    vi.mocked(svc.GetSettings).mockResolvedValueOnce({
      unifiedInbox: false,
      disableNotifications: false, disableSpellcheck: false, disableUndoSend: false,
      threadedList: false,
      readingPane: "",
      background: "background-1.png",
    });
    vi.mocked(svc.BackgroundImage).mockResolvedValue("data:image/png;base64,AAAA");
    const user = userEvent.setup();
    renderDialog("reading");
    expect(await screen.findByAltText("Fondo actual")).toBeTruthy();
    await user.click(await screen.findByRole("button", { name: "Quitar" }));
    await waitFor(() => expect(svc.ClearBackground).toHaveBeenCalled());
  });

  it("arranca en General y la barra lateral cambia de sección", async () => {
    const user = userEvent.setup();
    renderDialog();
    const nav = screen.getByRole("navigation", { name: "Secciones de Configuración" });
    expect(within(nav).getByRole("button", { name: "General" }).getAttribute("aria-current")).toBe("page");
    expect(await screen.findByRole("switch", { name: "Bandeja unificada" })).toBeTruthy();
    expect(screen.queryByRole("combobox", { name: "Panel de lectura" })).toBeNull();

    await user.click(within(nav).getByRole("button", { name: "Lectura" }));
    expect(await screen.findByRole("combobox", { name: "Panel de lectura" })).toBeTruthy();
    expect(screen.queryByRole("switch", { name: "Bandeja unificada" })).toBeNull();
    expect(within(nav).getByRole("button", { name: "Lectura" }).getAttribute("aria-current")).toBe("page");

    await user.click(within(nav).getByRole("button", { name: "Reglas" }));
    expect(await screen.findByRole("heading", { name: "Reglas" })).toBeTruthy();
  });

  it("focusSection abre directo en esa sección", async () => {
    renderDialog("labels");
    expect(await screen.findByRole("heading", { name: "Etiquetas" })).toBeTruthy();
    expect(screen.queryByRole("switch", { name: "Bandeja unificada" })).toBeNull();
  });

  it("Lectura lista los remitentes con imágenes permitidas y deja quitarlos", async () => {
    vi.mocked(svc.ListImageSenders).mockResolvedValue(["avisos@banco.com"]);
    const user = userEvent.setup();
    renderDialog("reading");
    const list = await screen.findByRole("list", { name: "Remitentes con imágenes permitidas" });
    expect(within(list).getByText("avisos@banco.com")).toBeInTheDocument();
    await user.click(within(list).getByRole("button", { name: "Bloquear de nuevo las imágenes de avisos@banco.com" }));
    expect(svc.UntrustImageSender).toHaveBeenCalledWith("avisos@banco.com");
  });

  it("Redacción permite apagar deshacer envío", async () => {
    const user = userEvent.setup();
    renderDialog("compose");
    const sw = await screen.findByRole("switch", { name: "Deshacer envío" });
    await waitFor(() => expect(sw).toBeEnabled());
    expect(sw).toBeChecked();
    await user.click(sw);
    await waitFor(() => expect(svc.SetSettings).toHaveBeenCalledWith(expect.objectContaining({ disableUndoSend: true })));
  });
});

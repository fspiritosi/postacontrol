import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeAll, beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  TaskappStatus: vi.fn(),
  ConnectTaskapp: vi.fn(),
  DisconnectTaskapp: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(),
  SetSettings: vi.fn(() => Promise.resolve()),
  GetStartAtLogin: vi.fn(() => Promise.resolve(false)),
  SetStartAtLogin: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { GeneralPage } from "./SettingsPages";
import { TaskappSection } from "./TaskappSection";

const settings = {
  unifiedInbox: false, disableNotifications: false, disableSpellcheck: false, threadedList: false,
  readingPane: "", background: "", disableUndoSend: false, autoBackupDir: "", theme: "",
  taskReminderMinutes: null, eventReminderMinutes: null, taskappApiUrl: "", taskappWebUrl: "",
};
const off = { connected: false, userId: 0, userName: "", userEmail: "" };
const on = { connected: true, userId: 0, userName: "Ana", userEmail: "ana@ej.com" };

beforeAll(() => {
  Element.prototype.hasPointerCapture = () => false;
  Element.prototype.setPointerCapture = () => {};
  Element.prototype.releasePointerCapture = () => {};
  Element.prototype.scrollIntoView = () => {};
});

function setup(ui: React.ReactElement = <TaskappSection />) {
  render(<QueryClientProvider client={new QueryClient()}>{ui}</QueryClientProvider>);
  return userEvent.setup();
}

describe("TaskappSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.GetSettings).mockResolvedValue(settings);
  });

  it("desconectado: conectar llama a ConnectTaskapp y muestra la espera", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(off);
    vi.mocked(svc.ConnectTaskapp).mockReturnValue(new Promise(() => {}) as never);
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Conectar con Taskapp" }));
    expect(svc.ConnectTaskapp).toHaveBeenCalled();
    expect(await screen.findByText("Esperando la autorización en el navegador…")).toBeInTheDocument();
  });

  it("al autorizar avisa y recarga el estado", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(off);
    vi.mocked(svc.ConnectTaskapp).mockResolvedValue(on);
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Conectar con Taskapp" }));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("Conectado con Taskapp."));
  });

  it("un error de la conexión se avisa", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(off);
    vi.mocked(svc.ConnectTaskapp).mockRejectedValue(new Error("Taskapp rechazó el permiso."));
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Conectar con Taskapp" }));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("Taskapp rechazó el permiso."));
  });

  it("Cancelar vuelve al estado inicial y ignora el resultado tardío", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(off);
    let reject!: (e: Error) => void;
    vi.mocked(svc.ConnectTaskapp).mockReturnValue(new Promise((_, r) => (reject = r)) as never);
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Conectar con Taskapp" }));
    await user.click(await screen.findByRole("button", { name: "Cancelar" }));
    expect(await screen.findByRole("button", { name: "Conectar con Taskapp" })).toBeInTheDocument();
    reject(new Error("Se canceló la conexión con Taskapp."));
    await new Promise((r) => setTimeout(r, 10));
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("conectado: muestra el nombre y el correo", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(on);
    setup();
    expect(await screen.findByText("Conectado como Ana (ana@ej.com)")).toBeInTheDocument();
  });

  it("desconectar pide confirmación y llama a DisconnectTaskapp", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(on);
    const user = setup();
    await user.click(await screen.findByRole("button", { name: "Desconectar" }));
    expect(svc.DisconnectTaskapp).not.toHaveBeenCalled();
    const dialog = (await screen.findByRole("dialog")) as HTMLElement;
    await user.click(within(dialog).getByRole("button", { name: "Desconectar" }));
    await waitFor(() => expect(svc.DisconnectTaskapp).toHaveBeenCalled());
  });

  it("Avanzado guarda la URL al salir del campo", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(off);
    const user = setup();
    await user.click(await screen.findByText("Avanzado"));
    const input = screen.getByLabelText("URL de la API");
    await user.type(input, "https://api.ej.com");
    await user.tab();
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({ ...settings, taskappApiUrl: "https://api.ej.com" }),
    );
  });

  it("si el core rechaza la URL avisa y vuelve al valor anterior", async () => {
    vi.mocked(svc.TaskappStatus).mockResolvedValue(off);
    vi.mocked(svc.SetSettings).mockRejectedValue(new Error("La URL no es válida."));
    const user = setup();
    await user.click(await screen.findByText("Avanzado"));
    const input = screen.getByLabelText("URL de Taskapp");
    await user.type(input, "xx");
    await user.tab();
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("La URL no es válida."));
    expect(input).toHaveValue("");
  });
});

describe("General: aviso de tareas por defecto", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(svc.GetSettings).mockResolvedValue(settings);
  });

  it("muestra 15 minutos antes por defecto y guarda lo elegido", async () => {
    const user = setup(<GeneralPage />);
    const trigger = await screen.findByLabelText("Aviso de tareas por defecto");
    await waitFor(() => expect(trigger).toHaveTextContent("15 minutos antes"));
    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: "1 hora antes" }));
    await waitFor(() =>
      expect(svc.SetSettings).toHaveBeenCalledWith({ ...settings, taskReminderMinutes: 60 }),
    );
  });
});

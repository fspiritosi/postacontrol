import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

vi.mock("../../../bindings/postacontrol/app/mailservice", () => ({
  ExportBackup: vi.fn(),
  ImportBackup: vi.fn(),
  AutoBackupInfo: vi.fn(),
  PickAutoBackupDir: vi.fn(() => Promise.resolve(true)),
  ClearAutoBackupDir: vi.fn(() => Promise.resolve()),
  RunAutoBackupNow: vi.fn(() => Promise.resolve()),
}));
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

import { toast } from "sonner";
import * as svc from "../../../bindings/postacontrol/app/mailservice";
import { BackupSection, describeImport } from "./BackupSection";

const none = {
  contactsAdded: 0, contactsSkipped: 0, signaturesAdded: 0, signaturesSkipped: 0,
  labelsAdded: 0, labelsSkipped: 0, labelsFailed: 0, rulesAdded: 0, rulesSkipped: 0, rulesFailed: 0,
  imageSendersAdded: 0, imageSendersSkipped: 0, templatesAdded: 0, templatesSkipped: 0,
};

function setup(info = { dir: "", lastDate: "" }) {
  vi.mocked(svc.AutoBackupInfo).mockResolvedValue(info);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <BackupSection />
    </QueryClientProvider>,
  );
  return userEvent.setup();
}

describe("BackupSection", () => {
  beforeEach(() => vi.clearAllMocks());

  it("exportar avisa cuando se guardó", async () => {
    vi.mocked(svc.ExportBackup).mockResolvedValue(true);
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Exportar respaldo…" }));
    expect(svc.ExportBackup).toHaveBeenCalled();
    expect(toast.success).toHaveBeenCalledWith("Respaldo guardado.");
  });

  it("cancelar el diálogo de exportar no avisa nada", async () => {
    vi.mocked(svc.ExportBackup).mockResolvedValue(false);
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Exportar respaldo…" }));
    expect(toast.success).not.toHaveBeenCalled();
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("importar muestra el resumen de lo que se sumó", async () => {
    vi.mocked(svc.ImportBackup).mockResolvedValue({ ...none, contactsAdded: 2, rulesAdded: 1, contactsSkipped: 1 });
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Importar respaldo…" }));
    const status = await screen.findByRole("status");
    expect(status).toHaveTextContent("Se sumaron: 2 contactos y 1 regla.");
    expect(status).toHaveTextContent("Ya estaban y no se tocaron: 1 contacto.");
  });

  it("cancelar el diálogo de importar no muestra nada", async () => {
    vi.mocked(svc.ImportBackup).mockResolvedValue(null);
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Importar respaldo…" }));
    expect(screen.queryByRole("status")).toBeNull();
  });

  it("un error se avisa", async () => {
    vi.mocked(svc.ImportBackup).mockRejectedValue(new Error("Ese archivo no es un respaldo de PostaControl."));
    const user = setup();
    await user.click(screen.getByRole("button", { name: "Importar respaldo…" }));
    expect(toast.error).toHaveBeenCalledWith("Ese archivo no es un respaldo de PostaControl.");
  });
});

describe("BackupSection: respaldo automático", () => {
  beforeEach(() => vi.clearAllMocks());

  it("apagado: se activa eligiendo una carpeta", async () => {
    const user = setup();
    expect(await screen.findByText("Apagado.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Elegir carpeta…" }));
    expect(svc.PickAutoBackupDir).toHaveBeenCalled();
  });

  it("prendido: muestra la carpeta y la última copia, copia ahora y se apaga", async () => {
    const user = setup({ dir: "/home/yo/Drive/respaldos", lastDate: "2026-11-10" });
    expect(await screen.findByText("/home/yo/Drive/respaldos")).toBeInTheDocument();
    expect(screen.getByText(/Última copia: 10\/11\/2026/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Hacer una copia ahora" }));
    await waitFor(() => expect(svc.RunAutoBackupNow).toHaveBeenCalled());
    expect(toast.success).toHaveBeenCalledWith("Copia guardada.");
    await user.click(screen.getByRole("button", { name: "Apagar" }));
    expect(svc.ClearAutoBackupDir).toHaveBeenCalled();
  });
});

describe("describeImport", () => {
  it("sin nada nuevo lo dice", () => {
    expect(describeImport(none)).toEqual(["No había nada nuevo para sumar."]);
  });

  it("lista lo sumado, lo que ya estaba y lo que no se pudo", () => {
    expect(
      describeImport({ ...none, signaturesAdded: 1, labelsAdded: 3, rulesAdded: 2, imageSendersAdded: 2, labelsSkipped: 1, rulesFailed: 1 }),
    ).toEqual([
      "Se sumaron: 1 firma, 3 etiquetas, 2 reglas y 2 remitentes con imágenes permitidas.",
      "Ya estaban y no se tocaron: 1 etiqueta.",
      "No se pudieron restaurar: 1 regla. Falta la cuenta, la carpeta o la etiqueta de la que dependen, o ya se llegó al máximo.",
    ]);
  });
});
